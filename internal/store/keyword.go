package store

import (
	"fmt"
	"strings"
	"time"
)

// buildMatchQuery turns an untrusted user query into a single literal FTS5
// phrase so NO FTS5 operator from user text reaches the FTS5 parser as syntax
// (S-PI-N1, the load-bearing injection-neutralisation contract).
//
// Mechanism (live-proven, mattn/go-sqlite3 v1.14.34): wrap the whole string in
// double-quotes and double any internal double-quote — FTS5's phrase-quoting
// escape. Inside a phrase, AND/OR/NOT/NEAR/*/":/^/( ) are matched as literal
// text, not operators. The exact-identifier case (e.g. "HS-049") still matches
// under the default unicode61 tokenizer because the phrase tokenises to the same
// terms as the indexed content.
//
// The result is still bound as a SQL ? parameter by the caller — ?-binding stops
// classic SQL injection, and the phrase-quoting stops the second-order FTS5
// expression injection that ?-binding alone does NOT close.
func buildMatchQuery(query string) string {
	escaped := strings.ReplaceAll(query, `"`, `""`)
	return `"` + escaped + `"`
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
			n.updated_at, n.last_surfaced
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

		if err := rows.Scan(
			&nodeID, &bm25Score,
			&sn.ID, &sn.Type, &subtype, &sn.Content, &metadata, &sn.Importance, &sn.DecayRate,
			&sn.AccessCount, &sn.TimesReinforced, &sn.Status, &sn.EmbeddingModel,
			&sn.CreatedAt, &sn.LastAccessed, &lastReinf,
			&updatedAt, &lastSurfaced,
		); err != nil {
			return nil, fmt.Errorf("scanning keyword result: %w", err)
		}

		if v, ok := subtype.(string); ok {
			sn.Subtype = v
		}
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
