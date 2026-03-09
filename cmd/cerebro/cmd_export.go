package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var exportOutputFlag string

func init() {
	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export brain contents",
		Long: `Export the brain as JSON, SQL statements, or a raw SQLite file copy.

Formats:
  json    Full JSON dump (default, written to stdout unless --output is set)
  sql     SQL INSERT statements (written to stdout unless --output is set)
  sqlite  Raw database file copy (requires --output)`,
		RunE: runExport,
	}
	cmd.Flags().StringVarP(&exportOutputFlag, "output", "o", "", "Output file path (required for sqlite format)")
	rootCmd.AddCommand(cmd)
}

func runExport(cmd *cobra.Command, _ []string) error {
	b, err := openBrain()
	if err != nil {
		return err
	}
	defer func() { _ = b.Close() }()

	switch formatFlag {
	case "sqlite":
		if exportOutputFlag == "" {
			return fmt.Errorf("--output is required for sqlite format")
		}
		if err := b.ExportSQLite(exportOutputFlag); err != nil {
			return err
		}
		if !quietFlag {
			fmt.Fprintf(os.Stderr, "Exported to %s\n", exportOutputFlag)
		}
		return nil

	case "sql":
		w := os.Stdout
		if exportOutputFlag != "" {
			f, err := os.Create(exportOutputFlag) //nolint:gosec // user-specified export path
			if err != nil {
				return fmt.Errorf("creating output file: %w", err)
			}
			defer f.Close() //nolint:errcheck // best-effort cleanup
			w = f
		}
		if err := b.ExportSQL(w); err != nil {
			return err
		}
		if exportOutputFlag != "" && !quietFlag {
			fmt.Fprintf(os.Stderr, "Exported SQL to %s\n", exportOutputFlag)
		}
		return nil

	default: // json
		w := os.Stdout
		if exportOutputFlag != "" {
			f, err := os.Create(exportOutputFlag) //nolint:gosec // user-specified export path
			if err != nil {
				return fmt.Errorf("creating output file: %w", err)
			}
			defer f.Close() //nolint:errcheck // best-effort cleanup
			w = f
		}
		if err := b.ExportJSON(w); err != nil {
			return err
		}
		if exportOutputFlag != "" && !quietFlag {
			fmt.Fprintf(os.Stderr, "Exported JSON to %s\n", exportOutputFlag)
		}
		return nil
	}
}
