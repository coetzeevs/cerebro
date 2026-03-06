// Package embed defines the embedding provider interface and registry.
package embed

import "context"

// Provider generates vector embeddings from text.
type Provider interface {
	// Embed generates an embedding vector for the given text.
	Embed(ctx context.Context, text string) ([]float32, error)

	// Dimensions returns the number of dimensions in the output vectors.
	Dimensions() int

	// Model returns the model identifier (e.g., "nomic-embed-text-v1.5").
	Model() string
}
