package rarutil

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"io/fs"
	"path/filepath"
	"testing"
	"time"

	"github.com/nwaples/rardecode/v2"
)

// TestConvert_Fidelity converts a real .rar fixture and asserts the resulting
// .zip contains exactly the same files (names + content) and directories.
// It is fixture-agnostic: drop any .rar into testdata/ and it gets exercised.
// Skips (not fails) when no fixture is present so the suite stays green in
// environments without the (large) binary fixture committed.
func TestConvert_Fidelity(t *testing.T) {
	matches, err := filepath.Glob("../../testdata/*.rar")
	if err != nil {
		t.Fatalf("glob fixtures: %v", err)
	}
	if len(matches) == 0 {
		t.Skip("no testdata/*.rar fixture present; skipping fidelity test")
	}

	for _, src := range matches {
		src := src
		t.Run(filepath.Base(src), func(t *testing.T) {
			dst := filepath.Join(t.TempDir(), "out.zip")
			if err := Convert(src, dst, Options{}); err != nil {
				t.Fatalf("Convert(%s): %v", src, err)
			}

			wantFiles, wantDirs := digestRar(t, src)
			gotFiles, gotDirs := digestZip(t, dst)

			if len(wantFiles) != len(gotFiles) {
				t.Errorf("file count: rar=%d zip=%d", len(wantFiles), len(gotFiles))
			}
			for name, sum := range wantFiles {
				got, ok := gotFiles[name]
				if !ok {
					t.Errorf("zip missing file %q", name)
					continue
				}
				if got != sum {
					t.Errorf("content mismatch for %q: rar=%s zip=%s", name, sum, got)
				}
			}
			for d := range wantDirs {
				if !gotDirs[d+"/"] {
					t.Errorf("zip missing directory entry %q/", d)
				}
			}
		})
	}
}

// TestConvert_PreservesMetadata asserts that file entries in the output ZIP
// carry the RAR entry's modification time and Unix mode, and are Deflate-compressed.
func TestConvert_PreservesMetadata(t *testing.T) {
	matches, err := filepath.Glob("../../testdata/*.rar")
	if err != nil {
		t.Fatalf("glob fixtures: %v", err)
	}
	if len(matches) == 0 {
		t.Skip("no testdata/*.rar fixture present; skipping metadata test")
	}
	src := matches[0]

	type meta struct {
		mtime time.Time
		mode  fs.FileMode
	}
	want := map[string]meta{}
	rr, err := rardecode.OpenReader(src)
	if err != nil {
		t.Fatalf("open rar: %v", err)
	}
	for {
		hdr, err := rr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("rar next: %v", err)
		}
		if hdr.IsDir {
			continue
		}
		want[hdr.Name] = meta{hdr.ModificationTime, hdr.Mode()}
	}
	rr.Close()

	dst := filepath.Join(t.TempDir(), "out.zip")
	if err := Convert(src, dst, Options{}); err != nil {
		t.Fatalf("Convert: %v", err)
	}

	zr, err := zip.OpenReader(dst)
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}
	defer zr.Close()

	mtimeChecked := 0
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		w, ok := want[f.Name]
		if !ok {
			t.Errorf("zip has unexpected entry %q", f.Name)
			continue
		}
		if f.Method != zip.Deflate {
			t.Errorf("%q: method = %d, want Deflate(%d)", f.Name, f.Method, zip.Deflate)
		}
		if w.mode != 0 && f.Mode() != w.mode {
			t.Errorf("%q: mode = %v, want %v", f.Name, f.Mode(), w.mode)
		}
		if !w.mtime.IsZero() {
			if d := f.Modified.Sub(w.mtime); d < -2*time.Second || d > 2*time.Second {
				t.Errorf("%q: mtime = %v, want ~%v", f.Name, f.Modified, w.mtime)
			}
			mtimeChecked++
		}
	}
	if mtimeChecked == 0 {
		t.Skip("fixture entries carry no modification time; mtime preservation not exercised")
	}
}

// TestConvert_OnEntry verifies the progress callback fires once per archive entry.
func TestConvert_OnEntry(t *testing.T) {
	matches, _ := filepath.Glob("../../testdata/*.rar")
	if len(matches) == 0 {
		t.Skip("no testdata/*.rar fixture present; skipping progress test")
	}
	src := matches[0]

	var seen []string
	dst := filepath.Join(t.TempDir(), "out.zip")
	if err := Convert(src, dst, Options{OnEntry: func(name string) { seen = append(seen, name) }}); err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if len(seen) == 0 {
		t.Fatal("OnEntry never called")
	}

	// One callback per stored entry (files + directories).
	zr, err := zip.OpenReader(dst)
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}
	defer zr.Close()
	if len(seen) != len(zr.File) {
		t.Errorf("OnEntry calls = %d, zip entries = %d", len(seen), len(zr.File))
	}
}

// digestRar returns sha256 hex per file entry and the set of directory names.
func digestRar(t *testing.T, path string) (files map[string]string, dirs map[string]bool) {
	t.Helper()
	files, dirs = map[string]string{}, map[string]bool{}
	rr, err := rardecode.OpenReader(path)
	if err != nil {
		t.Fatalf("open rar: %v", err)
	}
	defer rr.Close()
	for {
		hdr, err := rr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("rar next: %v", err)
		}
		if hdr.IsDir {
			dirs[hdr.Name] = true
			continue
		}
		h := sha256.New()
		if _, err := io.Copy(h, rr); err != nil {
			t.Fatalf("read rar entry %q: %v", hdr.Name, err)
		}
		files[hdr.Name] = hex.EncodeToString(h.Sum(nil))
	}
	return files, dirs
}

// digestZip returns sha256 hex per file entry and the set of directory names.
func digestZip(t *testing.T, path string) (files map[string]string, dirs map[string]bool) {
	t.Helper()
	files, dirs = map[string]string{}, map[string]bool{}
	zr, err := zip.OpenReader(path)
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}
	defer zr.Close()
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			dirs[f.Name] = true
			continue
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open zip entry %q: %v", f.Name, err)
		}
		h := sha256.New()
		if _, err := io.Copy(h, rc); err != nil {
			rc.Close()
			t.Fatalf("read zip entry %q: %v", f.Name, err)
		}
		rc.Close()
		files[f.Name] = hex.EncodeToString(h.Sum(nil))
	}
	return files, dirs
}
