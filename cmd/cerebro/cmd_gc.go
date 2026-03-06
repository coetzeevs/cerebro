package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

var gcThresholdFlag float64
var gcDryRunFlag bool

func init() {
	cmd := &cobra.Command{
		Use:   "gc",
		Short: "Evict decayed memories to archive",
		RunE:  runGC,
	}
	cmd.Flags().Float64Var(&gcThresholdFlag, "threshold", 0.01, "Eviction threshold (nodes below this score are evicted)")
	cmd.Flags().BoolVar(&gcDryRunFlag, "dry-run", false, "Show what would be evicted without actually evicting")
	rootCmd.AddCommand(cmd)
}

func runGC(cmd *cobra.Command, args []string) error {
	b, err := openBrain()
	if err != nil {
		return err
	}
	defer b.Close()

	// TODO: implement eviction logic in store/lifecycle package
	// For now, just report stats
	stats, err := b.Stats()
	if err != nil {
		return err
	}

	if !quietFlag {
		fmt.Printf("GC: %d active nodes, threshold=%.4f\n", stats.ActiveNodes, gcThresholdFlag)
		if gcDryRunFlag {
			fmt.Println("(dry-run mode — no changes made)")
		}
		fmt.Println("GC implementation pending — no nodes evicted")
	}
	return nil
}
