package simple_test

import (
	"encoding/binary"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kopia/kopia/internal/faketime"
	"github.com/kopia/kopia/internal/repotesting"
	"github.com/kopia/kopia/internal/testlogging"
	"github.com/kopia/kopia/repo"
	"github.com/kopia/kopia/repo/content"
	"github.com/kopia/kopia/repo/object"
)

func TestContentsInRepo(t *testing.T) {
	ctx := testlogging.Context(t)
	baseTime := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	fakeTime := faketime.NewTimeAdvance(baseTime, time.Second)
	fakeTimeOption := func(o *repo.Options) {
		o.TimeNowFunc = fakeTime.NowFunc()
	}

	_, e := repotesting.NewEnvironment(t, content.FormatVersion2, repotesting.Options{OpenOptions: fakeTimeOption})

	require.NotNil(t, e)
	require.NotNil(t, e.Repository)
	require.NotNil(t, e.RepositoryWriter)

	defer e.RepositoryWriter.Close(ctx)

	const contentCount = 10000

	require.NoError(t, e.Repository.Refresh(ctx))

	// create 10000 unreferenced contents
	oids := create4ByteObjects(t, e.Repository, 0, contentCount)

	require.NoError(t, e.RepositoryWriter.Flush(ctx))
	require.NoError(t, e.Repository.Refresh(ctx))

	cids := objectIDsToContentIDs(t, oids)

	// check contents are not marked as deleted
	checkContentDeletion(t, e.Repository, cids, false)
}

func create4ByteObjects(t *testing.T, r repo.Repository, base, count int) []object.ID {
	t.Helper()

	oids := make([]object.ID, 0, count)
	ctx := testlogging.Context(t)

	ctx, rw, err := r.NewWriter(ctx, repo.WriteSessionOptions{})
	require.NoError(t, err)

	var b [4]byte

	for i := base; i < base+count; i++ {
		w := rw.NewObjectWriter(ctx, object.WriterOptions{Description: "create-test-contents"})

		binary.BigEndian.PutUint32(b[:], uint32(i))

		_, err := w.Write(b[:])
		require.NoError(t, err)

		oid, err := w.Result()
		require.NoError(t, err)

		require.NoError(t, w.Close())

		oids = append(oids, oid)
	}

	return oids
}

func objectIDsToContentIDs(t *testing.T, oids []object.ID) []content.ID {
	t.Helper()

	cids := make([]content.ID, 0, len(oids))

	for _, oid := range oids {
		cid, _, ok := oid.ContentID()

		require.True(t, ok)

		cids = append(cids, cid)
	}

	return cids
}

func checkContentDeletion(t *testing.T, r repo.Repository, cids []content.ID, deleted bool) {
	t.Helper()

	ctx := testlogging.Context(t)

	for i, cid := range cids {
		ci, err := r.ContentInfo(ctx, cid)

		require.NoErrorf(t, err, "i:%d cid:%s", i, cid)
		require.Equalf(t, deleted, ci.GetDeleted(), "i:%d cid:%s", i, cid)
	}
}
