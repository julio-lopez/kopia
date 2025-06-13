//go:build windows
// +build windows

package stat

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetFileAllocSize(t *testing.T) {
	const size uint64 = 4096

	f := filepath.Join(t.TempDir(), "test")

	err := os.WriteFile(f, []byte{1}, os.ModePerm)
	require.NoError(t, err)

	s, err := GetFileAllocSize(f)
	require.NoError(t, err, "error getting file alloc size for", f)

	require.GreaterOrEqual(t, s, size, "invalid allocated file size")
}

func TestGetBlockSize(t *testing.T) {
	size, err := GetBlockSize(".")
	require.NoError(t, err)
	require.Greater(t, size, uint64(0))
}
