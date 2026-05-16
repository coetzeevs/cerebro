package main

// Package-level doc: cmd_pi_init implements the `cerebro pi-init` subcommand.
//
// WHY this command exists:
// Stage 2 of the Hello Stack sequencing plan introduces Cerebro as a Pi
// extension. Rather than hand-crafting the pi.config.json snippet, operators
// run `cerebro pi-init -p <project-dir>` to get a deterministic, idempotent
// JSON fragment they can pipe or merge into their repo-root pi.config.json
// (assembled by HS-017). The command resolves the project path through
// filepath.EvalSymlinks (Ontology §5.14, rule 26) so that the derived brain
// path matches any other cerebro command invoked against the same realpath.
//
// Idempotency mechanism:
// The output is a pure function of (realpathDir, embedProvider="ollama"). No
// timestamps, no UUIDs, no host-specific data beyond the resolved path. The
// brain-creation side effect is idempotent: if the brain already exists,
// brain.Open+Close is called instead of brain.Init. JSON is marshalled via
// encoding/json (struct field order + MarshalIndent), producing stable bytes.
//
// Output channel discipline (Security A09, Design §2):
// stdout ← pure JSON snippet only (no status messages ever).
// stderr ← one status line: "Created brain at <path>" or "Verified existing brain at <path>".

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/coetzeevs/cerebro/brain"
	"github.com/spf13/cobra"
)

// piInitSnippet is the top-level JSON document emitted to stdout.
// The extensions array is pre-wrapped so HS-017 can merge it with a single
// jq '.extensions += $fragment.extensions' without additional reshaping.
type piInitSnippet struct {
	Extensions []piInitExtension `json:"extensions"`
}

// piInitExtension represents one entry in the pi.config.json extensions array.
type piInitExtension struct {
	Name    string        `json:"name"`
	Package string        `json:"package"`
	Options piInitOptions `json:"options"`
}

// piInitOptions carries the per-extension configuration consumed by pi-cerebro.
type piInitOptions struct {
	ProjectDir string     `json:"projectDir"`
	BrainPath  string     `json:"brainPath"`
	Boot       piInitBoot `json:"boot"`
}

// piInitBoot configures the session-start recall priming behaviour.
// Limit matches Sequencing Plan §2: "cerebro recall --boot --limit 1".
// If HS-009 makes this configurable, add --boot-limit flag at that time.
type piInitBoot struct {
	Limit int `json:"limit"`
}

var piInitCmd = &cobra.Command{
	Use:   "pi-init",
	Short: "Emit pi.config.json snippet for pi-cerebro",
	Long: `Emit a deterministic pi.config.json extension snippet for the pi-cerebro
extension. The snippet is printed to stdout as pure JSON; status messages
are written to stderr only (Design §2, Security A09).

The project path is resolved via filepath.EvalSymlinks (Ontology §5.14,
rule 26) before hashing. The corresponding brain is verified or created:
  - absent → brain.Init (creates ~/.cerebro/projects/<sha256(realpath)>.sqlite)
  - present → brain.Open + Close (validates schema, no re-init)

Output is idempotent: a second run with the same project directory produces
byte-identical stdout.

Example:
  cerebro pi-init -p /Users/q/projects/myproject | jq .`,
	RunE: runPiInit,
}

func init() {
	rootCmd.AddCommand(piInitCmd)
}

