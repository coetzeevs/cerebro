package store

import (
	"database/sql"
	"fmt"
)

// ConsolidateInto records a consolidation: it flips each source episode to
// status='consolidated' AND writes a derived_from edge from the into-node to
// each source episode, so the concept/procedure/reflection that synthesized the
// episodes carries structural provenance back to them (agentic-lbjg AC3).
//
// Atomicity (Security NOTE B + Tech Lead lock): the status flips and the
// derived_from edge writes ride a SINGLE transaction. The edge upsert is issued
// via tx.Exec with the same ON CONFLICT ... DO UPDATE ... RETURNING id SQL as the
// connection-level AddEdge — NOT by calling AddEdge (which uses s.db, a separate
// connection: writing edges on the connection while the status flip is open in a
// tx would risk a lock/visibility hazard and break all-or-nothing). A
// half-applied consolidation (some edges, some flips) would corrupt provenance
// truth, so the operation is all-or-nothing.
//
// Fail-closed validation runs BEFORE any write: the into-node must exist, and
// every source must resolve to an episode (any status). If any source ID is
// unresolvable or not an episode, the whole command is rejected with a non-zero
// error and ZERO partial write (the tx never commits). An empty source list is
// a misuse and is rejected.
//
// Idempotency: the edge write rides the UNIQUE(source_id, target_id, relation)
// upsert, so a re-run updates-in-place rather than duplicating. A source that is
// a valid episode but already consolidated re-asserts its edge without error
// (re-running a consolidation is safe).
func (s *Store) ConsolidateInto(intoID string, episodeIDs []string) error {
	if len(episodeIDs) == 0 {
		return fmt.Errorf("consolidate: no source episodes given")
	}

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("beginning consolidation transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Fail-closed validation (before any write): the into-node must exist...
	if err := txNodeExists(tx, intoID); err != nil {
		return fmt.Errorf("consolidate --into: %w", err)
	}
	// ...and every source must be an existing episode (any status).
	for _, epID := range episodeIDs {
		if err := txEpisodeExists(tx, epID); err != nil {
			return fmt.Errorf("consolidate source %s: %w", epID, err)
		}
	}

	// Status flip: the exact MarkConsolidated predicate (active episodes only).
	// An already-consolidated episode affects 0 rows — that is a valid
	// re-assertion, not an error (validation already confirmed it is an episode).
	flipStmt, err := tx.Prepare(
		`UPDATE nodes SET status = 'consolidated' WHERE id = ? AND type = 'episode' AND status = 'active'`,
	)
	if err != nil {
		return fmt.Errorf("preparing consolidation flip: %w", err)
	}
	defer flipStmt.Close() //nolint:errcheck // best-effort cleanup

	// Edge upsert via tx.Exec — same SQL as AddEdge, INSIDE this tx (open-ended
	// NULL/NULL validity window; provenance edges carry no bi-temporal window in
	// v1, asOf is out of scope). RETURNING id keeps the upsert path consistent.
	edgeStmt, err := tx.Prepare(`
		INSERT INTO edges (source_id, target_id, relation, valid_at, invalid_at)
		VALUES (?, ?, ?, NULL, NULL)
		ON CONFLICT (source_id, target_id, relation)
		DO UPDATE SET valid_at = excluded.valid_at, invalid_at = excluded.invalid_at
		RETURNING id`)
	if err != nil {
		return fmt.Errorf("preparing derived_from edge upsert: %w", err)
	}
	defer edgeStmt.Close() //nolint:errcheck // best-effort cleanup

	for _, epID := range episodeIDs {
		if _, err := flipStmt.Exec(epID); err != nil {
			return fmt.Errorf("consolidating episode %s: %w", epID, err)
		}
		var edgeID int64
		if err := edgeStmt.QueryRow(intoID, epID, RelationDerivedFrom).Scan(&edgeID); err != nil {
			return fmt.Errorf("writing derived_from edge %s->%s: %w", intoID, epID, err)
		}
	}

	return tx.Commit()
}

// txNodeExists returns nil if a node with the given id exists in the tx, or a
// "not found" error otherwise.
func txNodeExists(tx *sql.Tx, id string) error {
	var got string
	err := tx.QueryRow(`SELECT id FROM nodes WHERE id = ?`, id).Scan(&got)
	if err == sql.ErrNoRows {
		return fmt.Errorf("node %q not found", id)
	}
	if err != nil {
		return fmt.Errorf("checking node %q: %w", id, err)
	}
	return nil
}

// txEpisodeExists returns nil if the id refers to an existing episode node (any
// status), or an error if it is missing or not an episode.
func txEpisodeExists(tx *sql.Tx, id string) error {
	var nodeType string
	err := tx.QueryRow(`SELECT type FROM nodes WHERE id = ?`, id).Scan(&nodeType)
	if err == sql.ErrNoRows {
		return fmt.Errorf("episode %q not found", id)
	}
	if err != nil {
		return fmt.Errorf("checking episode %q: %w", id, err)
	}
	if nodeType != string(TypeEpisode) {
		return fmt.Errorf("source %q is a %s, not an episode", id, nodeType)
	}
	return nil
}
