package main

// plugin_assets_test.go — lockstep guard for the Claude Code plugin assets
// (agentic-k7dv). The plugin skills at claude-plugin/cerebro/skills/ are
// byte-identical copies of the embedded init templates; the 0p3w lesson is
// that duplicated surfaces drift silently unless a test fails on divergence.

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

const pluginRoot = "../../claude-plugin/cerebro"

func TestPluginSkills_MatchEmbeddedTemplates(t *testing.T) {
	pairs := map[string][]byte{
		"remember":    skillRememberTemplate,
		"recall":      skillRecallTemplate,
		"consolidate": skillConsolidateTemplate,
		"develop":     skillDevelopTemplate,
	}
	for name, want := range pairs {
		got, err := os.ReadFile(pluginRoot + "/skills/" + name + "/SKILL.md")
		if err != nil {
			t.Fatalf("plugin skill %s missing: %v", name, err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("plugin skill %q drifted from embedded template (update both in lockstep)", name)
		}
	}
}

func TestPluginRulesSkill_CarriesClaudeMDSection(t *testing.T) {
	got, err := os.ReadFile(pluginRoot + "/skills/rules/SKILL.md")
	if err != nil {
		t.Fatalf("rules skill missing: %v", err)
	}
	if !bytes.Contains(got, claudeMDSectionTemplate) {
		t.Error("rules skill no longer contains the claudemd_section template verbatim")
	}
	if !bytes.HasPrefix(got, []byte("---\nname: rules\n")) {
		t.Error("rules skill missing frontmatter")
	}
}

func TestPluginManifest_Shape(t *testing.T) {
	data, err := os.ReadFile(pluginRoot + "/.claude-plugin/plugin.json")
	if err != nil {
		t.Fatalf("plugin.json missing: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("plugin.json invalid JSON: %v", err)
	}
	if m["name"] != "cerebro" {
		t.Errorf("plugin name: got %v want cerebro", m["name"])
	}
	if m["hooks"] != "./hooks/hooks.json" {
		t.Errorf("plugin hooks path: got %v", m["hooks"])
	}
	if v, ok := m["version"].(string); !ok || v == "" {
		t.Error("plugin.json must pin an explicit version")
	}
}

func TestPluginHooks_ShapeAndNoStopHook(t *testing.T) {
	data, err := os.ReadFile(pluginRoot + "/hooks/hooks.json")
	if err != nil {
		t.Fatalf("hooks.json missing: %v", err)
	}
	var h struct {
		Hooks map[string]json.RawMessage `json:"hooks"`
	}
	if err := json.Unmarshal(data, &h); err != nil {
		t.Fatalf("hooks.json invalid JSON: %v", err)
	}
	for _, ev := range []string{"SessionStart", "UserPromptSubmit", "PreCompact", "PostCompact", "SessionEnd"} {
		if _, ok := h.Hooks[ev]; !ok {
			t.Errorf("plugin hooks missing %s", ev)
		}
	}
	// Deliberate exclusion (agentic-3xz9 / ruling C4): plugins have no
	// per-hook disable, so the stop-guard ships un-wired; users opt in via
	// their own settings.json. A Stop key appearing here means that decision
	// was overturned without updating this guard.
	if _, ok := h.Hooks["Stop"]; ok {
		t.Error("plugin hooks.json must NOT wire the Stop hook (C4: stop-guard is opt-in)")
	}
	// Every hook command self-gates so the plugin stays silent in projects
	// without a cerebro brain.
	if n := strings.Count(string(data), "cerebro stats -p"); n < 8 {
		t.Errorf("expected self-gating on all plugin hook commands, found %d gates", n)
	}
}

func TestInitSettingsTemplate_UsesGuardedHookCommands(t *testing.T) {
	s := string(settingsTemplate)
	for _, want := range []string{"cerebro hook prime", "cerebro hook post-compact", "cerebro hook session-end"} {
		if !strings.Contains(s, want) {
			t.Errorf("init settings template missing guarded command %q", want)
		}
	}
}
