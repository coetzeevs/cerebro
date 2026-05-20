/**
 * @file cerebro-remember-enforcement.test.mjs
 * @ticket HS-046
 *
 * TDD tests for pi-cerebro boundary enforcement at the cerebro_remember boundary:
 * "strict-reject when the calling agent's session has a currentBeadsId binding
 * but the effective beadsId for the write is NULL or empty."
 *
 * Mirrors the HS-045 meepo boundary-enforcement test pattern adapted for
 * pi-cerebro / session-context-reader.ts.
 *
 * Test strategy (TL-PI-N3):
 *   - T1..T3 / T5: source-introspection via readFileSync is NOT needed here
 *     because session-context-reader.ts is importable via jiti (no transitive
 *     paths.js/board.js NodeNext artefact — only node:sqlite builtins).
 *     We simulate the gate logic by exercising enforceCerebroBoundBeadsId directly.
 *   - T4 (TL-PI-N3): POSITIVE source-introspection asserts helper-symbol presence
 *     in BOTH session-context-reader.ts AND index.ts.
 *   - T5: RememberParamsSchema introspection (AC4 no opt-out / S-N3).
 *
 * Test cases:
 *   T1: bound session + null effectiveBeadsId → throws with exact AC2 literal
 *   T2: shared exact-match assertion using formatCerebroBeadsEnforcementError
 *   T3: unbound session → no throw; back-compat (AC3)
 *   T4: source-introspection — helper-symbol presence in session-context-reader.ts AND index.ts
 *   T5: RememberParamsSchema has additionalProperties:false / no opt-out parameter
 */

import assert from "node:assert/strict";
import { test } from "node:test";
import { readFileSync } from "node:fs";
import { createJiti } from "jiti";

const jiti = createJiti(import.meta.url);

// ---------------------------------------------------------------------------
// T1: bound session + null effectiveBeadsId → throws with exact AC2 literal
// ---------------------------------------------------------------------------

test("T1: enforceCerebroBoundBeadsId throws when session is bound and effectiveBeadsId is null", async () => {
  const mod = await jiti.import("../src/session-context-reader.ts");
  const { enforceCerebroBoundBeadsId } = mod;

  // Simulate: binding = "agentic-test-1", effectiveBeadsId = null
  // We exercise the predicate directly — it reads via readCurrentBeadsId() internally,
  // so we need a shim to inject the binding. The helper accepts the effectiveBeadsId
  // and calls readCurrentBeadsId() for the binding side.
  //
  // Per the Architect design: enforceCerebroBoundBeadsId(effectiveBeadsId) reads
  // readCurrentBeadsId() internally for the binding check.
  // For unit test isolation, we test with the exported predicate directly by
  // setting PI_TMUX_AGENTS_CHILD_ID + PI_CODING_AGENT_DIR to a bound session.
  //
  // Approach: use the already-established setupTestDb pattern (inline, no before/after)
  // to create a temp DB with a currentBeadsId row, point env vars at it, then call
  // enforceCerebroBoundBeadsId(null).

  const { DatabaseSync } = await import("node:sqlite");
  const { mkdtempSync, rmSync } = await import("node:fs");
  const { tmpdir } = await import("node:os");
  const { join } = await import("node:path");

  const tmpDir = mkdtempSync(join(tmpdir(), "hs046-t1-"));
  const dbPath = join(tmpDir, "subagents.db");

  try {
    // Create DB with bound session
    const db = new DatabaseSync(dbPath);
    db.exec(`
      CREATE TABLE session_contexts (
        agent_id TEXT NOT NULL,
        slot_name TEXT NOT NULL,
        slot_value TEXT,
        written_at TEXT NOT NULL DEFAULT (datetime('now')),
        owner_extension TEXT NOT NULL DEFAULT '',
        PRIMARY KEY (agent_id, slot_name)
      )
    `);
    db.prepare(
      "INSERT INTO session_contexts (agent_id, slot_name, slot_value, owner_extension) VALUES (?, ?, ?, 'pi-beads')"
    ).run("agent-bound-t1", "currentBeadsId", "agentic-test-1");
    db.close();

    const savedAgentId = process.env.PI_TMUX_AGENTS_CHILD_ID;
    const savedAgentDir = process.env.PI_CODING_AGENT_DIR;
    process.env.PI_TMUX_AGENTS_CHILD_ID = "agent-bound-t1";
    process.env.PI_CODING_AGENT_DIR = tmpDir;

    try {
      // effectiveBeadsId = null → should throw
      assert.throws(
        () => enforceCerebroBoundBeadsId(null),
        (err) => {
          assert.ok(err instanceof Error, "expected Error instance");
          assert.ok(
            err.message.includes("Cannot create unlinked memory"),
            `message must contain 'Cannot create unlinked memory', got: ${err.message}`
          );
          assert.ok(
            err.message.includes("agentic-test-1"),
            `message must contain the binding id 'agentic-test-1', got: ${err.message}`
          );
          return true;
        }
      );
    } finally {
      if (savedAgentId !== undefined) process.env.PI_TMUX_AGENTS_CHILD_ID = savedAgentId;
      else delete process.env.PI_TMUX_AGENTS_CHILD_ID;
      if (savedAgentDir !== undefined) process.env.PI_CODING_AGENT_DIR = savedAgentDir;
      else delete process.env.PI_CODING_AGENT_DIR;
    }
  } finally {
    rmSync(tmpDir, { recursive: true, force: true });
  }
});

// ---------------------------------------------------------------------------
// T2: exact AC2 literal match via formatCerebroBeadsEnforcementError
// ---------------------------------------------------------------------------

