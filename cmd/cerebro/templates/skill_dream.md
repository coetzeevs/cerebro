---
name: dream
description: "Off-hot-path memory maintenance pass: review the inbox, consolidate accumulated episodes into higher-order memories with provenance, resolve contradictions, prune, and measure. Run manually at natural maintenance points — never on a schedule."
argument-hint: "[optional: topic to focus the pass on]"
disable-model-invocation: true
allowed-tools: Bash(cerebro *), Bash(jq *), Bash(xargs -n 60 cerebro *), Read, Write
---

# Dream — Off-Hot-Path Consolidation Pass

The full memory-maintenance pass: inbox review, dedup-first consolidation with
provenance, contradiction resolution, pruning, and a before/after measurement.
Run it in a dedicated session at a natural stopping point — never mid-task, and
never on a schedule. Cerebro never calls an LLM: you (the agent) own every
judgement; cerebro provides selection (`consolidate --suggest`), atomic wiring
(`consolidate --into`), and the measurement ruler (`eval`). If `$ARGUMENTS`
names a topic, focus the P4 thematic carving on it.

## Safety rules (read first, apply throughout)

1. **Brain addressing.** Resolve the target brain ONCE: `BRAIN="$CLAUDE_PROJECT_DIR"`
   (or the operator-named project directory). EVERY cerebro call passes
   `-p "$BRAIN"` immediately after the subcommand. Bare calls resolve to
   `CLAUDE_PROJECT_DIR`, not the cwd — from a repo subdirectory that is a
   different brain. A trailing `-p` after a long content argument has failed
   live; keep it up front.
2. **Backup gate.** No mutating command (P3 onward) runs before the P1 backup
   path is recorded in the pass report.
3. **Prune discipline.** NEVER pass `--threshold` to `cerebro gc`. P8's abort
   bounds apply; a high threshold evicts nearly the whole brain.
4. **Content quoting.** Never paste memory-derived content directly into a
   double-quoted shell argument: embedded backticks or `$(…)` would execute as
   command substitution at parse time, not be stored. Author every synthesis
   with the Write tool into a scratch file, then pass it as
   `"$(cat /tmp/dream-synthesis-N.txt)"` — substitution output is not
   re-parsed, so backticks and `$(` inside the file are inert. When authoring
   the file, write fresh prose; do not copy raw recalled content verbatim.

## P0 — Preconditions

Snapshot the starting state (no mutation yet):

```bash
BRAIN="$CLAUDE_PROJECT_DIR"   # or the operator-named project directory
cerebro stats -p "$BRAIN" --format json > /tmp/dream-stats-before.json
cerebro config list -p "$BRAIN"
```

From the config listing, record the effective `gc_threshold`. If it is set
above 0.05, note it now: P8's gc step will be SKIPPED and surfaced to the
operator (a brain-config override makes a bare `cerebro gc` evict at that
threshold, not the 0.01 default).

## P1 — Backup

```bash
cerebro backup -p "$BRAIN"
```

Record the printed backup path in the pass report before anything mutates.
This backup is the rollback substrate for the whole pass — gc "archive" is
NOT a full-fidelity rollback (see P8).

## P2 — Eval BEFORE + ground-truth exclusion (corpus-backed brains only)

Only a brain with a committed eval corpus can be measured (today: the brain
dogfooded by the cerebro checkout). Corpus paths are cwd-relative, so run from
the cerebro repo root:

```bash
cerebro eval -p "$BRAIN" --out /tmp/dream-eval-before.json
jq -r '.relevant_node_ids[]' docs/evals/ground-truth.jsonl | sort -u > /tmp/dream-gt-exclude.txt
```

If eval aborts because no ground-truth IDs resolve, the brain is not
corpus-backed: record a **stats-only pass** (the recall A/B is non-evaluable)
and continue. Every id in `/tmp/dream-gt-exclude.txt` must still be `active`
when the pass ends (P7) — consolidating a ground-truth node invalidates the
ruler for every future measurement.

## P3 — Inbox review

```bash
cerebro inbox list -p "$BRAIN"
```

Judge each candidate individually: approve it if it is a real memory worth
keeping, discard it if it is noise. One deliberate decision — and one
command — per candidate; never bulk-approve.

```bash
cerebro inbox approve <id> -p "$BRAIN"   # real memory
cerebro inbox discard <id> -p "$BRAIN"   # noise
```

An empty inbox is not a gate — proceed to P4.

## P4 — Candidate selection

Mechanical clusters first, then thematic carving of the subtype-less mass:

```bash
cerebro consolidate --suggest -p "$BRAIN" --limit 10
cerebro search "<theme>" -p "$BRAIN" --limit 50 --threshold 0.3 --format json
cerebro list -p "$BRAIN" --type episode --status active --format json
```

Use `$ARGUMENTS` (if given) as the first carving theme.

## P5 — Dedup-first synthesis

