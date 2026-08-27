package main

// cmd_usage.go — `cerebro usage`: the at-load capability map (agentic-trko).
//
// Agents were discovering cerebro by running `--help` and per-command help
// mid-task, because no surface enumerated the capabilities up front. This
// command renders a compact situation → command map, and the CLAUDE.md
// template that `cerebro init` writes embeds the SAME rendered block (a test
// asserts byte-equality), so the map is in the agent's context from session
// load. A second test asserts every registered command appears here — a new
// command cannot ship invisible.

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// usageEntry is one capability line: the situation an agent is in, the
// command that answers it, and the one-line contract that matters.
type usageEntry struct {
	Situation string
	Command   string // the cobra command name this entry covers (drift guard)
	Line      string // rendered line: invocation + contract
}

type usageSection struct {
	Title   string
	Entries []usageEntry
}

// usageSections is the curated capability map. Keep every line SHORT — this
// block ships inside CLAUDE.md and is read at every session load.
var usageSections = []usageSection{
	{"Remember (write)", []usageEntry{
		{"learned something worth keeping", "add",
			"`cerebro add \"<content>\" -t episode|concept|procedure|reflection -i 0.0-1.0 [--subtype s]` — search first, reconcile (see supersede/update/reinforce)"},
		{"a file proves the memory", "add",
			"`cerebro add ... --anchor <path> [--anchor-ref <sha>]` — cites the source; recall reports verified|stale|missing"},
		{"new info contradicts/replaces a memory", "supersede",
			"`cerebro supersede <old-id> \"<new content>\" -t <type>` — old stays as history, new takes over"},
		{"refine an existing memory in place", "update",
			"`cerebro update <id> --content|--importance|--subtype`"},
		{"a memory proved right again (no new info)", "reinforce",
			"`cerebro reinforce <id>` — boosts retention"},
		{"unsure it belongs in the brain yet", "inbox",
			"`cerebro inbox add \"<content>\"` — quarantined until `inbox approve <id>` / `inbox discard <id>`; `inbox list` to review"},
		{"two memories are related", "edge",
			"`cerebro edge <src-id> <dst-id> <relation>` — use `cerebro relation list` names; register new ones deliberately"},
		{"curate the relation vocabulary", "relation",
			"`cerebro relation add <name> [--class c] | list | rm <name>`"},
	}},
	{"Retrieve (read)", []usageEntry{
		{"need context on a topic", "recall",
			"`cerebro recall \"<query>\"` — composite-scored, THE default retrieval; `--prime` for session-start selection"},
		{"pure semantic similarity", "search",
			"`cerebro search \"<query>\" [--limit N --threshold 0.x]` — vector lane only"},
		{"inspect one memory + its edges", "get",
			"`cerebro get <id> [--with-provenance] [--as-of <time>]` — JSON carries origin/provenance/anchor status"},
		{"browse or filter", "list",
			"`cerebro list [--type t] [--status s] [--subtype x]`"},
	}},
	{"Feedback (close the loop — do this after acting on a recall)", []usageEntry{
		{"a recalled memory helped", "outcome",
			"`cerebro outcome <id> --success` — boosts its future ranking"},
		{"a recalled memory misled you", "outcome",
			"`cerebro outcome <id> --failure` — sinks it (and consider supersede)"},
	}},
	{"Distill & maintain", []usageEntry{
		{"many episodes accumulated", "consolidate",
			"`cerebro consolidate --suggest` to see clusters; synthesize a concept with add, then `cerebro consolidate --into <new-id> <episode-ids...>` (wires provenance, marks sources)"},
		{"episodes distilled elsewhere", "mark-consolidated",
			"`cerebro mark-consolidated <ids...>` — status flip only, no provenance"},
		{"scrub a subject before sharing a brain", "forget",
			"`cerebro forget --subject \"<pattern>\" [--subtype s] [--hard]` — DRY-RUN by default; add --apply to execute"},
		{"vectors missing (import, embed failures)", "embed",
			"`cerebro embed --pending` — backfills; oversized content chunks automatically"},
		{"evict decayed memories", "gc",
			"`cerebro gc [--dry-run]` — score-based, archives"},
		{"brain health check", "stats",
			"`cerebro stats` — counts, schema, pending embeddings"},
	}},
	{"Share & lifecycle", []usageEntry{
		{"memory useful across all projects", "promote",
			"`cerebro promote <id>` — copies to the global brain with provenance"},
		{"move/backup brains", "export",
			"`cerebro export [--format json|sql|sqlite]` — full-fidelity bundle"},
		{"restore/merge a bundle", "import",
			"`cerebro import <file> [--on-conflict skip|replace]`"},
		{"snapshot before risky work", "backup",
			"`cerebro backup`"},
		{"consolidate duplicate brains / formats", "migrate",
			"`cerebro migrate --realpath-hashes [--dry-run]`"},
		{"new project setup", "init",
			"`cerebro init -p <dir>` — brain + hooks + skills + this capability map in CLAUDE.md"},
	}},
	{"Operator/infra (rarely agent-invoked)", []usageEntry{
		{"see this map again", "usage", "`cerebro usage`"},
		{"tune per-brain defaults", "config", "`cerebro config list|get|set` — thresholds, seams (e.g. indegree_bonus_enabled)"},
		{"recall-quality measurement", "eval", "`cerebro eval` — A/B protocol in docs/evals/README.md"},
		{"session metrics", "ingest", "`cerebro ingest`"},
		{"metrics dashboard", "dashboard", "`cerebro dashboard`"},
		{"lifecycle hooks (wired by init/plugin)", "hook", "`cerebro hook prime|post-compact|session-end` — session-guarded, not for manual use"},
		{"premature-stop detector (opt-in, default off)", "stop-guard", "`cerebro stop-guard` — inert unless stop_guard_enabled=true AND a Stop hook is wired"},
		{"Pi runtime config snippet", "pi-init", "`cerebro pi-init`"},
	}},
}

