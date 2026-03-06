# ADR-002: Embedding Strategy — Pluggable Provider with Local Default

## Status
Proposed

## Context

Cerebro needs to generate embedding vectors for every memory node to enable semantic similarity search. The choice of embedding approach affects:
- **Search quality** — how well similar memories are found
- **Latency** — how long it takes to embed a new memory
- **Storage** — how large the vector table grows
- **Infrastructure** — whether an API connection or local runtime is required
- **Portability** — whether embeddings work offline

The original architecture specified `float[1536]` dimensions (OpenAI's default), which implicitly coupled Cerebro to an API provider. The first revision hardcoded Ollama, which traded one coupling for another. Neither approach is right — Cerebro should be **provider-agnostic** while recommending sensible defaults.

## Decision

Treat embedding generation as a **pluggable provider** behind a uniform interface:

```
embed(text: string) → float[N]
```

Three provider tiers are supported:

| Tier | Examples | Tradeoff |
|------|----------|----------|
| **Local** | Ollama (nomic-embed-text, mxbai-embed-large) | Free, offline, requires local model runtime |
| **API** | Voyage AI (Anthropic's recommended provider), OpenAI, Cohere | Paid, requires internet + API key, zero local setup |
| **None** | — | Graph-only mode, no semantic search |

The **recommended default** is nomic-embed-text-v1.5 via Ollama (768-dim), but this is a recommendation, not a requirement. Users choose their provider at `cerebro init` or via configuration.

### Configuration

```toml
# ~/.cerebro/config.toml

[embeddings]
provider = "ollama"                    # "ollama" | "voyage" | "openai" | "none"
model = "nomic-embed-text"             # Provider-specific model identifier
dimensions = 768                       # Must match the model's output

[embeddings.ollama]
base_url = "http://localhost:11434"

[embeddings.voyage]
# api_key from env: CEREBRO_VOYAGE_API_KEY
model = "voyage-3.5"                   # Or voyage-code-3 for code-heavy projects
dimensions = 1024

[embeddings.openai]
# api_key from env: CEREBRO_OPENAI_API_KEY
model = "text-embedding-3-small"
dimensions = 1536
```

API keys are **never stored in config files** — they are read from environment variables or the system keychain.

Project-level config can override global config, enabling per-project provider choice.

### Critical Constraint: No Mixed Vector Spaces

Vectors from different models are completely incompatible. All vectors in a single `vec_nodes` table must come from the same model. Switching providers triggers a full migration (re-embed everything), not a mix. The `embedding_model` field on each node and the `schema_meta` table enforce this.

## Why a Pluggable Provider

Hardcoding any single provider creates unnecessary coupling:

- **Hardcoded Ollama** — penalizes users who don't want to install/run a local model server. On headless Linux, in CI, or on constrained hardware, Ollama may not be practical.
- **Hardcoded API** — contradicts the local-first, zero-cost philosophy. Creates a hard dependency on internet connectivity and a billing account.
- **Hardcoded "none"** — throws away the most powerful retrieval signal (semantic similarity).

The provider abstraction costs almost nothing architecturally — it's a thin layer that resolves to a single function call. The rest of the system (reconciliation, lifecycle, graph traversal, retrieval scoring) is entirely independent of how embeddings are generated.

## Recommended Default: nomic-embed-text-v1.5 via Ollama

| Property | Value |
|----------|-------|
| Dimensions | 768 |
| Context window | 8,192 tokens |
| Latency (M-series) | ~10ms per embedding |
| Model size | ~274 MB |
| MTEB Retrieval (NDCG@10) | ~53 |
| Matryoshka support | 768, 512, 256, 128 dims |
| License | Apache 2.0 |

**Why this default over alternatives:**

| Candidate | Why Not Default |
|-----------|----------------|
| all-MiniLM-L6-v2 (384-dim) | Inferior retrieval quality (~41 vs ~53). 256-token max context too short. |
| mxbai-embed-large (1024-dim) | 2.5x larger, 2x slower, marginal quality gain. 512-token max context. |
| bge-large-en-v1.5 (1024-dim) | No first-class Ollama support. Requires additional runtimes. |
| OpenAI text-embedding-3-small | Strong quality (~55) but requires API key + internet. Not in the Anthropic ecosystem. |
| Voyage AI voyage-3.5 | Best quality, API-only. **Recommended API provider** — Anthropic's official embedding partner. |

## Unavailability Handling

When the configured provider is unavailable (Ollama not running, API unreachable):

1. Store the memory node with all metadata and content
2. Set `status='pending_embedding'` — the node exists but has no vector
3. Graph-based recall (edge traversal, keyword matching) still works for unembedded nodes
4. On next successful provider connection, process the pending queue
5. No data is lost — embedding is deferred, not skipped

This applies equally to local and API providers.

## Provider Migration

When switching providers (e.g., Ollama → Voyage, or upgrading to a newer model):

1. Create new `vec_nodes_v2` virtual table with new dimensions
2. Re-embed all active nodes using the new provider (background, batched)
3. Switch query routing to new table
4. Drop old table
5. Update `schema_meta`

| Provider | Est. time (100K nodes) | Est. cost |
|----------|----------------------|-----------|
| Ollama (local) | ~15-20 min (M-series) | Free |
| Voyage AI | ~5-10 min (batched) | ~$0.12 |
| OpenAI | ~5-10 min (batched) | ~$0.04 |

The `embedding_model` field on each node tracks provenance. Nodes whose `embedding_model` doesn't match the active model are flagged for re-embedding.

## Consequences

### Positive
- **No vendor lock-in.** Users choose what works for their setup — local, API, or none.
- **Local-first by default.** The recommended path (Ollama) is free and offline, aligning with Cerebro's philosophy.
- **API path is first-class.** Users who prefer API providers get a fully supported experience, not a second-class workaround.
- **Graceful degradation.** If the provider is unavailable, memories are stored and queued. Nothing is lost.
- **768 dimensions** (with the recommended default) halves storage and search time compared to the original 1536-dim spec.
- **Migration path is clean.** Switching providers is a well-defined operation, not a hack.

### Negative
- **Configuration surface.** Users must choose a provider at init time (or accept the default). Adds a setup step.
- **Dimension mismatch risk.** If a user configures `dimensions = 768` but their model outputs 1024, vectors will be truncated or padded incorrectly. Mitigation: validate dimensions against the provider at init time.
- **Testing burden.** Multiple provider implementations to maintain and test.

### Risks
- **Provider proliferation.** Supporting too many providers creates maintenance burden. Mitigation: start with Ollama (local) + Voyage AI (API) as the two first-class providers. Voyage is Anthropic's official embedding partner, aligning with the Claude ecosystem Cerebro is built within. Add others based on demand — the interface is simple enough that community contributions are feasible.
- **Inconsistent quality across providers.** A brain initialized with Voyage (NDCG@10 ~61) will have better semantic recall than one using all-minilm (~41). This is expected and acceptable — the user made the choice. Document the quality tradeoffs clearly.
- **Ollama as recommended default may not suit all users.** Some users don't want to install Ollama, especially on servers. Mitigation: `cerebro init` should prompt for provider choice and make "none" (graph-only) a valid option with a clear explanation of what's lost.

## Schema Impact

```sql
-- Vector table dimensions are provider-dependent
CREATE VIRTUAL TABLE vec_nodes USING vec0(
    node_id TEXT,
    embedding float[768],           -- dimension from config; 768 for nomic, 1024 for voyage, etc.
    distance_metric = 'cosine'
);

-- Tracking on nodes table
embedding_model TEXT NOT NULL       -- e.g., 'nomic-embed-text-v1.5', 'voyage-3.5', 'voyage-code-3'

-- Global tracking
INSERT INTO schema_meta VALUES ('embedding_provider', 'ollama');
INSERT INTO schema_meta VALUES ('embedding_model', 'nomic-embed-text-v1.5');
INSERT INTO schema_meta VALUES ('embedding_dimensions', '768');
```

## Recommended API Provider: Voyage AI

Anthropic does not offer its own embedding model. Their [official documentation](https://platform.claude.com/docs/en/build-with-claude/embeddings) recommends Voyage AI as the endorsed embedding provider for the Claude ecosystem.

Current Voyage models:

| Model | Dims | Context | Use Case |
|-------|------|---------|----------|
| `voyage-3.5` | 1024 (default), 256-2048 | 32K | General-purpose, balanced quality/cost |
| `voyage-3.5-lite` | 1024 (default), 256-2048 | 32K | Latency and cost optimized |
| `voyage-3-large` | 1024 (default), 256-2048 | 32K | Best retrieval quality |
| `voyage-code-3` | 1024 (default), 256-2048 | 32K | Code retrieval — relevant for coding orchestrators |

All Voyage models support Matryoshka-style dimension reduction (1024 → 512 → 256) and quantization (float32, int8, binary), which aligns well with Cerebro's scaling strategy.

For Cerebro users already in the Claude/Anthropic ecosystem, Voyage is the natural API choice — same ecosystem, officially endorsed, and `voyage-code-3` is particularly compelling for an agent brain that primarily assists with software engineering tasks.

## References
- [Research: Embedding strategies](../research/embedding-strategy-research.md)
- [Anthropic embeddings guide (recommends Voyage)](https://platform.claude.com/docs/en/build-with-claude/embeddings)
- [nomic-embed-text on Ollama](https://ollama.com/library/nomic-embed-text)
- [Voyage AI documentation](https://docs.voyageai.com/docs/embeddings)
- [MTEB Leaderboard](https://huggingface.co/spaces/mteb/leaderboard)
