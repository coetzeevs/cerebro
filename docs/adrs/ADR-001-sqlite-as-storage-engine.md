# ADR-001: SQLite + sqlite-vec as Storage Engine

## Status
Proposed

## Context

Cerebro needs a storage layer that combines:
1. **Graph storage** — nodes and typed edges for structured knowledge relationships
2. **Vector storage** — semantic embeddings for similarity-based retrieval
3. **Zero infrastructure** — no servers, no Docker, no background processes (beyond Ollama)
4. **Single-file portability** — the entire brain should be one file that can be copied, backed up, or inspected

The original architecture musing proposed SQLite with sqlite-vec. This ADR formalizes that decision after evaluating alternatives.

## Decision

Use **SQLite** as the storage engine with the **sqlite-vec** extension for vector search.

The graph layer (nodes + edges) uses standard SQLite tables with foreign keys and indexes. The vector layer uses sqlite-vec's `vec0` virtual table with cosine distance and 768-dimensional float32 embeddings.

## Alternatives Considered

### 1. sqlite-vss (SQLite + Faiss wrapper)
- **Rejected because:** Archived/unmaintained since mid-2024. Heavy C++ dependencies (Faiss, OpenBLAS, LAPACK). 30MB+ binary vs sqlite-vec's ~300KB. Limited platform support.

### 2. usearch (standalone HNSW library)
- **Rejected because:** No SQLite virtual table integration. Would require a separate index file alongside the SQLite database, breaking the single-file guarantee. Synchronizing two stores loses ACID guarantees. Added complexity for marginal benefit at Cerebro's expected scale (<100K nodes).
- **Reserved as escape hatch** if brute-force performance becomes insufficient at scale.

### 3. LanceDB (embedded vector database)
- **Rejected because:** Not SQLite — would lose the graph layer (nodes/edges with SQL JOINs) or require maintaining two separate storage systems. No SQL interface. Multi-file storage (directory of Lance fragments). Architecturally incompatible with the single-file design.

### 4. DuckDB with vector extensions
- **Rejected because:** OLAP-oriented engine. Agent memory is fundamentally OLTP (frequent small reads/writes, not analytical bulk scans). Single inserts are much slower than SQLite. VSS extension is experimental. Larger binary footprint.

### 5. Neo4j / dedicated graph database + Pinecone / dedicated vector database
- **Rejected because:** Server-based infrastructure. Two systems to maintain. Contradicts the zero-infrastructure principle. Overkill for single-user agent memory.

## Consequences

### Positive
- **Single-file architecture** preserved. brain.sqlite contains everything.
- **Zero dependencies** beyond SQLite itself (which is ubiquitous).
- **SQL-native hybrid queries.** Graph traversal and vector search combine in a single SQL statement:
  ```sql
  WITH semantic_matches AS (
      SELECT node_id, distance FROM vec_nodes
      WHERE embedding MATCH ? AND k = 5
  ),
  expanded AS (
      SELECT DISTINCT e.target_id AS id
      FROM semantic_matches sm JOIN edges e ON e.source_id = sm.node_id
      UNION SELECT node_id AS id FROM semantic_matches
  )
  SELECT n.* FROM nodes n JOIN expanded ex ON n.id = ex.id;
  ```
- **ACID compliance** for the graph layer. Safe concurrent reads under WAL mode.
- **Excellent cross-platform support.** Pure C extension, compiles everywhere SQLite does. Prebuilt binaries for macOS (ARM + x86), Linux, Windows.
- **Mature ecosystem.** SQLite is the most deployed database in the world. Tooling is abundant.

### Negative
- **Brute-force only vector search.** sqlite-vec does not (yet) support ANN indexes. Performance degrades linearly with vector count. Interactive latency limit is ~100K vectors at 768 dimensions (~40ms on M-series).
- **Single writer.** SQLite WAL mode allows concurrent readers but only one writer. Not a problem for orchestrator-only access, but constrains future multi-agent scenarios.
- **Pre-1.0 extension.** sqlite-vec is actively developed but has not reached 1.0. API changes are possible (though the core `vec0` interface has been stable).

### Risks
- **Scale ceiling.** If a project brain exceeds ~100K active nodes, brute-force search will become slow. Mitigation: (1) int8 quantization extends the limit to ~200-400K, (2) Matryoshka dimension reduction to 256-dim extends further, (3) sqlite-vec ANN support is on the roadmap, (4) usearch can be added as a secondary index if needed.
- **Extension availability.** sqlite-vec must be loadable at runtime. Mitigation: bundle the prebuilt extension with Cerebro's distribution (it's ~300KB). Fall back to pip/npm install if needed.

## References
- [sqlite-vec GitHub](https://github.com/asg017/sqlite-vec)
- [Research: sqlite-vec capabilities](../research/sqlite-vec-research.md)
