package main

// cmd_add_beadsid_test.go — TDD tests for --beads-id cobra flag and N-S1 validation.
//
// Ticket: HS-039
//
// RED phase: these tests are written FIRST and drive the implementation of:
//   - --beads-id cobra flag on `cerebro add`
//   - N-S1 input validation (HS-029 canonical regex)
//   - brain.WithMetadata({"beadsId":"..."}) wiring
//
// N-S1 HIGH BLOCKING: validate against HS-029 canonical regex
// ^[a-z][a-z0-9-]{0,31}-[0-9a-z]{3,32}$  (byte-identical to validate-hello-stack.sh:243,
// validate-meepo-beads-link.sh:79, and validate-beads-pi.sh:150)
//   - empty post-trim  → flag treated as absent (no metadata write)
//   - non-matching     → CLI error with canonical pattern in message
//
// Stale-metadata merge contract (§2 forward-compatibility):
// If a future --metadata flag is added, beadsId MUST WIN on key collision.
// Documented here to lock the contract for the ticket adding --metadata.
//
// Implementation note: HOME is redirected via t.Setenv("HOME", tmpHome) so
// brain.ProjectPath hashes resolve under a temp dir (same pattern as
// cmd_pi_init_test.go), preventing contamination of the operator's real brain.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/coetzeevs/cerebro/brain"
	"github.com/coetzeevs/cerebro/internal/store"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// setupAddTest creates a temp project dir + isolated HOME, initialises the
// brain at the path that brain.ProjectPath will resolve to, and wires up
// the global add flags. Returns (projectDir, cleanup) where cleanup is
// registered via t.Cleanup.
//
// WHY redirect HOME? brain.ProjectPath hashes the project dir and stores
// the brain at ~/.cerebro/projects/<hash>. Tests MUST use an isolated HOME
// so they don't touch the operator's real brain (and so they can create the
// brain file at the expected location before the command runs).
//
// WHY save/restore flags instead of rootCmd.ResetFlags?  Cobra global
// persistent flags (-p, --format, etc.) must NOT be reset — doing so
// unregisters -p and causes "unknown shorthand flag: 'p'" errors (per memory
// a4ec2a1d / cmd_pi_init_test.go pattern).
func setupAddTest(t *testing.T) (projectDir string) {
	t.Helper()

	// Isolated HOME so .cerebro goes to a temp dir
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	// Temp project dir
	projectDir = t.TempDir()

	// Pre-init the brain at the path brain.ProjectPath will compute
	brainPath := brain.ProjectPath(projectDir)
	if err := createParentDir(brainPath); err != nil {
		t.Fatalf("could not create brain parent dir: %v", err)
	}
	b, err := brain.Init(brainPath, brain.EmbedConfig{Provider: "none"})
	if err != nil {
		t.Fatalf("brain.Init: %v", err)
	}
	if err := b.Close(); err != nil {
		t.Fatalf("brain.Close: %v", err)
	}

	// Save and restore global flag vars (memory a4ec2a1d / HS-009 pattern)
	oldProject := projectFlag
	oldType := addTypeFlag
	oldImportance := addImportanceFlag
	oldSubtype := addSubtypeFlag
	oldBeadsId := addBeadsIdFlag
	oldFormat := formatFlag
	t.Cleanup(func() {
		projectFlag = oldProject
		addTypeFlag = oldType
		addImportanceFlag = oldImportance
		addSubtypeFlag = oldSubtype
		addBeadsIdFlag = oldBeadsId
		formatFlag = oldFormat
	})

	// Set defaults for add tests
	projectFlag = projectDir
	addTypeFlag = "episode"
	addImportanceFlag = 0.5
	addSubtypeFlag = ""
	addBeadsIdFlag = ""
	formatFlag = "md"

	rootCmd.SilenceUsage = true

	return projectDir
}

// listNodesForProject opens the brain at brain.ProjectPath(projectDir) and
// returns all stored nodes.
func listNodesForProject(t *testing.T, projectDir string) []store.Node {
	t.Helper()
	brainPath := brain.ProjectPath(projectDir)
	b, err := brain.Open(brainPath)
	if err != nil {
		t.Fatalf("brain.Open: %v", err)
	}
	defer func() { _ = b.Close() }()
	nodes, err := b.List(store.ListNodesOpts{})
	if err != nil {
		t.Fatalf("b.List: %v", err)
	}
	return nodes
}

// createParentDir ensures the directory for a file path exists.
func createParentDir(path string) error {
	dir := filepath.Dir(path)
	return os.MkdirAll(dir, 0o750)
}

// ---------------------------------------------------------------------------
// RED: N-S1 validation — valid beads-id is accepted and stored in metadata
// ---------------------------------------------------------------------------

