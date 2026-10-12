package main

// compact_recall_test.go — agentic-thnd AC5 + operator gate F4: recall
// --compact / --budget, flag validation, query-mode compact rendering with
// telemetry for delivered ids only, and budget-key resolution.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/coetzeevs/cerebro/internal/store"
)

var queryLineRE = regexp.MustCompile(`^- [0-9a-f]{8} (episode|concept|procedure|reflection) [0-9]+\.[0-9]{3} .+$`)

// resetRecallCompactState clears recall flag values and cobra's sticky
// Changed bits that leak across Execute on the shared rootCmd.
func resetRecallCompactState(t *testing.T) {
	t.Helper()
	c, _, err := rootCmd.Find([]string{"recall"})
	if err != nil {
		t.Fatal(err)
	}
	for name, def := range map[string]string{"compact": "false", "budget": "0", "prime": "false", "limit": "20", "threshold": "0.3", "global": "false"} {
		if f := c.Flags().Lookup(name); f != nil {
			_ = f.Value.Set(def)
			f.Changed = false
		}
	}
}

func runRecallCLI(t *testing.T, proj string, args ...string) (string, error) {
	t.Helper()
	resetRecallCompactState(t)
	t.Cleanup(func() { resetRecallCompactState(t) })
	return runCLI(t, proj, append([]string{"recall"}, args...)...)
}

func TestRecallCompact_FlagValidation(t *testing.T) {
	proj := setupAddTest(t)
	seedPrimeBrain(t, proj, 3, 50, nil)
	cases := []struct {
		args    []string
		wantErr string
	}{
		{[]string{"--prime", "--budget", "3000"}, "--budget requires --compact"},
		{[]string{"--prime", "--compact", "--format", "json"}, "--compact cannot be combined with --format json"},
		{[]string{"q", "--compact", "--with-provenance"}, "--compact cannot be combined with --with-provenance"},
		{[]string{"q", "--compact", "--provenance-depth", "2"}, "--compact cannot be combined with --with-provenance"},
		{[]string{"--prime", "--compact", "--budget", "1999"}, "--budget: must be between 2000 and 10000"},
		{[]string{"--prime", "--compact", "--budget", "10001"}, "--budget: must be between 2000 and 10000"},
	}
	for _, tc := range cases {
		_, err := runRecallCLI(t, proj, tc.args...)
		if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
			t.Errorf("recall %v: err=%v, want it to contain %q", tc.args, err, tc.wantErr)
		}
	}
}

