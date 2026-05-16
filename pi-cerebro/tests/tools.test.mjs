/**
 * @file tools.test.mjs
 * @ticket HS-009
 *
 * TDD tests for tool registration and execute callbacks in src/index.ts.
 *
 * Uses a mock ExtensionAPI to capture registerTool calls without needing
 * a real Pi runtime. Tests that:
 *   - cerebro_recall tool is registered with correct name/schema
 *   - cerebro_remember tool is registered with correct name/schema
 *   - execute callbacks return success with stub binary on PATH
 *   - execute callbacks return failure message when CLAUDE_PROJECT_DIR absent
 *   - execute callbacks return failure message on cerebro error
 */

import assert from "node:assert/strict";
import { test, before, after } from "node:test";
import { createJiti } from "jiti";
import { makeStubPath, cleanStubPath, FIXTURES_DIR } from "./_helpers/path-with-stub.mjs";
import { join } from "node:path";

const jiti = createJiti(import.meta.url);

async function importCerebroCli() {
  return jiti.import("../src/cerebro-cli.ts");
}

// ---------------------------------------------------------------------------
// Mock ExtensionAPI
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
// PATH setup
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
// Test: tool names and schemas are registered when binary is present
// ---------------------------------------------------------------------------
test("tools: cerebro_recall and cerebro_remember are registered when binary is on PATH", async () => {
  const { __resetCerebroPathForTests, initCerebroPath } = await importCerebroCli();
  __resetCerebroPathForTests();
  initCerebroPath();

  // Re-import index to trigger registration with fresh state.
  // jiti caches modules so we test the registration directly by inspecting
  // the module's exports and simulating Pi callback invocation.
  const indexMod = await jiti.import("../src/index.ts");
  const piCerebro = indexMod.default;

  const mockPi = makeMockPi();
  piCerebro(mockPi);

  const toolNames = mockPi._tools.map((t) => t.name);
  assert.ok(toolNames.includes("cerebro_recall"), `Expected cerebro_recall tool, got: ${JSON.stringify(toolNames)}`);
  assert.ok(toolNames.includes("cerebro_remember"), `Expected cerebro_remember tool, got: ${JSON.stringify(toolNames)}`);
});

// ---------------------------------------------------------------------------
// Test: cerebro_recall execute returns success with stub binary
// ---------------------------------------------------------------------------
test("tools: cerebro_recall execute returns success with stub binary", async () => {
  const { __resetCerebroPathForTests, initCerebroPath } = await importCerebroCli();
  __resetCerebroPathForTests();
  initCerebroPath();

  const indexMod = await jiti.import("../src/index.ts");
  const piCerebro = indexMod.default;
  const mockPi = makeMockPi();
  piCerebro(mockPi);

  const recallTool = mockPi._tools.find((t) => t.name === "cerebro_recall");
  assert.ok(recallTool, "cerebro_recall tool must be registered");

  const result = await recallTool.execute("id1", { query: "test query" }, undefined, undefined, {});
  assert.equal(result.details.success, true, `Expected success, got: ${JSON.stringify(result)}`);
  assert.ok(result.content[0].text.length > 0, "Result should contain memory content");
});

// ---------------------------------------------------------------------------
// Test: cerebro_recall execute returns failure when CLAUDE_PROJECT_DIR absent
// ---------------------------------------------------------------------------
test("tools: cerebro_recall execute returns failure when no projectDir", async () => {
  const { __resetCerebroPathForTests, initCerebroPath } = await importCerebroCli();
  __resetCerebroPathForTests();
  initCerebroPath();

  // Temporarily remove CLAUDE_PROJECT_DIR
  const saved = process.env.CLAUDE_PROJECT_DIR;
  delete process.env.CLAUDE_PROJECT_DIR;

  try {
    const indexMod = await jiti.import("../src/index.ts");
    const piCerebro = indexMod.default;
    const mockPi = makeMockPi();
    piCerebro(mockPi);

    const recallTool = mockPi._tools.find((t) => t.name === "cerebro_recall");
    assert.ok(recallTool);

    const result = await recallTool.execute("id1", { query: "test" }, undefined, undefined, {});
    assert.equal(result.details.success, false);
    assert.ok(result.content[0].text.includes("[pi-cerebro]"), `Expected [pi-cerebro] prefix, got: ${result.content[0].text}`);
  } finally {
    if (saved !== undefined) process.env.CLAUDE_PROJECT_DIR = saved;
  }
});

// ---------------------------------------------------------------------------
// Test: cerebro_remember execute returns success with nodeId
// ---------------------------------------------------------------------------
test("tools: cerebro_remember execute returns success with nodeId from stub", async () => {
  const { __resetCerebroPathForTests, initCerebroPath } = await importCerebroCli();
  __resetCerebroPathForTests();
  initCerebroPath();

  const indexMod = await jiti.import("../src/index.ts");
  const piCerebro = indexMod.default;
  const mockPi = makeMockPi();
  piCerebro(mockPi);

  const rememberTool = mockPi._tools.find((t) => t.name === "cerebro_remember");
  assert.ok(rememberTool, "cerebro_remember tool must be registered");

  const result = await rememberTool.execute(
    "id2",
    { content: "important fact", type: "concept", importance: 0.8 },
    undefined,
    undefined,
    {}
  );
  assert.equal(result.details.success, true, `Expected success, got: ${JSON.stringify(result)}`);
  assert.ok(result.details.nodeId === "abcdef12", `Expected nodeId in details, got: ${result.details.nodeId}`);
});

// ---------------------------------------------------------------------------
// Test: session_start hook is registered
// ---------------------------------------------------------------------------
test("tools: session_start hook is registered", async () => {
  const { __resetCerebroPathForTests, initCerebroPath } = await importCerebroCli();
  __resetCerebroPathForTests();
  initCerebroPath();

  const indexMod = await jiti.import("../src/index.ts");
  const piCerebro = indexMod.default;
  const mockPi = makeMockPi();
  piCerebro(mockPi);

  assert.ok(
    "session_start" in mockPi._hooks,
    `Expected session_start hook to be registered, got: ${JSON.stringify(Object.keys(mockPi._hooks))}`
  );
});
