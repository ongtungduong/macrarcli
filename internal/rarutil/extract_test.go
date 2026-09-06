package rarutil

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nwaples/rardecode/v2"
)

// TestExtract_Fidelity extracts a real .rar fixture and asserts the resulting
// directory tree contains exactly the same files (names + content) and
// directories. Fixture-agnostic: drop any .rar into testdata/ and it gets
// exercised. Skips (not fails) when no fixture is present, matching this
// repo's established convention (RAR is proprietary; no programmatic fixture
// generation path exists).
func TestExtract_Fidelity(t *testing.T) {
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
			destDir := t.TempDir()
			if _, err := Extract(src, destDir, Options{}); err != nil {
				t.Fatalf("Extract(%s): %v", src, err)
			}

			wantFiles, wantDirs := digestRar(t, src)
			gotFiles, gotDirs := digestDir(t, destDir)

			if len(wantFiles) != len(gotFiles) {
				t.Errorf("file count: rar=%d dir=%d", len(wantFiles), len(gotFiles))
			}
			for name, sum := range wantFiles {
				got, ok := gotFiles[name]
				if !ok {
					t.Errorf("dir missing file %q", name)
					continue
				}
				if got != sum {
					t.Errorf("content mismatch for %q: rar=%s dir=%s", name, sum, got)
				}
			}
			for d := range wantDirs {
				if !gotDirs[d] {
					t.Errorf("dir missing directory %q", d)
				}
			}
		})
	}
}

// TestExtract_PreservesMetadata asserts extracted files carry the RAR entry's
// modification time and Unix permission bits.
func TestExtract_PreservesMetadata(t *testing.T) {
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
		want[hdr.Name] = meta{hdr.ModificationTime, safeMode(hdr.Mode(), false)}
	}
	rr.Close()

	destDir := t.TempDir()
	if _, err := Extract(src, destDir, Options{}); err != nil {
		t.Fatalf("Extract: %v", err)
	}

	mtimeChecked := 0
	err = filepath.WalkDir(destDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(destDir, p)
		w, ok := want[rel]
		if !ok {
			t.Errorf("dir has unexpected entry %q", rel)
			return nil
		}
		fi, statErr := d.Info()
		if statErr != nil {
			return statErr
		}
		if w.mode.Perm() != 0 && fi.Mode().Perm() != w.mode.Perm() {
			t.Errorf("%q: mode = %v, want %v", rel, fi.Mode().Perm(), w.mode.Perm())
		}
		if !w.mtime.IsZero() {
			if d := fi.ModTime().Sub(w.mtime); d < -2*time.Second || d > 2*time.Second {
				t.Errorf("%q: mtime = %v, want ~%v", rel, fi.ModTime(), w.mtime)
			}
			mtimeChecked++
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if mtimeChecked == 0 {
		t.Skip("fixture entries carry no modification time; mtime preservation not exercised")
	}
}

// TestExtract_OnEntry verifies the progress callback fires once per archive entry.
func TestExtract_OnEntry(t *testing.T) {
	matches, _ := filepath.Glob("../../testdata/*.rar")
	if len(matches) == 0 {
		t.Skip("no testdata/*.rar fixture present; skipping progress test")
	}
	src := matches[0]

	var seen []string
	destDir := t.TempDir()
	if _, err := Extract(src, destDir, Options{OnEntry: func(name string) { seen = append(seen, name) }}); err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if len(seen) == 0 {
		t.Fatal("OnEntry never called")
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

// digestDir returns sha256 hex per file (relative path) and the set of
// directory relative paths under root.
func digestDir(t *testing.T, root string) (files map[string]string, dirs map[string]bool) {
	t.Helper()
	files, dirs = map[string]string{}, map[string]bool{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p == root {
			return nil
		}
		rel, relErr := filepath.Rel(root, p)
		if relErr != nil {
			return relErr
		}
		if d.IsDir() {
			dirs[rel] = true
			return nil
		}
		f, openErr := os.Open(p)
		if openErr != nil {
			return openErr
		}
		defer f.Close()
		h := sha256.New()
		if _, err := io.Copy(h, f); err != nil {
			return err
		}
		files[rel] = hex.EncodeToString(h.Sum(nil))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files, dirs
}

// TestCommitStaged_ConcurrentJobsSharedDestDir proves the exact scenario the
// red-team's Critical Finding 1 described: two "jobs" committing into the
// same destDir concurrently never lose either job's output, and one job's
// failure never deletes the other's already-succeeded entries. Uses
// commitStaged directly (no RAR needed) since the concurrency guarantee lives
// entirely in the staging/commit boundary, not in RAR decoding.
func TestCommitStaged_ConcurrentJobsSharedDestDir(t *testing.T) {
	destDir := t.TempDir()

	stagingA := t.TempDir()
	writeStagingFile(t, stagingA, "from-a.txt", "content-a")
	stagingB := t.TempDir()
	writeStagingFile(t, stagingB, "from-b.txt", "content-b")

	done := make(chan error, 2)
	go func() { _, err := commitStaged(stagingA, destDir, Options{}); done <- err }()
	go func() { _, err := commitStaged(stagingB, destDir, Options{}); done <- err }()

	for i := 0; i < 2; i++ {
		if err := <-done; err != nil {
			t.Fatalf("commitStaged: %v", err)
		}
	}

	if got := readFile(t, filepath.Join(destDir, "from-a.txt")); got != "content-a" {
		t.Errorf("from-a.txt = %q, want \"content-a\" (job A's output lost)", got)
	}
	if got := readFile(t, filepath.Join(destDir, "from-b.txt")); got != "content-b" {
		t.Errorf("from-b.txt = %q, want \"content-b\" (job B's output lost)", got)
	}
}
