package noop

import (
	"context"
	"testing"
)

func TestProvider(t *testing.T) {
	p := New()

	if p.Dimensions() != 0 {
		t.Errorf("expected dimensions=0, got %d", p.Dimensions())
	}
	if p.Model() != "none" {
		t.Errorf("expected model=none, got %q", p.Model())
	}

	vec, err := p.Embed(context.Background(), "test text")
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if vec != nil {
		t.Errorf("expected nil vector, got %v", vec)
	}
}
