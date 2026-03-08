package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/coetzeevs/cerebro/internal/store"
	"github.com/spf13/cobra"
)

var gcThresholdFlag float64
var gcDryRunFlag bool
var gcLogFlag string

func init() {
	cmd := &cobra.Command{
		Use:   "gc",
		Short: "Evict decayed memories to archive",
		RunE:  runGC,
	}
	cmd.Flags().Float64Var(&gcThresholdFlag, "threshold", 0.01, "Eviction threshold (nodes below this score are evicted)")
	cmd.Flags().BoolVar(&gcDryRunFlag, "dry-run", false, "Show what would be evicted without actually evicting")
	cmd.Flags().StringVar(&gcLogFlag, "log", "", "Write eviction log to file (appends JSON lines)")
	rootCmd.AddCommand(cmd)
}

func runGC(cmd *cobra.Command, args []string) error {
	b, err := openBrain()
	if err != nil {
		return err
	}
	defer func() { _ = b.Close() }()

	result, err := b.GC(gcThresholdFlag, gcDryRunFlag)
	if err != nil {
		return err
	}

	// Write eviction log if --log is set and there are evictions
	if gcLogFlag != "" && len(result.Evicted) > 0 {
		if err := writeEvictionLog(gcLogFlag, result); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to write eviction log: %v\n", err)
		}
	}

	if formatFlag == "json" {
		outputJSON(result)
		return nil
	}

	if quietFlag {
		return nil
	}

	fmt.Printf("GC: evaluated %d nodes, threshold=%.4f\n", result.Evaluated, gcThresholdFlag)
	if gcDryRunFlag {
		fmt.Println("(dry-run mode — no changes made)")
	}
	if result.Archived == 0 {
		fmt.Println("No nodes evicted.")
	} else {
		fmt.Printf("Archived %d nodes:\n", result.Archived)
		for _, e := range result.Evicted {
			preview := e.Content
			if len(preview) > 80 {
				preview = preview[:80] + "..."
			}
			fmt.Printf("  - %s [%s] imp=%.2f score=%.4f: %s\n",
				e.ID[:8], e.Type, e.Importance, e.RetentionScore, preview)
		}
	}
	return nil
}

func writeEvictionLog(logPath string, result *store.GCResult) error {
	if err := os.MkdirAll(filepath.Dir(logPath), 0o750); err != nil {
		return err
	}
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644) //nolint:gosec // log path from CLI flag
	if err != nil {
		return err
	}
	defer f.Close() //nolint:errcheck

	entry := struct {
		Timestamp string                `json:"timestamp"`
		Threshold float64               `json:"threshold"`
		DryRun    bool                  `json:"dry_run"`
		Evicted   []store.GCEvictedNode `json:"evicted"`
	}{
		Timestamp: time.Now().Format(time.RFC3339),
		Threshold: gcThresholdFlag,
		DryRun:    gcDryRunFlag,
		Evicted:   result.Evicted,
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(f, "%s\n", data)
	return err
}
