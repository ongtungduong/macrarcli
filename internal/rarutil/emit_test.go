package rarutil

import (
	"archive/zip"
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
)

// readZip returns name->content for every file entry and the set of directory
// names in the ZIP held in buf.
func readZip(t *testing.T, buf *bytes.Buffer) (files map[string]string, dirs map[string]bool) {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}
	files, dirs = map[string]string{}, map[string]bool{}
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			dirs[f.Name] = true
			continue
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open entry %q: %v", f.Name, err)
		}
		b, _ := io.ReadAll(rc)
		rc.Close()
		files[f.Name] = string(b)
	}
	return files, dirs
}

func TestDedupVariant(t *testing.T) {
	cases := []struct{ in, want string }{
		{"f.txt", "f (1).txt"},
		{"a/b.txt", "a/b (1).txt"},
		{"noext", "noext (1)"},
		{"d/", "d (1)/"},
	}
	for _, c := range cases {
		if got := dedupVariant(c.in, 1); got != c.want {
			t.Errorf("dedupVariant(%q,1) = %q, want %q", c.in, got, c.want)
		}
	}
}

// An exact-duplicate name (same raw source) is preserved by renaming, never
// dropped — guarding against silent data loss on legitimate repeats.
func TestZipEmitter_RenamesExactDuplicate(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	em := newZipEmitter(zw, limits{}, nil)

	if err := em.emitFile("dup.txt", "dup.txt", &zip.FileHeader{Method: zip.Deflate}, strings.NewReader("first")); err != nil {
		t.Fatalf("first emit: %v", err)
	}
	if err := em.emitFile("dup.txt", "dup.txt", &zip.FileHeader{Method: zip.Deflate}, strings.NewReader("second")); err != nil {
		t.Fatalf("second emit: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}

	files, _ := readZip(t, &buf)
	if len(files) != 2 {
		t.Fatalf("entries = %d, want 2 (no silent collapse): %v", len(files), files)
	}
	if files["dup.txt"] != "first" || files["dup (1).txt"] != "second" {
		t.Errorf("got %v, want dup.txt=first dup (1).txt=second", files)
	}
}

// Two DIFFERENT raw names that map to the same sanitized name (a post-sanitize
// collision) are rejected outright.
func TestZipEmitter_RejectsPostSanitizeCollision(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	em := newZipEmitter(zw, limits{}, nil)

	if err := em.emitFile("b", "b", &zip.FileHeader{Method: zip.Deflate}, strings.NewReader("x")); err != nil {
		t.Fatalf("first emit: %v", err)
	}
	// "a/../b" sanitizes to "b" but its raw form differs -> collision.
	err := em.emitFile("a/../b", "b", &zip.FileHeader{Method: zip.Deflate}, strings.NewReader("y"))
	if err == nil {
		t.Fatal("expected post-sanitize collision to be rejected")
	}
}

// A file "b" and a directory "b" produce distinct ZIP names ("b" vs "b/") and
// must not be treated as a collision.
func TestZipEmitter_FileAndDirSameBaseCoexist(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	em := newZipEmitter(zw, limits{}, nil)

	if err := em.emitFile("b", "b", &zip.FileHeader{Method: zip.Deflate}, strings.NewReader("x")); err != nil {
		t.Fatalf("file emit: %v", err)
	}
	if err := em.emitDir("b", "b", &zip.FileHeader{}); err != nil {
		t.Fatalf("dir emit: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	files, dirs := readZip(t, &buf)
	if len(files) != 1 || !dirs["b/"] {
		t.Errorf("want file b/ and dir b/: files=%v dirs=%v", files, dirs)
	}
}

func TestZipEmitter_ByteCapErrors(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	em := newZipEmitter(zw, limits{maxBytes: 4}, nil)

	err := em.emitFile("big.txt", "big.txt", &zip.FileHeader{Method: zip.Store}, strings.NewReader("0123456789"))
	if !errors.Is(err, errLimitBytes) {
		t.Fatalf("err = %v, want errLimitBytes", err)
	}
}

func TestZipEmitter_EntryCapErrors(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	em := newZipEmitter(zw, limits{maxEntries: 1}, nil)

	if err := em.emitFile("a.txt", "a.txt", &zip.FileHeader{Method: zip.Deflate}, strings.NewReader("x")); err != nil {
		t.Fatalf("first emit: %v", err)
	}
	err := em.emitFile("b.txt", "b.txt", &zip.FileHeader{Method: zip.Deflate}, strings.NewReader("y"))
	if !errors.Is(err, errLimitEntries) {
		t.Fatalf("err = %v, want errLimitEntries", err)
	}
}
