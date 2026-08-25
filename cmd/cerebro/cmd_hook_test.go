package main

// cmd_hook_test.go — agentic-k7dv Strategy-1 idempotency (TDD).
//
// Both distribution paths (cerebro init hooks AND the Claude Code plugin)
// route lifecycle events through `cerebro hook <event>`, and the binary
// no-ops a repeat of the same event for the same session id. Neither path
// needs to know about the other; double-fires converge in the state file at
// ~/.cerebro/session-state/<session-id>.json.
//
// The work itself (prime recall, gc, ingest) is behind runner seams so these
// tests exercise the GUARD machinery; the default runners re-exec the real
// binary and are covered by the live e2e smoke.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// stubHookRunners replaces the work runners with counters and restores them
// on cleanup. Returns pointers to the fire counts.
func stubHookRunners(t *testing.T) (primeCount, endCount *int) {
	t.Helper()
	var p, e int
	oldPrime, oldEnd := hookPrimeWork, hookSessionEndWork
	hookPrimeWork = func(event string) (bool, error) { p++; return true, nil }
	hookSessionEndWork = func() error { e++; return nil }
	t.Cleanup(func() { hookPrimeWork, hookSessionEndWork = oldPrime, oldEnd })
	return &p, &e
}

func setupHookTest(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("CLAUDE_SESSION_ID", "sess-hook-test")
}

func TestHookPrime_SecondInvocationSameSessionIsNoop(t *testing.T) {
	setupHookTest(t)
	primes, _ := stubHookRunners(t)

	if err := runHookEvent("prime"); err != nil {
		t.Fatalf("first prime: %v", err)
	}
	if err := runHookEvent("prime"); err != nil {
		t.Fatalf("second prime: %v", err)
	}
	if *primes != 1 {
		t.Errorf("prime work fired %d times for one session, want 1", *primes)
	}
}

func TestHookPostCompact_ClearsPrimeSoItFiresAgain(t *testing.T) {
	setupHookTest(t)
	primes, _ := stubHookRunners(t)

	_ = runHookEvent("prime")
	if err := runHookEvent("post-compact"); err != nil {
		t.Fatalf("post-compact: %v", err)
	}
	_ = runHookEvent("prime")
	if *primes != 2 {
		t.Errorf("prime after post-compact fired %d times total, want 2 (sentinel cleared)", *primes)
	}
}

func TestHookSessionEnd_SecondInvocationIsNoop(t *testing.T) {
	setupHookTest(t)
	_, ends := stubHookRunners(t)

	_ = runHookEvent("session-end")
	_ = runHookEvent("session-end")
	if *ends != 1 {
		t.Errorf("session-end work fired %d times for one session, want 1", *ends)
	}
}

func TestHookSessionID_DistinctSessionsIndependent(t *testing.T) {
	setupHookTest(t)
	primes, _ := stubHookRunners(t)

	t.Setenv("CLAUDE_SESSION_ID", "sess-a")
	_ = runHookEvent("prime")
	t.Setenv("CLAUDE_SESSION_ID", "sess-b")
	_ = runHookEvent("prime")
	if *primes != 2 {
		t.Errorf("distinct sessions must each prime once, got %d fires", *primes)
	}
}

func TestHookSessionID_FallsBackToDefault(t *testing.T) {
	setupHookTest(t)
	t.Setenv("CLAUDE_SESSION_ID", "")
	primes, _ := stubHookRunners(t)

	_ = runHookEvent("prime")
	_ = runHookEvent("prime")
	if *primes != 1 {
		t.Errorf("default-session guard broken: %d fires, want 1", *primes)
	}
	if _, err := os.Stat(filepath.Join(os.Getenv("HOME"), ".cerebro", "session-state", "default.json")); err != nil { //nolint:gosec // HOME is t.TempDir()
		t.Errorf("expected default.json state file: %v", err)
	}
}

func TestHookState_LazyGCOfStaleFiles(t *testing.T) {
	setupHookTest(t)
	stubHookRunners(t)

	dir := filepath.Join(os.Getenv("HOME"), ".cerebro", "session-state")
	if err := os.MkdirAll(dir, 0o750); err != nil { //nolint:gosec // HOME is t.TempDir()
		t.Fatalf("mkdir: %v", err)
	}
	stale := filepath.Join(dir, "sess-ancient.json")
	if err := os.WriteFile(stale, []byte(`{"session_id":"sess-ancient","events_fired":{}}`), 0o600); err != nil { //nolint:gosec // HOME is t.TempDir()
		t.Fatalf("writing stale file: %v", err)
	}
	old := time.Now().Add(-8 * 24 * time.Hour)
	if err := os.Chtimes(stale, old, old); err != nil {
		t.Fatalf("backdating stale file: %v", err)
	}

	_ = runHookEvent("prime") // any state write triggers lazy GC

	if _, err := os.Stat(stale); !os.IsNotExist(err) { //nolint:gosec // HOME is t.TempDir()
		t.Errorf("stale session-state file (>7 days) not reaped: err=%v", err)
	}
}

