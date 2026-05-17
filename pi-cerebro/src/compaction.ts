/**
 * @file compaction.ts
 * @ticket HS-010
 *
 * Heuristic compaction detector for pi-cerebro.
 *
 * Watches the count of Pi session entries between message_end ticks and fires
 * when entries drop by more than 50% in a single observation — the heuristic
 * proxy for a context compaction event until Pi exposes `session_compact`
 * natively (see §10 of the Architect Design for the future fold-in path).
 *
 * THRESHOLD SEMANTICS
 * `COMPACTION_DROP_RATIO = 0.5` means "more than half the entries vanished in
 * one tick". Strict `>` (not `>=`) — exactly 50% does NOT fire.  This matches
 * the ticket AC wording "more than 50%" and Operational Ontology §5.5 Q1.
 *
 * STATE MANAGEMENT
 * `lastSeen: number | null = null` is module-scope (not closure-in-handler) so
 * that the baseline survives across `piCerebro()` factory calls in tests.
 * Test isolation relies on __resetCompactionStateForTests() being called at
 * the top of every test (Tech Lead N1 — jiti module-cache survives per file).
 */

// ---------------------------------------------------------------------------
// Threshold constant (DoD bullet 3: named constant, not a magic number)
// ---------------------------------------------------------------------------

/**
 * Fraction-of-previous-length drop that triggers a re-prime.
 * >0.5 means more than half the entries vanished in one observation.
 * Per Operational Ontology §5.5 Decision Q1; satisfies DoD bullet 3
 * (no magic number — threshold lives here, not in any call site).
 */
export const COMPACTION_DROP_RATIO = 0.5;

// ---------------------------------------------------------------------------
// Module-scope state
// ---------------------------------------------------------------------------

/** Last observed entries length. null means no observation yet (fresh state). */
let lastSeen: number | null = null;

// ---------------------------------------------------------------------------
// Pure decision helper (testable without Pi)
// ---------------------------------------------------------------------------

/**
 * Returns true if a drop from `previous` to `current` exceeds the threshold.
 *
 * Edge cases handled:
 * - `previous <= 0`: divide-by-zero guard — returns false (new session).
 * - `current >= previous`: growth or steady-state — returns false.
 * - Exactly 50% drop: returns false (strict `>`, not `>=`).
 *
 * @param previous  The entries length from the prior observation.
 * @param current   The entries length from the current observation.
 */
export function isCompactionDrop(previous: number, current: number): boolean {
  if (previous <= 0) return false;       // edge: divide-by-zero guard
  if (current >= previous) return false; // edge: growth or steady
  const dropRatio = (previous - current) / previous;
  return dropRatio > COMPACTION_DROP_RATIO;
}

// ---------------------------------------------------------------------------
// Stateful tick observer
// ---------------------------------------------------------------------------

/**
 * Observe a new entries-length sample and update the internal baseline.
 *
 * On the first call (lastSeen === null), the value arms the baseline without
 * firing — this prevents a false compaction signal on session start.
 *
 * @param current  The entries length from ctx.sessionManager.getEntries().length.
 * @returns `{ fired: boolean }` — true if the compaction threshold was crossed.
 */
export function observeEntriesLength(current: number): { fired: boolean } {
  if (lastSeen === null) {
    // First observation: prime the baseline, do NOT fire.
    lastSeen = current;
    return { fired: false };
  }

  const fired = isCompactionDrop(lastSeen, current);
  lastSeen = current;
  return { fired };
}

// ---------------------------------------------------------------------------
// Test isolation
// ---------------------------------------------------------------------------

/**
 * Reset module state for test isolation.
 *
 * MUST be called at the top of every test in compaction.test.mjs to clear
 * the `lastSeen` baseline. node:test has no global beforeEach and jiti module
 * caching means state survives across tests in the same file.
 *
 * Production code must never call this.
 *
 * @internal
 */
export function __resetCompactionStateForTests(): void {
  lastSeen = null;
}
