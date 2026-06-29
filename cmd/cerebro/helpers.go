package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/coetzeevs/cerebro/brain"
	"github.com/coetzeevs/cerebro/internal/store"
)

// resolveProjectDir determines the project directory from flag, env var, or cwd.
// Resolution order: --project flag > CLAUDE_PROJECT_DIR env var > cwd.
func resolveProjectDir() string {
	if projectFlag != "" {
		return projectFlag
	}
	if dir := os.Getenv("CLAUDE_PROJECT_DIR"); dir != "" {
		return dir
	}
	dir, _ := os.Getwd()
	return dir
}

// resolveBrainPath determines the brain file path from the resolved project directory.
func resolveBrainPath() string {
	return brain.ProjectPath(resolveProjectDir())
}

// openBrain opens the brain for the current project.
func openBrain() (*brain.Brain, error) {
	return brain.Open(resolveBrainPath())
}

// outputJSON writes v as indented JSON to stdout.
func outputJSON(v any) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

// maxProvenanceDepth is the documented upper bound on a provenance-walk depth
// (Security NOTE A — defense-in-depth bound on the BFS hop budget). The BFS is
// already O(reachable nodes) by the visited set, but the clamp bounds the
// per-level batched-query count for a hostile/typo'd --provenance-depth. Any
// requested depth above this is silently clamped to it.
const maxProvenanceDepth = 100

// clampProvenanceDepth bounds a requested provenance-walk depth to
// [0, maxProvenanceDepth]. Negative values are passed through to WalkRelation
// (which treats <=0 as "start node only").
func clampProvenanceDepth(depth int) int {
	if depth > maxProvenanceDepth {
		return maxProvenanceDepth
	}
	return depth
}

// provenanceChainItem is one node in a rendered provenance chain (md/json).
type provenanceChainItem struct {
	ID      string         `json:"id"`
	Type    store.NodeType `json:"type"`
	Content string         `json:"content"`
	Depth   int            `json:"depth"`
}

// toProvenanceChain converts a WalkProvenance result (which includes the start
// node at depth 0) into the rendered chain of SOURCES — the start node is
// dropped, only depth>=1 lineage nodes remain.
func toProvenanceChain(walk []store.NodeWithDepth) []provenanceChainItem {
	chain := make([]provenanceChainItem, 0, len(walk))
	for i := range walk {
		if walk[i].Depth == 0 {
			continue // skip the start node itself
		}
		chain = append(chain, provenanceChainItem{
			ID:      walk[i].ID,
			Type:    walk[i].Type,
			Content: walk[i].Content,
			Depth:   walk[i].Depth,
		})
	}
	return chain
}

// renderProvenanceChainMD prints a "## Provenance (depth N)" block to stdout.
// Only called when --with-provenance is engaged, so flag-absent output is
// byte-identical to the pre-lbjg path.
func renderProvenanceChainMD(chain []provenanceChainItem, depth int) {
	fmt.Printf("## Provenance (depth %d)\n", depth)
	if len(chain) == 0 {
		fmt.Printf("  (no recorded provenance)\n\n")
		return
	}
	for i := range chain {
		c := &chain[i]
		fmt.Printf("  %s [%s] depth=%d\n", c.ID[:8], c.Type, c.Depth)
	}
	fmt.Println()
}

// outputNode formats a node for display.
func outputNode(n *store.Node) {
	if formatFlag == "json" {
		outputJSON(n)
		return
	}
	fmt.Printf("## %s [%s/%s] (importance: %.2f)\n", n.ID[:8], n.Type, n.Status, n.Importance)
	fmt.Printf("%s\n\n", n.Content)
}

// outputScoredNode formats a scored node for display.
func outputScoredNode(n *store.ScoredNode) {
	if formatFlag == "json" {
		outputJSON(n)
		return
	}
	fmt.Printf("## %s [%s] score=%.3f sim=%.3f imp=%.2f\n",
		n.ID[:8], n.Type, n.Score, n.Similarity, n.Importance)
	fmt.Printf("%s\n\n", n.Content)
}

// outputNodeList formats a list of nodes.
func outputNodeList(nodes []store.Node) {
	if formatFlag == "json" {
		outputJSON(nodes)
		return
	}
	if len(nodes) == 0 {
		fmt.Println("No memories found.")
		return
	}
	for i := range nodes {
		outputNode(&nodes[i])
	}
}

// outputScoredList formats a list of scored nodes.
func outputScoredList(nodes []store.ScoredNode) {
	if formatFlag == "json" {
		outputJSON(nodes)
		return
	}
	if len(nodes) == 0 {
		fmt.Println("No relevant memories found.")
		return
	}
	for i := range nodes {
		outputScoredNode(&nodes[i])
	}
}

// outputStats formats stats for display.
func outputStats(stats *store.Stats) {
	if formatFlag == "json" {
		outputJSON(stats)
		return
	}
	fmt.Printf("# Brain Stats\n\n")
	fmt.Printf("Schema version: %s\n", stats.SchemaVersion)
	fmt.Printf("Embedding model: %s (%s dims)\n\n", stats.EmbeddingModel, stats.EmbeddingDimensions)
	fmt.Printf("## Nodes\n")
	fmt.Printf("Total: %d | Active: %d | Consolidated: %d | Superseded: %d | Archived: %d\n\n",
		stats.TotalNodes, stats.ActiveNodes, stats.ConsolidatedNodes, stats.SupersededNodes, stats.ArchivedNodes)
	fmt.Printf("## By Type (active)\n")
	for t, c := range stats.NodesByType {
		fmt.Printf("  %s: %d\n", t, c)
	}
	fmt.Printf("\nEdges: %d\n", stats.TotalEdges)
	if stats.PendingEmbeddings > 0 {
		fmt.Printf("Pending embeddings: %d\n", stats.PendingEmbeddings)
	}
}

