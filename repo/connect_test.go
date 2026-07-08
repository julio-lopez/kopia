package repo_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kopia/kopia/internal/blobtesting"
	"github.com/kopia/kopia/internal/repotesting"
	"github.com/kopia/kopia/internal/testlogging"
	"github.com/kopia/kopia/internal/testutil"
	"github.com/kopia/kopia/repo"
	"github.com/kopia/kopia/repo/content"
)

func TestConnectSkipVerify(t *testing.T) {
	ctx := testlogging.Context(t)

	st := repotesting.NewReconnectableStorage(t, blobtesting.NewMapStorage(blobtesting.DataMap{}, nil, nil))

	correctPassword := "correct-password"
	wrongPassword := "wrong-password"

	// Initialize the repository with the correct password.
	require.NoError(t, repo.Initialize(ctx, st, nil, correctPassword))

	t.Run("SkipVerifyTrue_WrongPassword", func(t *testing.T) {
		configFile := filepath.Join(testutil.TempDirectory(t), "kopia.config")

		// Connect with a wrong password but SkipVerifyConnect=true.
		// This should succeed because verification (which opens the repo) is skipped.
		err := repo.Connect(ctx, configFile, st, wrongPassword, &repo.ConnectOptions{
			CachingOptions:    content.CachingOptions{CacheDirectory: testutil.TempDirectory(t)},
			SkipVerifyConnect: true,
		})
		require.NoError(t, err)

		// Verify that the config file was actually written.
		_, err = os.Stat(configFile)
		require.NoError(t, err, "config file should exist after connect with skip-verify")

		// Verify that Open with the wrong password fails (proving verification was indeed skipped).
		_, err = repo.Open(ctx, configFile, wrongPassword, nil)
		require.Error(t, err, "opening with wrong password should fail")
	})

	t.Run("SkipVerifyFalse_WrongPassword", func(t *testing.T) {
		configFile := filepath.Join(testutil.TempDirectory(t), "kopia.config")

		// Connect with a wrong password and SkipVerifyConnect=false (default).
		// This should fail because verification opens the repo and password is wrong.
		err := repo.Connect(ctx, configFile, st, wrongPassword, &repo.ConnectOptions{
			CachingOptions: content.CachingOptions{CacheDirectory: testutil.TempDirectory(t)},
		})
		require.Error(t, err, "connect without skip-verify should fail with wrong password")

		// The config file should have been removed by verifyConnect's cleanup.
		_, err = os.Stat(configFile)
		require.True(t, os.IsNotExist(err), "config file should be removed after failed verification")
	})

	t.Run("SkipVerifyTrue_CorrectPassword", func(t *testing.T) {
		configFile := filepath.Join(testutil.TempDirectory(t), "kopia.config")

		// Connect with correct password and SkipVerifyConnect=true.
		// Should succeed and write config file.
		err := repo.Connect(ctx, configFile, st, correctPassword, &repo.ConnectOptions{
			CachingOptions:    content.CachingOptions{CacheDirectory: testutil.TempDirectory(t)},
			SkipVerifyConnect: true,
		})
		require.NoError(t, err)

		// Verify the config file was written and the repo can be opened.
		r, err := repo.Open(ctx, configFile, correctPassword, nil)
		require.NoError(t, err)
		require.NoError(t, r.Close(ctx))
	})

	t.Run("SkipVerifyFalse_CorrectPassword", func(t *testing.T) {
		configFile := filepath.Join(testutil.TempDirectory(t), "kopia.config")

		// Connect with correct password and SkipVerifyConnect=false (default).
		// Should succeed (verification passes).
		err := repo.Connect(ctx, configFile, st, correctPassword, &repo.ConnectOptions{
			CachingOptions: content.CachingOptions{CacheDirectory: testutil.TempDirectory(t)},
		})
		require.NoError(t, err)

		// Verify the config file was written and the repo can be opened.
		r, err := repo.Open(ctx, configFile, correctPassword, nil)
		require.NoError(t, err)
		require.NoError(t, r.Close(ctx))
	})
}
