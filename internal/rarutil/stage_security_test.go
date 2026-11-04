package rarutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCommitRoot_DestinationRenameCannotRedirectWrites(t *testing.T) {
	parent := t.TempDir()
	dest := filepath.Join(parent, "dest")
	stage, err := makeStagingDir(dest)
	if err != nil {
		t.Fatal(err)
	}
	writeStagingFile(t, stage, "file", "safe")
	root, err := os.OpenRoot(dest)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	moved := filepath.Join(parent, "moved")
	if err := os.Rename(dest, moved); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, dest); err != nil {
		t.Fatal(err)
	}
	if _, err := commitRoot(root, filepath.Base(stage), Options{}); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, filepath.Join(moved, "file")); got != "safe" {
		t.Fatal("wrong output")
	}
	if _, err := os.Stat(filepath.Join(outside, "file")); !os.IsNotExist(err) {
		t.Fatalf("escaped root: %v", err)
	}
}

func TestPublishFile_RejectsParentSymlinkAndExistingTargets(t *testing.T) {
	dest := t.TempDir()
	root, err := os.OpenRoot(dest)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if err := root.WriteFile("source", []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := root.Mkdir("dir", 0o755); err != nil {
		t.Fatal(err)
	}
	// Model the swap after a directory existence check, before publication.
	if err := root.Remove("dir"); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err := root.Symlink(outside, "dir"); err != nil {
		t.Fatal(err)
	}
	if _, err := publishFile(root, "source", "dir/escaped"); err == nil {
		t.Fatal("accepted outside parent")
	}
	if _, err := os.Stat(filepath.Join(outside, "escaped")); !os.IsNotExist(err) {
		t.Fatalf("escaped: %v", err)
	}
	if err := root.WriteFile("existing", []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := publishFile(root, "source", "existing"); !os.IsExist(err) {
		t.Fatalf("want exclusive publication error, got %v", err)
	}
	if got := readFile(t, filepath.Join(dest, "existing")); got != "original" {
		t.Fatal("existing file overwritten")
	}
}
