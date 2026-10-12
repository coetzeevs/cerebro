package main

// compact_hook_test.go — agentic-thnd e2e over the REAL hookPrimeWork on temp
// brains (isolated HOME via setupAddTest): AC1 budget at both hook events,
// AC2 get round-trip and prefix-of-selection, AC3 stamping = delivery and
// sentinel iff k >= 1, AC4 frame at hook level, bounded GC trailer.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/coetzeevs/cerebro/brain"
	"github.com/coetzeevs/cerebro/internal/store"
)

// init guards the test binary against being re-executed as `cerebro <cmd>`:
// selfExecutable() is the test binary under `go test`, and the pre-thnd hook
// prime re-exec'd it with `recall --prime ...`, which re-ran the whole suite
// recursively. A test binary is always invoked with -test.* flags first, so a
// leading non-flag argument means a re-exec: fail fast instead of recursing.
func init() {
	if len(os.Args) > 1 && !strings.HasPrefix(os.Args[1], "-") {
		fmt.Fprintf(os.Stderr, "cerebro test binary re-executed as a CLI (%q); refusing to recurse\n", os.Args[1])
		os.Exit(3)
	}
}

var id8LineRE = regexp.MustCompile(`(?m)^- ([0-9a-f]{8}) `)

// seedPrimeBrain adds n memories with distinct importance (deterministic
// selection order) and the given content size, and applies config overrides.
func seedPrimeBrain(t *testing.T, proj string, n, contentBytes int, cfg map[string]string) map[string]string {
	t.Helper()
	b, err := brain.Open(brain.ProjectPath(proj))
	if err != nil {
		t.Fatalf("brain.Open: %v", err)
	}
	defer func() { _ = b.Close() }()
	types := []store.NodeType{store.TypeConcept, store.TypeProcedure, store.TypeEpisode, store.TypeReflection}
	contents := map[string]string{}
	for i := 0; i < n; i++ {
		c := fmt.Sprintf("memory %04d headline %s\nbody line two", i, strings.Repeat("é東🧠x", 1+contentBytes/10))
		id, err := b.Add(c, types[i%len(types)], brain.WithImportance(0.99-float64(i)/float64(n+1)))
		if err != nil {
			t.Fatalf("Add: %v", err)
		}
		contents[id] = c
	}
	for k, v := range cfg {
		if err := b.Store().SetMeta(configMetaPrefix+k, v); err != nil {
			t.Fatal(err)
		}
	}
	return contents
}

// runPrime runs the real hookPrimeWork for event and returns the model-facing
// string (decoded additionalContext for sessionstart, whole stdout otherwise).
func runPrime(t *testing.T, event string) (text string, primed bool) {
	t.Helper()
	var err error
	out := captureStdout(t, func() { primed, err = hookPrimeWork(event) })
	if err != nil {
		t.Fatalf("hookPrimeWork: %v", err)
	}
	if event != "sessionstart" || out == "" {
		return out, primed
	}
	var p primePayload
	if jerr := json.Unmarshal([]byte(out), &p); jerr != nil {
		t.Fatalf("sessionstart output is not JSON: %v (%q)", jerr, out)
	}
	if p.SystemMessage != strings.SplitN(p.HookSpecificOutput.AdditionalContext, "\n", 2)[0] {
		t.Errorf("systemMessage %q must equal the banner line", p.SystemMessage)
	}
	return p.HookSpecificOutput.AdditionalContext, primed
}

func deliveredID8s(text string) []string {
	var ids []string
	for _, m := range id8LineRE.FindAllStringSubmatch(text, -1) {
		ids = append(ids, m[1])
	}
	return ids
}

func surfacedState(t *testing.T, proj string) map[string]bool {
	t.Helper()
	b := brainAtDir(t, proj)
	defer func() { _ = b.Close() }()
	nodes, err := b.List(store.ListNodesOpts{})
	if err != nil {
		t.Fatal(err)
	}
	st := map[string]bool{}
	for i := range nodes {
		st[nodes[i].ID] = nodes[i].LastSurfaced != nil
	}
	return st
}

