// Package pproflogging for debug helper functions.
package pproflogging

import (
	"bufio"
	"bytes"
	"context"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"runtime/pprof"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kopia/kopia/repo/logging"
)

var log = logging.Module("kopia/pproflogging")

// ProfileName the name of the profile (see: runtime/pprof/Lookup).
type ProfileName string

const (
	pair = 2
	// PPROFDumpTimeout Set a time upper bound for dumps.
	PPROFDumpTimeout = 15 * time.Second
)

const (
	// DefaultDebugProfileRate default sample/data fraction for profile sample collection rates (1/x, where x is the
	// data fraction sample rate).
	DefaultDebugProfileRate = 100
	// DefaultDebugProfileDumpBufferSizeB default size of the pprof output buffer.
	DefaultDebugProfileDumpBufferSizeB = 1 << 17
)

const (
	// EnvVarKopiaDebugPprof environment variable that contains the pprof dump configuration.
	EnvVarKopiaDebugPprof = "KOPIA_DEBUG_PPROF"
)

// flags used to configure profiling in EnvVarKopiaDebugPprof.
const (
	// KopiaDebugFlagForceGc force garbage collection before dumping heap data.
	KopiaDebugFlagForceGc = "forcegc"
	// KopiaDebugFlagDebug value of the profiles `debug` parameter.
	KopiaDebugFlagDebug = "debug"
	// KopiaDebugFlagRate rate setting for the named profile (if available). always an integer.
	KopiaDebugFlagRate = "rate"
)

const (
	// ProfileNameBlock block profile key.
	ProfileNameBlock ProfileName = "block"
	// ProfileNameMutex mutex profile key.
	ProfileNameMutex = "mutex"
	// ProfileNameCPU cpu profile key.
	ProfileNameCPU = "cpu"
)

var (
	// ErrEmptyConfiguration returned when attempt to configure profile buffers without a configuration string.
	ErrEmptyConfiguration = errors.New("empty profile configuration")
	// ErrEmptyProfileName returned when a profile configuration flag has no argument.
	ErrEmptyProfileName = errors.New("empty profile flag")

	//nolint:gochecknoglobals
	pprofConfigs = newProfileConfigs(os.Stderr)
)

// Writer interface supports destination for PEM output.
type Writer interface {
	io.Writer
	io.StringWriter
}

// profileConfig configuration flags for a profile.
type profileConfig struct {
	flags []string
	buf   *bytes.Buffer
}

// profileConfigs configuration flags for all requested profiles.
type profileConfigs struct {
	mu sync.Mutex
	// +checklocks:mu
	wrt Writer
	// +checklocks:mu
	pcm map[ProfileName]*profileConfig
}

type pprofSetRate struct {
	setter       func(int)
	defaultValue int
}

//nolint:gochecknoglobals
var pprofProfileRates = map[ProfileName]pprofSetRate{
	ProfileNameBlock: {
		setter:       func(x int) { runtime.SetBlockProfileRate(x) },
		defaultValue: DefaultDebugProfileRate,
	},
	ProfileNameMutex: {
		setter:       func(x int) { runtime.SetMutexProfileFraction(x) },
		defaultValue: DefaultDebugProfileRate,
	},
}

// MaybeStartProfileBuffers start profile buffers for this process.
func MaybeStartProfileBuffers(ctx context.Context) {
	pcm, err := loadProfileConfig(ctx, os.Getenv(EnvVarKopiaDebugPprof))
	if err != nil {
		log(ctx).With("error", err).Debug("cannot start configured profile buffers")
		return
	}

	if pcm == nil {
		log(ctx).Debug("no profile buffer configuration to start")
		return
	}

	pprofConfigs.mu.Lock()
	defer pprofConfigs.mu.Unlock()

	pprofConfigs.pcm = pcm

	log(ctx).Debug("no profile buffer configuration to start")

	pprofConfigs.startProfileBuffers(ctx)
}

// MaybeStopProfileBuffers stop and dump the contents of the buffers to the log as PEMs.  Buffers
// supplied here are from MaybeStartProfileBuffers.
func MaybeStopProfileBuffers(ctx context.Context) {
	if pprofConfigs == nil || len(pprofConfigs.pcm) == 0 {
		log(ctx).Debug("no profile buffer configuration to stop")
		return
	}

	pprofConfigs.mu.Lock()
	defer pprofConfigs.mu.Unlock()

	pprofConfigs.stopProfileBuffers(ctx)
}

func newProfileConfigs(wrt Writer) *profileConfigs {
	q := &profileConfigs{
		wrt: wrt,
	}

	return q
}

