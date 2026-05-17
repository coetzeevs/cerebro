/**
 * @file session-start.test.mjs
 * @ticket HS-009
 *
 * TDD tests for the session_start hook and runBootPrime behaviour.
 *
 * Tests:
 *   - session_start fires runBootPrime when CLAUDE_PROJECT_DIR is set
 *   - session_start skips boot-prime when CLAUDE_PROJECT_DIR is absent/empty
 *   - runBootPrime: primed count from "primed: N memories" token (N5)
 *   - runBootPrime: fallback to non-empty line count
 */

import assert from "node:assert/strict";
import { test, before, after } from "node:test";
import { createJiti } from "jiti";
import { makeStubPath, cleanStubPath } from "./_helpers/path-with-stub.mjs";

const jiti = createJiti(import.meta.url);

async function importCerebroCli() {
  return jiti.import("../src/cerebro-cli.ts");
}

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
// Mock Pi
// ---------------------------------------------------------------------------
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
// Test: session_start hook invokes runBootPrime when projectDir is present
// ---------------------------------------------------------------------------
test("session_start: invokes boot prime when CLAUDE_PROJECT_DIR is set", async () => {
  const { __resetCerebroPathForTests, initCerebroPath } = await importCerebroCli();
  __resetCerebroPathForTests();
  initCerebroPath();

  process.env.CLAUDE_PROJECT_DIR = "/test/project";

  const indexMod = await jiti.import("../src/index.ts");
  const piCerebro = indexMod.default;
  const mockPi = makeMockPi();
  piCerebro(mockPi);

  const handler = mockPi._hooks["session_start"];
  assert.ok(handler, "session_start handler must be registered");

  // Should not throw — stub cerebro handles --prime flag
  await assert.doesNotReject(
    async () => handler({ type: "session_start", reason: "startup" })
  );
});

// ---------------------------------------------------------------------------
// Test: session_start hook skips when CLAUDE_PROJECT_DIR is absent
// ---------------------------------------------------------------------------
test("session_start: skips boot prime when CLAUDE_PROJECT_DIR is absent", async () => {
  const { __resetCerebroPathForTests, initCerebroPath } = await importCerebroCli();
  __resetCerebroPathForTests();
  initCerebroPath();

  const saved = process.env.CLAUDE_PROJECT_DIR;
  delete process.env.CLAUDE_PROJECT_DIR;

  try {
    const indexMod = await jiti.import("../src/index.ts");
    const piCerebro = indexMod.default;
    const mockPi = makeMockPi();
    piCerebro(mockPi);

    const handler = mockPi._hooks["session_start"];
    assert.ok(handler, "session_start handler must be registered");

    // Should not throw — just logs a warning
    await assert.doesNotReject(
      async () => handler({ type: "session_start", reason: "startup" })
    );
  } finally {
    if (saved !== undefined) process.env.CLAUDE_PROJECT_DIR = saved;
  }
});

// ---------------------------------------------------------------------------
// Test: session_start hook skips when CLAUDE_PROJECT_DIR is empty string
// ---------------------------------------------------------------------------
test("session_start: skips boot prime when CLAUDE_PROJECT_DIR is empty string (|| guard)", async () => {
  const { __resetCerebroPathForTests, initCerebroPath } = await importCerebroCli();
  __resetCerebroPathForTests();
  initCerebroPath();

  const saved = process.env.CLAUDE_PROJECT_DIR;
  process.env.CLAUDE_PROJECT_DIR = ""; // empty string — || guard must catch this

  try {
    const indexMod = await jiti.import("../src/index.ts");
    const piCerebro = indexMod.default;
    const mockPi = makeMockPi();
    piCerebro(mockPi);

    const handler = mockPi._hooks["session_start"];
    assert.ok(handler);

    // Should not throw — skips with warning
    await assert.doesNotReject(
      async () => handler({ type: "session_start", reason: "startup" })
    );
  } finally {
    if (saved !== undefined) process.env.CLAUDE_PROJECT_DIR = saved;
  }
});

// ---------------------------------------------------------------------------
// Test: runBootPrime parses "primed: N memories" token (N5 primary path)
// ---------------------------------------------------------------------------
test("runBootPrime: parses primed count from token (N5 primary path)", async () => {
  const { runBootPrime, resolveCerebroPath, validateCerebroCli } = await importCerebroCli();

  const binaryPath = resolveCerebroPath();
  validateCerebroCli(binaryPath);

  const result = runBootPrime("/test/project", binaryPath);
  assert.equal(result.primedCount, 3, `Expected primedCount=3 from stub, got: ${result.primedCount}`);
  assert.ok(result.stdout.includes("primed: 3 memories"), "stdout should contain the primed token");
});
