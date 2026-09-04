package rarutil

import (
	"archive/zip"
	"compress/flate"
	"io"
)

// entryMethod returns the ZIP compression method for opts: zip.Store when the
// caller asked for no compression, otherwise zip.Deflate.
func entryMethod(opts Options) uint16 {
	if opts.Store {
		return zip.Store
	}
	return zip.Deflate
}

// registerCompressor installs a Deflate compressor at an explicit level (1..9)
// on zw. It is a no-op when storing (no compression) or when Level is 0, in
// which case the stdlib's default Deflate compressor is used.
func registerCompressor(zw *zip.Writer, opts Options) {
	if opts.Store || opts.Level <= 0 {
		return
	}
	level := opts.Level
	zw.RegisterCompressor(zip.Deflate, func(w io.Writer) (io.WriteCloser, error) {
		return flate.NewWriter(w, level)
	})
}
