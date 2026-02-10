package filesystem

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kopia/kopia/internal/gather"
	"github.com/kopia/kopia/internal/testlogging"
	"github.com/kopia/kopia/internal/testutil"
	"github.com/kopia/kopia/repo/blob"
	"github.com/kopia/kopia/repo/blob/sharded"
)

type verifySyncBeforeCloseFile struct {
	osWriteFile

	fatal func(args ...any)

	mu    sync.Mutex
	dirty bool
}

func (vf *verifySyncBeforeCloseFile) Write(p []byte) (n int, err error) {
	vf.mu.Lock()
	defer vf.mu.Unlock()

	vf.dirty = true

	return vf.osWriteFile.Write(p)
}

func (vf *verifySyncBeforeCloseFile) Sync() error {
	vf.mu.Lock()
	defer vf.mu.Unlock()

	err := vf.osWriteFile.Sync()
	if err == nil {
		vf.dirty = false
	}

	fmt.Println("sync err:", err)

	return err
}

func (vf *verifySyncBeforeCloseFile) Close() error {
	vf.mu.Lock()
	defer vf.mu.Unlock()

	err := vf.osWriteFile.Close()

	if vf.dirty {
		vf.fatal("close called without calling sync after a write")
	}

	return err
}

// These tests reuse the retry/error-count mock to assert sync handling in PutBlob.
func TestPutBlob_SyncBeforeClose(t *testing.T) {
	t.Parallel()

	ctx := testlogging.Context(t)
	dataDir := testutil.TempDirectory(t)

	osi := newMockOS()

	osi.wrapNewFile = func(wf osWriteFile) osWriteFile {
		return &verifySyncBeforeCloseFile{
			osWriteFile: wf,
			fatal:       t.Fatal,
		}
	}

	st, err := New(ctx, &Options{
		Path:    dataDir,
		Options: sharded.Options{DirectoryShards: []int{1}},

		osInterfaceOverride: osi,
	}, true)
	require.NoError(t, err)

	t.Cleanup(func() { _ = st.Close(ctx) })

	require.NoError(t, st.PutBlob(ctx, "blob-sync-ok", gather.FromSlice([]byte("hello")), blob.PutOptions{}))

	var buf gather.WriteBuffer
	t.Cleanup(buf.Close)

	require.NoError(t, st.GetBlob(ctx, "blob-sync-ok", 0, -1, &buf))
	require.Equal(t, []byte("hello"), buf.ToByteSlice())
}

func TestPutBlob_FailsOnSyncError(t *testing.T) {
	t.Parallel()

	ctx := testlogging.Context(t)
	dataDir := testutil.TempDirectory(t)

	osi := newMockOS()

	st, err := New(ctx, &Options{
		Path:    dataDir,
		Options: sharded.Options{DirectoryShards: []int{1}},

		osInterfaceOverride: osi,
	}, true)
	require.NoError(t, err)
	t.Cleanup(func() { _ = st.Close(ctx) })

	// Test HACK: write a dummy blob to force writing the sharding configuration file, so writing the
	// config file does not interfere with the test. While this is coupled to the specifics of the
	// current implementation, it is required to be able to test the failure case.
	err = st.PutBlob(ctx, "dummy", gather.FromSlice([]byte("hello")), blob.PutOptions{})
	require.NoError(t, err)

	// Inject a failure per create (re-)try, 10 is the default number of retries
	osi.writeFileSyncRemainingErrors.Store(10)

	err = st.PutBlob(ctx, "blob-sync-fail", gather.FromSlice([]byte("hello")), blob.PutOptions{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "can't sync temporary file data")

	_, merr := st.GetMetadata(ctx, "blob-sync-fail")
	require.ErrorIs(t, merr, blob.ErrBlobNotFound)
}
