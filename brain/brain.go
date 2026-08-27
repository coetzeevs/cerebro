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

// ProjectPath returns the SQLite path for a project directory, resolving
// symlinks before hashing per Operational Ontology §5.14 rule 26 (HS-008).
//
// Behaviour: EvalSymlinks(filepath.Abs(projectDir)). If EvalSymlinks fails
// (e.g. the path does not exist yet), the function falls back to the absolute
// path with no symlink resolution, preserving pre-HS-008 hashing for that case.
// This is intentional: a caller bootstrapping a brain for a path that does not
// yet exist (CI runner pre-checkout, `cerebro pi-init` on a freshly-cloned repo
// that pi-cerebro will later populate) still gets a deterministic hash.
//
// Order of resolution is Abs-then-EvalSymlinks (not EvalSymlinks-then-Abs)
// because EvalSymlinks on a relative path is implementation-defined; running
// Abs first gives EvalSymlinks an absolute starting point.
func ProjectPath(projectDir string) string {
	abs, _ := filepath.Abs(projectDir)
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		resolved = abs // fall back to abs (pre-HS-008 behaviour) when path does not exist
	}
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(resolved)))
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
		ProvenanceRoot: o.ProvenanceRoot,
		OriginActor:    o.OriginActor,
		OriginChannel:  o.OriginChannel,
		OriginSession:  o.OriginSession,
		OriginHost:     o.OriginHost,
	})
	if err != nil {
		return "", err
	}

	// Generate and store embedding
	if err := b.embedAndStore(id, content); err != nil {
		// Node is stored but embedding failed — mark as pending, and say so:
		// silent swallowing is how invisible-to-vector-recall debt accrued
		// (agentic-h6gc). Recover with `cerebro embed --pending`.
		fmt.Fprintf(os.Stderr, "Warning: embedding failed for %s: %v (node stored; run `cerebro embed --pending`)\n", id[:8], err)
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
		Subtype:    o.Subtype,
	}

	if err := b.store.UpdateNode(id, storeOpts); err != nil {
		return err
	}

	// Re-embed if content changed
	if o.Content != nil {
		if err := b.embedAndStore(id, *o.Content); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: embedding failed for %s: %v (update saved; run `cerebro embed --pending`)\n", id[:8], err)
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
		OriginActor:    o.OriginActor,
		OriginChannel:  o.OriginChannel,
		OriginSession:  o.OriginSession,
		OriginHost:     o.OriginHost,
	})
	if err != nil {
		return "", err
	}

	if err := b.embedAndStore(newID, content); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: embedding failed for %s: %v (node stored; run `cerebro embed --pending`)\n", newID[:8], err)
		_ = b.store.SetMeta("has_pending_embeddings", "true")
	}

	return newID, nil
}

// Reinforce increments a node's access count and updates timestamps.
func (b *Brain) Reinforce(id string) error {
	return b.store.ReinforceNode(id)
}

// AddEdge creates a relationship between two nodes, carrying the optional
// bi-temporal validity window (agentic-xtzn). Re-adding an existing
// (source, target, relation) edge re-asserts the full window in place via the
// store-layer upsert and returns the persisted id. Pass a zero AddEdgeOpts for
// an open-ended (NULL/NULL) edge — the pre-xtzn default.
func (b *Brain) AddEdge(sourceID, targetID, relation string, opts store.AddEdgeOpts) (int64, error) { //nolint:gocritic // hugeParam: value-struct opts, OO-011 precedent (cb41ad95)
	return b.store.AddEdge(sourceID, targetID, relation, opts)
}

// MarkConsolidated marks episodes as consolidated.
func (b *Brain) MarkConsolidated(ids []string) error {
	return b.store.MarkConsolidated(ids)
}

// Consolidate flips each source episode to consolidated AND auto-writes a
// derived_from edge from the into-node to each source, in a single atomic
// transaction (agentic-lbjg AC3). Fail-closed: the into-node and every source
// must resolve as an episode, else a non-zero error with zero partial write.
// Idempotent (UNIQUE(source,target,relation) upsert). This is additive — the
// existing status-only MarkConsolidated is unchanged.
func (b *Brain) Consolidate(intoID string, episodeIDs []string) error {
	return b.store.ConsolidateInto(intoID, episodeIDs)
}

// RecordOutcome records an agent-supplied outcome signal (success/failure)
// on a memory — agentic-do71. Counters live in metadata and multiply the
// composite score at retrieval.
func (b *Brain) RecordOutcome(id string, success bool) error {
	return b.store.RecordOutcome(id, success)
}

