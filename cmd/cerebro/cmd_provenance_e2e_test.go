package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/coetzeevs/cerebro/brain"
	"github.com/coetzeevs/cerebro/internal/store"
)

// captureStdout runs fn with os.Stdout redirected to a pipe and returns what was
// written. Mirrors the repo's CLI-output capture idiom.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout = w
	defer func() { os.Stdout = orig }()

	done := make(chan string)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		done <- buf.String()
	}()

	fn()
	_ = w.Close()
	return <-done
}

// newCLIBrain initialises a brain at the path the CLI resolves for projectDir
// (the realpath-hashed ~/.cerebro/projects/<hash>.sqlite), so a subsequent
// `cerebro ... --project projectDir` opens the same brain.
func newCLIBrain(t *testing.T, projectDir string) *brain.Brain {
	t.Helper()
	p := brain.ProjectPath(projectDir)
	b, err := brain.Init(p, brain.EmbedConfig{Provider: "none"})
	if err != nil {
		t.Fatalf("brain.Init: %v", err)
	}
	t.Cleanup(func() {
		for _, suffix := range []string{"", "-wal", "-shm"} {
			_ = os.Remove(p + suffix)
		}
	})
	return b
}

// provenanceTestEnv builds a brain with a concept consolidated from two episodes,
// returns the project dir + the relevant IDs, and points the CLI at it.
func provenanceTestEnv(t *testing.T) (projectDir, concept, e1, e2 string) {
	t.Helper()
	projectDir = t.TempDir()
	b := newCLIBrain(t, projectDir)
	concept, _ = b.Add("synthesis concept", store.TypeConcept, brain.WithImportance(0.8))
	e1, _ = b.Add("episode one", store.TypeEpisode)
	e2, _ = b.Add("episode two", store.TypeEpisode)
	if err := b.Consolidate(concept, []string{e1, e2}); err != nil {
		t.Fatalf("Consolidate: %v", err)
	}
	_ = b.Close()
	t.Setenv("CLAUDE_PROJECT_DIR", "")
	projectFlag = ""
	return projectDir, concept, e1, e2
}

// runCLI executes rootCmd against the brain for projectDir and returns stdout.
func runCLI(t *testing.T, projectDir string, args ...string) (string, error) {
	t.Helper()
	// Reset global flag state between invocations (cobra flags leak across Execute).
	resetProvenanceCLIState()
	full := append([]string{"--project", projectDir}, args...)
	var execErr error
	out := captureStdout(t, func() {
		rootCmd.SetArgs(full)
		execErr = rootCmd.Execute()
	})
	return out, execErr
}

// resetProvenanceCLIState resets the flag globals AND cobra's per-flag Changed
// bits this suite touches, so a prior Execute's value does not leak into the
// next. cobra keeps a flag.Changed bit set across Execute on a reused command
// instance (the documented flag-leak-across-Execute footgun); production runs
// each command in a fresh process, so this reset is a test-harness concern only.
func resetProvenanceCLIState() {
	formatFlag = "md"
	quietFlag = false
	getWithProvenanceFlag = 5
	recallWithProvenanceFlag = 1
	recallProvenanceDepthFlag = 1
	addProvenanceRootFlag = false
	consolidateIntoFlag = ""

	// Clear sticky Changed bits on the persistent + per-command flags this suite
	// exercises, so Flags().Changed(...) reflects only the current invocation.
	// Reset by name via Lookup so the test imports no extra package (pflag is a
	// transitive dep of cobra; importing it directly would promote it in go.mod).
	clearChanged := func(cmdName string, flags ...string) {
		c, _, err := rootCmd.Find([]string{cmdName})
		if err != nil || c == nil {
			return
		}
		for _, fn := range flags {
			if f := c.Flags().Lookup(fn); f != nil {
				f.Changed = false
			}
		}
	}
	if pf := rootCmd.PersistentFlags().Lookup("format"); pf != nil {
		pf.Changed = false
	}
	clearChanged("get", "with-provenance", "as-of", "format")
	clearChanged("recall", "with-provenance", "provenance-depth", "as-of", "subtype", "format")
	clearChanged("list", "subtype", "format")
	clearChanged("add", "provenance-root", "subtype", "type", "format")
	clearChanged("consolidate", "into", "format")
}

// brainAtDir opens the brain the CLI resolves for a given project dir.
func brainAtDir(t *testing.T, projectDir string) *brain.Brain {
	t.Helper()
	b, err := brain.Open(brain.ProjectPath(projectDir))
	if err != nil {
		t.Fatalf("brain.Open: %v", err)
	}
	return b
}

// --- AC5: get --with-provenance ---

