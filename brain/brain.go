// Package brain is the public Go API for Cerebro.
// It wraps the internal store and embedding packages into a unified interface.
package brain

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"

	"github.com/coetzeevs/cerebro/internal/embed"
	"github.com/coetzeevs/cerebro/internal/embed/noop"
	"github.com/coetzeevs/cerebro/internal/embed/ollama"
	"github.com/coetzeevs/cerebro/internal/embed/voyage"
	"github.com/coetzeevs/cerebro/internal/store"
)

// Brain is the primary handle for a Cerebro memory store.
type Brain struct {
	store    *store.Store
	embedder embed.Provider
}

// cerebroDir returns the base Cerebro directory (~/.cerebro).
func cerebroDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".cerebro")
}

// ProjectPath returns the SQLite path for a project directory.
func ProjectPath(projectDir string) string {
	abs, _ := filepath.Abs(projectDir)
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(abs)))
	return filepath.Join(cerebroDir(), "projects", hash+".sqlite")
}

// GlobalPath returns the SQLite path for the global store.
func GlobalPath() string {
	return filepath.Join(cerebroDir(), "global.sqlite")
}

// Init creates and initializes a new brain at the given path.
// It also sets up the embedding provider based on configuration and creates the vector table.
func Init(path string, cfg EmbedConfig) (*Brain, error) {
	s, err := store.Init(path)
	if err != nil {
		return nil, err
	}

	embedder := newEmbedder(cfg)

	// Set meta for embedding config
	if err := s.SetMeta("embedding_provider", cfg.Provider); err != nil {
		_ = s.Close()
		return nil, fmt.Errorf("setting embedding_provider: %w", err)
	}
	if err := s.SetMeta("embedding_model", embedder.Model()); err != nil {
		_ = s.Close()
		return nil, fmt.Errorf("setting embedding_model: %w", err)
	}
	if err := s.SetMeta("embedding_dimensions", strconv.Itoa(embedder.Dimensions())); err != nil {
		_ = s.Close()
		return nil, fmt.Errorf("setting embedding_dimensions: %w", err)
	}

	// Create vector table if embedding is enabled
	if embedder.Dimensions() > 0 {
		if err := s.InitVectorTable(embedder.Dimensions()); err != nil {
			_ = s.Close()
			return nil, fmt.Errorf("initializing vector table: %w", err)
		}
	}

	return &Brain{store: s, embedder: embedder}, nil
}

// Open opens an existing brain at the given path.
func Open(path string) (*Brain, error) {
	s, err := store.Open(path)
	if err != nil {
		return nil, err
	}

	// Read embedding config from meta
	provider, _ := s.GetMeta("embedding_provider")
	model, _ := s.GetMeta("embedding_model")
	dimStr, _ := s.GetMeta("embedding_dimensions")
	dim, _ := strconv.Atoi(dimStr)

	embedder := newEmbedder(EmbedConfig{
		Provider:   provider,
		Model:      model,
		Dimensions: dim,
	})

	return &Brain{store: s, embedder: embedder}, nil
}

// Close closes the brain's database connection.
func (b *Brain) Close() error {
	return b.store.Close()
}

// Store returns the underlying store for advanced operations.
func (b *Brain) Store() *store.Store {
	return b.store
}

// EmbedConfig configures the embedding provider.
type EmbedConfig struct {
	Provider   string // "ollama", "voyage", "none"
	Model      string
	Dimensions int
	BaseURL    string // Ollama base URL
	APIKey     string // Voyage API key
}

func newEmbedder(cfg EmbedConfig) embed.Provider {
	switch cfg.Provider {
	case "ollama":
		return ollama.New(cfg.BaseURL, cfg.Model, cfg.Dimensions)
	case "voyage":
		return voyage.New(cfg.APIKey, cfg.Model, cfg.Dimensions)
	case "none", "":
		return noop.New()
	default:
		return noop.New()
	}
}

// Add stores a new memory node and generates its embedding.
func (b *Brain) Add(content string, nodeType store.NodeType, opts ...AddOption) (string, error) {
	o := addDefaults()
	for _, fn := range opts {
		fn(&o)
	}

	id, err := b.store.AddNode(&store.AddNodeOpts{
		Type:           nodeType,
		Subtype:        o.Subtype,
		Content:        content,
		Metadata:       o.Metadata,
		Importance:     o.Importance,
		EmbeddingModel: b.embedder.Model(),
	})
	if err != nil {
		return "", err
	}

	// Generate and store embedding
	if err := b.embedAndStore(id, content); err != nil {
		// Node is stored but embedding failed — mark as pending
		_ = b.store.SetMeta("has_pending_embeddings", "true")
	}

	return id, nil
}

