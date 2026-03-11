package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

//go:embed templates/settings.json
var settingsTemplate []byte

//go:embed templates/skill_remember.md
var skillRememberTemplate []byte

//go:embed templates/skill_recall.md
var skillRecallTemplate []byte

//go:embed templates/skill_consolidate.md
var skillConsolidateTemplate []byte

//go:embed templates/claudemd_section.md
var claudeMDSectionTemplate []byte

// scaffoldSettings creates or merges .claude/settings.json with cerebro hooks.
// Returns true if changes were made, false if cerebro hooks already present.
func scaffoldSettings(projectDir string) (bool, error) {
	claudeDir := filepath.Join(projectDir, ".claude")
	settingsPath := filepath.Join(claudeDir, "settings.json")

	if err := os.MkdirAll(claudeDir, 0o750); err != nil {
		return false, fmt.Errorf("creating .claude directory: %w", err)
	}

	// Parse the template hooks
	var templateSettings map[string]any
	if err := json.Unmarshal(settingsTemplate, &templateSettings); err != nil {
		return false, fmt.Errorf("parsing settings template: %w", err)
	}

	// Check if file exists
	existingData, err := os.ReadFile(settingsPath)
	if err != nil {
		if !os.IsNotExist(err) {
			return false, fmt.Errorf("reading settings.json: %w", err)
		}
		// File doesn't exist — write the template directly
		if err := os.WriteFile(settingsPath, settingsTemplate, 0o644); err != nil { //nolint:gosec // settings.json needs to be readable
			return false, fmt.Errorf("writing settings.json: %w", err)
		}
		return true, nil
	}

	// File exists — check if cerebro hooks already present
	if strings.Contains(string(existingData), "cerebro") {
		return false, nil
	}

	// Parse existing settings
	var existingSettings map[string]any
	if err := json.Unmarshal(existingData, &existingSettings); err != nil {
		return false, fmt.Errorf("parsing existing settings.json: %w", err)
	}

	// Merge hooks
	merged := mergeHooks(existingSettings, templateSettings)
	out, err := json.MarshalIndent(merged, "", "  ")
	if err != nil {
		return false, fmt.Errorf("marshaling merged settings: %w", err)
	}
	out = append(out, '\n')

	if err := os.WriteFile(settingsPath, out, 0o644); err != nil { //nolint:gosec // settings.json needs to be readable
		return false, fmt.Errorf("writing merged settings.json: %w", err)
	}
	return true, nil
}

// mergeHooks merges template hooks into existing settings.
// For each event type, template hooks are appended to existing hooks.
func mergeHooks(existing, template map[string]any) map[string]any {
	if existing == nil {
		return template
	}

	existingHooks, _ := existing["hooks"].(map[string]any)
	templateHooks, _ := template["hooks"].(map[string]any)

	if existingHooks == nil {
		existingHooks = make(map[string]any)
	}

	for event, tHooks := range templateHooks {
		tArr, ok := tHooks.([]any)
		if !ok {
			continue
		}

		eArr, _ := existingHooks[event].([]any)
		existingHooks[event] = append(eArr, tArr...)
	}

	existing["hooks"] = existingHooks
	return existing
}

// scaffoldSkills creates .claude/skills/{remember,recall,consolidate}/SKILL.md files.
// Skips any skill file that already exists. Returns count of files created.
func scaffoldSkills(projectDir string) (int, error) {
	skills := map[string][]byte{
		"remember":    skillRememberTemplate,
		"recall":      skillRecallTemplate,
		"consolidate": skillConsolidateTemplate,
	}

	created := 0
	for name, content := range skills {
		skillDir := filepath.Join(projectDir, ".claude", "skills", name)
		skillPath := filepath.Join(skillDir, "SKILL.md")

		// Skip if exists
		if _, err := os.Stat(skillPath); err == nil {
			continue
		}

		if err := os.MkdirAll(skillDir, 0o750); err != nil {
			return created, fmt.Errorf("creating skill directory %s: %w", name, err)
		}

		if err := os.WriteFile(skillPath, content, 0o644); err != nil { //nolint:gosec // skill files need to be readable
			return created, fmt.Errorf("writing skill %s: %w", name, err)
		}
		created++
	}

	return created, nil
}

// scaffoldCLAUDEMD appends the Cerebro Memory System section to CLAUDE.md.
// Creates the file if it doesn't exist. Returns true if changes were made.
func scaffoldCLAUDEMD(projectDir string) (bool, error) {
	claudeMDPath := filepath.Join(projectDir, "CLAUDE.md")

	existing, err := os.ReadFile(claudeMDPath)
	if err != nil && !os.IsNotExist(err) {
		return false, fmt.Errorf("reading CLAUDE.md: %w", err)
	}

	// Check if marker already present
	if strings.Contains(string(existing), "## Cerebro Memory System") {
		return false, nil
	}

	// Append (or create) with the cerebro section
	var content []byte
	if len(existing) > 0 {
		content = existing
		if !strings.HasSuffix(string(content), "\n") {
			content = append(content, '\n')
		}
		content = append(content, '\n')
	}
	content = append(content, claudeMDSectionTemplate...)

	if err := os.WriteFile(claudeMDPath, content, 0o644); err != nil { //nolint:gosec // CLAUDE.md needs to be readable
		return false, fmt.Errorf("writing CLAUDE.md: %w", err)
	}
	return true, nil
}

// OllamaStatus reports whether ollama is available for use.
type OllamaStatus struct {
	Installed  bool
	Running    bool
	ModelReady bool
}

// checkOllama checks whether ollama is installed, running, and has the model.
func checkOllama(model string) OllamaStatus {
	var status OllamaStatus

	// Check installed
	if _, err := exec.LookPath("ollama"); err != nil {
		return status
	}
	status.Installed = true

	// Check running by listing models
	out, err := exec.Command("ollama", "list").Output() //nolint:gosec // fixed command
	if err != nil {
		return status
	}
	status.Running = true

	// Check if model is available
	if strings.Contains(string(out), model) {
		status.ModelReady = true
	}

	return status
}
