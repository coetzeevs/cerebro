package main

// Package-level doc: cmd_migrate implements the `cerebro migrate` subcommand.
//
// WHY this command exists:
// Before HS-008, brain.ProjectPath hashed the raw absolute path without resolving
// symlinks. On macOS, /tmp is a symlink to /private/tmp, causing duplicate brains
// for the same project when accessed via different path aliases. HS-008 fixes
// ProjectPath to resolve symlinks before hashing; this command migrates existing
// brains created under the old (unresolved) hash to the new (realpath) hash.
//
// Algorithm (Design §5):
// Walk --scan-root directories (default $HOME) to depth --max-depth (default 4).
// For each directory, compute oldHash = sha256(abs) and newHash = sha256(realpath).
// If oldHash == newHash: no action (Case B or D). Otherwise:
//   Case A: only old brain exists → atomic rename to new hash (+ WAL companions)
//   Case B: only new brain exists → no-op (already migrated)
//   Case C: both exist → backup both, merge old into new (ConflictSkip), delete old
//   Case D: neither exists → no-op
//
// Safety:
//   - Acquires ~/.cerebro/migrate.lock (O_EXCL) before any mutation (N2, S-MED-3)
//   - Skips symlinked directories in the walk (S-MED-1, CWE-59)
//   - Tracks visited dirs by resolved path to avoid double-processing (N4)
//   - Backup is mandatory for Case C; no --no-backup flag
//   - --dry-run previews all actions without mutating
//
// Output discipline (HS-007 precedent, Ontology rule 28):
//   stdout ← summary only
//   stderr ← per-directory progress (suppressed with --quiet/-q)

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/coetzeevs/cerebro/brain"
	"github.com/coetzeevs/cerebro/internal/store"
	"github.com/spf13/cobra"
)

var (
	migrateRealpathHashesFlag bool
	migrateScanRootsFlag      []string
	migrateMaxDepthFlag       int
	migrateDryRunFlag         bool
)

// migrateCmd is the exported cobra command for testability.
var migrateCmd *cobra.Command

func init() {
	migrateCmd = &cobra.Command{
		Use:   "migrate",
		Short: "Migrate brain data between formats or versions",
		Long: `Migrate brain data between formats or storage conventions.

Currently supported migration kinds:

  --realpath-hashes   Consolidate legacy duplicate brains created before HS-008.
                      Scans project directories, resolves symlinks, and renames or
                      merges brains keyed under old unresolved-path hashes into
                      the correct realpath-keyed hash. Safe to run multiple times
                      (idempotent). Use --dry-run to preview changes.`,
		RunE: runMigrate,
	}
	migrateCmd.Flags().BoolVar(&migrateRealpathHashesFlag, "realpath-hashes", false,
		"Migrate brains from unresolved-path hashes to realpath hashes (HS-008)")
	migrateCmd.Flags().StringArrayVar(&migrateScanRootsFlag, "scan-root", nil,
		"Root directory to scan (repeatable; default $HOME)")
	migrateCmd.Flags().IntVar(&migrateMaxDepthFlag, "max-depth", 4,
		"Maximum directory depth from each scan root (default 4)")
	migrateCmd.Flags().BoolVar(&migrateDryRunFlag, "dry-run", false,
		"Preview changes without mutating brain files")
	rootCmd.AddCommand(migrateCmd)
}

// runMigrate is the cobra RunE handler.
func runMigrate(cmd *cobra.Command, _ []string) error {
	if !migrateRealpathHashesFlag {
		return fmt.Errorf("at least one migration kind flag is required (e.g. --realpath-hashes); see --help")
	}

	// Determine scan roots.
	scanRoots := migrateScanRootsFlag
	if len(scanRoots) == 0 {
		home, err := os.UserHomeDir()
		if err != nil {
			dir, wdErr := os.Getwd()
			if wdErr != nil {
				return fmt.Errorf("cannot determine home or working directory: %w", err)
			}
			fmt.Fprintf(os.Stderr, "WARNING: $HOME not set; scanning cwd %s\n", dir)
			home = dir
		}
		scanRoots = []string{home}
	}

	opts := migrateOpts{
		ScanRoots: scanRoots,
		MaxDepth:  migrateMaxDepthFlag,
		DryRun:    migrateDryRunFlag,
		Stderr:    cmd.ErrOrStderr(),
	}

	result, err := migrateRealpathHashes(opts)
	if err != nil {
		return err
	}

	printMigrateResult(cmd.OutOrStdout(), result)
	return nil
}

