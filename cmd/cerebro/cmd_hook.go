package main

// cmd_hook.go — session-guarded lifecycle subcommands (agentic-k7dv,
// Strategy 1: idempotent cerebro CLI).
//
// If a user runs `cerebro init` AND installs the Claude Code plugin, both
// register the same lifecycle hooks, so SessionStart prime / PostCompact
// sentinel-clear / SessionEnd GC would each fire twice. Rather than either
// installation path detecting the other (rejected as fragile), the binary
// itself tracks per-session-id state at
// ~/.cerebro/session-state/<session-id>.json and no-ops a repeat of the same
// event for the same session. Hook events are sequential per session; the
// double-fire race window is microseconds and the state writes converge.
//
//	cerebro hook prime        — session-start prime (budgeted compact index);
//	                            no-ops if already primed this session
//	cerebro hook post-compact — clears the primed flag so the next prime
//	                            re-fires (post-compaction context recovery)
//	cerebro hook session-end  — gc + ingest; no-ops on repeat
//
// Stale state files (>7 days) are lazily reaped on any state write.

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/coetzeevs/cerebro/brain"
	"github.com/coetzeevs/cerebro/internal/store"
	"github.com/spf13/cobra"
)

func init() {
	hookCmd := &cobra.Command{
		Use:   "hook",
		Short: "Session-guarded lifecycle hooks (prime, post-compact, session-end)",
		Long: `Lifecycle subcommands for Claude Code hooks, guarded by per-session-id
state so a double-registered hook (cerebro init AND the cerebro plugin) fires
its work exactly once per session. State lives at
~/.cerebro/session-state/<session-id>.json; the session id comes from
$CLAUDE_SESSION_ID (fallback "default"). Stale state files (>7 days) are
reaped lazily on any state write.`,
	}
	for _, ev := range []string{"prime", "post-compact", "session-end"} {
		event := ev
		sub := &cobra.Command{
			Use:          event,
			Short:        "Run the " + event + " lifecycle event (idempotent per session)",
			Args:         cobra.NoArgs,
			SilenceUsage: true,
			RunE: func(cmd *cobra.Command, args []string) error {
				return runHookEvent(event)
			},
		}
		if event == "prime" {
			// --event picks the emission channel (agentic-kpko): SessionStart
			// stdout is NOT a reliable context-injection surface (DE-327 /
			// code.claude.com/docs/en/hooks), so "sessionstart" emits the
			// hookSpecificOutput.additionalContext JSON shape; anything else
			// (UserPromptSubmit, manual runs) keeps plain stdout, which IS a
			// supported channel there.
			sub.Flags().StringVar(&hookPrimeEventFlag, "event", "",
				`Hook event emitting this prime: "sessionstart" (additionalContext JSON) or "userpromptsubmit"/empty (plain stdout)`)
		}
		hookCmd.AddCommand(sub)
	}
	rootCmd.AddCommand(hookCmd)
}

// hookSessionState is the persisted per-session record.
type hookSessionState struct {
	SessionID   string            `json:"session_id"`
	EventsFired map[string]string `json:"events_fired"`
}

// hookPrimeEventFlag selects the emission channel for `hook prime`.
var hookPrimeEventFlag string

// Work runners are seams so the guard machinery is unit-testable without
// invoking the real prime/gc/ingest paths. hookPrimeWork returns primed=true
// only when memories were actually delivered — the guard records session
// state ONLY then, so an undelivered prime retries on the next hook instead
// of silently marking the session primed (the v3.3.0 regression, agentic-kpko).
//
// The prime runs in-process (agentic-thnd, ADR-018): select WITHOUT stamping,
// render the budgeted compact index, print it, then stamp last_surfaced for
// exactly the ids that were printed. The previous re-exec of
// `recall --prime` stamped every selected memory in the child before any
// budget could apply, so stamps recorded deliveries that never happened.
// hookPrimeSelect is the prime selection seam (same pattern as the work-runner
// seams above), so tests can force a selection failure.
var hookPrimeSelect = selectPrime