// Update modifies an existing node. If content changes, re-embeds.
func (b *Brain) Update(id string, opts ...UpdateOption) error {
	o := updateDefaults()
	for _, fn := range opts {
		fn(&o)
	}

	storeOpts := store.UpdateNodeOpts{
		Content:    o.Content,
		Metadata:   o.Metadata,
		Importance: o.Importance,
	}

	if err := b.store.UpdateNode(id, storeOpts); err != nil {
		return err
	}

	// Re-embed if content changed
	if o.Content != nil {
		if err := b.embedAndStore(id, *o.Content); err != nil {
			_ = b.store.SetMeta("has_pending_embeddings", "true")
		}
	}

	return nil
}

// Supersede marks an old node as superseded and creates a new replacement.
func (b *Brain) Supersede(oldID, content string, nodeType store.NodeType, opts ...AddOption) (string, error) {
	o := addDefaults()
	for _, fn := range opts {
		fn(&o)
	}

	newID, err := b.store.SupersedeNode(oldID, &store.AddNodeOpts{
		Type:           nodeType,
		Subtype:        o.Subtype,
		Content:        content,
		Metadata:       o.Metadata,
		Importance:     o.Importance,
		EmbeddingModel: b.embedder.Model(),
	})
	if err != nil {
		return "", err
	}

	if err := b.embedAndStore(newID, content); err != nil {
		_ = b.store.SetMeta("has_pending_embeddings", "true")
	}

	return newID, nil
}

// Reinforce increments a node's access count and updates timestamps.
func (b *Brain) Reinforce(id string) error {
	return b.store.ReinforceNode(id)
}

// AddEdge creates a relationship between two nodes.
func (b *Brain) AddEdge(sourceID, targetID, relation string) (int64, error) {
	return b.store.AddEdge(sourceID, targetID, relation)
}

// MarkConsolidated marks episodes as consolidated.
func (b *Brain) MarkConsolidated(ids []string) error {
	return b.store.MarkConsolidated(ids)
}

// Get retrieves a node with its edges.
func (b *Brain) Get(id string) (*store.NodeWithEdges, error) {
	return b.store.GetNodeWithEdges(id)
}

// List returns nodes matching the given filters.
func (b *Brain) List(opts store.ListNodesOpts) ([]store.Node, error) {
	return b.store.ListNodes(opts)
}

// Stats returns brain health metrics.
func (b *Brain) Stats() (*store.Stats, error) {
	return b.store.GetStats()
}

// GC evicts decayed memories to the archive. If dryRun is true,
// it reports what would be archived without modifying data.
func (b *Brain) GC(threshold float64, dryRun bool) (*store.GCResult, error) {
	return b.store.GC(threshold, dryRun)
}

// Search performs vector similarity search and returns scored results.
func (b *Brain) Search(ctx context.Context, query string, limit int, threshold float64) ([]store.ScoredNode, error) {
	if b.embedder.Dimensions() == 0 {
		return nil, fmt.Errorf("no embedding provider configured — search requires embeddings")
	}

	vec, err := b.embedder.Embed(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("embedding query: %w", err)
	}

	results, err := b.store.VectorSearch(vec, limit, threshold)
	if err != nil {
		return nil, err
	}

	return b.store.ExpandGraph(results, limit)
}

// SearchWithGlobal searches both the project store (weight 1.0) and the global
// store (weight 0.7), merges results, and returns the top-K by weighted score.
func (b *Brain) SearchWithGlobal(ctx context.Context, query string, limit int, threshold float64, global *Brain) ([]store.ScoredNode, error) {
	if b.embedder.Dimensions() == 0 {
		return nil, fmt.Errorf("no embedding provider configured — search requires embeddings")
	}

	vec, err := b.embedder.Embed(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("embedding query: %w", err)
	}

	// Search project store
	projectResults, err := b.store.VectorSearch(vec, limit*2, threshold)
	if err != nil {
		return nil, fmt.Errorf("project search: %w", err)
	}
	projectResults, err = b.store.ExpandGraph(projectResults, limit*2)
	if err != nil {
		return nil, fmt.Errorf("project graph expansion: %w", err)
	}

	// Search global store
	globalResults, err := global.store.VectorSearch(vec, limit*2, threshold)
	if err != nil {
		return nil, fmt.Errorf("global search: %w", err)
	}
	globalResults, err = global.store.ExpandGraph(globalResults, limit*2)
	if err != nil {
		return nil, fmt.Errorf("global graph expansion: %w", err)
	}

	return mergeSearchResults(projectResults, globalResults, limit), nil
}

// mergeSearchResults merges project and global results.
// Project results keep their score. Global results are weighted by 0.7.
// If a node ID appears in both, the project version wins.
// Returns top-limit results sorted by score descending.
func mergeSearchResults(project, global []store.ScoredNode, limit int) []store.ScoredNode {
	seen := make(map[string]bool, len(project))
	merged := make([]store.ScoredNode, 0, len(project)+len(global))

	// Project results at full weight
	for i := range project {
		seen[project[i].ID] = true
		merged = append(merged, project[i])
	}

	// Global results weighted by 0.7, skip duplicates
	for i := range global {
		if seen[global[i].ID] {
			continue
		}
		global[i].Score *= 0.7
		merged = append(merged, global[i])
	}

	// Sort by score descending
	sort.Slice(merged, func(i, j int) bool {
		return merged[i].Score > merged[j].Score
	})

	if len(merged) > limit {
		merged = merged[:limit]
	}
	return merged
}

