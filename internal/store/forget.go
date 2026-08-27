package store

// forget.go — subject-scoped bulk forget (agentic-dpgh). Distinct from GC:
// GC evicts by retention score; forget targets by CONTENT (case-insensitive
// substring) with an optional subtype filter — "remove everything about
// project X" before sharing or handing over a brain. The cascade removes
// embeddings, FTS presence, and every touching edge in the same transaction
// so nothing about the subject stays retrievable through any lane; archive
// mode keeps the (de-indexed) rows for audit, hard mode removes them.

import (
	"fmt"
	"strings"
)

// ForgetResult reports one forget pass.
type ForgetResult struct {
	// Matched lists the nodes the subject pattern selected (dry-run: what
	// WOULD be forgotten).
	Matched []Node `json:"matched"`
	// EdgesRemoved / EmbeddingsRemoved count the cascade (0 on dry-run).
	EdgesRemoved      int  `json:"edges_removed"`
	EmbeddingsRemoved int  `json:"embeddings_removed"`
	Hard              bool `json:"hard"`
	DryRun            bool `json:"dry_run"`
}

// ForgetSubject selects every non-archived node whose content contains
// pattern (case-insensitive; subtype narrows when non-empty) and — unless
// dryRun — forgets them: edges touching a matched node, its vec_nodes row,
// and its FTS presence are removed; the node itself is archived, or deleted
// entirely when hard. Everything mutating runs in ONE transaction. Dry-run
// performs the selection only and is guaranteed write-free.
func (s *Store) ForgetSubject(pattern, subtype string, hard, dryRun bool) (*ForgetResult, error) {
	if strings.TrimSpace(pattern) == "" {
		return nil, fmt.Errorf("subject pattern must not be empty")
	}

	query := `
		SELECT id, type, subtype, content, metadata, importance, decay_rate,
			access_count, times_reinforced, status, embedding_model,
			created_at, last_accessed, last_reinforced,
			updated_at, last_surfaced, provenance_root,
			origin_actor, origin_channel, origin_session, origin_host
		FROM nodes
		WHERE status != 'archived' AND instr(lower(content), lower(?)) > 0`
	args := []any{pattern}
	if subtype != "" {
		query += ` AND subtype = ?`
		args = append(args, subtype)
	}
	query += ` ORDER BY created_at ASC`

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("selecting subject nodes: %w", err)
	}
	defer rows.Close() //nolint:errcheck // best-effort cleanup

	res := &ForgetResult{Hard: hard, DryRun: dryRun}
	for rows.Next() {
		n, err := scanNodeFromRows(rows)
		if err != nil {
			return nil, err
		}
		res.Matched = append(res.Matched, *n)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if dryRun || len(res.Matched) == 0 {
		return res, nil
	}

	tx, err := s.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("beginning forget: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	ids := make([]string, len(res.Matched))
	for i := range res.Matched {
		ids[i] = res.Matched[i].ID
	}
	placeholders, inArgs := inPlaceholders(ids)

	//nolint:gosec // G201: placeholders are ? bytes, not user input.
	edgeRes, err := tx.Exec(fmt.Sprintf(
		`DELETE FROM edges WHERE source_id IN (%s) OR target_id IN (%s)`, placeholders, placeholders),
		append(append([]any{}, inArgs...), inArgs...)...)
	if err != nil {
		return nil, fmt.Errorf("cascading edges: %w", err)
	}
	if n, err := edgeRes.RowsAffected(); err == nil {
		res.EdgesRemoved = int(n)
	}

	if s.vecAvailable() {
		//nolint:gosec // G201: placeholders are ? bytes, not user input.
		vecRes, err := tx.Exec(fmt.Sprintf(
			`DELETE FROM vec_nodes WHERE node_id IN (%s)`, placeholders), inArgs...)
		if err != nil {
			return nil, fmt.Errorf("cascading embeddings: %w", err)
		}
		if n, err := vecRes.RowsAffected(); err == nil {
			res.EmbeddingsRemoved = int(n)
		}
	}

	if s.ftsAvailable() {
		//nolint:gosec // G201: placeholders are ? bytes, not user input.
		if _, err := tx.Exec(fmt.Sprintf(
			`DELETE FROM nodes_fts WHERE node_id IN (%s)`, placeholders), inArgs...); err != nil {
			return nil, fmt.Errorf("cascading FTS: %w", err)
		}
	}

	if hard {
		//nolint:gosec // G201: placeholders are ? bytes, not user input.
		if _, err := tx.Exec(fmt.Sprintf(
			`DELETE FROM nodes WHERE id IN (%s)`, placeholders), inArgs...); err != nil {
			return nil, fmt.Errorf("hard-deleting nodes: %w", err)
		}
	} else {
		//nolint:gosec // G201: placeholders are ? bytes, not user input.
		if _, err := tx.Exec(fmt.Sprintf(
			`UPDATE nodes SET status = 'archived' WHERE id IN (%s)`, placeholders), inArgs...); err != nil {
			return nil, fmt.Errorf("archiving nodes: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("committing forget: %w", err)
	}
	return res, nil
}
