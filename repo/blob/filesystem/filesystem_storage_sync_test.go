package filesystem

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kopia/kopia/internal/gather"
	"github.com/kopia/kopia/internal/testlogging"
	"github.com/kopia/kopia/internal/testutil"
	"github.com/kopia/kopia/repo/blob"
	"github.com/kopia/kopia/repo/blob/sharded"
)

// These tests reuse the retry/error-count mock to assert sync handling in PutBlob.
func TestPutBlob_SyncCalledAndWriteSucceeds(t *testing.T) {
	t.Parallel()

	ctx := testlogging.Context(t)
	dataDir := testutil.TempDirectory(t)

	osi := newMockOS()

	st, err := New(ctx, &Options{
		Path:    dataDir,
		Options: sharded.Options{DirectoryShards: []int{1}},
	}, true)
	require.NoError(t, err)
	t.Cleanup(func() { _ = st.Close(ctx) })

	asFsImpl(t, st).osi = osi

	require.NoError(t, st.PutBlob(ctx, "blob-sync-ok", gather.FromSlice([]byte("hello")), blob.PutOptions{}))

	var buf gather.WriteBuffer
	t.Cleanup(buf.Close)

	require.NoError(t, st.GetBlob(ctx, "blob-sync-ok", 0, -1, &buf))
	require.Equal(t, []byte("hello"), buf.ToByteSlice())
}

func TestPutBlob_SyncErrorIsReturnedAndNoRename(t *testing.T) {
	t.Parallel()

	ctx := testlogging.Context(t)
	dataDir := testutil.TempDirectory(t)

	osi := newMockOS()
	osi.writeFileSyncRemainingErrors.Store(1) // first create will inject sync failure

	st, err := New(ctx, &Options{
		Path:    dataDir,
		Options: sharded.Options{DirectoryShards: []int{1}},
	}, true)
	require.NoError(t, err)
	t.Cleanup(func() { _ = st.Close(ctx) })

	asFsImpl(t, st).osi = osi

	err = st.PutBlob(ctx, "blob-sync-fail", gather.FromSlice([]byte("hello")), blob.PutOptions{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "can't sync temporary file data")

	_, merr := st.GetMetadata(ctx, "blob-sync-fail")
	require.ErrorIs(t, merr, blob.ErrBlobNotFound)
}

func TestPutBlob_CloseErrorAfterSyncIsReturnedAndNoRename(t *testing.T) {
	t.Parallel()

	ctx := testlogging.Context(t)
	dataDir := testutil.TempDirectory(t)

	osi := newMockOS()
	osi.writeFileCloseRemainingErrors.Store(1) // first create will inject close failure

	st, err := New(ctx, &Options{
		Path:    dataDir,
		Options: sharded.Options{DirectoryShards: []int{1}},
	}, true)
	require.NoError(t, err)
	t.Cleanup(func() { _ = st.Close(ctx) })

	asFsImpl(t, st).osi = osi

	err = st.PutBlob(ctx, "blob-close-fail", gather.FromSlice([]byte("hello")), blob.PutOptions{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "can't close temporary file")

	_, merr := st.GetMetadata(ctx, "blob-close-fail")
	require.ErrorIs(t, merr, blob.ErrBlobNotFound)
}