For each cluster, in this order:

1. **Search for an EXISTING adequate concept/procedure/reflection first** and
   plan to wire the episodes into it (P6). Most clusters land on existing
   nodes — prefer that over minting near-duplicates.
2. Mint a new node only where no adequate node exists. Author the synthesis
   with the Write tool into `/tmp/dream-synthesis-N.txt` (safety rule 4:
   fresh prose, absolute dates), then:

```bash
CEREBRO_ORIGIN_ACTOR="${CEREBRO_ORIGIN_ACTOR:-claude-code}" cerebro add -p "$BRAIN" --type concept|procedure|reflection --importance <0.6-0.9> --origin-channel skill "$(cat /tmp/dream-synthesis-N.txt)"
```

3. Where new information contradicts an existing memory, supersede it (the
   old node stays as history). The replacement text follows the same
   scratch-file discipline:

```bash
cerebro supersede <old-id> -p "$BRAIN" -t <type> "$(cat /tmp/dream-synthesis-N.txt)"
```

4. **Absolute-ize every relative date** at synthesis time ("yesterday" →
   "2026-09-07"): the memory outlives the session that wrote it.

## P6 — Atomic wiring

Consolidate each cluster into its target — one atomic, fail-closed
transaction wires a `derived_from` edge per source AND flips the sources to
`consolidated`:

```bash
cerebro consolidate -p "$BRAIN" --into <target-id> <episode-id> [<episode-id>...]
```

For a large cluster, Write the source ids to an id-file (one per line) and
batch — shells do not word-split unquoted variables reliably; never a
while-read loop:

```bash
xargs -n 60 cerebro consolidate -p "$BRAIN" --into <target-id> < /tmp/dream-ids.txt
```

The `--into` target need only EXIST (any type) — wiring into an existing
concept is the dedup-first mechanic, not an error.

## P7 — Exclusions honored

Three standing exclusions; verify before pruning:

1. **Eval ground-truth nodes stay active** (mechanical check — every id in
   the P2 exclusion set):

```bash
cerebro get <id> -p "$BRAIN" --format json | jq -r .status
```

   Any ground-truth id no longer `active` means the pass consolidated the
   ruler — restore from the P1 backup and redo without it.
2. **In-flight work arcs** — episodes belonging to work still in progress
   stay unconsolidated (judgement).
3. **Hot recent episodes** — access count > 0 AND < 7 days old stay
   unconsolidated (judgement).

## P8 — Prune

`cerebro gc` eviction is archive-then-delete: content survives in
`nodes_archive`, but the embedding, FTS row, edges, and node identity do
not — re-adding mints a new id and orphans provenance. The P1 backup is the
only full-fidelity rollback. So the step is bounded three ways:

1. **Threshold bound:** if P0's effective `gc_threshold` exceeds 0.05, SKIP
   this step and surface it to the operator. Do not proceed.
2. **Dry-run bound:** dry-run first and count:

```bash
cerebro gc -p "$BRAIN" --dry-run --format json | jq '.archived'
```

   If the count exceeds `max(10, 2% of active_nodes from /tmp/dream-stats-before.json)`,
   ABORT the gc step and surface the eviction list to the operator. Do not
   proceed.
3. **Flag bound:** the real run is bare — never pass `--threshold`:

```bash
cerebro gc -p "$BRAIN"
```

Review the dry-run's eviction list before the real run; anything that looks
wrong is a reason to stop, not to tune the threshold.

## P9 — Eval AFTER, outcomes, report

On a corpus-backed brain, run the after leg and judge the **same-session
pair** — never the committed `baseline.json`, which drifts:

```bash
cerebro eval -p "$BRAIN" --out /tmp/dream-eval-after.json
jq '{metrics, brain}' /tmp/dream-eval-before.json /tmp/dream-eval-after.json
```

**Non-regression rule:** each of `recall@5`, `recall@10`, `recall@20`, and
`MRR` must be ≥ its before-leg value − 0.001. The tolerance absorbs embedder
variance only. If any metric fails AND the two legs' `active_nodes` differ
(another session wrote mid-pass), the pair is contaminated — re-run both eval
legs back-to-back before concluding regression. A confirmed regression means:
investigate, and restore from the P1 backup if needed.

Close the loop on the memories that guided the pass:

```bash
cerebro outcome <id> -p "$BRAIN" --success   # it helped
cerebro outcome <id> -p "$BRAIN" --failure   # it misled
```

Final state and report:

```bash
cerebro stats -p "$BRAIN" --format json > /tmp/dream-stats-after.json
```

The pass report records: the P1 backup path; stats before/after; episodes
consolidated (delta of `consolidated_nodes`); `total_edges` delta; density =
`total_edges / active_nodes` (and `/ total_nodes`) before vs after; the eval
pair verdict (or "stats-only pass"); inbox decisions; exclusions honored;
outcomes recorded.
