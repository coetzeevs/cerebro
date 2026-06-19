package store

import (
	"encoding/json"
	"fmt"
	"time"

	"database/sql"
)

// storageTimeLayout is the SQLite TEXT DATETIME layout cerebro writes for all
// agent-supplied times. It matches the existing created_at convention
// (nodes.go) and sorts lexicographically == chronologically, so '<'/'<='/'>'
// comparisons on the column are correct date comparisons.
const storageTimeLayout = "2006-01-02 15:04:05"

// formatBound formats an optional validity bound for SQL binding: the UTC
// storage-layout string when set, or nil (=> SQL NULL) when the pointer is nil.
func formatBound(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.UTC().Format(storageTimeLayout)
}

// AddEdge creates a directed relationship between two nodes, carrying the
// optional bi-temporal validity window (agentic-xtzn). Re-adding an existing
// (source, target, relation) edge UPDATES its window in place via
// ON CONFLICT … DO UPDATE — a full-window re-assertion, NOT a partial patch: a
// nil bound in opts overwrites any prior non-NULL value to NULL. The persisted
// row id is always returned (via RETURNING id) — on the conflict/update path
// LastInsertId() is unreliable (AUTOINCREMENT is not re-fired), so a re-add
// returns the existing id, never 0 or a stale value (TL-PI-N4).
func (s *Store) AddEdge(sourceID, targetID, relation string, opts AddEdgeOpts) (int64, error) {
	var id int64
	err := s.db.QueryRow(`
		INSERT INTO edges (source_id, target_id, relation, valid_at, invalid_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (source_id, target_id, relation)
		DO UPDATE SET valid_at = excluded.valid_at, invalid_at = excluded.invalid_at
		RETURNING id`,
		sourceID, targetID, relation, formatBound(opts.ValidAt), formatBound(opts.InvalidAt),
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("inserting edge: %w", err)
	}
	return id, nil
}

// asOfPredicate returns the half-open validity-window SQL fragment and its bind
// args for an optional as-of instant. When asOf is nil it returns an empty
// fragment and no args, so the query is byte-identical to the pre-xtzn path
// (AC4). When set, it filters to edges valid at that instant under the half-open
// convention [valid_at, invalid_at): valid_at == asOf is INCLUDED, invalid_at ==
// asOf is EXCLUDED, NULL bounds are open-ended. The instant is bound twice.
func asOfPredicate(asOf *time.Time) (fragment string, args []any) {
	if asOf == nil {
		return "", nil
	}
	bound := asOf.UTC().Format(storageTimeLayout)
	return ` AND (valid_at IS NULL OR valid_at <= ?) AND (invalid_at IS NULL OR invalid_at > ?)`,
		[]any{bound, bound}
}

// getEdgesForNode returns all edges where the node is source or target. When
// asOf is non-nil, only edges whose validity window contains that instant are
// returned (agentic-xtzn).
func (s *Store) getEdgesForNode(nodeID string, asOf *time.Time) ([]Edge, error) {
	pred, predArgs := asOfPredicate(asOf)
	args := append([]any{nodeID, nodeID}, predArgs...)
	//nolint:gosec // G201: pred is static SQL text (no user input); bounds are ? placeholders.
	query := `
		SELECT id, source_id, target_id, relation, weight, metadata, created_at, valid_at, invalid_at
		FROM edges WHERE (source_id = ? OR target_id = ?)` + pred + `
		ORDER BY created_at`
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("querying edges: %w", err)
	}
	defer rows.Close() //nolint:errcheck // best-effort cleanup

	var edges []Edge
	for rows.Next() {
		e, err := scanEdge(rows)
		if err != nil {
			return nil, err
		}
		edges = append(edges, *e)
	}
	return edges, rows.Err()
}

// GetEdgesBatch returns edges for multiple nodes in a single query.
// The result maps each input node ID to its edges (where it appears as source or target).
// When asOf is non-nil, only edges valid at that instant are returned (agentic-xtzn).
func (s *Store) GetEdgesBatch(nodeIDs []string, asOf *time.Time) (map[string][]Edge, error) {
	result := make(map[string][]Edge)
	if len(nodeIDs) == 0 {
		return result, nil
	}

	// Build IN clause
	placeholders := make([]byte, 0, len(nodeIDs)*2)
	// We need the IDs twice (source and target)
	args := make([]any, 0, len(nodeIDs)*2)
	for i, id := range nodeIDs {
		if i > 0 {
			placeholders = append(placeholders, ',')
		}
		placeholders = append(placeholders, '?')
		args = append(args, id)
	}
	// Duplicate args for the second IN clause
	for _, id := range nodeIDs {
		args = append(args, id)
	}

	// The as-of predicate (when set) appends its two bound args AFTER the IN args.
	pred, predArgs := asOfPredicate(asOf)
	args = append(args, predArgs...)

	query := fmt.Sprintf(`SELECT id, source_id, target_id, relation, weight, metadata, created_at, valid_at, invalid_at
		FROM edges WHERE (source_id IN (%s) OR target_id IN (%s))%s
		ORDER BY created_at`, placeholders, placeholders, pred) //nolint:gosec  // G201: placeholders and pred are ? / static SQL, not user input

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("batch get edges: %w", err)
	}
	defer rows.Close() //nolint:errcheck // best-effort cleanup

	// Build a set for fast lookup
	idSet := make(map[string]bool, len(nodeIDs))
	for _, id := range nodeIDs {
		idSet[id] = true
	}

	for rows.Next() {
		e, err := scanEdge(rows)
		if err != nil {
			return nil, err
		}
		// Map edge to each input node it connects to
		if idSet[e.SourceID] {
			result[e.SourceID] = append(result[e.SourceID], *e)
		}
		if idSet[e.TargetID] && e.SourceID != e.TargetID {
			result[e.TargetID] = append(result[e.TargetID], *e)
		}
	}
	return result, rows.Err()
}

func scanEdge(rows *sql.Rows) (*Edge, error) {
	e := &Edge{}
	var metadata sql.NullString
	// valid_at / invalid_at are nullable DATETIME columns. mattn/go-sqlite3
	// auto-parses declared DATETIME columns into time.Time, so sql.NullTime
	// round-trips the stored "2006-01-02 15:04:05" layout directly — no manual
	// time.Parse (TL-PI-N3; the RFC3339 time.Parse idiom in search.go would
	// silently mis-parse this layout and defeat the --as-of filter).
	var validAt, invalidAt sql.NullTime
	err := rows.Scan(&e.ID, &e.SourceID, &e.TargetID, &e.Relation, &e.Weight, &metadata, &e.CreatedAt, &validAt, &invalidAt)
	if err != nil {
		return nil, fmt.Errorf("scanning edge: %w", err)
	}
	if metadata.Valid {
		e.Metadata = json.RawMessage(metadata.String)
	}
	if validAt.Valid {
		t := validAt.Time
		e.ValidAt = &t
	}
	if invalidAt.Valid {
		t := invalidAt.Time
		e.InvalidAt = &t
	}
	return e, nil
}