// AC1 + AC4 at hook level, both events, budget enforced on a large brain.
func TestHookPrime_BudgetAndFrameBothEvents(t *testing.T) {
	for _, event := range []string{"sessionstart", "userpromptsubmit"} {
		for _, budget := range []string{"2000", "9500", "10000"} {
			t.Run(event+"/"+budget, func(t *testing.T) {
				proj := setupAddTest(t)
				seedPrimeBrain(t, proj, 400, 5000, map[string]string{"prime_limit": "100", "prime_budget": budget})
				// one 200 KB memory
				b := brainAtDir(t, proj)
				_, _ = b.Add(strings.Repeat("Z", 200_000), store.TypeConcept, brain.WithImportance(1.0))
				_ = b.Close()

				text, primed := runPrime(t, event)
				var want int
				_, _ = fmt.Sscan(budget, &want)
				if !primed {
					t.Fatal("non-empty brain must prime")
				}
				if len(text) > want || utf16Len(text) > want || !utf8.ValidString(text) {
					t.Fatalf("prime text %d bytes / %d UTF-16 units / valid=%v exceeds budget %d",
						len(text), utf16Len(text), utf8.ValidString(text), want)
				}
				lines := strings.Split(text, "\n")
				if !strings.HasPrefix(lines[0], "Cerebro online. ") || !strings.HasSuffix(lines[0], " memories indexed (compact).") {
					t.Errorf("banner: %q", lines[0])
				}
				if lines[1]+"\n"+lines[2]+"\n" != testFrameHeaderProject {
					t.Errorf("frame header missing after banner: %q / %q", lines[1], lines[2])
				}
				if strings.Count(text, compactFrameFooter) != 1 {
					t.Error("footer must appear exactly once")
				}
				for _, l := range lines[3:] {
					if l == compactFrameFooter {
						break
					}
					if !primeLineRE.MatchString(l) {
						t.Errorf("inner line fails grammar: %q", l)
					}
				}
				if event == "userpromptsubmit" && strings.HasPrefix(text, "{") {
					t.Error("userpromptsubmit must stay plain stdout")
				}
			})
		}
	}
}

// An out-of-range prime_budget that bypassed config set (e.g. import) is
// ignored at read time.
func TestHookPrime_HostileStoredBudgetFallsBack(t *testing.T) {
	proj := setupAddTest(t)
	seedPrimeBrain(t, proj, 300, 2000, map[string]string{"prime_limit": "100", "prime_budget": "50000"})
	text, _ := runPrime(t, "sessionstart")
	if len(text) > defaultCompactBudget {
		t.Fatalf("stored budget 50000 must fall back to %d; got %d bytes", defaultCompactBudget, len(text))
	}
}

// AC3: stamped ids == delivered ids; undelivered rows unchanged. AC2: the
// delivered lines are a rank-ordered prefix of the selection and every id8
// round-trips through `cerebro get`.
func TestHookPrime_StampsOnlyDeliveredPrefix(t *testing.T) {
	proj := setupAddTest(t)
	contents := seedPrimeBrain(t, proj, 100, 300, map[string]string{"prime_limit": "100", "prime_budget": "2000"})

	b := brainAtDir(t, proj)
	selection := selectPrime(b, 100)
	_ = b.Close()
	before := surfacedState(t, proj)
	for id, s := range before {
		if s {
			t.Fatalf("precondition: %s already surfaced", id[:8])
		}
	}

	text, primed := runPrime(t, "sessionstart")
	got := deliveredID8s(text)
	if !primed || len(got) == 0 {
		t.Fatalf("expected a delivered prime; primed=%v k=%d", primed, len(got))
	}
	if len(selection) <= len(got) {
		t.Fatalf("budget 2000 must force k < N (k=%d, N=%d)", len(got), len(selection))
	}
	for i, id8 := range got {
		if selection[i].ID[:8] != id8 {
			t.Fatalf("delivered[%d]=%s is not selection[%d]=%s (not a rank-ordered prefix)", i, id8, i, selection[i].ID[:8])
		}
	}

	after := surfacedState(t, proj)
	delivered := map[string]bool{}
	for _, id8 := range got {
		delivered[id8] = true
	}
	for id, stamped := range after {
		if stamped != delivered[id[:8]] {
			t.Errorf("node %s: stamped=%v delivered=%v", id[:8], stamped, delivered[id[:8]])
		}
	}

	for _, id8 := range got {
		out, err := runCLI(t, proj, "get", id8)
		if err != nil {
			t.Fatalf("get %s: %v", id8, err)
		}
		found := false
		for id, c := range contents {
			if id[:8] == id8 && strings.Contains(out, c) {
				found = true
			}
		}
		if !found {
			t.Errorf("get %s did not return the memory's full content", id8)
		}
	}
}

// AC3: the session_start sentinel is recorded iff k >= 1; an empty brain
// changes no row and records nothing.
func TestHookPrime_SentinelOnlyWhenDelivered(t *testing.T) {
	for _, n := range []int{0, 3} {
		t.Run(fmt.Sprintf("N=%d", n), func(t *testing.T) {
			proj := setupAddTest(t)
			seedPrimeBrain(t, proj, n, 50, nil)
			t.Setenv("CLAUDE_SESSION_ID", "")
			oldStdin, oldEvent := hookStdin, hookPrimeEventFlag
			hookStdin = strings.NewReader(`{"session_id":"sess-thnd-ac3"}`)
			hookPrimeEventFlag = "sessionstart"
			t.Cleanup(func() { hookStdin, hookPrimeEventFlag = oldStdin, oldEvent })

			out := captureStdout(t, func() {
				if err := runHookEvent("prime"); err != nil {
					t.Errorf("runHookEvent: %v", err)
				}
			})
			st, _ := loadHookState("sess-thnd-ac3")
			recorded := st.EventsFired["session_start"] != ""
			if recorded != (n > 0) {
				t.Errorf("N=%d: sentinel recorded=%v, want %v (out=%q)", n, recorded, n > 0, out)
			}
			for id, s := range surfacedState(t, proj) {
				if n == 0 && s {
					t.Errorf("empty brain stamped %s", id[:8])
				}
			}
			if n == 0 && out != "" {
				t.Errorf("empty brain must be silent at sessionstart; got %q", out)
			}
		})
	}
}