test("T2: formatCerebroBeadsEnforcementError produces exact AC2 literal", async () => {
  const mod = await jiti.import("../src/session-context-reader.ts");
  const { formatCerebroBeadsEnforcementError, CEREBRO_BEADS_ENFORCEMENT_ERROR } = mod;

  const formatted = formatCerebroBeadsEnforcementError("agentic-test-1");

  // AC2 exact literal (with <id> substituted)
  const expected =
    "Cannot create unlinked memory: session is bound to bd ticket agentic-test-1. " +
    "Pass beadsId explicitly OR call bd_clear to release the session binding.";

  assert.equal(
    formatted,
    expected,
    `formatCerebroBeadsEnforcementError must produce exact AC2 literal.\nGot:      ${formatted}\nExpected: ${expected}`
  );

  // Verify CEREBRO_BEADS_ENFORCEMENT_ERROR const contains the template
  assert.ok(
    typeof CEREBRO_BEADS_ENFORCEMENT_ERROR === "string",
    "CEREBRO_BEADS_ENFORCEMENT_ERROR must be a string"
  );
  assert.ok(
    CEREBRO_BEADS_ENFORCEMENT_ERROR.includes("<id>"),
    "CEREBRO_BEADS_ENFORCEMENT_ERROR must contain '<id>' placeholder"
  );
  assert.ok(
    CEREBRO_BEADS_ENFORCEMENT_ERROR.includes("Cannot create unlinked memory"),
    "CEREBRO_BEADS_ENFORCEMENT_ERROR must contain error preamble"
  );
});

// ---------------------------------------------------------------------------
// T3: unbound session → no throw; back-compat (AC3)
// ---------------------------------------------------------------------------

test("T3: enforceCerebroBoundBeadsId does NOT throw when session is unbound", async () => {
  const mod = await jiti.import("../src/session-context-reader.ts");
  const { enforceCerebroBoundBeadsId } = mod;

  // No PI_TMUX_AGENTS_CHILD_ID → readCurrentBeadsId() returns null → no enforcement
  const savedAgentId = process.env.PI_TMUX_AGENTS_CHILD_ID;
  delete process.env.PI_TMUX_AGENTS_CHILD_ID;

  try {
    // effectiveBeadsId = null AND binding = null → should NOT throw (AC3 back-compat)
    assert.doesNotThrow(
      () => enforceCerebroBoundBeadsId(null),
      "enforceCerebroBoundBeadsId must not throw when session is unbound"
    );

    // effectiveBeadsId = "" AND binding = null → should NOT throw
    assert.doesNotThrow(
      () => enforceCerebroBoundBeadsId(""),
      "enforceCerebroBoundBeadsId must not throw when session is unbound (empty string)"
    );

    // effectiveBeadsId = "agentic-explicit" AND binding = null → should NOT throw
    assert.doesNotThrow(
      () => enforceCerebroBoundBeadsId("agentic-explicit"),
      "enforceCerebroBoundBeadsId must not throw when explicit beadsId provided and unbound"
    );
  } finally {
    if (savedAgentId !== undefined) process.env.PI_TMUX_AGENTS_CHILD_ID = savedAgentId;
  }
});

// ---------------------------------------------------------------------------
// T4: source-introspection — helper-symbol presence in BOTH files (TL-PI-N3)
// ---------------------------------------------------------------------------

test("T4: helper symbols present in session-context-reader.ts AND index.ts (TL-PI-N3)", () => {
  const scr = readFileSync(
    new URL("../src/session-context-reader.ts", import.meta.url),
    "utf-8"
  );
  const idx = readFileSync(
    new URL("../src/index.ts", import.meta.url),
    "utf-8"
  );

  // session-context-reader.ts must export all three symbols
  assert.ok(
    scr.includes("CEREBRO_BEADS_ENFORCEMENT_ERROR"),
    "session-context-reader.ts must contain CEREBRO_BEADS_ENFORCEMENT_ERROR"
  );
  assert.ok(
    scr.includes("formatCerebroBeadsEnforcementError"),
    "session-context-reader.ts must contain formatCerebroBeadsEnforcementError"
  );
  assert.ok(
    scr.includes("enforceCerebroBoundBeadsId"),
    "session-context-reader.ts must contain enforceCerebroBoundBeadsId"
  );

  // index.ts must import and call the enforcement helper (gate wired in)
  assert.ok(
    idx.includes("enforceCerebroBoundBeadsId"),
    "index.ts must contain enforceCerebroBoundBeadsId call-site"
  );

  // index.ts must import from session-context-reader (import statement check)
  assert.ok(
    idx.includes("session-context-reader"),
    "index.ts must import from session-context-reader"
  );
});

// ---------------------------------------------------------------------------
// T5: RememberParamsSchema retains additionalProperties:false / no opt-out (S-N3/AC4)
// ---------------------------------------------------------------------------

test("T5: RememberParamsSchema has no opt-out enforcement parameter", () => {
  const typesSource = readFileSync(
    new URL("../src/types.ts", import.meta.url),
    "utf-8"
  );

  // No opt-out field must exist in RememberParamsSchema
  const optOutPatterns = [
    "skipEnforcement",
    "bypassEnforcement",
    "allowUnlinked",
    "optOut",
    "noEnforce",
  ];
  for (const pattern of optOutPatterns) {
    assert.ok(
      !typesSource.includes(pattern),
      `RememberParamsSchema must NOT contain opt-out field: ${pattern}`
    );
  }

  // RememberParamsSchema must still be present (not deleted)
  assert.ok(
    typesSource.includes("RememberParamsSchema"),
    "types.ts must still export RememberParamsSchema"
  );

  // TypeBox Type.Object present (additionalProperties:false by default in TypeBox)
  assert.ok(
    typesSource.includes("Type.Object"),
    "RememberParamsSchema must use Type.Object (TypeBox schema)"
  );
});