// backupBrain creates a timestamped copy of the brain database.
// Returns the backup file path.
func backupBrain(brainPath, backupsDir string) (string, error) {
	if _, err := os.Stat(brainPath); os.IsNotExist(err) {
		return "", fmt.Errorf("brain not found at %s", brainPath)
	}

	if err := os.MkdirAll(backupsDir, 0o750); err != nil {
		return "", fmt.Errorf("creating backups directory: %w", err)
	}

	base := strings.TrimSuffix(filepath.Base(brainPath), filepath.Ext(brainPath))
	ts := time.Now().UTC().Format("20060102_150405")
	backupPath := filepath.Join(backupsDir, fmt.Sprintf("%s_%s.sqlite", base, ts))

	src, err := os.Open(brainPath)
	if err != nil {
		return "", fmt.Errorf("opening brain: %w", err)
	}
	defer src.Close() //nolint:errcheck // read-only

	dst, err := os.Create(backupPath) //nolint:gosec // backup path is derived internally
	if err != nil {
		return "", fmt.Errorf("creating backup: %w", err)
	}
	defer dst.Close() //nolint:errcheck // best-effort cleanup

	if _, err := io.Copy(dst, src); err != nil {
		return "", fmt.Errorf("copying brain: %w", err)
	}

	if err := dst.Close(); err != nil {
		return "", fmt.Errorf("finalizing backup: %w", err)
	}

	return backupPath, nil
}

// backupBrainTo creates a copy of the brain database at the specified path.
// Returns the output path.
func backupBrainTo(brainPath, outputPath string) (string, error) {
	if _, err := os.Stat(brainPath); os.IsNotExist(err) {
		return "", fmt.Errorf("brain not found at %s", brainPath)
	}

	dir := filepath.Dir(outputPath)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return "", fmt.Errorf("creating output directory: %w", err)
	}

	src, err := os.Open(brainPath)
	if err != nil {
		return "", fmt.Errorf("opening brain: %w", err)
	}
	defer src.Close() //nolint:errcheck // read-only

	dst, err := os.Create(outputPath) //nolint:gosec // user-specified output path
	if err != nil {
		return "", fmt.Errorf("creating backup: %w", err)
	}
	defer dst.Close() //nolint:errcheck // best-effort cleanup

	if _, err := io.Copy(dst, src); err != nil {
		return "", fmt.Errorf("copying brain: %w", err)
	}

	if err := dst.Close(); err != nil {
		return "", fmt.Errorf("finalizing backup: %w", err)
	}

	return outputPath, nil
}

// defaultBackupsDir returns the default backups directory (~/.cerebro/backups).
func defaultBackupsDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".cerebro", "backups")
}

// resolveID opens the brain and resolves a short ID prefix to a full UUID.
func resolveID(b *brain.Brain, prefix string) (string, error) {
	return b.ResolveID(prefix)
}

// parseAsOf parses an agent/operator-supplied time string for the --as-of /
// --valid-at / --invalid-at flags (agentic-xtzn). It accepts two layouts:
//
//   - RFC3339 (e.g. "2026-06-17T14:30:00Z" or with an offset) — the primary,
//     machine-readable form cerebro already emits elsewhere; AND
//   - date-only "2006-01-02" (e.g. "2026-06-17"), interpreted as midnight UTC
//     for ergonomics.
//
// The result is ALWAYS normalized to UTC (matching the nodes.go storage idiom),
// so callers can hand it straight to the store layer. parseAsOf returns an error
// — and NEVER panics — for empty, whitespace, garbage, partial, or out-of-range
// input (Security guardrail): a malformed flag must surface as a CLI error
// before any store write or query, never reach the DB unparsed.
func parseAsOf(s string) (time.Time, error) {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return time.Time{}, fmt.Errorf("empty time value: provide an RFC3339 timestamp (2006-01-02T15:04:05Z) or a date (2006-01-02)")
	}
	// RFC3339 first (the primary form); time.Parse validates ranges and returns
	// an error for out-of-range or malformed input — no panic.
	if t, err := time.Parse(time.RFC3339, trimmed); err == nil {
		return t.UTC(), nil
	}
	// Date-only fallback: midnight UTC.
	if t, err := time.Parse("2006-01-02", trimmed); err == nil {
		return t.UTC(), nil
	}
	return time.Time{}, fmt.Errorf(
		"invalid time %q: expected RFC3339 (2006-01-02T15:04:05Z) or date (2006-01-02)", s)
}

// parseNodeType validates and returns a NodeType.
func parseNodeType(s string) (store.NodeType, error) {
	s = strings.ToLower(s)
	switch s {
	case "episode", "concept", "procedure", "reflection":
		return store.NodeType(s), nil
	default:
		return "", fmt.Errorf("invalid type %q: must be episode, concept, procedure, or reflection", s)
	}
}
