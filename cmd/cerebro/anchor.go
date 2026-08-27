package main

// anchor.go — cite-and-verify source anchors (agentic-k8an).
//
// A memory optionally cites the source that proved it: a file path (stored
// as given — relative paths stay portable across machines sharing a repo
// layout) plus the sha256 of the file's content at write time, and an
// optional ref label (e.g. a commit SHA). Recall re-verifies cheaply:
//
//	verified — the anchor resolves and its content hash still matches
//	stale    — the file exists but its content changed since citation
//	missing  — the file is gone
//	(empty)  — the memory carries no anchor
//
// The status is computed at read time and surfaced beside provenance_status
// and origin_status; nothing is stored beyond the citation itself. Anchors
// live in the node's metadata JSON under the reserved "anchor" key — no
// schema change, and export/import carry them for free.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/coetzeevs/cerebro/internal/store"
)

// nodeAnchor is the reserved metadata shape under the "anchor" key.
type nodeAnchor struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Ref    string `json:"ref,omitempty"`
}

// hashFile returns the sha256 hex of a file's content.
func hashFile(path string) (string, error) {
	data, err := os.ReadFile(path) //nolint:gosec // operator-supplied anchor path
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

// resolveAnchorPath resolves a stored anchor path against the project dir
// (absolute paths pass through).
func resolveAnchorPath(anchorPath, projectDir string) string {
	if filepath.IsAbs(anchorPath) {
		return anchorPath
	}
	return filepath.Join(projectDir, anchorPath)
}

// anchorMetadata builds the metadata map for an anchor, hashing the file NOW
// (used at write time; the caller has already validated existence).
func anchorMetadata(projectDir, anchorPath, ref string) map[string]any {
	hash, err := hashFile(resolveAnchorPath(anchorPath, projectDir))
	if err != nil {
		return nil
	}
	a := nodeAnchor{Path: anchorPath, SHA256: hash, Ref: ref}
	return map[string]any{"anchor": a}
}

// parseAnchor extracts the anchor from a node's metadata; nil when absent.
func parseAnchor(metadata json.RawMessage) *nodeAnchor {
	if len(metadata) == 0 {
		return nil
	}
	var wrapper struct {
		Anchor *nodeAnchor `json:"anchor"`
	}
	if err := json.Unmarshal(metadata, &wrapper); err != nil || wrapper.Anchor == nil || wrapper.Anchor.Path == "" {
		return nil
	}
	return wrapper.Anchor
}

// anchorStatusFor re-verifies a node's anchor: verified | stale | missing,
// or "" for anchorless nodes. Best-effort and read-only — an unreadable
// file reports missing rather than erroring the recall.
func anchorStatusFor(n *store.Node, projectDir string) string {
	a := parseAnchor(n.Metadata)
	if a == nil {
		return ""
	}
	hash, err := hashFile(resolveAnchorPath(a.Path, projectDir))
	if err != nil {
		return "missing"
	}
	if hash != a.SHA256 {
		return "stale"
	}
	return "verified"
}
