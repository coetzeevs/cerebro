package main

// edge_validity_e2e_test.go — CLI E2E tests for the agentic-xtzn bi-temporal
// validity window: AC2 (--valid-at/--invalid-at on `cerebro edge`), AC6 (upsert
// window-update + NULL overwrite), and the Security negative cases (malformed
// flags reject with a clear error, non-zero exit, no panic, no write).
//
// HOME is redirected via t.Setenv so brain.ProjectPath resolves under a temp
// dir, never the operator's real brain (cmd_add_beadsid_test.go pattern).

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coetzeevs/cerebro/brain"
	"github.com/coetzeevs/cerebro/internal/store"
)

// setupEdgeTest creates an isolated brain with two nodes and returns the project
// dir plus the two full node IDs. Global edge/get/format flags are saved and
// restored via t.Cleanup.
func setupEdgeTest(t *testing.T) (projectDir, idA, idB string) {
	t.Helper()

	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	projectDir = t.TempDir()

	brainPath := brain.ProjectPath(projectDir)
	if err := os.MkdirAll(filepath.Dir(brainPath), 0o750); err != nil {
		t.Fatalf("mkdir brain parent: %v", err)
	}
	b, err := brain.Init(brainPath, brain.EmbedConfig{Provider: "none"})
	if err != nil {
		t.Fatalf("brain.Init: %v", err)
	}
	idA, err = b.Add("source node", store.TypeConcept, brain.WithImportance(0.5))
	if err != nil {
		t.Fatalf("Add A: %v", err)
	}
	idB, err = b.Add("target node", store.TypeConcept, brain.WithImportance(0.5))
	if err != nil {
		t.Fatalf("Add B: %v", err)
	}
	if err := b.Close(); err != nil {
		t.Fatalf("brain.Close: %v", err)
	}

	oldProject, oldFormat, oldQuiet := projectFlag, formatFlag, quietFlag
	oldValidAt, oldInvalidAt, oldGetAsOf := edgeValidAtFlag, edgeInvalidAtFlag, getAsOfFlag
	t.Cleanup(func() {
		projectFlag, formatFlag, quietFlag = oldProject, oldFormat, oldQuiet
		edgeValidAtFlag, edgeInvalidAtFlag, getAsOfFlag = oldValidAt, oldInvalidAt, oldGetAsOf
	})
	projectFlag = projectDir
	formatFlag = "json"
	quietFlag = false
	edgeValidAtFlag, edgeInvalidAtFlag, getAsOfFlag = "", "", ""
	rootCmd.SilenceUsage = true

	return projectDir, idA, idB
}

// runCmd executes rootCmd with the given args, capturing stdout.
//
// Cobra retains per-flag state (bound vars AND the Changed() bit) across
// Execute calls within one process — in production each CLI invocation is a
// fresh process, so this only bites multi-Execute tests (memory 9db36629 §3).
// We therefore reset the validity-window flags and their Changed state on the
// resolved subcommand before each run, so a flag absent from THIS invocation's
// args is correctly seen as unset.
func runCmd(t *testing.T, args ...string) (string, error) {
	t.Helper()
	resetValidityFlags(t, args)

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	rootCmd.SetArgs(args)
	execErr := rootCmd.Execute()

	_ = w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	return buf.String(), execErr
}

// resetValidityFlags clears the bound vars and Changed() bit for the
// validity-window flags on whichever subcommand is being invoked.
func resetValidityFlags(t *testing.T, args []string) {
	t.Helper()
	edgeValidAtFlag, edgeInvalidAtFlag, getAsOfFlag = "", "", ""
	if len(args) == 0 {
		return
	}
	cmd, _, err := rootCmd.Find(args)
	if err != nil || cmd == nil {
		return
	}
	for _, name := range []string{"valid-at", "invalid-at", "as-of"} {
		if f := cmd.Flags().Lookup(name); f != nil {
			f.Changed = false
			_ = f.Value.Set(f.DefValue)
		}
	}
}

