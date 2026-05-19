/**
 * @file cerebro-remember-beadsid.test.mjs
 * @ticket HS-039
 *
 * TDD tests for pi-cerebro beadsId tagging:
 *   - session-context-reader.ts: readCurrentBeadsId() cross-extension read
 *   - cerebro-cli.ts: runAdd gains beadsId param, appends --beads-id to argv
 *   - index.ts: cerebro_remember execute wires readCurrentBeadsId + runAdd
 *
 * Test strategy:
 *   - Use PI_TMUX_AGENTS_CHILD_ID env var to simulate agent identity
 *   - Use PI_CODING_AGENT_DIR env var (from @earendil-works/pi-coding-agent config.js:360)
 *     to redirect getAgentDir() to a temp dir containing a stub subagents.db
 *   - Use in-memory DatabaseSync (:memory:) per memory ffe7c494 (Meepo test pattern)
 *     — but for cross-extension reads we write a real temp DB since readCurrentBeadsId
 *     reads from a file path
 *   - Use PI_CEREBRO_STUB_ARGV_LOG (from stub-cerebro.mjs [HS-024]) to capture argv
 *
 * Test cases (AC1-AC4, all Wire):
 *   AC1/AC4: cerebro_remember reads currentBeadsId from session_contexts row,
 *            calls getSlot exactly once (via readCurrentBeadsId), passes to runAdd,
 *            runAdd argv contains ["--beads-id", "agentic-abc"] (TL-N2 positional form)
 *   AC2:     --beads-id and value are positional separate argv tokens (not inline-concat)
 *   AC3:     null currentBeadsId -> no --beads-id flag in argv, no error
 *   N-S4:    any error in readCurrentBeadsId -> sanitised warning, best-effort null,
 *            cerebro_remember still succeeds
 */

