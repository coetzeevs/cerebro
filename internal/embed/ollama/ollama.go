// Package ollama implements the embedding provider for Ollama's local API.
package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// Provider generates embeddings via Ollama's HTTP API.
type Provider struct {
	baseURL    string
	model      string
	dimensions int
	client     *http.Client
}

// New creates an Ollama embedding provider.
func New(baseURL, model string, dimensions int) *Provider {
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	if model == "" {
		model = "nomic-embed-text"
	}
	if dimensions == 0 {
		dimensions = 768
	}
	return &Provider{
		baseURL:    baseURL,
		model:      model,
		dimensions: dimensions,
		client:     &http.Client{},
	}
}

type embeddingRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
}

type embeddingResponse struct {
	Embedding []float64 `json:"embedding"`
}

func (p *Provider) Embed(ctx context.Context, text string) ([]float32, error) {
	payload, err := json.Marshal(embeddingRequest{
		Model:  p.model,
		Prompt: text,
	})
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/api/embeddings", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("calling Ollama API: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck // best-effort close of response body

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ollama API returned %d: %s", resp.StatusCode, string(body))
	}

	var result embeddingResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	// Convert float64 to float32
	vec := make([]float32, len(result.Embedding))
	for i, v := range result.Embedding {
		vec[i] = float32(v)
	}

	return vec, nil
}

func (p *Provider) Dimensions() int {
	return p.dimensions
}

func (p *Provider) Model() string {
	return p.model
}