// getEdgeFromBrain reads the single edge connected to idA directly from the
// store (bypassing the CLI) for assertions on the persisted row.
func getEdgeFromBrain(t *testing.T, projectDir, idA string) store.Edge {
	t.Helper()
	b, err := brain.Open(brain.ProjectPath(projectDir))
	if err != nil {
		t.Fatalf("brain.Open: %v", err)
	}
	defer func() { _ = b.Close() }()
	nwe, err := b.Get(idA, nil)
	if err != nil {
		t.Fatalf("b.Get: %v", err)
	}
	if len(nwe.Edges) != 1 {
		t.Fatalf("expected exactly 1 edge on node, got %d", len(nwe.Edges))
	}
	return nwe.Edges[0]
}

// edgeCount returns the total number of edge rows in the brain.
func edgeCount(t *testing.T, projectDir string) int {
	t.Helper()
	b, err := brain.Open(brain.ProjectPath(projectDir))
	if err != nil {
		t.Fatalf("brain.Open: %v", err)
	}
	defer func() { _ = b.Close() }()
	edges, err := b.Store().ListAllEdges()
	if err != nil {
		t.Fatalf("ListAllEdges: %v", err)
	}
	return len(edges)
}

// TestEdgeCLI_StoresValidityWindow (AC2) — `cerebro edge a b rel --valid-at
// <RFC3339> --invalid-at <RFC3339>` stores the UTC bounds; the JSON output
// reports an id.
func TestEdgeCLI_StoresValidityWindow(t *testing.T) {
	projectDir, idA, idB := setupEdgeTest(t)

	out, err := runCmd(t, "edge", idA, idB, "relates_to",
		"--valid-at", "2026-01-01T00:00:00Z", "--invalid-at", "2026-06-01T00:00:00Z")
	if err != nil {
		t.Fatalf("edge command errored: %v", err)
	}

	var resp map[string]int64
	if jsonErr := json.Unmarshal([]byte(out), &resp); jsonErr != nil {
		t.Fatalf("output not JSON id object: %q (%v)", out, jsonErr)
	}
	if resp["id"] == 0 {
		t.Errorf("expected non-zero edge id in JSON output, got %q", out)
	}

	e := getEdgeFromBrain(t, projectDir, idA)
	wantValid := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	wantInvalid := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	if e.ValidAt == nil || !e.ValidAt.UTC().Equal(wantValid) {
		t.Errorf("valid_at: got %v want %v", e.ValidAt, wantValid)
	}
	if e.InvalidAt == nil || !e.InvalidAt.UTC().Equal(wantInvalid) {
		t.Errorf("invalid_at: got %v want %v", e.InvalidAt, wantInvalid)
	}
}

// TestEdgeCLI_OmittedFlagStoresNull (AC2) — omitting --invalid-at stores NULL.
func TestEdgeCLI_OmittedFlagStoresNull(t *testing.T) {
	projectDir, idA, idB := setupEdgeTest(t)

	if _, err := runCmd(t, "edge", idA, idB, "relates_to", "--valid-at", "2026-03-01"); err != nil {
		t.Fatalf("edge command errored: %v", err)
	}
	e := getEdgeFromBrain(t, projectDir, idA)
	if e.ValidAt == nil {
		t.Error("valid_at should be set")
	}
	if e.InvalidAt != nil {
		t.Errorf("invalid_at should be NULL when flag omitted, got %v", e.InvalidAt)
	}
	// Date-only form is interpreted as midnight UTC.
	want := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	if e.ValidAt != nil && !e.ValidAt.UTC().Equal(want) {
		t.Errorf("date-only valid_at: got %v want %v", e.ValidAt, want)
	}
}

