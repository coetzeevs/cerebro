package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/coetzeevs/cerebro/internal/store"
)

// captureEval executes the eval command with the given args and returns
// (stdout bytes, stderr bytes, error). Streams are redirected independently
// so that metric output (stdout) and preflight/progress messages (stderr)
// can be asserted separately — per R6 stdout-purity discipline.
//
// Cobra persistent flags (-p, -f, -q) are registered once at package init.
// We reset only the bound global variables between calls; do NOT reset the
// root command's persistent flag set (that would unregister -p/-f/-q).
func captureEval(t *testing.T, args ...string) (stdout, stderr []byte, err error) {
	t.Helper()

	// Reset global flag variables so each test starts clean.
	projectFlag = ""
	formatFlag = "md"
	quietFlag = false

	// Reset eval-specific flags to defaults.
	evalQueriesFlag = "docs/evals/queries.jsonl"
	evalGroundTruthFlag = "docs/evals/ground-truth.jsonl"
	evalCorpusFlag = "docs/evals/corpus.md"
	evalOutFlag = "docs/evals/baseline.json"
	evalLimitFlag = 20
	evalThresholdFlag = 0.3

	var outBuf, errBuf bytes.Buffer

	rootCmd.SetOut(&outBuf)
	rootCmd.SetErr(&errBuf)
	evalCmd.SetOut(&outBuf)
	evalCmd.SetErr(&errBuf)

	// Suppress cobra's auto-usage-on-error so stdout stays clean on error
	// paths. Restore original SilenceUsage after the call.
	prev := evalCmd.SilenceUsage
	evalCmd.SilenceUsage = true
	defer func() { evalCmd.SilenceUsage = prev }()

	rootCmd.SetArgs(append([]string{"eval"}, args...))

	err = rootCmd.Execute()
	return outBuf.Bytes(), errBuf.Bytes(), err
}

// ---- AC1: --help exits 0 and references corpus/queries/ground-truth flags ----

// TestEvalHelp verifies AC1: eval --help exits 0 and stdout references the
// three key flags (--corpus, --queries, --ground-truth).
func TestEvalHelp(t *testing.T) {
	stdout, _, err := captureEval(t, "--help")
	if err != nil {
		t.Fatalf("eval --help returned error: %v", err)
	}

	out := string(stdout)
	for _, flag := range []string{"--corpus", "--queries", "--ground-truth"} {
		if !strings.Contains(out, flag) {
			t.Errorf("expected --help output to reference %q; stdout=%q", flag, out)
		}
	}
}

// ---- Pure metric helpers: computeRecallAtK ----

// TestComputeRecallAtK_AllRelevantInTopK verifies perfect recall when all
// ground-truth nodes appear in the top-K returned list.
func TestComputeRecallAtK_AllRelevantInTopK(t *testing.T) {
	gt := map[string]struct{}{
		"aaa": {},
		"bbb": {},
	}
	nodes := []store.ScoredNode{
		{Node: store.Node{ID: "aaa"}},
		{Node: store.Node{ID: "bbb"}},
		{Node: store.Node{ID: "ccc"}},
	}

	got := computeRecallAtK(nodes, gt, 5)
	if got != 1.0 {
		t.Errorf("expected recall@5=1.0 when all relevant in top-5, got %.4f", got)
	}
}

// TestComputeRecallAtK_NoneInTopK verifies 0.0 when no relevant nodes appear.
func TestComputeRecallAtK_NoneInTopK(t *testing.T) {
	gt := map[string]struct{}{
		"aaa": {},
		"bbb": {},
	}
	nodes := []store.ScoredNode{
		{Node: store.Node{ID: "ccc"}},
		{Node: store.Node{ID: "ddd"}},
	}

	got := computeRecallAtK(nodes, gt, 5)
	if got != 0.0 {
		t.Errorf("expected recall@5=0.0 when no relevant nodes found, got %.4f", got)
	}
}

