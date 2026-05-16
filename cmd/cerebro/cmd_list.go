package main

import (
	"github.com/coetzeevs/cerebro/internal/store"
	"github.com/spf13/cobra"
)

var listTypeFlag string
var listStatusFlag string
var listLimitFlag int
var listSubtypeFlag string

func init() {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List memory nodes with optional filters",
		RunE:  runList,
	}
	cmd.Flags().StringVarP(&listTypeFlag, "type", "t", "", "Filter by type: episode, concept, procedure, reflection")
	cmd.Flags().StringVarP(&listStatusFlag, "status", "s", "", "Filter by status: active, consolidated, superseded")
	cmd.Flags().IntVarP(&listLimitFlag, "limit", "l", 0, "Maximum results (0 = unlimited)")
	cmd.Flags().StringVar(&listSubtypeFlag, "subtype", "",
		`Filter by subtype. Use --subtype "" to list only nodes with no subtype (NULL).
Non-empty value performs exact match. Omit the flag to return all nodes regardless of subtype.
NOTE: --subtype "" on list *filters* for NULL-subtype nodes. On update, --subtype "" *clears* the subtype.`)
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
	// --subtype uses Changed() to distinguish "not provided" (no filter) from
	// "provided as empty string" (filter for NULL-subtype rows).
	if cmd.Flags().Changed("subtype") {
		opts.Subtype = &listSubtypeFlag
	}

	nodes, err := b.List(opts)
	if err != nil {
		return err
	}

	outputNodeList(nodes)
	return nil
}
