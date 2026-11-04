package rarutil

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestResolveName_OccupiedVariants(t *testing.T) {
	e := newTestFsEmitter(t, t.TempDir(), limits{}, nil)
	for _, tc := range []struct{ raw, want string }{
		{"f.txt", "f.txt"}, {"f (1).txt", "f (1).txt"},
		{"f.txt", "f (2).txt"}, {"f (3).txt", "f (3).txt"},
		{"f.txt", "f (4).txt"}, {"f.txt", "f (5).txt"},
		{"f (1).txt", "f (1) (1).txt"},
		{"d/", "d/"}, {"d (1)/", "d (1)/"}, {"d/", "d (2)/"},
	} {
		got, err := e.resolveName(tc.raw, tc.raw)
		if err != nil || got != tc.want {
			t.Fatalf("resolveName(%q) = %q, %v; want %q", tc.raw, got, err, tc.want)
		}
	}
	if _, err := e.resolveName("f (2).txt", "f (2).txt"); err == nil {
		t.Fatal("literal name must not overwrite an already generated variant")
	}
}

func TestFsEmitter_ExistingFileNotTruncated(t *testing.T) {
	root := t.TempDir()
	full := filepath.Join(root, "f")
	if err := os.WriteFile(full, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	e := newTestFsEmitter(t, root, limits{}, nil)
	err := e.emitFile("f", "f", 0o600, time.Time{}, strings.NewReader("replacement"))
	if !errors.Is(err, os.ErrExist) {
		t.Errorf("emit = %v; want existing-file error", err)
	}
	assertStagedContent(t, full, "original")
	if e.count != 0 || e.bytes != 0 {
		t.Errorf("failed entry counted: count=%d bytes=%d", e.count, e.bytes)
	}
}

func TestFsEmitter_FilesystemAliases(t *testing.T) {
	for _, names := range [][2]string{{"Report", "report"}, {"caf\u00e9", "cafe\u0301"}} {
		for _, kind := range []string{"file", "implicit dir", "explicit dir", "implicit then explicit"} {
			t.Run(names[0]+"/"+kind, func(t *testing.T) {
				root := t.TempDir()
				e := newTestFsEmitter(t, root, limits{}, nil)
				first, second := names[0], names[1]
				var err error
				if kind == "explicit dir" {
					err = e.emitDir(first, first)
				} else {
					if kind != "file" {
						first += "/first"
					}
					err = e.emitFile(first, first, 0o600, time.Time{}, strings.NewReader("original"))
				}
				if err != nil {
					t.Fatal(err)
				}
				// Probe actual filesystem equivalence, never assume it from GOOS.
				_, statErr := os.Stat(filepath.Join(root, second))
				aliases := statErr == nil
				if statErr != nil && !os.IsNotExist(statErr) {
					t.Fatal(statErr)
				}
				t.Logf("filesystem aliases names: %v", aliases)
				if kind == "explicit dir" || kind == "implicit then explicit" {
					err = e.emitDir(second, second)
				} else {
					if kind == "implicit dir" {
						second += "/second"
					}
					err = e.emitFile(second, second, 0o600, time.Time{}, strings.NewReader("replacement"))
				}
				if aliases && !errors.Is(err, os.ErrExist) {
					t.Errorf("alias emit = %v; want existing-path error", err)
				} else if !aliases && err != nil {
					t.Errorf("distinct filesystem name rejected: %v", err)
				}
				if kind != "explicit dir" {
					assertStagedContent(t, filepath.Join(root, first), "original")
				}
				if !aliases && (kind == "file" || kind == "implicit dir") {
					assertStagedContent(t, filepath.Join(root, second), "replacement")
				}
			})
		}
	}
}

func TestFsEmitter_ImplicitDirectoryCanBecomeExplicit(t *testing.T) {
	root := t.TempDir()
	e := newTestFsEmitter(t, root, limits{}, nil)
	if err := e.emitFile("a/b/f", "a/b/f", 0o600, time.Time{}, strings.NewReader("data")); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"a", "a/b", "a/b"} {
		if err := e.emitDir(name, name); err != nil {
			t.Fatal(err)
		}
	}
	assertStagedContent(t, filepath.Join(root, "a/b/f"), "data")
	if fi, err := os.Stat(filepath.Join(root, "a/b (1)")); err != nil || !fi.IsDir() {
		t.Fatalf("duplicate directory missing: %v, %v", fi, err)
	}
}

func assertStagedContent(t *testing.T, full, want string) {
	t.Helper()
	got, err := os.ReadFile(full)
	if err != nil || string(got) != want {
		t.Errorf("%s = %q, %v; want %q", full, got, err, want)
	}
}
