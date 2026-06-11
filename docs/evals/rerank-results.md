# Reranking before/after eval results — agentic-2ixw

Measured with `cerebro eval` against the dogfooded agentic brain, the same
corpus committed at `docs/evals/{queries,ground-truth}.jsonl` (10 queries,
`--limit 20 --threshold 0.3`). Reference reranker: `cross-encoder/ms-marco-MiniLM-L6-v2`
via `docs/evals/reranker/rerank_minilm.py` (see that directory's README).

## Combine modes compared

When reranking is **enabled**, the reranker ranking is combined with the
composite ranking in one of two modes (`rerank_fusion` config key):

- **`reorder`** (legacy pure-reorder): sort the candidate set by the reranker
  score alone, *discarding* the composite order.
- **`rrf`** (new default): Reciprocal Rank Fusion —
  `fused = 1/(k+rank_composite) + 1/(k+rank_reranker)`, `k=60`
  (Cormack et al., SIGIR 2009; Elasticsearch `rrf` retriever default
  `rank_constant`). Combines both rankings instead of replacing one with the
  other.

Reranking overall is **default OFF** (`rerank_enabled=false`); this comparison
is about the combine mode WHEN enabled.

## The four-way result (same 550-node brain, deterministic N=3)

All four configurations run against the **same** dogfooded brain in the same
session (550 active nodes at measurement time), with the reference MiniLM
reranker. The cross-encoder and the embeddings are deterministic — all four
configs produced **byte-identical metrics across N=3 runs each**.

| config                          | recall@5 | recall@10 | recall@20 | MRR     |
|---------------------------------|----------|-----------|-----------|---------|
| disabled (baseline)             | 0.5667   | 0.7833    | 0.8167    | 0.5033  |
| enabled + **pure-reorder**      | 0.7333   | **0.7667**| 0.8667    | **0.8571** |
| enabled + **RRF fusion** (new)  | 0.7500   | **0.8333**| 0.8667    | 0.6825  |

Deltas vs the **disabled** baseline (the honest apples-to-apples floor):

| metric    | reorder − disabled | RRF − disabled | RRF − reorder |
|-----------|--------------------|----------------|---------------|
| recall@5  | **+0.1667**        | **+0.1833**    | +0.0167       |
| recall@10 | **−0.0167** (dip)  | **+0.0500**    | **+0.0667**   |
| recall@20 | **+0.0500**        | **+0.0500**    | 0.0000        |
| MRR       | **+0.3538**        | **+0.1792**    | −0.1746       |

### What this says

- **RRF recovers the recall@10 dip and overshoots to above the disabled
  baseline** (`0.7833 → 0.8333`, +0.05), while pure-reorder *loses* recall@10
  (`−0.0167`). RRF beats pure-reorder on recall@10 by **+0.0667**.
- **RRF still improves MRR** (`0.5033 → 0.6825`, +0.18) and recall@5 (+0.18) and
  recall@20 (+0.05) — it improves **all four** metrics over disabled.
- The one tradeoff: **RRF's MRR gain (+0.18) is smaller than pure-reorder's
  (+0.35).** This is expected and intended — fusion tempers the cross-encoder's
  aggressive top-1 promotion with the composite rank, producing a more balanced
  ranking. Pure-reorder maximises MRR by letting the single best item win
  outright, at the cost of recall@10.

RRF is the **principled win**: it combines two rankings rather than discarding
one, and it lifts recall@10 to parity-or-better while holding an MRR improvement.

## Pinpointing the displaced node (single-node effect confirmed)

A per-query trace (top-10 under each mode, ground-truth ranks) confirms the
recall@10 dip is the single-node displacement the hypothesis predicted:

- **Query q04** ("session start compaction detection re-prime memories"),
  ground-truth node **`77cdcf02-4f3a-497c-9069-a80bb1bccca3`**:
  - disabled: rank **#1** (in top-10) → q04 recall@10 = 1.00
  - pure-reorder: **dropped out of the top-10 (MISS)** while the *other* GT node
    `741a7615` was promoted to #1 → q04 recall@10 = 0.50 (MRR up, recall down)
  - **RRF: recovered to rank #4** → q04 recall@10 = 1.00

- A second node behaved the same way: **Query q06**
  ("GitHub Flow branch pull request admin merge"), ground-truth node
  **`c4bc1a91-...`** — disabled #3, pure-reorder MISS, **RRF recovered to #8**.

RRF recovers *both* nodes that pure-reorder pushed below rank 10, which is
exactly why macro recall@10 rises from `0.7667` (reorder) to `0.8333` (RRF),
above the disabled `0.7833`.

## Corpus-granularity caveat (read this before over-reading the deltas)

The corpus is **10 queries / ~20 ground-truth nodes**. At that size recall@10
resolves to roughly **one node** (≈0.033–0.05 macro per ground-truth node). So:

- The original pure-reorder `−0.0167` recall@10 swing is **sub-single-node** —
  smaller than one node's worth of macro recall, i.e. within the measurement
  noise of this corpus.
- Any combine-mode tuning result on THIS corpus is **low-confidence in
  magnitude.** The numbers are reproducible and deterministic, but the corpus is
  too small for the exact deltas to be high-confidence.

RRF is adopted on **principled** grounds (it fuses rather than discards a
ranking, and recovers recall@10 to parity-or-better while holding MRR), not
because the small-corpus numeric delta is itself decisive. **Recommendation:**
grow the eval corpus before drawing high-confidence conclusions about
RRF-vs-α-blend or the exact `k`. Until then, RRF is the safe default — it strictly
dominates disabled on all four metrics here and never discards a ranking.

## AC2b — disabled path is byte-identical to pre-rerank (PASS)

Same brain (550 active nodes), `rerank_enabled=false`: the disabled path is the
exact `VectorSearch(limit) → ExpandGraph(limit) → filter` pipeline, unchanged by
this ticket. Disabled metrics are deterministic across N=3
(recall@5=0.5667, recall@10=0.7833, recall@20=0.8167, MRR=0.5033) and match the
pre-rerank path — the RRF change touches only the enabled combine step.

## A note on the committed baseline floor (brain drift)

The committed `docs/evals/baseline.json` was captured on 2026-06-02 at **538
active nodes**; the brain has since grown (549 → 550). New memories shift which
ground-truth nodes land in each top-K window, so the *committed* baseline floor
(r@5≥0.78, r@10≥0.82, MRR≥0.71) is not reproducible by ANY path on the current
brain — including the disabled path (whose recall@5/@10 are themselves below the
old floor on the drifted brain). The honest comparison is therefore
**enabled-vs-disabled on the same current brain**, run in the same session (the
tables above). Regenerating the committed baseline against the current brain is
an operator decision deliberately NOT taken here (it would rewrite the committed
reference and is out of scope for this combine-mode investigation).

## Reproduce

```bash
# 1. Build
go build -o /tmp/cerebro ./cmd/cerebro

# 2. Reference reranker (one-time)
python3 -m venv docs/evals/reranker/.venv
docs/evals/reranker/.venv/bin/pip install sentence-transformers
export CEREBRO_RERANK_COMMAND="$(pwd)/docs/evals/reranker/.venv/bin/python $(pwd)/docs/evals/reranker/rerank_minilm.py"

# 3. Disabled (baseline) — explicit --out so the committed baseline is never clobbered
/tmp/cerebro config set rerank_enabled false -p <brain>
/tmp/cerebro eval -p <brain> --out /tmp/rerank-disabled.json

# 4. Enabled + pure-reorder (legacy)
/tmp/cerebro config set rerank_enabled true -p <brain>
/tmp/cerebro config set rerank_fusion reorder -p <brain>
/tmp/cerebro eval -p <brain> --out /tmp/rerank-reorder.json

# 5. Enabled + RRF fusion (default)
/tmp/cerebro config set rerank_fusion rrf -p <brain>
/tmp/cerebro eval -p <brain> --out /tmp/rerank-rrf.json

# 6. Restore default-OFF
/tmp/cerebro config set rerank_enabled false -p <brain>
/tmp/cerebro config reset rerank_fusion -p <brain>
```