// TestComputeRecallAtK_PartialRelevance verifies fractional recall.
func TestComputeRecallAtK_PartialRelevance(t *testing.T) {
	gt := map[string]struct{}{
		"aaa": {},
		"bbb": {},
		"ccc": {},
		"ddd": {},
	}
	nodes := []store.ScoredNode{
		{Node: store.Node{ID: "aaa"}},
		{Node: store.Node{ID: "zzz"}},
		{Node: store.Node{ID: "bbb"}},
	}

	got := computeRecallAtK(nodes, gt, 10)
	// 2 of 4 relevant nodes found → recall = 0.5
	const want = 0.5
	if got != want {
		t.Errorf("expected recall@10=%.4f, got %.4f", want, got)
	}
}

// TestComputeRecallAtK_KCutsOff verifies that nodes beyond K are not counted.
func TestComputeRecallAtK_KCutsOff(t *testing.T) {
	gt := map[string]struct{}{
		"aaa": {},
		"bbb": {},
	}
	// bbb is at position 3, beyond K=2.
	nodes := []store.ScoredNode{
		{Node: store.Node{ID: "aaa"}},
		{Node: store.Node{ID: "zzz"}},
		{Node: store.Node{ID: "bbb"}},
	}

	// With K=2, only aaa is considered → recall = 0.5
	got := computeRecallAtK(nodes, gt, 2)
	const want = 0.5
	if got != want {
		t.Errorf("expected recall@2=%.4f (K cutoff), got %.4f", want, got)
	}
}

// TestComputeRecallAtK_EmptyGroundTruth verifies that an empty GT set returns 0.
// Guards against divide-by-zero.
func TestComputeRecallAtK_EmptyGroundTruth(t *testing.T) {
	nodes := []store.ScoredNode{
		{Node: store.Node{ID: "aaa"}},
	}
	got := computeRecallAtK(nodes, map[string]struct{}{}, 5)
	if got != 0.0 {
		t.Errorf("expected recall=0.0 for empty ground truth, got %.4f", got)
	}
}

// TestComputeRecallAtK_EmptyResults verifies 0.0 when the returned list is empty.
func TestComputeRecallAtK_EmptyResults(t *testing.T) {
	gt := map[string]struct{}{"aaa": {}}
	got := computeRecallAtK(nil, gt, 5)
	if got != 0.0 {
		t.Errorf("expected recall=0.0 for empty results, got %.4f", got)
	}
}

// ---- Pure metric helpers: computeMRR ----

// TestComputeMRR_FirstHitAtRank1 verifies MRR=1.0 when the first result is relevant.
func TestComputeMRR_FirstHitAtRank1(t *testing.T) {
	gt := map[string]struct{}{"aaa": {}}
	nodes := []store.ScoredNode{
		{Node: store.Node{ID: "aaa"}},
		{Node: store.Node{ID: "bbb"}},
	}
	got := computeMRR(nodes, gt)
	if got != 1.0 {
		t.Errorf("expected MRR=1.0 for first-hit at rank 1, got %.4f", got)
	}
}

// TestComputeMRR_FirstHitAtRank2 verifies MRR=0.5 when the second result is the first hit.
func TestComputeMRR_FirstHitAtRank2(t *testing.T) {
	gt := map[string]struct{}{"bbb": {}}
	nodes := []store.ScoredNode{
		{Node: store.Node{ID: "aaa"}},
		{Node: store.Node{ID: "bbb"}},
	}
	got := computeMRR(nodes, gt)
	const want = 0.5
	if got != want {
		t.Errorf("expected MRR=%.4f for first-hit at rank 2, got %.4f", want, got)
	}
}

// TestComputeMRR_NoHit verifies MRR=0.0 when no relevant node appears.
func TestComputeMRR_NoHit(t *testing.T) {
	gt := map[string]struct{}{"zzz": {}}
	nodes := []store.ScoredNode{
		{Node: store.Node{ID: "aaa"}},
		{Node: store.Node{ID: "bbb"}},
	}
	got := computeMRR(nodes, gt)
	if got != 0.0 {
		t.Errorf("expected MRR=0.0 when no hit, got %.4f", got)
	}
}