// renderCapabilityMap renders the map as compact markdown. This exact output
// is embedded in the CLAUDE.md template (byte-equality test) — change here,
// regenerate there with `cerebro usage --claudemd-block`.
func renderCapabilityMap() string {
	var b strings.Builder
	b.WriteString("Always pass `-p \"$CLAUDE_PROJECT_DIR\"` (EDP estates: `-p \"${EDP_BRAIN_ROOT:-$CLAUDE_PROJECT_DIR}\"`). `--format json` for structured output. Full detail: `cerebro usage` or `cerebro <cmd> --help`.\n")
	for _, s := range usageSections {
		fmt.Fprintf(&b, "\n**%s**\n", s.Title)
		for _, e := range s.Entries {
			fmt.Fprintf(&b, "- %s → %s\n", e.Situation, e.Line)
		}
	}
	return b.String()
}

var usageClaudeMDBlockFlag bool

func init() {
	cmd := &cobra.Command{
		Use:   "usage",
		Short: "Agent-oriented capability map: what cerebro can do and when to reach for it",
		Long: `Prints the situation -> command capability map. This is the discovery
surface for agents: one call instead of help-spelunking. The same block is
embedded in the CLAUDE.md section cerebro init writes, so it is in context
from session load; a CI guard keeps binary and template byte-identical.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if usageClaudeMDBlockFlag {
				fmt.Print(renderCapabilityMap())
				return nil
			}
			fmt.Printf("# Cerebro capability map\n\n%s", renderCapabilityMap())
			return nil
		},
	}
	cmd.Flags().BoolVar(&usageClaudeMDBlockFlag, "claudemd-block", false,
		"Emit only the raw block for pasting between the CLAUDE.md template markers")
	rootCmd.AddCommand(cmd)
}
