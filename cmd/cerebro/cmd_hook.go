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
//	cerebro hook prime        — session-start prime (recall --prime banner);
//	                            no-ops if already primed this session
//	cerebro hook post-compact — clears the primed flag so the next prime
//	                            re-fires (post-compaction context recovery)
//	cerebro hook session-end  — gc + ingest; no-ops on repeat
//
// Stale state files (>7 days) are lazily reaped on any state write.

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

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
		hookCmd.AddCommand(&cobra.Command{
			Use:          event,
			Short:        "Run the " + event + " lifecycle event (idempotent per session)",
			Args:         cobra.NoArgs,
			SilenceUsage: true,
			RunE: func(cmd *cobra.Command, args []string) error {
				return runHookEvent(event)
			},
		})
	}
	rootCmd.AddCommand(hookCmd)
}

// hookSessionState is the persisted per-session record.
type hookSessionState struct {
	SessionID   string            `json:"session_id"`
	EventsFired map[string]string `json:"events_fired"`
}

// Work runners are seams so the guard machinery is unit-testable without
// invoking the real prime/gc/ingest paths. The defaults re-exec this binary
// so hook output stays byte-equivalent to the raw commands the pre-plugin
// settings.json templates ran.
var hookPrimeWork = func() error {
	proj := resolveProjectDir()
	out, err := exec.Command(selfExecutable(), "recall", "--prime", "--format", "md", "-p", proj).Output() //nolint:gosec // argv-array, no shell
	if err != nil {
		return nil // no brain / recall failure: hooks are best-effort, stay silent
	}
	if len(out) == 0 {
		return nil
	}
	count := 0
	for _, line := range splitLines(string(out)) {
		if len(line) > 12 && line[0] == '#' && line[1] == '#' {
			count++
		}
	}
	fmt.Printf("Cerebro online. %d memories loaded.\n\n%s", count, out)
	// Surface the last GC eviction line for operator review, as the raw
	// settings.json hook did.
	gcLog := filepath.Join(proj, ".cerebro-gc.log")
	if data, err := os.ReadFile(gcLog); err == nil && len(data) > 0 { //nolint:gosec // project-local log path
		lines := splitLines(string(data))
		if len(lines) > 0 {
			fmt.Printf("\n--- CEREBRO GC EVICTIONS (review and re-add if needed) ---\n%s\n", lines[len(lines)-1])
		}
	}
	return nil
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

func splitLines(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			if i > start {
				out = append(out, s[start:i])
			}
			start = i + 1
		}
	}
	if start < len(s) {
		out = append(out, s[start:])
	}
	return out
}

// hookSessionID resolves the session identity for state addressing.
func hookSessionID() string {
	if sid := os.Getenv("CLAUDE_SESSION_ID"); sid != "" {
		return sid
	}
	return "default"
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
		if err := hookPrimeWork(); err != nil {
			return err
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