var hookPrimeWork = func(event string) (bool, error) {
	proj := resolveProjectDir()
	var nodes []store.Node
	budget := defaultCompactBudget
	var b *brain.Brain
	if _, err := os.Stat(brain.ProjectPath(proj)); err == nil {
		// Any open/select failure degrades to "nothing selected" (unprimed,
		// retried on the next hook), as the re-exec's error path did.
		if opened, err := brain.Open(brain.ProjectPath(proj)); err == nil {
			b = opened
			defer func() { _ = b.Close() }()
			limit := resolveConfigInt(b.Store(), "prime_limit", 20)
			if limit < 1 {
				limit = 20 // stored value bypassed validation (e.g. import)
			}
			budget = resolveBudget(b.Store(), "prime_budget")
			nodes = hookPrimeSelect(b, limit)
		}
	}
	rendered, delivered := renderPrimeOutput(event, nodes, budget, proj)
	if rendered != "" {
		fmt.Print(rendered)
	}
	if b != nil && len(delivered) > 0 {
		_ = b.Store().TouchSurfaced(delivered) // best-effort, as before
	}
	return len(delivered) > 0, nil
}

// primePayload is the SessionStart JSON emission shape (Claude Code hooks
// protocol: exit 0 + JSON). additionalContext carries the model-facing text
// and systemMessage the UI notice; Claude Code caps each field at 10,000
// characters and replaces overflow with a file path plus a 2,000-character
// preview (https://code.claude.com/docs/en/hooks), hence the budgeted render.
type primePayload struct {
	SystemMessage      string `json:"systemMessage"`
	HookSpecificOutput struct {
		HookEventName     string `json:"hookEventName"`
		AdditionalContext string `json:"additionalContext"`
	} `json:"hookSpecificOutput"`
}

// renderPrimeOutput renders the prime emission for one hook event and returns
// the ids it delivered. Pure over (event, nodes, budget, projectDir) apart
// from reading the GC log tail and checking brain-file existence. The
// model-facing text (additionalContext at sessionstart, the whole stdout
// otherwise) is at most budget UTF-8 bytes.
func renderPrimeOutput(event string, nodes []store.Node, budget int, projectDir string) (out string, delivered []string) {
	sessionStart := event == "sessionstart"

	if len(nodes) == 0 {
		// Distinguish "no brain" (worth a one-time notice at session start)
		// from "brain exists, nothing recalled" (stay silent; the
		// UserPromptSubmit fallback may retry). Never primed either way.
		if !sessionStart {
			return "", nil
		}
		if _, err := os.Stat(brain.ProjectPath(projectDir)); err == nil {
			return "", nil
		}
		var p primePayload
		p.SystemMessage = "Cerebro: no brain initialized. Run: cerebro init"
		p.HookSpecificOutput.HookEventName = "SessionStart"
		p.HookSpecificOutput.AdditionalContext = "Cerebro: no brain initialized for this project. Run: cerebro init"
		data, err := json.Marshal(p)
		if err != nil {
			return "", nil
		}
		return string(data) + "\n", nil
	}

	text, delivered := renderCompactIndex(compactPrimeBanner, compactFrameHeader("importance", compactProjectSource),
		primeRecords(nodes), budget, gcNotice(projectDir))
	if len(delivered) == 0 {
		return "", nil
	}
	if !sessionStart {
		return text, delivered
	}
	var p primePayload
	p.SystemMessage = text[:strings.IndexByte(text, '\n')] // the banner line
	p.HookSpecificOutput.HookEventName = "SessionStart"
	p.HookSpecificOutput.AdditionalContext = text
	data, err := json.Marshal(p)
	if err != nil {
		return "", nil
	}
	return string(data) + "\n", delivered
}

var hookSessionEndWork = func() error {
	proj := resolveProjectDir()
	gcLog := filepath.Join(proj, ".cerebro-gc.log")
	_ = exec.Command(selfExecutable(), "gc", "--log", gcLog, "-p", proj).Run() //nolint:gosec // argv-array, no shell
	_ = exec.Command(selfExecutable(), "ingest", "-p", proj).Run()             //nolint:gosec // argv-array, no shell
	return nil
}

// selfExecutable resolves the running cerebro binary for re-exec; falls back
// to PATH lookup.
func selfExecutable() string {
	if exe, err := os.Executable(); err == nil {
		return exe
	}
	return "cerebro"
}

// hookStdin is the hook input stream; a var so tests can feed synthetic
// hook JSON. In production it is the JSON Claude Code pipes to every hook.
var hookStdin io.Reader = os.Stdin

