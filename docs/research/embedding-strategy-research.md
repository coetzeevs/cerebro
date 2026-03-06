# Embedding Generation Strategy Research

> **Date:** 2026-03-04
> **Context:** Research for Cerebro, a local-first, zero-infrastructure agent brain using SQLite (with sqlite-vec) for persistent agent memory.

---

## 1. Local Embedding Models (2024-2025 Landscape)

### 1.1 all-MiniLM-L6-v2 (Sentence Transformers)

| Property | Value |
|---|---|
| **Dimensions** | 384 |
| **Parameters** | ~22M |
| **Model size** | ~80 MB |
| **MTEB Average** | ~49.5 |
| **Retrieval (NDCG@10)** | ~41-43 |
| **Speed (CPU)** | ~2-5ms per sentence on M-series Mac |
| **Max sequence length** | 256 tokens |

**Assessment:** The long-reigning "default" small embedding model. Extremely fast, tiny footprint. Quality is acceptable for simple similarity tasks but noticeably weaker on retrieval benchmarks. The 256-token limit is a real constraint for paragraph-level memory chunks. Surpassed by newer models at similar sizes.

### 1.2 nomic-embed-text-v1.5

| Property | Value |
|---|---|
| **Dimensions** | 768 (supports Matryoshka: 768, 512, 256, 128) |
| **Parameters** | ~137M |
| **Model size** | ~274 MB (fp16), Ollama quantized ~140 MB |
| **MTEB Average** | ~62.3 |
| **Retrieval (NDCG@10)** | ~52-53 |
| **Max sequence length** | 8192 tokens |
| **Ollama support** | First-class: `ollama pull nomic-embed-text` |

**Assessment:** Standout model for local-first use. Matryoshka embedding support is a killer feature -- generate 768-dim embeddings but store and search at 256 or 128 dims for speed, then use full 768 dims when precision matters. 8192-token context window is excellent for longer memory chunks. Open-source (Apache 2.0). Ollama integration is seamless. **Strongest candidate for Cerebro's primary local model.**

### 1.3 gte-small / gte-base (Alibaba DAMO)

| Model | Dimensions | Parameters | MTEB Avg | Retrieval |
|---|---|---|---|---|
| gte-small | 384 | ~33M | ~52.5 | ~46 |
| gte-base | 768 | ~110M | ~55.2 | ~49 |
| gte-large | 1024 | ~335M | ~57.0 | ~51 |

**Assessment:** gte-small outperforms MiniLM but has been surpassed by nomic-embed-text. No native Ollama support. Max sequence length is 512 tokens -- a significant limitation.

### 1.4 BAAI/bge Family

| Model | Dimensions | Parameters | MTEB Avg | Retrieval |
|---|---|---|---|---|
| bge-small-en-v1.5 | 384 | ~33M | ~53.0 | ~46.5 |
| bge-base-en-v1.5 | 768 | ~110M | ~56.3 | ~50.5 |
| bge-large-en-v1.5 | 1024 | ~335M | ~58.5 | ~53.0 |
| bge-m3 | 1024 | ~568M | ~60.0 | ~54.5 |

**Assessment:** Popular open-source series. bge-m3 supports dense, sparse, and multi-vector retrieval -- overkill for Cerebro. No first-class Ollama support for most variants.

### 1.5 mxbai-embed-large (mixedbread.ai)

| Property | Value |
|---|---|
| **Dimensions** | 1024 (supports Matryoshka: 1024, 512, 256) |
| **Parameters** | ~335M |
| **Model size** | ~670 MB (fp16), Ollama q4: ~335 MB |
| **MTEB Average** | ~60.0 |
| **Retrieval (NDCG@10)** | ~54.0 |
| **Max sequence length** | 512 tokens |
| **Ollama support** | Native: `ollama pull mxbai-embed-large` |

**Assessment:** Strong quality but 335M params -- larger than nomic-embed-text (137M) with marginal quality improvement. 512-token max is limiting.

### 1.6 Local vs API Quality Gap

The gap has narrowed dramatically:

- **Top local models** (nomic, bge-large, mxbai): ~52-55 retrieval NDCG@10
- **OpenAI text-embedding-3-small**: ~54-55
- **OpenAI text-embedding-3-large**: ~58-60
- **Voyage AI voyage-3**: ~59-61