func (p *profileConfigs) getProfileConfig(nm ProfileName) *profileConfig {
	if p == nil {
		return nil
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	return p.pcm[nm]
}

// Parse ppconfigs to configure profiling.
func loadProfileConfig(ctx context.Context, ppconfigss string) (map[ProfileName]*profileConfig, error) {
	// if empty, then don't bother configuring but emit a log message - use might be expecting them to be configured
	if ppconfigss == "" {
		return nil, nil
	}

	bufSizeB := DefaultDebugProfileDumpBufferSizeB

	// look for matching services.  "*" signals all services for profiling
	log(ctx).Info("configuring profile buffers")

	// acquire global lock when performing operations with global side-effects
	return parseProfileConfigs(bufSizeB, ppconfigss)
}

// getValue get the value of the named flag, `s`.  False will be returned
// if the flag does not exist. True will be returned if flag exists without
// a value.
func (p *profileConfig) getValue(s string) (string, bool) {
	if p == nil {
		return "", false
	}

	for _, f := range p.flags {
		kvs := strings.SplitN(f, "=", pair)
		if kvs[0] != s {
			continue
		}

		if len(kvs) == 1 {
			return "", true
		}

		return kvs[1], true
	}

	return "", false
}

// parseProfileConfigs.
func parseProfileConfigs(bufSizeB int, ppconfigs string) (map[ProfileName]*profileConfig, error) {
	pbs := map[ProfileName]*profileConfig{}
	allProfileOptions := strings.Split(ppconfigs, ":")

	for _, profileOptionWithFlags := range allProfileOptions {
		// of those, see if any have profile specific settings
		profileFlagNameValuePairs := strings.SplitN(profileOptionWithFlags, "=", pair)
		flagValue := ""

		if len(profileFlagNameValuePairs) == 0 {
			return nil, ErrEmptyConfiguration
		} else if len(profileFlagNameValuePairs) > 1 {
			// only <key>=<value? allowed
			flagValue = profileFlagNameValuePairs[1]
		}

		flagKey := ProfileName(profileFlagNameValuePairs[0])
		if flagKey == "" {
			return nil, ErrEmptyProfileName
		}

		pbs[flagKey] = newProfileConfig(bufSizeB, flagValue)
	}

	return pbs, nil
}

// newProfileConfig create a new profiling configuration.
func newProfileConfig(bufSizeB int, ppconfig string) *profileConfig {
	q := &profileConfig{
		buf: bytes.NewBuffer(make([]byte, 0, bufSizeB)),
	}

	flgs := strings.Split(ppconfig, ",")
	if len(flgs) > 0 && flgs[0] != "" { // len(flgs) > 1 && flgs[0] == "" should never happen
		q.flags = flgs
	}

	return q
}

// setupProfileFractions somewhat complex setup for profile buffers.  The intent
// is to implement a generic method for setting up _any_ pprofule.  This is done
// in anticipation of using different or custom profiles.
func setupProfileFractions(ctx context.Context, profileBuffers map[ProfileName]*profileConfig) {
	for k, pprofset := range pprofProfileRates {
		v, ok := profileBuffers[k]
		if !ok {
			// profile not configured - leave it alone
			continue
		}

		if v == nil {
			// profile configured, but no rate - set to default
			pprofset.setter(pprofset.defaultValue)
			continue
		}

		s, _ := v.getValue(KopiaDebugFlagRate)
		if s == "" {
			// flag without an argument - set to default
			pprofset.setter(pprofset.defaultValue)
			continue
		}

		n1, err := strconv.Atoi(s)
		if err != nil {
			log(ctx).With("cause", err).Warnf("invalid PPROF rate, %q, for %s: %v", s, k)
			continue
		}

		log(ctx).Debugf("setting PPROF rate, %d, for %s", n1, k)
		pprofset.setter(n1)
	}
}

// clearProfileFractions set the profile fractions to their zero values.
func clearProfileFractions(profileBuffers map[ProfileName]*profileConfig) {
	for k, pprofset := range pprofProfileRates {
		v := profileBuffers[k]
		if v == nil { // fold missing values and empty values
			continue
		}

		_, ok := v.getValue(KopiaDebugFlagRate)
		if !ok { // only care if a value might have been set before
			continue
		}

		pprofset.setter(0)
	}
}

// startProfileBuffers start profile buffers for enabled profiles/trace.  Buffers
// are returned in a slice of buffers: CPU, Heap and trace respectively.  class
// is used to distinguish profiles external to kopia.
func (p *profileConfigs) startProfileBuffers(ctx context.Context) {
	// profiling rates need to be set before starting profiling
	setupProfileFractions(ctx, p.pcm)

	// cpu has special initialization
	v, ok := p.pcm[ProfileNameCPU]
	if !ok {
		return
	}

	err := pprof.StartCPUProfile(v.buf)
	if err != nil {
		delete(p.pcm, ProfileNameCPU)
		log(ctx).With("cause", err).Warn("cannot start cpu PPROF")
	}
}

// dumpPEM dump a PEM version of the byte slice, bs, into writer, wrt.
func dumpPEM(ctx context.Context, bs []byte, types string, wrt Writer) error {
	// err0 for background process
	var err0 error

	blk := &pem.Block{
		Type:  types,
		Bytes: bs,
	}
	// wrt is likely a line oriented writer, so writing individual lines
	// will make best use of output buffer and help prevent overflows or
	// stalls in the output path.
	pr, pw := io.Pipe()
	// encode PEM in the background and output in a line oriented
	// fashion - this prevents the need for a large buffer to hold
	// the encoded PEM.
	go func() {
		// writer close on exit of background process
		//nolint:errcheck
		defer pw.Close()
		// do the encoding
		err0 = pem.Encode(pw, blk)
		if err0 != nil {
			return
		}
	}()

	// connect rdr to pipe reader
	rdr := bufio.NewReader(pr)

	// err1 for reading
	// err2 for writing
	// err3 for context
	var err1, err2, err3 error
	err3 = ctx.Err()

	for err1 == nil && err2 == nil && err3 == nil {
		var ln []byte
		ln, err1 = rdr.ReadBytes('\n')
		// err1 can return ln and non-nil err1, so always call write
		_, err2 = wrt.Write(ln)
		err3 = ctx.Err()
	}

	// got a context error.  this has precedent
	if err3 != nil {
		return fmt.Errorf("could not write PEM: %w", err3)
	}

	// got a write error.
	if err2 != nil {
		return fmt.Errorf("could not write PEM: %w", err2)
	}

	// did not get a read error.  file ends in newline
	if err1 == nil {
		return nil
	}

	// if file does not end in newline, then output one
	if errors.Is(err1, io.EOF) {
		_, err2 = wrt.WriteString("\n")
		if err2 != nil {
			return fmt.Errorf("could not write PEM: %w", err2)
		}

		return nil
	}

	return fmt.Errorf("error reading bytes: %w", err1)
}

func parseDebugNumber(v *profileConfig) (int, error) {
	debugs, ok := v.getValue(KopiaDebugFlagDebug)
	if !ok {
		return 0, nil
	}

	debug, err := strconv.Atoi(debugs)
	if err != nil {
		return 0, fmt.Errorf("could not parse number %q: %w", debugs, err)
	}

	return debug, nil
}

// stopProfileBuffers stop and dump the contents of the buffers to the log as PEMs.  Buffers
// supplied here are from MaybeStartProfileBuffers.
func (p *profileConfigs) stopProfileBuffers(ctx context.Context) {
	defer func() {
		// clear the profile rates and fractions to effectively stop profiling
		clearProfileFractions(p.pcm)
		p.pcm = map[ProfileName]*profileConfig{}
	}()

	log(ctx).Debugf("saving %d PEM buffers for output", len(p.pcm))

	// cpu and heap profiles requires special handling
	for nm, v := range p.pcm {
		if v == nil {
			// silently ignore empty profiles
			continue
		}

		log(ctx).Debugf("stopping PPROF profile %q", nm)

		_, ok := v.getValue(KopiaDebugFlagForceGc)
		if ok {
			log(ctx).Debug("performing GC before PPROF dump ...")
			runtime.GC()
		}

		// stop CPU profile after GC
		if nm == ProfileNameCPU {
			pprof.StopCPUProfile()
		} else {
			// look up the profile.  must not be nil
			pent := pprof.Lookup(string(nm))
			if pent == nil {
				log(ctx).Warnf("no system PPROF entry for %q", nm)
				delete(p.pcm, nm)

				continue
			}

			// parse the debug number if supplied
			debug, err := parseDebugNumber(v)
			if err != nil {
				log(ctx).With("cause", err).Warnf("%q: invalid PPROF configuration debug number", nm)
				continue
			}

			// write the profile data to the buffer associated with the profile
			err = pent.WriteTo(v.buf, debug)
			if err != nil {
				log(ctx).With("cause", err).Warnf("%q: error writing PPROF buffer", nm)
				continue
			}
		}
	}

	// dump the profiles out into their respective PEMs
	for k, v := range p.pcm {
		// PEM headings always in upper case
		unm := strings.ToUpper(string(k))

		// process context
		err := ctx.Err()
		if err != nil {
			// ctx context may be bad, so use context.Background for safety
			log(ctx).With("cause", err).Warnf("%q: cannot write PEM", unm)
			return
		}

		if v == nil {
			continue
		}

		log(ctx).Infof("dumping PEM for %q", unm)

		err = dumpPEM(ctx, v.buf.Bytes(), unm, p.wrt)
		if err != nil {
			log(ctx).With("cause", err).Errorf("%q: cannot write PEM", unm)
			return
		}
	}
}