# ADR-011: Recall-Quality Eval Harness

**Date:** 2026-06-02
**Status:** Accepted
**Ticket:** agentic-x183
**Supersedes:** —
**Superseded by:** —

## Context

Cerebro has a composite scorer (`0.35·relevance + 0.25·importance + 0.25·recency + 0.15·structural`,
`internal/store/search.go:compositeScore`) but no measurement infrastructure for whether
changes improve or degrade recall quality. Four downstream tickets are blocked on this:

- `agentic-2lak` — BM25 keyword search via SQLite FTS5
- `agentic-2ixw` — Optional local cross-encoder reranking (BGE)
- `agentic-sx4u` — Alpha-decayed frontier-pruned graph traversal
- `agentic-psis` — Pre-baked top-K neighbour cache (FORA-style Forward-Push)

All four need a *before* baseline to prove their value. This ADR records the three
non-obvious design decisions made in `agentic-x183`.

## Decision 1: `cerebro eval` subcommand over standalone binary or `go test` harness

**Chosen:** First-class Cobra subcommand in `cmd/cerebro/cmd_eval.go`.

**Rejected alternatives:**

1. **`tests/evals/`-style `go test` harness** (literal graphiti mirror): a `go test`-gated
   harness conflates measurement with the strict-TDD test gate and cannot cleanly satisfy
   AC1 ("operator runs the CLI entry point with `--help`"). The CLAUDE.md "TDD (strict)"
   convention reserves `go test` for unit/integration tests, not ad-hoc measurement tooling.

2. **Standalone `cmd/cerebro-eval/` binary**: duplicates the `openBrain()` / `resolveProjectDir()` /
   format-flag plumbing that already exists as `package main` helpers in `cmd/cerebro`, and
   breaks the "single `cerebro` binary, building-block subcommands" CLAUDE.md convention.

**Rationale:** The `cerebro eval` subcommand self-registers in `init()` via `rootCmd.AddCommand`,
reuses `openBrain()`/`resolveProjectDir()`/`-p` verbatim, matches `cmd_recall.go` precedent,
and makes `--help` work for free via Cobra's built-in usage rendering. The graphiti
`eval_cli.py` thin-CLI shape is preserved in Cobra idiom.

## Decision 2: Committed node-IDs + live-validation + provenance manifest, not a frozen DB snapshot

**Chosen:** Commit hand-authored queries + opaque UUIDs + a provenance manifest. Validate
ground-truth IDs against the live `nodes` table at harness run time (AC2b preflight).

**Rejected alternative:**

- **Frozen brain snapshot committed as a `.sqlite` blob** (option A): would (i) bloat the
  git repo with a binary blob, (ii) embed potentially-sensitive dogfooded memory content
  into git history (full PII scrubbing is explicitly out of scope for `agentic-x183`, and
  committing the raw brain would force that scope in), and (iii) decouple the measurement
  from the live, evolving brain the harness is meant to dogfood.

**Rationale:** Committing node IDs (opaque UUIDs) and hand-authored queries — not raw memory
content — to `docs/evals/` materially reduces the PII surface while keeping the measurement
grounded in the live brain. The AC2b preflight detects GC-pruned IDs before they silently
corrupt recall denominators. The provenance manifest (`corpus.md`) records the brain's state
at assembly time (schema version, node count, embedding model) for reproducibility auditing.

**Security posture:** This pattern is accepted at LOW/INFO for a solo-operator public repo
(no third-party PII, no credentials, opaque UUIDs). Re-grades to MEDIUM/HIGH if the corpus
ever contains third-party PII or `corpus.md` is enriched with example memory snippets.
See `agentic-x183` security review (OWASP 0/0/0/2-LOW/3-INFO).

## Decision 3: Macro-averaged recall@K + MRR as the v1 metric set; per-component contribution deferred

**Chosen:** Report `recall@5`, `recall@10`, `recall@20`, and `MRR`. No per-component breakdown.

**Deferred (not in scope for v1):** Per-component contribution (breaking out the relevance /
importance / recency / structural sub-terms per query).

**Rationale:** `Brain.Search` returns only the final blended `ScoredNode.Score` and raw
`Similarity` (from `store/types.go:66-72`). The four weighted sub-terms are computed *inside*
`compositeScore` and not returned. Recovering them in the harness would require either
(a) re-deriving each term from returned `Node` fields — a duplicate of the `compositeScore`
formula that drifts independently — or (b) instrumenting the store layer, which edges toward
the forbidden scorer modification (Out of Scope per `agentic-x183` brief). Neither is
required by any formal acceptance criterion.

The macro-averaged recall@K + MRR set is sufficient to prove or disprove value for all four
downstream tickets. Per-component attribution is filed as a follow-up concern, especially for
`agentic-sx4u` which replaces the structural term.

## Consequences

1. Downstream tickets `agentic-2lak`, `agentic-2ixw`, `agentic-sx4u`, `agentic-psis` can
   now measure improvement against the committed `baseline.json`.
2. The harness is additive and read-only against the brain — no schema change, no scorer change.
3. The committed corpus must be kept sanitised per S-LOW-1/S-LOW-2 (no raw content, no paths).
4. If a future scoring ticket needs per-component attribution, it should file a dedicated
   ticket that either (a) instruments `compositeScore` to return sub-terms or (b) adds a
   thin evaluation-only mirror of the formula alongside the harness.
5. The Ollama dependency at run time (claim 14 in the `agentic-x183` design) is documented
   in `docs/evals/README.md` — AC3/AC4 require a running Ollama instance.

## Scorer weights recorded (AC4 contract)

These are the weights at the time of `agentic-x183` implementation, sourced from
`internal/store/search.go:compositeScore`:

```
relevance  = 0.35
importance = 0.25
recency    = 0.25
structural = 0.15
```

Any future change to these weights constitutes a scorer change and must be tracked with a
corresponding harness re-run and updated `baseline.json`.
