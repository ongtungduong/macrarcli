// Package rarutil performs the core RAR-to-ZIP transformation.
package rarutil

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nwaples/rardecode/v2"
)

// Options tunes a Convert run.
type Options struct {
	// OnEntry, if non-nil, is called once per archive entry (files and
	// directories) with the sanitized entry name, for progress reporting.
	OnEntry func(name string)
	// OnVerbose, if non-nil, receives extra diagnostics (which decode path was
	// used, per-archive timing). It may run from multiple goroutines in a batch,
	// so it must be safe for concurrent use.
	OnVerbose func(msg string)
	// Password, if set, decrypts password-protected RAR archives.
	Password string
	// Force allows overwriting an existing output file. When false, Convert
	// refuses (and does not touch) a destination that already exists.
	Force bool
	// Store writes entries uncompressed (zip.Store) instead of Deflate.
	Store bool
	// Level sets the Deflate compression level. 0 (the zero value) means the
	// stdlib default; 1..9 selects an explicit level (1 = fastest, 9 = best).
	// Ignored when Store is set. For no compression, use Store.
	Level int
	// Verify reopens the finished ZIP and confirms every expected entry is
	// present with a matching uncompressed size before reporting success.
	Verify bool
	// AllowFallback permits shelling out to a system unrar/7z when the pure-Go
	// decoder cannot read the archive. This waives the no-dependency guarantee.
	//
	// WARNING: the fallback extracts the whole archive to a temp dir with the
	// external tool BEFORE rar2zip's caps run, so MaxTotalBytes/MaxEntries do
	// NOT bound it. --allow-fallback is unsafe against untrusted archives until
	// the fallback gains its own pre-extraction bound.
	AllowFallback bool
	// MaxTotalBytes caps the total uncompressed size across all entries on the
	// native path; 0 (the default) means unlimited. Exceeding it aborts the
	// conversion with no output (decompression-bomb defense).
	MaxTotalBytes int64
	// MaxEntries caps the number of archive entries on the native path; 0 (the
	// default) means unlimited.
	MaxEntries int
	// SkipExisting, in a batch, makes an already-existing output a skip (ErrSkipped)
	// rather than a failure. It is advisory (a pre-stat, TOCTOU-prone under the
	// concurrent default), NOT the collision-safety mechanism — that remains the
	// inter-job destination check plus the atomic temp+rename.
	SkipExisting bool
}

// ErrSkipped reports that Convert declined to run because the output already
// exists and SkipExisting was set (and Force was not). It is not a failure; the
// batch layer records it as a skipped result.
var ErrSkipped = errors.New("output exists; skipped (--skip-existing)")

// limits projects the bomb caps from Options into the emitter's limits.
func (o Options) limits() limits {
	return limits{maxBytes: o.MaxTotalBytes, maxEntries: o.MaxEntries}
}

// DefaultLevel is the Level value (zero) that selects the stdlib default
// Deflate compression rather than an explicit 1..9 level.
const DefaultLevel = 0

// Convert reads the RAR archive at srcRar and writes an equivalent ZIP archive
// at dstZip. Entries are streamed one at a time so memory stays bounded
// regardless of archive size. Per-entry modification time and Unix mode are
// preserved; entry names are sanitized to defend against Zip Slip. On any
// failure after the output file is created, the partial output is removed.
//
// When the pure-Go decoder cannot read the archive and opts.AllowFallback is
// set, Convert retries by shelling out to a system unrar/7z (see fallback.go).
func Convert(srcRar, dstZip string, opts Options) error {
	if !opts.Force {
		if _, err := os.Stat(dstZip); err == nil {
			if opts.SkipExisting {
				return ErrSkipped
			}
			return fmt.Errorf("output exists (use -f to overwrite): %s", dstZip)
		} else if !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("stat output %q: %w", dstZip, err)
		}
	}

	start := time.Now()
	base := filepath.Base(srcRar)
	err := convertNative(srcRar, dstZip, opts)
	if err != nil && opts.AllowFallback {
		opts.vlog("%s: native decode failed (%v); trying system unrar/7z fallback", base, err)
		err = convertViaFallback(srcRar, dstZip, opts, err)
		if err == nil {
			opts.vlog("%s: converted via fallback in %s", base, time.Since(start).Round(time.Millisecond))
		}
		return err
	}
	if err == nil {
		opts.vlog("%s: converted via native decoder in %s", base, time.Since(start).Round(time.Millisecond))
	}
	return err
}

