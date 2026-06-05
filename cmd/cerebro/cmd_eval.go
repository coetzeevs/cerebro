package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/coetzeevs/cerebro/internal/store"
	"github.com/spf13/cobra"
)

// Eval command flag variables — package-level so tests can reset them.
var (
	evalQueriesFlag     string
	evalGroundTruthFlag string
	evalCorpusFlag      string
	evalOutFlag         string
	evalLimitFlag       int
	evalThresholdFlag   float64
)

// evalCmd is exported at package scope so tests can set SilenceUsage on it.
var evalCmd *cobra.Command

func init() {
	evalCmd = &cobra.Command{
		Use:   "eval",
		Short: "Measure recall quality against a ground-truth corpus",
		Long: `Eval runs the recall-quality evaluation harness against a real brain.

For each query in --queries, it calls Brain.Search(limit, threshold) and
measures recall@5, recall@10, recall@20, and MRR against the --ground-truth
file. Results are printed to stdout and written to --out as a baseline JSON
artifact.

Prerequisites:
  - Ollama must be running locally (default: http://localhost:11434)
  - The brain must have been initialised with a real embedding provider
  - Run 'cerebro stats -p <dir>' to verify embedding model and node count

Corpus files default to docs/evals/ relative to the working directory.
Use -p / --project to point at the brain under evaluation.`,
		RunE: runEval,
	}

	evalCmd.Flags().StringVar(&evalQueriesFlag, "queries",
		"docs/evals/queries.jsonl",
		"Path to queries JSONL file (one {id, query, note} per line)")
	evalCmd.Flags().StringVar(&evalGroundTruthFlag, "ground-truth",
		"docs/evals/ground-truth.jsonl",
		"Path to ground-truth JSONL file (one {query_id, relevant_node_ids} per line)")
	evalCmd.Flags().StringVar(&evalCorpusFlag, "corpus",
		"docs/evals/corpus.md",
		"Path to corpus provenance manifest")
	evalCmd.Flags().StringVar(&evalOutFlag, "out",
		"docs/evals/baseline.json",
		"Path to write the baseline JSON artifact")
	evalCmd.Flags().IntVarP(&evalLimitFlag, "limit", "l", 20,
		"Top-K ceiling for Brain.Search (covers recall@5, @10, @20)")
	evalCmd.Flags().Float64VarP(&evalThresholdFlag, "threshold", "T", 0.3,
		"Minimum similarity threshold passed to Brain.Search")

	rootCmd.AddCommand(evalCmd)
}

// ── File-format types ──────────────────────────────────────────────────────

// evalQuery represents one line from queries.jsonl.
type evalQuery struct {
	ID    string `json:"id"`
	Query string `json:"query"`
	Note  string `json:"note,omitempty"`
}

// evalGroundTruthEntry represents one line from ground-truth.jsonl.
type evalGroundTruthEntry struct {
	QueryID         string   `json:"query_id"`
	RelevantNodeIDs []string `json:"relevant_node_ids"`
}

// ── Baseline output types (AC4) ────────────────────────────────────────────

// brainSummary records which brain was measured (path class, not raw path).
type brainSummary struct {
	SchemaVersion  string `json:"schema_version"`
	ActiveNodes    int    `json:"active_nodes"`
	EmbeddingModel string `json:"embedding_model"`
	Dimensions     int    `json:"dimensions"`
}

// scorerWeights records the composite scorer formula coefficients.
// These are the literal values from internal/store/search.go:compositeScore.
type scorerWeights struct {
	Relevance  float64 `json:"relevance"`
	Importance float64 `json:"importance"`
	Recency    float64 `json:"recency"`
	Structural float64 `json:"structural"`
}

// evalParams records the harness configuration for the run.
type evalParams struct {
	Limit     int     `json:"limit"`
	Threshold float64 `json:"threshold"`
	Queries   int     `json:"queries"`
}

