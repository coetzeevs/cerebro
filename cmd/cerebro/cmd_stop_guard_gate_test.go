package main

// cmd_stop_guard_gate_test.go — agentic-3xz9 fixture suite.
//
// The defect: the guard forced continuation past legitimate human-confirmation
// gates (a push, a destructive apply, a merge needing sign-off), inverting its
// intent and effectively self-approving actions that require a human. It was
// removed from the global Stop hook ~2026-07-18 because of this.
//
// The fix under test: a first-class confirmation-gate detector that runs BEFORE
// every premature-stop category. A message that stops to request human
// confirmation for an irreversible or outward-facing action is ALWAYS allowed
// through — even when it also matches a premature-stop pattern.
//
// Fixtures are real phrasings: the EDP estate's standing rules ("never push
// without confirming") and actual gate messages produced by agent sessions.

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/coetzeevs/cerebro/brain"
)

// Gate class: every one of these stops to request a human decision on an
// irreversible/outward action and MUST be allowed (no decision field).
func TestStopGuard_AllowsConfirmationGates(t *testing.T) {
	gates := []string{
		// EDP standing-rule phrasings (never push without confirming)
		`All gates are green. I'm at the push gate and need your go to push and open the PR.`,
		`Work is committed on the feature branch. Per the push protocol I will not push without your explicit confirmation.`,
		`Ready to push — awaiting your confirmation.`,
		`I've stopped before pushing because pushing requires confirmation. Confirm and I'll push the branch.`,
		// Merge / release / deploy gates
		`CI is fully green on PR #62 but the merge needs your approval. Please run: gh pr merge 62 --squash --admin`,
		`The release tag is ready. Cutting it publishes the release — give me the green light and I'll proceed.`,
		`Deployment plan validated. Awaiting your go-ahead before I apply it to production.`,
		// Destructive-action gates
		`This would delete 14 datasets permanently. I need your explicit confirmation before running the delete.`,
		`terraform apply will destroy and recreate the instance. Stopping here for your sign-off.`,
		`Dropping the table is irreversible, so I'm waiting for your approval before proceeding.`,
		// Decision-required delivery shape (delivery-discipline format)
		`Recommendation: Option A. Decisions required: (1) confirm the migration window, (2) approve the push.`,
		// Blocked on input only the user can provide
		`I'm blocked on a credential only you can supply — the gcloud auth login must run interactively on your side.`,
		// Gate phrasing that ALSO contains a premature-stop trigger ("let me
		// know") — the gate must win.
		`The branch is ready. Let me know when it's ok to push and I'll take it from the tag onward.`,
		`For now, I've held off on the force-push — it rewrites shared history, so I need your confirmation first.`,
	}
	for _, msg := range gates {
		decision := runStopGuard(t, makeStopInput(msg))
		if decision.Decision != "" {
			t.Errorf("confirmation gate wrongly blocked (%q): %q — %q", decision.Decision, msg, decision.Reason)
		}
	}
}

// Lazy class: no human gate invoked, work plainly dangling — the guard may
// still block these (regression guard: the fix must not neuter the detector).
func TestStopGuard_StillBlocksLazyStops(t *testing.T) {
	lazy := []string{
		`Shall I continue with the implementation?`,
		`I can stop here if that looks good.`,
		`The remaining refactors are beyond the scope of this change.`,
		`I've done the first two files. Would you like me to do the rest?`,
		`That should be sufficient for now, I'll leave it there.`,
	}
	for _, msg := range lazy {
		decision := runStopGuard(t, makeStopInput(msg))
		if decision.Decision != "block" {
			t.Errorf("lazy stop not blocked: %q", msg)
		}
	}
}

// The safety valve is unchanged: once blocked this turn, always allow.
func TestStopGuard_GateAndStopHookActiveBothAllow(t *testing.T) {
	decision := runStopGuard(t, []byte(`{"last_assistant_message": "Shall I continue?", "stop_hook_active": true}`))
	if decision.Decision != "" {
		t.Errorf("stop_hook_active safety valve broken: %q", decision.Decision)
	}
}

// ---- operator ruling 2026-08-25: stop-guard is DISABLED BY DEFAULT ----
// The guard only evaluates when brain config stop_guard_enabled == "true"
// (strict, mirroring the rerank_enabled opt-in gate). Unset, "false", any
// other value, or no resolvable brain → always allow, no evaluation.

func TestStopGuardGate_DisabledByDefault(t *testing.T) {
	projectDir := setupAddTest(t)
	lazy := []byte(`{"last_assistant_message": "Shall I continue with the implementation?"}`)

	var out bytes.Buffer
	if err := runStopGuardGated(projectDir, bytes.NewReader(lazy), &out); err != nil {
		t.Fatalf("runStopGuardGated: %v", err)
	}
	var d stopHookDecision
	if err := json.Unmarshal(out.Bytes(), &d); err != nil {
		t.Fatalf("output not JSON: %v (%q)", err, out.String())
	}
	if d.Decision != "" {
		t.Errorf("guard evaluated while disabled (default): got %q", d.Decision)
	}
}

func TestStopGuardGate_OptInEnables(t *testing.T) {
	projectDir := setupAddTest(t)
	b, err := brain.Open(brain.ProjectPath(projectDir))
	if err != nil {
		t.Fatalf("brain.Open: %v", err)
	}
	if err := b.SetMeta("config.stop_guard_enabled", "true"); err != nil {
		t.Fatalf("SetMeta: %v", err)
	}
	_ = b.Close()

	lazy := []byte(`{"last_assistant_message": "Shall I continue with the implementation?"}`)
	var out bytes.Buffer
	if err := runStopGuardGated(projectDir, bytes.NewReader(lazy), &out); err != nil {
		t.Fatalf("runStopGuardGated: %v", err)
	}
	var d stopHookDecision
	_ = json.Unmarshal(out.Bytes(), &d)
	if d.Decision != "block" {
		t.Errorf("opted-in guard must evaluate: got %q", d.Decision)
	}
}

func TestStopGuardGate_NoBrainAllows(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	var out bytes.Buffer
	if err := runStopGuardGated(t.TempDir(), bytes.NewReader([]byte(`{"last_assistant_message": "Shall I continue?"}`)), &out); err != nil {
		t.Fatalf("runStopGuardGated must not error without a brain: %v", err)
	}
	var d stopHookDecision
	_ = json.Unmarshal(out.Bytes(), &d)
	if d.Decision != "" {
		t.Errorf("no-brain case must allow: got %q", d.Decision)
	}
}