// ForgetSubject bulk-forgets nodes about a subject (content substring,
// optional subtype), cascading embeddings/FTS/edges — agentic-dpgh. dryRun
// selects without writing; hard deletes rows instead of archiving.
func (b *Brain) ForgetSubject(pattern, subtype string, hard, dryRun bool) (*store.ForgetResult, error) {
	return b.store.ForgetSubject(pattern, subtype, hard, dryRun)
}

// ConsolidationCandidates surfaces rollup candidates — active episodes
// grouped by subtype, biggest groups first, oldest first within a group
// (agentic-eq7a). The agent synthesizes; cerebro only selects.
func (b *Brain) ConsolidationCandidates(perGroupLimit int) ([]store.CandidateGroup, error) {
	return b.store.ConsolidationCandidates(perGroupLimit)
}

// WalkProvenance returns the derived_from lineage chain walked outward from id up
// to depth hops (agentic-lbjg AC5): WalkRelation(id, derived_from, depth,
// outgoing=true). The start node is first at depth 0; every reachable source
// appears exactly once at its minimum BFS depth; cycles terminate on the visited
// set. depth<=0 returns just the start node.
func (b *Brain) WalkProvenance(id string, depth int) ([]store.NodeWithDepth, error) {
	return b.store.WalkRelation(id, store.RelationDerivedFrom, depth, true)
}

// ProvenanceStatus returns id -> provenance_status (complete|none|legacy) for the
// given node IDs, computed at query time in one batched pass (no N+1) —
// agentic-lbjg AC6.
func (b *Brain) ProvenanceStatus(ids []string) (map[string]string, error) {
	return b.store.ProvenanceStatusBatch(ids)
}

// SetMeta writes a schema_meta key (used by config and feature gates).
func (b *Brain) SetMeta(key, value string) error {
	return b.store.SetMeta(key, value)
}

// GetMeta reads a schema_meta key; missing keys return an empty string.
func (b *Brain) GetMeta(key string) (string, error) {
	return b.store.GetMeta(key)
}

// RegisterRelation records a relation name (with an optional traversal class)
// in the typed-relation registry — agentic-8l2g. Idempotent.
func (b *Brain) RegisterRelation(name, class string) error {
	return b.store.RegisterRelation(name, class)
}

// ListRelations returns every registered relation, name-ordered.
func (b *Brain) ListRelations() ([]store.Relation, error) {
	return b.store.ListRelations()
}

// RemoveRelation deletes a relation from the registry (existing edges keep it).
func (b *Brain) RemoveRelation(name string) error {
	return b.store.RemoveRelation(name)
}

// RelationRegistered reports whether the named relation is in the registry.
func (b *Brain) RelationRegistered(name string) (bool, error) {
	return b.store.RelationRegistered(name)
}

// OriginStatus classifies a node's origin record (recorded|legacy|unknown)
// against the brain's origin-convention boundary — agentic-goc7.
func (b *Brain) OriginStatus(n *store.Node) string {
	boundary, err := b.store.OriginBoundary()
	if err != nil {
		boundary = nil
	}
	return store.OriginStatusFor(n, boundary)
}

// ResolveID resolves a full UUID or short prefix to a full node ID.
func (b *Brain) ResolveID(prefix string) (string, error) {
	return b.store.ResolvePrefix(prefix)
}

// Get retrieves a node with its edges. When asOf is non-nil, only edges valid
// at that instant are returned (agentic-xtzn); nil returns all edges
// (pre-xtzn behaviour).
func (b *Brain) Get(id string, asOf *time.Time) (*store.NodeWithEdges, error) {
	return b.store.GetNodeWithEdges(id, asOf)
}