// ---- agentic-kpko: SessionStart additionalContext emission (TDD) ----
//
// Plain stdout is NOT a reliable context-injection surface for SessionStart
// (code.claude.com/docs/en/hooks — only hookSpecificOutput.additionalContext
// reliably reaches the model; DE-327/HA-039). And the v3.3.0 guard recorded
// session_start even when the stdout injection was dropped, turning the
// UserPromptSubmit fallback from delivery-guarantee into dedup-only. The fix:
// --event sessionstart emits the additionalContext JSON shape, and the guard
// records state only when memories were actually delivered (primed=true).

func TestRenderPrime_SessionStartEmitsAdditionalContextJSON(t *testing.T) {
	md := "## abcd1234 [concept/active] (importance: 0.80)\nA memory.\n\n## ef567890 [episode/active] (importance: 0.50)\nAnother.\n"
	out, primed := renderPrimeOutput("sessionstart", md, t.TempDir())
	if !primed {
		t.Fatal("non-empty prime must report primed=true")
	}
	var payload struct {
		SystemMessage      string `json:"systemMessage"`
		HookSpecificOutput struct {
			HookEventName     string `json:"hookEventName"`
			AdditionalContext string `json:"additionalContext"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("sessionstart output must be JSON: %v (%q)", err, out)
	}
	if payload.HookSpecificOutput.HookEventName != "SessionStart" {
		t.Errorf("hookEventName: got %q", payload.HookSpecificOutput.HookEventName)
	}
	if payload.SystemMessage != "Cerebro online. 2 memories loaded." {
		t.Errorf("systemMessage: got %q", payload.SystemMessage)
	}
	if !strings.Contains(payload.HookSpecificOutput.AdditionalContext, "A memory.") ||
		!strings.Contains(payload.HookSpecificOutput.AdditionalContext, "Cerebro online. 2 memories loaded.") {
		t.Errorf("additionalContext missing banner or memories: %q", payload.HookSpecificOutput.AdditionalContext)
	}
}

func TestRenderPrime_SessionStartEmptyWithBrainStaysSilent(t *testing.T) {
	// A brain that exists but returned no memories: no output, not primed —
	// the UserPromptSubmit fallback may retry later.
	proj := setupAddTest(t) // creates a real (empty) brain for proj
	out, primed := renderPrimeOutput("sessionstart", "", proj)
	if out != "" || primed {
		t.Errorf("empty prime with existing brain must be silent/unprimed: out=%q primed=%v", out, primed)
	}
}

func TestRenderPrime_SessionStartNoBrainEmitsNotice(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	out, primed := renderPrimeOutput("sessionstart", "", t.TempDir())
	if primed {
		t.Error("no-brain must not report primed")
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("no-brain notice must be JSON: %v (%q)", err, out)
	}
	if msg, _ := payload["systemMessage"].(string); !strings.Contains(msg, "no brain") {
		t.Errorf("no-brain systemMessage: got %q", msg)
	}
}

func TestRenderPrime_DefaultEventKeepsPlainStdout(t *testing.T) {
	md := "## abcd1234 [concept/active] (importance: 0.80)\nA memory.\n"
	out, primed := renderPrimeOutput("userpromptsubmit", md, t.TempDir())
	if !primed {
		t.Fatal("non-empty prime must report primed=true")
	}
	if strings.HasPrefix(strings.TrimSpace(out), "{") {
		t.Errorf("userpromptsubmit output must stay plain (stdout is a supported channel there): %q", out)
	}
	if !strings.Contains(out, "Cerebro online. 1 memories loaded.") || !strings.Contains(out, "A memory.") {
		t.Errorf("plain output missing banner or memories: %q", out)
	}
}

// The guard must NOT record session_start when nothing was delivered — an
// undelivered prime retries on the next hook prime (the delivery guarantee).
func TestHookPrime_UnprimedRunDoesNotRecordState(t *testing.T) {
	setupHookTest(t)
	calls := 0
	oldPrime := hookPrimeWork
	hookPrimeWork = func(event string) (bool, error) { calls++; return false, nil }
	t.Cleanup(func() { hookPrimeWork = oldPrime })

	_ = runHookEvent("prime")
	_ = runHookEvent("prime")
	if calls != 2 {
		t.Errorf("unprimed runs must retry (state not recorded): got %d calls, want 2", calls)
	}
}
