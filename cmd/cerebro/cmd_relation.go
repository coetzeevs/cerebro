package main

// cmd_relation.go — `cerebro relation` subcommands (agentic-8l2g).
//
// The typed-relation registry is ADVISORY: it names the relations a brain's
// edges are expected to use so that `cerebro edge` can warn (never error) on
// an unregistered relation. Removing a relation touches only the registry —
// existing edges carrying the name are untouched.

import (
	"fmt"

	"github.com/spf13/cobra"
)

var relationClassFlag string

func init() {
	relationCmd := &cobra.Command{
		Use:   "relation",
		Short: "Manage the typed-relation registry",
	}

	addCmd := &cobra.Command{
		Use:   "add <name>",
		Short: "Register a relation name (idempotent)",
		Args:  cobra.ExactArgs(1),
		RunE:  runRelationAdd,
	}
	addCmd.Flags().StringVar(&relationClassFlag, "class", "",
		"Traversal class hint for the relation (e.g. structural, topical)")

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List registered relations",
		Args:  cobra.NoArgs,
		RunE:  runRelationList,
	}

	rmCmd := &cobra.Command{
		Use:   "rm <name>",
		Short: "Remove a relation from the registry (existing edges keep it)",
		Args:  cobra.ExactArgs(1),
		RunE:  runRelationRm,
	}

	relationCmd.AddCommand(addCmd, listCmd, rmCmd)
	rootCmd.AddCommand(relationCmd)
}

func runRelationAdd(cmd *cobra.Command, args []string) error {
	b, err := openBrain()
	if err != nil {
		return err
	}
	defer func() { _ = b.Close() }()

	if err := b.RegisterRelation(args[0], relationClassFlag); err != nil {
		return err
	}
	if formatFlag == "json" {
		outputJSON(map[string]string{"name": args[0], "traversal_class": relationClassFlag})
	} else if !quietFlag {
		fmt.Println(args[0])
	}
	return nil
}

func runRelationList(cmd *cobra.Command, args []string) error {
	b, err := openBrain()
	if err != nil {
		return err
	}
	defer func() { _ = b.Close() }()

	rels, err := b.ListRelations()
	if err != nil {
		return err
	}
	if formatFlag == "json" {
		outputJSON(rels)
		return nil
	}
	for _, r := range rels {
		if r.TraversalClass != "" {
			fmt.Printf("%s\t%s\n", r.Name, r.TraversalClass)
		} else {
			fmt.Println(r.Name)
		}
	}
	return nil
}

func runRelationRm(cmd *cobra.Command, args []string) error {
	b, err := openBrain()
	if err != nil {
		return err
	}
	defer func() { _ = b.Close() }()

	if err := b.RemoveRelation(args[0]); err != nil {
		return err
	}
	if !quietFlag && formatFlag != "json" {
		fmt.Println("removed", args[0])
	}
	if formatFlag == "json" {
		outputJSON(map[string]string{"removed": args[0]})
	}
	return nil
}
