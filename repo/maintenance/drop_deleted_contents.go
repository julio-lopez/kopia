package maintenance

import (
	"context"
	"time"

	"github.com/kopia/kopia/internal/repotracing"
	"github.com/kopia/kopia/internal/repotracing/logparam"
	"github.com/kopia/kopia/repo"
	"github.com/kopia/kopia/repo/content/indexblob"
	"github.com/kopia/kopia/repo/maintenancestats"
)

// dropDeletedContents rewrites indexes while dropping deleted contents above certain age.
func dropDeletedContents(ctx context.Context, rep repo.DirectRepositoryWriter, dropDeletedBefore time.Time, safety SafetyParameters) (*maintenancestats.CompactIndexesStats, error) {
	ctx = repotracing.WithParams(ctx,
		logparam.String("span:drop-deleted-contents", repotracing.RandomSpanID()))

	log := rep.LogManager().NewLogger("maintenance-drop-deleted-contents")

	repotracing.Log1(ctx, log, "Dropping deleted contents", logparam.Time("dropDeletedBefore", dropDeletedBefore))

	//nolint:wrapcheck
	return rep.ContentManager().CompactIndexes(ctx, indexblob.CompactOptions{
		AllIndexes:                       true,
		DropDeletedBefore:                dropDeletedBefore,
		DisableEventualConsistencySafety: safety.DisableEventualConsistencySafety,
	})
}
