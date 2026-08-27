package brain

// embed_pending.go — pending-embedding backfill + oversized-content chunking
// (agentic-h6gc).
//
// Every embed-failure path sets has_pending_embeddings, and Import's
// "nodes will need re-embedding" contract assumed a backfill existed — it
// never did, so failed/imported nodes stayed invisible to vector recall
// indefinitely. EmbedPending is that backfill. Oversized content (the
// installed Ollama hard-fails on inputs beyond ~6-7.8KB chars) chunks and
// mean-pools through embedContent, which the regular write paths share — so
// a large memory embeds at Add time instead of silently going pending.

import (
	"context"
	"fmt"
	"math"
	"os"
)

// embedChunkThreshold is the content size (chars) above which embedding
// chunks. Conservative floor under the observed Ollama nomic-embed-text
// failure band (~6-7.8KB chars, HTTP 500; live-measured 2026-08-25).
const embedChunkThreshold = 6000

// embedChunkSize is the per-chunk character budget, comfortably under the
// threshold so every chunk embeds on the constrained provider.
const embedChunkSize = 4000

// embedContent produces one vector for content: a single provider call for
// normal sizes; for oversized content, rune-safe chunks are embedded
// individually and mean-pooled, and the mean is L2-normalized so the stored
// vector lives on the same unit sphere as single-call embeddings (cosine /
// normalized-dot comparability).
func (b *Brain) embedContent(ctx context.Context, content string) ([]float32, error) {
	if len(content) <= embedChunkThreshold {
		return b.embedder.Embed(ctx, content)
	}

	chunks := chunkRunes(content, embedChunkSize)
	dims := b.embedder.Dimensions()
	sum := make([]float64, dims)
	for i, chunk := range chunks {
		vec, err := b.embedder.Embed(ctx, chunk)
		if err != nil {
			return nil, fmt.Errorf("embedding chunk %d/%d: %w", i+1, len(chunks), err)
		}
		if len(vec) != dims {
			return nil, fmt.Errorf("chunk %d/%d: provider returned %d dims, want %d", i+1, len(chunks), len(vec), dims)
		}
		for j, v := range vec {
			sum[j] += float64(v)
		}
	}

	n := float64(len(chunks))
	var norm float64
	for j := range sum {
		sum[j] /= n
		norm += sum[j] * sum[j]
	}
	norm = math.Sqrt(norm)
	out := make([]float32, dims)
	if norm == 0 {
		return out, nil
	}
	for j := range sum {
		out[j] = float32(sum[j] / norm)
	}
	return out, nil
}

// chunkRunes splits s into pieces of at most size runes, never splitting a
// rune. Byte-length per chunk can exceed size for multi-byte content but
// stays far under the provider threshold at these budgets.
func chunkRunes(s string, size int) []string {
	runes := []rune(s)
	var out []string
	for start := 0; start < len(runes); start += size {
		end := start + size
		if end > len(runes) {
			end = len(runes)
		}
		out = append(out, string(runes[start:end]))
	}
	return out
}

// EmbedPendingResult reports one backfill pass.
type EmbedPendingResult struct {
	// Embedded lists node IDs that gained a vector this pass.
	Embedded []string `json:"embedded"`
	// Failed maps node ID -> failure reason for nodes that stayed pending.
	Failed map[string]string `json:"failed,omitempty"`
	// Remaining is the pending count after the pass.
	Remaining int `json:"remaining"`
}

// EmbedPending embeds every active node lacking a vec_nodes row, reporting
// per-node results. has_pending_embeddings is cleared only when zero nodes
// remain pending; failures keep it set and are surfaced loudly — a silent
// embed failure is how this debt accumulated in the first place. Idempotent:
// nodes that already carry a vector are never re-embedded.
func (b *Brain) EmbedPending(ctx context.Context) (*EmbedPendingResult, error) {
	if b.embedder.Dimensions() == 0 {
		return nil, fmt.Errorf("no embedding provider configured (provider \"none\") — nothing can embed")
	}

	pending, err := b.store.PendingEmbeddingNodes()
	if err != nil {
		return nil, fmt.Errorf("selecting pending nodes: %w", err)
	}

	res := &EmbedPendingResult{Failed: map[string]string{}}
	for i := range pending {
		n := &pending[i]
		vec, err := b.embedContent(ctx, n.Content)
		if err == nil {
			err = b.store.StoreEmbedding(n.ID, vec)
		}
		if err != nil {
			res.Failed[n.ID] = err.Error()
			fmt.Fprintf(os.Stderr, "Warning: embedding failed for %s: %v (still pending)\n", n.ID[:8], err)
			continue
		}
		res.Embedded = append(res.Embedded, n.ID)
	}

	res.Remaining = len(pending) - len(res.Embedded)
	flag := "false"
	if res.Remaining > 0 {
		flag = "true"
	}
	if err := b.store.SetMeta("has_pending_embeddings", flag); err != nil {
		return res, fmt.Errorf("updating pending flag: %w", err)
	}
	return res, nil
}
