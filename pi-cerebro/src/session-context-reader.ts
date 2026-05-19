/**
 * @file session-context-reader.ts
 * @ticket HS-039
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
