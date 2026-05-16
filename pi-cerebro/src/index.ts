/**
 * @file index.ts
 * @ticket HS-009
 *
 * Pi extension entry point for pi-cerebro.
 *
 * Registers two LLM-callable tools:
 *   - `cerebro_recall`  — semantic memory recall via `cerebro recall`
 *   - `cerebro_remember` — persist a new memory via `cerebro add`
 *
 * And one lifecycle hook:
 *   - `session_start` — runs `cerebro recall --boot` to prime recent memories
 *     into the agent's context at session start.
 *
 * WHY FAIL-FAST?
 * If the `cerebro` binary is absent or fails the stale-shim check,
 * validateCerebroCli() throws and we catch it here: tools are NOT registered,
 * a clear error is printed to stderr, and Pi continues without cerebro
 * capability. Silently registering broken tools would produce confusing LLM
 * failures mid-session.
 *
 * CLAUDE_PROJECT_DIR FALLBACK
 * We read the project dir via `||` (not `??`) so that an empty string is
 * treated as absent. This prevents an empty-string projectDir from being
 * passed as -p "" to cerebro (which would be invalid).
 */

import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";
import { Type } from "typebox";
import {
  getCerebroPath,
  initCerebroPath,
  runAdd,
  runBootPrime,
  runRecall,
  sanitise,
} from "./cerebro-cli.js";
import { RecallParamsSchema, RememberParamsSchema } from "./types.js";

// ---------------------------------------------------------------------------
// Module init — resolve + validate the cerebro binary once at load time.
// If this fails, no tools are registered (fail-fast, Tech Lead N3).
// ---------------------------------------------------------------------------
initCerebroPath();

// ---------------------------------------------------------------------------
// Helper: resolve the project dir from session context or environment.
// Uses || guard (not ??) to catch empty-string (Security CLAUDE_PROJECT_DIR).
// ---------------------------------------------------------------------------
function resolveProjectDir(overrideFromParams?: string): string | null {
  // Prefer explicit override (for future testability / advanced use)
  const fromParams = overrideFromParams || null;
  if (fromParams) return fromParams;

  // Fall back to CLAUDE_PROJECT_DIR env var.
  // || catches both undefined and empty string — per Security reviewer note.
  const fromEnv = process.env["CLAUDE_PROJECT_DIR"] || null;
  return fromEnv;
}

// ---------------------------------------------------------------------------
// Extension default export — Pi calls this with the ExtensionAPI instance
// ---------------------------------------------------------------------------
export default function piCerebro(pi: ExtensionAPI): void {
  const binary = getCerebroPath();
  if (!binary) {
    // Binary absent or stale-shim detected at init time.
    // Error already logged in initCerebroPath(). Register nothing.
    return;
  }

  // -------------------------------------------------------------------------
  // Tool: cerebro_recall
  // -------------------------------------------------------------------------
  pi.registerTool({
    name: "cerebro_recall",
    label: "Cerebro Recall",
    description:
      "Recall relevant memories from cerebro memory storage using semantic search. " +
      "Returns markdown-formatted memory nodes ranked by relevance.",
    parameters: RecallParamsSchema,

    async execute(_toolCallId, params, _signal, _onUpdate, _ctx) {
      // CLAUDE_PROJECT_DIR guard: || catches empty string (Security reviewer note)
      const projectDir = resolveProjectDir(params.projectDir);
      if (!projectDir) {
        const msg = "[pi-cerebro] No project directory available. Ensure CLAUDE_PROJECT_DIR is set.";
        return {
          content: [{ type: "text" as const, text: msg }],
          details: { success: false },
        };
      }

      const limit = params.limit ?? 10;

      try {
        const { stdout } = runRecall(params.query, projectDir, limit, binary);
        const text = stdout || "(no memories found)";
        return {
          content: [{ type: "text" as const, text }],
          details: { success: true },
        };
      } catch (err) {
        const msg = err instanceof Error ? sanitise(err.message) : "unknown error";
        return {
          content: [{ type: "text" as const, text: msg }],
          details: { success: false },
        };
      }
    },
  });

  // -------------------------------------------------------------------------
  // Tool: cerebro_remember
  // -------------------------------------------------------------------------
  pi.registerTool({
    name: "cerebro_remember",
    label: "Cerebro Remember",
    description:
      "Persist a new memory in cerebro storage. Use this to save important facts, " +
      "decisions, patterns, or observations that should persist across sessions.",
    parameters: RememberParamsSchema,

    async execute(_toolCallId, params, _signal, _onUpdate, _ctx) {
      const projectDir = resolveProjectDir(params.projectDir);
      if (!projectDir) {
        const msg = "[pi-cerebro] No project directory available. Ensure CLAUDE_PROJECT_DIR is set.";
        return {
          content: [{ type: "text" as const, text: msg }],
          details: { success: false },
        };
      }

      const type = params.type ?? "episode";
      const importance = params.importance ?? 0.75;

      try {
        const { nodeId } = runAdd(params.content, projectDir, type, importance, binary);
        const idStr = nodeId ? ` (id: ${nodeId})` : "";
        const text = `Memory stored${idStr}.`;
        return {
          content: [{ type: "text" as const, text }],
          details: { success: true, nodeId },
        };
      } catch (err) {
        const msg = err instanceof Error ? sanitise(err.message) : "unknown error";
        return {
          content: [{ type: "text" as const, text: msg }],
          details: { success: false },
        };
      }
    },
  });

  // -------------------------------------------------------------------------
  // Hook: session_start — boot-prime recent memories into agent context
  // -------------------------------------------------------------------------
  pi.on("session_start", async (_event) => {
    const projectDir = resolveProjectDir();
    if (!projectDir) {
      console.warn("[pi-cerebro] session_start: no CLAUDE_PROJECT_DIR — skipping boot prime.");
      return;
    }

    try {
      const { primedCount } = runBootPrime(projectDir, binary);
      console.error(`[pi-cerebro] session_start: primed ${primedCount} memories.`);
    } catch (err) {
      const msg = err instanceof Error ? sanitise(err.message) : "unknown error";
      console.error(`[pi-cerebro] session_start: boot prime failed — ${msg}`);
    }
  });
}
