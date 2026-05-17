/**
 * @file compaction.test.mjs
 * @ticket HS-010
 *
 * TDD tests for src/compaction.ts — heuristic compaction detector.
 *
 * RESET DISCIPLINE (Tech Lead N1):
 * node:test has no global beforeEach. jiti module caching means module-scope
 * state (lastSeen) survives across tests in the same file. Call
 * __resetCompactionStateForTests() at the top of EVERY test to guarantee
 * isolation. Mirror: session-start.test.mjs:66-68 and HS-009 pattern.
 *
 * MOCK SESSION MANAGER HELPER (Tech Lead N3):
 * makeMockSessionManager(lengths) returns successive values from the array on
 * each getEntries() call. Index is clamped via Math.min to prevent off-by-one
 * errors when a test invokes the handler more times than the array supplies.
 *
 * Tests:
 *   1. AC1 — 100 → 40 (60% drop > 50%) fires + exact log substring
 *   2. AC2 — 100 → 80 (20% drop) does not fire
 *   3. AC3 — 50 → 75 (increase) does not fire
 *   4. Baseline — first observation does not fire (arms lastSeen)
 *   5. AC4 — exact log-line contract for HS-016 grep -F binding
 */

import assert from "node:assert/strict";
import { test, before, after } from "node:test";
import { createJiti } from "jiti";
import { makeStubPath, cleanStubPath } from "./_helpers/path-with-stub.mjs";

const jiti = createJiti(import.meta.url);

async function importCompaction() {
  return jiti.import("../src/compaction.ts");
}

async function importCerebroCli() {
  return jiti.import("../src/cerebro-cli.ts");
}

// ---------------------------------------------------------------------------
// Mock helpers
// ---------------------------------------------------------------------------

/**
 * Deterministic mock session manager (Tech Lead N3).
 * Successive calls to getEntries() return { length: lengths[i] }.
 * Index is clamped at lengths.length - 1 to prevent over-read errors.
 *
 * @param {number[]} lengths  Array of successive entry counts to return.
 */
function makeMockSessionManager(lengths) {
  let i = 0;
  return { getEntries: () => ({ length: lengths[Math.min(i++, lengths.length - 1)] }) };
}

function makeMockPi() {
  const tools = [];
  const hooks = {};
  return {
    registerTool(def) { tools.push(def); },
    on(event, handler) { hooks[event] = handler; },
    _tools: tools,
    _hooks: hooks,
  };
}

// ---------------------------------------------------------------------------
// PATH setup (stub-cerebro on PATH for wiring tests)
// ---------------------------------------------------------------------------
let stubBinDir;
let originalPath;
let originalProjectDir;

before(() => {
  originalPath = process.env.PATH;
  originalProjectDir = process.env.CLAUDE_PROJECT_DIR;
  const { path, binDir } = makeStubPath("stub-cerebro");
  process.env.PATH = path;
  stubBinDir = binDir;
  process.env.CLAUDE_PROJECT_DIR = "/test/project";
});

after(() => {
  process.env.PATH = originalPath;
  if (originalProjectDir !== undefined) {
    process.env.CLAUDE_PROJECT_DIR = originalProjectDir;
  } else {
    delete process.env.CLAUDE_PROJECT_DIR;
  }
  cleanStubPath(stubBinDir);
});

// ---------------------------------------------------------------------------
// Test 1 — AC1: 100 → 40 fires (60% drop > 50%)
// ---------------------------------------------------------------------------
test("compaction: 100→40 (60% drop) fires observeEntriesLength and isCompactionDrop returns true", async () => {
  // RESET (Tech Lead N1): jiti caches module state — reset on every test.
  const { __resetCompactionStateForTests, observeEntriesLength, isCompactionDrop } = await importCompaction();
  __resetCompactionStateForTests();

  // Pure function check
  assert.equal(isCompactionDrop(100, 40), true, "60% drop must return true");

  // Stateful: first tick primes baseline (no fire)
  const tick1 = observeEntriesLength(100);
  assert.equal(tick1.fired, false, "First tick must not fire (primes baseline)");

  // Second tick: 40 from 100 = 60% drop > 50% threshold → fires
  const tick2 = observeEntriesLength(40);
  assert.equal(tick2.fired, true, "60% drop must fire");
});

