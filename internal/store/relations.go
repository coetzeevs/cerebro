package store

import (
	"database/sql"
	"fmt"
	"time"
)

// RelationDerivedFrom is the reserved built-in edge relation that links a
// derived node (concept/procedure/reflection) back to a source episode it was
// synthesized from. It is written automatically at consolidation time
// (ConsolidateInto) and is the relation provenance walks traverse outward
// (WalkRelation with outgoing=true). Reserving it as a single exported constant
// (rather than scattering the "derived_from" string literal) keeps the
// consolidation writer, the walk, and the provenance-status check in agreement.
//
// The typed-relation REGISTRY that seeds reserved relations on init is
// agentic-8l2g (the relations table + CRUD below); lbjg reserved the string.
const RelationDerivedFrom = "derived_from"

// MetaProvenanceConventionSince is the schema_meta key holding the instant the
// provenance convention took effect for a brain (the v4->v5 migration instant,
// or the brain's birth instant for a fresh v5 Init). provenanceStatus compares a
// node's created_at against this boundary to distinguish "legacy" (predates the
// convention — absence of provenance is expected) from "none" (created after the
// convention with no recorded source). The value is stored as a string in the
// repo's storageTimeLayout and parsed once into a time.Time before comparing —
// the comparison is a time.Time compare, never a lexicographic string compare.
const MetaProvenanceConventionSince = "provenance_convention_since"

// MetaOriginConventionSince is the schema_meta key holding the instant the
// origin-identity convention took effect for a brain (the v5->v6 migration
// instant, or the brain's birth instant for a fresh v6 Init). OriginStatusFor
// compares a node's created_at against this boundary to distinguish "legacy"
// (predates the convention — absence of origin is expected) from "unknown"
// (created after the convention with no recorded origin). Same storage and
// comparison discipline as MetaProvenanceConventionSince: storageTimeLayout
// string in schema_meta, parsed into a time.Time before comparing.
const MetaOriginConventionSince = "origin_convention_since"

// Relation is a row in the typed-relation registry (agentic-8l2g). Name is the
// edge-relation string; TraversalClass is a free-form grouping hint ("structural",
// "topical", ...) consumed by future traversal policies — empty is legal.
type Relation struct {
	Name           string    `json:"name"`
	TraversalClass string    `json:"traversal_class,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

// RegisterRelation records a relation name in the registry. Idempotent: a name
// already present keeps its existing traversal class (INSERT OR IGNORE) and no
// error is returned — registration is a declaration, not a mutation.
func (s *Store) RegisterRelation(name, class string) error {
	if name == "" {
		return fmt.Errorf("relation name must not be empty")
	}
	_, err := s.db.Exec(
		`INSERT OR IGNORE INTO relations (name, traversal_class) VALUES (?, ?)`,
		name, nullString(class),
	)
	if err != nil {
		return fmt.Errorf("registering relation %q: %w", name, err)
	}
	return nil
}

// ListRelations returns every registered relation, name-ordered.
func (s *Store) ListRelations() ([]Relation, error) {
	rows, err := s.db.Query(
		`SELECT name, traversal_class, created_at FROM relations ORDER BY name`,
	)
	if err != nil {
		return nil, fmt.Errorf("listing relations: %w", err)
	}
	defer rows.Close() //nolint:errcheck // best-effort cleanup

	var out []Relation
	for rows.Next() {
		var r Relation
		var class sql.NullString
		if err := rows.Scan(&r.Name, &class, &r.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning relation: %w", err)
		}
		r.TraversalClass = class.String
		out = append(out, r)
	}
	return out, rows.Err()
}

// RemoveRelation deletes a relation from the registry. Existing edges carrying
// the relation are untouched — the registry is advisory (warn-not-error), so
// removal only affects future registration checks.
func (s *Store) RemoveRelation(name string) error {
	if _, err := s.db.Exec(`DELETE FROM relations WHERE name = ?`, name); err != nil {
		return fmt.Errorf("removing relation %q: %w", name, err)
	}
	return nil
}

// RelationRegistered reports whether the named relation is in the registry.
func (s *Store) RelationRegistered(name string) (bool, error) {
	var one int
	err := s.db.QueryRow(`SELECT 1 FROM relations WHERE name = ?`, name).Scan(&one)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("checking relation %q: %w", name, err)
	}
	return true, nil
}

// OriginBoundary returns the origin-convention boundary instant, or (nil, nil)
// when the meta is absent (a pre-v6 brain opened read-only, or a corrupt value —
// mirrors provenanceBoundary's missing-or-unparseable = no-boundary contract).
func (s *Store) OriginBoundary() (*time.Time, error) {
	raw, err := s.GetMeta(MetaOriginConventionSince)
	if err != nil || raw == "" {
		return nil, nil //nolint:nilerr // missing meta is a legal no-boundary state
	}
	t, perr := time.Parse(storageTimeLayout, raw)
	if perr != nil {
		return nil, nil //nolint:nilerr // unparseable = treat as unstamped
	}
	u := t.UTC()
	return &u, nil
}

// OriginStatusFor classifies a node's origin record against the convention
// boundary: "recorded" (an actor was stamped), "legacy" (no actor, created
// before the convention existed — absence is expected), "unknown" (no actor,
// created under the convention — absence is a gap). With no boundary, absence
// is indistinguishable from legacy, so it classifies "legacy".
func OriginStatusFor(n *Node, boundary *time.Time) string {
	if n.OriginActor != "" {
		return "recorded"
	}
	if boundary == nil || n.CreatedAt.UTC().Before(*boundary) {
		return "legacy"
	}
	return "unknown"
}