// hookSessionID resolves the session identity for state addressing, in
// contract order (agentic-rcj6): the hook's stdin JSON `session_id` first —
// Claude Code does NOT export CLAUDE_SESSION_ID to hook processes
// (code.claude.com/docs/en/env-vars.md), stdin is the only in-hook source —
// then the env var (manual runs, smokes, the agent's Bash environment),
// then "default". Env-only resolution collapsed all real hook invocations
// onto one state file, letting one session's state suppress every later
// session's prime and session-end.
func hookSessionID() string {
	if sid := stdinSessionID(); sid != "" {
		return sid
	}
	if sid := os.Getenv("CLAUDE_SESSION_ID"); sid != "" {
		return sid
	}
	return "default"
}

// stdinSessionID reads the hook stdin JSON and extracts session_id. When
// hookStdin is the process stdin, it is read only if it is not a terminal
// (so a manual `cerebro hook prime` at a shell never blocks waiting for
// input). Reading consumes stdin — callers resolve the session id once per
// invocation, which runHookEvent does.
func stdinSessionID() string {
	if f, ok := hookStdin.(*os.File); ok && f == os.Stdin {
		info, err := os.Stdin.Stat()
		if err != nil || info.Mode()&os.ModeCharDevice != 0 {
			return "" // interactive terminal: nothing piped
		}
	}
	data, err := io.ReadAll(io.LimitReader(hookStdin, 1<<20))
	if err != nil || len(data) == 0 {
		return ""
	}
	var payload struct {
		SessionID string `json:"session_id"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return ""
	}
	return payload.SessionID
}

func sessionStateDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".cerebro", "session-state"), nil
}

func loadHookState(sid string) (*hookSessionState, error) {
	dir, err := sessionStateDir()
	if err != nil {
		return nil, err
	}
	st := &hookSessionState{SessionID: sid, EventsFired: map[string]string{}}
	data, err := os.ReadFile(filepath.Join(dir, sid+".json")) //nolint:gosec // state dir under $HOME
	if err != nil {
		return st, nil // missing/unreadable = fresh state
	}
	if err := json.Unmarshal(data, st); err != nil || st.EventsFired == nil {
		st = &hookSessionState{SessionID: sid, EventsFired: map[string]string{}}
	}
	return st, nil
}

// saveHookState persists the state and lazily reaps state files older than
// 7 days (small constant cost on each write).
func saveHookState(st *hookSessionState) error {
	dir, err := sessionStateDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	data, err := json.Marshal(st)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, st.SessionID+".json"), data, 0o600); err != nil {
		return err
	}
	// Lazy GC of stale session-state files.
	cutoff := time.Now().Add(-7 * 24 * time.Hour)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil //nolint:nilerr // reaping is best-effort
	}
	for _, e := range entries {
		info, err := e.Info()
		if err == nil && !e.IsDir() && info.ModTime().Before(cutoff) {
			_ = os.Remove(filepath.Join(dir, e.Name()))
		}
	}
	return nil
}

// runHookEvent executes one lifecycle event under the per-session guard.
func runHookEvent(event string) error {
	sid := hookSessionID()
	st, err := loadHookState(sid)
	if err != nil {
		// State unavailable (e.g. no HOME): run the work unguarded rather
		// than silently dropping a lifecycle event — double-fire is benign,
		// a missed prime is not.
		st = &hookSessionState{SessionID: sid, EventsFired: map[string]string{}}
	}
	now := time.Now().UTC().Format(time.RFC3339)

	switch event {
	case "prime":
		if st.EventsFired["session_start"] != "" {
			return nil // already primed this session
		}
		primed, err := hookPrimeWork(hookPrimeEventFlag)
		if err != nil {
			return err
		}
		if !primed {
			// Nothing delivered — leave state unrecorded so a later prime
			// (e.g. the UserPromptSubmit fallback) retries. This is the
			// delivery guarantee the v3.3.0 guard had dropped.
			return nil
		}
		st.EventsFired["session_start"] = now
	case "post-compact":
		// Clearing the primed flag is naturally idempotent — a double-fire
		// converges. The next `hook prime` re-fires, restoring context.
		delete(st.EventsFired, "session_start")
		st.EventsFired["post_compact"] = now
	case "session-end":
		if st.EventsFired["session_end"] != "" {
			return nil
		}
		if err := hookSessionEndWork(); err != nil {
			return err
		}
		st.EventsFired["session_end"] = now
	default:
		return fmt.Errorf("unknown hook event %q", event)
	}
	return saveHookState(st)
}