// vlog emits a verbose diagnostic when OnVerbose is set.
func (o Options) vlog(format string, a ...any) {
	if o.OnVerbose != nil {
		o.OnVerbose(fmt.Sprintf(format, a...))
	}
}

// convertNative performs the pure-Go RAR→ZIP conversion (no external tools).
func convertNative(srcRar, dstZip string, opts Options) error {
	var ropts []rardecode.Option
	if opts.Password != "" {
		ropts = append(ropts, rardecode.Password(opts.Password))
	}
	rr, err := rardecode.OpenReader(srcRar, ropts...)
	if err != nil {
		return fmt.Errorf("open rar %q: %w", srcRar, err)
	}
	defer rr.Close()

	// Write to a temporary file in the destination directory, then atomically
	// rename it into place only after a complete archive exists. A failed
	// conversion therefore never truncates or deletes the destination — which a
	// naive create-in-place would do under --force on a mid-stream failure.
	tmp, err := os.CreateTemp(filepath.Dir(dstZip), "."+filepath.Base(dstZip)+".*.tmp")
	if err != nil {
		return fmt.Errorf("create temp output for %q: %w", dstZip, err)
	}
	tmpName := tmp.Name()
	expected, err := writeZip(tmp, rr, opts)
	if err != nil {
		_ = tmp.Close()        // no-op if writeZip already closed it
		_ = os.Remove(tmpName) // discard the partial temp; dst is untouched
		return err
	}
	return finalizeOutput(tmpName, dstZip, expected, opts)
}

// finalizeOutput chmods the temp ZIP to 0644, atomically renames it into place,
// and (when opts.Verify) reopens it to confirm it matches expected. Any failure
// removes the partial output so a broken ZIP never lands at dstZip. Shared by
// the native and shell-out fallback paths.
func finalizeOutput(tmpName, dstZip string, expected map[string]int64, opts Options) error {
	// os.CreateTemp makes the file 0600; match the conventional 0644 for output.
	if err := os.Chmod(tmpName, 0o644); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("set output permissions %q: %w", dstZip, err)
	}
	if err := os.Rename(tmpName, dstZip); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("finalize output %q: %w", dstZip, err)
	}
	if opts.Verify {
		if err := verify(dstZip, expected); err != nil {
			_ = os.Remove(dstZip) // a ZIP that fails its own check is not a usable output
			return err
		}
	}
	return nil
}

// writeZip streams every entry from rr into a new ZIP written to out, closing
// the ZIP writer and out on success so late flush errors surface (rather than
// producing a silently truncated archive). It returns an expected name->size
// map (files by uncompressed size, directories as 0) that --verify checks the
// finished archive against.
func writeZip(out *os.File, rr *rardecode.ReadCloser, opts Options) (map[string]int64, error) {
	zw := zip.NewWriter(out)
	registerCompressor(zw, opts)
	em := newZipEmitter(zw, opts.limits(), opts.OnEntry)
	for {
		hdr, err := rr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read rar entry: %w", err)
		}

		// Reject any entry that would escape the archive root before it can
		// be written anywhere. rardecode reports names with '/' separators.
		name, err := sanitize(hdr.Name)
		if err != nil {
			return nil, fmt.Errorf("unsafe rar entry %q: %w", hdr.Name, err)
		}

		fh := &zip.FileHeader{Method: entryMethod(opts)}
		fh.SetMode(safeMode(hdr.Mode(), hdr.IsDir))
		if !hdr.ModificationTime.IsZero() {
			fh.Modified = hdr.ModificationTime
		}

		if hdr.IsDir {
			if err := em.emitDir(hdr.Name, strings.TrimRight(name, "/"), fh); err != nil {
				return nil, err
			}
			continue
		}
		if err := em.emitFile(hdr.Name, name, fh, rr); err != nil {
			return nil, err
		}
	}

	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("finalize zip: %w", err)
	}
	// Close explicitly and check: a late write failure (ENOSPC, network/FUSE)
	// only surfaces here; ignoring it would report success on a truncated archive.
	if err := out.Close(); err != nil {
		return nil, fmt.Errorf("close zip: %w", err)
	}
	return em.expected, nil
}