// evalMetrics holds the aggregate recall@K and MRR values.
type evalMetrics struct {
	RecallAt5  float64 `json:"recall@5"`
	RecallAt10 float64 `json:"recall@10"`
	RecallAt20 float64 `json:"recall@20"`
	MRR        float64 `json:"MRR"`
}

// baselineOutput is the AC4 baseline JSON artifact.
type baselineOutput struct {
	GeneratedAt   string        `json:"generated_at"`
	Brain         brainSummary  `json:"brain"`
	ScorerWeights scorerWeights `json:"scorer_weights"`
	Params        evalParams    `json:"params"`
	Metrics       evalMetrics   `json:"metrics"`
}

// queryResult is an internal aggregate of per-query search results + ground truth.
type queryResult struct {
	QueryID string
	Hits    []store.ScoredNode
	GT      map[string]struct{}
}

// ── Parser helpers ─────────────────────────────────────────────────────────

// parseQueriesJSONL reads a queries.jsonl file into a slice of evalQuery.
func parseQueriesJSONL(path string) ([]evalQuery, error) {
	f, err := os.Open(path) //nolint:gosec // operator-supplied path; operator-run CLI only
	if err != nil {
		return nil, fmt.Errorf("opening queries file %q: %w", path, err)
	}
	defer f.Close() //nolint:errcheck // read-only

	var queries []evalQuery
	scanner := bufio.NewScanner(f)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "//") {
			continue
		}
		var q evalQuery
		if err := json.Unmarshal([]byte(line), &q); err != nil {
			return nil, fmt.Errorf("queries line %d: %w", lineNum, err)
		}
		if q.ID == "" {
			return nil, fmt.Errorf("queries line %d: missing id field", lineNum)
		}
		if q.Query == "" {
			return nil, fmt.Errorf("queries line %d: missing query field", lineNum)
		}
		queries = append(queries, q)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading queries file: %w", err)
	}
	return queries, nil
}

// parseGroundTruthJSONL reads a ground-truth.jsonl file into a map from
// query_id to the set of relevant node IDs.
func parseGroundTruthJSONL(path string) (map[string]map[string]struct{}, error) {
	f, err := os.Open(path) //nolint:gosec // operator-supplied path
	if err != nil {
		return nil, fmt.Errorf("opening ground-truth file %q: %w", path, err)
	}
	defer f.Close() //nolint:errcheck // read-only

	result := make(map[string]map[string]struct{})
	scanner := bufio.NewScanner(f)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "//") {
			continue
		}
		var entry evalGroundTruthEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			return nil, fmt.Errorf("ground-truth line %d: %w", lineNum, err)
		}
		if entry.QueryID == "" {
			return nil, fmt.Errorf("ground-truth line %d: missing query_id", lineNum)
		}
		set := make(map[string]struct{}, len(entry.RelevantNodeIDs))
		for _, id := range entry.RelevantNodeIDs {
			set[id] = struct{}{}
		}
		result[entry.QueryID] = set
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading ground-truth file: %w", err)
	}
	return result, nil
}

// ── Pure metric helpers (unit-testable without Ollama) ─────────────────────

// computeRecallAtK returns |gtSet ∩ top-K(nodes)| / |gtSet|.
// Returns 0.0 if gtSet is empty (guards divide-by-zero).
func computeRecallAtK(nodes []store.ScoredNode, gtSet map[string]struct{}, k int) float64 {
	if len(gtSet) == 0 {
		return 0.0
	}
	limit := k
	if len(nodes) < limit {
		limit = len(nodes)
	}
	hits := 0
	for i := 0; i < limit; i++ {
		if _, ok := gtSet[nodes[i].ID]; ok {
			hits++
		}
	}
	return float64(hits) / float64(len(gtSet))
}

