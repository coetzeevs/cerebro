package main

import (
	"bytes"
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
// When force is true, existing cerebro hooks are replaced with the latest template.
// Returns true if changes were made, false if cerebro hooks already present.
func scaffoldSettings(projectDir string, force bool) (bool, error) {
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

	// Parse existing settings
	var existingSettings map[string]any
	if err := json.Unmarshal(existingData, &existingSettings); err != nil {
		return false, fmt.Errorf("parsing existing settings.json: %w", err)
	}

	// If cerebro hooks exist, handle upgrade or force-replace
	if strings.Contains(string(existingData), "cerebro") {
		if force {
			replaceCerebro(existingSettings, templateSettings)
		} else {
			added := addMissingEvents(existingSettings, templateSettings)
			if !added {
				return false, nil
			}
		}
	} else {
		// Fresh merge — no cerebro hooks yet
		existingSettings = mergeHooks(existingSettings, templateSettings)
	}

	out, err := json.MarshalIndent(existingSettings, "", "  ")
	if err != nil {
		return false, fmt.Errorf("marshaling merged settings: %w", err)
	}
	out = append(out, '\n')

	if err := os.WriteFile(settingsPath, out, 0o644); err != nil { //nolint:gosec // settings.json needs to be readable
		return false, fmt.Errorf("writing merged settings.json: %w", err)
	}
	return true, nil
}

// replaceCerebro removes cerebro hook entries from each event, then merges in the template.
// Non-cerebro user hooks are preserved.
func replaceCerebro(existing, template map[string]any) {
	existingHooks, _ := existing["hooks"].(map[string]any)
	templateHooks, _ := template["hooks"].(map[string]any)

	if existingHooks == nil {
		existingHooks = make(map[string]any)
	}

	// Strip cerebro entries from existing events
	for event, eHooks := range existingHooks {
		eArr, ok := eHooks.([]any)
		if !ok {
			continue
		}
		var kept []any
		for _, entry := range eArr {
			raw, _ := json.Marshal(entry)
			if !strings.Contains(string(raw), "cerebro") {
				kept = append(kept, entry)
			}
		}
		if len(kept) > 0 {
			existingHooks[event] = kept
		} else {
			delete(existingHooks, event)
		}
	}

	// Merge template hooks into the cleaned settings
	for event, tHooks := range templateHooks {
		tArr, ok := tHooks.([]any)
		if !ok {
			continue
		}
		eArr, _ := existingHooks[event].([]any)
		existingHooks[event] = append(eArr, tArr...)
	}

	existing["hooks"] = existingHooks
}

// addMissingEvents adds template event types that don't exist in the current settings.
// Existing events are left untouched. Returns true if any events were added.
func addMissingEvents(existing, template map[string]any) bool {
	existingHooks, _ := existing["hooks"].(map[string]any)
	templateHooks, _ := template["hooks"].(map[string]any)

	if existingHooks == nil {
		existingHooks = make(map[string]any)
	}

	added := false
	for event, tHooks := range templateHooks {
		if _, exists := existingHooks[event]; !exists {
			existingHooks[event] = tHooks
			added = true
		}
	}

	if added {
		existing["hooks"] = existingHooks
	}
	return added
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
// Skips any skill file that already exists unless force is true.
// Returns count of files written.
func scaffoldSkills(projectDir string, force bool) (int, error) {
	skills := map[string][]byte{
		"remember":    skillRememberTemplate,
		"recall":      skillRecallTemplate,
		"consolidate": skillConsolidateTemplate,
	}

	created := 0
	for name, content := range skills {
		skillDir := filepath.Join(projectDir, ".claude", "skills", name)
		skillPath := filepath.Join(skillDir, "SKILL.md")

		// Skip if exists (unless force)
		if _, err := os.Stat(skillPath); err == nil && !force {
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
// Creates the file if it doesn't exist. When force is true, replaces an
// existing Cerebro section with the latest template. Returns true if changes were made.
func scaffoldCLAUDEMD(projectDir string, force bool) (bool, error) {
	claudeMDPath := filepath.Join(projectDir, "CLAUDE.md")

	existing, err := os.ReadFile(claudeMDPath)
	if err != nil && !os.IsNotExist(err) {
		return false, fmt.Errorf("reading CLAUDE.md: %w", err)
	}

	marker := "## Cerebro Memory System"
	hasMarker := strings.Contains(string(existing), marker)

	// Skip if marker present and not forcing
	if hasMarker && !force {
		return false, nil
	}

	var content []byte
	if hasMarker && force {
		// Replace: keep everything before the marker, insert new template,
		// then preserve any independent sections that follow.
		idx := bytes.Index(existing, []byte(marker))
		before := strings.TrimRight(string(existing[:idx]), "\n\r\t ")

		// Find the end of the Cerebro section: the next "## " heading after the marker.
		after := existing[idx+len(marker):]
		var trailing []byte
		if nextH2 := bytes.Index(after, []byte("\n## ")); nextH2 >= 0 {
			trailing = after[nextH2+1:] // keep the "## " heading and everything after
		}

		content = []byte(before)
		if len(content) > 0 {
			content = append(content, '\n', '\n')
		}
		content = append(content, claudeMDSectionTemplate...)
		if len(trailing) > 0 {
			// Ensure one blank line between the template and the trailing section
			if !bytes.HasSuffix(content, []byte("\n\n")) {
				if bytes.HasSuffix(content, []byte("\n")) {
					content = append(content, '\n')
				} else {
					content = append(content, '\n', '\n')
				}
			}
			content = append(content, trailing...)
		}
	} else {
		// Append (or create)
		if len(existing) > 0 {
			content = existing
			if !strings.HasSuffix(string(content), "\n") {
				content = append(content, '\n')
			}
			content = append(content, '\n')
		}
		content = append(content, claudeMDSectionTemplate...)
	}

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
