package store

// outcome_scoring.go — outcome- and citation-based reinforcement signals
// (agentic-do71). Model B: the AGENT supplies the outcome ("that memory led
// to a working fix" / "that memory misled me"); cerebro stores the counters
// in metadata and folds them into retrieval weighting. In-degree — how many
// edges cite a node — gives the structural score component a baseline for
// plain search results, where it was always 0 outside graph expansion.

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
)

// outcomeFactor converts a node's recorded outcomes into a score multiplier:
// neutral 1.0 with no signal; successes boost (+0.10·ln(1+n)), failures
// penalize harder (−0.15·ln(1+m) — a misleading memory is worse than an
// unproven one is good), floored at 0.2 so even a repeatedly-failed memory
// stays findable for deliberate supersession rather than vanishing.
func outcomeFactor(metadata json.RawMessage) float64 {
	if len(metadata) == 0 || !strings.Contains(string(metadata), `"outcomes"`) {
		return 1.0
	}
	var wrapper struct {
		Outcomes struct {
			Success int `json:"success"`
			Failure int `json:"failure"`
		} `json:"outcomes"`
	}
	if err := json.Unmarshal(metadata, &wrapper); err != nil {
		return 1.0
	}
	f := 1.0 + 0.10*math.Log1p(float64(wrapper.Outcomes.Success)) -
		0.15*math.Log1p(float64(wrapper.Outcomes.Failure))
	if f < 0.2 {
		return 0.2
	}
	return f
}

// RecordOutcome increments the node's success or failure counter in its
// metadata, preserving every other metadata key (the beadsId/anchor merge
// discipline). The write stamps updated_at via the normal metadata-update
// path semantics.
func (s *Store) RecordOutcome(id string, success bool) error {
	n, err := s.GetNode(id)
	if err != nil {
		return err
	}
	meta := map[string]any{}
	if len(n.Metadata) > 0 {
		if err := json.Unmarshal(n.Metadata, &meta); err != nil {
			return fmt.Errorf("parsing existing metadata: %w", err)
		}
	}
	outcomes := map[string]any{}
	if existing, ok := meta["outcomes"].(map[string]any); ok {
		outcomes = existing
	}
	key := "success"
	if !success {
		key = "failure"
	}
	current := 0.0
	if v, ok := outcomes[key].(float64); ok {
		current = v
	}
	outcomes[key] = current + 1
	meta["outcomes"] = outcomes

	data, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("encoding outcomes: %w", err)
	}
	if _, err := s.db.Exec(`UPDATE nodes SET metadata = ? WHERE id = ?`, string(data), id); err != nil {
		return fmt.Errorf("recording outcome: %w", err)
	}
	return nil
}

// MetaIndegreeBonusEnabled is the config key gating the in-degree structural
// baseline (t3c9 A/B seam). Default ON; only the literal "false" disables.
const MetaIndegreeBonusEnabled = "config.indegree_bonus_enabled"

// indegreeEnabled reads the seam.
func (s *Store) indegreeEnabled() bool {
	val, _ := s.GetMeta(MetaIndegreeBonusEnabled)
	return val != "false"
}

// InDegrees returns id -> incoming CITATION count for the given nodes in
// one batched query. Only citation-class relations count (derived_from,
// supports — the research's "derive_from or reference"): topical hub
// relations like relates_to inflate hubs without evidencing value, and the
// A/B evidence (2026-08-27, 604-node brain, 18 queries, t3c9 pairs):
// all-relations variant recall@5 -0.074 / MRR -0.044; +learned_from variant
// recall@5 -0.046; derived_from+supports variant zero delta (the class is
// still sparse — 3 edges). Ship the sparse-but-safe class; re-run the A/B
// via the indegree_bonus_enabled seam as consolidation densifies
// derived_from.
func (s *Store) InDegrees(ids []string) (map[string]int, error) {
	out := map[string]int{}
	if len(ids) == 0 {
		return out, nil
	}
	placeholders, args := inPlaceholders(ids)
	//nolint:gosec // G201: placeholders are ? bytes, not user input.
	rows, err := s.db.Query(fmt.Sprintf(
		`SELECT target_id, COUNT(*) FROM edges WHERE target_id IN (%s) AND relation IN ('derived_from','supports') GROUP BY target_id`, placeholders), args...)
	if err != nil {
		return nil, fmt.Errorf("counting in-degrees: %w", err)
	}
	defer rows.Close() //nolint:errcheck // best-effort cleanup
	for rows.Next() {
		var id string
		var n int
		if err := rows.Scan(&id, &n); err != nil {
			return nil, err
		}
		out[id] = n
	}
	return out, rows.Err()
}

// indegreeStructural maps an in-degree to the [0,1] structural baseline:
// ln-scaled so the first citations matter most, saturating at ~10 citations.
func indegreeStructural(in int) float64 {
	if in <= 0 {
		return 0
	}
	v := math.Log1p(float64(in)) / math.Log1p(10)
	if v > 1 {
		return 1
	}
	return v
}

// applyIndegreeStructural rescores results with the in-degree structural
// baseline (one batched lookup). No-op when the seam is off or on error —
// scoring never fails a search.
func (s *Store) applyIndegreeStructural(results []ScoredNode) {
	if len(results) == 0 || !s.indegreeEnabled() {
		return
	}
	ids := make([]string, len(results))
	for i := range results {
		ids[i] = results[i].ID
	}
	degrees, err := s.InDegrees(ids)
	if err != nil {
		return
	}
	for i := range results {
		if in := degrees[results[i].ID]; in > 0 {
			results[i].Score = compositeScore(&results[i].Node, results[i].Similarity, indegreeStructural(in))
		}
	}
}