// migrateOpts configures the migrateRealpathHashes function for testability.
type migrateOpts struct {
	ScanRoots []string // scan root directories
	MaxDepth  int      // max depth from each scan root
	DryRun    bool     // preview only
	Stderr    io.Writer
}

// migrationResult summarises the outcome of a migrateRealpathHashes run.
type migrationResult struct {
	Migrated int            `json:"migrated"` // total brains acted on
	Renamed  []renamedBrain `json:"renamed"`  // Case A entries
	Merged   []mergedBrain  `json:"merged"`   // Case C entries
	Scanned  int            `json:"scanned"`  // directories visited
	DryRun   bool           `json:"dryRun"`
}

// renamedBrain records a Case A rename.
type renamedBrain struct {
	BrainPath  string `json:"brainPath"`  // new hash path
	ProjectDir string `json:"projectDir"` // resolved real path
	OldHash    string `json:"oldHash"`
	NewHash    string `json:"newHash"`
}

// mergedBrain records a Case C merge.
type mergedBrain struct {
	DestBrainPath string `json:"destBrainPath"` // new hash path
	SrcBrainPath  string `json:"srcBrainPath"`  // old hash path (deleted post-merge)
	ProjectDir    string `json:"projectDir"`
	NodesImported int    `json:"nodesImported"`
	NodesSkipped  int    `json:"nodesSkipped"`
	BackupDir     string `json:"backupDir"`
}

// cerebroProjectsDir returns the ~/.cerebro/projects directory.
func cerebroProjectsDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".cerebro", "projects")
}

// migrateRealpathHashes performs the path-driven scan and reconciliation.
// It is exported (via package-level function) for direct testability.
func migrateRealpathHashes(opts migrateOpts) (*migrationResult, error) {
	// --- N2: acquire process-level lockfile ---
	lockPath := filepath.Join(func() string { h, _ := os.UserHomeDir(); return h }(), ".cerebro", "migrate.lock")
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o750); err != nil {
		return nil, fmt.Errorf("creating .cerebro directory: %w", err)
	}
	lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600) //nolint:gosec // internal lock path
	if err != nil {
		return nil, fmt.Errorf("another migration is in progress (lock at %s); remove if stale", lockPath)
	}
	defer func() {
		_ = lockFile.Close()
		_ = os.Remove(lockPath)
	}()

	result := &migrationResult{DryRun: opts.DryRun}
	projectsDir := cerebroProjectsDir()

	// --- N4: tracked-dirs keyed on resolved path ---
	visited := make(map[string]struct{})

	stderr := opts.Stderr
	if stderr == nil {
		stderr = os.Stderr
	}

	for _, root := range opts.ScanRoots {
		// Walk from the unresolved absolute root so that we encounter any
		// symlinked project directories that may have produced old-style hashes.
		// The S-MED-1 symlink-skip applies to entries *within* the walk
		// (subdirectories that are symlinks), not to the root itself.
		absRoot, _ := filepath.Abs(root)

		if err := walkDirUnresolved(absRoot, opts.MaxDepth, func(dir string) error {
			// dir is the directory path as encountered during the walk (may be
			// under a realpath-resolved root, but subdirs are real entries).
			abs, _ := filepath.Abs(dir)
			resolved, evalErr := filepath.EvalSymlinks(abs)
			if evalErr != nil {
				// Broken symlink or unreadable — skip silently.
				return nil
			}

			// N4: skip if we've already processed this resolved path.
			if _, seen := visited[resolved]; seen {
				return nil
			}
			visited[resolved] = struct{}{}

			if abs == resolved {
				// No symlink divergence for this dir — hashes are identical, no migration.
				return nil
			}

			result.Scanned++

			oldHash := hashPath(abs)
			newHash := hashPath(resolved)

			oldBrain := filepath.Join(projectsDir, oldHash+".sqlite")
			newBrain := filepath.Join(projectsDir, newHash+".sqlite")

			oldExists := fileExists(oldBrain)
			newExists := fileExists(newBrain)

			switch {
			case oldExists && !newExists:
				// Case A: only old brain exists → rename.
				if !quietFlag {
					_, _ = fmt.Fprintf(stderr, "  [rename] %s → %s (project: %s)\n", oldHash[:12], newHash[:12], resolved)
				}
				if !opts.DryRun {
					if err := renameWithCompanions(oldBrain, newBrain); err != nil {
						_, _ = fmt.Fprintf(stderr, "  [ERROR] rename failed for %s: %v\n", resolved, err)
						return nil
					}
				}
				result.Renamed = append(result.Renamed, renamedBrain{
					BrainPath:  newBrain,
					ProjectDir: resolved,
					OldHash:    oldHash,
					NewHash:    newHash,
				})
				result.Migrated++

			case oldExists && newExists:
				// Case C: both exist → backup, merge old into new, delete old.
				if !quietFlag {
					_, _ = fmt.Fprintf(stderr, "  [merge]  %s → %s (project: %s)\n", oldHash[:12], newHash[:12], resolved)
				}
				if !opts.DryRun {
					merged, mergeErr := mergeBrains(oldBrain, newBrain)
					if mergeErr != nil {
						_, _ = fmt.Fprintf(stderr, "  [ERROR] merge failed for %s: %v\n", resolved, mergeErr)
						return nil
					}
					merged.ProjectDir = resolved
					result.Merged = append(result.Merged, *merged)
				} else {
					result.Merged = append(result.Merged, mergedBrain{
						DestBrainPath: newBrain,
						SrcBrainPath:  oldBrain,
						ProjectDir:    resolved,
					})
				}
				result.Migrated++

			case !oldExists && newExists:
				// Case B: already migrated — no-op.

			case !oldExists && !newExists:
				// Case D: no brain for this dir — no-op.
			}

			return nil
		}); err != nil {
			_, _ = fmt.Fprintf(stderr, "WARNING: walk error under %s: %v\n", absRoot, err)
		}
	}

	return result, nil
}

