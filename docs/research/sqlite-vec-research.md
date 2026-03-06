# sqlite-vec Research Findings for Cerebro

> **Date:** 2026-03-04
> **Context:** Research for Cerebro, a local-first agent memory system combining graph + vector search in a single SQLite file.

---

## 1. sqlite-vec Current State

### 1.1 Overview

sqlite-vec is a SQLite extension created by Alex Garcia (`asg017/sqlite-vec` on GitHub) that provides vector search capabilities directly inside SQLite. It is the successor to sqlite-vss and was designed from the ground up to be simpler, more portable, and free of heavy C++ dependencies.

- **Author:** Alex Garcia
- **License:** MIT / Apache 2.0
- **Language:** Pure C (no C++ dependencies, unlike sqlite-vss)

### 1.2 Index Types

**sqlite-vec supports only brute-force (exact) k-NN search.** There is no built-in ANN index (no HNSW, no IVF, no product quantization).

However, Alex Garcia has publicly discussed plans for ANN support:

- **Planned ANN approach:** The roadmap included a "partitioned" approach (IVF-like) rather than HNSW, because IVF-style indexes map more naturally to SQLite's page-based storage model.
- **DiskANN / Vamana:** There has been discussion of DiskANN-style indexing, which is designed for disk-resident vectors (a good fit for SQLite's storage model).

**What this means for Cerebro:** At scales below ~50k vectors, brute-force KNN is perfectly adequate for most use cases (sub-100ms queries). Beyond that, lack of ANN becomes a bottleneck.

### 1.3 Performance Characteristics

Approximate numbers based on community benchmarks. All times assume a single query vector against the full dataset, using 1536-dimensional float32 embeddings on modern hardware (M-series Mac or recent x86):

| Scale | Brute-Force Query Time | Notes |
|-------|----------------------|-------|
| 1,000 vectors | ~1-5ms | Essentially instant |
| 10,000 vectors | ~5-20ms | Still very fast |
| 100,000 vectors | ~50-200ms | Usable but noticeable latency |
| 1,000,000 vectors | ~500ms-2s+ | Impractical for interactive use without ANN |

**Key performance notes:**
- sqlite-vec uses SIMD-optimized distance calculations where available (SSE, AVX on x86; NEON on ARM/Apple Silicon).
- Performance scales linearly with dataset size for brute-force.
- Quantized vectors (int8) provide ~4x memory savings and ~2-3x speed improvement over float32 at the cost of some recall accuracy.
- Smaller dimensions (e.g., 384 from MiniLM) are proportionally faster than 1536.

### 1.4 Supported Embedding Dimensions

- **float32 vectors:** Arbitrary dimensions via `float[N]`.
- **int8 vectors:** Supported for quantized storage via `int8[N]`.
- **Binary/bit vectors:** Supported for hamming-distance search via `bit[N]`.
- **No hard upper limit on dimensions**, but practical limits governed by SQLite's row size limits and performance. Dimensions tested in practice: 384, 512, 768, 1024, 1536, 3072.

### 1.5 Production Readiness

- **Stability:** The core brute-force search functionality is stable and has been used in production by multiple projects.
- **Community adoption:** Growing adoption in the local-first / edge AI community. Used by Obsidian plugins, local RAG systems, and various AI agent projects.
- **Risk assessment for Cerebro:** Low risk for the core use case. The brute-force `vec0` API is unlikely to break.

### 1.6 API Surface

#### Virtual Tables

```sql
CREATE VIRTUAL TABLE vec_items USING vec0(
    item_id TEXT,           -- optional: link to external table
    embedding float[1536],  -- vector column with dimension
    +metadata TEXT           -- auxiliary columns (prefixed with +)
);
```

**Query syntax:**

```sql
-- KNN search
SELECT item_id, distance
FROM vec_items
WHERE embedding MATCH ?  -- serialized vector (JSON array or binary blob)
  AND k = 10;            -- return top-10 results

-- With pre-filtering
SELECT item_id, distance
FROM vec_items
WHERE embedding MATCH ?
  AND k = 10
  AND item_id IN (SELECT id FROM items WHERE category = 'bugs');
```

#### Scalar Functions

| Function | Purpose |
|----------|---------|
| `vec_length(v)` | Number of elements in a vector |
| `vec_distance_L2(a, b)` | Euclidean distance |
| `vec_distance_cosine(a, b)` | Cosine distance |
| `vec_distance_L1(a, b)` | Manhattan distance |
| `vec_normalize(v)` | L2-normalized vector |
| `vec_quantize_int8(v)` | Quantize float32 to int8 |
| `vec_quantize_binary(v)` | Quantize to binary vector |
| `vec_to_json(v)` | Convert binary vector to JSON array |

#### Distance Metrics

- **L2 (Euclidean)** -- default
- **Cosine distance** -- via `distance_metric='cosine'`
- **L1 (Manhattan)**
- **Hamming distance** -- for bit vectors

---

## 2. Alternatives Evaluation

### 2.1 sqlite-vss

**Status: Deprecated / Superseded by sqlite-vec.**

| Aspect | sqlite-vss | sqlite-vec |
|--------|-----------|------------|
| **Backend** | Wraps Facebook's Faiss library (C++) | Pure C implementation |
| **Index types** | IVF-PQ via Faiss | Brute-force only (ANN planned) |
| **Dependencies** | Heavy (Faiss, OpenBLAS, LAPACK) | Zero external dependencies |
| **Binary size** | ~20-50MB+ | ~200-500KB |
| **Maintenance** | Archived/unmaintained since mid-2024 | Actively developed |

**Verdict:** Do not use. Abandoned in favor of sqlite-vec.

### 2.2 usearch with SQLite Bindings

**USearch** (by Unum) is a high-performance ANN library supporting HNSW indexing.

- Very fast HNSW implementation, cross-platform, ~1MB binary.
- **No official SQLite virtual table extension.** Would require custom wrapper.
- At 100k vectors: usearch HNSW ~1-5ms vs sqlite-vec brute-force ~50-200ms.
- **Two-file problem:** Breaks the single-file guarantee.
- **Sync complexity:** No ACID guarantees across two stores.

**Verdict:** Best escape hatch if sqlite-vec brute-force becomes insufficient, but sacrifices single-file simplicity.

### 2.3 LanceDB

Embedded vector database built on the Lance columnar format.

- Built-in ANN indexing (IVF-PQ, DiskANN).
- **Not SQLite:** Loses graph layer or requires two storage systems.
- No SQL interface. No JOINs between graph edges and vector results.
- Multi-file storage (directory of Lance fragments).

**Verdict:** Architecturally incompatible with Cerebro's single-file design. Only consider if abandoning SQLite entirely.

### 2.4 DuckDB with Vector Extensions

- OLAP-oriented (not OLTP). Agent memory has OLTP access patterns.
- Single inserts are much slower than SQLite.
- VSS extension is experimental.

**Verdict:** Wrong tool for this job. OLAP orientation is a fundamental mismatch.

---

## 3. Concurrency Characteristics

### 3.1 sqlite-vec Under WAL Mode

Fully compatible with SQLite WAL mode:

- **Multiple concurrent readers:** Fully supported. No blocking.
- **Single writer:** Only one process can write at a time. Write transactions do not block readers.
- **Reader isolation:** Each reader sees a consistent snapshot.

**Recommended configuration:**
```sql
PRAGMA journal_mode=WAL;
PRAGMA busy_timeout=5000;
PRAGMA cache_size=-65536;  -- 64MB page cache
```

### 3.2 Single Writer Implications for Cerebro

- If the orchestrator is the sole writer (as designed), this is a non-issue.
- Vector inserts are heavier than regular row inserts — write transactions hold the lock longer.
- **NFS / network filesystems:** Never use SQLite on network filesystems.

---

## 4. Practical Limitations

### 4.1 Storage Size Estimates

| Scale | Vector Data (float32, 1536-dim) | Total DB Estimate |
|-------|--------------------------------|-------------------|
| 10k vectors | ~60 MB | ~100-200 MB |
| 100k vectors | ~600 MB | ~1-2 GB |
| 1M vectors | ~5.7 GB | ~8-15 GB |

### 4.2 Memory Consumption

- sqlite-vec does NOT load the entire index into memory. Reads vectors via SQLite's pager.
- Memory usage during query is proportional to vectors being scanned, not total dataset.
- Increase page cache for vector workloads: `PRAGMA cache_size=-65536` (64MB).

### 4.3 Platform Support

Excellent cross-platform support due to pure C implementation:

| Platform | Status |
|----------|--------|
| macOS (ARM/Apple Silicon) | Full support |
| macOS (x86_64) | Full support |
| Linux (x86_64) | Full support |
| Linux (ARM64) | Full support |
| Windows (x86_64) | Full support |
| WebAssembly | Supported |

Language bindings: Python, Node.js, Ruby, Rust, Go, Deno.

---

## 5. Recommendations for Cerebro

### 5.1 sqlite-vec is the Right Choice

1. **Single-file architecture** is preserved.
2. **Zero dependencies** beyond SQLite itself.
3. **Cross-platform** without compilation headaches.
4. **SQL-native** — graph queries and vector queries combine in a single SQL statement.
5. **Concurrency model** is sufficient for orchestrator-only writes.

### 5.2 Scaling Strategy

| Phase | Vector Count | Strategy |
|-------|-------------|----------|
| Phase 1 (now) | 0 - 50k | Brute-force float32 with sqlite-vec |
| Phase 2 (optimize) | 50k - 200k | int8 quantization, increase page cache, smaller embeddings (384-dim) |
| Phase 3 (ANN) | 200k - 1M+ | Use sqlite-vec ANN when available, or two-tier hot/cold approach |
| Phase 4 (escape hatch) | 1M+ | Add usearch as secondary index alongside SQLite |

### 5.3 The Killer Feature: Hybrid Queries

The power of keeping everything in SQLite — semantic search AND graph expansion in one query:

```sql
WITH semantic_matches AS (
    SELECT node_id, distance
    FROM vec_nodes
    WHERE embedding MATCH ?
      AND k = 5
),
expanded AS (
    SELECT DISTINCT e.target_id AS id
    FROM semantic_matches sm
    JOIN edges e ON e.source_id = sm.node_id
    UNION
    SELECT node_id AS id FROM semantic_matches
)
SELECT n.id, n.type, n.metadata, n.created_at
FROM nodes n
JOIN expanded ex ON n.id = ex.id;
```

---

## 6. Summary Comparison

| Feature | sqlite-vec | sqlite-vss | usearch | LanceDB | DuckDB+vss |
|---------|-----------|------------|---------|---------|------------|
| **Single SQLite file** | Yes | Yes | No | No | Yes |
| **ANN indexing** | No (planned) | Yes (IVF-PQ) | Yes (HNSW) | Yes | Yes (HNSW) |
| **Dependencies** | None | Faiss/OpenBLAS | Minimal | Rust runtime | DuckDB core |
| **Binary size** | ~300KB | ~30MB+ | ~1MB | ~50MB+ | ~30MB+ |
| **Maintained** | Active | Archived | Active | Active | Experimental |
| **OLTP-friendly** | Yes | Yes | N/A | Moderate | No |
| **SQL JOINs with graph** | Native | Native | Manual | Not possible | Native |
| **Platform support** | Excellent | Limited | Good | Good | Good |
| **Best scale (interactive)** | <100k | <500k | Millions | Millions | Millions |
