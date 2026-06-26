package store

import (
	"testing"
	"time"
)

// TestProvenanceStatusBatchComplete asserts a node with >=1 outgoing
// derived_from edge reads "complete".
func TestProvenanceStatusBatchComplete(t *testing.T) {
	s := testStore(t)
	concept, _ := s.AddNode(&AddNodeOpts{Type: TypeConcept, Content: "synthesis", Importance: 0.8})
	ep, _ := s.AddNode(&AddNodeOpts{Type: TypeEpisode, Content: "source", Importance: 0.5})
	if err := s.ConsolidateInto(concept, []string{ep}); err != nil {
		t.Fatalf("ConsolidateInto: %v", err)
	}

	statuses, err := s.ProvenanceStatusBatch([]string{concept})
	if err != nil {
		t.Fatalf("ProvenanceStatusBatch: %v", err)
	}
	if statuses[concept] != "complete" {
		t.Fatalf("concept with derived_from edge should be 'complete', got %q", statuses[concept])
	}
}

// TestProvenanceStatusBatchNone asserts a node created at/after the boundary with
// no derived_from edge reads "none" (a fresh-Init brain has no legacy era).
func TestProvenanceStatusBatchNone(t *testing.T) {
	s := testStore(t)
	// Fresh Init stamps the boundary at brain birth, so a node added now is
	// at/after the boundary -> "none" (no provenance recorded).
	n, _ := s.AddNode(&AddNodeOpts{Type: TypeConcept, Content: "asserted, no source", Importance: 0.5})

	statuses, err := s.ProvenanceStatusBatch([]string{n})
	if err != nil {
		t.Fatalf("ProvenanceStatusBatch: %v", err)
	}
	if statuses[n] != "none" {
		t.Fatalf("post-boundary node with no provenance should be 'none', got %q", statuses[n])
	}
}

// TestProvenanceStatusBatchLegacy asserts a node created BEFORE the boundary with
// no derived_from edge reads "legacy".
func TestProvenanceStatusBatchLegacy(t *testing.T) {
	s := testStore(t)

	// Move the convention boundary into the future so an existing node predates it.
	future := time.Now().UTC().Add(24 * time.Hour).Format(storageTimeLayout)
	if err := s.SetMeta(MetaProvenanceConventionSince, future); err != nil {
		t.Fatalf("SetMeta boundary: %v", err)
	}
	n, _ := s.AddNode(&AddNodeOpts{Type: TypeConcept, Content: "old memory", Importance: 0.5})

	statuses, err := s.ProvenanceStatusBatch([]string{n})
	if err != nil {
		t.Fatalf("ProvenanceStatusBatch: %v", err)
	}
	if statuses[n] != "legacy" {
		t.Fatalf("pre-boundary node with no provenance should be 'legacy', got %q", statuses[n])
	}
}

// TestProvenanceStatusBatchLegacyOverriddenByEdge asserts a pre-boundary node
// WITH a derived_from edge is "complete", not "legacy" (the edge wins).
func TestProvenanceStatusBatchLegacyOverriddenByEdge(t *testing.T) {
	s := testStore(t)
	future := time.Now().UTC().Add(24 * time.Hour).Format(storageTimeLayout)
	if err := s.SetMeta(MetaProvenanceConventionSince, future); err != nil {
		t.Fatalf("SetMeta boundary: %v", err)
	}
	concept, _ := s.AddNode(&AddNodeOpts{Type: TypeConcept, Content: "old concept", Importance: 0.5})
	ep, _ := s.AddNode(&AddNodeOpts{Type: TypeEpisode, Content: "source", Importance: 0.5})
	if err := s.ConsolidateInto(concept, []string{ep}); err != nil {
		t.Fatalf("ConsolidateInto: %v", err)
	}

	statuses, err := s.ProvenanceStatusBatch([]string{concept})
	if err != nil {
		t.Fatalf("ProvenanceStatusBatch: %v", err)
	}
	if statuses[concept] != "complete" {
		t.Fatalf("pre-boundary node WITH derived_from should be 'complete', got %q", statuses[concept])
	}
}

// TestProvenanceStatusBatchMixed asserts a single batched call classifies a mix.
func TestProvenanceStatusBatchMixed(t *testing.T) {
	s := testStore(t)
	concept, _ := s.AddNode(&AddNodeOpts{Type: TypeConcept, Content: "synth", Importance: 0.8})
	ep, _ := s.AddNode(&AddNodeOpts{Type: TypeEpisode, Content: "src", Importance: 0.5})
	none, _ := s.AddNode(&AddNodeOpts{Type: TypeConcept, Content: "bare", Importance: 0.5})
	if err := s.ConsolidateInto(concept, []string{ep}); err != nil {
		t.Fatalf("ConsolidateInto: %v", err)
	}

	statuses, err := s.ProvenanceStatusBatch([]string{concept, none})
	if err != nil {
		t.Fatalf("ProvenanceStatusBatch: %v", err)
	}
	if statuses[concept] != "complete" {
		t.Fatalf("concept should be complete, got %q", statuses[concept])
	}
	if statuses[none] != "none" {
		t.Fatalf("bare node should be none, got %q", statuses[none])
	}
}

// TestProvenanceStatusBatchEmpty asserts an empty input is a clean empty result.
func TestProvenanceStatusBatchEmpty(t *testing.T) {
	s := testStore(t)
	statuses, err := s.ProvenanceStatusBatch(nil)
	if err != nil {
		t.Fatalf("ProvenanceStatusBatch(nil): %v", err)
	}
	if len(statuses) != 0 {
		t.Fatalf("empty input should give empty map, got %v", statuses)
	}
}

// TestProvenanceStatusNoStoredColumn asserts provenance_status is computed, NOT
// a stored nodes column (AC6 — cheap, no schema column).
func TestProvenanceStatusNoStoredColumn(t *testing.T) {
	s := testStore(t)
	if nodeColumns(t, s)["provenance_status"] {
		t.Fatal("provenance_status must NOT be a stored nodes column")
	}
}
