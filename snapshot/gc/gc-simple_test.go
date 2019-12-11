package gc

import (
	"context"
	"io/ioutil"
	"path/filepath"
	"testing"
	"time"

	"github.com/kylelemons/godebug/pretty"
	"github.com/stretchr/testify/require"

	"github.com/kopia/kopia/internal/mockfs"
	"github.com/kopia/kopia/repo"
	"github.com/kopia/kopia/repo/blob/filesystem"
	"github.com/kopia/kopia/repo/content"
	"github.com/kopia/kopia/snapshot"
	"github.com/kopia/kopia/snapshot/snapshotfs"
)

const (
	masterPassword     = "foofoofoofoofoofoofoofoo"
	defaultPermissions = 0777
)

type testHarness struct {
	sourceDir *mockfs.Directory
	repoDir   string
	repo      *repo.Repository
}

func TestGC(t *testing.T) {
	ctx := context.Background()

	th := setupGcTestEnv(t)
	require.NotNil(t, th)
	require.NotNil(t, th.sourceDir)

	si := snapshot.SourceInfo{
		Host:     "host",
		UserName: "user",
		Path:     "/foo",
	}
	s1, err := snapshotfs.Create(ctx, th.repo, th.sourceDir, si, "")
	require.NoError(t, err)
	require.NotNil(t, s1)
	t.Log("snap 1:", pretty.Sprint(s1))

	flushRepo(ctx, t, th.repo)

	err = th.repo.Manifests.Delete(ctx, s1.ID)
	require.NoError(t, err)

	flushRepo(ctx, t, th.repo)

	_, err = Run(ctx, th.repo, time.Millisecond, true)
	require.NoError(t, err)

	flushRepo(ctx, t, th.repo)

	s2, err := snapshotfs.Create(ctx, th.repo, th.sourceDir, si, "")
	require.NoError(t, err)
	require.NotNil(t, s2)
	t.Log("snap 2:", pretty.Sprint(s2))

	flushRepo(ctx, t, th.repo)

	info, err := th.repo.Content.ContentInfo(ctx, content.ID(s2.RootObjectID()))
	require.NoError(t, err)

	info.Payload = nil
	t.Log("root info:", pretty.Sprint(info))
}

func flushRepo(ctx context.Context, t *testing.T, r *repo.Repository) {
	err := r.Flush(ctx)
	require.NoError(t, err)
}

func setupGcTestEnv(t *testing.T) *testHarness {
	t.Helper()

	th := newTestHarness(t)
	require.NotNil(t, th)
	require.NotNil(t, th.repo)

	d := mockfs.NewDirectory()
	d.AddDir("d1", defaultPermissions)
	d.AddFile("d1/f2", []byte{1, 2, 3, 4}, defaultPermissions)

	th.sourceDir = d

	return th
}

func newTestHarness(t *testing.T) *testHarness {
	t.Helper()

	ctx := context.Background()

	check := require.New(t)

	repoDir, err := ioutil.TempDir("", "kopia-repo")
	check.NoError(err, "cannot create temp directory")
	t.Log("repo dir:", repoDir)

	storage, err := filesystem.New(context.Background(), &filesystem.Options{
		Path: repoDir,
	})
	check.NoError(err, "cannot create storage directory")

	err = repo.Initialize(ctx, storage, &repo.NewRepositoryOptions{}, masterPassword)
	check.NoError(err, "cannot create repository")

	configFile := filepath.Join(repoDir, "kopia.config")
	err = repo.Connect(ctx, configFile, storage, masterPassword, repo.ConnectOptions{})
	check.NoError(err, "unable to connect to repository")

	rep, err := repo.Open(ctx, configFile, masterPassword, &repo.Options{})
	check.NoError(err, "unable to open repository")

	return &testHarness{
		repoDir: repoDir,
		repo:    rep,
	}
}
