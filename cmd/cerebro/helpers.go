package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/coetzeevs/cerebro/brain"
	"github.com/coetzeevs/cerebro/internal/store"
)

// resolveBrainPath determines the brain file path from flags or cwd.
func resolveBrainPath() string {
	dir := projectFlag
	if dir == "" {
		dir, _ = os.Getwd()
	}
	return brain.ProjectPath(dir)
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
