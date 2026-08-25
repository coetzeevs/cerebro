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
	"os"
	"path/filepath"
	"testing"
	"time"
)

// stubHookRunners replaces the work runners with counters and restores them
// on cleanup. Returns pointers to the fire counts.
func stubHookRunners(t *testing.T) (primeCount, endCount *int) {
	t.Helper()
	var p, e int
	oldPrime, oldEnd := hookPrimeWork, hookSessionEndWork
	hookPrimeWork = func() error { p++; return nil }
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