import assert from "node:assert/strict";
import { test, before, after } from "node:test";
import { mkdtempSync, writeFileSync, rmSync, mkdirSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { DatabaseSync } from "node:sqlite";
import { createJiti } from "jiti";
import { makeStubPath, cleanStubPath } from "./_helpers/path-with-stub.mjs";

const jiti = createJiti(import.meta.url);

// ---------------------------------------------------------------------------
// Test-scoped state
// ---------------------------------------------------------------------------
let stubBinDir;
let originalPath;
let originalProjectDir;
let originalAgentChildId;
let originalAgentDir;
let tmpAgentDir;
let tmpDbPath;

before(() => {
  // Set up stub cerebro binary on PATH
  originalPath = process.env.PATH;
  const { path, binDir } = makeStubPath("stub-cerebro");
  process.env.PATH = path;
  stubBinDir = binDir;

  // Set up CLAUDE_PROJECT_DIR
  originalProjectDir = process.env.CLAUDE_PROJECT_DIR;
  process.env.CLAUDE_PROJECT_DIR = "/test/project";

  // Save agent env vars
  originalAgentChildId = process.env.PI_TMUX_AGENTS_CHILD_ID;
  originalAgentDir = process.env.PI_CODING_AGENT_DIR;

  // Create temp agent dir with a stub subagents.db
  tmpAgentDir = mkdtempSync(join(tmpdir(), "pi-cerebro-beadsid-test-"));
  tmpDbPath = join(tmpAgentDir, "subagents.db");
  setupTestDb(tmpDbPath, []);

  // Point getAgentDir() at our temp dir
  process.env.PI_CODING_AGENT_DIR = tmpAgentDir;
});

after(() => {
  process.env.PATH = originalPath;
  if (originalProjectDir !== undefined) {
    process.env.CLAUDE_PROJECT_DIR = originalProjectDir;
  } else {
    delete process.env.CLAUDE_PROJECT_DIR;
  }
  if (originalAgentChildId !== undefined) {
    process.env.PI_TMUX_AGENTS_CHILD_ID = originalAgentChildId;
  } else {
    delete process.env.PI_TMUX_AGENTS_CHILD_ID;
  }
  if (originalAgentDir !== undefined) {
    process.env.PI_CODING_AGENT_DIR = originalAgentDir;
  } else {
    delete process.env.PI_CODING_AGENT_DIR;
  }
  cleanStubPath(stubBinDir);
  try {
    rmSync(tmpAgentDir, { recursive: true, force: true });
  } catch { /* ignore */ }
});

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

/**
 * Create a minimal subagents.db with session_contexts table at `dbPath`.
 * Optionally pre-populate with rows: [{agent_id, slot_name, slot_value}].
 */
function setupTestDb(dbPath, rows) {
  const db = new DatabaseSync(dbPath);
  db.exec(`
    CREATE TABLE IF NOT EXISTS session_contexts (
      agent_id TEXT NOT NULL,
      slot_name TEXT NOT NULL,
      slot_value TEXT,
      written_at TEXT NOT NULL DEFAULT (datetime('now')),
      owner_extension TEXT NOT NULL DEFAULT '',
      PRIMARY KEY (agent_id, slot_name)
    )
  `);
  for (const row of rows) {
    db.prepare(
      "INSERT OR REPLACE INTO session_contexts (agent_id, slot_name, slot_value, owner_extension) VALUES (?, ?, ?, 'pi-beads')"
    ).run(row.agent_id, row.slot_name, row.slot_value);
  }
  db.close();
}

// ---------------------------------------------------------------------------
// AC1 / AC4: readCurrentBeadsId reads from session_contexts row
// ---------------------------------------------------------------------------

test("readCurrentBeadsId: returns slot_value when agent has currentBeadsId row", async () => {
  // Set up DB with a currentBeadsId row for agent "agent-test-1"
  setupTestDb(tmpDbPath, [
    { agent_id: "agent-test-1", slot_name: "currentBeadsId", slot_value: "agentic-abc" },
  ]);

  process.env.PI_TMUX_AGENTS_CHILD_ID = "agent-test-1";

  const mod = await jiti.import("../src/session-context-reader.ts");
  const { readCurrentBeadsId } = mod;

  const result = readCurrentBeadsId();
  assert.equal(result, "agentic-abc", `Expected "agentic-abc", got: ${result}`);
});

// ---------------------------------------------------------------------------
// AC3: null currentBeadsId -> readCurrentBeadsId returns null
// ---------------------------------------------------------------------------

test("readCurrentBeadsId: returns null when agent has no currentBeadsId row", async () => {
  // DB has no row for this agent
  setupTestDb(tmpDbPath, []);
  process.env.PI_TMUX_AGENTS_CHILD_ID = "agent-no-row";

  const mod = await jiti.import("../src/session-context-reader.ts");
  const { readCurrentBeadsId } = mod;

  const result = readCurrentBeadsId();
  assert.equal(result, null, `Expected null, got: ${result}`);
});

// ---------------------------------------------------------------------------
// AC3: no env var -> readCurrentBeadsId returns null (top-level agent)
// ---------------------------------------------------------------------------

test("readCurrentBeadsId: returns null when PI_TMUX_AGENTS_CHILD_ID is not set", async () => {
  const saved = process.env.PI_TMUX_AGENTS_CHILD_ID;
  delete process.env.PI_TMUX_AGENTS_CHILD_ID;

  try {
    const mod = await jiti.import("../src/session-context-reader.ts");
    const { readCurrentBeadsId } = mod;

    const result = readCurrentBeadsId();
    assert.equal(result, null, `Expected null for top-level agent (no env var), got: ${result}`);
  } finally {
    if (saved !== undefined) process.env.PI_TMUX_AGENTS_CHILD_ID = saved;
  }
});

// ---------------------------------------------------------------------------
// N-S4: DB missing -> best-effort null, no throw
// ---------------------------------------------------------------------------

test("readCurrentBeadsId: returns null (best-effort) when DB does not exist", async () => {
  // Point to a non-existent DB path
  const savedDir = process.env.PI_CODING_AGENT_DIR;
  process.env.PI_CODING_AGENT_DIR = "/nonexistent/path/that/does/not/exist";
  process.env.PI_TMUX_AGENTS_CHILD_ID = "agent-db-missing";

  try {
    const mod = await jiti.import("../src/session-context-reader.ts");
    const { readCurrentBeadsId } = mod;

    // Must NOT throw — best-effort policy
    let result;
    assert.doesNotThrow(() => { result = readCurrentBeadsId(); });
    assert.equal(result, null, `Expected null when DB is missing, got: ${result}`);
  } finally {
    process.env.PI_CODING_AGENT_DIR = savedDir ?? tmpAgentDir;
  }
});

// ---------------------------------------------------------------------------
// TL-N2: runAdd appends --beads-id and value as separate positional tokens
// (not inline-concat like --beads-id=agentic-abc)
// ---------------------------------------------------------------------------

test("runAdd: argv contains --beads-id and value as separate positional tokens (TL-N2)", async () => {
  // Use PI_CEREBRO_STUB_ARGV_LOG to capture the argv the stub receives
  const tmpLog = join(tmpAgentDir, "argv.json");
  process.env.PI_CEREBRO_STUB_ARGV_LOG = tmpLog;

  try {
    const { __resetCerebroPathForTests, initCerebroPath } = await jiti.import(
      "../src/cerebro-cli.ts"
    );
    __resetCerebroPathForTests();
    initCerebroPath();

    const mod = await jiti.import("../src/cerebro-cli.ts");
    const { runAdd } = mod;

    // Call runAdd with a beadsId
    const binaryPath = (await jiti.import("../src/cerebro-cli.ts")).getCerebroPath();
    runAdd("test content", "/test/project", "episode", 0.75, binaryPath, "agentic-abc");

    // Read the captured argv
    const { readFileSync } = await import("node:fs");
    const argv = JSON.parse(readFileSync(tmpLog, "utf-8"));

    // TL-N2: --beads-id and agentic-abc must be separate tokens
    const beadsIdIdx = argv.indexOf("--beads-id");
    assert.ok(beadsIdIdx !== -1, "argv must contain --beads-id token");
    assert.equal(
      argv[beadsIdIdx + 1],
      "agentic-abc",
      `argv[beadsIdIdx+1] must be 'agentic-abc' (separate token), got: ${argv[beadsIdIdx + 1]}`
    );
    // Must NOT use inline-concat form
    const hasInlineConcat = argv.some((a) => a.startsWith("--beads-id="));
    assert.ok(!hasInlineConcat, "argv must NOT contain --beads-id=... inline-concat form");
  } finally {
    delete process.env.PI_CEREBRO_STUB_ARGV_LOG;
  }
});

// ---------------------------------------------------------------------------
// AC3: runAdd with null beadsId -> no --beads-id in argv
// ---------------------------------------------------------------------------

test("runAdd: no --beads-id token when beadsId is null", async () => {
  const tmpLog = join(tmpAgentDir, "argv-null.json");
  process.env.PI_CEREBRO_STUB_ARGV_LOG = tmpLog;

  try {
    const { __resetCerebroPathForTests, initCerebroPath, runAdd, getCerebroPath } =
      await jiti.import("../src/cerebro-cli.ts");
    __resetCerebroPathForTests();
    initCerebroPath();

    const binaryPath = getCerebroPath();
    runAdd("test content", "/test/project", "episode", 0.75, binaryPath, null);

    const { readFileSync } = await import("node:fs");
    const argv = JSON.parse(readFileSync(tmpLog, "utf-8"));

    assert.ok(!argv.includes("--beads-id"), "argv must NOT contain --beads-id when beadsId is null");
  } finally {
    delete process.env.PI_CEREBRO_STUB_ARGV_LOG;
  }
});