func TestRecallCompact_PrimeUsesBudgetAndStampsDelivered(t *testing.T) {
	proj := setupAddTest(t)
	seedPrimeBrain(t, proj, 100, 300, map[string]string{"prime_limit": "100", "prime_budget": "2000"})
	// The GC trailer belongs to the hook only, never to recall.
	if err := os.WriteFile(filepath.Join(proj, ".cerebro-gc.log"),
		[]byte(gcEntry("2026-10-11T22:00:01Z", false, "1a2b3c4d-0")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := runRecallCLI(t, proj, "--prime", "--compact")
	if err != nil {
		t.Fatalf("recall --prime --compact: %v", err)
	}
	if len(out) > 2000 {
		t.Fatalf("output %d bytes exceeds prime_budget 2000", len(out))
	}
	if !strings.HasPrefix(out, "Cerebro online. ") || !strings.Contains(out, testFrameHeaderProject) || !strings.HasSuffix(out, compactFrameFooter+"\n") {
		t.Errorf("compact prime output not framed as specified:\n%s", out)
	}
	if strings.Contains(out, "Cerebro GC") {
		t.Error("recall --compact must not carry the GC notice")
	}
	got := deliveredID8s(out)
	if len(got) == 0 || len(got) >= 100 {
		t.Fatalf("expected 0 < k < 100 at budget 2000, k=%d", len(got))
	}
	delivered := map[string]bool{}
	for _, id8 := range got {
		delivered[id8] = true
	}
	for id, stamped := range surfacedState(t, proj) {
		if stamped != delivered[id[:8]] {
			t.Errorf("node %s: stamped=%v delivered=%v", id[:8], stamped, delivered[id[:8]])
		}
	}

	// --budget overrides prime_budget.
	out, err = runRecallCLI(t, proj, "--prime", "--compact", "--budget", "3000")
	if err != nil {
		t.Fatalf("recall --prime --compact --budget 3000: %v", err)
	}
	if len(out) <= 2000 || len(out) > 3000 {
		t.Errorf("--budget 3000 output is %d bytes; want (2000, 3000]", len(out))
	}
}

func TestRecallCompact_PrimeEmptyBrain(t *testing.T) {
	proj := setupAddTest(t)
	out, err := runRecallCLI(t, proj, "--prime", "--compact")
	if err != nil {
		t.Fatal(err)
	}
	if out != "No memories found.\n" {
		t.Errorf("empty brain: got %q", out)
	}
}

func TestRecallFull_PrimeUnchanged(t *testing.T) {
	// Without --compact, recall --prime keeps the full-content markdown.
	proj := setupAddTest(t)
	seedPrimeBrain(t, proj, 3, 50, nil)
	out, err := runRecallCLI(t, proj, "--prime")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "<cerebro-memories>") || !strings.Contains(out, "body line two") || !strings.HasPrefix(out, "## ") {
		t.Errorf("full-mode prime output changed:\n%s", out)
	}
}

func scoredFromBrain(t *testing.T, proj string) []store.ScoredNode {
	t.Helper()
	b := brainAtDir(t, proj)
	defer func() { _ = b.Close() }()
	nodes, err := b.List(store.ListNodesOpts{OrderBy: "importance"})
	if err != nil {
		t.Fatal(err)
	}
	res := make([]store.ScoredNode, len(nodes))
	for i := range nodes {
		res[i] = store.ScoredNode{Node: nodes[i], Score: 1.5 - float64(i)/100}
	}
	return res
}

func TestRenderCompactScored_GrammarAndHeader(t *testing.T) {
	proj := setupAddTest(t)
	seedPrimeBrain(t, proj, 5, 50, nil)
	res := scoredFromBrain(t, proj)

	out, delivered := renderCompactScored(res, defaultCompactBudget, false)
	if len(delivered) != 5 {
		t.Fatalf("delivered %d of 5", len(delivered))
	}
	if !strings.HasPrefix(out, "5 of 5 results (compact).\n<cerebro-memories>\n") {
		t.Errorf("query banner/frame: %q", strings.SplitN(out, "\n", 3)[:2])
	}
	if !strings.Contains(out, "Format: - <id> <type> <score> <headline>.") || !strings.Contains(out, "from this project's Cerebro brain.") {
		t.Error("query header must name the score metric and the project brain")
	}
	for _, l := range strings.Split(out, "\n") {
		if strings.HasPrefix(l, "- ") && !queryLineRE.MatchString(l) {
			t.Errorf("query line fails grammar: %q", l)
		}
	}

	// F4: --global must not claim the records come from "this project's" brain.
	gout, _ := renderCompactScored(res, defaultCompactBudget, true)
	if strings.Contains(gout, "this project's") || !strings.Contains(gout, "from the project and global Cerebro brains.") {
		t.Errorf("global header wording wrong:\n%s", strings.SplitN(gout, "\n", 4)[2])
	}

	if eout, d := renderCompactScored(nil, defaultCompactBudget, false); eout != "No relevant memories found.\n" || d != nil {
		t.Errorf("empty results: got %q %v", eout, d)
	}
}

func TestCompactQueryOutput_TouchesOnlyDelivered(t *testing.T) {
	proj := setupAddTest(t)
	seedPrimeBrain(t, proj, 100, 300, nil)
	res := scoredFromBrain(t, proj)
	before := map[string]int{}
	for i := range res {
		before[res[i].ID] = res[i].AccessCount
	}

	b := brainAtDir(t, proj)
	out := compactQueryOutput(b, res, minCompactBudget, false)
	_ = b.Close()
	if len(out) > minCompactBudget {
		t.Fatalf("output %d bytes exceeds budget", len(out))
	}
	delivered := map[string]bool{}
	for _, id8 := range deliveredID8s(out) {
		delivered[id8] = true
	}
	if len(delivered) == 0 || len(delivered) >= len(res) {
		t.Fatalf("expected 0 < k < N, k=%d", len(delivered))
	}
	for _, n := range scoredFromBrain(t, proj) {
		want := before[n.ID]
		if delivered[n.ID[:8]] {
			want++
		}
		if n.AccessCount != want {
			t.Errorf("node %s: access_count %d, want %d (delivered=%v)", n.ID[:8], n.AccessCount, want, delivered[n.ID[:8]])
		}
	}
}

// F4: `recall --prime "<q>" --compact` reaches the query branch and uses
// recall_budget; prime without a query uses prime_budget; --budget wins.
func TestCompactBudget_KeySelection(t *testing.T) {
	b := testBrainForConfig(t)
	s := b.Store()
	_ = s.SetMeta(configMetaPrefix+"prime_budget", "2000")
	_ = s.SetMeta(configMetaPrefix+"recall_budget", "3000")
	if got := compactBudget(s, false, 0, true); got != 2000 {
		t.Errorf("prime without query: %d, want prime_budget 2000", got)
	}
	if got := compactBudget(s, false, 0, false); got != 3000 {
		t.Errorf("query (incl. --prime \"<q>\"): %d, want recall_budget 3000", got)
	}
	if got := compactBudget(s, true, 4000, true); got != 4000 {
		t.Errorf("--budget: %d, want 4000", got)
	}
}