// TestEdgeCLI_UpsertUpdatesInPlace (AC6) — re-running edge updates the window in
// place: id retained, no duplicate row.
func TestEdgeCLI_UpsertUpdatesInPlace(t *testing.T) {
	projectDir, idA, idB := setupEdgeTest(t)

	out1, err := runCmd(t, "edge", idA, idB, "relates_to",
		"--valid-at", "2026-01-01T00:00:00Z", "--invalid-at", "2026-02-01T00:00:00Z")
	if err != nil {
		t.Fatalf("first edge: %v", err)
	}
	var r1 map[string]int64
	_ = json.Unmarshal([]byte(out1), &r1)
	firstID := r1["id"]
	if firstID == 0 {
		t.Fatal("first edge id is 0")
	}

	out2, err := runCmd(t, "edge", idA, idB, "relates_to",
		"--valid-at", "2026-03-01T00:00:00Z", "--invalid-at", "2026-04-01T00:00:00Z")
	if err != nil {
		t.Fatalf("re-edge: %v", err)
	}
	var r2 map[string]int64
	_ = json.Unmarshal([]byte(out2), &r2)
	if r2["id"] != firstID {
		t.Errorf("re-add reported id %d, expected existing id %d (TL-PI-N4)", r2["id"], firstID)
	}

	if n := edgeCount(t, projectDir); n != 1 {
		t.Errorf("expected 1 edge after re-add (no duplicate), got %d", n)
	}

	e := getEdgeFromBrain(t, projectDir, idA)
	if e.ID != firstID {
		t.Errorf("edge id changed on upsert: got %d want %d", e.ID, firstID)
	}
	wantValid := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	if e.ValidAt == nil || !e.ValidAt.UTC().Equal(wantValid) {
		t.Errorf("window not updated in place: valid_at=%v want %v", e.ValidAt, wantValid)
	}
}

// TestEdgeCLI_UpsertNullOverwrite (AC6) — re-adding with only --valid-at clears
// invalid_at to NULL (full-window re-assertion, not a partial patch).
func TestEdgeCLI_UpsertNullOverwrite(t *testing.T) {
	projectDir, idA, idB := setupEdgeTest(t)

	if _, err := runCmd(t, "edge", idA, idB, "relates_to",
		"--valid-at", "2026-01-01T00:00:00Z", "--invalid-at", "2026-06-01T00:00:00Z"); err != nil {
		t.Fatalf("first edge: %v", err)
	}
	// Re-add with ONLY --valid-at: invalid_at must be cleared to NULL.
	if _, err := runCmd(t, "edge", idA, idB, "relates_to", "--valid-at", "2026-02-01T00:00:00Z"); err != nil {
		t.Fatalf("re-edge: %v", err)
	}
	e := getEdgeFromBrain(t, projectDir, idA)
	if e.InvalidAt != nil {
		t.Errorf("invalid_at should be cleared to NULL on full-window re-assertion, got %v", e.InvalidAt)
	}
	wantValid := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	if e.ValidAt == nil || !e.ValidAt.UTC().Equal(wantValid) {
		t.Errorf("valid_at should be updated to %v, got %v", wantValid, e.ValidAt)
	}
}

// TestEdgeCLI_RejectsInvertedWindow (E-2) — valid_at strictly after invalid_at
// is rejected with a clear error and writes nothing.
func TestEdgeCLI_RejectsInvertedWindow(t *testing.T) {
	projectDir, idA, idB := setupEdgeTest(t)

	_, err := runCmd(t, "edge", idA, idB, "relates_to",
		"--valid-at", "2026-06-01T00:00:00Z", "--invalid-at", "2026-01-01T00:00:00Z")
	if err == nil {
		t.Fatal("expected error for inverted window (valid_at > invalid_at), got nil")
	}
	if !strings.Contains(err.Error(), "invalid window") {
		t.Errorf("error should mention 'invalid window', got: %v", err)
	}
	if n := edgeCount(t, projectDir); n != 0 {
		t.Errorf("inverted-window edge must NOT be written, found %d edges", n)
	}
}

// TestEdgeCLI_EqualBoundsAllowed (E-2 boundary) — equal bounds (zero-width
// [t,t)) are ALLOWED (a well-defined degenerate window), not rejected.
func TestEdgeCLI_EqualBoundsAllowed(t *testing.T) {
	projectDir, idA, idB := setupEdgeTest(t)

	if _, err := runCmd(t, "edge", idA, idB, "relates_to",
		"--valid-at", "2026-04-01T00:00:00Z", "--invalid-at", "2026-04-01T00:00:00Z"); err != nil {
		t.Fatalf("equal bounds should be allowed, got error: %v", err)
	}
	if n := edgeCount(t, projectDir); n != 1 {
		t.Errorf("expected 1 zero-width edge written, got %d", n)
	}
}

