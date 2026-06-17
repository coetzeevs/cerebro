# Lazy expansion gating — eval sweep and before/after results (agentic-73l6)

Threshold-sweep evidence behind the shipped defaults of the lazy expansion
gate (`expand_threshold` / `expand_spread_threshold`), per the ticket's
Definition of Done items 2–6. See `docs/adrs/ADR-014-lazy-expansion-gating.md`
for the design decision record.

## Protocol

- Same session, same brain, same Ollama instance for every run (the committed
  `baseline.json` is a historical reference only; the same-session
  gate-disabled floor governs non-regression, per the agentic-2ixw/t3c9
  judging discipline).
- Corpus: the committed 18-query set (`queries.jsonl` + `ground-truth.jsonl`).
- Brain under evaluation: the project-scoped dogfooded brain (path class
  `~/.cerebro/projects/<sha256(realpath)>.sqlite`), **561 active nodes**,
  embedding model `nomic-embed-text` (768 dims), schema version 3.
- Pipeline state: `rerank_enabled=false` (the 2ixw shipped default) and
  `bm25_enabled=true` (the 2lak shipped default) on every primary run, so both
  prior-feature lanes are present (DoD-5/6).
- Skip-rate: delta of the `stats.expansion_skips` counter (`schema_meta`)
  across each run, divided by 18 queries (`cerebro eval` drives `Brain.Search`
  — one expansion site per query, so skip events = gated queries).
- Eval invocation: `cerebro eval -p <project> --out <scratch>.json` (scratch
  outputs only; the committed `baseline.json` was not touched).

## Same-session floor (gate disabled: 0.0 / 0.0 — the AC3/AC6-NR reference)

| Metric | Floor |
|---|---|
| recall@5 | 0.7315 |
| recall@10 | 0.8704 |
| recall@20 | 0.9537 |
| MRR | 0.6459 |

These floor numbers reproduce the agentic-2lak certification run on the same
brain state exactly — eval-level evidence that the gate-disabled pipeline is
the pre-feature pipeline (AC3).

## Threshold sweep (`expand_threshold` × spread 0.0, rerank off, BM25 on)

| `expand_threshold` | Skips | Skip-rate | recall@5 | recall@10 | recall@20 | MRR | Verdict |
|---|---|---|---|---|---|---|---|
| 0.72 | 8/18 | 44% | 0.7315 | 0.8704 | 0.9537 | **0.6181** | REJECTED — MRR regression (−0.0278) |
| **0.75** | **4/18** | **22%** | 0.7315 | 0.8704 | 0.9537 | 0.6459 | **SHIPPED** — bit-identical to floor |
| 0.78 | 3/18 | 17% | 0.7315 | 0.8704 | 0.9537 | 0.6459 | clean (higher than needed) |
| 0.80 | 2/18 | 11% | 0.7315 | 0.8704 | 0.9537 | 0.6459 | clean (higher than needed) |
| 0.85 | 0/18 | 0% | 0.7315 | 0.8704 | 0.9537 | 0.6459 | inert on this brain |

All "0.6459"/"0.7315"-class equalities above are **bit-identical at full float
precision** to the floor, not rounded coincidences.

**Selection rule applied (Design §5):** shipped default = the lowest
`expand_threshold` ≥ 0.75 with zero regression on all four metrics vs the
same-session floor AND skip-rate > 0 → **0.75** (22% skip-rate, inside the
L-RAG-cited 8–26% retrieval-reduction envelope). The Architect's prior was
0.80; the sweep adjusted it downward within the grid.

## Spread spot-check (`expand_spread_threshold` at T1 = 0.75)

| `expand_spread_threshold` | Skips | recall@5/10/20, MRR vs floor |
|---|---|---|
| 0.02 | 4/18 | bit-identical (spread added no fires over T1) |
| 0.05 | 6/18 | bit-identical (2 additional T2 fires, no regression) |

No regression was observed on this corpus, but **the spread condition ships
OFF (0.0)** per the design decision: on this brain the top-1→top-K spread
*anti-correlates* with confidence (the lowest-confidence queries have the
smallest spreads), so an active spread default risks skipping expansion on
exactly the queries that need it as the corpus evolves. The mechanism is
implemented, validated and covered by unit tests; enabling it is config-only.

## Shipped-defaults run (0.75 / 0.0, compiled defaults via `config reset`)

| Metric | Floor | Shipped defaults | Δ |
|---|---|---|---|
| recall@5 | 0.7315 | 0.7315 | 0.0 |
| recall@10 | 0.8704 | 0.8704 | 0.0 |
| recall@20 | 0.9537 | 0.9537 | 0.0 |
| MRR | 0.6459 | 0.6459 | 0.0 |

- **AC6-NR (gating): PASS** — all four metrics ≥ floor (bit-identical), same
  561-node brain, same session, both 2ixw/2lak lanes present.
- **AC6-SKP (non-gating): PASS** — skip-rate 4/18 = **22.2%** > 0%.
- The run used `cerebro config reset` for both keys first, so the thresholds
  came from the compiled defaults through the brain-side resolvers — a live
  verification of the resolver default path.

## Rerank-enabled spot-check (DoD-5 optional clause) — FLAGGED FINDING

With the reference MiniLM cross-encoder configured (`rerank_enabled=true`,
operator-local subprocess) and the gate at shipped defaults:

| Metric | rerank-true floor (0.0/0.0) | rerank-true @ 0.75 | Δ |
|---|---|---|---|
| recall@5 | 0.8241 | 0.8241 | 0.0 |
| recall@10 | 0.9259 | 0.9259 | 0.0 |
| recall@20 | 0.9537 | 0.9537 | 0.0 |
| MRR | 0.8098 | 0.8085 | **−0.0013** |

Characterisation (all measured, deterministic across repeated runs of both
configurations):

- The delta is exactly one query's first-relevant hit moving one rank
  (ΔRR = (1/6 − 1/7)/18 ≈ −0.00132). All recall@K are unaffected — no
  ground-truth node leaves the result set.
- Mechanism: on gated queries the reranker receives a candidate pool without
  expansion neighbours, so the RRF fusion ranks shift slightly. This is the
  intended skip semantic (no edge reads on confident queries), surfacing on
  the rerank-enabled path only.
- The same −0.0013 persists at every active threshold (0.75 / 0.78 / 0.80 —
  the moving query is one of the two most confident, gated even at 0.80) and
  disappears only at the inert 0.85.
- The **binding** non-regression gate (AC6-NR) is judged on the shipped
  pipeline contract — `rerank_enabled=false` — which is bit-identical.
  Reranking remains default-OFF; an operator who enables reranking and wants
  rerank-path byte-identity can set `expand_threshold 0.0` (gate off).

This finding is recorded for the post-implementation reviewers to judge
against DoD-5's optional clause rather than silently absorbed.

## Reproduction

```bash
# floor
cerebro config set expand_threshold 0.0 -p <project>
cerebro config set expand_spread_threshold 0.0 -p <project>
cerebro eval -p <project> --out /tmp/floor.json

# shipped defaults (compiled)
cerebro config reset expand_threshold -p <project>
cerebro config reset expand_spread_threshold -p <project>
cerebro eval -p <project> --out /tmp/shipped.json

# skip-rate: delta of the counter across the run
sqlite3 <brain.sqlite> "SELECT value FROM schema_meta WHERE key='stats.expansion_skips'"
```

Re-run this sweep whenever the committed eval baseline is regenerated
(agentic-t3c9 lineage) — threshold calibration is empirical and drifts with
the brain's score distribution.