func TestAddBeadsId_ValidId_Accepted(t *testing.T) {
	// A valid beads-id matching ^[a-z][a-z0-9-]{0,31}-[0-9a-z]{3,32}$ must:
	//   - succeed (exit 0)
	//   - store the node with metadata {"beadsId":"agentic-abc"}
	//   - leave content unchanged (AC2 back-compat)
	projectDir := setupAddTest(t)

	rootCmd.SetArgs([]string{"add", "test memory content", "--beads-id", "agentic-abc"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("expected no error for valid --beads-id 'agentic-abc', got: %v", err)
	}

	nodes := listNodesForProject(t, projectDir)
	if len(nodes) == 0 {
		t.Fatal("expected at least one stored node, got 0")
	}

	// AC2: metadata has beadsId; content is unchanged (not "BD_ID: agentic-abc ...")
	found := false
	for _, n := range nodes {
		if n.Metadata == nil {
			continue
		}
		var meta map[string]string
		if err := json.Unmarshal(n.Metadata, &meta); err != nil {
			continue
		}
		if meta["beadsId"] == "agentic-abc" {
			found = true
			// AC2: content must be verbatim; no BD_ID embedding
			if n.Content != "test memory content" {
				t.Errorf("content should be verbatim, got: %q", n.Content)
			}
		}
	}
	if !found {
		t.Error("expected stored node to have metadata with beadsId=agentic-abc")
	}
}

// ---------------------------------------------------------------------------
// RED: N-S1 validation — uppercase beads-id is rejected
// ---------------------------------------------------------------------------

func TestAddBeadsId_UppercaseId_Rejected(t *testing.T) {
	// N-S1: uppercase value must not match ^[a-z][a-z0-9-]{0,31}-[0-9a-z]{3,32}$
	// Expect a CLI error containing "invalid --beads-id" with the canonical pattern.
	setupAddTest(t)

	rootCmd.SetArgs([]string{"add", "test content", "--beads-id", "AGENTIC-abc"})
	execErr := rootCmd.Execute()
	if execErr == nil {
		t.Fatal("expected error for invalid --beads-id 'AGENTIC-abc' (uppercase), got nil")
	}
	if !strings.Contains(execErr.Error(), "invalid --beads-id") {
		t.Errorf("error message should contain 'invalid --beads-id', got: %v", execErr)
	}
	if !strings.Contains(execErr.Error(), "^[a-z][a-z0-9-]{0,31}-[0-9a-z]{3,32}$") {
		t.Errorf("error message should contain canonical regex, got: %v", execErr)
	}
}

// ---------------------------------------------------------------------------
// RED: N-S1 validation — no-hyphen value is rejected
// ---------------------------------------------------------------------------

func TestAddBeadsId_NoHyphenId_Rejected(t *testing.T) {
	// N-S1: 'foo' (no hyphen, does not match the suffix requirement) must return error.
	setupAddTest(t)

	rootCmd.SetArgs([]string{"add", "test content", "--beads-id", "foo"})
	execErr := rootCmd.Execute()
	if execErr == nil {
		t.Fatal("expected error for invalid --beads-id 'foo' (no hyphen), got nil")
	}
	if !strings.Contains(execErr.Error(), "invalid --beads-id") {
		t.Errorf("error message should contain 'invalid --beads-id', got: %v", execErr)
	}
}

// ---------------------------------------------------------------------------
// RED: N-S1 validation — empty after trim is treated as absent (no error, no metadata)
// ---------------------------------------------------------------------------

func TestAddBeadsId_EmptyAfterTrim_TreatedAsAbsent(t *testing.T) {
	// N-S1: empty string post-trim → flag treated as absent.
	// Memory stored successfully; no metadata.beadsId field written.
	projectDir := setupAddTest(t)

	rootCmd.SetArgs([]string{"add", "test content", "--beads-id", ""})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("expected no error for empty --beads-id, got: %v", err)
	}

	nodes := listNodesForProject(t, projectDir)
	if len(nodes) == 0 {
		t.Fatal("expected at least one stored node")
	}
	for _, n := range nodes {
		if n.Metadata == nil {
			continue
		}
		var meta map[string]interface{}
		if err := json.Unmarshal(n.Metadata, &meta); err != nil {
			continue
		}
		if _, hasBeadsId := meta["beadsId"]; hasBeadsId {
			t.Error("expected no beadsId in metadata when --beads-id is empty, but found one")
		}
	}
}

// ---------------------------------------------------------------------------
// RED: N-S1 validation — whitespace-only is treated as absent (no error, no metadata)
// ---------------------------------------------------------------------------

func TestAddBeadsId_WhitespaceOnly_TreatedAsAbsent(t *testing.T) {
	// N-S1: whitespace-only post-trim → flag treated as absent.
	// Same semantics as empty: stored successfully, no beadsId in metadata.
	projectDir := setupAddTest(t)

	rootCmd.SetArgs([]string{"add", "test content", "--beads-id", "   "})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("expected no error for whitespace-only --beads-id, got: %v", err)
	}

	nodes := listNodesForProject(t, projectDir)
	if len(nodes) == 0 {
		t.Fatal("expected at least one stored node")
	}
	for _, n := range nodes {
		if n.Metadata == nil {
			continue
		}
		var meta map[string]interface{}
		if err := json.Unmarshal(n.Metadata, &meta); err != nil {
			continue
		}
		if _, hasBeadsId := meta["beadsId"]; hasBeadsId {
			t.Error("expected no beadsId in metadata for whitespace-only --beads-id")
		}
	}
}
