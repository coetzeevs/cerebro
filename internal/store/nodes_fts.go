package store

import (
	"database/sql"
	"fmt"
	"os"
)

// ftsExecer is the subset of *sql.DB / *sql.Tx used by the FTS sync helpers, so
// the same write logic can run either on the connection (AddNode/UpdateNode,
// which have no explicit transaction) or inside an existing transaction
// (SupersedeNode, GC) — the S-PI-N2 transaction-integrity requirement: the FTS
// write shares the primary nodes write's transaction boundary where one exists.
type ftsExecer interface {
	Exec(query string, args ...any) (sql.Result, error)
}

// syncFTSInsert writes (or refreshes) the nodes_fts row for a node. It is a
// graceful no-op when nodes_fts is absent (a binary built without the fts5 tag,
// per D2) — keyword recall simply contributes nothing rather than failing the
// primary write. content and subtype are bound as ? parameters (S-PI-N2: node
// content is attacker-influenceable text and must never be concatenated into
// SQL). Existing rows for node_id are deleted first so updates do not duplicate.
//
// On the connection path (no tx), a sync failure is logged and swallowed so a
// transient FTS error cannot fail an AddNode/UpdateNode whose primary nodes
// write already succeeded; on the tx path the caller propagates the error so the
// whole transaction rolls back atomically. The mustSucceed flag selects which.
func (s *Store) syncFTSInsert(ex ftsExecer, mustSucceed bool, nodeID, content, subtype string) error {
	if !s.ftsAvailable() {
		return nil // no-fts5 binary: graceful degrade (D2)
	}
	if err := s.deleteFTS(ex, mustSucceed, nodeID); err != nil {
		return err
	}
	if _, err := ex.Exec(
		`INSERT INTO nodes_fts (node_id, content, subtype) VALUES (?, ?, ?)`,
		nodeID, content, subtype,
	); err != nil {
		if mustSucceed {
			return fmt.Errorf("syncing nodes_fts insert for %s: %w", nodeID, err)
		}
		fmt.Fprintf(os.Stderr, "cerebro: nodes_fts sync failed for %s (%v); keyword recall may be stale\n", nodeID, err)
	}
	return nil
}

// deleteFTS removes the nodes_fts row(s) for a node. Graceful no-op when
// nodes_fts is absent. mustSucceed mirrors syncFTSInsert's error policy.
func (s *Store) deleteFTS(ex ftsExecer, mustSucceed bool, nodeID string) error {
	if !s.ftsAvailable() {
		return nil
	}
	if _, err := ex.Exec(`DELETE FROM nodes_fts WHERE node_id = ?`, nodeID); err != nil {
		if mustSucceed {
			return fmt.Errorf("syncing nodes_fts delete for %s: %w", nodeID, err)
		}
		fmt.Fprintf(os.Stderr, "cerebro: nodes_fts delete-sync failed for %s (%v); keyword recall may be stale\n", nodeID, err)
	}
	return nil
}
