# @coetzeevs/pi-cerebro

Pi extension for [cerebro](https://github.com/coetzeevs/cerebro) memory integration.

## What it does

Registers two LLM-callable tools and one session lifecycle hook in the Pi coding agent:

| Surface | Description |
|---|---|
| `cerebro_recall` | Semantic memory search via `cerebro recall` |
| `cerebro_remember` | Persist a new memory via `cerebro add` |
| `session_start` hook | Boot-primes recent memories into context via `cerebro recall --boot` |

## Requirements

- Node.js >= 22.6.0
- `cerebro` CLI binary on PATH (install from [github.com/coetzeevs/cerebro](https://github.com/coetzeevs/cerebro/releases))
- Pi coding agent (`@earendil-works/pi-coding-agent`)

## Configuration

The extension reads `CLAUDE_PROJECT_DIR` (set by Pi at session start) to determine which cerebro brain to query. It uses `cerebro -p $CLAUDE_PROJECT_DIR` for all operations.

`pi-init` snippet (emitted by `cerebro pi-init -p <dir>`):

```json
{
  "extensions": [
    {
      "name": "pi-cerebro",
      "package": "@coetzeevs/pi-cerebro",
      "options": {
        "projectDir": "<your project path>"
      }
    }
  ]
}
```

## Compaction detection (heuristic)

pi-cerebro registers a `message_end` hook that watches
`ctx.sessionManager.getEntries().length` on every turn boundary. When the
count drops by **more than 50%** in a single tick, the detector treats this as
a probable context compaction and re-primes memories.

### Threshold

```typescript
export const COMPACTION_DROP_RATIO = 0.5; // src/compaction.ts
```

Strict `>` (not `>=`): an exactly 50% drop does NOT fire. Per Operational
Ontology §5.5 Decision Q1.

### Log-line contract (HS-016 binding)

On detection, this exact substring is emitted to stderr:

```
[pi-cerebro] compaction detected: re-priming memories
```

`HS-016/validate-cerebro-pi.sh` greps for this verbatim (`grep -F`). The
substring is a compile-time static literal — no path or session-state values
are interpolated into it.

### Cascading drops

Consecutive drops (e.g. 100 → 40 → 15) re-fire on each tick. This is
deliberate: each tick re-evaluates from the most recent `lastSeen` value, so
a 40→15 drop (62%) is itself a compaction signal. The idempotent `cerebro
recall --boot` call is safe to repeat; deduplication is deferred until Pi
exposes `session_compact` natively (Architect Design §10 future fold-in).

### Future fold-in

Pi v0.x exposes `session_compact` and `session_before_compact` events natively
(`types.d.ts:402, 410`). Once HS-010 + HS-016 stabilise, a follow-up ticket
can replace the heuristic with the native event while preserving the log-line
contract above.

## Fail-fast behaviour

At extension load time, pi-cerebro resolves `cerebro` from PATH once and validates its version output matches `/\d+\.\d+/`. If the binary is absent or fails validation (stale shim), the extension logs an error to stderr and **registers nothing** — Pi continues without cerebro capability rather than crashing.

## Security notes

- All subprocess invocations use **argv-array form** (`execFileSync(cmd, [...args])`) — never `shell: true` or string concatenation. This eliminates shell injection risk even if user-supplied query strings contain special characters.
- **Stale-shim defence**: the resolved binary must produce version output matching `/\d+\.\d+/`. A shim that exits 0 with empty output is rejected at init time (defence-in-depth against planted binaries on PATH).
- **Bounded nodeId capture**: the `cerebro_remember` response captures the new node ID via `/^([0-9a-f]{8,64})/m` — bounded hex, anchored at line start — to prevent injection from unexpected subprocess output.
- **sanitise()**: all external subprocess output (stdout, stderr, error messages) is passed through `sanitise()` before embedding in log messages or tool results. Strips ASCII control characters and caps at 200 chars.
- **CLAUDE_PROJECT_DIR guard**: the project dir is read via `||` (not `??`) so that an empty string is treated as absent, the same as undefined. An empty-string projectDir would silently pass `??` and reach cerebro as `-p ""` (invalid).
- **PATH-trust posture**: HS-009 resolves `cerebro` from PATH once at module init. Absolute-path caching (HS-022-style) is deferred to HS-022 scope per both-reviewer scope-expansion rule.

## Test infrastructure

Tests use `node:test` (ADR-0001 stack-frame default). Stub binaries in `tests/fixtures/` are invoked via PATH prepend so production code exercises the same `which cerebro` + `execFileSync` path that production does.

```bash
npm test         # run all tests
npm run typecheck  # TypeScript strict check (noEmit)
```

## Contributing

See [CONTRIBUTING.md](../CONTRIBUTING.md) in the cerebro repository root.
