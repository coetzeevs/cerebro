package main

// cmd_forget.go — `cerebro forget --subject` (agentic-dpgh): subject-scoped
// bulk forget, distinct from gc. DRY-RUN BY DEFAULT — the command only
// mutates with an explicit --apply, because the cascade (embeddings, FTS,
// edges, and optionally the rows themselves with --hard) is irreversible.

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	forgetSubjectFlag string
	forgetSubtypeFlag string
	forgetHardFlag    bool
	forgetApplyFlag   bool
)

func init() {
	cmd := &cobra.Command{
		Use:   "forget --subject <pattern>",
		Short: "Bulk-forget memories about a subject (dry-run by default)",
		Long: `Select every non-archived memory whose content contains <pattern>
(case-insensitive; --subtype narrows) and forget it: edges, embedding, and
FTS presence are removed so nothing stays retrievable through any lane. The
node is archived for audit, or removed entirely with --hard.

Without --apply this is a DRY-RUN: it lists what would be forgotten and
writes nothing. Use case: scrubbing a subject from a brain before sharing
or handing it over.`,
		Args: cobra.NoArgs,
		RunE: runForget,
	}
	cmd.Flags().StringVar(&forgetSubjectFlag, "subject", "", "Content pattern selecting the memories to forget (required)")
	cmd.Flags().StringVar(&forgetSubtypeFlag, "subtype", "", "Narrow the match to one subtype")
	cmd.Flags().BoolVar(&forgetHardFlag, "hard", false, "Delete the rows entirely instead of archiving")
	cmd.Flags().BoolVar(&forgetApplyFlag, "apply", false, "Execute the forget (omit for a write-free dry-run)")
	_ = cmd.MarkFlagRequired("subject")
	rootCmd.AddCommand(cmd)
}

func runForget(cmd *cobra.Command, args []string) error {
	b, err := openBrain()
	if err != nil {
		return err
	}
	defer func() { _ = b.Close() }()

	res, err := b.ForgetSubject(forgetSubjectFlag, forgetSubtypeFlag, forgetHardFlag, !forgetApplyFlag)
	if err != nil {
		return err
	}

	if formatFlag == "json" {
		outputJSON(res)
		return nil
	}
	if quietFlag {
		return nil
	}
	mode := "ARCHIVE"
	if forgetHardFlag {
		mode = "HARD DELETE"
	}
	for i := range res.Matched {
		n := &res.Matched[i]
		preview := n.Content
		if len(preview) > 80 {
			preview = preview[:80] + "…"
		}
		fmt.Printf("  %s  [%s]  %s\n", n.ID[:8], n.Type, preview)
	}
	if res.DryRun {
		fmt.Printf("DRY-RUN: %d memory(ies) would be forgotten (%s). Re-run with --apply to execute.\n", len(res.Matched), mode)
		return nil
	}
	fmt.Printf("Forgot %d memory(ies) (%s): %d edge(s) and %d embedding(s) removed.\n",
		len(res.Matched), mode, res.EdgesRemoved, res.EmbeddingsRemoved)
	return nil
}
