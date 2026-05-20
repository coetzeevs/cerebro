/**
 * @file session-context-reader.ts
 * @ticket HS-039, HS-046
 *
 * Cross-extension read helper: reads the calling agent's `currentBeadsId` slot
 * from Meepo's session-context substrate (subagents.db) and returns it for
 * tagging memories with the active beads task id.
 *
 * WHY DIRECT SQLite READ (not a shared package or MCP call)?
 * pi-cerebro cannot import Meepo's session-context.ts directly (different repo,
 * no published npm package). Instead we open the DB read-only and issue a single
 * parameterised SELECT — the same logic as meepo/extensions/tmux-agents/session-context.ts
 * getSlot(), but from the consumer side. This is the "direct SQLite read" approach
 * chosen in HS-039 §3 (Option i) for symmetry with HS-037/HS-038.
 *
 * AGENT IDENTITY
 * Meepo sets process.env.PI_TMUX_AGENTS_CHILD_ID for child agents (per
 * child-runtime.ts:351-356). Top-level agents have no such env var.
 * Resolution: `process.env["PI_TMUX_AGENTS_CHILD_ID"]?.trim() || null`
 * (`||` not `??` per HS-009 empty-string guard convention; memory 00bd0076).
 * If null → no agentId → skip read → null (AC3 back-compat).
 *
 * DB PATH
 * `getAgentDir() + "/subagents.db"` per paths.ts:36 / config.js:359.
 * `getAgentDir()` respects PI_CODING_AGENT_DIR env var override — used
 * by tests to inject a temp DB without touching the real subagents.db.
 *
 * BEST-EFFORT POLICY
 * Any error (DB missing, schema mismatch, query throws, no row) → log a single
 * sanitised warning via console.warn and return null. cerebro_remember MUST NOT
 * fail because of session-context unavailability (N-S4 / HS-039 §3).
 * Sanitise via the existing sanitise() helper (200-char cap, ASCII ctrl strip).
 *
 * WRITE BOUNDARY
 * This module is READ-ONLY. It exports NO setSlot or write helpers. Writes to
 * session_contexts are owned by slot-owner extensions only (pi-beads via HS-037).
 *
 * BOUNDARY ENFORCEMENT (HS-046)
 * enforceCerebroBoundBeadsId() is the memory-write gate: if the calling agent's
 * session has a currentBeadsId binding (non-null from readCurrentBeadsId()) AND
 * the effective beadsId for the write is null or empty, it throws a hard error
 * with CEREBRO_BEADS_ENFORCEMENT_ERROR. This mirrors HS-045's enforceBoundBeadsId
 * at the Meepo MCP boundary.
 *
 * TRUST-SPLIT NOTE (S-N1 / OO-014 §7.1)
 * HS-029 regex is validated at the writer boundary (HS-037 pi-beads + TypeBox in
 * HS-039). This module trusts the substrate: it reads what was written and does
 * NOT re-validate the beadsId format on the read path. The enforcement predicate
 * cares only about presence/absence (null vs non-null), not format.
 *
 * @returns The currentBeadsId for the calling agent, or null if unavailable.
 */

import { DatabaseSync } from "node:sqlite";
import { join } from "node:path";
import { getAgentDir } from "@earendil-works/pi-coding-agent";
import { sanitise } from "./cerebro-cli.js";

/**
 * Read the calling agent's currentBeadsId from Meepo's session-context substrate.
 *
 * Best-effort: returns null on any error (DB missing, env unset, no row,
 * schema mismatch). cerebro_remember must succeed regardless.
 *
 * Agent identity: resolved from process.env.PI_TMUX_AGENTS_CHILD_ID
 * (set by Meepo child-runtime.ts:351-356). Top-level agents (no Meepo parent)
 * have no env var → null → no beadsId tagging (AC3 back-compat).
 *
 * N-S2 (TL-N9): env var read uses || not ?? so empty string is treated as absent.
 *
 * @returns The currentBeadsId string, or null if unavailable.
 */
// N-S2: agentId is used as a SQL parameter (no injection risk), but we still
// length-cap and alphabet-restrict to prevent oversized or malformed values
// from reaching the DB. Regex [A-Za-z0-9_-]{1,256} matches Meepo's agent-id
// format (childRuntimeEnvironment.childId) and caps at 256 chars.
const AGENT_ID_PATTERN = /^[A-Za-z0-9_-]{1,256}$/;

