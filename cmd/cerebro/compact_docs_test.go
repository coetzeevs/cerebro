package main

// compact_docs_test.go — agentic-thnd AC7 (drill-down documented, the false
// "reliable ... channel" additionalContext claim removed from every copy,
// including repo CLAUDE.md per operator gate F2) and AC8 (the /recall skill
// teaches the compact scan plus the get drill-down).

import (
	"os"
	"strings"
	"testing"
)

// falseChannelClaim is split so a repo-wide grep for the claim returns nothing.
const falseChannelClaim = "reliable " + "additionalContext channel"

const sessionStartLine = "- Session start: a compact index of selected memories (one line each, kept under Claude Code's 10,000-character hook cap) is injected via hook; run `cerebro get <id>` for any memory's full content"

func TestCapabilityMap_NamesCompactAndDrillDown(t *testing.T) {
	m := renderCapabilityMap()
	for _, want := range []string{
		"`--prime` for session-start selection; `--compact` for a budgeted one-line index",
		"`cerebro get <id> [--with-provenance] [--as-of <time>]` — full content behind a prime/compact index line",
	} {
		if !strings.Contains(m, want) {
			t.Errorf("capability map missing %q", want)
		}
	}
}

func TestClaudeMDCopies_SessionStartLineAndNoFalseClaim(t *testing.T) {
	repoClaude, err := os.ReadFile("../../CLAUDE.md")
	if err != nil {
		t.Fatal(err)
	}
	rules, err := os.ReadFile(pluginRoot + "/skills/rules/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	hookSrc, err := os.ReadFile("cmd_hook.go")
	if err != nil {
		t.Fatal(err)
	}
	copies := map[string]string{
		"templates/claudemd_section.md": string(claudeMDSectionTemplate),
		"rules SKILL.md":                string(rules),
		"repo CLAUDE.md":                string(repoClaude),
	}
	for name, body := range copies {
		if !strings.Contains(body, sessionStartLine) {
			t.Errorf("%s missing the corrected session-start line", name)
		}
		if strings.Contains(body, falseChannelClaim) {
			t.Errorf("%s still claims a reliable channel: %q", name, falseChannelClaim)
		}
	}
	if strings.Contains(string(hookSrc), falseChannelClaim) ||
		strings.Contains(string(hookSrc), "additionalContext is the reliable") {
		t.Error("cmd_hook.go still claims additionalContext is reliable")
	}
}

// F2: no prior test read repo CLAUDE.md; its capability block must match the
// renderer like the template's does.
func TestRepoClaudeMD_EmbedsRenderedMap(t *testing.T) {
	data, err := os.ReadFile("../../CLAUDE.md")
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	begin, end := "<!-- cerebro:capabilities:begin -->", "<!-- cerebro:capabilities:end -->"
	bi, ei := strings.Index(s, begin), strings.Index(s, end)
	if bi < 0 || ei <= bi {
		t.Fatal("repo CLAUDE.md missing capability markers")
	}
	if strings.TrimSpace(s[bi+len(begin):ei]) != strings.TrimSpace(renderCapabilityMap()) {
		t.Error("repo CLAUDE.md capability block drifted from renderCapabilityMap()")
	}
}

func TestRecallSkill_TeachesCompactAndGet(t *testing.T) {
	plugin, err := os.ReadFile(pluginRoot + "/skills/recall/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		"templates/skill_recall.md": string(skillRecallTemplate),
		"plugin recall SKILL.md":    string(plugin),
	} {
		for _, want := range []string{
			`cerebro recall "$ARGUMENTS" --compact -p "$CLAUDE_PROJECT_DIR"`,
			`cerebro get <id> -p "$CLAUDE_PROJECT_DIR"`,
		} {
			if !strings.Contains(body, want) {
				t.Errorf("%s missing %q", name, want)
			}
		}
	}
}
