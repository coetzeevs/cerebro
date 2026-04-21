package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var backupOutputFlag string

func init() {
	cmd := &cobra.Command{
		Use:   "backup",
		Short: "Create a backup of the brain database",
		Long: `Create a timestamped copy of the brain database.

By default, backups are saved to ~/.cerebro/backups/.
Use --output to specify a custom path.`,
		RunE: runBackup,
	}
	cmd.Flags().StringVarP(&backupOutputFlag, "output", "o", "", "Output file path (default: ~/.cerebro/backups/<hash>_<timestamp>.sqlite)")
	rootCmd.AddCommand(cmd)
}

func runBackup(_ *cobra.Command, _ []string) error {
	brainPath := resolveBrainPath()

	// Verify brain exists
	if _, err := os.Stat(brainPath); os.IsNotExist(err) {
		return fmt.Errorf("brain not found at %s — run 'cerebro init' first", brainPath)
	}

	backupsDir := defaultBackupsDir()
	if backupOutputFlag != "" {
		// Use the explicit output path directly
		backupsDir = ""
	}

	var backupPath string
	var err error

	if backupOutputFlag != "" {
		// Direct file copy to specified path
		backupPath, err = backupBrainTo(brainPath, backupOutputFlag)
	} else {
		backupPath, err = backupBrain(brainPath, backupsDir)
	}

	if err != nil {
		return err
	}

	if !quietFlag {
		fmt.Fprintf(os.Stderr, "Backed up to %s\n", backupPath)
	}
	return nil
}
