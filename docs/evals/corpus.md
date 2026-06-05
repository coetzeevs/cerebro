# Corpus Provenance Manifest

## Brain under evaluation

| Field | Value |
|-------|-------|
| **Path class** | Project-scoped brain for the `agentic` monorepo (at `~/.cerebro/projects/<sha256(realpath)>.sqlite`) |
| **Schema version** | 2 |
| **Active nodes at assembly** | 537 |
| **Embedding model** | `nomic-embed-text` |
| **Embedding dimensions** | 768 |
| **Assembly date** | 2026-06-02 |

> **S-LOW-2 guardrail:** This manifest records the brain *path class* only — never the raw
> `~/.cerebro/projects/<sha256>.sqlite` resolved path. The SHA-256 hash encodes the operator's
> realpath and must not be disclosed in committed artefacts.

## Ground-truth construction method

Ground-truth was assembled by:

1. Issuing `cerebro recall <query>` for each query in `queries.jsonl` against the live brain.
2. Manually reviewing the top-returned nodes and selecting the node IDs that are clearly
   relevant to the query's *topic* (not just syntactically co-present).
3. Recording only the opaque RFC-4122 UUID node IDs in `ground-truth.jsonl` — no raw
   memory content is committed to this repository.

Selection criteria:
- The node's *type* and *content* are primarily about the query topic (not incidental mentions).
- For procedural/episodic queries, the most direct episode or procedure node is preferred.
- Ground-truth sets are kept small (1–3 nodes per query) to produce meaningful recall@K
  discrimination at K=5/10/20 with a 537-node corpus.

## Node-count note

With 537 active nodes, recall@20 may approach 1.0 for queries where the ground-truth set
is small (1–2 nodes) and those nodes are highly relevant. This is a property of the
dogfooded corpus size, not a harness defect. Future sessions that expand the corpus will
lower this ceiling. The baseline.json records the node count at run time so readers can
interpret results in context.

## Stability

Ground-truth node IDs are validated at harness run time via AC2b preflight
(`cerebro eval` preflight step). If GC prunes a node between assembly and a run,
the harness emits a stderr warning and skips the affected query's missing IDs from the
recall denominator. The committed IDs were all `status='active'` at assembly time.

## Sensitivity classification

This corpus contains:
- **Queries**: Hand-authored, abstract engineering queries — no raw memory content.
- **Ground-truth**: Opaque UUIDs only — no content, no absolute paths.
- **Manifest**: Brain metadata (schema version, count, embedding model) and path *class*.

No third-party PII, credentials, or internal secret material is present.
See `docs/evals/README.md` for the public-repo caution and content hygiene rules.
