package main

// cmd_inbox.go — `cerebro inbox` (agentic-m8m3): capture-with-approval.
// The agent PROPOSES candidate memories (`inbox add`, e.g. from a SessionEnd
// skill pass); a human or agent then curates (`inbox list/approve/discard`).
// Candidates live outside the nodes table — invisible to every retrieval
// surface until approved. Cerebro never auto-commits facts mined from
// transcripts (Model B; low-level traces transfer badly).

import (
	"fmt"
	"strings"

	"github.com/coetzeevs/cerebro/brain"
	"github.com/coetzeevs/cerebro/internal/store"
	"github.com/spf13/cobra"
)

var inboxTypeFlag string
var inboxImportanceFlag float64
var inboxSubtypeFlag string

// inboxOriginFlags carries --origin-* overrides for `inbox add`.
var inboxOriginFlags originFlags

func init() {
	inboxCmd := &cobra.Command{
		Use:   "inbox",
		Short: "Quarantine inbox: propose, review, approve, or discard candidate memories",
	}

	addCmd := &cobra.Command{
		Use:   "add <content>",
		Short: "Propose a candidate memory (quarantined until approved)",
		Args:  cobra.MinimumNArgs(1),
		RunE:  runInboxAdd,
	}
	addCmd.Flags().StringVarP(&inboxTypeFlag, "type", "t", "episode", "Memory type: episode, concept, procedure, reflection")
	addCmd.Flags().Float64VarP(&inboxImportanceFlag, "importance", "i", 0.5, "Importance score (0.0-1.0)")
	addCmd.Flags().StringVar(&inboxSubtypeFlag, "subtype", "", "Memory subtype")
	inboxOriginFlags.register(addCmd)

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List quarantined candidates (oldest first)",
		Args:  cobra.NoArgs,
		RunE:  runInboxList,
	}

	approveCmd := &cobra.Command{
		Use:   "approve <id>",
		Short: "Approve a candidate: it becomes a real, indexed, embedded memory",
		Args:  cobra.ExactArgs(1),
		RunE:  runInboxApprove,
	}

	discardCmd := &cobra.Command{
		Use:   "discard <id>",
		Short: "Discard a candidate (it never becomes a memory)",
		Args:  cobra.ExactArgs(1),
		RunE:  runInboxDiscard,
	}

	inboxCmd.AddCommand(addCmd, listCmd, approveCmd, discardCmd)
	rootCmd.AddCommand(inboxCmd)
}

func runInboxAdd(cmd *cobra.Command, args []string) error {
	nodeType, err := parseNodeType(inboxTypeFlag)
	if err != nil {
		return err
	}
	b, err := openBrain()
	if err != nil {
		return err
	}
	defer func() { _ = b.Close() }()

	opts := []brain.AddOption{brain.WithImportance(inboxImportanceFlag), inboxOriginFlags.option(cmd)}
	if inboxSubtypeFlag != "" {
		opts = append(opts, brain.WithSubtype(inboxSubtypeFlag))
	}
	id, err := b.AddCandidate(strings.Join(args, " "), store.NodeType(nodeType), opts...)
	if err != nil {
		return err
	}
	if formatFlag == "json" {
		outputJSON(map[string]string{"id": id, "status": "candidate"})
	} else if !quietFlag {
		fmt.Println(id)
	}
	return nil
}

func runInboxList(cmd *cobra.Command, args []string) error {
	b, err := openBrain()
	if err != nil {
		return err
	}
	defer func() { _ = b.Close() }()

	list, err := b.ListCandidates()
	if err != nil {
		return err
	}
	if formatFlag == "json" {
		outputJSON(list)
		return nil
	}
	if len(list) == 0 {
		if !quietFlag {
			fmt.Println("Inbox empty.")
		}
		return nil
	}
	for i := range list {
		n := &list[i]
		preview := n.Content
		if len(preview) > 80 {
			preview = preview[:80] + "…"
		}
		actor := n.OriginActor
		if actor == "" {
			actor = "?"
		}
		fmt.Printf("  %s  %s  [%s]  by %s  %s\n", n.ID[:8], n.CreatedAt.Format("2006-01-02"), n.Type, actor, preview)
	}
	fmt.Printf("%d candidate(s). Approve: cerebro inbox approve <id> | Discard: cerebro inbox discard <id>\n", len(list))
	return nil
}

func runInboxApprove(cmd *cobra.Command, args []string) error {
	b, err := openBrain()
	if err != nil {
		return err
	}
	defer func() { _ = b.Close() }()
	if err := b.ApproveCandidate(args[0]); err != nil {
		return err
	}
	if formatFlag == "json" {
		outputJSON(map[string]string{"id": args[0], "status": "active"})
	} else if !quietFlag {
		fmt.Println("approved", args[0])
	}
	return nil
}

func runInboxDiscard(cmd *cobra.Command, args []string) error {
	b, err := openBrain()
	if err != nil {
		return err
	}
	defer func() { _ = b.Close() }()
	if err := b.DiscardCandidate(args[0]); err != nil {
		return err
	}
	if formatFlag == "json" {
		outputJSON(map[string]string{"id": args[0], "status": "discarded"})
	} else if !quietFlag {
		fmt.Println("discarded", args[0])
	}
	return nil
}