// TestComputeMRR_EmptyGroundTruth verifies MRR=0.0 for an empty GT set.
func TestComputeMRR_EmptyGroundTruth(t *testing.T) {
	nodes := []store.ScoredNode{
		{Node: store.Node{ID: "aaa"}},
	}
	got := computeMRR(nodes, map[string]struct{}{})
	if got != 0.0 {
		t.Errorf("expected MRR=0.0 for empty ground truth, got %.4f", got)
	}
}

// TestComputeMRR_EmptyResults verifies MRR=0.0 for empty results.
func TestComputeMRR_EmptyResults(t *testing.T) {
	gt := map[string]struct{}{"aaa": {}}
	got := computeMRR(nil, gt)
	if got != 0.0 {
		t.Errorf("expected MRR=0.0 for empty results, got %.4f", got)
	}
}

// ---- macroRecallAtK / macroMRR aggregation helpers ----

// TestMacroMetrics_SingleQuery verifies that single-query macro average equals the query value.
func TestMacroMetrics_SingleQuery(t *testing.T) {
	// One query with 2 relevant, 1 hit in top-5 → recall@5 = 0.5
	results := []queryResult{
		{
			QueryID: "q1",
			Hits:    []store.ScoredNode{{Node: store.Node{ID: "aaa"}}},
			GT:      map[string]struct{}{"aaa": {}, "bbb": {}},
		},
	}
	r5 := macroAvgRecallAtK(results, 5)
	if r5 != 0.5 {
		t.Errorf("macro recall@5 expected 0.5, got %.4f", r5)
	}
	mrr := macroMRR(results)
	if mrr != 1.0 {
		t.Errorf("macro MRR expected 1.0 (first hit), got %.4f", mrr)
	}
}

// TestMacroMetrics_MultiQuery verifies averaging across multiple queries.
func TestMacroMetrics_MultiQuery(t *testing.T) {
	results := []queryResult{
		{
			QueryID: "q1",
			Hits:    []store.ScoredNode{{Node: store.Node{ID: "aaa"}}},
			GT:      map[string]struct{}{"aaa": {}},
		},
		{
			QueryID: "q2",
			Hits:    []store.ScoredNode{{Node: store.Node{ID: "zzz"}}},
			GT:      map[string]struct{}{"aaa": {}},
		},
	}
	// q1: recall@5=1.0, q2: recall@5=0.0 → macro=0.5
	r5 := macroAvgRecallAtK(results, 5)
	const want = 0.5
	if r5 != want {
		t.Errorf("macro recall@5 expected %.4f, got %.4f", want, r5)
	}
}

// TestMacroMetrics_EmptyResults verifies 0.0 for an empty query list.
func TestMacroMetrics_EmptyResults(t *testing.T) {
	r5 := macroAvgRecallAtK(nil, 5)
	if r5 != 0.0 {
		t.Errorf("macro recall@5 expected 0.0 for empty list, got %.4f", r5)
	}
	mrr := macroMRR(nil)
	if mrr != 0.0 {
		t.Errorf("macro MRR expected 0.0 for empty list, got %.4f", mrr)
	}
}

// ---- AC2b: ground-truth preflight against an in-test brain ----

// TestGroundTruthPreflight_AllActive verifies that a ground-truth set where every
// ID resolves to an active node returns no missing IDs.
func TestGroundTruthPreflight_AllActive(t *testing.T) {
	b := testBrain(t) // noop embedder — no Ollama needed

	id1, err := b.Add("memory about composite scoring weights", store.TypeConcept)
	if err != nil {
		t.Fatalf("Add concept: %v", err)
	}
	id2, err := b.Add("Brain.Search returns early without embedder", store.TypeProcedure)
	if err != nil {
		t.Fatalf("Add procedure: %v", err)
	}

	gtByQuery := map[string]map[string]struct{}{
		"q1": {id1: {}},
		"q2": {id1: {}, id2: {}},
	}

	missing := groundTruthPreflight(b.Store(), gtByQuery)
	if len(missing) != 0 {
		t.Errorf("expected no missing IDs, got %v", missing)
	}
}

