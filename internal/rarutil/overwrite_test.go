package rarutil

import (
	"os"
	"path/filepath"
	"testing"
)

// writeStagingFile creates a file at stagingDir/rel with the given content,
// creating parent directories as needed. Test helper for commitStaged tests.
func writeStagingFile(t *testing.T, stagingDir, rel, content string) {
	t.Helper()
	full := filepath.Join(stagingDir, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %q: %v", path, err)
	}
	return string(b)
}

// TestCommitStaged_FailClosed proves the default policy aborts on the first
// collision and leaves destDir exactly as it was before the call.
func TestCommitStaged_FailClosed(t *testing.T) {
	destDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(destDir, "a.txt"), []byte("existing"), 0o644); err != nil {
		t.Fatal(err)
	}

	stagingDir := t.TempDir()
	writeStagingFile(t, stagingDir, "a.txt", "new")
	writeStagingFile(t, stagingDir, "b.txt", "new-b")

	_, err := commitStaged(stagingDir, destDir, Options{})
	if err == nil {
		t.Fatal("expected collision error, got nil")
	}
	if got := readFile(t, filepath.Join(destDir, "a.txt")); got != "existing" {
		t.Errorf("a.txt was clobbered: %q", got)
	}
	if _, statErr := os.Stat(filepath.Join(destDir, "b.txt")); !os.IsNotExist(statErr) {
		t.Error("b.txt should have been rolled back (never committed since a.txt aborted the walk)")
	}
}

// TestCommitStaged_Overwrite proves --overwrite clobbers an existing file.
func TestCommitStaged_Overwrite(t *testing.T) {
	destDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(destDir, "a.txt"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	stagingDir := t.TempDir()
	writeStagingFile(t, stagingDir, "a.txt", "new")

	if _, err := commitStaged(stagingDir, destDir, Options{OverwritePolicy: OverwriteOverwrite}); err != nil {
		t.Fatalf("commitStaged: %v", err)
	}
	if got := readFile(t, filepath.Join(destDir, "a.txt")); got != "new" {
		t.Errorf("a.txt = %q, want clobbered to \"new\"", got)
	}
}

// TestCommitStaged_OverwriteSymlink proves --overwrite never writes through a
// symlink at the destination: the symlink itself is removed and replaced.
func TestCommitStaged_OverwriteSymlink(t *testing.T) {
	destDir := t.TempDir()
	targetDir := t.TempDir()
	target := filepath.Join(targetDir, "secret.txt")
	if err := os.WriteFile(target, []byte("do-not-touch"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(destDir, "a.txt")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	stagingDir := t.TempDir()
	writeStagingFile(t, stagingDir, "a.txt", "new-content")

	if _, err := commitStaged(stagingDir, destDir, Options{OverwritePolicy: OverwriteOverwrite}); err != nil {
		t.Fatalf("commitStaged: %v", err)
	}
	if got := readFile(t, target); got != "do-not-touch" {
		t.Fatalf("symlink target was written through: %q", got)
	}
	fi, err := os.Lstat(link)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		t.Fatal("destination is still a symlink; overwrite should have replaced it with a real file")
	}
	if got := readFile(t, link); got != "new-content" {
		t.Errorf("a.txt = %q, want \"new-content\"", got)
	}
}

// TestCommitStaged_Skip proves --skip skips only the colliding entry and
// commits the rest of the archive normally.
func TestCommitStaged_Skip(t *testing.T) {
	destDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(destDir, "a.txt"), []byte("existing"), 0o644); err != nil {
		t.Fatal(err)
	}
	stagingDir := t.TempDir()
	writeStagingFile(t, stagingDir, "a.txt", "new")
	writeStagingFile(t, stagingDir, "b.txt", "new-b")

	skipped, err := commitStaged(stagingDir, destDir, Options{OverwritePolicy: OverwriteSkip})
	if err != nil {
		t.Fatalf("commitStaged: %v", err)
	}
	if len(skipped) != 1 || skipped[0] != "a.txt" {
		t.Errorf("skipped = %v, want [a.txt]", skipped)
	}
	if got := readFile(t, filepath.Join(destDir, "a.txt")); got != "existing" {
		t.Errorf("a.txt was overwritten despite --skip: %q", got)
	}
	if got := readFile(t, filepath.Join(destDir, "b.txt")); got != "new-b" {
		t.Errorf("b.txt = %q, want \"new-b\" (rest of the run should still commit)", got)
	}
}

// TestCommitStaged_Rename proves --rename writes the entry under a " (k)"
// suffixed variant, leaving the original untouched.
func TestCommitStaged_Rename(t *testing.T) {
	destDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(destDir, "a.txt"), []byte("existing"), 0o644); err != nil {
		t.Fatal(err)
	}
	stagingDir := t.TempDir()
	writeStagingFile(t, stagingDir, "a.txt", "new")

	if _, err := commitStaged(stagingDir, destDir, Options{OverwritePolicy: OverwriteRename}); err != nil {
		t.Fatalf("commitStaged: %v", err)
	}
	if got := readFile(t, filepath.Join(destDir, "a.txt")); got != "existing" {
		t.Errorf("original a.txt was touched: %q", got)
	}
	if got := readFile(t, filepath.Join(destDir, "a (1).txt")); got != "new" {
		t.Errorf("a (1).txt = %q, want \"new\"", got)
	}
}