func TestGetWithProvenanceMD(t *testing.T) {
	projectDir, concept, _, _ := provenanceTestEnv(t)
	out, err := runCLI(t, projectDir, "get", concept, "--with-provenance")
	if err != nil {
		t.Fatalf("get --with-provenance: %v", err)
	}
	if !strings.Contains(out, "## Provenance (depth 5)") {
		t.Fatalf("expected a Provenance block at default depth 5, got:\n%s", out)
	}
	// Both source episodes should appear in the chain.
	if !strings.Contains(out, "depth=1") {
		t.Fatalf("expected depth=1 sources in the chain, got:\n%s", out)
	}
}

func TestGetWithProvenanceDepthOverride(t *testing.T) {
	projectDir, concept, _, _ := provenanceTestEnv(t)
	out, err := runCLI(t, projectDir, "get", concept, "--with-provenance=3")
	if err != nil {
		t.Fatalf("get --with-provenance=3: %v", err)
	}
	if !strings.Contains(out, "## Provenance (depth 3)") {
		t.Fatalf("expected depth 3 in the Provenance header, got:\n%s", out)
	}
}

func TestGetWithProvenanceJSON(t *testing.T) {
	projectDir, concept, _, _ := provenanceTestEnv(t)
	out, err := runCLI(t, projectDir, "get", concept, "--with-provenance", "--format", "json")
	if err != nil {
		t.Fatalf("get json: %v", err)
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(out), &obj); err != nil {
		t.Fatalf("json: %v\n%s", err, out)
	}
	if obj["provenance_status"] != "complete" {
		t.Fatalf("expected provenance_status=complete, got %v", obj["provenance_status"])
	}
	prov, ok := obj["provenance"].([]any)
	if !ok || len(prov) != 2 {
		t.Fatalf("expected 2-element provenance chain, got %v", obj["provenance"])
	}
}

// --- AC6: provenance_status always in JSON for get/list ---

func TestGetProvenanceStatusInJSON(t *testing.T) {
	projectDir, _, e1, _ := provenanceTestEnv(t)
	// An episode is a leaf (no outgoing derived_from); it should read "none".
	out, err := runCLI(t, projectDir, "get", e1, "--format", "json")
	if err != nil {
		t.Fatalf("get json: %v", err)
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(out), &obj); err != nil {
		t.Fatalf("json: %v\n%s", err, out)
	}
	if obj["provenance_status"] != "none" {
		t.Fatalf("episode should be provenance_status=none, got %v", obj["provenance_status"])
	}
	// No chain attached without the flag.
	if _, present := obj["provenance"]; present {
		t.Fatalf("provenance chain should be absent without --with-provenance, got %v", obj["provenance"])
	}
}

func TestListProvenanceStatusInJSON(t *testing.T) {
	projectDir, concept, _, _ := provenanceTestEnv(t)
	out, err := runCLI(t, projectDir, "list", "--format", "json")
	if err != nil {
		t.Fatalf("list json: %v", err)
	}
	var arr []map[string]any
	if err := json.Unmarshal([]byte(out), &arr); err != nil {
		t.Fatalf("json: %v\n%s", err, out)
	}
	found := false
	for _, n := range arr {
		if n["id"] == concept {
			found = true
			if n["provenance_status"] != "complete" {
				t.Fatalf("concept in list should be complete, got %v", n["provenance_status"])
			}
		}
		if _, ok := n["provenance_status"]; !ok {
			t.Fatalf("every list node should carry provenance_status, missing on %v", n["id"])
		}
	}
	if !found {
		t.Fatal("concept not found in list output")
	}
}

// --- AC5 byte-identity: flag-absent get is byte-identical to pre-lbjg ---

func TestGetFlagAbsentByteIdentity(t *testing.T) {
	projectDir, concept, _, _ := provenanceTestEnv(t)

	// Capture the md output without any provenance flag.
	out, err := runCLI(t, projectDir, "get", concept)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	// Build the expected pre-lbjg md output directly from the same node, using the
	// exact format the pre-lbjg cmd_get.go produced (no provenance lines).
	b := brainAtDir(t, projectDir)
	defer func() { _ = b.Close() }()
	nwe, err := b.Get(concept, nil)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	var want bytes.Buffer
	writeLegacyGetMD(&want, nwe)

	if out != want.String() {
		t.Fatalf("flag-absent get md is NOT byte-identical to pre-lbjg.\n--- got ---\n%q\n--- want ---\n%q", out, want.String())
	}
	// And it must NOT contain any provenance annotation.
	if strings.Contains(out, "Provenance") || strings.Contains(out, "provenance_status") {
		t.Fatalf("flag-absent md leaked provenance text:\n%s", out)
	}
}

