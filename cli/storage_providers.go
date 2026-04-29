package cli

import (
	"context"
	"io"
	"sync"

	"github.com/alecthomas/kingpin/v2"

	"github.com/kopia/kopia/repo/blob"
	"github.com/kopia/kopia/repo/blob/throttling"
)

// StorageProviderServices is implemented by the cli App that allows the cli
// and tests to mutate the default storage providers.
type StorageProviderServices interface {
	EnvName(s string) string
	setPasswordFromToken(pwd string)
	storageProviders() []StorageProvider
	stdin() io.Reader
}

// StorageFlags is implemented by cli storage providers which need to support a
// particular backend. This requires the common setup and connection methods
// implemented by all the cli storage providers.
type StorageFlags interface {
	Setup(sps StorageProviderServices, cmd *kingpin.CmdClause)
	Connect(ctx context.Context, isCreate bool, formatVersion int) (blob.Storage, error)
}

// StorageProvider is a CLI provider for storage options and allows the CLI to
// multiplex between various provider CLI flag constructors.
type StorageProvider struct {
	Name        string
	Description string
	NewFlags    func() StorageFlags
}

//nolint:gochecknoglobals
var (
	registeredProviders   []StorageProvider
	registeredProvidersMu sync.Mutex
)

// AddSupportedStorageProvider registers a storage provider for use with the CLI repository connect command.
// This function is similar to blob.AddSupportedStorage but for CLI storage providers.
// It should typically be called from init() functions in storage provider packages.
func AddSupportedStorageProvider(name, description string, newFlags func() StorageFlags) {
	registeredProvidersMu.Lock()
	defer registeredProvidersMu.Unlock()

	registeredProviders = append(registeredProviders, StorageProvider{
		Name:        name,
		Description: description,
		NewFlags:    newFlags,
	})
}

// GetRegisteredStorageProviders returns a copy of all registered storage providers.
// This is used internally by the App to build the list of available storage providers.
func GetRegisteredStorageProviders() []StorageProvider {
	registeredProvidersMu.Lock()
	defer registeredProvidersMu.Unlock()

	// Return a copy to prevent external modification
	providers := make([]StorageProvider, len(registeredProviders))
	copy(providers, registeredProviders)

	return providers
}

func commonThrottlingFlags(cmd *kingpin.CmdClause, limits *throttling.Limits) {
	cmd.Flag("max-download-speed", "Limit the download speed.").PlaceHolder("BYTES_PER_SEC").FloatVar(&limits.DownloadBytesPerSecond)
	cmd.Flag("max-upload-speed", "Limit the upload speed.").PlaceHolder("BYTES_PER_SEC").FloatVar(&limits.UploadBytesPerSecond)
}

// AddStorageProvider adds a new StorageProvider at runtime after the App has
// been initialized with the default providers. This is used in tests which
// require custom storage providers to simulate various edge cases.
func (c *App) AddStorageProvider(p StorageProvider) {
	c.cliStorageProviders = append(c.cliStorageProviders, p)
}

func (c *App) storageProviders() []StorageProvider {
	return c.cliStorageProviders
}
