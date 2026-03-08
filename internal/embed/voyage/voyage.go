// Package voyage implements the embedding provider for Voyage AI's API.
package voyage

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
)

// Provider generates embeddings via Voyage AI's HTTP API.
type Provider struct {
	apiKey     string
	model      string
	dimensions int
	client     *http.Client
}

// New creates a Voyage AI embedding provider.
// API key is read from the CEREBRO_VOYAGE_API_KEY environment variable if not provided.
func New(apiKey, model string, dimensions int) *Provider {
	if apiKey == "" {
		apiKey = os.Getenv("CEREBRO_VOYAGE_API_KEY")
	}
	if model == "" {
		model = "voyage-3.5"
	}
	if dimensions == 0 {
		dimensions = 1024
	}
	return &Provider{
		apiKey:     apiKey,
		model:      model,
		dimensions: dimensions,
		client:     &http.Client{},
	}
}

type embeddingRequest struct {
	Input     []string `json:"input"`
	Model     string   `json:"model"`
	InputType string   `json:"input_type,omitempty"`
}

type embeddingResponse struct {
	Data []struct {
		Embedding []float64 `json:"embedding"`
	} `json:"data"`
}

func (p *Provider) Embed(ctx context.Context, text string) ([]float32, error) {
	if p.apiKey == "" {
		return nil, fmt.Errorf("voyage API key not set (CEREBRO_VOYAGE_API_KEY)")
	}

	payload, err := json.Marshal(embeddingRequest{
		Input: []string{text},
		Model: p.model,
	})
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.voyageai.com/v1/embeddings", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("calling Voyage API: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("voyage API returned %d: %s", resp.StatusCode, string(body))
	}

	var result embeddingResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	if len(result.Data) == 0 {
		return nil, fmt.Errorf("voyage API returned no embeddings")
	}

	vec := make([]float32, len(result.Data[0].Embedding))
	for i, v := range result.Data[0].Embedding {
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