// writeLegacyGetMD reproduces the EXACT pre-lbjg cmd_get.go md rendering, so the
// byte-identity test is anchored to the historical output shape, not to the
// current code (which would make the test tautological).
func writeLegacyGetMD(buf *bytes.Buffer, nwe *store.NodeWithEdges) {
	fprintf := func(format string, a ...any) { _, _ = fmt.Fprintf(buf, format, a...) }
	fprintf("# %s\n\n", nwe.ID)
	fprintf("Type: %s", nwe.Type)
	if nwe.Subtype != "" {
		fprintf("/%s", nwe.Subtype)
	}
	fprintf("\nStatus: %s\n", nwe.Status)
	fprintf("Importance: %.2f | Decay: %.4f | Access count: %d\n", nwe.Importance, nwe.DecayRate, nwe.AccessCount)
	fprintf("Created: %s | Last accessed: %s\n\n", nwe.CreatedAt.Format("2006-01-02 15:04"), nwe.LastAccessed.Format("2006-01-02 15:04"))
	fprintf("## Content\n%s\n\n", nwe.Content)
	if len(nwe.Edges) > 0 {
		fprintf("## Edges (%d)\n", len(nwe.Edges))
		for i := range nwe.Edges {
			e := &nwe.Edges[i]
			arrow := "→"
			other := e.TargetID
			if e.SourceID != nwe.ID {
				arrow = "←"
				other = e.SourceID
			}
			fprintf("  %s %s [%s]%s\n", arrow, other[:8], e.Relation, formatEdgeWindow(e.ValidAt, e.InvalidAt))
		}
	}
}

// --- AC1: add --provenance-root ---

func TestAddProvenanceRootFlag(t *testing.T) {
	dir := t.TempDir()
	b := newCLIBrain(t, dir)
	_ = b.Close()

	out, err := runCLI(t, dir, "add", "root memory", "--provenance-root", "--type", "episode")
	if err != nil {
		t.Fatalf("add --provenance-root: %v", err)
	}
	id := strings.TrimSpace(out)

	b2 := brainAtDir(t, dir)
	defer func() { _ = b2.Close() }()
	n, err := b2.Get(id, nil)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !n.ProvenanceRoot {
		t.Fatal("add --provenance-root did not set provenance_root=1")
	}
}

// --- AC3: consolidate command, idempotent + fail-closed ---

func TestConsolidateCommandWritesEdges(t *testing.T) {
	dir := t.TempDir()
	b := newCLIBrain(t, dir)
	concept, _ := b.Add("synth", store.TypeConcept)
	e1, _ := b.Add("ep1", store.TypeEpisode)
	_ = b.Close()

	if _, err := runCLI(t, dir, "consolidate", "--into", concept, e1); err != nil {
		t.Fatalf("consolidate: %v", err)
	}

	b2 := brainAtDir(t, dir)
	defer func() { _ = b2.Close() }()
	chain, _ := b2.WalkProvenance(concept, 5)
	if len(chain) != 2 {
		t.Fatalf("expected concept + 1 source in chain, got %d", len(chain))
	}
}

func TestConsolidateCommandFailsClosed(t *testing.T) {
	dir := t.TempDir()
	b := newCLIBrain(t, dir)
	concept, _ := b.Add("synth", store.TypeConcept)
	e1, _ := b.Add("ep1", store.TypeEpisode)
	_ = b.Close()

	// Second source does not exist (and 4+ chars so ResolvePrefix actually
	// searches and fails, rather than erroring on length).
	_, execErr := runCLI(t, dir, "consolidate", "--into", concept, e1, "ghostnode")
	if execErr == nil {
		t.Fatal("consolidate with an unresolvable source should exit non-zero")
	}

	b2 := brainAtDir(t, dir)
	defer func() { _ = b2.Close() }()
	chain, _ := b2.WalkProvenance(concept, 5)
	if len(chain) != 1 { // concept only — no partial edge written
		t.Fatalf("fail-closed: expected no partial write, got chain len %d", len(chain))
	}
}

// --- AC5: recall flag help text registration ---

func TestProvenanceFlagsRegistered(t *testing.T) {
	getCmd, _, _ := rootCmd.Find([]string{"get"})
	if getCmd.Flags().Lookup("with-provenance") == nil {
		t.Error("get is missing --with-provenance")
	}
	if getCmd.Flags().Lookup("with-provenance").NoOptDefVal != "5" {
		t.Error("get --with-provenance NoOptDefVal should be 5")
	}
	recallCmd, _, _ := rootCmd.Find([]string{"recall"})
	if recallCmd.Flags().Lookup("with-provenance") == nil {
		t.Error("recall is missing --with-provenance")
	}
	if recallCmd.Flags().Lookup("with-provenance").NoOptDefVal != "1" {
		t.Error("recall --with-provenance NoOptDefVal should be 1")
	}
	if recallCmd.Flags().Lookup("provenance-depth") == nil {
		t.Error("recall is missing --provenance-depth")
	}
	addCmd, _, _ := rootCmd.Find([]string{"add"})
	if addCmd.Flags().Lookup("provenance-root") == nil {
		t.Error("add is missing --provenance-root")
	}
	consolidateCmd, _, _ := rootCmd.Find([]string{"consolidate"})
	if consolidateCmd == nil || consolidateCmd.Flags().Lookup("into") == nil {
		t.Error("consolidate command or --into flag missing")
	}
}
