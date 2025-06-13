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
	const size = 4096

	d := t.TempDir()
	f := filepath.Join(d, "test")
	data := bytes.Repeat([]byte{1}, size)

	err := os.WriteFile(f, data, os.ModePerm)
	require.NoError(t, err)

	s, err := GetFileAllocSize(f)
	require.NoError(t, err, "error getting file alloc size for %s: %v", f, err)

	require.GreaterOrEqual(t, s, size, "invalid allocated file size %d, expected at least %d", s, size)
}

func TestGetBlockSize(t *testing.T) {
	size, err := GetBlockSize(".")
	require.NoError(t, err)
	require.Greater(t, size, uint64(0))
}
