package rarutil

import (
	"os"
	"path/filepath"
	"testing"
)

// TestMakeStagingDir_CreatesUnderDestDir proves the staging dir lands inside
// destDir (same filesystem, so the later commit's os.Rename is O(1)) and that
// destDir is created if it doesn't exist yet.
func TestMakeStagingDir_CreatesUnderDestDir(t *testing.T) {
	parent := t.TempDir()
	destDir := filepath.Join(parent, "does", "not", "exist", "yet")

	staging, err := makeStagingDir(destDir)
	if err != nil {
		t.Fatalf("makeStagingDir: %v", err)
	}
	defer os.RemoveAll(staging)

	if filepath.Dir(staging) != destDir {
		t.Errorf("staging dir %q is not directly under destDir %q", staging, destDir)
	}
	if fi, statErr := os.Stat(destDir); statErr != nil || !fi.IsDir() {
		t.Errorf("destDir was not created: %v", statErr)
	}
}

// TestCommitStaged_NestedDirRollback proves that when a commit walk creates
// several nested directory levels in one pass and then a LATER entry
// collides and aborts the walk, every directory this commit created is
// removed — not just a single tracked path per emit call. This is the
// concrete regression the red-team's manifest-based-rollback finding (a
// single filepath.Dir(fullPath) MkdirAll call can create multiple levels at
// once) is about; the staging+commit redesign closes it by tracking every
// directory actually created during the WalkDir pass, not just one per call.
func TestCommitStaged_NestedDirRollback(t *testing.T) {
	destDir := t.TempDir()
	// Only a root-level collision exists; "deep/nested/path/" doesn't exist
	// yet on the destDir side at all, so the commit walk must create all
	// three levels fresh before it reaches the collision.
	if err := os.WriteFile(filepath.Join(destDir, "zzz-conflict.txt"), []byte("existing"), 0o644); err != nil {
		t.Fatal(err)
	}

	stagingDir := t.TempDir()
	// Lexically "deep/..." sorts before "zzz-conflict.txt", so WalkDir visits
	// (and successfully commits) the nested entry — creating "deep/",
	// "deep/nested/", and "deep/nested/path/" along the way — before it
	// reaches the collision and aborts.
	writeStagingFile(t, stagingDir, "deep/nested/path/a.txt", "new-a")
	writeStagingFile(t, stagingDir, "zzz-conflict.txt", "new-conflict")

	before := snapshotTree(t, destDir)

	if _, err := commitStaged(stagingDir, destDir, Options{}); err == nil {
		t.Fatal("expected collision error")
	}

	after := snapshotTree(t, destDir)
	if before != after {
		t.Errorf("destDir tree changed by a rolled-back commit (nested dirs not fully cleaned up):\nbefore: %q\nafter:  %q", before, after)
	}
}

// snapshotTree returns a stable, comparable description of every path under
// root (relative paths + whether each is a directory).
func snapshotTree(t *testing.T, root string) string {
	t.Helper()
	var out string
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(root, p)
		if relErr != nil {
			return relErr
		}
		if d.IsDir() {
			out += rel + "/\n"
		} else {
			out += rel + "\n"
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}