// ---------------------------------------------------------------------------
// Test 2 — AC2: 100 → 80 does not fire (20% drop ≤ 50%)
// ---------------------------------------------------------------------------
test("compaction: 100→80 (20% drop) does not fire", async () => {
  const { __resetCompactionStateForTests, observeEntriesLength, isCompactionDrop } = await importCompaction();
  __resetCompactionStateForTests();

  assert.equal(isCompactionDrop(100, 80), false, "20% drop must return false");

  observeEntriesLength(100); // prime baseline
  const tick2 = observeEntriesLength(80);
  assert.equal(tick2.fired, false, "20% drop must NOT fire");
});

// ---------------------------------------------------------------------------
// Test 3 — AC3: 50 → 75 (increase) does not fire
// ---------------------------------------------------------------------------
test("compaction: 50→75 (increase) does not fire", async () => {
  const { __resetCompactionStateForTests, observeEntriesLength, isCompactionDrop } = await importCompaction();
  __resetCompactionStateForTests();

  assert.equal(isCompactionDrop(50, 75), false, "Increase must return false");

  observeEntriesLength(50); // prime baseline
  const tick2 = observeEntriesLength(75);
  assert.equal(tick2.fired, false, "Increase must NOT fire");
});

// ---------------------------------------------------------------------------
// Test 4 — Baseline: first observation never fires (any value)
// ---------------------------------------------------------------------------
test("compaction: first observation does not fire (arms lastSeen baseline)", async () => {
  const { __resetCompactionStateForTests, observeEntriesLength } = await importCompaction();
  __resetCompactionStateForTests();

  // Even a very large value on first tick must not fire
  const tick1 = observeEntriesLength(1000);
  assert.equal(tick1.fired, false, "First observation must never fire");

  // Second tick with a tiny value should fire (confirms baseline was armed)
  const tick2 = observeEntriesLength(1);
  assert.equal(tick2.fired, true, "After baseline is armed, a drop > 50% must fire");
});

// ---------------------------------------------------------------------------
// Test 5 — AC4: exact log-line + message_end wiring via index.ts
//            Verifies the HS-016 grep -F contract is satisfied
// ---------------------------------------------------------------------------
test("compaction: message_end handler fires runBootPrime and logs exact HS-016 contract string", async () => {
  // RESET (Tech Lead N1): reset both compaction state and cerebro path state.
  const { __resetCompactionStateForTests } = await importCompaction();
  __resetCompactionStateForTests();

  const { __resetCerebroPathForTests, initCerebroPath } = await importCerebroCli();
  __resetCerebroPathForTests();
  initCerebroPath();

  process.env.CLAUDE_PROJECT_DIR = "/test/project";

  const indexMod = await jiti.import("../src/index.ts");
  const piCerebro = indexMod.default;
  const mockPi = makeMockPi();
  piCerebro(mockPi);

  const handler = mockPi._hooks["message_end"];
  assert.ok(handler, "message_end handler must be registered");

  // Capture stderr output to verify the exact HS-016 contract substring
  const stderrLines = [];
  const origStderrWrite = process.stderr.write.bind(process.stderr);
  process.stderr.write = (chunk, ...rest) => {
    if (typeof chunk === "string") stderrLines.push(chunk);
    else stderrLines.push(chunk.toString());
    return origStderrWrite(chunk, ...rest);
  };

  try {
    // First tick: prime baseline (no fire expected)
    const sm100 = makeMockSessionManager([100]);
    await handler({ type: "message_end" }, { sessionManager: sm100 });

    // Clear collected lines before the firing tick
    stderrLines.length = 0;

    // Second tick: 40 from 100 = 60% drop → must fire
    const sm40 = makeMockSessionManager([40]);
    await handler({ type: "message_end" }, { sessionManager: sm40 });
  } finally {
    process.stderr.write = origStderrWrite;
  }

  // Verify the exact HS-016 grep -F contract substring is present (S-N7: static literal only)
  const allLines = stderrLines.join("");
  assert.ok(
    allLines.includes("[pi-cerebro] compaction detected: re-priming memories"),
    `Expected exact log substring '[pi-cerebro] compaction detected: re-priming memories' in stderr. Got: ${allLines}`
  );
});
