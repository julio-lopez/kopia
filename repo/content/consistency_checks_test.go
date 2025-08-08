package content

import (
	"encoding/binary"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kopia/kopia/internal/blobtesting"
	"github.com/kopia/kopia/internal/epoch"
	"github.com/kopia/kopia/internal/gather"
	"github.com/kopia/kopia/internal/testlogging"
	"github.com/kopia/kopia/repo/blob"
	"github.com/kopia/kopia/repo/content/index"
	"github.com/kopia/kopia/repo/format"
)

func newTestingMapStorage() blob.Storage {
	data := blobtesting.DataMap{}
	keyTime := map[blob.ID]time.Time{}

	return blobtesting.NewMapStorage(data, keyTime, nil)
}

// newTestWriteManager is a helper to create a WriteManager for testing.
func newTestWriteManager(t *testing.T, st blob.Storage) *WriteManager {
	t.Helper()

	fp := mustCreateFormatProvider(t, &format.ContentFormat{
		Hash:       "HMAC-SHA256-128",
		Encryption: "AES256-GCM-HMAC-SHA256",
		HMACSecret: []byte("test-hmac"),
		MasterKey:  []byte("0123456789abcdef0123456789abcdef"),
		MutableParameters: format.MutableParameters{
			Version:         2,
			EpochParameters: epoch.DefaultParameters(),
			IndexVersion:    index.Version2,
			MaxPackSize:     1024 * 1024, // 1 MB
		},
	})

	bm, err := NewManagerForTesting(testlogging.Context(t), st, fp, nil, nil)

	require.NoError(t, err, "cannot create content write manager")

	return bm
}

func TestGetPackSetFromStorage(t *testing.T) {
	st := newTestingMapStorage()
	bm := newTestWriteManager(t, st)
	ctx := testlogging.Context(t)

	// Write a content in a p pack.
	_, err := bm.WriteContent(ctx, gather.FromSlice([]byte("hello")), "", NoCompression)
	require.NoError(t, err)

	// Write a content in a q pack.
	_, err = bm.WriteContent(ctx, gather.FromSlice([]byte("hello")), "k", NoCompression)
	require.NoError(t, err)

	err = bm.Flush(ctx)
	require.NoError(t, err)

	blobs, err := getPackSetFromStorage(ctx, st)
	require.NoError(t, err)
	require.Len(t, blobs, 2)
}

func TestVerifyContentToPackMapping_NoMissingPack(t *testing.T) {
	st := newTestingMapStorage()
	bm := newTestWriteManager(t, st)
	ctx := testlogging.Context(t)

	// Create pack by writing contents.
	_, err := bm.WriteContent(ctx, gather.FromSlice([]byte("hello")), "", NoCompression)
	require.NoError(t, err)

	_, err = bm.WriteContent(ctx, gather.FromSlice([]byte("hello prefixed")), "k", NoCompression)
	require.NoError(t, err)

	require.NoError(t, bm.Flush(ctx))

	err = bm.VerifyContentToPackMapping(ctx)
	require.NoError(t, err, "verification should pass as the pack exists")
}

func TestVerifyContentToPackMapping_MissingPackP(t *testing.T) {
	st := newTestingMapStorage()
	bm := newTestWriteManager(t, st)
	ctx := testlogging.Context(t)

	// Create pack by writing contents.
	_, err := bm.WriteContent(ctx, gather.FromSlice([]byte("hello")), "", NoCompression)
	require.NoError(t, err)

	_, err = bm.WriteContent(ctx, gather.FromSlice([]byte("hello prefixed")), "k", NoCompression)
	require.NoError(t, err)

	require.NoError(t, bm.Flush(ctx))

	// Delete the p pack from storage.
	blobs, err := blob.ListAllBlobs(ctx, st, PackBlobIDPrefixRegular)
	require.NoError(t, err)
	require.Len(t, blobs, 1)
	require.NoError(t, st.DeleteBlob(ctx, blobs[0].BlobID))

	// Verification should fail with the specific error for missing packs.
	err = bm.VerifyContentToPackMapping(ctx)
	require.Error(t, err)
	require.ErrorIs(t, err, errMissingPacks)
}

func TestVerifyContentToPackMapping_MissingPackQ(t *testing.T) {
	st := newTestingMapStorage()
	bm := newTestWriteManager(t, st)
	ctx := testlogging.Context(t)

	// Create pack by writing contents.
	_, err := bm.WriteContent(ctx, gather.FromSlice([]byte("hello")), "", NoCompression)
	require.NoError(t, err)

	_, err = bm.WriteContent(ctx, gather.FromSlice([]byte("hello prefixed")), "k", NoCompression)
	require.NoError(t, err)

	require.NoError(t, bm.Flush(ctx))

	// Delete the q pack from storage.
	blobs, err := blob.ListAllBlobs(ctx, st, PackBlobIDPrefixSpecial)
	require.NoError(t, err)
	require.Len(t, blobs, 1)
	require.NoError(t, st.DeleteBlob(ctx, blobs[0].BlobID))

	// Verification should fail with the specific error for missing packs.
	err = bm.VerifyContentToPackMapping(ctx)
	require.Error(t, err)
	require.ErrorIs(t, err, errMissingPacks)
}

func TestVerifyContentToPackMapping_TooManyMissingPacks(t *testing.T) {
	st := newTestingMapStorage()
	bm := newTestWriteManager(t, st)
	ctx := testlogging.Context(t)

	// Create more than 'missingPackThreshold' contents, each in a new pack.
	// This is inefficient but serves the purpose of the test.
	var buf [4]byte

	for i := range missingPackThreshold + 5 {
		binary.LittleEndian.PutUint32(buf[:], uint32(i))
		_, err := bm.WriteContent(ctx, gather.FromSlice(buf[:]), "", NoCompression)
		require.NoError(t, err)
		require.NoError(t, bm.Flush(ctx))
	}

	// Delete all pack blobs from storage.
	blobs, err := blob.ListAllBlobs(ctx, st, PackBlobIDPrefixRegular)
	require.NoError(t, err)

	for _, b := range blobs {
		require.NoError(t, st.DeleteBlob(ctx, b.BlobID))
	}

	// Verification should fail with the error for too many missing packs.
	err = bm.VerifyContentToPackMapping(ctx)
	require.Error(t, err)
	require.ErrorIs(t, err, errTooManyMissingPacks)
}
