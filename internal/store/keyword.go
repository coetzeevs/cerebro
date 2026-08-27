package store

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// buildMatchQuery turns an untrusted user query into an injection-safe FTS5
// MATCH expression by tokenising on whitespace, wrapping EACH token in its own
// double-quoted FTS5 phrase (doubling any internal double-quote — FTS5's
// phrase-escape), and joining the phrases with ` OR ` (S-PI-N1, the load-bearing
// injection-neutralisation contract).
//
// Why per-token-OR and not one whole-query phrase (the rework — agentic-2lak):
// wrapping the WHOLE query in one phrase (`"OO-015 determinism wire"`) requires
// every term to be ADJACENT in the indexed text, so any multi-word query matched
// nothing (live-proven: that phrase → 0 rows; the entire eval corpus is
// multi-word, so BM25 was inert on every query). Tokenising and OR-joining
// (`"OO-015" OR "determinism" OR "wire"`) lets each term match independently, so
// the rare identifier token matches inside a multi-word query and bm25()'s
// term-rarity weighting floats those rare matches to the top of the keyword lane.
//
// Injection safety is preserved EXACTLY (live-proven, mattn/go-sqlite3
// v1.14.34): every user token is an individual quoted phrase, so no FTS5
// operator from user text (AND/OR/NOT/NEAR/*/:/^/( )) reaches the parser as
// syntax — it is literal phrase content. The ` OR ` is OURS, not the user's. A
// user word like `OR`/`AND`/`NEAR` becomes a quoted literal phrase (inert). The
// result is still bound as a SQL ? parameter by the caller — ?-binding stops
// classic SQL injection; the per-token phrase-quoting stops the second-order
// FTS5 expression injection that ?-binding alone does NOT close.
//
// Empty / all-whitespace input yields "" (no tokens); KeywordSearch's
// empty-query guard short-circuits before this is ever reached as a MATCH.
func buildMatchQuery(query string) string {
	fields := strings.Fields(query)
	if len(fields) == 0 {
		return ""
	}
	quoted := make([]string, 0, len(fields))
	for _, tok := range fields {
		quoted = append(quoted, `"`+strings.ReplaceAll(tok, `"`, `""`)+`"`)
	}
	return strings.Join(quoted, " OR ")
}

