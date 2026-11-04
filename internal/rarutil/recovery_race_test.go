package rarutil

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBackupFile_PreservesSubstitutedDirectory(t *testing.T) {
	dest := t.TempDir()
	root, err := os.OpenRoot(dest)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if err := root.WriteFile("file", []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	expected, err := root.Lstat("file")
	if err != nil {
		t.Fatal(err)
	}
	if err := root.Remove("file"); err != nil {
		t.Fatal(err)
	}
	if err := root.Mkdir("file", 0o700); err != nil {
		t.Fatal(err)
	}
	if err := root.WriteFile("file/unrelated", []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	tx := commitTransaction{root: root}
	if _, err := tx.backupFile("file", expected); err == nil {
		t.Fatal("accepted substituted directory")
	}
	if err := tx.rollback(); err == nil {
		t.Fatal("missing recovery error")
	}
	if got := readFile(t, filepath.Join(dest, tx.backupDir, "original-0/unrelated")); got != "keep" {
		t.Fatal("deleted unrelated content")
	}
}

func TestRollback_PreservesSubstitutedFilesAndDirectories(t *testing.T) {
	for _, directory := range []bool{false, true} {
		t.Run(map[bool]string{true: "directory", false: "file"}[directory], func(t *testing.T) {
			dest := t.TempDir()
			root, err := os.OpenRoot(dest)
			if err != nil {
				t.Fatal(err)
			}
			defer root.Close()
			if directory {
				err = root.Mkdir("owned", 0o700)
			} else {
				err = root.WriteFile("owned", []byte("ours"), 0o600)
			}
			if err != nil {
				t.Fatal(err)
			}
			expected, err := root.Lstat("owned")
			if err != nil {
				t.Fatal(err)
			}
			// Keep the original inode allocated so replacement identity differs.
			if err := root.Rename("owned", "moved"); err != nil {
				t.Fatal(err)
			}
			if err := root.WriteFile("owned", []byte("concurrent"), 0o600); err != nil {
				t.Fatal(err)
			}
			tx := commitTransaction{root: root}
			if directory {
				tx.createdDirs = []commitDir{{"owned", expected}}
			} else {
				tx.done = []commitEntry{{dest: "owned", published: expected}}
			}
			if err := tx.rollback(); err == nil {
				t.Fatal("missing recovery error")
			}
			if got := readFile(t, filepath.Join(dest, tx.backupDir, "rollback-0")); got != "concurrent" {
				t.Fatal("lost replacement")
			}
		})
	}
}

func TestExtract_StagingSubstitutionPreservesBothTrees(t *testing.T) {
	src := writeReaderFixture(t, readerFixtureEntry{name: "file", data: "original"})
	dest := t.TempDir()
	var replaced string
	_, err := Extract(src, dest, Options{OnEntry: func(string) {
		names, err := os.ReadDir(dest)
		if err != nil {
			t.Fatal(err)
		}
		for _, name := range names {
			if strings.HasPrefix(name.Name(), ".macrarcli-staging-") {
				replaced = filepath.Join(dest, name.Name())
				break
			}
		}
		if replaced == "" {
			t.Fatal("no staging")
		}
		if err := os.Rename(replaced, filepath.Join(dest, "moved")); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(replaced, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(replaced, "unrelated"), []byte("keep"), 0o600); err != nil {
			t.Fatal(err)
		}
	}})
	if err == nil {
		t.Fatal("accepted staging replacement")
	}
	if got := readFile(t, filepath.Join(replaced, "unrelated")); got != "keep" {
		t.Fatal("deleted replacement")
	}
	if got := readFile(t, filepath.Join(dest, "moved/file")); got != "original" {
		t.Fatal("lost original staging")
	}
	if _, err := os.Stat(filepath.Join(dest, "file")); !os.IsNotExist(err) {
		t.Fatal("published substituted staging")
	}
}

func TestFileTimes_PreservesNanoseconds(t *testing.T) {
	f, err := os.Create(filepath.Join(t.TempDir(), "file"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	want := time.Date(2025, 1, 2, 3, 4, 5, 123456700, time.UTC)
	if err := setFileTimes(f, want); err != nil {
		t.Fatal(err)
	}
	info, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if !info.ModTime().Equal(want) {
		t.Fatalf("mtime=%v want=%v", info.ModTime(), want)
	}
}
