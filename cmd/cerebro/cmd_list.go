package main

import (
	"github.com/coetzeevs/cerebro/internal/store"
	"github.com/spf13/cobra"
)

var listTypeFlag string
var listStatusFlag string
var listLimitFlag int

func init() {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List memory nodes with optional filters",
		RunE:  runList,
	}
	cmd.Flags().StringVarP(&listTypeFlag, "type", "t", "", "Filter by type: episode, concept, procedure, reflection")
	cmd.Flags().StringVarP(&listStatusFlag, "status", "s", "", "Filter by status: active, consolidated, superseded")
	cmd.Flags().IntVarP(&listLimitFlag, "limit", "l", 0, "Maximum results (0 = unlimited)")
	rootCmd.AddCommand(cmd)
}

func runList(cmd *cobra.Command, args []string) error {
	b, err := openBrain()
	if err != nil {
		return err
	}
	defer func() { _ = b.Close() }()

	opts := store.ListNodesOpts{
		Status: listStatusFlag,
		Limit:  listLimitFlag,
	}
	if listTypeFlag != "" {
		t, err := parseNodeType(listTypeFlag)
		if err != nil {
			return err
		}
		opts.Type = t
	}

	nodes, err := b.List(opts)
	if err != nil {
		return err
	}

	outputNodeList(nodes)
	return nil
}
