package main

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestStopGuard_BlocksPermissionSeeking(t *testing.T) {
	phrases := []string{
		`Shall I continue with the implementation?`,
		`Would you like me to proceed?`,
		`Let me know if you want me to continue.`,
	}
	for _, phrase := range phrases {
		input := makeStopInput(phrase)
		decision := runStopGuard(t, input)
		if decision.Decision != "block" {
			t.Errorf("expected block for %q, got %q", phrase, decision.Decision)
		}
		if decision.Reason == "" {
			t.Errorf("expected non-empty reason for blocked phrase %q", phrase)
		}
	}
}

func TestStopGuard_BlocksPrematureStopping(t *testing.T) {
	phrases := []string{
		`I can stop here if that looks good.`,
		`I'll leave it at that for now.`,
		`That should be sufficient for the feature.`,
	}
	for _, phrase := range phrases {
		input := makeStopInput(phrase)
		decision := runStopGuard(t, input)
		if decision.Decision != "block" {
			t.Errorf("expected block for %q, got %q", phrase, decision.Decision)
		}
	}
}

func TestStopGuard_BlocksScopeReduction(t *testing.T) {
	phrases := []string{
		`That's beyond the scope of this task.`,
		`This is out of scope for now.`,
		`We can add that as a future enhancement.`,
	}
	for _, phrase := range phrases {
		input := makeStopInput(phrase)
		decision := runStopGuard(t, input)
		if decision.Decision != "block" {
			t.Errorf("expected block for %q, got %q", phrase, decision.Decision)
		}
	}
}

func TestStopGuard_AllowsNormalCompletion(t *testing.T) {
	phrases := []string{
		`Done. All tests pass and the linter is clean.`,
		`The feature is implemented and committed.`,
		`I've updated the CHANGELOG and README.`,
		`Here's a summary of what was changed.`,
	}
	for _, phrase := range phrases {
		input := makeStopInput(phrase)
		decision := runStopGuard(t, input)
		if decision.Decision != "" {
			t.Errorf("expected allow (empty decision) for %q, got %q", phrase, decision.Decision)
		}
	}
}

func TestStopGuard_HandlesEmptyInput(t *testing.T) {
	decision := runStopGuard(t, []byte(""))
	if decision.Decision != "" {
		t.Errorf("expected allow on empty input, got %q", decision.Decision)
	}
}

func TestStopGuard_HandlesMissingField(t *testing.T) {
	input := []byte(`{"session_id": "abc123", "hook_event_name": "Stop"}`)
	decision := runStopGuard(t, input)
	if decision.Decision != "" {
		t.Errorf("expected allow when last_assistant_message missing, got %q", decision.Decision)
	}
}

func TestStopGuard_HandlesEmptyMessage(t *testing.T) {
	input := makeStopInput("")
	decision := runStopGuard(t, input)
	if decision.Decision != "" {
		t.Errorf("expected allow on empty message, got %q", decision.Decision)
	}
}

func TestStopGuard_CaseInsensitive(t *testing.T) {
	input := makeStopInput("SHALL I continue?")
	decision := runStopGuard(t, input)
	if decision.Decision != "block" {
		t.Errorf("expected case-insensitive block for 'SHALL I', got %q", decision.Decision)
	}
}

func TestStopGuard_ReasonDiffersPerCategory(t *testing.T) {
	permission := runStopGuard(t, makeStopInput("Shall I continue?"))
	premature := runStopGuard(t, makeStopInput("I can stop here."))
	scope := runStopGuard(t, makeStopInput("That's beyond the scope."))

	if permission.Reason == premature.Reason {
		t.Error("permission-seeking and premature-stopping should have different reasons")
	}
	if premature.Reason == scope.Reason {
		t.Error("premature-stopping and scope-reduction should have different reasons")
	}
	if permission.Reason == scope.Reason {
		t.Error("permission-seeking and scope-reduction should have different reasons")
	}
}

func TestStopGuard_OutputIsValidJSON(t *testing.T) {
	// Block case
	out := runStopGuardRaw(t, makeStopInput("Shall I continue?"))
	var blockResult map[string]any
	if err := json.Unmarshal(out, &blockResult); err != nil {
		t.Fatalf("block output is not valid JSON: %v\noutput: %s", err, out)
	}
	if _, ok := blockResult["decision"]; !ok {
		t.Error("block output missing 'decision' field")
	}
	if _, ok := blockResult["reason"]; !ok {
		t.Error("block output missing 'reason' field")
	}

	// Allow case
	out = runStopGuardRaw(t, makeStopInput("All done."))
	var allowResult map[string]any
	if err := json.Unmarshal(out, &allowResult); err != nil {
		t.Fatalf("allow output is not valid JSON: %v\noutput: %s", err, out)
	}
	if _, ok := allowResult["decision"]; ok {
		t.Error("allow output should not have 'decision' field")
	}
}

// --- Test helpers ---

type stopDecision struct {
	Decision string `json:"decision"`
	Reason   string `json:"reason"`
}

func makeStopInput(message string) []byte {
	input := map[string]any{
		"session_id":             "test-session",
		"hook_event_name":        "Stop",
		"stop_hook_active":       false,
		"last_assistant_message": message,
	}
	data, _ := json.Marshal(input)
	return data
}

func runStopGuardRaw(t *testing.T, input []byte) []byte {
	t.Helper()
	var out bytes.Buffer
	err := evalStopGuard(bytes.NewReader(input), &out)
	if err != nil {
		t.Fatalf("evalStopGuard error: %v", err)
	}
	return out.Bytes()
}

func runStopGuard(t *testing.T, input []byte) stopDecision {
	t.Helper()
	raw := runStopGuardRaw(t, input)
	var d stopDecision
	if err := json.Unmarshal(raw, &d); err != nil {
		t.Fatalf("unmarshal decision: %v\nraw: %s", err, raw)
	}
	return d
}
