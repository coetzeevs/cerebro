/**
 * @file run-boot-prime-argv.test.mjs
 * @ticket HS-024
 *
 * Regression test: asserts that runBootPrime passes "--prime" (not "--boot")
 * to the cerebro subprocess.
 *
 * Pattern: stub-binary-on-PATH (HS-009 memory db5b8cd9). The stub records its
 * argv to a tempfile when PI_CEREBRO_STUB_ARGV_LOG is set; the test reads the
 * file and asserts on the argv shape.
 *
 * Two test cases:
 *   1. argv contains "--prime"
 *   2. argv does NOT contain "--boot" (exact token, TL-N5 anti-regression)
 *
 * The same stub invocation covers both assertions; they are split into distinct
 * tests so a future regression surfaces with an unambiguous failure message.
 */

import assert from "node:assert/strict";
import { test, before, after } from "node:test";
import { readFileSync, mkdtempSync, rmSync } from "node:fs";
import { join } from "node:path";
import { tmpdir } from "node:os";
import { createJiti } from "jiti";
import { makeStubPath, cleanStubPath } from "./_helpers/path-with-stub.mjs";

const jiti = createJiti(import.meta.url);

async function importCerebroCli() {
  return jiti.import("../src/cerebro-cli.ts");
}

// ---------------------------------------------------------------------------
// Shared state
// ---------------------------------------------------------------------------

let stubBinDir;
let originalPath;
let originalProjectDir;
let originalArgvLog;
let tmpDir;
let argvLogPath;

before(() => {
  originalPath = process.env.PATH;
  originalProjectDir = process.env.CLAUDE_PROJECT_DIR;
  originalArgvLog = process.env.PI_CEREBRO_STUB_ARGV_LOG;

  // Stub on PATH
  const { path, binDir } = makeStubPath("stub-cerebro");
  process.env.PATH = path;
  stubBinDir = binDir;

  // Temp dir for argv log
  tmpDir = mkdtempSync(join(tmpdir(), "hs024-argv-"));
  argvLogPath = join(tmpDir, "argv.json");
  process.env.PI_CEREBRO_STUB_ARGV_LOG = argvLogPath;

  process.env.CLAUDE_PROJECT_DIR = "/test/project";
});

after(() => {
  process.env.PATH = originalPath;
  if (originalProjectDir !== undefined) {
    process.env.CLAUDE_PROJECT_DIR = originalProjectDir;
  } else {
    delete process.env.CLAUDE_PROJECT_DIR;
  }
  if (originalArgvLog !== undefined) {
    process.env.PI_CEREBRO_STUB_ARGV_LOG = originalArgvLog;
  } else {
    delete process.env.PI_CEREBRO_STUB_ARGV_LOG;
  }
  cleanStubPath(stubBinDir);
  try {
    rmSync(tmpDir, { recursive: true, force: true });
  } catch {
    // non-fatal
  }
});

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function invokeRunBootPrime(argvLog) {
  // Each invocation gets a fresh module state to avoid jiti cache bleed.
  // __resetCerebroPathForTests ensures resolveCerebroPath() re-resolves which.
  const mod = jiti.import("../src/cerebro-cli.ts");
  return mod.then(({ __resetCerebroPathForTests, initCerebroPath, resolveCerebroPath, validateCerebroCli, runBootPrime }) => {
    __resetCerebroPathForTests();
    initCerebroPath();
    const binary = resolveCerebroPath();
    validateCerebroCli(binary);
    // Set the argv-log path before calling so the stub picks it up
    process.env.PI_CEREBRO_STUB_ARGV_LOG = argvLog;
    runBootPrime("/test/project", binary);
    return JSON.parse(readFileSync(argvLog, "utf8"));
  });
}

// ---------------------------------------------------------------------------
// Test 1: argv must contain "--prime"
// ---------------------------------------------------------------------------
test("runBootPrime: argv contains --prime", async () => {
  const recorded = await invokeRunBootPrime(argvLogPath);
  assert.ok(
    recorded.includes("--prime"),
    `runBootPrime argv must contain "--prime"; got: ${JSON.stringify(recorded)}`
  );

  // Positional sanity: "--prime" immediately follows "recall" subcommand
  const recallIdx = recorded.indexOf("recall");
  assert.ok(recallIdx >= 0, `argv must contain "recall" subcommand; got: ${JSON.stringify(recorded)}`);
  assert.equal(
    recorded[recallIdx + 1],
    "--prime",
    `"--prime" must immediately follow "recall" in argv; got: ${JSON.stringify(recorded)}`
  );

  // Sanity: -p flag present
  assert.ok(
    recorded.includes("-p"),
    `argv must contain "-p" project flag; got: ${JSON.stringify(recorded)}`
  );
});

// ---------------------------------------------------------------------------
// Test 2: argv must NOT contain "--boot" (exact token, TL-N5)
// ---------------------------------------------------------------------------
test("runBootPrime: argv does NOT contain --boot (anti-regression sentinel)", async () => {
  const recorded = await invokeRunBootPrime(argvLogPath);
  const bootHits = recorded.filter((a) => a === "--boot").length;
  assert.equal(
    bootHits,
    0,
    `runBootPrime argv must contain zero "--boot" tokens; got: ${JSON.stringify(recorded)}`
  );
});
