package rarutil

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

// writeTestZip builds a ZIP at path containing the given file entries (name ->
// content) plus the given directory entries, and returns the expected
// name->size map a successful verify should accept.
func writeTestZip(t *testing.T, path string, files map[string]string, dirs []string) map[string]int64 {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	expected := map[string]int64{}
	for name, content := range files {
		w, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
		expected[name] = int64(len(content))
	}
	for _, d := range dirs {
		name := d + "/"
		if _, err := zw.CreateHeader(&zip.FileHeader{Name: name}); err != nil {
			t.Fatal(err)
		}
		expected[name] = 0
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return expected
}

func TestVerify_Matches(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ok.zip")
	expected := writeTestZip(t, path,
		map[string]string{"a.txt": "hello", "sub/b.txt": "world!!"},
		[]string{"sub"})

	if err := verify(path, expected); err != nil {
		t.Errorf("verify of a faithful ZIP failed: %v", err)
	}
}

func TestVerify_SizeMismatch(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.zip")
	expected := writeTestZip(t, path, map[string]string{"a.txt": "hello"}, nil)
	expected["a.txt"] = 999 // lie about the size

	if err := verify(path, expected); err == nil {
		t.Error("verify accepted a size mismatch, want error")
	}
}

func TestVerify_MissingEntry(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.zip")
	expected := writeTestZip(t, path, map[string]string{"a.txt": "hello"}, nil)
	expected["ghost.txt"] = 3 // expected but not in the ZIP

	if err := verify(path, expected); err == nil {
		t.Error("verify accepted a missing entry, want error")
	}
}

// TestVerify_ContentCorruption proves verify reads each entry to force a CRC
// check: a flipped content byte leaves the declared size unchanged, so a
// size-only check would pass — verify must catch it via the checksum.
func TestVerify_ContentCorruption(t *testing.T) {
	path := filepath.Join(t.TempDir(), "corrupt.zip")
	canary := "verify-crc-canary-AAAA"
	// Store (no compression) so the content bytes appear verbatim in the file
	// and can be located + flipped deterministically.
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	w, err := zw.CreateHeader(&zip.FileHeader{Name: "a.txt", Method: zip.Store})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte(canary)); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	f.Close()
	expected := map[string]int64{"a.txt": int64(len(canary))}

	// Sanity: the faithful ZIP verifies.
	if err := verify(path, expected); err != nil {
		t.Fatalf("verify of faithful ZIP failed: %v", err)
	}

	// Flip a byte inside the stored content; size is unchanged.
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	idx := bytesIndex(raw, []byte(canary))
	if idx < 0 {
		t.Fatal("canary content not found in ZIP bytes")
	}
	raw[idx] ^= 0xFF
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := verify(path, expected); err == nil {
		t.Error("verify accepted content corruption (CRC mismatch), want error")
	}
}

// bytesIndex is a tiny substring search to avoid importing bytes for one call.
func bytesIndex(haystack, needle []byte) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		match := true
		for j := range needle {
			if haystack[i+j] != needle[j] {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}

func TestVerify_CountMismatch(t *testing.T) {
	path := filepath.Join(t.TempDir(), "extra.zip")
	expected := writeTestZip(t, path,
		map[string]string{"a.txt": "x", "b.txt": "y"}, nil)
	delete(expected, "b.txt") // ZIP has one more entry than expected

	if err := verify(path, expected); err == nil {
		t.Error("verify accepted an entry-count mismatch, want error")
	}
}