// List returns nodes matching the given filters.
func (b *Brain) List(opts store.ListNodesOpts) ([]store.Node, error) { //nolint:gocritic // hugeParam: ListNodesOpts is intentionally a value type for API clarity; passed by value at call sites
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
// subtypeFilter, when non-nil, post-filters results by subtype after composite
// scoring and graph expansion. This preserves threshold and ranking semantics:
// composite scoring and --threshold cutoff are applied first (inside VectorSearch),
// then ExpandGraph adds graph neighbours, then the subtype filter is applied.
// Note: the filter may shrink the result count below `limit`; the caller's
// `--limit` is a ceiling, not a guarantee (threshold can already shrink results).
// Pass nil for subtypeFilter to get pre-OO-011 behaviour (no subtype filter).
//
// asOf, when non-nil, threads a bi-temporal as-of instant into graph expansion
// (agentic-xtzn): ExpandGraph then traverses only edges valid at that instant.
// Pass nil for asOf to get pre-xtzn behaviour (no validity filter; the edge
// predicate is omitted entirely). NOTE: when the lazy-expansion gate fires
// (agentic-73l6), ExpandGraph is skipped and asOf has no effect on that query —
// no edges are traversed, so there is nothing to filter (TL-PI-N2).
func (b *Brain) Search(ctx context.Context, query string, limit int, threshold float64, subtypeFilter *string, asOf *time.Time) ([]store.ScoredNode, error) {
	if b.embedder.Dimensions() == 0 {
		return nil, fmt.Errorf("no embedding provider configured — search requires embeddings")
	}

	vec, err := b.embedder.Embed(ctx, query)
	if err != nil {
		// N3 availability fallback: a configured-but-unavailable embedder
		// (e.g. Ollama down) must not take recall down with it — the BM25
		// keyword lane needs no embedding. Degrade to keyword-only; the
		// 'none' provider never reaches here (Dimensions()==0 precondition
		// above), so configured-out behaviour is unchanged.
		return b.searchKeywordOnly(query, limit, subtypeFilter, err)
	}

	// Reranking is OFF by default (agentic-2ixw). When disabled, this is the
	// exact pre-rerank path: VectorSearch(limit) → ExpandGraph(limit) → filter,
	// so eval metrics are byte-identical to the baseline by construction (AC2b).
	if resolveRerankEnabled(b.store) {
		return b.searchReranked(ctx, query, vec, limit, threshold, subtypeFilter, asOf)
	}

	results, err := b.store.VectorSearch(vec, limit, threshold)
	if err != nil {
		return nil, err
	}

	// Lazy expansion gate (agentic-73l6): skip ExpandGraph when the vector
	// top-K is already confident. The else-branch is the pre-feature path
	// verbatim; at (0.0, 0.0) the predicate is constant-false (AC3).
	var expanded []store.ScoredNode
	if shouldSkipExpansion(results, limit, resolveExpandThreshold(b.store), resolveExpandSpreadThreshold(b.store)) {
		noteExpansionSkipped(b.store)
		expanded = cutByScore(results, limit)
	} else {
		expanded, err = b.store.ExpandGraph(results, limit, asOf)
		if err != nil {
			return nil, err
		}
	}

	// Compose the BM25 keyword lane (agentic-2lak) BEFORE the cut/filter. When
	// bm25_enabled=false this is the literal pre-BM25 path (TL finding 2); when
	// enabled, the keyword-aware fused set replaces the composite order. The cut
	// to limit is then applied so a keyword-only node can enter the result set.
	fused := b.fuseKeywordLane(query, expanded, limit)
	if limit > 0 && len(fused) > limit {
		fused = fused[:limit]
	}

	// Post-fusion subtype filter: applied after all scoring so that composite
	// scores and threshold are unaffected. Every returned node is guaranteed to
	// match the requested subtype.
	return filterScoredNodesBySubtype(fused, subtypeFilter), nil
}

// searchReranked is the enabled-path recall pipeline (agentic-2ixw): it
// over-retrieves max(limit, rerankOverRetrieve) candidates, combines the
// composite ranking with the reranker ranking per the configured rerank_fusion
// mode (RRF by default, legacy pure-reorder when set to "reorder"), then cuts
// to limit and applies the subtype filter. The composite score is preserved on
// each node — fusion governs ordering only. On any reranker failure
// applyRerankWithFusion degrades to the composite order, so the AC4-NR
// non-regression floor holds.
func (b *Brain) searchReranked(ctx context.Context, query string, vec []float32, limit int, threshold float64, subtypeFilter *string, asOf *time.Time) ([]store.ScoredNode, error) {
	over := limit
	if rerankOverRetrieve > over {
		over = rerankOverRetrieve
	}

	results, err := b.store.VectorSearch(vec, over, threshold)
	if err != nil {
		return nil, err
	}

	// Lazy expansion gate (agentic-73l6) — same seam as the disabled path,
	// evaluated on this site's own over-retrieved result set.
	var expanded []store.ScoredNode
	if shouldSkipExpansion(results, over, resolveExpandThreshold(b.store), resolveExpandSpreadThreshold(b.store)) {
		noteExpansionSkipped(b.store)
		expanded = cutByScore(results, over)
	} else {
		expanded, err = b.store.ExpandGraph(results, over, asOf)
		if err != nil {
			return nil, err
		}
	}

	// Compose the BM25 keyword lane into the over-retrieved candidate set BEFORE
	// the reranker runs (agentic-2lak D4), so the 2ixw reranker receives a
	// keyword-aware-but-composite-ordered set exactly as it does today. The
	// reranker code and its config keys are untouched. When bm25_enabled=false
	// fuseKeywordLane is the identity, so this path is byte-identical to 2ixw.
	fused := b.fuseKeywordLane(query, expanded, over)

	reranked := applyRerankWithFusion(ctx, newReranker(b.store), query, fused, limit, resolveRerankFusion(b.store))
	return filterScoredNodesBySubtype(reranked, subtypeFilter), nil
}

// SearchWithGlobal searches both the project store (weight 1.0) and the global
// store (weight 0.7), merges results, and returns the top-K by weighted score.
// subtypeFilter, when non-nil, is applied post-merge (after ExpandGraph on both
// stores) so that every returned node is guaranteed to match the subtype.
// Pass nil for subtypeFilter to get pre-OO-011 behaviour (no subtype filter).
//
// asOf threads a bi-temporal as-of instant into BOTH stores' graph expansion
// (agentic-xtzn); nil omits the validity predicate (pre-xtzn behaviour). As in
// Search, a fired lazy-expansion gate makes asOf a no-op for that store's query
// (TL-PI-N2).
func (b *Brain) SearchWithGlobal(ctx context.Context, query string, limit int, threshold float64, global *Brain, subtypeFilter *string, asOf *time.Time) ([]store.ScoredNode, error) {
	if b.embedder.Dimensions() == 0 {
		return nil, fmt.Errorf("no embedding provider configured — search requires embeddings")
	}

	vec, err := b.embedder.Embed(ctx, query)
	if err != nil {
		// N3 availability fallback (see Search): keyword-only on the PROJECT
		// store. The global store's keyword lane is out of scope by the same
		// contract that excludes it from BM25 fusion — the warning says so.
		return b.searchKeywordOnly(query, limit, subtypeFilter, err)
	}

	// Over-retrieve width per store. Disabled path keeps today's limit*2 merge
	// pool byte-identical (AC2b). Enabled path widens to max(limit*2,
	// rerankOverRetrieve) so the rerank window is never narrower than the
	// existing global merge pool (M1, Tech Lead review).
	rerankOn := resolveRerankEnabled(b.store)
	perStore := limit * 2
	if rerankOn && rerankOverRetrieve > perStore {
		perStore = rerankOverRetrieve
	}

	// Lazy expansion gate (agentic-73l6): thresholds are resolved ONCE per
	// query from the PROJECT brain's config (the resolveRerankEnabled(b.store)
	// precedent above — the project brain's config governs the whole call).
	// Each store's expansion is then gated independently on its own result
	// set's confidence, and BOTH skip events — including the global store's —
	// increment the PROJECT brain's stats.expansion_skips counter (TL Finding
	// 2: a single counter keeps the skip-rate arithmetic on one brain; do not
	// "fix" the global site to global.store).
	expTh, spTh := resolveExpandThreshold(b.store), resolveExpandSpreadThreshold(b.store)

	// Search project store
	projectResults, err := b.store.VectorSearch(vec, perStore, threshold)
	if err != nil {
		return nil, fmt.Errorf("project search: %w", err)
	}
	if shouldSkipExpansion(projectResults, perStore, expTh, spTh) {
		noteExpansionSkipped(b.store)
		projectResults = cutByScore(projectResults, perStore)
	} else {
		projectResults, err = b.store.ExpandGraph(projectResults, perStore, asOf)
		if err != nil {
			return nil, fmt.Errorf("project graph expansion: %w", err)
		}
	}

	// Search global store
	globalResults, err := global.store.VectorSearch(vec, perStore, threshold)
	if err != nil {
		return nil, fmt.Errorf("global search: %w", err)
	}
	if shouldSkipExpansion(globalResults, perStore, expTh, spTh) {
		noteExpansionSkipped(b.store) // project brain's counter — see above
		globalResults = cutByScore(globalResults, perStore)
	} else {
		globalResults, err = global.store.ExpandGraph(globalResults, perStore, asOf)
		if err != nil {
			return nil, fmt.Errorf("global graph expansion: %w", err)
		}
	}

	if rerankOn {
		// Rerank the MERGED pool once, then cut to limit (M1: do not rerank the
		// project and global pools separately). The 0.7 global weighting and
		// mergeSearchResults semantics are untouched — merge keeps the full
		// pool by passing a wide ceiling, rerank reorders, then we cut to limit.
		merged := mergeSearchResults(projectResults, globalResults, perStore*2)
		// Compose the project-store keyword lane into the merged pool before the
		// reranker (agentic-2lak). The global store's keyword lane is out of scope
		// for this single-query path; fusion is the identity when bm25 disabled.
		fused := b.fuseKeywordLane(query, merged, perStore*2)
		reranked := applyRerankWithFusion(ctx, newReranker(b.store), query, fused, limit, resolveRerankFusion(b.store))
		return filterScoredNodesBySubtype(reranked, subtypeFilter), nil
	}

	// Disabled-BM25 path is the LITERAL pre-BM25 merge-then-cut (TL finding 2):
	// merge with the limit ceiling exactly as before, no fusion. This preserves
	// the AC4-NR same-session disabled floor byte-for-byte.
	if !resolveBM25Enabled(b.store) {
		merged := mergeSearchResults(projectResults, globalResults, limit)
		return filterScoredNodesBySubtype(merged, subtypeFilter), nil
	}

	// Enabled: merge a wide pool so a keyword-only node can enter, fuse the
	// keyword lane, then cut to limit.
	merged := mergeSearchResults(projectResults, globalResults, perStore*2)
	fused := b.fuseKeywordLane(query, merged, perStore*2)
	if limit > 0 && len(fused) > limit {
		fused = fused[:limit]
	}

	// Post-merge subtype filter: applied after all scoring and graph expansion.
	return filterScoredNodesBySubtype(fused, subtypeFilter), nil
}

// filterScoredNodesBySubtype applies an optional subtype filter to a slice of
// ScoredNode results. It is called after ExpandGraph so that composite scoring
// and threshold semantics are unaffected.
//
// Filter semantics:
//   - nil filter: return the input slice unchanged (no-op; backward compatible).
//   - &"":  return only nodes whose Subtype is "" (i.e., NULL in the database).
//   - &"x": return only nodes whose Subtype equals "x" (exact match).
func filterScoredNodesBySubtype(nodes []store.ScoredNode, filter *string) []store.ScoredNode {
	if filter == nil {
		return nodes
	}
	out := nodes[:0:0] // zero-length slice sharing no backing array with input
	for i := range nodes {
		if nodes[i].Subtype == *filter {
			out = append(out, nodes[i])
		}
	}
	return out
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
// Oversized content chunks + mean-pools via embedContent (agentic-h6gc), so
// large memories embed at write time instead of silently going pending.
func (b *Brain) embedAndStore(nodeID, content string) error {
	if b.embedder.Dimensions() == 0 {
		return nil // noop provider
	}

	vec, err := b.embedContent(context.Background(), content)
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
		// Promotion copies the memory: the original author's identity travels
		// with it (the promotion event itself is in the provenance metadata).
		OriginActor:   srcNode.OriginActor,
		OriginChannel: srcNode.OriginChannel,
		OriginSession: srcNode.OriginSession,
		OriginHost:    srcNode.OriginHost,
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
	Subtype        string
	Metadata       json.RawMessage
	Importance     float64
	ProvenanceRoot bool
	OriginActor    string
	OriginChannel  string
	OriginSession  string
	OriginHost     string
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

// WithProvenanceRoot marks the new node as a first-class provenance source
// (nodes.provenance_root=1) — agentic-lbjg. Additive: a flagless Add still
// defaults provenance_root to 0, so existing callers are unaffected.
func WithProvenanceRoot() AddOption {
	return func(o *addOptions) { o.ProvenanceRoot = true }
}

// WithOrigin stamps the write-time identity on the new node (agentic-goc7):
// who wrote it (actor), through what (channel), from which session and host.
// Empty fields store as NULL — origin is recorded, never fabricated, so an
// option-less Add leaves all four unset.
func WithOrigin(actor, channel, session, host string) AddOption {
	return func(o *addOptions) {
		o.OriginActor = actor
		o.OriginChannel = channel
		o.OriginSession = session
		o.OriginHost = host
	}
}

type updateOptions struct {
	Content    *string
	Metadata   json.RawMessage
	Importance *float64
	// Subtype, when non-nil, updates the node's subtype.
	// &"" clears to NULL; &"x" sets to "x"; nil leaves unchanged.
	Subtype *string
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

// WithUpdatedSubtype sets or clears the subtype on an existing node.
// Pass an empty string to clear the subtype to NULL.
// Pass a non-empty string to set the subtype to that value.
// Note: subtype changes stamp updated_at because subtype is knowledge-classification
// metadata — it changes what the memory means to the retrieval taxonomy.
func WithUpdatedSubtype(s string) UpdateOption {
	return func(o *updateOptions) { o.Subtype = &s }
}

// TouchAccessed forwards retrieval-usage telemetry to the store (N2). The CLI
// calls this for query-mode recall/search results; library consumers (and the
// eval harness, which must not perturb the brain between A/B runs) do not get
// implicit touching — Search stays read-only.
func (b *Brain) TouchAccessed(ids []string) error {
	return b.store.TouchAccessed(ids)
}
