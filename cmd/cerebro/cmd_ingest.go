package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/coetzeevs/cerebro/internal/metrics"
	"github.com/spf13/cobra"
)

var ingestForceFlag bool
var ingestSessionDirFlag string

func init() {
	cmd := &cobra.Command{
		Use:   "ingest",
		Short: "Parse Claude Code session files and collect performance metrics",
		Long: `Discovers Claude Code JSONL session files for the current project,
parses them incrementally, and populates the metrics database with
per-turn performance data (tool usage, token consumption, thinking depth).

The metrics database is stored alongside the brain at
~/.cerebro/projects/<hash>.metrics.sqlite.

Incremental by default: only new lines since the last ingest are parsed.
Use --force to re-parse all files from the beginning.`,
		RunE: runIngest,
	}
	cmd.Flags().BoolVar(&ingestForceFlag, "force", false, "Re-parse all files from the beginning")
	cmd.Flags().StringVar(&ingestSessionDirFlag, "session-dir", "", "Override JSONL directory (auto-detected from project)")
	rootCmd.AddCommand(cmd)
}

func runIngest(cmd *cobra.Command, args []string) error {
	projectDir := resolveProjectDir()

	// Open or create the metrics database.
	metricsPath := metrics.MetricsPath(projectDir)
	store, err := metrics.OpenOrInit(metricsPath)
	if err != nil {
		return fmt.Errorf("opening metrics database: %w", err)
	}
	defer func() { _ = store.Close() }()

	// Discover JSONL session files.
	sessionDir := ingestSessionDirFlag
	if sessionDir == "" {
		sessionDir = claudeSessionDir(projectDir)
	}

	files, err := filepath.Glob(filepath.Join(sessionDir, "*.jsonl"))
	if err != nil {
		return fmt.Errorf("discovering session files: %w", err)
	}

	if len(files) == 0 {
		if !quietFlag {
			fmt.Printf("No session files found in %s\n", sessionDir)
		}
		return nil
	}

	var totalTurns, totalToolCalls, filesProcessed int

	for _, file := range files {
		// Check if file has grown since last ingest.
		info, err := os.Stat(file)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: cannot stat %s: %v\n", filepath.Base(file), err)
			continue
		}
		if !ingestForceFlag {
			state, err := store.GetIngestState(file)
			if err != nil {
				return fmt.Errorf("reading ingest state for %s: %w", file, err)
			}
			if state.LastOffset >= info.Size() {
				continue // file hasn't grown since last ingest
			}
		}

		// Always parse from beginning for correct turn numbering.
		result, newOffset, err := metrics.ParseJSONL(file)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to parse %s: %v\n", filepath.Base(file), err)
			continue
		}

		if len(result.Turns) == 0 {
			continue
		}

		// Delete existing tool_calls for sessions in this file before re-inserting.
		for _, sid := range result.SessionIDs() {
			if err := store.DeleteToolCallsForSession(sid); err != nil {
				return fmt.Errorf("clearing tool calls for session %s: %w", sid[:8], err)
			}
		}

		// INSERT OR REPLACE turn metrics (latest parse wins for growing sessions).
		if err := store.InsertTurnMetrics(result.Turns); err != nil {
			return fmt.Errorf("inserting turn metrics: %w", err)
		}

		// Insert fresh tool call details.
		if len(result.ToolCalls) > 0 {
			if err := store.InsertToolCalls(result.ToolCalls); err != nil {
				return fmt.Errorf("inserting tool calls: %w", err)
			}
		}

		// Update ingest state to current file size.
		if err := store.SetIngestState(file, newOffset); err != nil {
			return fmt.Errorf("updating ingest state: %w", err)
		}

		totalTurns += len(result.Turns)
		totalToolCalls += len(result.ToolCalls)
		filesProcessed++
	}

	if formatFlag == "json" {
		outputJSON(map[string]any{
			"files_processed": filesProcessed,
			"turns_ingested":  totalTurns,
			"tool_calls":      totalToolCalls,
			"metrics_path":    metricsPath,
		})
		return nil
	}

	if !quietFlag {
		fmt.Printf("Ingested %d turns and %d tool calls from %d files\n", totalTurns, totalToolCalls, filesProcessed)
	}

	return nil
}

// claudeSessionDir returns the Claude Code session directory for a project.
// Claude Code stores sessions at ~/.claude/projects/-<encoded-path>/*.jsonl
// where the encoded path replaces / with - and prepends -.
func claudeSessionDir(projectDir string) string {
	abs, _ := filepath.Abs(projectDir)
	encoded := "-" + strings.ReplaceAll(abs[1:], string(filepath.Separator), "-")
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude", "projects", encoded)
}
