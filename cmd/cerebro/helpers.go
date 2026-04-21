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
