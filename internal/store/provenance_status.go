package store

import (
	"fmt"
	"strings"
	"time"
)

// Provenance status values (agentic-lbjg AC6).
const (
	// ProvenanceComplete: the node has >=1 outgoing derived_from edge.
	ProvenanceComplete = "complete"
	// ProvenanceNone: no derived_from edge, created at/after the convention
	// boundary (asserted without a recorded source).
	ProvenanceNone = "none"
	// ProvenanceLegacy: no derived_from edge, created before the convention
	// boundary (predates the convention — absence of provenance is expected).
	ProvenanceLegacy = "legacy"
)

// ProvenanceStatusBatch computes the provenance_status for each given node ID in
// a single batched pass (no N+1 on recall/list), returning id -> status
// (complete|none|legacy). It is computed at query time — there is no stored
// provenance_status column.
//
// Rules (AC6):
//   - complete: the node has >=1 outgoing derived_from edge.
//   - none:     no such edge AND created_at >= the convention boundary.
//   - legacy:   no such edge AND created_at <  the convention boundary.
//
// The boundary is the schema_meta provenance_convention_since instant, stored as
// a string in storageTimeLayout and parsed ONCE into a time.Time. The legacy
// comparison is node.CreatedAt.Before(boundary) — a time.Time compare in Go,
// strict '<' (a node created at the exact boundary instant is non-legacy), never
// a lexicographic string compare. If the boundary meta is missing or unparseable,
// no node is classified legacy (every no-edge node reads "none") — a defensive
// default that never mislabels a node as predating a convention we cannot date.
func (s *Store) ProvenanceStatusBatch(ids []string) (map[string]string, error) {
	out := make(map[string]string, len(ids))
	if len(ids) == 0 {
		return out, nil
	}

	// 1. has-provenance set: one query over edges grouped by source.
	hasProv, err := s.derivedFromSources(ids)
	if err != nil {
		return nil, err
	}

	// 2. parse the convention boundary once.
	boundary, hasBoundary := s.provenanceBoundary()

	// 3. created_at per node (any status), one batched query.
	createdAt, err := s.createdAtBatch(ids)
	if err != nil {
		return nil, err
	}

	for _, id := range ids {
		switch {
		case hasProv[id]:
			out[id] = ProvenanceComplete
		case hasBoundary && createdAt[id].Before(boundary):
			out[id] = ProvenanceLegacy
		default:
			out[id] = ProvenanceNone
		}
	}
	return out, nil
}

// derivedFromSources returns the set of ids (from the given list) that have at
// least one outgoing derived_from edge.
func (s *Store) derivedFromSources(ids []string) (map[string]bool, error) {
	placeholders, args := inPlaceholders(ids)
	args = append(args, RelationDerivedFrom)
	//nolint:gosec // G201: placeholders are ? bytes, not user input; relation is bound.
	query := fmt.Sprintf(
		`SELECT DISTINCT source_id FROM edges WHERE source_id IN (%s) AND relation = ?`, placeholders)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("querying derived_from sources: %w", err)
	}
	defer rows.Close() //nolint:errcheck // best-effort cleanup

	set := make(map[string]bool)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scanning derived_from source: %w", err)
		}
		set[id] = true
	}
	return set, rows.Err()
}

// createdAtBatch returns id -> created_at for the given ids (any status).
func (s *Store) createdAtBatch(ids []string) (map[string]time.Time, error) {
	placeholders, args := inPlaceholders(ids)
	//nolint:gosec // G201: placeholders are ? bytes, not user input.
	query := fmt.Sprintf(`SELECT id, created_at FROM nodes WHERE id IN (%s)`, placeholders)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("querying created_at: %w", err)
	}
	defer rows.Close() //nolint:errcheck // best-effort cleanup

	out := make(map[string]time.Time)
	for rows.Next() {
		var id string
		var createdAt time.Time
		if err := rows.Scan(&id, &createdAt); err != nil {
			return nil, fmt.Errorf("scanning created_at: %w", err)
		}
		out[id] = createdAt
	}
	return out, rows.Err()
}

// provenanceBoundary parses the convention boundary instant from schema_meta.
// Returns (zero, false) if the meta is missing or unparseable.
func (s *Store) provenanceBoundary() (time.Time, bool) {
	raw, err := s.GetMeta(MetaProvenanceConventionSince)
	if err != nil || raw == "" {
		return time.Time{}, false
	}
	t, perr := time.Parse(storageTimeLayout, raw)
	if perr != nil {
		return time.Time{}, false
	}
	return t.UTC(), true
}

// inPlaceholders builds a "?,?,?" string and the matching []any args for an IN
// clause from a slice of string ids.
func inPlaceholders(ids []string) (placeholders string, args []any) {
	marks := make([]string, len(ids))
	args = make([]any, len(ids))
	for i, id := range ids {
		marks[i] = "?"
		args[i] = id
	}
	return strings.Join(marks, ","), args
}
