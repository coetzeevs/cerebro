/**
 * @file missing-binary.test.mjs
 * @ticket HS-009
 *
 * TDD test for Scenario 5: missing cerebro binary fail-fast.
 *
 * When the cerebro binary is absent from PATH:
 *   - initCerebroPath() should log an error and set getCerebroPath() to null
 *   - The extension default export should register NO tools
 *   - The extension default export should register NO hooks
 *
 * This is the "graceful degradation" path: Pi continues without cerebro
 * capability rather than crashing the whole extension.
 */

import assert from "node:assert/strict";
import { test, before, after } from "node:test";
import { createJiti } from "jiti";

const jiti = createJiti(import.meta.url);

async function importCerebroCli() {
  return jiti.import("../src/cerebro-cli.ts");
}

// ---------------------------------------------------------------------------
// PATH setup — use an empty PATH so cerebro cannot be found
// ---------------------------------------------------------------------------
let originalPath;
let originalProjectDir;

before(() => {
  originalPath = process.env.PATH;
  originalProjectDir = process.env.CLAUDE_PROJECT_DIR;
  // Set PATH to only /usr/bin so `which cerebro` cannot find a cerebro binary
  // (cerebro is not shipped with macOS/Linux base system)
  process.env.PATH = "/usr/bin:/bin";
  process.env.CLAUDE_PROJECT_DIR = "/test/project";
});

after(() => {
  process.env.PATH = originalPath;
  if (originalProjectDir !== undefined) {
    process.env.CLAUDE_PROJECT_DIR = originalProjectDir;
  } else {
    delete process.env.CLAUDE_PROJECT_DIR;
  }
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
// Test: initCerebroPath logs error and getCerebroPath returns null
// ---------------------------------------------------------------------------
test("missing-binary: initCerebroPath logs error and getCerebroPath returns null", async () => {
  const { initCerebroPath, getCerebroPath, __resetCerebroPathForTests } = await importCerebroCli();
  __resetCerebroPathForTests();

  // Should not throw — logs error internally
  assert.doesNotThrow(() => initCerebroPath());

  // getCerebroPath must return null (binary not found)
  const path = getCerebroPath();
  assert.equal(path, null, `Expected null getCerebroPath(), got: ${path}`);
});

// ---------------------------------------------------------------------------
// Test: extension registers nothing when binary is absent (Scenario 5)
// ---------------------------------------------------------------------------
test("missing-binary: extension registers no tools and no hooks when binary absent", async () => {
  const { __resetCerebroPathForTests, initCerebroPath } = await importCerebroCli();
  __resetCerebroPathForTests();
  initCerebroPath(); // will fail — binary not on this PATH

  const indexMod = await jiti.import("../src/index.ts");
  const piCerebro = indexMod.default;

  const mockPi = makeMockPi();
  piCerebro(mockPi);

  assert.equal(
    mockPi._tools.length,
    0,
    `Expected 0 tools registered, got: ${mockPi._tools.length}`
  );
  assert.equal(
    Object.keys(mockPi._hooks).length,
    0,
    `Expected 0 hooks registered, got: ${JSON.stringify(Object.keys(mockPi._hooks))}`
  );
});
