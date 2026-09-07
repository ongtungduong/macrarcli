package rarutil

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

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
func TestFsEmitter_RenamesExactDuplicate(t *testing.T) {
	root := t.TempDir()
	em := newFsEmitter(root, limits{}, nil)

	if err := em.emitFile("dup.txt", "dup.txt", 0o644, time.Time{}, strings.NewReader("first")); err != nil {
		t.Fatalf("first emit: %v", err)
	}
	if err := em.emitFile("dup.txt", "dup.txt", 0o644, time.Time{}, strings.NewReader("second")); err != nil {
		t.Fatalf("second emit: %v", err)
	}

	a, err := os.ReadFile(filepath.Join(root, "dup.txt"))
	if err != nil || string(a) != "first" {
		t.Errorf("dup.txt = %q, %v, want \"first\"", a, err)
	}
	b, err := os.ReadFile(filepath.Join(root, "dup (1).txt"))
	if err != nil || string(b) != "second" {
		t.Errorf("dup (1).txt = %q, %v, want \"second\"", b, err)
	}
}

// Two DIFFERENT raw names that map to the same sanitized name (a post-sanitize
// collision) are rejected outright.
func TestFsEmitter_RejectsPostSanitizeCollision(t *testing.T) {
	root := t.TempDir()
	em := newFsEmitter(root, limits{}, nil)

	if err := em.emitFile("b", "b", 0o644, time.Time{}, strings.NewReader("x")); err != nil {
		t.Fatalf("first emit: %v", err)
	}
	// "a/../b" sanitizes to "b" but its raw form differs -> collision.
	err := em.emitFile("a/../b", "b", 0o644, time.Time{}, strings.NewReader("y"))
	if err == nil {
		t.Fatal("expected post-sanitize collision to be rejected")
	}
}

// A file "b" and a directory "b" produce distinct staging paths ("b" vs "b/"
// as dedup keys, "b" vs the "b" directory on disk) and must not collide.
func TestFsEmitter_FileAndDirSameBaseCoexist(t *testing.T) {
	root := t.TempDir()
	em := newFsEmitter(root, limits{}, nil)

	if err := em.emitDir("b", "b"); err != nil {
		t.Fatalf("dir emit: %v", err)
	}
	if err := em.emitFile("b/c.txt", "b/c.txt", 0o644, time.Time{}, strings.NewReader("x")); err != nil {
		t.Fatalf("file emit: %v", err)
	}
	fi, err := os.Stat(filepath.Join(root, "b"))
	if err != nil || !fi.IsDir() {
		t.Errorf("root/b should be a directory: %v, %v", fi, err)
	}
}

func TestFsEmitter_ByteCapErrors(t *testing.T) {
	root := t.TempDir()
	em := newFsEmitter(root, limits{maxBytes: 4}, nil)

	err := em.emitFile("big.txt", "big.txt", 0o644, time.Time{}, strings.NewReader("0123456789"))
	if !errors.Is(err, errLimitBytes) {
		t.Fatalf("err = %v, want errLimitBytes", err)
	}
}

func TestFsEmitter_EntryCapErrors(t *testing.T) {
	root := t.TempDir()
	em := newFsEmitter(root, limits{maxEntries: 1}, nil)

	if err := em.emitFile("a.txt", "a.txt", 0o644, time.Time{}, strings.NewReader("x")); err != nil {
		t.Fatalf("first emit: %v", err)
	}
	err := em.emitFile("b.txt", "b.txt", 0o644, time.Time{}, strings.NewReader("y"))
	if !errors.Is(err, errLimitEntries) {
		t.Fatalf("err = %v, want errLimitEntries", err)
	}
}

// TestFsEmitter_PreservesModeAndMtime proves emitFile restores real POSIX
// permission bits and modification time on the staged file — the actual new
// capability this phase adds over the old zip-header-only preservation.
func TestFsEmitter_PreservesModeAndMtime(t *testing.T) {
	root := t.TempDir()
	em := newFsEmitter(root, limits{}, nil)

	want := time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)
	if err := em.emitFile("exe", "exe", 0o750, want, strings.NewReader("bin")); err != nil {
		t.Fatalf("emit: %v", err)
	}
	fi, err := os.Stat(filepath.Join(root, "exe"))
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o750 {
		t.Errorf("mode = %v, want 0750", fi.Mode().Perm())
	}
	if !fi.ModTime().Equal(want) {
		t.Errorf("mtime = %v, want %v", fi.ModTime(), want)
	}
}

// TestFsEmitter_OnEntryFiresOncePerEntry verifies the progress callback fires
// once per staged entry (files and directories).
func TestFsEmitter_OnEntryFiresOncePerEntry(t *testing.T) {
	root := t.TempDir()
	var seen []string
	em := newFsEmitter(root, limits{}, func(name string) { seen = append(seen, name) })

	if err := em.emitDir("d", "d"); err != nil {
		t.Fatal(err)
	}
	if err := em.emitFile("d/f.txt", "d/f.txt", 0o644, time.Time{}, strings.NewReader("x")); err != nil {
		t.Fatal(err)
	}
	if len(seen) != 2 {
		t.Fatalf("OnEntry calls = %d, want 2: %v", len(seen), seen)
	}
}
