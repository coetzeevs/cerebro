package main

// provenance_output.go — presentation wrappers that attach the computed
// provenance_status (AC6) and the optional --with-provenance lineage chain (AC5)
// to get/recall/list JSON output WITHOUT mutating the pure store structs or
// adding a stored column.
//
// Byte-identity discipline (the xtzn nil-omit idiom): the md plain-text output
// stays byte-identical to the pre-lbjg path when no provenance flag is given —
// the provenance CHAIN block is rendered only when --with-provenance is engaged.
// provenance_status is surfaced ALWAYS in JSON (an additive field; AC6's
// verification asserts the field in json output) and in md only on the
// provenance-engaged paths.

import (
	"github.com/coetzeevs/cerebro/brain"
	"github.com/coetzeevs/cerebro/internal/store"
)

// nodeWithProvenance is the JSON presentation wrapper for a single get result.
// It embeds NodeWithEdges and adds the always-present provenance_status plus the
// optional provenance chain (omitted from JSON when nil/absent).
type nodeWithProvenance struct {
	*store.NodeWithEdges
	ProvenanceStatus string `json:"provenance_status"`
	// OriginStatus (agentic-goc7): recorded|legacy|unknown, computed against
	// the origin-convention boundary. Always present in get JSON, mirroring
	// provenance_status; the raw origin_* fields ride on the embedded Node.
	OriginStatus string                `json:"origin_status"`
	Provenance   []provenanceChainItem `json:"provenance,omitempty"`
}

// scoredNodeWithProvenance is the JSON presentation wrapper for one recall result.
type scoredNodeWithProvenance struct {
	store.ScoredNode
	ProvenanceStatus string                `json:"provenance_status"`
	OriginStatus     string                `json:"origin_status"`
	Provenance       []provenanceChainItem `json:"provenance,omitempty"`
}

// nodeWithProvenanceStatus is the JSON presentation wrapper for one list result
// (status only — list never attaches a chain).
type nodeWithProvenanceStatus struct {
	store.Node
	ProvenanceStatus string `json:"provenance_status"`
	OriginStatus     string `json:"origin_status"`
}

// provenanceStatusFor returns the provenance_status for a single id, defaulting
// to "none" on any lookup error so the read path never fails over a status that
// is best-effort presentation metadata.
func provenanceStatusFor(b *brain.Brain, id string) string {
	statuses, err := b.ProvenanceStatus([]string{id})
	if err != nil {
		return store.ProvenanceNone
	}
	if s, ok := statuses[id]; ok {
		return s
	}
	return store.ProvenanceNone
}

// provenanceStatusForAll batches a slice of node IDs to id->status, defaulting
// to "none" on error.
func provenanceStatusForAll(b *brain.Brain, ids []string) map[string]string {
	statuses, err := b.ProvenanceStatus(ids)
	if err != nil {
		statuses = make(map[string]string, len(ids))
	}
	for _, id := range ids {
		if _, ok := statuses[id]; !ok {
			statuses[id] = store.ProvenanceNone
		}
	}
	return statuses
}
