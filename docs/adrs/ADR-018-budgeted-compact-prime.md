# ADR-018: The session-start prime is a budgeted compact index (progressive disclosure)

**Status:** Accepted (on merge)
**Date:** 2026-10-12
**Ticket:** agentic-thnd
**Amends:** ADR-006 (Startup / session-start flow), ADR-007 (§1b `last_surfaced` behaviour)

## Context

Claude Code caps each hook `additionalContext` string, and plain hook stdout, at
10,000 characters. Over the cap it saves the output to a file and passes Claude
the file path plus a preview of up to the first 2,000 characters; Claude Code
does not ask Claude to read the file, and no setting raises the cap
(https://code.claude.com/docs/en/hooks, size limits on hook output).

The prime was rendered as full memory content with no budget. On copies of the
live brains (2026-10-12) the estate prime (`prime_limit=100`) rendered 274,980
characters and the agentic prime (`prime_limit=20`) 29,518, so both overflowed
and the model saw only the 2,000-character preview. Worse, `hook prime`
re-executed `recall --prime`, whose child process stamped `last_surfaced` on
every selected memory before the parent saw the text. The stamps recorded a
delivery that never happened and suppressed those memories' surprise term in
later primes (`PrimeScore = 0.6·importance + 0.4·Surprise`).

## Decision

1. The prime renders in-process as a framed, one-line-per-memory index under
   `prime_budget` (default 9500 bytes, range [2000, 10000]), measured in UTF-8
   bytes after sanitisation. UTF-8 bytes ≥ UTF-16 code units ≥ code points, so
   the byte budget also bounds whatever "character" count Claude Code uses.
   Lines are added in selection order and rendering stops at the first line
   that does not fit, so the delivered set is a prefix of the selection.
2. Full content is pulled on demand with `cerebro get <id>`.
3. `last_surfaced` and the session `primed` sentinel are written only for the
   delivered ids. "Surfaced" now means "id and headline delivered in the
   emitted briefing".
4. Records sit inside a fixed `<cerebro-memories>` frame whose header states
   they are data, not instructions. Stored content cannot forge the frame:
   each record is a single line, every ASCII/fullwidth/small-form angle
   bracket in a headline becomes a guillemet, and control, format (Cf) and
   variation-selector runes are dropped. Records whose id does not start with
   8 lowercase hex characters are skipped, never rendered or stamped.
5. The headline is deterministic: the first non-blank line of the first
   4,096 bytes, sanitised and rune-truncated with an ellipsis. No LLM (Model B).
6. `recall --compact [--budget N]` exposes the same renderer (`prime_budget`
   with `--prime` and no query, `recall_budget` otherwise). Full mode is
   unchanged.
7. The GC eviction notice appended to the prime carries only cerebro-generated
   tokens (count, timestamp, up to five 8-hex ids), is capped at 320 bytes,
   reads at most the last 64 KiB of `.cerebro-gc.log`, and sits after the
   frame footer. It previously injected the raw last log line, which carried
   the full content of every evicted memory.

## Evidence

- Source: https://code.claude.com/docs/en/hooks (JSON output, size-limit
  bullets: `additionalContext`, `systemMessage` and plain stdout capped at
  10,000 characters; overflow becomes a file path plus a 2,000-character
  preview; no setting raises the cap).
- Live, before (Design baseline, 2026-10-12): full hook `additionalContext`
  274,980 characters (estate copy) and 29,518 (agentic copy).
- Live, after (branch binary on fresh read-only copies, 2026-10-12): estate
  compact prime 8,951 bytes with 100 of 100 memories delivered (full
  `recall --prime` 262,248 bytes); agentic 4,123 bytes with 20 of 20 (full
  27,620 bytes).
- UTF-8 bytes ≥ UTF-16 units: 0 violations over 200,000 random strings
  (Design E2); the budget invariant is asserted by table, property and
  `FuzzRenderCompactIndex` tests in bytes and UTF-16 units.

## Consequences

- The prime reaches the model instead of a 2,000-character preview.
- The surprise signal stays truthful: only delivered memories are stamped.
- The agent sees headlines, not bodies. A change below a memory's first line
  no longer re-surprises once its headline was delivered. Accepted: the body
  is one `cerebro get <id>` away.
- The hook-prime output contract changed (banner wording, frame, one line per
  memory); the two render tests that asserted the old contract were rewritten.
- Whether a model honours the data frame, or drills down with `get`, is agent
  behaviour and not guaranteed. The frame signposts untrusted content (OWASP
  LLM01 mitigation) but cannot prevent memory poisoning.

## Alternatives considered

- **Keep the re-exec and pass a budget to the child.** Rejected: the parent's
  banner and GC overhead need config the parent does not read, and two
  processes would split one invariant (stamp only what was delivered).
- **Lower `prime_limit`.** Rejected by operator ruling `cc8abcb3`, and
  insufficient: the agentic brain overflows at `prime_limit=20`.
- **Fixed-width headline.** Rejected: the Design simulation delivered 70/100
  at 100 runes against 98/100 with an adaptive width.
- **Agent-written summary field set at add time.** Deferred: it needs a schema
  change and no acceptance criterion requires it.
