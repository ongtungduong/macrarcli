package rarutil

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
	"sync"
)

// copyBufSize is the per-entry streaming copy buffer size. 512 KB cuts the
// syscall/iteration count on large entries well below io.Copy's default 32 KB
// without holding meaningful memory (the buffers are pooled and shared).
const copyBufSize = 512 << 10

// copyBufPool reuses streaming buffers across entries and concurrent jobs so a
// large-entry copy neither allocates per entry nor scales memory with archive
// size. Storing *[]byte (not []byte) keeps Put allocation-free.
var copyBufPool = sync.Pool{
	New: func() any {
		b := make([]byte, copyBufSize)
		return &b
	},
}

// errLimitBytes and errLimitEntries report that a conversion exceeded the
// configured decompression-bomb caps. They are distinct sentinels (never
// io.EOF) so a tripped cap propagates through io.Copy and the emitter as a real
// error — triggering the caller's partial-output cleanup — instead of silently
// truncating the archive the way a bare io.LimitReader would.
var (
	errLimitBytes   = errors.New("archive exceeds maximum total uncompressed size")
	errLimitEntries = errors.New("archive exceeds maximum entry count")
)

// limits caps how far an archive may expand. A zero field means that dimension
// is unlimited; caps are opt-in so default conversions behave exactly as before.
type limits struct {
	maxBytes   int64 // total uncompressed bytes across all entries
	maxEntries int   // number of entries (files + directories)
}

// zipEmitter is the shared write step for both conversion paths: the native one
// (streaming a single RAR reader) and the fallback one (walking an extracted
// directory). It enforces the bomb caps and a duplicate-name guard so neither
// path can be used as a bomb or silently lose colliding entries. Per-path
// concerns — symlink neutralization and non-regular skipping — stay in the
// callers; the emitter only ever sees the entries a caller chooses to feed it.
type zipEmitter struct {
	zw       *zip.Writer
	lim      limits
	onEntry  func(string)
	expected map[string]int64
	origin   map[string]string // final ZIP name -> first raw name that produced it
	bytes    int64
	count    int
}

func newZipEmitter(zw *zip.Writer, lim limits, onEntry func(string)) *zipEmitter {
	return &zipEmitter{
		zw:       zw,
		lim:      lim,
		onEntry:  onEntry,
		expected: map[string]int64{},
		origin:   map[string]string{},
	}
}

// resolveName applies the duplicate-name guard to a fully-formed ZIP name
// (files have no trailing slash, directories do). A repeat is suspicious only
// when it arrives from a DIFFERENT raw archive name — i.e. a path that used
// traversal/separators to collide after sanitizing ("a/../b" vs "b"). That is a
// data-loss/overwrite attempt and is rejected. A repeat of the SAME raw name,
// which legitimate multi-volume/recovery archives may re-stream, is preserved by
// renaming to a free " (k)" variant rather than dropped. (Reject-vs-rename is
// the conservative, data-preserving choice pending an empirical multi-volume
// re-emission experiment.)
func (e *zipEmitter) resolveName(raw, name string) (string, error) {
	first, seen := e.origin[name]
	if !seen {
		e.origin[name] = raw
		return name, nil
	}
	if first != raw {
		return "", fmt.Errorf("post-sanitize name collision: %q and a prior entry both map to %q", raw, name)
	}
	for k := 1; ; k++ {
		cand := dedupVariant(name, k)
		if _, taken := e.origin[cand]; !taken {
			e.origin[cand] = raw
			return cand, nil
		}
	}
}

// dedupVariant inserts " (k)" before the extension (or before the trailing
// slash for directories): "f.txt" -> "f (1).txt", "d/" -> "d (1)/".
func dedupVariant(name string, k int) string {
	isDir := strings.HasSuffix(name, "/")
	trimmed := strings.TrimSuffix(name, "/")
	ext := path.Ext(trimmed)
	v := fmt.Sprintf("%s (%d)%s", strings.TrimSuffix(trimmed, ext), k, ext)
	if isDir {
		v += "/"
	}
	return v
}

// emitDir writes a directory entry. name is the sanitized entry name without a
// trailing slash; fh carries mode/mtime/method as the caller's path requires.
func (e *zipEmitter) emitDir(raw, name string, fh *zip.FileHeader) error {
	if err := e.checkCount(); err != nil {
		return err
	}
	final, err := e.resolveName(raw, name+"/")
	if err != nil {
		return err
	}
	fh.Name = final
	if _, err := e.zw.CreateHeader(fh); err != nil {
		return fmt.Errorf("write zip dir %q: %w", raw, err)
	}
	e.expected[final] = 0
	e.count++
	e.fire(final)
	return nil
}

// emitFile writes a file entry, streaming content through the total-bytes cap.
// name is the sanitized entry name; fh carries mode/mtime/method.
func (e *zipEmitter) emitFile(raw, name string, fh *zip.FileHeader, content io.Reader) error {
	if err := e.checkCount(); err != nil {
		return err
	}
	final, err := e.resolveName(raw, name)
	if err != nil {
		return err
	}
	fh.Name = final
	w, err := e.zw.CreateHeader(fh)
	if err != nil {
		return fmt.Errorf("write zip entry %q: %w", raw, err)
	}
	bufp := copyBufPool.Get().(*[]byte)
	n, err := io.CopyBuffer(&cappedWriter{w: w, total: &e.bytes, limit: e.lim.maxBytes}, content, *bufp)
	copyBufPool.Put(bufp)
	if err != nil {
		return fmt.Errorf("copy entry %q: %w", raw, err)
	}
	e.expected[final] = n
	e.count++
	e.fire(final)
	return nil
}

// checkCount enforces the entry-count cap before an entry is written. It lives
// in the emitter (the per-entry chokepoint of both iteration cores) so every
// write path — and any future read-only iterator that reuses it — is bounded.
func (e *zipEmitter) checkCount() error {
	if e.lim.maxEntries > 0 && e.count >= e.lim.maxEntries {
		return errLimitEntries
	}
	return nil
}

func (e *zipEmitter) fire(name string) {
	if e.onEntry != nil {
		e.onEntry(name)
	}
}

// cappedWriter forwards writes to w while accumulating *total, returning
// errLimitBytes (not a short write) the moment the running total would exceed
// limit. A zero limit disables the cap. Erroring before the overflowing chunk
// lands guarantees the bomb is never even partially written before the caller
// discards the temp output.
type cappedWriter struct {
	w     io.Writer
	total *int64
	limit int64
}

func (c *cappedWriter) Write(p []byte) (int, error) {
	if c.limit > 0 && *c.total+int64(len(p)) > c.limit {
		return 0, errLimitBytes
	}
	n, err := c.w.Write(p)
	*c.total += int64(n)
	return n, err
}