// embedAndStore generates an embedding and stores it in vec_nodes.
func (b *Brain) embedAndStore(nodeID, content string) error {
	if b.embedder.Dimensions() == 0 {
		return nil // noop provider
	}

	vec, err := b.embedder.Embed(context.Background(), content)
	if err != nil {
		return err
	}

	return b.store.StoreEmbedding(nodeID, vec)
}

// Promote copies a node from this brain to the destination (global) brain.
// The global copy gets importance=0.5 and provenance metadata.
// The source node's metadata is updated with a promoted_to_global reference.
// Use WithPromoteContent to supply generalized content.
func (b *Brain) Promote(ctx context.Context, nodeID string, dst *Brain, opts ...PromoteOption) (string, error) {
	var o promoteOptions
	for _, fn := range opts {
		fn(&o)
	}

	// Load source node
	srcNode, err := b.store.GetNode(nodeID)
	if err != nil {
		return "", fmt.Errorf("reading source node: %w", err)
	}

	content := srcNode.Content
	if o.Content != "" {
		content = o.Content
	}

	// Build provenance metadata for the global copy
	provenance := map[string]any{
		"promoted_from_node":    nodeID,
		"promoted_from_project": projectHash(b.store.Path()),
		"promoted_at":           time.Now().UTC().Format(time.RFC3339),
	}
	globalMeta, err := mergeMetadata(srcNode.Metadata, provenance)
	if err != nil {
		return "", fmt.Errorf("building provenance metadata: %w", err)
	}

	// Add to destination store with importance=0.5
	globalID, err := dst.store.AddNode(&store.AddNodeOpts{
		Type:           srcNode.Type,
		Subtype:        srcNode.Subtype,
		Content:        content,
		Metadata:       globalMeta,
		Importance:     0.5,
		EmbeddingModel: dst.embedder.Model(),
	})
	if err != nil {
		return "", fmt.Errorf("adding to global store: %w", err)
	}

	// Embed in destination store
	if dst.embedder.Dimensions() > 0 {
		vec, embedErr := dst.embedder.Embed(ctx, content)
		if embedErr == nil {
			_ = dst.store.StoreEmbedding(globalID, vec)
		}
	}

	// Update source node metadata with reference to global copy
	srcMeta, err := mergeMetadata(srcNode.Metadata, map[string]any{
		"promoted_to_global": globalID,
	})
	if err != nil {
		return globalID, nil // node was promoted, metadata update is best-effort
	}
	_ = b.store.UpdateNode(nodeID, store.UpdateNodeOpts{Metadata: srcMeta})

	return globalID, nil
}

// projectHash extracts the project hash from a store path like ~/.cerebro/projects/<hash>.sqlite.
func projectHash(storePath string) string {
	base := filepath.Base(storePath)
	ext := filepath.Ext(base)
	return base[:len(base)-len(ext)]
}

// mergeMetadata merges extra key-value pairs into existing JSON metadata.
func mergeMetadata(existing json.RawMessage, extra map[string]any) (json.RawMessage, error) {
	base := make(map[string]any)
	if len(existing) > 0 {
		if err := json.Unmarshal(existing, &base); err != nil {
			return nil, err
		}
	}
	for k, v := range extra {
		base[k] = v
	}
	return json.Marshal(base)
}

// Promote option types

type promoteOptions struct {
	Content string
}

// PromoteOption configures a Promote call.
type PromoteOption func(*promoteOptions)

// WithPromoteContent overrides the content for the global copy.
func WithPromoteContent(c string) PromoteOption {
	return func(o *promoteOptions) { o.Content = c }
}

// Option types

type addOptions struct {
	Subtype    string
	Metadata   json.RawMessage
	Importance float64
}

func addDefaults() addOptions {
	return addOptions{Importance: 0.5}
}

// AddOption configures an Add or Supersede call.
type AddOption func(*addOptions)

func WithSubtype(s string) AddOption     { return func(o *addOptions) { o.Subtype = s } }
func WithImportance(i float64) AddOption { return func(o *addOptions) { o.Importance = i } }
func WithMetadata(m json.RawMessage) AddOption {
	return func(o *addOptions) { o.Metadata = m }
}

type updateOptions struct {
	Content    *string
	Metadata   json.RawMessage
	Importance *float64
}

func updateDefaults() updateOptions { return updateOptions{} }

// UpdateOption configures an Update call.
type UpdateOption func(*updateOptions)

func WithContent(c string) UpdateOption {
	return func(o *updateOptions) { o.Content = &c }
}

func WithUpdatedImportance(i float64) UpdateOption {
	return func(o *updateOptions) { o.Importance = &i }
}
