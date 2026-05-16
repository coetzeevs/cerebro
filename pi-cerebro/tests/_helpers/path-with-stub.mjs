/**
 * @file path-with-stub.mjs
 * @ticket HS-009
 *
 * Test helper: build a PATH string that prepends the directory containing the
 * stub cerebro binary, so that `which cerebro` and execFileSync("cerebro", ...)
 * resolve to the stub rather than any real binary on the operator's PATH.
 *
 * Usage:
 *   import { makeStubPath, STUB_CEREBRO_PATH } from "./_helpers/path-with-stub.mjs";
 *
 *   // In a test that needs the stub on PATH:
 *   process.env.PATH = makeStubPath("stub-cerebro");
 *
 * WHY a helper?
 * Multiple test files exercise the shell-out wrappers. Without a shared helper,
 * each test would duplicate the __dirname resolution and PATH construction.
 * Per memory 684e9af6 (jiti module-cache pattern), shared reset helpers
 * prevent tribal knowledge fragmentation.
 */

import { fileURLToPath } from "node:url";
import { dirname, join } from "node:path";

const __filename = fileURLToPath(import.meta.url);
const __dir = dirname(__filename);

/** Absolute path to the tests/fixtures/ directory. */
export const FIXTURES_DIR = join(__dir, "..", "fixtures");

/**
 * The absolute path to the stub-cerebro.mjs fixture (the healthy stub).
 * Tests can also point CLAUDE_CODE_CLI at this for direct-path invocation.
 */
export const STUB_CEREBRO_PATH = join(FIXTURES_DIR, "stub-cerebro.mjs");

/**
 * The absolute path to the failing stub fixture.
 */
export const STUB_CEREBRO_FAIL_PATH = join(FIXTURES_DIR, "stub-cerebro-fail.mjs");

/**
 * Build a PATH string that prepends a symlink directory for the stub binary.
 *
 * Since our stubs are named `stub-cerebro.mjs` but need to be found as
 * `cerebro`, we create a temp directory with a `cerebro` symlink pointing to
 * the stub, then prepend that directory to PATH.
 *
 * Returns an object with:
 *   - `path` — the modified PATH string to assign to process.env.PATH
 *   - `binDir` — the temp bin directory (callers may clean it up if desired)
 *
 * @param stubName  "stub-cerebro" | "stub-cerebro-fail" — which fixture to use
 * @param originalPath  The original PATH value (defaults to process.env.PATH)
 */
import { mkdtempSync, symlinkSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";

export function makeStubPath(stubName = "stub-cerebro", originalPath = process.env.PATH ?? "") {
  const stubFile = join(FIXTURES_DIR, `${stubName}.mjs`);
  const binDir = mkdtempSync(join(tmpdir(), "pi-cerebro-test-"));
  const cerebroLink = join(binDir, "cerebro");
  symlinkSync(stubFile, cerebroLink);
  const path = `${binDir}:${originalPath}`;
  return { path, binDir };
}

/**
 * Clean up a bin directory created by makeStubPath.
 */
export function cleanStubPath(binDir) {
  try {
    rmSync(binDir, { recursive: true, force: true });
  } catch {
    // non-fatal; tmp dirs are ephemeral anyway
  }
}
