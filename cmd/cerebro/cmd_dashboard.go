package main

import (
	"fmt"
	"os"
	"path/filepath"

	tea "charm.land/bubbletea/v2"
	"github.com/coetzeevs/cerebro/internal/dashboard"
	"github.com/coetzeevs/cerebro/internal/metrics"
	"github.com/spf13/cobra"
)

var dashboardLastFlag int

func init() {
	cmd := &cobra.Command{
		Use:   "dashboard",
		Short: "Interactive performance metrics dashboard",
		Long: `Launch a full-screen interactive dashboard showing per-turn
performance metrics, quality signals, and brain health.

The dashboard auto-refreshes every 5 seconds by re-parsing Claude Code
session files for new data. Press 'r' to force a manual refresh.

Panel navigation: 1-5 or Tab/Shift+Tab. Press 'q' to quit.`,
		RunE: runDashboard,
	}
	cmd.Flags().IntVar(&dashboardLastFlag, "last", 200, "Number of recent turns to display")
	rootCmd.AddCommand(cmd)
}

func runDashboard(cmd *cobra.Command, args []string) error {
	projectDir := resolveProjectDir()

	// Open metrics DB (read-write for live ingest).
	metricsPath := metrics.MetricsPath(projectDir)
	ms, err := metrics.OpenOrInit(metricsPath)
	if err != nil {
		return fmt.Errorf("opening metrics database: %w", err)
	}
	defer func() { _ = ms.Close() }()

	// Open brain DB (read-only for health queries).
	cfg := dashboard.Config{
		MetricsStore: ms,
		SessionDir:   claudeSessionDir(projectDir),
		LastN:        dashboardLastFlag,
	}

	b, brainErr := openBrain()
	if brainErr == nil {
		cfg.BrainDB = b.Store().DB()
		defer func() { _ = b.Close() }()
	}

	// Run initial ingest to ensure data is current.
	if !quietFlag {
		fmt.Print("Ingesting session data...")
	}
	ingestForDashboard(ms, cfg.SessionDir)
	if !quietFlag {
		fmt.Println(" done.")
	}

	// Launch Bubble Tea program.
	p := tea.NewProgram(dashboard.New(cfg))
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("dashboard error: %w", err)
	}

	return nil
}

// ingestForDashboard runs a quick ingest before dashboard launch.
func ingestForDashboard(store *metrics.MetricsStore, sessionDir string) {
	files, err := filepath.Glob(filepath.Join(sessionDir, "*.jsonl"))
	if err != nil || len(files) == 0 {
		return
	}
	for _, file := range files {
		info, err := os.Stat(file)
		if err != nil {
			continue
		}
		state, _ := store.GetIngestState(file)
		if state.LastOffset >= info.Size() {
			continue
		}
		result, newOffset, err := metrics.ParseJSONL(file)
		if err != nil || len(result.Turns) == 0 {
			continue
		}
		for _, sid := range result.SessionIDs() {
			_ = store.DeleteToolCallsForSession(sid)
		}
		_ = store.InsertTurnMetrics(result.Turns)
		if len(result.ToolCalls) > 0 {
			_ = store.InsertToolCalls(result.ToolCalls)
		}
		_ = store.SetIngestState(file, newOffset)
	}
}
