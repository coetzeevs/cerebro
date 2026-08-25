package brain

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/coetzeevs/cerebro/internal/store"
)

// TestAddWithOrigin: WithOrigin stamps the write-time identity through the
// brain layer; an option-less Add records nothing (never fabricated).
func TestAddWithOrigin(t *testing.T) {
	b := testBrain(t)

	id, err := b.Add("origin through brain", store.TypeEpisode,
		WithOrigin("agent", "cli", "sess-42", "machine-a"))
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	n, err := b.Get(id, nil)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if n.OriginActor != "agent" || n.OriginChannel != "cli" || n.OriginSession != "sess-42" || n.OriginHost != "machine-a" {
		t.Errorf("origin not stamped through brain layer: %+v", n)
	}

	bare, err := b.Add("no origin", store.TypeConcept)
	if err != nil {
		t.Fatalf("Add bare: %v", err)
	}
	nb, _ := b.Get(bare, nil)
	if nb.OriginActor != "" || nb.OriginHost != "" {
		t.Errorf("bare Add fabricated origin: %+v", nb)
	}
}

// TestSupersedeWithOrigin: the replacement carries the SUPERSEDER's identity.
func TestSupersedeWithOrigin(t *testing.T) {
	b := testBrain(t)

	old, err := b.Add("v1", store.TypeConcept, WithOrigin("human", "cli", "", "machine-a"))
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	repl, err := b.Supersede(old, "v2", store.TypeConcept,
		WithOrigin("agent", "hook", "sess-9", "machine-b"))
	if err != nil {
		t.Fatalf("Supersede: %v", err)
	}
	n, _ := b.Get(repl, nil)
	if n.OriginActor != "agent" || n.OriginChannel != "hook" || n.OriginHost != "machine-b" {
		t.Errorf("superseder origin not carried: %+v", n)
	}
}

// TestPromoteCarriesSourceOrigin: promotion is a COPY of an existing memory —
// the global replica keeps the original author's identity, not the promoter's.
func TestPromoteCarriesSourceOrigin(t *testing.T) {
	src := testBrain(t)
	dstPath := filepath.Join(t.TempDir(), "global.sqlite")
	dst, err := Init(dstPath, EmbedConfig{Provider: "none"})
	if err != nil {
		t.Fatalf("Init dst: %v", err)
	}
	t.Cleanup(func() { _ = dst.Close() })

	id, err := src.Add("promotable memory", store.TypeConcept,
		WithOrigin("human", "cli", "sess-1", "machine-a"))
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	globalID, err := src.Promote(context.Background(), id, dst)
	if err != nil {
		t.Fatalf("Promote: %v", err)
	}
	n, err := dst.Get(globalID, nil)
	if err != nil {
		t.Fatalf("Get global: %v", err)
	}
	if n.OriginActor != "human" || n.OriginHost != "machine-a" {
		t.Errorf("promotion dropped source origin: %+v", n)
	}
}
