package tempfile

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCreateUnixFallback(t *testing.T) {
	f, err := createUnixFallback("")

	t.Log("create 1 error:", err)

	if f != nil {
		err = f.Close()
		require.NoError(t, err)
	}

	f, err = createUnixFallback("")

	t.Log("create 2 error:", err)

	if f != nil {
		err = f.Close()
		require.NoError(t, err)
	}
}

func TestCreate(t *testing.T) {
	f, err := Create("")
	require.NoError(t, err)

	err = f.Close()
	require.NoError(t, err)

	f, err = Create("")
	require.NoError(t, err)

	err = f.Close()
	require.NoError(t, err)
}
