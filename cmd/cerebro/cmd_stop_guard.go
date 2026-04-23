package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
)

func init() {
	cmd := &cobra.Command{
		Use:   "stop-guard",
		Short: "Evaluate whether Claude should be allowed to stop",
		Long: `Reads Claude Code Stop hook JSON from stdin, checks the last assistant
message for patterns indicating premature stopping, and outputs a JSON
decision to stdout.

Designed to run as a Stop hook command in .claude/settings.json.
Uses exit 0 + JSON decision protocol (omits decision field to allow stopping).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := evalStopGuard(os.Stdin, os.Stdout)
			return err
		},
		SilenceUsage: true,
	}
	rootCmd.AddCommand(cmd)
}

// stopHookInput is the JSON structure Claude Code passes to Stop hooks.
type stopHookInput struct {
	LastAssistantMessage string `json:"last_assistant_message"`
	StopHookActive       bool   `json:"stop_hook_active"`
}

// stopHookDecision is the JSON output format for Stop hooks.
type stopHookDecision struct {
	Decision string `json:"decision,omitempty"`
	Reason   string `json:"reason,omitempty"`
}

// phraseCategory groups related premature-stop patterns with a specific
// corrective reason. Each category addresses a distinct behavioral failure mode.
type phraseCategory struct {
	Name    string
	Pattern *regexp.Regexp
	Reason  string
}

var stopCategories = []phraseCategory{
	{
		Name:    "permission-seeking",
		Pattern: regexp.MustCompile(`(?i)\bshall I\b|\bwould you like me to\b|\blet me know if\b`),
		Reason:  "You asked for permission instead of acting. Complete the work directly. Only stop when the original request is fully addressed.",
	},
	{
		Name:    "premature-stopping",
		Pattern: regexp.MustCompile(`(?i)\bI can stop here\b|\bI'll leave it\b|\bthat should be sufficient\b`),
		Reason:  "You declared done without verification. Re-read the original request, check your work addresses every part of it, then either finish the remaining work or explain specifically what is complete and what is not.",
	},
	{
		Name:    "scope-reduction",
		Pattern: regexp.MustCompile(`(?i)\bbeyond the scope\b|\bout of scope\b|\bas a future\b|\bfor now,`),
		Reason:  "You reduced scope that was not yours to reduce. The user defines scope. Complete the original request as stated, or ask the user if they want to reduce scope.",
	},
}

// evalStopGuard reads hook input from r, evaluates the last assistant message
// against known premature-stop patterns, and writes a JSON decision to w.
// Returns the matched category name (empty string if no match / allowed).
func evalStopGuard(r io.Reader, w io.Writer) (string, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return "", fmt.Errorf("reading stdin: %w", err)
	}

	// Parse input — gracefully handle empty or malformed JSON.
	var input stopHookInput
	if len(data) > 0 {
		_ = json.Unmarshal(data, &input) // ignore parse errors; empty message = allow
	}

	msg := strings.TrimSpace(input.LastAssistantMessage)

	// Check each category in order; first match wins.
	for _, cat := range stopCategories {
		if msg != "" && cat.Pattern.MatchString(msg) {
			decision := stopHookDecision{
				Decision: "block",
				Reason:   cat.Reason,
			}
			return cat.Name, json.NewEncoder(w).Encode(decision)
		}
	}

	// No match — allow stopping by omitting the decision field.
	_, err = fmt.Fprintln(w, "{}")
	return "", err
}
