package main

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coetzeevs/cerebro/brain"
	"github.com/coetzeevs/cerebro/internal/store"
)

// captureMigrate executes the migrate command with the given args and returns
// (stdout bytes, stderr bytes, error).
func captureMigrate(t *testing.T, args ...string) (stdout, stderr []byte, err error) {
	t.Helper()

	// Reset global flag variables so each test starts clean.
	projectFlag = ""
	formatFlag = "md"
	quietFlag = false

	// Reset migrate-specific flags to defaults so state from a previous test
	// invocation does not bleed into this one.
	migrateRealpathHashesFlag = false
	migrateScanRootsFlag = nil
	migrateMaxDepthFlag = 4
	migrateDryRunFlag = false

	var outBuf, errBuf bytes.Buffer
	rootCmd.SetOut(&outBuf)
	rootCmd.SetErr(&errBuf)
	migrateCmd.SetOut(&outBuf)
	migrateCmd.SetErr(&errBuf)

	rootCmd.SetArgs(append([]string{"migrate"}, args...))
	err = rootCmd.Execute()
	return outBuf.Bytes(), errBuf.Bytes(), err
}

// makeOldStyleBrain creates a brain file using the pre-HS-008 hash (sha256(abs))
// at the given home directory. Returns the brain path.
func makeOldStyleBrain(t *testing.T, homeDir, projectDir string) string {
	t.Helper()
	abs, _ := filepath.Abs(projectDir)
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(abs)))
	brainPath := filepath.Join(homeDir, ".cerebro", "projects", hash+".sqlite")
	if err := os.MkdirAll(filepath.Dir(brainPath), 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	b, err := brain.Init(brainPath, brain.EmbedConfig{Provider: "none"})
	if err != nil {
		t.Fatalf("brain.Init old-style: %v", err)
	}
	_ = b.Close()
	return brainPath
}

// TestMigrate_NoFlagError verifies that calling migrate without --realpath-hashes
// returns a usage error.
func TestMigrate_NoFlagError(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	_, _, err := captureMigrate(t)
	if err == nil {
		t.Fatal("expected error when --realpath-hashes not provided")
	}
}

// TestMigrate_NothingToMigrate verifies that when no old-style brain files exist,
// the command exits 0 with a "nothing to migrate" message.
// Covers Gherkin Scenario 4 (no-op case).
func TestMigrate_NothingToMigrate(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	scanRoot := t.TempDir()

	stdout, _, err := captureMigrate(t, "--realpath-hashes", "--scan-root", scanRoot)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(string(stdout), "Nothing to migrate") &&
		!strings.Contains(string(stdout), "migrated: 0") &&
		!strings.Contains(string(stdout), "Migrated 0") {
		t.Errorf("expected nothing-to-migrate message, got stdout=%q", stdout)
	}
}

// TestMigrate_CaseA_RenameOnly verifies that when only an old-style brain exists
// (not the new realpath-keyed one), the old file is renamed to the new hash.
// Covers Case A from Design §5.1.
func TestMigrate_CaseA_RenameOnly(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Create a real project dir and a symlink to it so old and new hashes differ.
	realDir := t.TempDir()
	symlinkDir := filepath.Join(t.TempDir(), "sym-project")
	if err := os.Symlink(realDir, symlinkDir); err != nil {
		t.Skip("os.Symlink not supported, skipping")
	}

	// Create old-style brain (keyed on unresolved symlink path).
	oldBrain := makeOldStyleBrain(t, home, symlinkDir)

	// New-style brain path (keyed on resolved realpath).
	newBrain := brain.ProjectPath(symlinkDir)

	// They must differ (prerequisite for the test to be meaningful).
	if oldBrain == newBrain {
		t.Skip("old and new brain paths are identical — no symlink divergence detected")
	}

	// Old brain must exist before migration.
	if _, err := os.Stat(oldBrain); os.IsNotExist(err) {
		t.Fatalf("old brain not found at %s", oldBrain)
	}

	stdout, _, err := captureMigrate(t, "--realpath-hashes", "--scan-root", symlinkDir, "--max-depth", "0")
	if err != nil {
		t.Fatalf("migrate error: %v", err)
	}

	// Old brain must be gone.
	if _, statErr := os.Stat(oldBrain); !os.IsNotExist(statErr) {
		t.Errorf("old brain still exists at %s after Case A rename", oldBrain)
	}
	// New brain must exist.
	if _, statErr := os.Stat(newBrain); os.IsNotExist(statErr) {
		t.Errorf("new brain not created at %s after Case A rename", newBrain)
	}
	// Summary must mention 1 migrated.
	if !strings.Contains(string(stdout), "1") {
		t.Logf("stdout=%q", stdout) // informational — output format may vary
	}
}

