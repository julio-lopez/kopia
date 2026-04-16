package compression

import (
	"errors"
	"io"
)

func init() {
	registerUnsupportedCompressor("lz4", lz4Compressor{})
}

var errLZ4NotSupported = errors.New("LZ4 compressor is no longer supported; use an older Kopia version that still supports LZ4 to read legacy repositories that use the LZ4 compressor")

type lz4Compressor struct{}

func (c lz4Compressor) HeaderID() HeaderID {
	return headerLZ4Removed
}

func (c lz4Compressor) Compress(_ io.Writer, _ io.Reader) error {
	return errLZ4NotSupported
}

func (c lz4Compressor) Decompress(_ io.Writer, _ io.Reader, _ bool) error {
	return errLZ4NotSupported
}
