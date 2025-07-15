package content

import (
	"context"
	stderrors "errors"
	"sync/atomic"

	"github.com/pkg/errors"

	"github.com/kopia/kopia/internal/stats"
	"github.com/kopia/kopia/repo/blob"
	"github.com/kopia/kopia/repo/logging"
)

const missingPackThreshold = 1000

type empty struct{}

var (
	errTooManyMissingPacks = errors.New("too many missing packs")
	errMissingPacks        = errors.New("the repository is corrupted, it is missing pack blobs with referenced content")
)

func getPackSetFromStorage(ctx context.Context, st blob.Storage) (map[blob.ID]empty, error) {
	const blobIterateParallelism = 1

	existingPacks := map[blob.ID]empty{}

	err := blob.IterateAllPrefixesInParallel(ctx, blobIterateParallelism, st, PackBlobIDPrefixes, func(m blob.Metadata) error {
		existingPacks[m.BlobID] = empty{}

		return nil
	})
	if err != nil {
		return nil, errors.Wrap(err, "error building pack blob set from storage for safety dangling check")
	}

	return existingPacks, nil
}

// VerifyContentToPackMapping checks the consistency of mapping from content
// index entries to pack blobs to ensure that the indexes are not referencing
// packs that do not exist (any longer).
func (bm *WriteManager) VerifyContentToPackMapping(ctx context.Context) error {
	existingPacks, err := getPackSetFromStorage(ctx, bm.st)
	if err != nil {
		return err
	}

	var (
		missingPackCount atomic.Uint32
		missingPacks     stats.CountersMap[blob.ID]
	)

	cItCb := func(ci Info) error {
		// check all referenced packs, that is do not filter out any packs
		if _, found := existingPacks[ci.PackBlobID]; found {
			return nil
		}

		// dangling content, pack is missing
		bm.log.Debugw("dangling content", "cID", ci.ContentID)

		if seen := missingPacks.Increment(ci.PackBlobID); seen {
			return nil
		}

		// pack was not in missingPacks, track unique missing pack count
		bm.log.Debugw("missing pack", "blobID", ci.PackBlobID)

		if c := missingPackCount.Add(1); c > missingPackThreshold {
			return errTooManyMissingPacks
		}

		return nil
	}

	if err := bm.IterateContents(ctx, IterateOptions{IncludeDeleted: true}, cItCb); err != nil {
		err2 := verifyNoMissingPacks(bm.log, missingPackCount.Load(), &missingPacks)

		return errors.Wrap(stderrors.Join(err, err2), "error iterating contents to find missing packs")
	}

	return verifyNoMissingPacks(bm.log, missingPackCount.Load(), &missingPacks)
}

func verifyNoMissingPacks(log logging.Logger, missingPackCount uint32, missingPacks *stats.CountersMap[blob.ID]) error {
	if missingPackCount == 0 {
		return nil
	}

	var danglingContentCount, packCount int

	missingPacks.Range(func(packID blob.ID, contentRefCount uint32) bool {
		packCount++
		danglingContentCount += int(contentRefCount)

		log.Warnw("missing", "blobID", packID, "contentReferenceCount", contentRefCount)

		return true
	})

	log.Warnf("there are at least %v dangling contents and at least %v missing pack blobs", danglingContentCount, packCount)

	return errMissingPacks
}
