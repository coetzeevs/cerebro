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

func TestStopGuard_StopHookActive_AllowsStop(t *testing.T) {
	// Safety valve: if already blocked once this turn, allow stopping unconditionally.
	phrases := []string{
		`Shall I continue with the implementation?`,        // permission-seeking
		`I can stop here if that looks good.`,              // premature-stopping
		`That's beyond the scope of this task.`,            // scope-reduction
		`Would you like me to proceed with the next step?`, // permission-seeking
	}
	for _, phrase := range phrases {
		input := makeStopInputWithActive(phrase, true)
		decision := runStopGuard(t, input)
		if decision.Decision != "" {
			t.Errorf("expected allow when stop_hook_active=true for %q, got %q", phrase, decision.Decision)
		}
	}
}

func TestStopGuard_ExemptsApprovalGate(t *testing.T) {
	phrases := []string{
		"Here's my plan for the refactoring:\n1. Update handler\n2. Add tests\n\nShall I proceed with implementation?",
		"I propose the following approach:\n- Refactor the store layer\n- Update callers\n\nWould you like me to implement this?",
		"My design for the new feature uses a factory pattern. Shall I proceed?",
		"Here's the implementation strategy:\n\n1. Add migration\n2. Update model\n\nShall I move forward?",
		"Ready for your review. Shall I proceed with implementation?",
	}
	for _, phrase := range phrases {
		input := makeStopInput(phrase)
		decision := runStopGuard(t, input)
		if decision.Decision != "" {
			t.Errorf("expected allow for approval gate %q, got decision=%q reason=%q", phrase, decision.Decision, decision.Reason)
		}
	}
}

func TestStopGuard_BlocksBareProceedWithoutContext(t *testing.T) {
	// Bare "Shall I proceed?" without plan/review context is lazy permission-seeking.
	phrases := []string{
		`Shall I proceed with the implementation?`,
		`Would you like me to proceed?`,
		`Shall I move forward with this?`,
		`Would you like me to start coding?`,
	}
	for _, phrase := range phrases {
		input := makeStopInput(phrase)
		decision := runStopGuard(t, input)
		if decision.Decision != "block" {
			t.Errorf("expected block for bare proceed %q, got %q", phrase, decision.Decision)
		}
	}
}

func TestStopGuard_ExemptsRiskyOpConfirmation(t *testing.T) {
	phrases := []string{
		`All tests pass. Shall I push to origin?`,
		`Would you like me to deploy this?`,
		`Shall I merge into main?`,
		`Would you like me to delete the old branch?`,
		`Let me know if you want me to force push.`,
		`Shall I publish the package?`,
		`Would you like me to release this version?`,
		`Shall I push?`,
		`Ready to push and open draft PR. Shall I push?`,
	}
	for _, phrase := range phrases {
		input := makeStopInput(phrase)
		decision := runStopGuard(t, input)
		if decision.Decision != "" {
			t.Errorf("expected allow for risky op %q, got decision=%q", phrase, decision.Decision)
		}
	}
}

func TestStopGuard_ExemptsCompletionConfirmation(t *testing.T) {
	phrases := []string{
		`175 tests, 0 failures. Ready to push and open draft PR. Shall I push?`,
		`All tests pass and the linter is clean. Would you like me to proceed?`,
		`Build succeeded. Shall I push?`,
		`0 errors, 0 warnings. Let me know if you want me to push.`,
	}
	for _, phrase := range phrases {
		input := makeStopInput(phrase)
		decision := runStopGuard(t, input)
		if decision.Decision != "" {
			t.Errorf("expected allow for completion confirm %q, got decision=%q", phrase, decision.Decision)
		}
	}
}

func TestStopGuard_StillBlocksLazyPermissionSeeking(t *testing.T) {
	phrases := []string{
		`Shall I continue with the rest?`,
		`Would you like me to handle that?`,
		`Let me know if you need anything else.`,
		`Shall I also update the docs?`,
		`Would you like me to add error handling?`,
	}
	for _, phrase := range phrases {
		input := makeStopInput(phrase)
		decision := runStopGuard(t, input)
		if decision.Decision != "block" {
			t.Errorf("expected block for lazy permission-seeking %q, got %q", phrase, decision.Decision)
		}
	}
}

func TestStopGuard_ExemptionDoesNotAffectOtherCategories(t *testing.T) {
	// Even if the message contains exemption context, other categories still fire.
	phrases := []string{
		`I can stop here. The plan is ready.`,       // premature-stopping
		`That's beyond the scope of this approach.`, // scope-reduction
		`I'll leave it at that for this design.`,    // premature-stopping
	}
	for _, phrase := range phrases {
		input := makeStopInput(phrase)
		decision := runStopGuard(t, input)
		if decision.Decision != "block" {
			t.Errorf("expected block for non-permission category %q, got %q", phrase, decision.Decision)
		}
	}
}

// --- Test helpers ---

type stopDecision struct {
	Decision string `json:"decision"`
	Reason   string `json:"reason"`
}

func makeStopInput(message string) []byte {
	return makeStopInputWithActive(message, false)
}

func makeStopInputWithActive(message string, active bool) []byte {
	input := map[string]any{
		"session_id":             "test-session",
		"hook_event_name":        "Stop",
		"stop_hook_active":       active,
		"last_assistant_message": message,
	}
	data, _ := json.Marshal(input)
	return data
}

func runStopGuardRaw(t *testing.T, input []byte) []byte {
	t.Helper()
	var out bytes.Buffer
	_, err := evalStopGuard(bytes.NewReader(input), &out)
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
