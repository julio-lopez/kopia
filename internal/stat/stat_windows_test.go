//go:build windows
// +build windows

package stat

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetFileAllocSize(t *testing.T) {
	const size uint64 = 4096
	const writeSize int = int(size) + 600

	f := filepath.Join(t.TempDir(), "test")

	err := os.WriteFile(f, bytes.Repeat([]byte{1}, writeSize), os.ModePerm)
	require.NoError(t, err)

	s, err := GetFileAllocSize(f)
	require.NoErrorf(t, err, "error getting file alloc size for", f)
	t.Log("alloc size:", s)

	bs, err := GetBlockSize(filepath.Dir(f))
	require.NoErrorf(t, err, "error getting block size for", f)
	t.Log("block size:", bs)

	require.GreaterOrEqual(t, s, size, "invalid allocated file size")
	// require.GreaterOrEqual(t, s, size, "invalid allocated file size")
}

func TestGetBlockSize(t *testing.T) {
	size, err := GetBlockSize(".")
	require.NoError(t, err)
	require.Greater(t, size, uint64(0))
}
