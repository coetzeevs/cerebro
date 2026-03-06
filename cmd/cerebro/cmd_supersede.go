package main

import (
	"fmt"
	"strings"

	"github.com/coetzeevs/cerebro/brain"
	"github.com/coetzeevs/cerebro/internal/store"
	"github.com/spf13/cobra"
)

var supersedeTypeFlag string
var supersedeImportanceFlag float64

func init() {
	cmd := &cobra.Command{
		Use:   "supersede <old-id> <new-content>",
		Short: "Mark an old memory as superseded and store a new one",
		Args:  cobra.MinimumNArgs(2),
		RunE:  runSupersede,
	}
	cmd.Flags().StringVarP(&supersedeTypeFlag, "type", "t", "concept", "Type for new memory")
	cmd.Flags().Float64VarP(&supersedeImportanceFlag, "importance", "i", 0.5, "Importance for new memory")
	rootCmd.AddCommand(cmd)
}

func runSupersede(cmd *cobra.Command, args []string) error {
	oldID := args[0]
	content := strings.Join(args[1:], " ")

	nodeType, err := parseNodeType(supersedeTypeFlag)
	if err != nil {
		return err
	}

	b, err := openBrain()
	if err != nil {
		return err
	}
	defer b.Close()

	newID, err := b.Supersede(oldID, content, store.NodeType(nodeType),
		brain.WithImportance(supersedeImportanceFlag))
	if err != nil {
		return err
	}

	if formatFlag == "json" {
		outputJSON(map[string]string{"id": newID, "superseded": oldID})
	} else if !quietFlag {
		fmt.Println(newID)
	}

	return nil
}