// hashPath returns the hex SHA-256 of a path string (same algorithm as brain.ProjectPath).
func hashPath(p string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(p)))
}

// fileExists reports whether path exists and is a regular file (or is accessible).
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// walkDirUnresolved walks root and its real subdirectories up to maxDepth,
// calling fn for each directory path encountered.
//
// The root itself is always processed first (with its original unresolved path)
// even if it is a symlink — this is the key use case for migration: a scan root
// like "/tmp/myproject" (a symlink) must be processed so that its old-style hash
// (sha256("/tmp/myproject")) can be found and migrated.
//
// Subdirectory entries that are symlinks (detected via lstat) are skipped with
// filepath.SkipDir (S-MED-1 / CWE-59 guard). This prevents trojan-symlink
// directory traversal attacks while still allowing the root itself to be checked.
func walkDirUnresolved(root string, maxDepth int, fn func(dir string) error) error {
	// Always process the root itself (depth 0), even if it is a symlink.
	// This is the critical case for migration.
	if err := fn(root); err != nil {
		return err
	}

	if maxDepth == 0 {
		// No subdirectory scanning requested.
		return nil
	}

	// For the subdirectory walk, resolve the root to a real directory so that
	// WalkDir can descend into it. We walk from the resolved root but track
	// paths relative to the original root for depth computation.
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		// Root is a broken symlink or not accessible; nothing to descend.
		return nil //nolint:nilerr // broken root is not an error for the caller
	}

	baseDepth := countPathDepth(resolvedRoot)

	return filepath.WalkDir(resolvedRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			// Permission error or broken path; skip silently.
			return nil
		}

		if path == resolvedRoot {
			// Already processed as the root above.
			return nil
		}

		if !d.IsDir() {
			return nil
		}

		// S-MED-1: skip symlinked directories found *within* the resolved root.
		if d.Type()&os.ModeSymlink != 0 {
			return filepath.SkipDir
		}

		// Depth check relative to the resolved root.
		depth := countPathDepth(path) - baseDepth
		if depth > maxDepth {
			return filepath.SkipDir
		}

		return fn(path)
	})
}

// countPathDepth returns the number of path components in an absolute path.
// Used to compute relative depth between root and a walked path.
func countPathDepth(p string) int {
	return len(strings.Split(filepath.Clean(p), string(filepath.Separator)))
}

