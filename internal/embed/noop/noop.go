// Package noop implements a no-op embedding provider for graph-only mode.
package noop

import "context"

// Provider returns nil embeddings. Used when no embedding provider is configured.
type Provider struct{}

func New() *Provider { return &Provider{} }

func (p *Provider) Embed(_ context.Context, _ string) ([]float32, error) {
	return nil, nil
}

func (p *Provider) Dimensions() int { return 0 }
func (p *Provider) Model() string   { return "none" }
