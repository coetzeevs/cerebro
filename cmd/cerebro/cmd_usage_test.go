package main

// cmd_usage_test.go — agentic-trko (TDD): the at-load capability map.
//
// Agents kept running `cerebro help` mid-task because no surface enumerated
// the tool's capabilities at session load. `cerebro usage` renders a compact
// situation -> command map; the CLAUDE.md template embeds the SAME rendered
// block between markers; drift guards make both unfalsifiable: every
// registered command must be covered, and the template block must equal the
// renderer's output byte-for-byte.

import (
	"strings"
	"testing"
)

// Every registered command is covered by the curated map — a new command
// cannot ship invisible. Exclusions are explicit and short.
func TestUsageMap_CoversEveryCommand(t *testing.T) {
	excluded := map[string]bool{"help": true, "completion": true}
	covered := map[string]bool{}
	for _, section := range usageSections {
		for _, e := range section.Entries {
			covered[e.Command] = true
		}
	}
	for _, c := range rootCmd.Commands() {
		name := c.Name()
		if excluded[name] {
			continue
		}
		if !covered[name] {
			t.Errorf("command %q not covered by the usage map — agents cannot discover it (add an entry to usageSections)", name)
		}
	}
	// And no stale entries for commands that no longer exist.
	registered := map[string]bool{}
	for _, c := range rootCmd.Commands() {
		registered[c.Name()] = true
	}
	for cmd := range covered {
		if !registered[cmd] {
			t.Errorf("usage map covers %q, which is not a registered command (stale entry)", cmd)
		}
	}
}

// The rendered map is compact, sectioned, and carries the situation lines.
func TestRenderCapabilityMap_Shape(t *testing.T) {
	out := renderCapabilityMap()
	for _, want := range []string{
		"cerebro recall", "cerebro outcome", "cerebro inbox", "cerebro forget",
		"cerebro embed --pending", "--anchor",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered map missing %q", want)
		}
	}
	if strings.Count(out, "\n") > 90 {
		t.Errorf("map too long for an at-load surface: %d lines", strings.Count(out, "\n"))
	}
}

// The CLAUDE.md template embeds the renderer's exact output between markers —
// the at-load surface can never drift from the binary (0p3w lockstep).
func TestClaudeMDTemplate_EmbedsRenderedMap(t *testing.T) {
	tpl := string(claudeMDSectionTemplate)
	begin := "<!-- cerebro:capabilities:begin -->"
	end := "<!-- cerebro:capabilities:end -->"
	bi := strings.Index(tpl, begin)
	ei := strings.Index(tpl, end)
	if bi < 0 || ei < 0 || ei <= bi {
		t.Fatal("claudemd_section.md missing capability markers")
	}
	embedded := strings.TrimSpace(tpl[bi+len(begin) : ei])
	rendered := strings.TrimSpace(renderCapabilityMap())
	if embedded != rendered {
		t.Error("CLAUDE.md template capability block drifted from renderCapabilityMap() — regenerate with `cerebro usage --claudemd-block` and paste between the markers")
	}
}