// The GC notice rides after the footer, outside the frame, and never carries
// evicted content.
func TestHookPrime_GCNoticeAfterFooter(t *testing.T) {
	proj := setupAddTest(t)
	seedPrimeBrain(t, proj, 5, 50, nil)
	log := gcEntry("2026-10-11T22:00:01Z", false, "1a2b3c4d-0000") + "\n"
	if err := os.WriteFile(filepath.Join(proj, ".cerebro-gc.log"), []byte(log), 0o600); err != nil {
		t.Fatal(err)
	}
	text, _ := runPrime(t, "sessionstart")
	if strings.Contains(text, "SECRET") {
		t.Fatal("prime leaked evicted content from the GC log")
	}
	fi := strings.Index(text, compactFrameFooter)
	if fi < 0 {
		t.Fatalf("no frame footer in prime output: %q", text)
	}
	tail := text[fi+len(compactFrameFooter):]
	if !strings.HasPrefix(tail, "\nCerebro GC archived 1 memories at 2026-10-11T22:00:01Z (ids: 1a2b3c4d).") {
		t.Errorf("GC notice must follow the footer; tail=%q", tail)
	}
}

// ---- Phase 3 rework (security M1) ----

// callPrimeNoPanic runs hookPrimeWork and turns an escaping panic into a test
// failure instead of crashing the test binary.
func callPrimeNoPanic(t *testing.T, event string) (out string, primed bool, err error) {
	t.Helper()
	out = captureStdout(t, func() {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("hookPrimeWork panicked (a Go panic exits 2, which blocks a UserPromptSubmit prompt): %v", r)
			}
		}()
		primed, err = hookPrimeWork(event)
	})
	return out, primed, err
}

// M1: a huge stored prime_limit (config set accepts it; import carries it)
// must not panic the in-process hook, and must not cut the selection short.
func TestHookPrime_HugePrimeLimitDoesNotPanic(t *testing.T) {
	for _, event := range []string{"userpromptsubmit", "sessionstart"} {
		t.Run(event, func(t *testing.T) {
			proj := setupAddTest(t)
			seedPrimeBrain(t, proj, 50, 50, map[string]string{"prime_limit": "1000000000000"})
			out, primed, err := callPrimeNoPanic(t, event)
			if err != nil || !primed {
				t.Fatalf("primed=%v err=%v, want a delivered prime", primed, err)
			}
			text := out
			if event == "sessionstart" {
				var p primePayload
				if jerr := json.Unmarshal([]byte(out), &p); jerr != nil {
					t.Fatalf("not JSON: %v", jerr)
				}
				text = p.HookSpecificOutput.AdditionalContext
			}
			if len(text) > defaultCompactBudget {
				t.Errorf("output %d bytes exceeds the default budget", len(text))
			}
			if k := len(deliveredID8s(text)); k != 50 {
				t.Errorf("delivered %d of 50; the clamp must not cut the selection below the active-node count", k)
			}
		})
	}
}

// M1 / Design R5: any panic in selection or rendering degrades to an
// unprimed session (exit 0, retried on the next hook) with no stdout.
func TestHookPrime_RecoversPanic(t *testing.T) {
	proj := setupAddTest(t)
	seedPrimeBrain(t, proj, 3, 50, nil)
	old := hookPrimeSelect
	hookPrimeSelect = func(*brain.Brain, int) []store.Node { panic("forced selection failure") }
	t.Cleanup(func() { hookPrimeSelect = old })

	out, primed, err := callPrimeNoPanic(t, "userpromptsubmit")
	if err != nil || primed {
		t.Errorf("recovered panic must return (false, nil); got primed=%v err=%v", primed, err)
	}
	if out != "" {
		t.Errorf("recovered panic must leave stdout empty; got %q", out)
	}

	// Through the guard: runHookEvent returns nil (exit 0) and records no sentinel.
	t.Setenv("CLAUDE_SESSION_ID", "")
	oldStdin, oldEvent := hookStdin, hookPrimeEventFlag
	hookStdin = strings.NewReader(`{"session_id":"sess-thnd-m1"}`)
	hookPrimeEventFlag = "userpromptsubmit"
	t.Cleanup(func() { hookStdin, hookPrimeEventFlag = oldStdin, oldEvent })
	var herr error
	_ = captureStdout(t, func() {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("runHookEvent panicked: %v", r)
			}
		}()
		herr = runHookEvent("prime")
	})
	if herr != nil {
		t.Errorf("runHookEvent: %v, want nil", herr)
	}
	if st, _ := loadHookState("sess-thnd-m1"); st.EventsFired["session_start"] != "" {
		t.Error("a recovered panic must not record the session_start sentinel")
	}
}
