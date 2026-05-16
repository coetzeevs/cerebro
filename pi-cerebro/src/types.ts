/**
 * @file types.ts
 * @ticket HS-009
 *
 * TypeBox schemas and plain interfaces for the pi-cerebro extension.
 *
 * Why TypeBox?
 * Pi's registerTool() requires a TypeBox TSchema for parameter validation.
 * We use the bare `typebox` package (not `@sinclair/typebox`) per the
 * pi-claude-code-cli precedent (src/index.ts:3) and Tech Lead note N1.
 *
 * All schemas are defined here and shared between src/index.ts (tool
 * registration) and tests (schema assertions).
 */

import { Type } from "typebox";

// ---------------------------------------------------------------------------
// cerebro_recall tool parameter schema
// ---------------------------------------------------------------------------

/**
 * Input schema for the cerebro_recall Pi tool.
 *
 * `query`     — Free-text semantic query (passed verbatim to `cerebro recall`).
 * `projectDir` — Absolute path to the project dir for the -p flag.
 *               Must come from CLAUDE_PROJECT_DIR (captured at session_start).
 *               Falls back to process.env.CLAUDE_PROJECT_DIR when the session
 *               capture is absent — never empty string (guarded by caller).
 * `limit`     — Max nodes returned. Defaults to 10.
 */
export const RecallParamsSchema = Type.Object({
  query: Type.String({ minLength: 1, description: "Semantic query to recall memories" }),
  projectDir: Type.Optional(Type.String({ description: "Project dir override (internal use)" })),
  limit: Type.Optional(Type.Number({ minimum: 1, maximum: 100, description: "Max results (default 10)" })),
});

// ---------------------------------------------------------------------------
// cerebro_remember tool parameter schema
// ---------------------------------------------------------------------------

/**
 * Input schema for the cerebro_remember Pi tool.
 *
 * `content`   — Text to store as a new memory node.
 * `type`      — Node type: episode | concept | procedure | reflection.
 * `projectDir` — Absolute path to the project dir for the -p flag.
 * `importance` — Float 0.0–1.0. Defaults to 0.75.
 */
export const RememberParamsSchema = Type.Object({
  content: Type.String({ minLength: 1, description: "Content to persist in memory" }),
  type: Type.Optional(
    Type.Union([
      Type.Literal("episode"),
      Type.Literal("concept"),
      Type.Literal("procedure"),
      Type.Literal("reflection"),
    ], { description: "Node type (default: episode)" })
  ),
  importance: Type.Optional(Type.Number({ minimum: 0, maximum: 1, description: "Importance weight 0.0–1.0 (default 0.75)" })),
  projectDir: Type.Optional(Type.String({ description: "Project dir override (internal use)" })),
});

// ---------------------------------------------------------------------------
// Internal result types (not Pi-facing schema — plain interfaces)
// ---------------------------------------------------------------------------

/** Result shape returned from runRecall. */
export interface RecallResult {
  stdout: string;
  exitCode: number | null;
}

/** Result shape returned from runAdd. */
export interface AddResult {
  nodeId: string | null;
  exitCode: number | null;
}

/** Result shape returned from runBootPrime. */
export interface BootPrimeResult {
  /** Number of memories primed (parsed from output or counted from lines). */
  primedCount: number;
  stdout: string;
}
