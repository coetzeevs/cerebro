package store

// inbox.go — capture-with-approval quarantine (agentic-m8m3).
//
// Candidates live in their OWN table (inbox_candidates, schema v7) — not in
// nodes at all — so quarantine is structural: no retrieval surface, score,
// GC pass, or export path can see a candidate, and the nodes-table status
// CHECK stays untouched. The agent proposes (`cerebro inbox add` from a
// skill or SessionEnd hook — Model B: cerebro never auto-commits facts
// mined from transcripts; low-level traces transfer badly), then a human or
// agent explicitly approves (the candidate becomes a real node, keeping its
// id, origin, and capture timestamp, and enters the normal reconciliation/
// embedding path) or discards (the row is deleted — it never was a memory).

import (
	"database/sql"
	"fmt"

	"github.com/google/uuid"
)

// AddCandidateNode inserts a QUARANTINED memory candidate into the inbox.
func (s *Store) AddCandidateNode(opts *AddNodeOpts) (string, error) {
	id := uuid.New().String()
	importance := opts.Importance
	if importance <= 0 {
		importance = 0.5
	}
	_, err := s.db.Exec(`
		INSERT INTO inbox_candidates (id, type, subtype, content, metadata, importance, origin_actor, origin_channel, origin_session, origin_host)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, opts.Type, nullString(opts.Subtype), opts.Content, nullJSON(opts.Metadata), importance,
		nullString(opts.OriginActor), nullString(opts.OriginChannel), nullString(opts.OriginSession), nullString(opts.OriginHost),
	)
	if err != nil {
		return "", fmt.Errorf("inserting candidate: %w", err)
	}
	return id, nil
}

// ListCandidates returns the quarantine inbox, oldest first, as Node values
// with Status "candidate" (presentation only — candidates are not nodes).
func (s *Store) ListCandidates() ([]Node, error) {
	rows, err := s.db.Query(`
		SELECT id, type, subtype, content, metadata, importance, created_at,
			origin_actor, origin_channel, origin_session, origin_host
		FROM inbox_candidates ORDER BY created_at ASC`)
	if err != nil {
		return nil, fmt.Errorf("listing candidates: %w", err)
	}
	defer rows.Close() //nolint:errcheck // best-effort cleanup

	var out []Node
	for rows.Next() {
		var n Node
		var subtype, metadata, oa, oc, osess, oh sql.NullString
		if err := rows.Scan(&n.ID, &n.Type, &subtype, &n.Content, &metadata, &n.Importance, &n.CreatedAt,
			&oa, &oc, &osess, &oh); err != nil {
			return nil, err
		}
		n.Subtype, n.OriginActor, n.OriginChannel, n.OriginSession, n.OriginHost =
			subtype.String, oa.String, oc.String, osess.String, oh.String
		if metadata.Valid {
			n.Metadata = []byte(metadata.String)
		}
		n.Status = "candidate"
		out = append(out, n)
	}
	return out, rows.Err()
}

// ApproveCandidate promotes a candidate into a real node — same id, origin,
// and capture timestamp — and indexes it for keyword search, all in one
// transaction. The embedding is the caller's concern (the brain layer embeds
// after approval; a failure there follows the normal pending path). Refuses
// unknown ids: only inbox rows can be approved.
func (s *Store) ApproveCandidate(id string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("beginning approve: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.Exec(`
		INSERT INTO nodes (id, type, subtype, content, metadata, importance, decay_rate, embedding_model, created_at, origin_actor, origin_channel, origin_session, origin_host)
		SELECT id, type, subtype, content, metadata, importance,
			CASE type WHEN 'episode' THEN 0.15 WHEN 'concept' THEN 0.02 WHEN 'procedure' THEN 0.005 ELSE 0.01 END,
			'', created_at, origin_actor, origin_channel, origin_session, origin_host
		FROM inbox_candidates WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("approving candidate: %w", err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return fmt.Errorf("no candidate %s in the inbox", id)
	}
	if _, err := tx.Exec(`DELETE FROM inbox_candidates WHERE id = ?`, id); err != nil {
		return fmt.Errorf("clearing approved candidate: %w", err)
	}

	var content string
	var subtype sql.NullString
	if err := tx.QueryRow(`SELECT content, subtype FROM nodes WHERE id = ?`, id).Scan(&content, &subtype); err != nil {
		return fmt.Errorf("reading approved node: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing approve: %w", err)
	}
	_ = s.syncFTSInsert(s.db, false, id, content, subtype.String)
	return nil
}

// DiscardCandidate removes a candidate — it never was a memory. Refuses
// unknown ids; forget and gc own deletion of real memories.
func (s *Store) DiscardCandidate(id string) error {
	res, err := s.db.Exec(`DELETE FROM inbox_candidates WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("discarding candidate: %w", err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return fmt.Errorf("no candidate %s in the inbox", id)
	}
	return nil
}