// TestEdgeCLI_RejectsMalformedValidAt (Security) — garbage --valid-at returns a
// clear error, writes nothing, and does not panic.
func TestEdgeCLI_RejectsMalformedValidAt(t *testing.T) {
	projectDir, idA, idB := setupEdgeTest(t)

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("malformed --valid-at panicked: %v", r)
		}
	}()
	_, err := runCmd(t, "edge", idA, idB, "relates_to", "--valid-at", "not-a-date")
	if err == nil {
		t.Fatal("expected error for malformed --valid-at, got nil")
	}
	if !strings.Contains(err.Error(), "--valid-at") {
		t.Errorf("error should mention --valid-at, got: %v", err)
	}
	if n := edgeCount(t, projectDir); n != 0 {
		t.Errorf("malformed-flag edge must NOT be written, found %d edges", n)
	}
}

// TestEdgeCLI_RejectsMalformedInvalidAt (Security) — garbage --invalid-at.
func TestEdgeCLI_RejectsMalformedInvalidAt(t *testing.T) {
	projectDir, idA, idB := setupEdgeTest(t)

	_, err := runCmd(t, "edge", idA, idB, "relates_to", "--invalid-at", "2026-13-40")
	if err == nil {
		t.Fatal("expected error for malformed --invalid-at, got nil")
	}
	if !strings.Contains(err.Error(), "--invalid-at") {
		t.Errorf("error should mention --invalid-at, got: %v", err)
	}
	if n := edgeCount(t, projectDir); n != 0 {
		t.Errorf("malformed-flag edge must NOT be written, found %d edges", n)
	}
}

// TestGetCLI_RejectsMalformedAsOf (Security) — garbage --as-of on `get` returns
// a clear error and does not panic.
func TestGetCLI_RejectsMalformedAsOf(t *testing.T) {
	_, idA, _ := setupEdgeTest(t)

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("malformed --as-of panicked: %v", r)
		}
	}()
	_, err := runCmd(t, "get", idA, "--as-of", "garbage'); DROP TABLE edges;--")
	if err == nil {
		t.Fatal("expected error for malformed --as-of, got nil")
	}
	if !strings.Contains(err.Error(), "--as-of") {
		t.Errorf("error should mention --as-of, got: %v", err)
	}
}

// TestGetCLI_AsOfFiltersEdges (AC3 via CLI) — `get --as-of` excludes an edge
// outside its window and includes one inside it (filter-not-defeated, per
// Security note for QA).
func TestGetCLI_AsOfFiltersEdges(t *testing.T) {
	_, idA, idB := setupEdgeTest(t)

	// Edge valid only [2026-01-01, 2026-02-01).
	if _, err := runCmd(t, "edge", idA, idB, "relates_to",
		"--valid-at", "2026-01-01T00:00:00Z", "--invalid-at", "2026-02-01T00:00:00Z"); err != nil {
		t.Fatalf("edge: %v", err)
	}

	// Inside the window: the edge is returned.
	out, err := runCmd(t, "get", idA, "--as-of", "2026-01-15T00:00:00Z")
	if err != nil {
		t.Fatalf("get inside window: %v", err)
	}
	var nweIn store.NodeWithEdges
	if jsonErr := json.Unmarshal([]byte(out), &nweIn); jsonErr != nil {
		t.Fatalf("get output not JSON: %q (%v)", out, jsonErr)
	}
	if len(nweIn.Edges) != 1 {
		t.Errorf("expected 1 edge inside window, got %d", len(nweIn.Edges))
	}

	// Outside the window (after invalid_at): the edge is filtered out.
	out2, err := runCmd(t, "get", idA, "--as-of", "2026-03-01T00:00:00Z")
	if err != nil {
		t.Fatalf("get outside window: %v", err)
	}
	var nweOut store.NodeWithEdges
	if jsonErr := json.Unmarshal([]byte(out2), &nweOut); jsonErr != nil {
		t.Fatalf("get output not JSON: %q (%v)", out2, jsonErr)
	}
	if len(nweOut.Edges) != 0 {
		t.Errorf("expected 0 edges outside window (filter not defeated), got %d", len(nweOut.Edges))
	}
}
