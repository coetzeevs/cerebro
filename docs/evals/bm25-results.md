# BM25/FTS5 Keyword Recall — Tokenizer Fix & Before/After Eval (agentic-2lak)

This is the AC3 artefact: it records the MATCH-builder tokenizer fix, the
composite-weight decision, and the same-session before/after `cerebro eval`
numbers for both rerank paths — on the **full** extended corpus AND on the
**exact-identifier subset** the keyword lane is designed to help. Measured against
the dogfooded brain (557 active nodes, `nomic-embed-text` 768-dim) over the
extended corpus (`docs/evals/queries.jsonl` = **18 queries** = 10 original
multi-word semantic queries + 8 new multi-word exact-identifier queries `q11`–`q18`;
`docs/evals/ground-truth.jsonl`).

## The defect this revision fixes (the honest correction)

The first agentic-2lak implementation shipped a `buildMatchQuery` that wrapped the
**whole** user query in a single FTS5 phrase (`"OO-015 determinism wire"`). An
FTS5 phrase requires all its terms to be **adjacent** in the indexed text, so any
multi-word query matched **nothing**:

```
nodes_fts MATCH '"OO-015 determinism wire enforcement"'   -> 0 rows   (live, 556-row index)
nodes_fts MATCH '"OO-015" OR "determinism" OR "wire" ...'  -> 303 rows (the fix)
nodes_fts MATCH '"HS-049"'                                  -> 36 rows  (single-term, already worked)
```

