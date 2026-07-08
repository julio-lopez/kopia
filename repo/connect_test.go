package repo_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kopia/kopia/internal/blobtesting"
	"github.com/kopia/kopia/internal/repotesting"
	"github.com/kopia/kopia/internal/testlogging"
	"github.com/kopia/kopia/repo"
	"github.com/kopia/kopia/repo/format"
)

const (
	connectTestPassword      = "correct-horse-battery-staple"
	connectTestWrongPassword = "wrong-password"
)

// TestConnectSkipVerifyInvalidPassword verifies that SkipVerifyConnect=true with a wrong
// password returns an error and does not leave the config file behind.
func TestConnectSkipVerifyInvalidPassword(t *testing.T) {
	t.Parallel()

	ctx := testlogging.Context(t)

	st := repotesting.NewReconnectableStorage(t, blobtesting.NewMapStorage(blobtesting.DataMap{}, nil, nil))

	require.NoError(t, repo.Initialize(ctx, st, &repo.NewRepositoryOptions{}, connectTestPassword))

	configFile := filepath.Join(t.TempDir(), "kopia.config")

	err := repo.Connect(ctx, configFile, st, connectTestWrongPassword, &repo.ConnectOptions{
		SkipVerifyConnect: true,
	})
	require.Error(t, err)
	require.ErrorIs(t, err, format.ErrInvalidPassword)

	_, statErr := os.Stat(configFile)
	require.True(t, os.IsNotExist(statErr), "config file must not exist after failed connect")
}

// TestConnectSkipVerifyValidPassword verifies that SkipVerifyConnect=true with the correct
// password succeeds, writes the config, and that the repository can subsequently be opened.
func TestConnectSkipVerifyValidPassword(t *testing.T) {
	t.Parallel()

	ctx := testlogging.Context(t)

	st := repotesting.NewReconnectableStorage(t, blobtesting.NewMapStorage(blobtesting.DataMap{}, nil, nil))

	require.NoError(t, repo.Initialize(ctx, st, &repo.NewRepositoryOptions{}, connectTestPassword))

	configFile := filepath.Join(t.TempDir(), "kopia.config")

	require.NoError(t, repo.Connect(ctx, configFile, st, connectTestPassword, &repo.ConnectOptions{
		SkipVerifyConnect: true,
	}))

	_, statErr := os.Stat(configFile)
	require.NoError(t, statErr, "config file must exist after successful connect")

	r, err := repo.Open(ctx, configFile, connectTestPassword, nil)
	require.NoError(t, err, "repository must be openable after skip-verify connect")
	require.NoError(t, r.Close(ctx))
}