// KeywordSearch runs an FTS5 BM25 query over nodes_fts and returns active nodes
// ranked best-first by bm25(). It constructs the MATCH expression internally
// (buildMatchQuery — no raw FTS5 syntax is exposed to users) and binds it as a ?
// parameter (S-PI-N1).
//
// Graceful-degrade contract (protects AC4-NR): returns an EMPTY slice (not an
// error) when nodes_fts is absent (binary built without the fts5 tag), when the
// query is empty/whitespace-only, or when the MATCH yields no rows. Each
// returned ScoredNode carries the node plus a normalised [0,1] keyword-relevance
// signal in Similarity; Score is left zero because recall fusion uses rank, not
// the raw bm25() value (which is unbounded-negative).
func (s *Store) KeywordSearch(query string, limit int) ([]ScoredNode, error) {
	if !s.ftsAvailable() {
		return nil, nil // no-fts5 binary: keyword lane contributes nothing
	}
	if strings.TrimSpace(query) == "" {
		return nil, nil // empty query: identity-degrade (S-PI-N1.3 / AC4-NR)
	}
	if limit <= 0 {
		limit = 10
	}

	match := buildMatchQuery(query)

	// bm25(nodes_fts) returns a score where MORE-NEGATIVE = better; ORDER BY rank
	// (ascending) puts the best match first. Join to nodes for active-only
	// filtering and full node data, mirroring VectorSearch's vec->nodes join.
	rows, err := s.db.Query(`
		SELECT
			f.node_id,
			bm25(nodes_fts) AS rank_score,
			n.id, n.type, n.subtype, n.content, n.metadata, n.importance, n.decay_rate,
			n.access_count, n.times_reinforced, n.status, n.embedding_model,
			n.created_at, n.last_accessed, n.last_reinforced,
			n.updated_at, n.last_surfaced,
			n.origin_actor, n.origin_channel, n.origin_session, n.origin_host
		FROM nodes_fts f
		JOIN nodes n ON n.id = f.node_id
		WHERE nodes_fts MATCH ? AND n.status = 'active'
		ORDER BY rank_score ASC
		LIMIT ?`,
		match, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("keyword search: %w", err)
	}
	defer rows.Close() //nolint:errcheck // rows.Close in defer is idiomatic

	var results []ScoredNode
	var rawScores []float64
	for rows.Next() {
		var sn ScoredNode
		var nodeID string
		var bm25Score float64
		var subtype, metadata, lastReinf, updatedAt, lastSurfaced interface{}
		var originActor, originChannel, originSession, originHost sql.NullString

		if err := rows.Scan(
			&nodeID, &bm25Score,
			&sn.ID, &sn.Type, &subtype, &sn.Content, &metadata, &sn.Importance, &sn.DecayRate,
			&sn.AccessCount, &sn.TimesReinforced, &sn.Status, &sn.EmbeddingModel,
			&sn.CreatedAt, &sn.LastAccessed, &lastReinf,
			&updatedAt, &lastSurfaced,
			&originActor, &originChannel, &originSession, &originHost,
		); err != nil {
			return nil, fmt.Errorf("scanning keyword result: %w", err)
		}

		if v, ok := subtype.(string); ok {
			sn.Subtype = v
		}
		sn.OriginActor = originActor.String
		sn.OriginChannel = originChannel.String
		sn.OriginSession = originSession.String
		sn.OriginHost = originHost.String
		if m, ok := metadata.(string); ok {
			sn.Metadata = []byte(m)
		}
		if lr, ok := lastReinf.(string); ok {
			t, _ := time.Parse(time.RFC3339, lr)
			sn.LastReinforced = &t
		}
		if ua, ok := updatedAt.(string); ok {
			t, _ := time.Parse(time.RFC3339, ua)
			sn.UpdatedAt = &t
		}
		if ls, ok := lastSurfaced.(string); ok {
			t, _ := time.Parse(time.RFC3339, ls)
			sn.LastSurfaced = &t
		}

		results = append(results, sn)
		rawScores = append(rawScores, bm25Score)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating keyword results: %w", err)
	}

	// Carry a normalised [0,1] keyword-relevance signal on Similarity so callers
	// (and AC2) can observe a non-zero BM25 contribution for a matched node. The
	// best (most-negative) bm25 score maps to 1.0; the fusion layer ignores this
	// value and uses rank instead, so the normalisation is purely informational.
	normaliseBM25Signal(results, rawScores)

	// In-degree structural baseline (agentic-do71) — parity with the vector
	// lane; the fusion layer uses rank, but keyword-only consumers see Score.
	s.applyIndegreeStructural(results)

	return results, nil
}

// normaliseBM25Signal maps the unbounded-negative bm25() scores (more-negative =
// better) of a single result set onto a [0,1] relevance signal stored in
// ScoredNode.Similarity. The most-negative score in the set maps to 1.0; equal
// scores map to 1.0. This is informational only — recall fusion uses rank, not
// this value — but it lets AC2 assert a non-zero BM25 contribution for a match.
func normaliseBM25Signal(results []ScoredNode, rawScores []float64) {
	if len(results) == 0 {
		return
	}
	best, worst := rawScores[0], rawScores[0]
	for _, sc := range rawScores {
		if sc < best {
			best = sc
		}
		if sc > worst {
			worst = sc
		}
	}
	span := worst - best
	for i := range results {
		if span == 0 {
			// All equal (or single result): every match is maximally relevant.
			results[i].Similarity = 1.0
			continue
		}
		// best (most-negative) -> 1.0; worst -> 0 ... but a matched node should
		// never read as zero contribution, so floor the signal just above 0.
		norm := (worst - rawScores[i]) / span
		if norm < 0.01 {
			norm = 0.01
		}
		results[i].Similarity = norm
	}
}

// FTSAvailable reports whether the FTS5 keyword index is usable in this build
// and store. Exported for the brain layer's embedder-failure fallback (N3):
// keyword-only recall is offered only when the lane can actually answer.
func (s *Store) FTSAvailable() bool {
	return s.ftsAvailable()
}

// RescoreKeywordOnly stamps composite scores onto keyword-lane results for the
// embedder-failure fallback: similarity and structural are unavailable (no
// query vector, no expansion), so each Score degrades to the importance and
// recency terms only. Ordering is NOT changed — BM25 rank remains the ranking
// signal, mirroring the fusion contract where the keyword lane contributes
// rank, not score.
func (s *Store) RescoreKeywordOnly(results []ScoredNode) []ScoredNode {
	for i := range results {
		results[i].Score = compositeScore(&results[i].Node, 0, 0)
	}
	return results
}