// TestGroundTruthPreflight_MissingID verifies that a non-existent ID is reported.
func TestGroundTruthPreflight_MissingID(t *testing.T) {
	b := testBrain(t)

	id1, err := b.Add("valid memory", store.TypeConcept)
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	const fakeID = "deadbeef-0000-0000-0000-000000000000"
	gtByQuery := map[string]map[string]struct{}{
		"q1": {id1: {}, fakeID: {}},
	}

	missing := groundTruthPreflight(b.Store(), gtByQuery)
	found := false
	for _, m := range missing {
		if m == fakeID {
			found = true
		}
	}
	if !found {
		t.Errorf("expected fakeID in missing list, got %v", missing)
	}
}

// ---- AC4: Baseline JSON shape ----

// TestBaselineJSONShape verifies that buildBaseline produces valid JSON with
// the required AC4 keys and values.
func TestBaselineJSONShape(t *testing.T) {
	metrics := evalMetrics{
		RecallAt5:  0.6,
		RecallAt10: 0.7,
		RecallAt20: 0.8,
		MRR:        0.65,
	}
	params := evalParams{
		Limit:     20,
		Threshold: 0.3,
		Queries:   10,
	}
	brainInfo := brainSummary{
		SchemaVersion:  "2",
		ActiveNodes:    537,
		EmbeddingModel: "nomic-embed-text",
		Dimensions:     768,
	}

	bl := buildBaseline(brainInfo, params, metrics)

	// Verify scorer weights exactly (AC4 contract).
	if bl.ScorerWeights.Relevance != 0.35 {
		t.Errorf("scorer_weights.relevance expected 0.35, got %f", bl.ScorerWeights.Relevance)
	}
	if bl.ScorerWeights.Importance != 0.25 {
		t.Errorf("scorer_weights.importance expected 0.25, got %f", bl.ScorerWeights.Importance)
	}
	if bl.ScorerWeights.Recency != 0.25 {
		t.Errorf("scorer_weights.recency expected 0.25, got %f", bl.ScorerWeights.Recency)
	}
	if bl.ScorerWeights.Structural != 0.15 {
		t.Errorf("scorer_weights.structural expected 0.15, got %f", bl.ScorerWeights.Structural)
	}

	// Verify metric values are reflected in output.
	if bl.Metrics.RecallAt5 != metrics.RecallAt5 {
		t.Errorf("recall@5 mismatch: want %.4f got %.4f", metrics.RecallAt5, bl.Metrics.RecallAt5)
	}
	if bl.Metrics.MRR != metrics.MRR {
		t.Errorf("MRR mismatch: want %.4f got %.4f", metrics.MRR, bl.Metrics.MRR)
	}

	// Verify brain info passes through.
	if bl.Brain.ActiveNodes != brainInfo.ActiveNodes {
		t.Errorf("brain.active_nodes mismatch: want %d got %d", brainInfo.ActiveNodes, bl.Brain.ActiveNodes)
	}

	// Verify it serialises to valid JSON.
	data, err := json.Marshal(bl)
	if err != nil {
		t.Fatalf("baseline marshals to invalid JSON: %v", err)
	}
	var check map[string]any
	if err := json.Unmarshal(data, &check); err != nil {
		t.Fatalf("baseline JSON parse error: %v", err)
	}

	for _, key := range []string{"generated_at", "brain", "scorer_weights", "params", "metrics"} {
		if _, ok := check[key]; !ok {
			t.Errorf("expected baseline JSON key %q not found", key)
		}
	}
}

// TestBaselineJSONShape_MetricsInRange verifies that all metric values from
// buildBaseline satisfy the [0.0, 1.0] range required by AC3a/AC3b.
func TestBaselineJSONShape_MetricsInRange(t *testing.T) {
	for _, tt := range []struct {
		name    string
		metrics evalMetrics
	}{
		{"zeros", evalMetrics{0.0, 0.0, 0.0, 0.0}},
		{"ones", evalMetrics{1.0, 1.0, 1.0, 1.0}},
		{"typical", evalMetrics{0.6, 0.7, 0.8, 0.65}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			bl := buildBaseline(
				brainSummary{SchemaVersion: "2", ActiveNodes: 100, EmbeddingModel: "m", Dimensions: 768},
				evalParams{Limit: 20, Threshold: 0.3, Queries: 5},
				tt.metrics,
			)
			for _, v := range []float64{
				bl.Metrics.RecallAt5,
				bl.Metrics.RecallAt10,
				bl.Metrics.RecallAt20,
				bl.Metrics.MRR,
			} {
				if v < 0.0 || v > 1.0 {
					t.Errorf("metric value %.4f outside [0.0, 1.0]", v)
				}
			}
		})
	}
}

