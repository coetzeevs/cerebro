package main

// cmd_anchor_test.go — agentic-k8an (TDD): cite-and-verify source anchors.
// The Copilot-Memory pattern: a memory optionally cites the source that
// proved it (file path + content hash); recall re-verifies the anchor so the
// agent knows whether the citation still holds — the field's only
// curator-free staleness answer, and the verifiable-external-pointer base
// the poisoning literature demands (OWASP ASI06).

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/coetzeevs/cerebro/internal/store"
)

func resetAnchorFlags(t *testing.T) {
	t.Helper()
	oldA, oldR := addAnchorFlag, addAnchorRefFlag
	addAnchorFlag, addAnchorRefFlag = "", ""
	t.Cleanup(func() { addAnchorFlag, addAnchorRefFlag = oldA, oldR })
}

func TestAddAnchor_WritesPathAndHash(t *testing.T) {
	projectDir := setupAddTest(t)
	resetAnchorFlags(t)
	src := filepath.Join(projectDir, "docs", "adr-001.md")
	if err := os.MkdirAll(filepath.Dir(src), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(src, []byte("Decision: use SQLite.\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	rootCmd.SetArgs([]string{"add", "we use sqlite", "--anchor", "docs/adr-001.md"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("add --anchor: %v", err)
	}

	nodes := listNodesForProject(t, projectDir)
	var meta struct {
		Anchor struct {
			Path   string `json:"path"`
			SHA256 string `json:"sha256"`
		} `json:"anchor"`
	}
	found := false
	for _, n := range nodes {
		if n.Content != "we use sqlite" || n.Metadata == nil {
			continue
		}
		if err := json.Unmarshal(n.Metadata, &meta); err == nil && meta.Anchor.Path != "" {
			found = true
		}
	}
	if !found {
		t.Fatal("anchor metadata not written")
	}
	if meta.Anchor.Path != "docs/adr-001.md" {
		t.Errorf("anchor path: got %q (relative paths stored as given for portability)", meta.Anchor.Path)
	}
	if len(meta.Anchor.SHA256) != 64 {
		t.Errorf("anchor sha256 malformed: %q", meta.Anchor.SHA256)
	}
}

func TestAddAnchor_MissingFileFails(t *testing.T) {
	setupAddTest(t)
	resetAnchorFlags(t)
	rootCmd.SetArgs([]string{"add", "cites nothing real", "--anchor", "docs/absent.md"})
	if err := rootCmd.Execute(); err == nil {
		t.Fatal("anchoring to a nonexistent file must fail loudly, not record a dead citation")
	}
}

func TestAnchorStatus_VerifiedStaleMissing(t *testing.T) {
	projectDir := t.TempDir()
	src := filepath.Join(projectDir, "ref.md")
	if err := os.WriteFile(src, []byte("original content"), 0o600); err != nil {
		t.Fatal(err)
	}
	meta := anchorMetadata(projectDir, "ref.md", "")
	node := makeAnchoredNode(t, meta)

	if got := anchorStatusFor(node, projectDir); got != "verified" {
		t.Errorf("untouched anchor: got %q want verified", got)
	}
	if err := os.WriteFile(src, []byte("content has changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := anchorStatusFor(node, projectDir); got != "stale" {
		t.Errorf("modified anchor: got %q want stale", got)
	}
	if err := os.Remove(src); err != nil {
		t.Fatal(err)
	}
	if got := anchorStatusFor(node, projectDir); got != "missing" {
		t.Errorf("removed anchor: got %q want missing", got)
	}
}

func TestAnchorStatus_NoAnchorIsEmpty(t *testing.T) {
	node := makeAnchoredNode(t, nil)
	if got := anchorStatusFor(node, t.TempDir()); got != "" {
		t.Errorf("anchorless node: got %q want empty", got)
	}
}

// makeAnchoredNode builds a bare store.Node carrying the given metadata map.
func makeAnchoredNode(t *testing.T, meta map[string]any) *store.Node {
	t.Helper()
	n := &store.Node{ID: "test-node", Content: "x"}
	if meta != nil {
		data, err := json.Marshal(meta)
		if err != nil {
			t.Fatal(err)
		}
		n.Metadata = data
	}
	return n
}

// End-to-end: an anchored memory surfaces anchor_status through get JSON.
func TestGetSurfacesAnchorStatus(t *testing.T) {
	projectDir := setupAddTest(t)
	resetAnchorFlags(t)
	src := filepath.Join(projectDir, "proof.md")
	if err := os.WriteFile(src, []byte("the proof"), 0o600); err != nil {
		t.Fatal(err)
	}
	rootCmd.SetArgs([]string{"add", "anchored memory", "--anchor", "proof.md"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("add: %v", err)
	}
	var id string
	for _, n := range listNodesForProject(t, projectDir) {
		if n.Content == "anchored memory" {
			id = n.ID
		}
	}
	out := captureStdout(t, func() {
		oldF := formatFlag
		formatFlag = "json"
		t.Cleanup(func() { formatFlag = oldF })
		rootCmd.SetArgs([]string{"get", id, "--format", "json"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("get: %v", err)
		}
	})
	var payload map[string]any
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("get output not JSON: %v", err)
	}
	if payload["anchor_status"] != "verified" {
		t.Errorf("anchor_status: got %v want verified", payload["anchor_status"])
	}
}
