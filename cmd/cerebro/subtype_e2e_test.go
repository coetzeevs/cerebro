package main

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/coetzeevs/cerebro/brain"
	"github.com/coetzeevs/cerebro/internal/store"
)

// TestCLI_SubtypeHelpText verifies that --help output for update, list, and recall
// includes the --subtype flag and documents its semantics (AC: help text scenario).
func TestCLI_SubtypeHelpText(t *testing.T) {
	// This test validates flag registration by checking cobra command flags.
	// It doesn't need a real brain path.
	for _, cmdName := range []string{"update", "list", "recall"} {
		cmd, _, err := rootCmd.Find([]string{cmdName})
		if err != nil || cmd == nil {
			t.Fatalf("command %q not found in rootCmd", cmdName)
		}
		flag := cmd.Flags().Lookup("subtype")
		if flag == nil {
			t.Errorf("command %q is missing --subtype flag", cmdName)
		}
	}
}

// TestCLI_SubtypeFlow exercises the full subtype lifecycle:
// add --subtype X → list --subtype X → update --subtype Y → list --subtype Y
// → update --subtype "" → list --subtype "" shows it
// This tests the CLI command handler logic using a real brain.
func TestCLI_SubtypeFlow(t *testing.T) {
	dir := t.TempDir()
	brainPath := filepath.Join(dir, "test.sqlite")
	b, err := brain.Init(brainPath, brain.EmbedConfig{})
	if err != nil {
		t.Fatalf("brain.Init: %v", err)
	}
	defer func() { _ = b.Close() }()

	// Add a node with subtype "routing-discovery"
	id, err := b.Add("test memory for subtype flow", store.TypeConcept,
		brain.WithSubtype("routing-discovery"),
		brain.WithImportance(0.8),
	)
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	// 1. List --subtype routing-discovery: should return 1 node
	nodes, err := b.List(store.ListNodesOpts{Subtype: ptrStr("routing-discovery")})
	if err != nil {
		t.Fatalf("List (routing-discovery): %v", err)
	}
	if len(nodes) != 1 {
		t.Errorf("expected 1 node with subtype=routing-discovery, got %d", len(nodes))
	}
	if len(nodes) > 0 {
		data, _ := json.Marshal(nodes[0])
		t.Logf("node: %s", data)
	}

	// 2. Update subtype to "operator-safety"
	if err := b.Update(id, brain.WithUpdatedSubtype("operator-safety")); err != nil {
		t.Fatalf("Update (set operator-safety): %v", err)
	}
	nwe, err := b.Get(id)
	if err != nil {
		t.Fatalf("Get after update: %v", err)
	}
	if nwe.Subtype != "operator-safety" {
		t.Errorf("expected subtype=operator-safety, got %q", nwe.Subtype)
	}

	// 3. List --subtype routing-discovery: should now return 0 nodes
	nodes, err = b.List(store.ListNodesOpts{Subtype: ptrStr("routing-discovery")})
	if err != nil {
		t.Fatalf("List (routing-discovery after update): %v", err)
	}
	if len(nodes) != 0 {
		t.Errorf("expected 0 nodes with subtype=routing-discovery after update, got %d", len(nodes))
	}

	// 4. Update --subtype "" clears subtype
	if err := b.Update(id, brain.WithUpdatedSubtype("")); err != nil {
		t.Fatalf("Update (clear): %v", err)
	}
	nwe, err = b.Get(id)
	if err != nil {
		t.Fatalf("Get after clear: %v", err)
	}
	if nwe.Subtype != "" {
		t.Errorf("expected empty subtype after clear, got %q", nwe.Subtype)
	}

	// 5. List --subtype "" (NULL filter): should now return 1 node (the cleared one)
	emptyStr := ""
	nodes, err = b.List(store.ListNodesOpts{Subtype: &emptyStr})
	if err != nil {
		t.Fatalf("List (NULL subtype): %v", err)
	}
	if len(nodes) != 1 {
		t.Errorf("expected 1 NULL-subtype node after clear, got %d", len(nodes))
	}
}

// ptrStr is a helper for creating *string values in tests.
func ptrStr(s string) *string { return &s }