// runPiInit is the cobra RunE handler for `cerebro pi-init`.
//
// Order of operations (Design §5 realpath discipline):
//  1. Resolve the project directory via the standard helper chain.
//  2. os.Stat to confirm existence and directory-ness.
//  3. filepath.EvalSymlinks to resolve all symlinks (including parent components).
//  4. filepath.Abs as a defensive belt-and-braces against relative EvalSymlinks output.
//  5. brain.ProjectPath to derive the SQLite path.
//  6. Verify-or-create the brain per N4 (Stat → Init if absent, Open+Close if present).
//  7. Marshal and emit the JSON snippet to stdout.
//
// TOCTOU note (Security S-MED-1): the gap between os.Stat (step 2) and
// EvalSymlinks (step 3) is an accepted TOCTOU on a single-operator
// workstation where only the operator can modify their own paths. This is not
// exploitable in the current single-principal deployment model (memory 697508d5).
func runPiInit(cmd *cobra.Command, _ []string) error {
	// Step 1: resolve the project directory via the standard helper.
	dir := resolveProjectDir()

	// Step 2: confirm the path exists and is a directory.
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		if err == nil {
			err = fmt.Errorf("path is not a directory")
		}
		return fmt.Errorf("project directory %q not found or not a directory: %w", dir, err)
	}

	// Step 3: resolve all symlinks (Ontology §5.14 rule 26).
	// EvalSymlinks follows every component including parents — this is the
	// TOCTOU-accepted design (see runPiInit doc comment, Security S-MED-1).
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return fmt.Errorf("resolving symlinks for %q: %w", dir, err)
	}

	// Step 4: defensive Abs — EvalSymlinks is documented to return an absolute
	// path on absolute input, but is implementation-defined on relative input.
	absResolved, err := filepath.Abs(resolved)
	if err != nil {
		return fmt.Errorf("making path absolute for %q: %w", resolved, err)
	}

	// Step 5: derive the brain path from the realpath.
	// brain.ProjectPath calls filepath.Abs internally, but since absResolved is
	// already absolute and resolved, the sha256 pre-image is sha256(absResolved),
	// matching the Ontology §5.14 realpath-hash contract.
	brainPath := brain.ProjectPath(absResolved)

	// Step 6: verify or create the brain (N4 integrity-preservation invariant).
	// - absent → brain.Init with ollama default (same as cmd_init.go:63-67).
	// - present → brain.Open + Close: validates schema without re-running Init
	//   (brain.go:49-79 — Init re-writes meta keys; must not be called on existing brain).
	if _, statErr := os.Stat(brainPath); os.IsNotExist(statErr) {
		// First run: create the brain.
		cfg := brain.EmbedConfig{
			Provider: "ollama",
			// Model and Dimensions left at zero-values: ollama.New fills in
			// "nomic-embed-text" and 768 respectively — same defaults as cmd_init.go.
		}
		b, initErr := brain.Init(brainPath, cfg)
		if initErr != nil {
			return fmt.Errorf("creating brain at %q: %w", brainPath, initErr)
		}
		_ = b.Close()
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Created brain at %s\n", brainPath)
	} else if statErr != nil {
		// Unexpected Stat error (permissions, stale NFS handle, etc.).
		return fmt.Errorf("checking brain at %q: %w", brainPath, statErr)
	} else {
		// Subsequent run: open to validate schema, then close.
		b, openErr := brain.Open(brainPath)
		if openErr != nil {
			return fmt.Errorf("opening brain at %q: %w", brainPath, openErr)
		}
		_ = b.Close()
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Verified existing brain at %s\n", brainPath)
	}

	// Step 7: build and emit the deterministic JSON snippet to stdout.
	// Use json.NewEncoder with SetIndent so stdout is pure JSON with no trailing
	// newline issues — matches the outputJSON helper precedent (helpers.go:40-44).
	// N3: @coetzeevs/pi-cerebro is the package name assumed from the HS-009 design
	// at the time of HS-007 implementation. HS-009 is filed but the npm package name
	// has not been published yet. Update this literal if HS-009 lands a different name.
	snippet := piInitSnippet{
		Extensions: []piInitExtension{
			{
				Name: "pi-cerebro",
				// HS-009 will confirm this package name; update if HS-009 lands a different one.
				Package: "@coetzeevs/pi-cerebro",
				Options: piInitOptions{
					ProjectDir: absResolved,
					BrainPath:  brainPath,
					Boot: piInitBoot{
						Limit: 1,
					},
				},
			},
		},
	}

	// A03: emit via encoding/json — NOT fmt.Sprintf. MarshalIndent produces
	// stable byte output (struct fields preserve declaration order; no map keys).
	out, err := json.MarshalIndent(snippet, "", "  ")
	if err != nil {
		return fmt.Errorf("marshalling snippet: %w", err)
	}
	// Append a trailing newline so the output is shell-friendly (same as
	// json.NewEncoder.Encode which always appends '\n').
	out = append(out, '\n')
	_, err = cmd.OutOrStdout().Write(out)
	return err
}