// computeMRR returns the reciprocal rank of the first relevant hit.
// Returns 0.0 if gtSet is empty or no relevant node appears in nodes.
func computeMRR(nodes []store.ScoredNode, gtSet map[string]struct{}) float64 {
	if len(gtSet) == 0 {
		return 0.0
	}
	for i := range nodes { //nolint:gocritic // rangeValCopy: indexing avoids copy
		if _, ok := gtSet[nodes[i].ID]; ok {
			return 1.0 / float64(i+1)
		}
	}
	return 0.0
}

// macroAvgRecallAtK computes the macro-averaged recall@K across all query results.
// Returns 0.0 for an empty slice.
func macroAvgRecallAtK(results []queryResult, k int) float64 {
	if len(results) == 0 {
		return 0.0
	}
	sum := 0.0
	for _, r := range results {
		sum += computeRecallAtK(r.Hits, r.GT, k)
	}
	return sum / float64(len(results))
}

// macroMRR computes the macro-averaged MRR across all query results.
// Returns 0.0 for an empty slice.
func macroMRR(results []queryResult) float64 {
	if len(results) == 0 {
		return 0.0
	}
	sum := 0.0
	for _, r := range results {
		sum += computeMRR(r.Hits, r.GT)
	}
	return sum / float64(len(results))
}

// ── Ground-truth preflight (AC2b) ─────────────────────────────────────────

// groundTruthPreflight validates that every node ID referenced in the
// ground-truth set exists in the nodes table with status='active'.
// Returns a slice of IDs that are absent or non-active (may be empty).
func groundTruthPreflight(s *store.Store, gtByQuery map[string]map[string]struct{}) []string {
	// Collect unique IDs across all queries.
	all := make(map[string]struct{})
	for _, ids := range gtByQuery {
		for id := range ids {
			all[id] = struct{}{}
		}
	}

	var missing []string
	for id := range all {
		node, err := s.GetNode(id)
		if err != nil || node == nil || node.Status != "active" {
			missing = append(missing, id)
		}
	}
	return missing
}

// ── Baseline builder (AC4) ─────────────────────────────────────────────────

// buildBaseline constructs the baseline JSON artifact. The scorer weights are
// the literal values from internal/store/search.go:compositeScore (AC4 contract).
func buildBaseline(bi brainSummary, params evalParams, metrics evalMetrics) baselineOutput {
	return baselineOutput{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Brain:       bi,
		// Literal weights from search.go:compositeScore — AC4 contract.
		ScorerWeights: scorerWeights{
			Relevance:  0.35,
			Importance: 0.25,
			Recency:    0.25,
			Structural: 0.15,
		},
		Params:  params,
		Metrics: metrics,
	}
}

// writeBaseline writes the baseline JSON to outPath.
// Creates the parent directory with 0o750 MkdirAll (S-INFO-3) — never follows
// symlinks, no arbitrary-path write beyond the intended output.
func writeBaseline(bl *baselineOutput, outPath string) error { //nolint:gocritic // hugeParam: pointer by design
	dir := filepath.Dir(outPath)
	// S-INFO-3: create only the parent dir, 0o750, no symlink-follow.
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("creating output directory %q: %w", dir, err)
	}

	enc, err := json.MarshalIndent(bl, "", "  ")
	if err != nil {
		return fmt.Errorf("marshalling baseline: %w", err)
	}
	enc = append(enc, '\n')

	// Write to a temp file first then rename to avoid partial writes.
	tmp, err := os.CreateTemp(dir, "baseline-*.json.tmp")
	if err != nil {
		return fmt.Errorf("creating temp baseline file: %w", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }() // clean up on error

	if _, err := tmp.Write(enc); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("writing baseline: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("closing baseline temp file: %w", err)
	}
	if err := os.Rename(tmpName, outPath); err != nil {
		return fmt.Errorf("renaming baseline to %q: %w", outPath, err)
	}
	return nil
}

// ── Main eval runner ───────────────────────────────────────────────────────