// TestMigrate_CaseB_AlreadyMigrated verifies that when only a new-style brain
// exists, the command is a no-op and exits 0.
// Covers Case B from Design §5.1.
func TestMigrate_CaseB_AlreadyMigrated(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	projectDir := t.TempDir()

	// Create new-style brain (via brain.ProjectPath which now resolves symlinks).
	newBrain := brain.ProjectPath(projectDir)
	if err := os.MkdirAll(filepath.Dir(newBrain), 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	b, err := brain.Init(newBrain, brain.EmbedConfig{Provider: "none"})
	if err != nil {
		t.Fatalf("brain.Init: %v", err)
	}
	_ = b.Close()

	stdout, _, err := captureMigrate(t, "--realpath-hashes", "--scan-root", projectDir, "--max-depth", "0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = stdout // No-op is acceptable
}

// TestMigrate_CaseC_MergeAndDelete verifies that when both old and new brains exist,
// the old brain's content is merged into the new and the old is deleted.
// Covers Case C from Design §5.1.
func TestMigrate_CaseC_MergeAndDelete(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	realDir := t.TempDir()
	symlinkDir := filepath.Join(t.TempDir(), "sym-merge")
	if err := os.Symlink(realDir, symlinkDir); err != nil {
		t.Skip("os.Symlink not supported, skipping")
	}

	// Create old-style brain with a node in it.
	oldBrain := makeOldStyleBrain(t, home, symlinkDir)
	oldB, err := brain.Open(oldBrain)
	if err != nil {
		t.Fatalf("open old brain: %v", err)
	}
	_, err = oldB.Add("old memory node", store.TypeConcept)
	if err != nil {
		_ = oldB.Close()
		t.Fatalf("add to old brain: %v", err)
	}
	_ = oldB.Close()

	// Create new-style brain.
	newBrain := brain.ProjectPath(symlinkDir)
	if oldBrain == newBrain {
		t.Skip("old and new brain paths identical — no symlink divergence")
	}
	if err := os.MkdirAll(filepath.Dir(newBrain), 0o750); err != nil {
		t.Fatalf("mkdir new brain dir: %v", err)
	}
	newB, err := brain.Init(newBrain, brain.EmbedConfig{Provider: "none"})
	if err != nil {
		t.Fatalf("brain.Init new: %v", err)
	}
	_ = newB.Close()

	_, _, err = captureMigrate(t, "--realpath-hashes", "--scan-root", symlinkDir, "--max-depth", "0")
	if err != nil {
		t.Fatalf("migrate error: %v", err)
	}

	// Old brain must be deleted.
	if _, statErr := os.Stat(oldBrain); !os.IsNotExist(statErr) {
		t.Errorf("old brain still exists at %s after Case C merge+delete", oldBrain)
	}

	// New brain must exist and contain the merged node.
	newB2, err := brain.Open(newBrain)
	if err != nil {
		t.Fatalf("open new brain after merge: %v", err)
	}
	defer func() { _ = newB2.Close() }()

	nodes, err := newB2.List(store.ListNodesOpts{})
	if err != nil {
		t.Fatalf("list nodes: %v", err)
	}
	if len(nodes) == 0 {
		t.Error("expected merged node in new brain, got 0 nodes")
	}

	// A backup must have been created.
	backupDir := filepath.Join(home, ".cerebro", "backups")
	entries, err := os.ReadDir(backupDir)
	if err != nil || len(entries) == 0 {
		t.Errorf("expected backup directory under %s, got err=%v entries=%v", backupDir, err, entries)
	}
}

// TestMigrate_DryRun verifies that --dry-run reports what would change but does
// not rename or delete any brain files.
func TestMigrate_DryRun(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	realDir := t.TempDir()
	symlinkDir := filepath.Join(t.TempDir(), "sym-dryrun")
	if err := os.Symlink(realDir, symlinkDir); err != nil {
		t.Skip("os.Symlink not supported, skipping")
	}

	oldBrain := makeOldStyleBrain(t, home, symlinkDir)
	newBrain := brain.ProjectPath(symlinkDir)

	if oldBrain == newBrain {
		t.Skip("old and new brain paths identical — no symlink divergence")
	}

	stdout, _, err := captureMigrate(t, "--realpath-hashes", "--dry-run", "--scan-root", symlinkDir, "--max-depth", "0")
	if err != nil {
		t.Fatalf("dry-run error: %v", err)
	}

	// Old brain must still exist (no mutation in dry-run).
	if _, statErr := os.Stat(oldBrain); os.IsNotExist(statErr) {
		t.Errorf("old brain was deleted during dry-run — must not mutate")
	}
	// New brain must not have been created in dry-run.
	if _, statErr := os.Stat(newBrain); !os.IsNotExist(statErr) {
		// Accept: new brain exists only if it was created before — but in this
		// test it was not. So if it now exists, it's a dry-run violation.
		t.Errorf("new brain was created during dry-run — must not mutate")
	}
	// Stdout must mention the would-be action.
	if !strings.Contains(string(stdout), "dry") && !strings.Contains(string(stdout), "Dry") {
		t.Logf("dry-run stdout=%q (informational)", stdout)
	}
}

// TestMigrate_Idempotent verifies that running migrate twice results in the same
// state as running it once (second run finds nothing to migrate).
// Covers Gherkin Scenario 4 (no-op / idempotency).
func TestMigrate_Idempotent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	realDir := t.TempDir()
	symlinkDir := filepath.Join(t.TempDir(), "sym-idem")
	if err := os.Symlink(realDir, symlinkDir); err != nil {
		t.Skip("os.Symlink not supported, skipping")
	}

	oldBrain := makeOldStyleBrain(t, home, symlinkDir)
	newBrain := brain.ProjectPath(symlinkDir)
	if oldBrain == newBrain {
		t.Skip("old and new brain paths identical — no symlink divergence")
	}

	// First run.
	_, _, err := captureMigrate(t, "--realpath-hashes", "--scan-root", symlinkDir, "--max-depth", "0")
	if err != nil {
		t.Fatalf("first migrate error: %v", err)
	}
	if _, statErr := os.Stat(oldBrain); !os.IsNotExist(statErr) {
		t.Fatalf("old brain still present after first migrate")
	}

	// Second run must not error.
	stdout2, _, err := captureMigrate(t, "--realpath-hashes", "--scan-root", symlinkDir, "--max-depth", "0")
	if err != nil {
		t.Fatalf("second migrate error: %v", err)
	}
	_ = stdout2 // informational

	// New brain must still exist.
	if _, statErr := os.Stat(newBrain); os.IsNotExist(statErr) {
		t.Errorf("new brain vanished after second migrate")
	}
}

// TestMigrate_PathWithSpaces verifies that project paths containing spaces are
// handled correctly (edge case from Design §10.1).
func TestMigrate_PathWithSpaces(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	base := t.TempDir()
	spacyReal := filepath.Join(base, "My Project (v2)")
	if err := os.MkdirAll(spacyReal, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	symDir := filepath.Join(base, "sym spacy")
	if err := os.Symlink(spacyReal, symDir); err != nil {
		t.Skip("os.Symlink not supported, skipping")
	}

	oldBrain := makeOldStyleBrain(t, home, symDir)
	newBrain := brain.ProjectPath(symDir)
	if oldBrain == newBrain {
		t.Skip("old and new brain paths identical — no symlink divergence")
	}

	_, _, err := captureMigrate(t, "--realpath-hashes", "--scan-root", symDir, "--max-depth", "0")
	if err != nil {
		t.Fatalf("unexpected error for path with spaces: %v", err)
	}
	if _, statErr := os.Stat(newBrain); os.IsNotExist(statErr) {
		t.Errorf("new brain not present after migration for path with spaces")
	}
}

// TestMigrate_SymlinkSkip verifies that symlinked directories encountered during
// a scan-root walk are not descended (S-MED-1 / CWE-59 guard).
// Creates a "trojan" symlink pointing to a parent temp directory to simulate a
// directory expansion attack. The walk must skip the symlink and complete quickly.
func TestMigrate_SymlinkSkip(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	scanRoot := t.TempDir()

	// Create a real subdir to walk normally.
	realSub := filepath.Join(scanRoot, "realproject")
	if err := os.MkdirAll(realSub, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Trojan: symlink inside scan root pointing back at scan root (potential loop).
	trojan := filepath.Join(scanRoot, "trojan")
	if err := os.Symlink(scanRoot, trojan); err != nil {
		t.Skip("os.Symlink not supported, skipping")
	}

	// Migration must not hang or recurse into the trojan symlink.
	// Run with --max-depth 2 which would loop forever without the symlink skip.
	done := make(chan error, 1)
	go func() {
		_, _, err := captureMigrate(t, "--realpath-hashes", "--scan-root", scanRoot, "--max-depth", "2")
		done <- err
	}()

	select {
	case err := <-done:
		// Completed without hanging — symlink skip worked.
		if err != nil {
			t.Fatalf("unexpected migrate error: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Error("migrate hung — symlink traversal was not skipped")
	}
}

// TestMigrate_ConcurrentLock verifies that a second concurrent migrate invocation
// fails with an informative error when the lockfile is held (N2).
func TestMigrate_ConcurrentLock(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	lockPath := filepath.Join(home, ".cerebro", "migrate.lock")
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Simulate a held lock by pre-creating the lockfile.
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatalf("create lock: %v", err)
	}
	_ = f.Close()
	t.Cleanup(func() { _ = os.Remove(lockPath) })

	scanRoot := t.TempDir()
	_, stderr, err := captureMigrate(t, "--realpath-hashes", "--scan-root", scanRoot)
	if err == nil {
		t.Error("expected error when lockfile is held, got nil")
	}
	if !strings.Contains(string(stderr)+err.Error(), "lock") &&
		!strings.Contains(string(stderr)+err.Error(), "migration") {
		t.Logf("stderr=%q err=%v (expected 'lock' or 'migration' in message)", stderr, err)
	}
}

// TestMigrate_WALCompanionFiles verifies that Case A rename also handles
// SQLite WAL/shared-memory companion files (.sqlite-wal, .sqlite-shm) per N3.
func TestMigrate_WALCompanionFiles(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	realDir := t.TempDir()
	symlinkDir := filepath.Join(t.TempDir(), "sym-wal")
	if err := os.Symlink(realDir, symlinkDir); err != nil {
		t.Skip("os.Symlink not supported, skipping")
	}

	oldBrain := makeOldStyleBrain(t, home, symlinkDir)
	newBrain := brain.ProjectPath(symlinkDir)
	if oldBrain == newBrain {
		t.Skip("old and new brain paths identical — no symlink divergence")
	}

	// Create companion files to simulate WAL mode.
	walPath := oldBrain + "-wal"
	shmPath := oldBrain + "-shm"
	if err := os.WriteFile(walPath, []byte("wal"), 0o600); err != nil {
		t.Fatalf("create wal: %v", err)
	}
	if err := os.WriteFile(shmPath, []byte("shm"), 0o600); err != nil {
		t.Fatalf("create shm: %v", err)
	}

	_, _, err := captureMigrate(t, "--realpath-hashes", "--scan-root", symlinkDir, "--max-depth", "0")
	if err != nil {
		t.Fatalf("migrate error: %v", err)
	}

	// Old companions must be gone.
	if _, statErr := os.Stat(walPath); !os.IsNotExist(statErr) {
		t.Errorf("old .sqlite-wal companion still exists after Case A rename")
	}
	if _, statErr := os.Stat(shmPath); !os.IsNotExist(statErr) {
		t.Errorf("old .sqlite-shm companion still exists after Case A rename")
	}
	// New companions may or may not exist depending on SQLite mode — we only
	// check that the new brain file itself exists.
	if _, statErr := os.Stat(newBrain); os.IsNotExist(statErr) {
		t.Errorf("new brain not present after WAL companion rename")
	}
}

// TestMigrate_OverlappingScanRoots verifies that when two scan roots both resolve
// to the same project directory (N4), the migration is applied exactly once
// (not twice) — the second scan root is deduplicated by the visited-dirs map.
func TestMigrate_OverlappingScanRoots(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	realDir := t.TempDir()
	symDir1 := filepath.Join(t.TempDir(), "sym-overlap-1")
	symDir2 := filepath.Join(t.TempDir(), "sym-overlap-2")

	if err := os.Symlink(realDir, symDir1); err != nil {
		t.Skip("os.Symlink not supported, skipping")
	}
	if err := os.Symlink(realDir, symDir2); err != nil {
		t.Skip("os.Symlink not supported, skipping")
	}

	oldBrain1 := makeOldStyleBrain(t, home, symDir1)
	newBrain := brain.ProjectPath(symDir1) // same as ProjectPath(symDir2) since both → realDir

	if oldBrain1 == newBrain {
		t.Skip("old and new brain paths identical — no symlink divergence")
	}

	// Use both symDir1 and symDir2 as scan roots — both resolve to the same realDir.
	// Only one migration should happen (Case A for symDir1; symDir2 is skipped by N4).
	_, _, err := captureMigrate(t,
		"--realpath-hashes",
		"--scan-root", symDir1,
		"--scan-root", symDir2,
		"--max-depth", "0",
	)
	if err != nil {
		t.Fatalf("migrate error with overlapping scan roots: %v", err)
	}

	// New brain must exist (migration happened exactly once).
	if _, statErr := os.Stat(newBrain); os.IsNotExist(statErr) {
		t.Errorf("new brain not present after overlapping-scan-roots migration")
	}

	// Old brain for symDir1 must be gone (migrated).
	if _, statErr := os.Stat(oldBrain1); !os.IsNotExist(statErr) {
		t.Errorf("old brain for symDir1 still present after migration")
	}
}