// renameWithCompanions atomically renames oldPath to newPath, and also renames
// any SQLite WAL/shared-memory companion files (N3: .sqlite-wal, .sqlite-shm).
func renameWithCompanions(oldPath, newPath string) error {
	if err := os.Rename(oldPath, newPath); err != nil {
		return fmt.Errorf("rename brain: %w", err)
	}
	// Companion files — best-effort: ignore absence, surface other errors.
	for _, suffix := range []string{"-wal", "-shm"} {
		oldComp := oldPath + suffix
		if _, statErr := os.Stat(oldComp); os.IsNotExist(statErr) {
			continue
		}
		newComp := newPath + suffix
		if renameErr := os.Rename(oldComp, newComp); renameErr != nil {
			// Non-fatal: the brain file itself was renamed; companion rename is best-effort.
			fmt.Fprintf(os.Stderr, "  WARNING: companion rename failed (%s): %v\n", suffix, renameErr)
		}
	}
	return nil
}

// mergeBrains backs up both src and dst, exports src as JSON, imports into dst
// with ConflictSkip, then deletes src. Returns a mergedBrain summary.
func mergeBrains(srcPath, dstPath string) (*mergedBrain, error) {
	// Step 1: backup both brains before any mutation.
	ts := time.Now().UTC().Format("2006-01-02T15-04-05")
	backupBase := filepath.Join(func() string { h, _ := os.UserHomeDir(); return h }(),
		".cerebro", "backups", "migrate-"+ts)

	if _, err := backupBrainTo(srcPath, filepath.Join(backupBase, filepath.Base(srcPath))); err != nil {
		return nil, fmt.Errorf("backup source: %w", err)
	}
	if _, err := backupBrainTo(dstPath, filepath.Join(backupBase, filepath.Base(dstPath))); err != nil {
		return nil, fmt.Errorf("backup destination: %w", err)
	}

	// Step 2: export source brain as JSON in-memory.
	srcBrain, err := brain.Open(srcPath)
	if err != nil {
		return nil, fmt.Errorf("open source brain: %w", err)
	}
	var jsonBuf bytes.Buffer
	if err := srcBrain.ExportJSON(&jsonBuf); err != nil {
		_ = srcBrain.Close()
		return nil, fmt.Errorf("export source brain: %w", err)
	}
	_ = srcBrain.Close()

	// Step 3: open destination and import with ConflictSkip.
	dstBrain, err := brain.Open(dstPath)
	if err != nil {
		return nil, fmt.Errorf("open destination brain: %w", err)
	}
	importResult, err := dstBrain.ImportFromJSON(&jsonBuf, store.ImportOptions{OnConflict: store.ConflictSkip})
	_ = dstBrain.Close()
	if err != nil {
		return nil, fmt.Errorf("import into destination: %w (backup at %s)", err, backupBase)
	}

	// Step 4: delete source only if import succeeded.
	if err := os.Remove(srcPath); err != nil {
		return nil, fmt.Errorf("delete source brain after merge: %w (backup at %s)", err, backupBase)
	}

	return &mergedBrain{
		DestBrainPath: dstPath,
		SrcBrainPath:  srcPath,
		NodesImported: importResult.NodesImported,
		NodesSkipped:  importResult.NodesSkipped,
		BackupDir:     backupBase,
	}, nil
}

// printMigrateResult writes the human-readable migration summary to w.
func printMigrateResult(w io.Writer, r *migrationResult) {
	if r.Migrated == 0 {
		_, _ = fmt.Fprintf(w, "Nothing to migrate. Scanned %d director(ies). All brains are already keyed by realpath hashes.\n", r.Scanned)
		return
	}

	dryLabel := ""
	if r.DryRun {
		dryLabel = " (dry run)"
	}
	_, _ = fmt.Fprintf(w, "Migrated %d brain(s)%s. Renamed %d. Merged %d.\n",
		r.Migrated, dryLabel, len(r.Renamed), len(r.Merged))

	for _, rn := range r.Renamed {
		_, _ = fmt.Fprintf(w, "  Renamed:  %s ← project %s\n", filepath.Base(rn.BrainPath), rn.ProjectDir)
	}
	for _, mg := range r.Merged {
		_, _ = fmt.Fprintf(w, "  Merged:   %s ← %s (%d nodes folded, %d conflicts skipped) backup=%s\n",
			filepath.Base(mg.DestBrainPath), filepath.Base(mg.SrcBrainPath),
			mg.NodesImported, mg.NodesSkipped, mg.BackupDir)
	}
	_, _ = fmt.Fprintf(w, "Scanned %d director(ies).\n", r.Scanned)
}
