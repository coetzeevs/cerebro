package main

import (
	"fmt"
	"strings"

	"github.com/coetzeevs/cerebro/brain"
	"github.com/coetzeevs/cerebro/internal/store"
	"github.com/spf13/cobra"
)

var addTypeFlag string
var addImportanceFlag float64
var addSubtypeFlag string

func init() {
	cmd := &cobra.Command{
		Use:   "add <content>",
		Short: "Store a new memory node",
		Args:  cobra.MinimumNArgs(1),
		RunE:  runAdd,
	}
	cmd.Flags().StringVarP(&addTypeFlag, "type", "t", "episode", "Memory type: episode, concept, procedure, reflection")
	cmd.Flags().Float64VarP(&addImportanceFlag, "importance", "i", 0.5, "Importance score (0.0-1.0)")
	cmd.Flags().StringVar(&addSubtypeFlag, "subtype", "", "Memory subtype")
	rootCmd.AddCommand(cmd)
}

func runAdd(cmd *cobra.Command, args []string) error {
	nodeType, err := parseNodeType(addTypeFlag)
	if err != nil {
		return err
	}

	content := strings.Join(args, " ")

	b, err := openBrain()
	if err != nil {
		return err
	}
	defer b.Close()

	var opts []brain.AddOption
	opts = append(opts, brain.WithImportance(addImportanceFlag))
	if addSubtypeFlag != "" {
		opts = append(opts, brain.WithSubtype(addSubtypeFlag))
	}

	id, err := b.Add(content, store.NodeType(nodeType), opts...)
	if err != nil {
		return err
	}

	if formatFlag == "json" {
		outputJSON(map[string]string{"id": id})
	} else if !quietFlag {
		fmt.Println(id)
	}

	return nil
}
