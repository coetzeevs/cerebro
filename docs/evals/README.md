# Cerebro Recall-Quality Eval Harness

This directory contains the evaluation corpus for cerebro's recall-quality harness
(`cerebro eval`), introduced by ticket `agentic-x183`.

## Purpose

The harness measures recall@5, recall@10, recall@20, and MRR of the composite scorer
against a hand-authored ground-truth set derived from the operator's real dogfooded brain.
Every future scoring change (BM25/FTS5, cross-encoder rerank, alpha-decay traversal,
neighbour cache) should be run through this harness before and after to prove improvement.

## Files

| File | Description |
|------|-------------|
| `queries.jsonl` | 10 hand-authored evaluation queries (one JSON object per line) |
| `ground-truth.jsonl` | Per-query relevant node IDs (opaque UUIDs; no raw content) |
| `corpus.md` | Provenance manifest: which brain, schema version, node count, assembly date |
| `baseline.json` | Committed baseline metrics from the initial harness run |
| `.gitignore` | Prevents raw SQLite brain blobs from being accidentally committed |

## Metrics

| Metric | Definition |
|--------|-----------|
| **recall@K** | `|groundTruthIDs ∩ top-K returned IDs| / |groundTruthIDs|` per query, macro-averaged |
| **MRR** | `mean(1 / rank of first relevant hit)` across queries; 0 if no hit |

All metric values are in `[0.0, 1.0]`. The harness reports recall@5, recall@10, and recall@20
from a single `Brain.Search(limit=20)` call per query (prefix slice).

## Scorer formula being measured

```
compositeScore = 0.35 · relevance
              + 0.25 · importance
              + 0.25 · recency
              + 0.15 · structural
```

Source: `cerebro/internal/store/search.go:compositeScore`. These weights are recorded as
`scorer_weights` in `baseline.json` and are the **baseline being measured** — not the
target of any change in `agentic-x183`. See `docs/adrs/ADR-011-recall-quality-eval-harness.md`
for the architectural rationale.

## Running the harness

### Prerequisites

- Ollama must be running locally: `ollama serve`
- The embedding model must be available: `ollama pull nomic-embed-text`
- Point at the brain under evaluation with `-p <project-dir>`

### Basic run (uses defaults from `docs/evals/`)

```bash
cerebro eval -p /path/to/your/project
```

### With explicit paths

```bash
cerebro eval \
  --queries docs/evals/queries.jsonl \
  --ground-truth docs/evals/ground-truth.jsonl \
  --corpus docs/evals/corpus.md \
  --out docs/evals/baseline.json \
  --limit 20 \
  --threshold 0.3 \
  -p /path/to/your/project
```

### Output

Metrics are printed to stdout:

```
recall@5:  0.XXXX
recall@10: 0.XXXX
recall@20: 0.XXXX
MRR:       0.XXXX
queries evaluated: 10
```

A `baseline.json` artifact is written to `--out` (default: `docs/evals/baseline.json`).
Progress messages and preflight warnings are routed to stderr.

### Without Ollama

AC1 (`--help`) and the unit tests (pure metric helpers, preflight, baseline shape) work
without Ollama. AC3/AC4 (live recall metrics) require a running Ollama instance. If Ollama
is unavailable, `cerebro eval` exits non-zero with a clear error — it does **not** fabricate
zeros. See R1 in `ADR-011`.

## Ground-truth construction

See `corpus.md` for the full provenance. In brief:

1. Issue `cerebro recall <query>` against the live brain for each query.
2. Manually select node IDs that are *primarily* about the query topic.
3. Commit only opaque UUIDs — no raw memory content.

## Corpus sensitivity and public-repo caution

> **This repository is PUBLIC.** The following content-hygiene rules MUST be followed
> when authoring or expanding the eval corpus:
>
> - `queries.jsonl`: queries must be abstract/sanitised. Do NOT embed raw memory content,
>   secrets, personal names, tokens, or absolute operator paths in query strings or note fields.
> - `ground-truth.jsonl`: record opaque UUIDs only. No inlined content.
> - `corpus.md`: record brain *path class* only — NEVER the raw
>   `~/.cerebro/projects/<sha256>.sqlite` path.
> - Do NOT enrich `corpus.md` with example memory snippets without first applying the
>   (currently out-of-scope) PII-scrubbing pipeline.
>
> These rules are enforced by the `agentic-x183` security review (S-LOW-1, S-LOW-2).
> Re-grading to MEDIUM/HIGH occurs if third-party PII or credentials ever enter the corpus.

## Interpreting the baseline

With 537 active nodes at the time of initial assembly, recall@20 can approach 1.0 for
queries with small ground-truth sets (1–2 nodes). This is a property of the corpus size,
not a harness defect. As the brain grows, these scores will naturally lower and provide
better discrimination. Always check `baseline.json:brain.active_nodes` to interpret
results in context.

## ADR

See `docs/adrs/ADR-011-recall-quality-eval-harness.md` for the architectural decisions
behind this harness: why `cerebro eval` over a standalone binary or `go test`-gated harness,
why committed node-IDs + live-validation over a frozen DB snapshot, and why macro-averaged
recall@K + MRR as the v1 metric set.