export function readCurrentBeadsId(): string | null {
  // TL-N9: || guard catches empty string (not just undefined/null)
  const rawAgentId = process.env["PI_TMUX_AGENTS_CHILD_ID"]?.trim() || null;
  if (!rawAgentId) return null;

  // N-S2: length-cap + alphabet restriction — best-effort sanitise
  if (!AGENT_ID_PATTERN.test(rawAgentId)) {
    console.warn(
      `[pi-cerebro] PI_TMUX_AGENTS_CHILD_ID format unexpected (best-effort, no beadsId tag): ${sanitise(rawAgentId)}`
    );
    return null;
  }
  const agentId = rawAgentId;
  if (!agentId) return null;

  let db: DatabaseSync | null = null;
  try {
    const dbPath = join(getAgentDir(), "subagents.db");
    // TL-N6 / N-S3: read-only open — pi-cerebro must never write to Meepo's DB
    db = new DatabaseSync(dbPath, { readOnly: true });
    const row = db
      .prepare(
        "SELECT slot_value FROM session_contexts WHERE agent_id = ? AND slot_name = ?"
      )
      .get(agentId, "currentBeadsId") as { slot_value: string | null } | undefined;
    return row?.slot_value ?? null;
  } catch (err) {
    // Best-effort: sanitise error message before logging (N-S4 / HS-039 §3)
    const msg = err instanceof Error ? sanitise(err.message) : "unknown error";
    console.warn(
      `[pi-cerebro] session-context read failed (best-effort, no beadsId tag): ${msg}`
    );
    return null;
  } finally {
    try {
      db?.close();
    } catch {
      // ignore close errors
    }
  }
}

// ---------------------------------------------------------------------------
// HS-046 Boundary enforcement helpers
// Sibling to HS-045's enforceBoundBeadsId at the Meepo MCP boundary.
// These are INTERNAL to pi-cerebro — not part of the Pi-facing tool API (S-N4).
// ---------------------------------------------------------------------------

/**
 * Static error template for the memory-write boundary enforcement gate.
 *
 * AC2 exact literal (OO-014 §7.1, HS-046). The `<id>` placeholder is
 * substituted by formatCerebroBeadsEnforcementError(). No template literals —
 * static String.replace() only, preserving testable byte-identity.
 *
 * TRUST-SPLIT (S-N1): format validated at writer (HS-037/TypeBox). This
 * const is message-only; it does NOT re-validate the id.
 *
 * bd_clear is forward-looking — see HS-050 backlog (TL-PI-N5).
 *
 * @internal HS-046
 */
export const CEREBRO_BEADS_ENFORCEMENT_ERROR =
  "Cannot create unlinked memory: session is bound to bd ticket <id>. " +
  "Pass beadsId explicitly OR call bd_clear to release the session binding.";

/**
 * Produce the enforcement error message for the given bound beadsId.
 *
 * Uses String.replace() on the `<id>` placeholder in CEREBRO_BEADS_ENFORCEMENT_ERROR.
 * No template literals. No branches. Mirrors HS-045's formatBeadsEnforcementError.
 *
 * @internal HS-046
 * @param currentBeadsId - The binding id returned by readCurrentBeadsId().
 * @returns Formatted error string with `<id>` replaced by currentBeadsId.
 */
export function formatCerebroBeadsEnforcementError(currentBeadsId: string): string {
  return CEREBRO_BEADS_ENFORCEMENT_ERROR.replace("<id>", currentBeadsId);
}

/**
 * Boundary-enforcement gate for the cerebro_remember memory-write path.
 *
 * Throws if BOTH conditions hold:
 *   1. The effective beadsId for the write is null or empty (TL-PI-N1 / S-N8:
 *      `effectiveBeadsId == null || effectiveBeadsId === ""`).
 *   2. The calling agent's session has a non-null currentBeadsId binding
 *      (readCurrentBeadsId() returns non-null).
 *
 * Unbound sessions (readCurrentBeadsId() returns null) pass through unchanged —
 * AC3 back-compat (HS-039) is fully preserved.
 *
 * DOUBLE-READ NOTE (TL-PI-N2): readCurrentBeadsId() performs a SQLite read.
 * In the cerebro_remember execute handler this means the DB is read twice:
 * once at line `const beadsId = readCurrentBeadsId()` to resolve the effective
 * beadsId, and once here inside enforceCerebroBoundBeadsId() for the binding
 * check. Both reads target the same session_contexts row; the second read is
 * an intentional design choice to keep enforcement logic self-contained and
 * avoid passing the binding as a parameter (which would require callers to
 * track it separately). The gate is called BEFORE runAdd so any binding
 * mismatch throws BEFORE any subprocess is launched (S-N2: block not parameterise).
 *
 * NEVER-THROWS NOTE (TL-PI-N6): readCurrentBeadsId() is best-effort and
 * never throws to the caller — all errors inside it are caught and return null
 * with a sanitised console.warn. enforceCerebroBoundBeadsId() inherits this
 * guarantee: if the DB read fails, binding = null and no enforcement fires.
 *
 * OWASP A03/A04/A05 (S-N5): no injection surface (predicate-only, no SQL in
 * this function), no privilege escalation, gate fires before subprocess launch.
 *
 * @internal HS-046
 * @param effectiveBeadsId - The resolved beadsId for this write (params.beadsId
 *   ?? readCurrentBeadsId() resolved in the execute handler). May be null or "".
 * @throws Error with formatCerebroBeadsEnforcementError message when gate fires.
 */
export function enforceCerebroBoundBeadsId(
  effectiveBeadsId: string | null | undefined
): void {
  // TL-PI-N1 / S-N8: twin-check — null AND empty-string both count as "absent"
  if (effectiveBeadsId != null && effectiveBeadsId !== "") return;

  // Gate only fires when session has an active binding
  const binding = readCurrentBeadsId();
  if (binding == null) return;

  throw new Error(formatCerebroBeadsEnforcementError(binding));
}