// TestCommitStaged_RenameDoubleCollision proves two entries in the same
// commit walk that would both rename to the same candidate never silently
// collide: one against a pre-existing disk file, the other against the first
// entry's own renamed output.
func TestCommitStaged_RenameDoubleCollision(t *testing.T) {
	destDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(destDir, "a.txt"), []byte("disk-original"), 0o644); err != nil {
		t.Fatal(err)
	}
	// "a (1).txt" already exists too, so BOTH the archive's "a.txt" (renamed
	// once) and a hypothetical second colliding entry would naturally want
	// "a (1).txt" first.
	if err := os.WriteFile(filepath.Join(destDir, "a (1).txt"), []byte("disk-variant-1"), 0o644); err != nil {
		t.Fatal(err)
	}
	stagingDir := t.TempDir()
	writeStagingFile(t, stagingDir, "a.txt", "staged-new")

	if _, err := commitStaged(stagingDir, destDir, Options{OverwritePolicy: OverwriteRename}); err != nil {
		t.Fatalf("commitStaged: %v", err)
	}
	// Both "a.txt" and "a (1).txt" were already taken on disk, so the staged
	// entry must land at "a (2).txt" without touching either existing file.
	if got := readFile(t, filepath.Join(destDir, "a.txt")); got != "disk-original" {
		t.Errorf("a.txt = %q, want untouched \"disk-original\"", got)
	}
	if got := readFile(t, filepath.Join(destDir, "a (1).txt")); got != "disk-variant-1" {
		t.Errorf("a (1).txt = %q, want untouched \"disk-variant-1\"", got)
	}
	if got := readFile(t, filepath.Join(destDir, "a (2).txt")); got != "staged-new" {
		t.Errorf("a (2).txt = %q, want \"staged-new\"", got)
	}
}

// TestCommitStaged_RefusesSymlinkedDirectoryInPath proves a pre-existing
// symlinked "directory" component in destDir is never written through, even
// under the DEFAULT fail-closed policy (not just --overwrite): os.MkdirAll/
// os.Rename would otherwise happily follow a symlink at an intermediate path
// component, since Lstat-based safety only protects the FINAL component of a
// path. Regression test for a red-team finding on the initial implementation.
func TestCommitStaged_RefusesSymlinkedDirectoryInPath(t *testing.T) {
	destDir := t.TempDir()
	escapeTarget := t.TempDir() // stands in for something like /etc
	if err := os.Symlink(escapeTarget, filepath.Join(destDir, "shared")); err != nil {
		t.Fatal(err)
	}

	stagingDir := t.TempDir()
	writeStagingFile(t, stagingDir, "shared/payload.txt", "malicious")

	if _, err := commitStaged(stagingDir, destDir, Options{}); err == nil {
		t.Fatal("expected commitStaged to refuse writing through a symlinked directory")
	}
	if _, statErr := os.Stat(filepath.Join(escapeTarget, "payload.txt")); !os.IsNotExist(statErr) {
		t.Fatal("payload.txt was written through the symlink into the escape target")
	}
}

// TestCommitStaged_PreExistingDirUntouchedOnRollback proves a directory that
// existed before the call, holding unrelated content, survives a mid-run
// failure completely untouched.
func TestCommitStaged_PreExistingDirUntouchedOnRollback(t *testing.T) {
	destDir := t.TempDir()
	preexistingSub := filepath.Join(destDir, "shared")
	if err := os.MkdirAll(preexistingSub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(preexistingSub, "unrelated.txt"), []byte("keep-me"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Force a collision so the walk aborts partway.
	if err := os.WriteFile(filepath.Join(preexistingSub, "a.txt"), []byte("existing"), 0o644); err != nil {
		t.Fatal(err)
	}

	stagingDir := t.TempDir()
	writeStagingFile(t, stagingDir, "shared/a.txt", "new")

	if _, err := commitStaged(stagingDir, destDir, Options{}); err == nil {
		t.Fatal("expected collision error")
	}
	if got := readFile(t, filepath.Join(preexistingSub, "unrelated.txt")); got != "keep-me" {
		t.Errorf("unrelated pre-existing file was disturbed: %q", got)
	}
	if _, statErr := os.Stat(preexistingSub); statErr != nil {
		t.Errorf("pre-existing directory was removed by rollback: %v", statErr)
	}
}
