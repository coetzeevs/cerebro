package store

import (
	"testing"
)

func TestGetEdgesBatch(t *testing.T) {
	s := testStore(t)

	// Create 4 nodes: A→B, A→C, D (isolated)
	idA, err := s.AddNode(&AddNodeOpts{Type: TypeConcept, Content: "A", Importance: 0.8})
	if err != nil {
		t.Fatalf("AddNode A: %v", err)
	}
	idB, err := s.AddNode(&AddNodeOpts{Type: TypeConcept, Content: "B", Importance: 0.6})
	if err != nil {
		t.Fatalf("AddNode B: %v", err)
	}
	idC, err := s.AddNode(&AddNodeOpts{Type: TypeProcedure, Content: "C", Importance: 0.7})
	if err != nil {
		t.Fatalf("AddNode C: %v", err)
	}
	idD, err := s.AddNode(&AddNodeOpts{Type: TypeEpisode, Content: "D", Importance: 0.5})
	if err != nil {
		t.Fatalf("AddNode D: %v", err)
	}

	if _, err := s.AddEdge(idA, idB, "relates_to"); err != nil {
		t.Fatalf("AddEdge A→B: %v", err)
	}
	if _, err := s.AddEdge(idA, idC, "supports"); err != nil {
		t.Fatalf("AddEdge A→C: %v", err)
	}

	// Batch query for A and D
	edgeMap, err := s.GetEdgesBatch([]string{idA, idD})
	if err != nil {
		t.Fatalf("GetEdgesBatch: %v", err)
	}

	// A should have 2 edges
	if len(edgeMap[idA]) != 2 {
		t.Errorf("expected 2 edges for A, got %d", len(edgeMap[idA]))
	}

	// D should have 0 edges
	if len(edgeMap[idD]) != 0 {
		t.Errorf("expected 0 edges for D, got %d", len(edgeMap[idD]))
	}

	// B should show up when queried (as target)
	edgeMap2, err := s.GetEdgesBatch([]string{idB})
	if err != nil {
		t.Fatalf("GetEdgesBatch B: %v", err)
	}
	if len(edgeMap2[idB]) != 1 {
		t.Errorf("expected 1 edge for B (as target), got %d", len(edgeMap2[idB]))
	}

	// Empty input should return empty map
	empty, err := s.GetEdgesBatch([]string{})
	if err != nil {
		t.Fatalf("GetEdgesBatch empty: %v", err)
	}
	if len(empty) != 0 {
		t.Errorf("expected empty map, got %d entries", len(empty))
	}
}
