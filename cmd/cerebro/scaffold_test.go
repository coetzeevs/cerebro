package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScaffoldSettings_NewFile(t *testing.T) {
	dir := t.TempDir()
	projectDir := filepath.Join(dir, "project")
	if err := os.MkdirAll(projectDir, 0o750); err != nil {
		t.Fatal(err)
	}

	created, err := scaffoldSettings(projectDir)
	if err != nil {
		t.Fatalf("scaffoldSettings: %v", err)
	}
	if !created {
		t.Error("expected created=true for new file")
	}

	// Verify file exists and is valid JSON
	path := filepath.Join(projectDir, ".claude", "settings.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading settings.json: %v", err)
	}

	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	// Should have hooks
	hooks, ok := settings["hooks"]
	if !ok {
		t.Fatal("expected hooks key in settings")
	}
	hooksMap, ok := hooks.(map[string]any)
	if !ok {
		t.Fatal("expected hooks to be an object")
	}

	// Should have all three event types
	for _, event := range []string{"SessionStart", "PreCompact", "SessionEnd"} {
		if _, ok := hooksMap[event]; !ok {
			t.Errorf("missing hook event: %s", event)
		}
	}
}

func TestScaffoldSettings_ExistingWithoutCerebro(t *testing.T) {
	dir := t.TempDir()
	projectDir := filepath.Join(dir, "project")
	claudeDir := filepath.Join(projectDir, ".claude")
	if err := os.MkdirAll(claudeDir, 0o750); err != nil {
		t.Fatal(err)
	}

	// Write existing settings with user hooks
	existing := map[string]any{
		"hooks": map[string]any{
			"SessionStart": []any{
				map[string]any{
					"matcher": "startup",
					"hooks": []any{
						map[string]any{"type": "command", "command": "echo user hook"},
					},
				},
			},
		},
	}
	data, _ := json.MarshalIndent(existing, "", "  ")
	if err := os.WriteFile(filepath.Join(claudeDir, "settings.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}

	created, err := scaffoldSettings(projectDir)
	if err != nil {
		t.Fatalf("scaffoldSettings: %v", err)
	}
	if !created {
		t.Error("expected created=true when merging new hooks")
	}

	// Verify merged - should have both user and cerebro hooks
	merged, err := os.ReadFile(filepath.Join(claudeDir, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}

	content := string(merged)
	if !strings.Contains(content, "echo user hook") {
		t.Error("existing user hook was clobbered")
	}
	if !strings.Contains(content, "cerebro") {
		t.Error("cerebro hooks not added")
	}
}

func TestScaffoldSettings_AlreadyHasCerebro(t *testing.T) {
	dir := t.TempDir()
	projectDir := filepath.Join(dir, "project")
	claudeDir := filepath.Join(projectDir, ".claude")
	if err := os.MkdirAll(claudeDir, 0o750); err != nil {
		t.Fatal(err)
	}

	// Write settings that already have cerebro hooks
	existing := map[string]any{
		"hooks": map[string]any{
			"SessionStart": []any{
				map[string]any{
					"matcher": "startup",
					"hooks": []any{
						map[string]any{"type": "command", "command": "cerebro recall --prime"},
					},
				},
			},
		},
	}
	data, _ := json.MarshalIndent(existing, "", "  ")
	if err := os.WriteFile(filepath.Join(claudeDir, "settings.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}

	created, err := scaffoldSettings(projectDir)
	if err != nil {
		t.Fatalf("scaffoldSettings: %v", err)
	}
	if created {
		t.Error("expected created=false when cerebro hooks already present")
	}
}

func TestScaffoldSkills_NewFiles(t *testing.T) {
	dir := t.TempDir()
	projectDir := filepath.Join(dir, "project")
	if err := os.MkdirAll(projectDir, 0o750); err != nil {
		t.Fatal(err)
	}

	created, err := scaffoldSkills(projectDir)
	if err != nil {
		t.Fatalf("scaffoldSkills: %v", err)
	}
	if created != 3 {
		t.Errorf("expected 3 skills created, got %d", created)
	}

	// Verify all three skill files exist
	for _, skill := range []string{"remember", "recall", "consolidate"} {
		path := filepath.Join(projectDir, ".claude", "skills", skill, "SKILL.md")
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("skill file not created: %s", path)
		}
	}
}

func TestScaffoldSkills_ExistingSkipped(t *testing.T) {
	dir := t.TempDir()
	projectDir := filepath.Join(dir, "project")
	skillDir := filepath.Join(projectDir, ".claude", "skills", "remember")
	if err := os.MkdirAll(skillDir, 0o750); err != nil {
		t.Fatal(err)
	}

	// Write existing skill
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("custom skill"), 0o600); err != nil {
		t.Fatal(err)
	}

	created, err := scaffoldSkills(projectDir)
	if err != nil {
		t.Fatalf("scaffoldSkills: %v", err)
	}
	if created != 2 {
		t.Errorf("expected 2 skills created (remember skipped), got %d", created)
	}

	// Existing file should not be overwritten
	data, _ := os.ReadFile(filepath.Join(skillDir, "SKILL.md"))
	if string(data) != "custom skill" {
		t.Error("existing skill file was overwritten")
	}
}

func TestScaffoldCLAUDEMD_NewFile(t *testing.T) {
	dir := t.TempDir()
	projectDir := filepath.Join(dir, "project")
	if err := os.MkdirAll(projectDir, 0o750); err != nil {
		t.Fatal(err)
	}

	created, err := scaffoldCLAUDEMD(projectDir)
	if err != nil {
		t.Fatalf("scaffoldCLAUDEMD: %v", err)
	}
	if !created {
		t.Error("expected created=true for new file")
	}

	data, err := os.ReadFile(filepath.Join(projectDir, "CLAUDE.md"))
	if err != nil {
		t.Fatalf("reading CLAUDE.md: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "## Cerebro Memory System") {
		t.Error("expected Cerebro Memory System section")
	}
}

func TestScaffoldCLAUDEMD_ExistingWithoutMarker(t *testing.T) {
	dir := t.TempDir()
	projectDir := filepath.Join(dir, "project")
	if err := os.MkdirAll(projectDir, 0o750); err != nil {
		t.Fatal(err)
	}

	// Write existing CLAUDE.md without cerebro section
	existing := "# My Project\n\nSome instructions.\n"
	if err := os.WriteFile(filepath.Join(projectDir, "CLAUDE.md"), []byte(existing), 0o600); err != nil {
		t.Fatal(err)
	}

	created, err := scaffoldCLAUDEMD(projectDir)
	if err != nil {
		t.Fatalf("scaffoldCLAUDEMD: %v", err)
	}
	if !created {
		t.Error("expected created=true when appending section")
	}

	data, _ := os.ReadFile(filepath.Join(projectDir, "CLAUDE.md"))
	content := string(data)
	if !strings.Contains(content, "# My Project") {
		t.Error("existing content was clobbered")
	}
	if !strings.Contains(content, "## Cerebro Memory System") {
		t.Error("cerebro section not appended")
	}
}

func TestScaffoldCLAUDEMD_AlreadyHasMarker(t *testing.T) {
	dir := t.TempDir()
	projectDir := filepath.Join(dir, "project")
	if err := os.MkdirAll(projectDir, 0o750); err != nil {
		t.Fatal(err)
	}

	existing := "# My Project\n\n## Cerebro Memory System\n\nAlready configured.\n"
	if err := os.WriteFile(filepath.Join(projectDir, "CLAUDE.md"), []byte(existing), 0o600); err != nil {
		t.Fatal(err)
	}

	created, err := scaffoldCLAUDEMD(projectDir)
	if err != nil {
		t.Fatalf("scaffoldCLAUDEMD: %v", err)
	}
	if created {
		t.Error("expected created=false when marker already present")
	}
}

func TestCheckOllama(t *testing.T) {
	// This test just verifies the function doesn't panic.
	// Actual ollama may or may not be installed.
	result := checkOllama("nomic-embed-text")
	if result.Installed && result.ModelReady && !result.Running {
		t.Error("model can't be ready if ollama isn't running")
	}
}
