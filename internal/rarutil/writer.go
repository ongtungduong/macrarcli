package rarutil

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"
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
// io.EOF) so a tripped cap propagates as a real error, triggering the
// caller's cleanup, instead of silently truncating the archive.
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

// fsEmitter streams archive entries into a private staging directory (never
// destDir directly). It enforces the bomb caps and the archive-internal
// duplicate-name guard — the same invariants the old zip-target emitter
// enforced, retargeted to real files. Because the staging directory is always
// freshly created and empty, os.MkdirAll here can never traverse a
// pre-existing symlink the way it could in a long-lived, reused destDir.
type fsEmitter struct {
	root    string // staging directory root
	lim     limits
	onEntry func(string)
	origin  map[string]string // final name -> first raw name that produced it
	bytes   int64
	count   int
}

func newFsEmitter(root string, lim limits, onEntry func(string)) *fsEmitter {
	return &fsEmitter{root: root, lim: lim, onEntry: onEntry, origin: map[string]string{}}
}

// resolveName applies the duplicate-name guard to a fully-formed staging name
// (files have no trailing slash, directories do). A repeat is suspicious only
// when it arrives from a DIFFERENT raw archive name — i.e. a path that used
// traversal/separators to collide after sanitizing. That is a data-loss/
// overwrite attempt and is rejected. A repeat of the SAME raw name, which
// legitimate multi-volume/recovery archives may re-stream, is preserved by
// renaming to a free " (k)" variant rather than dropped.
func (e *fsEmitter) resolveName(raw, name string) (string, error) {
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

// checkCount enforces the entry-count cap before an entry is written.
func (e *fsEmitter) checkCount() error {
	if e.lim.maxEntries > 0 && e.count >= e.lim.maxEntries {
		return errLimitEntries
	}
	return nil
}

func (e *fsEmitter) fire(name string) {
	if e.onEntry != nil {
		e.onEntry(name)
	}
}

// emitDir creates a directory entry in the staging tree. name is the
// sanitized entry name without a trailing slash. Directories are always
// created with a permissive 0755 regardless of the archive's recorded mode —
// unlike files, a directory must stay writable for the rest of this run to
// populate it, so exact mode restoration is intentionally not attempted here
// (matches the existing codebase's precedent: only file mode/mtime were ever
// asserted by tests).
func (e *fsEmitter) emitDir(raw, name string) error {
	if err := e.checkCount(); err != nil {
		return err
	}
	final, err := e.resolveName(raw, name+"/")
	if err != nil {
		return err
	}
	full := filepath.Join(e.root, strings.TrimSuffix(final, "/"))
	if err := os.MkdirAll(full, 0o755); err != nil {
		return fmt.Errorf("stage dir %q: %w", raw, err)
	}
	e.count++
	e.fire(final)
	return nil
}

// emitFile streams a file entry into the staging tree, enforcing the
// total-bytes cap, then restores its POSIX permission bits and modification
// time so the staged copy is already fully correct before it is ever moved
// into destDir.
func (e *fsEmitter) emitFile(raw, name string, mode fs.FileMode, modTime time.Time, content io.Reader) error {
	if err := e.checkCount(); err != nil {
		return err
	}
	final, err := e.resolveName(raw, name)
	if err != nil {
		return err
	}
	full := filepath.Join(e.root, final)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return fmt.Errorf("stage dir for %q: %w", raw, err)
	}
	f, err := os.Create(full)
	if err != nil {
		return fmt.Errorf("stage file %q: %w", raw, err)
	}
	bufp := copyBufPool.Get().(*[]byte)
	_, copyErr := io.CopyBuffer(&cappedWriter{w: f, total: &e.bytes, limit: e.lim.maxBytes}, content, *bufp)
	copyBufPool.Put(bufp)
	closeErr := f.Close()
	if copyErr != nil {
		return fmt.Errorf("stage file %q: %w", raw, copyErr)
	}
	if closeErr != nil {
		return fmt.Errorf("stage file %q: %w", raw, closeErr)
	}
	if err := os.Chmod(full, mode.Perm()); err != nil {
		return fmt.Errorf("chmod %q: %w", raw, err)
	}
	if !modTime.IsZero() {
		if err := os.Chtimes(full, modTime, modTime); err != nil {
			return fmt.Errorf("chtimes %q: %w", raw, err)
		}
	}
	e.count++
	e.fire(final)
	return nil
}

// cappedWriter forwards writes to w while accumulating *total, returning
// errLimitBytes (not a short write) the moment the running total would exceed
// limit. A zero limit disables the cap.
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
