# ADR-017: The dream consolidation pass is an agent-executed skill, not a command

**Status:** Accepted
**Date:** 2026-09-08
**Ticket:** agentic-modb

## Context

Cerebro's memory-flywheel surfaces (inbox quarantine, `consolidate --suggest`,
atomic `consolidate --into`, `supersede`, `outcome`, `gc`, `eval`) shipped as
composable building blocks, but nothing packaged them into a repeatable
off-hot-path maintenance workflow. The 2026-08 competitive re-audit named this
the keystone gap: the field automated the memory flywheel (Mem0 Dream's
merge/supersede/synthesize pass, Claude Code Auto Dream's four phases, Letta
sleeptime reflection) while cerebro's equivalent surfaces sat unused —
0 recorded outcomes, near-zero consolidation, flat graph density.

A manual dry-run of the full pass (agentic-fwmq, 2026-08-28) executed
end-to-end on two live brains with existing surfaces only and measured the
outcome on a same-session eval pair: recall@5 +0.148, MRR +0.199 on the
agentic brain. Consolidation is a recall-quality lever, not just hygiene — and
no missing mechanism surfaced.

## Decision

`/dream` ships as an **agent-executed skill** sequencing the existing
fail-closed primitives — an embedded init template
(`cmd/cerebro/templates/skill_dream.md`) plus a byte-identical plugin copy
(`claude-plugin/cerebro/skills/dream/SKILL.md`), both covered by the lockstep
drift guards (`plugin_assets_test.go`, `scaffold_test.go`). No new Go command,
no schema change, no scorer change.

The pass: preconditions snapshot (stats + effective `gc_threshold`) → backup →
eval BEFORE + ground-truth exclusion set (corpus-backed brains) → per-candidate
inbox review → candidate selection (`--suggest` + thematic carving) →
dedup-first synthesis (wire into existing nodes first; supersede
contradictions; absolute-ize dates) → atomic `--into` wiring → exclusion
checks → bounded prune (dry-run gated, never `--threshold`) → eval AFTER,
outcomes, density report.

### Why not a `cerebro dream` orchestration command

1. **Model B forbids a full orchestrator.** The essential steps (synthesis,
   dedup judgement, contradiction resolution, date absolute-ization) are
   cognition; a Go command executing them would put an LLM inside cerebro —
   refused by ADR-006 and the roadmap invariants.
2. **A partial command adds surface without determinism.** The only
   Model-B-legal command is a pre-flight/report wrapper over
   `stats`/`inbox list`/`consolidate --suggest`/`backup`. Every correctness
   guarantee it could claim already lives in the fail-closed primitives it
   would wrap, against the repo's building-block CLI pattern, while dragging
   mandatory surface cost (usage-map entry, README table, tests, CHANGELOG)
   for zero movement of any gating predicate.
3. **Governance hygiene.** Consolidation stays agent-judged — no timer, no
   threshold trigger. A binary `dream` subcommand is precisely the surface a
   future cron or hook would be pointed at. A skill with
   `disable-model-invocation: true` keeps invocation operator-manual and
   execution agent-judged (slash-only semantics confirmed against the Claude
   Code skills documentation: the skill "can only be triggered manually").
4. **The dry-run proved sufficiency.** agentic-fwmq ran the full pass with
   existing surfaces only and measured recall improvement.

### Alternatives considered

- **`cerebro dream` Go command** — rejected, grounds above.
- **Extending `/consolidate` in place** — rejected: different semantics →
  different name. `/consolidate` remains the in-session single-cluster
  judgement skill; `/dream` is the full off-hot-path maintenance pass that
  *contains* consolidation.
- **Scheduling/daemonisation** — out of scope by ticket; manual `/dream`
  first, scheduling is a separate follow-up decision.

## Consequences

- The correctness substrate stays wire-enforced where it matters: atomic
  fail-closed consolidation (`internal/store/consolidate.go` — single tx,
  validate-before-write, idempotent edge upsert), structural inbox quarantine,
  the eval harness's zero-ground-truth abort, and the drift guards that fail
  CI if the template and plugin copy diverge.
- Judgement quality (cluster selection, synthesis, supersede decisions) is
  deliberately agent-owned and measured, not gated: each pass reports the
  same-session eval pair and the graph-density delta.
- Safety bounds live in the skill text: backup before mutation; never pass
  `gc --threshold`; skip gc when the brain-config `gc_threshold` exceeds 0.05
  or the dry-run eviction count exceeds max(10, 2% of active nodes); pass
  memory-derived content via scratch files (`"$(cat file)"`) so embedded
  backticks/`$(` cannot execute (CWE-78). `allowed-tools` pre-approves only
  the fixed command shapes the pass uses (`cerebro`, `jq`,
  `xargs -n 60 cerebro`); `Write` is pre-approved unscoped because Claude
  Code consults path specifiers on `Edit`/`Read` rules only — a `Write(path)`
  rule is accepted but never checked (documented acceptance).
- Density growth across passes re-gates the deferred graph-traversal track
  when the evidence supports it.

## Evidence

- Field pattern: Mem0 Dream, Claude Code Auto Dream, Letta sleeptime
  (competitive re-audit 2026-08, §2.2).
- Live check: the fwmq dry-run's measured same-session pair (agentic brain
  recall@5 +0.148, MRR +0.199, 2026-08-28).
- Atomicity substrate: `internal/store/consolidate.go:33-91` and its tests.
