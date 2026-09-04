package rarutil

import (
	"archive/zip"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// fallbackTools lists the external extractors to try, in preference order.
var fallbackTools = []string{"unrar", "7z"}

// lookFallbackTool returns the first available external extractor (name + full
// path), or ("", "") when none is installed.
func lookFallbackTool() (string, string) {
	for _, tool := range fallbackTools {
		if p, err := exec.LookPath(tool); err == nil {
			return tool, p
		}
	}
	return "", ""
}

// convertViaFallback extracts srcRar with a system unrar/7z into a temp dir and
// re-packs that tree into dstZip. It is only reached when the pure-Go decoder
// failed (nativeErr) and the caller opted in via --allow-fallback. When no tool
// is installed it returns an error wrapping nativeErr (the original cause).
func convertViaFallback(srcRar, dstZip string, opts Options, nativeErr error) error {
	tool, toolPath := lookFallbackTool()
	if tool == "" {
		return fmt.Errorf("pure-Go decode failed and no fallback tool (unrar/7z) found: %w", nativeErr)
	}

	// Extraction stays in the system temp dir (TMPDIR): relocating the multi-GB
	// extracted tree onto the output volume could fill the user's home disk and
	// needs ~2x headroom (extracted tree + the zip). Users redirect via TMPDIR.
	workDir, err := os.MkdirTemp("", "rar2zip-fallback-*")
	if err != nil {
		return fmt.Errorf("fallback temp dir: %w", err)
	}
	defer os.RemoveAll(workDir)

	if err := ensureFallbackSpace(srcRar, workDir); err != nil {
		return err
	}

	if err := runExtract(toolPath, extractArgs(tool, srcRar, workDir, opts.Password)); err != nil {
		return fmt.Errorf("fallback %s extract failed: %w (pure-Go decode error: %v)", tool, err, nativeErr)
	}
	return zipDir(workDir, dstZip, opts)
}

// extractArgs builds the argument list to extract srcRar into destDir for the
// given tool, running non-interactively (never prompting for a password).
//
// A "--" end-of-options marker precedes the source path so a name beginning
// with '-' is treated as a file, not a switch. All switches (including 7z's
// -o<dir>) must come before "--". safeArgPath additionally defuses a leading
// '@', which both tools would otherwise read as a list-file directive.
func extractArgs(tool, srcRar, destDir, password string) []string {
	src := safeArgPath(srcRar)
	switch tool {
	case "7z":
		args := []string{"x", "-y"}
		if password != "" {
			args = append(args, "-p"+password)
		}
		return append(args, "-o"+destDir, "--", src)
	default: // unrar
		args := []string{"x", "-o+"}
		if password != "" {
			args = append(args, "-p"+password)
		} else {
			args = append(args, "-p-") // never query for a password
		}
		// destDir is rar2zip's own temp (not attacker-controlled); it follows
		// the source as a positional argument after the "--" marker.
		return append(args, "--", src, destDir+string(os.PathSeparator))
	}
}

// ensureFallbackSpace fails early when the temp filesystem clearly cannot hold
// the extraction. Because the native decoder already failed, rar2zip cannot read
// the archive's headers to size the extracted tree, so this is a conservative
// FLOOR — free space below the archive's own (compressed) size guarantees
// failure since the extracted tree is at least that large — not a full
// reservation. The extractor's own out-of-space error remains the hard bound;
// on platforms without statfs the check is skipped.
func ensureFallbackSpace(srcRar, workDir string) error {
	avail, err := availableBytes(workDir)
	if err != nil || avail < 0 {
		return nil // unknown free space: defer to the extractor's ENOSPC
	}
	fi, err := os.Stat(srcRar)
	if err != nil {
		return nil // can't size the source; let the extractor surface the problem
	}
	if insufficientSpace(avail, fi.Size()) {
		return fmt.Errorf("insufficient free space to extract %q: %d bytes free in temp dir %s, "+
			"but the archive alone is %d bytes (the fallback extracts before re-zipping; "+
			"free space or point TMPDIR at a larger volume)", srcRar, avail, os.TempDir(), fi.Size())
	}
	return nil
}

// insufficientSpace reports whether avail free bytes is below the archive's own
// size — a hard floor for extraction. A negative avail means "unknown" (the
// platform has no statfs) and never blocks.
func insufficientSpace(avail, rarSize int64) bool {
	return avail >= 0 && avail < rarSize
}

// safeArgPath rewrites a relative source path that begins with '@' to "./@…"
// so an extractor does not interpret it as a "@listfile" directive. The '-'
// case is handled by the caller's "--" end-of-options marker.
func safeArgPath(p string) string {
	if strings.HasPrefix(p, "@") {
		return "./" + p
	}
	return p
}

// runExtract executes the extractor, surfacing its combined output on failure.
func runExtract(toolPath string, args []string) error {
	cmd := exec.Command(toolPath, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// zipDir packs the tree rooted at srcDir into dstZip using the same atomic
// temp+rename+verify epilogue as the native path.
func zipDir(srcDir, dstZip string, opts Options) error {
	tmp, err := os.CreateTemp(filepath.Dir(dstZip), "."+filepath.Base(dstZip)+".*.tmp")
	if err != nil {
		return fmt.Errorf("create temp output for %q: %w", dstZip, err)
	}
	tmpName := tmp.Name()
	expected, err := writeZipFromDir(tmp, srcDir, opts)
	if err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	return finalizeOutput(tmpName, dstZip, expected, opts)
}

// writeZipFromDir walks srcDir and writes every file and directory into a new
// ZIP on out, preserving relative paths, mode (sanitized), and mtime. Symlinks
// are neutralized — their target is stored as inert regular-file content — and
// other non-regular entries (devices, pipes, sockets) are skipped. It returns
// the expected name->size map for --verify.
func writeZipFromDir(out *os.File, srcDir string, opts Options) (map[string]int64, error) {
	zw := zip.NewWriter(out)
	registerCompressor(zw, opts)
	em := newZipEmitter(zw, opts.limits(), opts.OnEntry)

	walkErr := filepath.WalkDir(srcDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p == srcDir {
			return nil // skip the root itself
		}
		rel, err := filepath.Rel(srcDir, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		info, err := d.Info()
		if err != nil {
			return err
		}

		if d.IsDir() {
			fh := &zip.FileHeader{}
			fh.SetMode(safeMode(info.Mode(), true))
			fh.Modified = info.ModTime()
			return em.emitDir(rel, rel, fh)
		}

		mode := info.Mode()
		fh := &zip.FileHeader{Method: entryMethod(opts)}
		fh.SetMode(safeMode(mode, false))
		fh.Modified = info.ModTime()

		// Symlink: store its target text as inert content (matches native
		// neutralization), never recreating a link that could escape on extract.
		if mode&fs.ModeSymlink != 0 {
			target, err := os.Readlink(p)
			if err != nil {
				return fmt.Errorf("read symlink %q: %w", rel, err)
			}
			return em.emitFile(rel, rel, fh, strings.NewReader(target))
		}
		if !mode.IsRegular() {
			return nil // devices/pipes/sockets carry no portable content
		}

		f, err := os.Open(p)
		if err != nil {
			return fmt.Errorf("open extracted file %q: %w", rel, err)
		}
		defer f.Close()
		return em.emitFile(rel, rel, fh, f)
	})
	if walkErr != nil {
		return nil, walkErr
	}

	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("finalize zip: %w", err)
	}
	if err := out.Close(); err != nil {
		return nil, fmt.Errorf("close zip: %w", err)
	}
	return em.expected, nil
}