Gap between a good local model and text-embedding-3-small is **negligible (1-3 points)**. Gap to the best API models is **5-8 points** -- noticeable but not dramatic for a memory system with graph-based retrieval as complement.

**Bottom line:** For a local-first system, local models are now good enough.

---

## 2. API-Based Embedding Services

### 2.1 OpenAI

| Model | Dimensions | Cost (per 1M tokens) | Max tokens | Quality |
|---|---|---|---|---|
| text-embedding-3-small | 1536 (supports 512, 256) | $0.02 | 8191 | ~54-55 |
| text-embedding-3-large | 3072 (supports 1024, 512, 256) | $0.13 | 8191 | ~58-60 |

### 2.2 Voyage AI

| Model | Dimensions | Cost (per 1M tokens) | Max tokens | Quality |
|---|---|---|---|---|
| voyage-3 | 1024 | $0.06 | 32000 | ~59-61 |
| voyage-3-lite | 512 | $0.02 | 32000 | ~55-56 |
| voyage-code-3 | 1024 | $0.06 | 32000 | ~58 (code) |

Anthropic has officially recommended Voyage embeddings.

### 2.3 Cohere

| Model | Dimensions | Cost (per 1M tokens) | Max tokens | Quality |
|---|---|---|---|---|
| embed-english-v3.0 | 1024 | $0.10 | 512 | ~55-57 |
| embed-english-light-v3.0 | 384 | $0.10 | 512 | ~50-52 |

### 2.4 Latency

| Service | Single text | Batch (100 texts) |
|---|---|---|
| OpenAI | 30-80ms | 200-500ms |
| Voyage AI | 40-100ms | 300-600ms |
| Cohere | 50-120ms | 400-800ms |

---

## 3. The Model Versioning Problem

### 3.1 The Core Issue

Embedding vectors from different models are **completely incompatible**. You cannot mix vectors from different models in the same search index. Even minor model updates can shift the vector space enough to degrade quality.

### 3.2 Migration Strategies

#### Strategy A: Re-embed Everything

| Aspect | Assessment |
|---|---|
| Complexity | Low |
| Downtime | Yes -- during migration |
| Storage | 1x |
| Requirement | Must retain original text |

**For Cerebro:** Viable. Re-embedding 100K nodes takes 3-15 minutes on M-series Mac.

#### Strategy B: Dual-Index (Blue-Green)

Create new vector table, run both, migrate in background, switch, drop old.

```sql
CREATE VIRTUAL TABLE vec_nodes_v1 USING vec0(node_id TEXT, embedding float[384]);
CREATE VIRTUAL TABLE vec_nodes_v2 USING vec0(node_id TEXT, embedding float[768]);
```

| Aspect | Assessment |
|---|---|
| Complexity | Medium |
| Downtime | None |
| Storage | 2x during migration |

**Recommended for seamless transitions.**

### 3.3 Universal Lesson

All production systems assume you will retain source text. **Never store only the embedding vector without the original text.**

### 3.4 Recommendation for Cerebro

1. **Always store original text** in the nodes table metadata.
2. **Use Blue-Green (Dual-Index)** for model transitions.
3. **Record model identifier** on each node.
4. **Design schema to expect changes** from day one:

```sql
-- Track on each node
embedding_model TEXT DEFAULT 'nomic-embed-text-v1.5'
```

---

## 4. Dimension Trade-offs

### 4.1 Quality Impact

| Dimensions | Relative Quality | Notes |
|---|---|---|
| 128 | ~80-85% of max | Acceptable for coarse filtering |
| 256 | ~90-92% of max | Good for most use cases |
| 384 | ~93-95% of max | Sweet spot for small models |
| 768 | ~97-99% of max | Strong default |
| 1024 | ~99% of max | Near-optimal |
| 1536 | ~99.5% of max | Rarely justified for local |

### 4.2 Storage Impact

| Dimensions | Per vector | 10K vectors | 100K vectors |
|---|---|---|---|
| 384 | 1.5 KB | 15 MB | 150 MB |
| 768 | 3 KB | 30 MB | 300 MB |
| 1024 | 4 KB | 40 MB | 400 MB |
| 1536 | 6 KB | 60 MB | 600 MB |

