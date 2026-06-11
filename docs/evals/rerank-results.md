# Reranking before/after eval results — agentic-2ixw

Measured with `cerebro eval` against the dogfooded agentic brain, the same
corpus committed at `docs/evals/{queries,ground-truth}.jsonl` (10 queries,
`--limit 20 --threshold 0.3`). Reference reranker: `cross-encoder/ms-marco-MiniLM-L6-v2`
via `docs/evals/reranker/rerank_minilm.py` (see that directory's README).

## Important measurement caveat: brain drift since the committed baseline

The committed `docs/evals/baseline.json` was captured on 2026-06-02 at **538
active nodes**. By the time this ticket was implemented (2026-06-11) the
dogfooded brain had grown to **549 active nodes**. New memories shift which
ground-truth nodes land in each top-K window, so the *committed* baseline is no
longer reproducible by ANY code path on the current brain — including the
disabled (pre-rerank) path. The honest, apples-to-apples comparison is therefore
**enabled vs disabled on the same 549-node brain**, run in the same session.

A clean `main` (da518c6) binary and this feature branch produce **byte-identical**
disabled-path metrics on the same brain (delta = 0.0 on all four metrics), which
proves the disabled path is a true no-op (AC2b) and that the drift is brain
growth, not this change.

## AC2b — disabled path is byte-identical to pre-rerank (PASS)

Same brain (549 active nodes), `rerank_enabled=false`:

| metric    | main @ da518c6 | feature branch | delta |
|-----------|----------------|----------------|-------|
| recall@5  | 0.5667         | 0.5667         | 0.0   |
| recall@10 | 0.7833         | 0.7833         | 0.0   |
| recall@20 | 0.8167         | 0.8167         | 0.0   |
| MRR       | 0.5533         | 0.5533         | 0.0   |

Delta = 0.0 (well within the ±0.001 AC2b envelope). The disabled branch is the
exact `VectorSearch(limit) → ExpandGraph(limit) → filter` path.

## AC4 — enabled vs disabled on the same brain (the apples-to-apples result)

Reference MiniLM reranker, `rerank_enabled=true`, over-retrieve 40 → rerank → cut 20.
Deterministic across N=3 runs (cross-encoder is deterministic; embeddings stable):

| metric    | disabled (549) | enabled (549) | delta (enabled − disabled) |
|-----------|----------------|---------------|----------------------------|
| recall@5  | 0.5667         | 0.7333        | **+0.1667** |
| recall@10 | 0.7833         | 0.7667        | −0.0167 |
| recall@20 | 0.8167         | 0.8667        | **+0.0500** |
| MRR       | 0.5533         | 0.8571        | **+0.3038** |

**Reranking improves 3 of 4 metrics**, most strongly MRR (+0.30) and recall@5
(+0.17) — i.e. the most relevant memories rank higher, which is the user-story
goal. recall@10 dips marginally (−0.017, one ground-truth node displaced in the
5–10 band). AC4-IMP (≥2/3 runs strictly beat baseline on recall@10 OR MRR) is
satisfied: MRR strictly improves in 3/3 runs.

## AC4-NR — non-regression vs the *committed* floor (brain-drift caveat)

The Wire gate (AC4-NR) is specified against the committed baseline floor
(r@5≥0.78, r@10≥0.82, r@20≥0.82, MRR≥0.71). On the current 549-node brain:

| metric    | committed floor | enabled | floor met? |
|-----------|-----------------|---------|------------|
| recall@5  | 0.7833          | 0.7333  | NO — brain drift (disabled path is also below: 0.567) |
| recall@10 | 0.8167          | 0.7667  | NO — brain drift (disabled path is also below: 0.783) |
| recall@20 | 0.8167          | 0.8667  | YES |
| MRR       | 0.7083          | 0.8571  | YES |

Because the disabled path on the *same* brain is itself below the committed
floor on recall@5 and recall@10, the floor is unreachable by any path on the
drifted brain — this is **not** a reranking regression. The reranker beats the
*same-brain disabled* baseline on 3/4 metrics (above). The clean re-measurement
of the committed floor requires regenerating `baseline.json` against the current
brain (`cerebro eval -p <brain> --out docs/evals/baseline.json`), which is an
operator decision deliberately NOT taken here (it would rewrite the committed
reference). **Recommendation:** the operator regenerates the committed baseline
against the current brain, then re-runs the enabled eval so AC4-NR is evaluated
against a contemporaneous floor.

## Reproduce

```bash
# 1. Build
go build -o /tmp/cerebro ./cmd/cerebro

# 2. Reference reranker (one-time)
python3 -m venv docs/evals/reranker/.venv
docs/evals/reranker/.venv/bin/pip install sentence-transformers
export CEREBRO_RERANK_COMMAND="$(pwd)/docs/evals/reranker/.venv/bin/python $(pwd)/docs/evals/reranker/rerank_minilm.py"

# 3. Disabled (AC2b) — explicit --out so the committed baseline is never clobbered
/tmp/cerebro eval -p <brain> --out /tmp/rerank-disabled.json

# 4. Enabled (AC4)
/tmp/cerebro config set rerank_enabled true -p <brain>
/tmp/cerebro eval -p <brain> --out /tmp/rerank-enabled.json
/tmp/cerebro config set rerank_enabled false -p <brain>   # restore
```
