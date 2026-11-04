package rarutil

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestFsEmitter_BorrowedRootOwnership(t *testing.T) {
	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	e := newFsEmitterRoot(root, limits{}, nil)
	if err := e.emitFile("f", "f", 0o600, time.Time{}, strings.NewReader("data")); err != nil {
		t.Fatal(err)
	}
	if err := e.close(); err != nil {
		t.Fatal(err)
	}
	if got, err := root.ReadFile("f"); err != nil || string(got) != "data" {
		t.Fatalf("borrowed root closed: %q, %v", got, err)
	}
}

func TestFsEmitter_RootOpenError(t *testing.T) {
	e := newFsEmitter(filepath.Join(t.TempDir(), "missing"), limits{}, nil)
	defer e.close()
	if err := e.emitFile("f", "f", 0o600, time.Time{}, strings.NewReader("data")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("emit = %v; want missing-root error", err)
	}
}

func TestFsEmitter_PinnedRootSurvivesPathReplacement(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires renaming an open directory and unprivileged symlinks")
	}
	parent := t.TempDir()
	root, moved, outside := filepath.Join(parent, "stage"), filepath.Join(parent, "moved"), t.TempDir()
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	e := newTestFsEmitter(t, root, limits{}, nil)
	if err := os.Rename(root, moved); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, root); err != nil {
		t.Fatal(err)
	}
	if err := e.emitFile("a/f", "a/f", 0o600, time.Time{}, strings.NewReader("data")); err != nil {
		t.Fatal(err)
	}
	assertStagedContent(t, filepath.Join(moved, "a/f"), "data")
	if entries, err := os.ReadDir(outside); err != nil || len(entries) != 0 {
		t.Fatalf("replacement path was used: %v, %v", entries, err)
	}
}

func TestFsEmitter_MetadataUsesOpenFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires renaming an open file and unprivileged symlinks")
	}
	root := t.TempDir()
	e := newTestFsEmitter(t, root, limits{}, nil)
	outside := filepath.Join(t.TempDir(), "untouched")
	if err := os.WriteFile(outside, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(outside)
	if err != nil {
		t.Fatal(err)
	}
	wantTime := time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)
	content := &beforeRead{Reader: strings.NewReader("data"), before: func() {
		if err := os.Rename(filepath.Join(root, "f"), filepath.Join(root, "moved")); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, filepath.Join(root, "f")); err != nil {
			t.Fatal(err)
		}
	}}
	if err := e.emitFile("f", "f", 0o740, wantTime, content); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(filepath.Join(root, "moved"))
	if err != nil || fi.Mode().Perm() != 0o740 || !fi.ModTime().Equal(wantTime) {
		t.Fatalf("open-file metadata not restored: %v, %v", fi, err)
	}
	after, err := os.Stat(outside)
	if err != nil || after.Mode() != before.Mode() || !after.ModTime().Equal(before.ModTime()) {
		t.Fatalf("outside metadata changed: %v, %v", after, err)
	}
	assertStagedContent(t, outside, "outside")
}

type beforeRead struct {
	io.Reader
	before func()
}

func (r *beforeRead) Read(p []byte) (int, error) {
	if r.before != nil {
		r.before()
		r.before = nil
	}
	return r.Reader.Read(p)
}