### 4.3 Search Speed (Brute-Force, M-series Mac)

| Vectors | 384 dims | 768 dims | 1536 dims |
|---|---|---|---|
| 10K | ~2ms | ~4ms | ~8ms |
| 100K | ~20ms | ~40ms | ~80ms |
| 1M | ~200ms | ~400ms | ~800ms |

### 4.4 Recommendation

**768 dimensions** is the recommended default:
- Native output of nomic-embed-text
- 97-99% of maximum retrieval quality
- 300 MB for 100K vectors -- acceptable
- ~40ms for 100K vectors brute-force -- excellent
- Matryoshka models can truncate to 256 dims for faster coarse search later

---

## 5. Practical Local-First Recommendations

### 5.1 Ollama Embedding Models

| Model | Command | Dimensions | Download |
|---|---|---|---|
| nomic-embed-text | `ollama pull nomic-embed-text` | 768 | ~274 MB |
| mxbai-embed-large | `ollama pull mxbai-embed-large` | 1024 | ~670 MB |
| snowflake-arctic-embed | `ollama pull snowflake-arctic-embed` | 1024 | ~670 MB |
| all-minilm | `ollama pull all-minilm` | 384 | ~45 MB |

### 5.2 Latency on M-Series Mac (Single Paragraph ~100 tokens)

| Model | M1 | M2 | M3 Pro |
|---|---|---|---|
| all-minilm | ~5ms | ~4ms | ~3ms |
| nomic-embed-text | ~12ms | ~10ms | ~8ms |
| mxbai-embed-large | ~25ms | ~20ms | ~15ms |
| Cold start (any) | ~1-3s | ~1-2s | ~0.8-1.5s |

### 5.3 Fallback Strategy

```
Embedding Request
       |
       v
  Is Ollama running?
  /              \
YES               NO
 |                 |
 v                 v
Use nomic-      Queue for later
embed-text      (store text, embed
                when Ollama available)
```

**Do NOT mix models.** Rather than falling back to an API with different vectors, use a queue-and-retry pattern. Only use API fallback if user explicitly enables it and accepts a SEPARATE vector table.

---

## 6. Two-Tier Embedding: Probably Not Worth It

nomic-embed-text at ~10ms per embedding is already fast enough for synchronous use. The complexity of maintaining two indexes is not justified:

- 10ms latency is imperceptible in agent workflows where LLM calls take 500ms-5s
- Quality gap between all-minilm (384d) and nomic (768d) is ~10-15 points on retrieval
- Only consider two-tier if bulk-importing thousands of memories

**Recommendation:** Use nomic-embed-text for everything, synchronously.

---

## 7. Final Recommendations

### 7.1 Primary Model: nomic-embed-text-v1.5 via Ollama

- 768 dimensions: optimal quality-to-cost ratio
- 8192-token context: embed entire memory chunks without truncation
- ~10ms per embedding: fast enough for synchronous use
- Matryoshka support: reduce to 256 dims later if needed
- First-class Ollama support
- Apache 2.0 license

### 7.2 Schema Change from Original Architecture

Update from `float[1536]` to `float[768]`:

```sql
CREATE VIRTUAL TABLE vec_nodes USING vec0(
    node_id TEXT,
    embedding float[768],
    distance_metric='cosine'
);
```

### 7.3 What NOT to Do

- **Do not use 1536 dimensions** unless using OpenAI's API.
- **Do not mix vectors from different models** in the same virtual table.
- **Do not store only vectors without source text.**
- **Do not implement two-tier embedding** unless profiling shows it's needed.
- **Do not use all-MiniLM-L6-v2** for new projects.

### Quick Reference

| Model | Dims | Size | Speed | Retrieval | Ollama | License |
|---|---|---|---|---|---|---|
| all-minilm | 384 | 45 MB | ~3ms | ~41 | Yes | Apache 2.0 |
| **nomic-embed-text** | **768** | **274 MB** | **~10ms** | **~53** | **Yes** | **Apache 2.0** |
| mxbai-embed-large | 1024 | 670 MB | ~20ms | ~54 | Yes | Apache 2.0 |
| OpenAI 3-small | 1536 | API | ~50ms | ~55 | N/A | Proprietary |
| Voyage 3 | 1024 | API | ~60ms | ~61 | N/A | Proprietary |