Because the entire eval corpus is multi-word, the keyword lane was empty on every
eval query and BM25 contributed nothing — which is why the prior results doc
recorded "no aggregate win" (the lane was inert, not weak). The fix tokenises the
query on whitespace, quotes **each** term (doubling internal `"`), and joins with
` OR ` — fully injection-safe (every user term is its own quoted phrase; the `OR`
is ours, not the user's), and the rare identifier token now matches inside a
multi-word query so `bm25()`'s term-rarity weighting floats it up. See ADR-013 D7
and `internal/store/keyword.go`.

## Composite weight (unchanged)

| | relevance | importance | recency | structural |
|---|---|---|---|---|
| **In force** | 0.35 | 0.25 | 0.25 | 0.15 |

**No composite re-weight.** BM25 enters the pipeline via **recall-layer RRF
fusion** of two recall sets (the vector/composite-ordered lane and the
keyword/BM25-ordered lane), NOT as a fifth term in `compositeScore` (ADR-013 D3).
`bm25()` is unbounded-negative while cosine similarity is `[0,1]`; folding BM25
into the composite would require normalising an unbounded score and re-tuning all
four weights. RRF consumes *ranks*, never raw scores, and degrades to the identity
when the keyword lane is empty, so the four-signal composite weights stand.

## Same-session before/after eval (both rerank paths)

Protocol (ADR-013 §6 / agentic-t3c9 / 9c1e0ec6 same-session-floor pattern): the
authoritative floor for non-regression is the **same-session BM25-disabled run**,
NOT the committed `docs/evals/baseline.json` (which drifts as the brain grows).
`bm25_enabled=false` produces the literal pre-BM25 code path. Binary built with
`-tags fts5`; eval against the live Ollama `nomic-embed-text` provider, N=3
deterministic (all three runs byte-identical metrics). All runs written to
`/tmp/*.json` — the committed `baseline.json` was **never** clobbered. The exact-id
subset (`q11`–`q18`) was computed from the same `Brain.Search(limit=20,
threshold=0.3)` path; the full-corpus reproduction matches the harness JSON
exactly (validating the subset method).

### Path 1 — rerank = OFF (the 2ixw default)

**Full corpus (18 queries):**

| metric | floor (BM25 off) | BM25 on | Δ |
|---|---|---|---|
| recall@5 | 0.6204 | 0.7315 | **+0.1111** |
| recall@10 | 0.7870 | 0.8704 | **+0.0833** |
| recall@20 | 0.7870 | 0.9537 | **+0.1667** |
| MRR | 0.3926 | 0.6459 | **+0.2533** |

**Exact-identifier subset (8 queries, q11–q18):**

| metric | floor (BM25 off) | BM25 on | Δ |
|---|---|---|---|
| recall@5 | 0.6250 | 0.7500 | **+0.1250** |
| recall@10 | 0.7500 | 0.8750 | **+0.1250** |
| recall@20 | 0.7500 | 1.0000 | **+0.2500** |
| MRR | 0.2646 | 0.5739 | **+0.3093** |

### Path 2 — rerank = RRF (rerank_enabled=true, reference MiniLM cross-encoder)

**Full corpus (18 queries):**

| metric | floor (BM25 off) | BM25 on | Δ |
|---|---|---|---|
| recall@5 | 0.8056 | 0.8241 | **+0.0185** |
| recall@10 | 0.9074 | 0.9259 | **+0.0185** |
| recall@20 | 0.9259 | 0.9537 | **+0.0278** |
| MRR | 0.6224 | 0.8098 | **+0.1874** |

**Exact-identifier subset (8 queries, q11–q18):**

| metric | floor (BM25 off) | BM25 on | Δ |
|---|---|---|---|
| recall@5 | 0.8750 | 0.8750 | +0.0000 |
| recall@10 | 1.0000 | 1.0000 | +0.0000 |
| recall@20 | 1.0000 | 1.0000 | +0.0000 |
| MRR | 0.5699 | 0.7500 | **+0.1801** |

## AC4-NR (gating): PASS on both paths

Every metric on the BM25-on path is `>=` its same-session BM25-disabled floor on
both rerank=off and rerank=rrf. No regression on any metric, full or subset.

## AC4-IMP (AQ, non-gating): measurable improvement — now real

With the tokenizer fix and the exact-identifier-stressed queries, BM25 shows a
**genuine, measured improvement** — not manufactured:

- **rerank=off (default):** the keyword lane lifts every metric on both the full
  corpus and the exact-id subset. The headline is **recall@20 0.75 → 1.00 on the
  exact-id subset** — vector recall (rerank off) was *missing* the canonical
  identifier record entirely on several queries; BM25 fusion pulls it into the cut.
- **rerank=rrf:** the cross-encoder already pulls the exact-id targets into top-10
  (recall@K at ceiling), so BM25 adds no *recall@K* there — but it still improves
  **MRR** (0.57 → 0.75 subset; 0.62 → 0.81 full) by ranking the genuine target
  higher. Honest reading: on this path BM25's value is rank-quality, not coverage.

### Per-query off→on evidence (rerank=off, the gap path)

Verified per-query rank of the genuine canonical ground-truth node, BM25 off vs
on (rank within the top-20 `Brain.Search` result; "miss" = outside top-20):

| qid | identifier | GT node | rank OFF | rank ON | genuine win |
|---|---|---|---|---|---|
| q11 | OO-015 | `6af4875c` | 6 | 2 | recall@5 |
| q12 | BUG-001 | `93a2a152` | 2 | 1 | MRR (already top-5) |
| q13 | HS-049 | `a0b4f48d` | **miss** | 11 | recall@20 |
| q14 | HS-030 | `67ce0517` | 2 | 1 | MRR (already top-5) |
| q15 | OO-011 | `a84e6121` | **miss** | 6 | recall@10, recall@20 |
| q16 | agentic-2ixw | `1b3aa14d` | 2 | 1 | MRR (already top-5) |
| q17 | agentic-x183 | `c8f1a8f4` | 4 | 2 | MRR (already top-5) |
| q18 | HS-046 | `51d5461c` | 5 | 3 | MRR (already top-5) |

Three queries (q11/q13/q15) are genuine **recall@K** wins (the GT node was outside
top-K, often missed entirely, with BM25 off and is pulled inside with BM25 on).
The other five had the GT already inside vector top-5, so they are honest
**MRR-only** improvements — recorded as such, not inflated into fake recall wins.
This is the honest bar: where there was no recall gap, none is claimed.

## FTS5 tokenizer choice

**Chosen: the default `unicode61` tokenizer (no custom `tokenchars`), per-token-OR
MATCH expression.**

`unicode61` lowercases and splits on punctuation, so `HS-049` tokenises to
`hs`+`049`. The per-token-OR builder wraps each query token as its own phrase
(`"HS-049"`), which tokenises to the same `hs`+`049` sequence as the indexed
content, so the identifier matches (live-proven count=36 for `HS-049`). A custom
`tokenize='unicode61 tokenchars '-''` (keeping `-` in tokens) was tested and also
yields a match — it offers no additional exact-identifier recall, so the default
is kept (simplest that works). The injection-neutralisation per-token phrase
quoting (S-PI-N1) is what makes the default tokenizer sufficient.

## Reproduction

```bash
# Build the FTS5-linked binary
go build -tags fts5 -o /tmp/cerebro-fts5 ./cmd/cerebro

# Path 1 floor + after (rerank off, the default)
/tmp/cerebro-fts5 config set rerank_enabled false -p <brain>
/tmp/cerebro-fts5 config set bm25_enabled false -p <brain>
/tmp/cerebro-fts5 eval --out /tmp/floor_rerankoff.json -p <brain>
/tmp/cerebro-fts5 config set bm25_enabled true -p <brain>
/tmp/cerebro-fts5 eval --out /tmp/bm25on_rerankoff.json -p <brain>

# Path 2 floor + after (rerank rrf)
export CEREBRO_RERANK_COMMAND="$(pwd)/docs/evals/reranker/.venv/bin/python \
    $(pwd)/docs/evals/reranker/rerank_minilm.py"
/tmp/cerebro-fts5 config set rerank_enabled true -p <brain>
/tmp/cerebro-fts5 config set rerank_fusion rrf -p <brain>
/tmp/cerebro-fts5 config set bm25_enabled false -p <brain>
/tmp/cerebro-fts5 eval --out /tmp/floor_rerankon.json -p <brain>
/tmp/cerebro-fts5 config set bm25_enabled true -p <brain>
/tmp/cerebro-fts5 eval --out /tmp/bm25on_rerankon.json -p <brain>

# The exact-id subset (q11–q18) metrics are computed from the same
# Brain.Search(20, 0.3) path the harness uses; the full-corpus reproduction
# matches the harness JSON exactly.

# NEVER pass --out docs/evals/baseline.json (do not clobber the committed reference)
```