// ---- AC4: --out write discipline (S-INFO-3) ----

// TestEvalOutWriteDiscipline verifies that the --out path's parent directory is
// created with MkdirAll when it does not yet exist, and that a valid JSON file
// is written there. This covers S-INFO-3 without Ollama.
func TestEvalOutWriteDiscipline(t *testing.T) {
	outDir := filepath.Join(t.TempDir(), "nested", "output")
	outPath := filepath.Join(outDir, "baseline.json")

	bl := buildBaseline(
		brainSummary{SchemaVersion: "2", ActiveNodes: 10, EmbeddingModel: "noop", Dimensions: 0},
		evalParams{Limit: 20, Threshold: 0.3, Queries: 0},
		evalMetrics{0.0, 0.0, 0.0, 0.0},
	)
	if err := writeBaseline(&bl, outPath); err != nil {
		t.Fatalf("writeBaseline: %v", err)
	}

	// Verify file exists and is valid JSON.
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("reading baseline output: %v", err)
	}
	var check map[string]any
	if err := json.Unmarshal(data, &check); err != nil {
		t.Fatalf("baseline output is not valid JSON: %v", err)
	}

	// Verify directory perms match 0o750 (or narrower — file system may mask).
	info, err := os.Stat(outDir)
	if err != nil {
		t.Fatalf("stat output dir: %v", err)
	}
	// Allow 0o750 or any subset (e.g. 0o700 on some filesystems).
	perm := info.Mode().Perm()
	if perm&0o022 != 0 {
		t.Errorf("output directory has world/group-write bits set: %o", perm)
	}
}

// ---- JSONL parse helpers ----

// TestParseQueriesJSONL verifies round-trip of queries.jsonl format.
func TestParseQueriesJSONL(t *testing.T) {
	content := `{"id":"q1","query":"composite scoring formula","note":"score weights"}
{"id":"q2","query":"Brain.Search API usage","note":"public API"}
`
	path := filepath.Join(t.TempDir(), "queries.jsonl")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	queries, err := parseQueriesJSONL(path)
	if err != nil {
		t.Fatalf("parseQueriesJSONL: %v", err)
	}
	if len(queries) != 2 {
		t.Fatalf("expected 2 queries, got %d", len(queries))
	}
	if queries[0].ID != "q1" || queries[0].Query != "composite scoring formula" {
		t.Errorf("unexpected first query: %+v", queries[0])
	}
	if queries[1].ID != "q2" {
		t.Errorf("unexpected second query ID: %q", queries[1].ID)
	}
}

// TestParseGroundTruthJSONL verifies round-trip of ground-truth.jsonl format.
func TestParseGroundTruthJSONL(t *testing.T) {
	content := `{"query_id":"q1","relevant_node_ids":["aaa-bbb","ccc-ddd"]}
{"query_id":"q2","relevant_node_ids":["eee-fff"]}
`
	path := filepath.Join(t.TempDir(), "gt.jsonl")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	gt, err := parseGroundTruthJSONL(path)
	if err != nil {
		t.Fatalf("parseGroundTruthJSONL: %v", err)
	}
	if len(gt) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(gt))
	}
	if _, ok := gt["q1"]["aaa-bbb"]; !ok {
		t.Errorf("expected aaa-bbb in q1 ground truth")
	}
	if _, ok := gt["q2"]["eee-fff"]; !ok {
		t.Errorf("expected eee-fff in q2 ground truth")
	}
}

// ---- applyConfigFlag helper: already used in cmd_recall.go ----

// (Tested implicitly via captureEval; no separate unit test needed for this
// existing helper since it is covered by cmd_recall_test.go patterns.)

// ---- Testable brain setup for integration-like tests ----
