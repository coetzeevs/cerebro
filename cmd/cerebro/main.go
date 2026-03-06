package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "cerebro",
	Short: "Persistent memory for AI agent orchestrators",
	Long: `Cerebro is a local-first, zero-infrastructure persistent memory system
for AI agent orchestrators. It combines a knowledge graph with vector
similarity search in a single SQLite file.`,
}

// Global flags
var (
	formatFlag  string
	projectFlag string
	quietFlag   bool
)

func init() {
	rootCmd.PersistentFlags().StringVarP(&formatFlag, "format", "f", "md", "Output format: md, json")
	rootCmd.PersistentFlags().StringVarP(&projectFlag, "project", "p", "", "Project directory (defaults to cwd)")
	rootCmd.PersistentFlags().BoolVarP(&quietFlag, "quiet", "q", false, "Suppress non-essential output")
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
