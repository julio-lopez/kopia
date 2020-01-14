package gc

import (
	"context"
	"testing"

	"github.com/kopia/kopia/repo"
)

func Test_checkRepairSnaps(t *testing.T) {
	// repo setup:
	// - populate content content (count=x)
	// - create snaps (count=s) referring to some of the content
	type args struct {
		ctx   context.Context
		rep   *repo.Repository
		snaps manifestIDSet
	}

	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		// TODO: Add test cases.

		// success path:
		// - pass no snaps
		// - pass a single snap from the set
		// - pass all snaps in the set
		// - include non-existing snaps

		// error paths
		// - fail to read snapshot: pass a non valid snapshot
		// - completely remove a content (needs purge support, may be able
		// 	 to fake it now)
		// - any other form of read content failure, e.g., a directory
		//   => is the walk tree callback called before reading the content?
		// - fail flush?

	}

	for _, tt := range tests {
		tt := tt // pacify linter, not needed in this case
		t.Run(tt.name, func(t *testing.T) {
			// success path
			// - delete a referenced content
			// - validate content is undeleted

			if err := checkRepairSnaps(tt.args.ctx, tt.args.rep, tt.args.snaps); (err != nil) != tt.wantErr {
				t.Errorf("checkRepairSnaps() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