func runEval(cmd *cobra.Command, _ []string) error {
	// --- Parse corpus files ---
	queries, err := parseQueriesJSONL(evalQueriesFlag)
	if err != nil {
		return err
	}
	gtByQuery, err := parseGroundTruthJSONL(evalGroundTruthFlag)
	if err != nil {
		return err
	}

	// --- Open brain ---
	b, err := openBrain()
	if err != nil {
		return err
	}
	defer func() { _ = b.Close() }()

	// --- AC2b preflight: validate ground-truth IDs against live nodes table ---
	missing := groundTruthPreflight(b.Store(), gtByQuery)
	for _, id := range missing {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "preflight warning: ground-truth node %q not active in brain — skipping\n", id)
	}
	// Remove missing IDs from ground-truth sets so they don't corrupt recall denominators.
	for qid, ids := range gtByQuery {
		for _, id := range missing {
			delete(ids, id)
		}
		gtByQuery[qid] = ids
	}

	// --- Brain stats for baseline (N1: never hardcode node count) ---
	stats, err := b.Stats()
	if err != nil {
		return fmt.Errorf("getting brain stats: %w", err)
	}
	dims := 0
	if stats.EmbeddingDimensions != "" {
		_, _ = fmt.Sscanf(stats.EmbeddingDimensions, "%d", &dims) //nolint:errcheck // best-effort parse
	}
	bi := brainSummary{
		SchemaVersion:  stats.SchemaVersion,
		ActiveNodes:    stats.ActiveNodes,
		EmbeddingModel: stats.EmbeddingModel,
		Dimensions:     dims,
	}

	// --- Run queries against Brain.Search ---
	var results []queryResult
	for _, q := range queries {
		gtSet, ok := gtByQuery[q.ID]
		if !ok {
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: no ground truth for query %q — skipping\n", q.ID)
			continue
		}
		if len(gtSet) == 0 {
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: ground truth for query %q is empty after preflight — skipping\n", q.ID)
			continue
		}

		// subtypeFilter=nil: measure the whole pipeline, no subtype filtering.
		hits, searchErr := b.Search(context.Background(), q.Query, evalLimitFlag, evalThresholdFlag, nil)
		if searchErr != nil {
			// R1: Ollama may be unavailable. Surface the error and bail — do NOT fabricate zeros.
			return fmt.Errorf("Brain.Search for query %q: %w (NOTE: Ollama must be running locally — e.g. 'ollama serve')", q.ID, searchErr)
		}

		results = append(results, queryResult{
			QueryID: q.ID,
			Hits:    hits,
			GT:      gtSet,
		})
	}

	// --- Compute aggregate metrics ---
	metrics := evalMetrics{
		RecallAt5:  macroAvgRecallAtK(results, 5),
		RecallAt10: macroAvgRecallAtK(results, 10),
		RecallAt20: macroAvgRecallAtK(results, 20),
		MRR:        macroMRR(results),
	}
	params := evalParams{
		Limit:     evalLimitFlag,
		Threshold: evalThresholdFlag,
		Queries:   len(results),
	}

	// --- Print metrics to stdout (AC3a/AC3b) ---
	// Stdout carries only the metric report; progress/warnings go to stderr (R6).
	out := cmd.OutOrStdout()
	_, _ = fmt.Fprintf(out, "recall@5:  %.4f\n", metrics.RecallAt5)
	_, _ = fmt.Fprintf(out, "recall@10: %.4f\n", metrics.RecallAt10)
	_, _ = fmt.Fprintf(out, "recall@20: %.4f\n", metrics.RecallAt20)
	_, _ = fmt.Fprintf(out, "MRR:       %.4f\n", metrics.MRR)
	_, _ = fmt.Fprintf(out, "queries evaluated: %d\n", len(results))

	// --- Write baseline JSON artifact (AC4) ---
	bl := buildBaseline(bi, params, metrics)
	if err := writeBaseline(&bl, evalOutFlag); err != nil {
		return fmt.Errorf("writing baseline to %q: %w", evalOutFlag, err)
	}
	_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "baseline written to %s\n", evalOutFlag)

	return nil
}
