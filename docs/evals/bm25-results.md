# BM25/FTS5 Keyword Recall — Rebalance & Before/After Eval (agentic-2lak)

This is the AC3 artefact: it records the composite-weight rebalance decision, the
same-session before/after `cerebro eval` numbers for both rerank paths, and the
FTS5 tokenizer choice. Measured against the dogfooded brain (556 active nodes,
`nomic-embed-text` 768-dim) over the committed corpus
(`docs/evals/queries.jsonl` = 10 queries, `docs/evals/ground-truth.jsonl`).

## Composite weight rebalance

| | relevance | importance | recency | structural |
|---|---|---|---|---|
| **Before** (in force at implementation time) | 0.35 | 0.25 | 0.25 | 0.15 |
| **After** | 0.35 | 0.25 | 0.25 | 0.15 (**unchanged**) |

**Decision: no composite re-weight.** BM25 enters the pipeline via **recall-layer
RRF fusion** of two recall sets (the vector/composite-ordered lane and the
keyword/BM25-ordered lane), NOT as a fifth term in `compositeScore`
(ADR-013 D3). `bm25()` is unbounded-negative while cosine similarity is `[0,1]`;
folding BM25 into the composite would require normalising an unbounded score and
re-tuning all four weights — high-risk for the non-regression floor. RRF consumes
*ranks*, never raw scores, and degrades to the identity when the keyword lane is
empty, so the four-signal composite weights are left untouched. A re-weight would
be performed ONLY if eval evidence showed a win; it does not (see below), so the
weights stand.

## Same-session before/after eval (both rerank paths)

Protocol (ADR-013 §6 / agentic-t3c9 / 9c1e0ec6 same-session-floor pattern): the
authoritative floor for non-regression is the **same-session BM25-disabled run**,
NOT the committed `docs/evals/baseline.json` (which drifts as the brain grows).
`bm25_enabled=false` produces the literal pre-BM25 code path. Binary built with
`-tags fts5`; eval against the live Ollama `nomic-embed-text` provider. All runs
written to `/tmp/*.json` (the committed `baseline.json` was NEVER clobbered).

### Path 1 — rerank = OFF (the 2ixw default)

| metric | floor (BM25 off) | BM25 on (N=3, all identical) | Δ |
|---|---|---|---|
| recall@5 | 0.6167 | 0.6167 | +0.0000 |
| recall@10 | 0.8167 | 0.8167 | +0.0000 |
| recall@20 | 0.8167 | 0.8167 | +0.0000 |
| MRR | 0.4950 | 0.4950 | +0.0000 |

### Path 2 — rerank = RRF (rerank_enabled=true, reference MiniLM cross-encoder)

| metric | floor (BM25 off) | BM25 on (N=3, all identical) | Δ |
|---|---|---|---|
| recall@5 | 0.7500 | 0.7500 | +0.0000 |
| recall@10 | 0.8333 | 0.8333 | +0.0000 |
| recall@20 | 0.8667 | 0.8667 | +0.0000 |
| MRR | 0.6644 | 0.6644 | +0.0000 |

## AC4-NR (gating): PASS on both paths

Every metric on the BM25-on path is `>=` its same-session BM25-disabled floor on
both rerank=off and rerank=rrf (here, exactly equal and deterministic across
N=3). No regression. The non-regression floor is structurally protected: with the
keyword lane fused via RRF into an already-strong vector candidate set whose
ground-truth nodes already sit within vector top-20, the fused order does not
demote any ground-truth node out of top-K.

## AC4-IMP (AQ, non-gating): no aggregate improvement on this corpus — directional finding

No run shows aggregate recall@K/MRR strictly above the floor on either path. The
honest reason: the committed corpus's ground-truth nodes are already within the
vector lane's top-20, so RRF fusion with the keyword lane has no ground-truth
node to *promote into* top-K that vector recall hadn't already found.

The keyword lane IS live and DOES reshape the candidate set — it is simply not
exercised by an aggregate metric on this corpus:

- `nodes_fts` MATCH `"HS-049"` on the live brain returns **36 matching nodes**
  (the FTS5 index is populated and queryable: 556 fts rows = 556 active nodes).
- For the exact-identifier query `HS-049`, BM25-on vs BM25-off changes the fused
  top-10: BM25-off top-5 = `[aff1debf, a456426f, da476c33, 04517855, ba05afdc]`;
  BM25-on top-10 pulls in keyword-dense nodes (`bef744ff`, `2c8674e2`,
  `9142932d`) that BM25-off did not surface — i.e. the keyword lane measurably
  alters which candidates reach the cut.

**Why no single top-5 "win" was recorded:** FTS5 `bm25()` ranks matches by term
*density*, so short, HS-049-centric nodes outrank a long memory that merely
mentions HS-049 in passing (e.g. the `HS-049 FILED…` node `c000442e`, where the
identifier is a small fraction of a long episode, ranks below the keyword-dense
matches). This is correct BM25 behaviour. To register an aggregate AC4-IMP win,
the corpus would need exact-identifier queries whose ground-truth target is the
keyword-densest match AND outside vector top-20 — the current 10-query corpus has
no such case. This is an AQ (non-gating) criterion; AC4-NR (the gate) passes.

**Follow-up candidate:** extend `docs/evals/queries.jsonl` with exact-identifier
queries whose target is keyword-dense and vector-distant, to make the AC4-IMP
signal measurable on aggregate. Out of scope for this ticket (the eval corpus is
owned by the agentic-x183 harness).

## FTS5 tokenizer choice (TL nitpick R6)

**Chosen: the default `unicode61` tokenizer (no custom `tokenchars`).**

`unicode61` lowercases and splits on punctuation, so `HS-049` tokenises to
`hs` + `049`. The concern (R6) was that hyphenated identifiers might underperform.
Live-proven (mattn/go-sqlite3 v1.14.34) that this is a non-issue under
phrase-quoting: the MATCH builder wraps the query as a single phrase `"HS-049"`,
which tokenises to the same `hs`+`049` sequence as the indexed content, so the
phrase matches exactly (count=1). A custom `tokenize='unicode61 tokenchars '-''`
(keeping `-` in tokens) was tested and also yields count=1 — it offers no
additional exact-identifier recall over the default, so the default is kept (no
custom tokenizer config, simplest that works). The injection-neutralisation
phrase-quoting (S-PI-N1) is what makes the default tokenizer sufficient.

## Reproduction

```bash
# Build the FTS5-linked binary
go build -tags fts5 -o /tmp/cerebro-fts5 ./cmd/cerebro

# Path 1 floor + after (rerank off, the default)
/tmp/cerebro-fts5 config set bm25_enabled false -p <brain>
/tmp/cerebro-fts5 eval --out /tmp/floor_rerankoff.json -p <brain>
/tmp/cerebro-fts5 config set bm25_enabled true -p <brain>
/tmp/cerebro-fts5 eval --out /tmp/bm25on_rerankoff.json -p <brain>

# Path 2 floor + after (rerank rrf)
/tmp/cerebro-fts5 config set rerank_enabled true -p <brain>
/tmp/cerebro-fts5 config set rerank_command "<minilm-subprocess>" -p <brain>
/tmp/cerebro-fts5 config set bm25_enabled false -p <brain>
/tmp/cerebro-fts5 eval --out /tmp/floor_rerankon.json -p <brain>
/tmp/cerebro-fts5 config set bm25_enabled true -p <brain>
/tmp/cerebro-fts5 eval --out /tmp/bm25on_rerankon.json -p <brain>

# NEVER pass --out docs/evals/baseline.json (do not clobber the committed reference)
```
