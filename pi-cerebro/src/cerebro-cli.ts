/**
 * @file cerebro-cli.ts
 * @ticket HS-009
 *
 * Shell-out wrappers for the `cerebro` CLI binary.
 *
 * WHY SHELL-OUT (not a library import)?
 * `cerebro` is a compiled Go binary. There is no shared JS/TS library.
 * We invoke it as a subprocess via execFileSync with an argv-array — never
 * via shell: true or string interpolation — to eliminate shell-injection risk
 * (Security: argv-array discipline enforced throughout this file).
 *
 * PATH-TRUST POSTURE (Tech Lead N3 + Security S-N3 / HS-022 deferred)
 * At module initialisation we look up `cerebro` from PATH exactly ONCE via
 * `which cerebro` (or `where cerebro` on Windows). This is stored in a
 * module-scoped constant. Benefits:
 *   1. Fail-fast: missing binary surfaces at extension load time, not on first
 *      tool call.
 *   2. Stale-shim defence: we validate the resolved binary exits 0 on
 *      `cerebro --version` AND the output matches /\d+\.\d+/ — a shim that
 *      exits 0 with empty output is rejected. See validateCerebroCli().
 *   3. Absolute-path caching (HS-022-style) is out of scope per scope-
 *      expansion both-reviewer rule; HS-022 covers pi-claude-code-cli's
 *      `claude` binary. We do PATH-lookup once at init and store the result.
 *
 * SANITISE RATIONALE
 * All strings from external subprocess (stdout, stderr, err.message) are
 * passed through sanitise() before being embedded in thrown Error messages,
 * console output, or tool results. This prevents unbounded subprocess output
 * from flooding logs and strips ASCII control characters that could corrupt
 * terminal rendering. The 200-char variant is copied verbatim from HS-005
 * (tests/claude-binary.test.mjs:100-107) per Security S-N5 — we do NOT
 * cross-package-extract into a shared module (deferred per memory 3a28fbf2).
 *
 * EMPTY-STRING vs NULL GUARD
 * CLAUDE_PROJECT_DIR is read via `||` guard (not `??`) so that an empty
 * string is treated as absent (falls to the fallback path). `??` only catches
 * undefined/null — a set-but-empty var would slip through.
 */

import { execFileSync } from "node:child_process";
import type { AddResult, BootPrimeResult, RecallResult } from "./types.js";

// ---------------------------------------------------------------------------
// sanitise() — 200-char variant, copied verbatim from HS-005 pattern
// Strip ASCII control chars (preserving TAB and LF), trim whitespace,
// cap to 200 chars to prevent log flooding from external-binary output.
// ---------------------------------------------------------------------------
export function sanitise(raw: string): string {
  return raw
    .replace(/[\x00-\x08\x0B-\x1F\x7F]/g, "")
    .trim()
    .slice(0, 200);
}

// ---------------------------------------------------------------------------
// Stale-shim regex — version output must match /\d+\.\d+/
// Defends against a shim that exits 0 with empty or non-version output.
// ---------------------------------------------------------------------------
const VERSION_PATTERN = /\d+\.\d+/;

// ---------------------------------------------------------------------------
// Module-init PATH resolution (fail-fast on missing binary)
// ---------------------------------------------------------------------------

/**
 * Resolve the cerebro binary path from PATH exactly once at module load.
 *
 * Returns the resolved path string, or throws if the binary is absent or
 * the version check fails the stale-shim regex.
 *
 * WHY NOT absolute-path caching per HS-022?
 * That discipline (resolveAbsPath → realpathSync → cache) is scoped to HS-022
 * for pi-claude-code-cli. HS-009 uses a simpler one-time PATH lookup per the
 * both-reviewer scope-expansion rule: we resolve once at init, store the
 * result, and reuse it for all subsequent calls.
 *
 * @internal
 */
export function resolveCerebroPath(): string {
  // Use `which` (POSIX) to resolve cerebro from PATH.
  // On Windows this would be `where`, but pi-cerebro targets macOS/Linux.
  try {
    const resolved = execFileSync("which", ["cerebro"], { stdio: "pipe" })
      .toString()
      .trim();
    if (!resolved) {
      throw new Error("which returned empty string");
    }
    return resolved;
  } catch (err) {
    const msg = err instanceof Error ? sanitise(err.message) : "unknown error";
    throw new Error(`[pi-cerebro] cerebro binary not found on PATH: ${msg}`);
  }
}

/**
 * Validate the cerebro binary at `binaryPath`:
 *   1. Exits 0 on `cerebro --version`.
 *   2. stdout matches the stale-shim regex /\d+\.\d+/.
 *
 * Throws if either check fails. Callers (src/index.ts) catch this to
 * suppress tool registration when the binary is absent or stale.
 *
 * WHY validate at module init?
 * Surface the problem at extension load time with a clear error message,
 * rather than on the first tool call during a session (Tech Lead N3).
 */
export function validateCerebroCli(binaryPath: string): void {
  let versionOut: string;
  try {
    versionOut = execFileSync(binaryPath, ["--version"], {
      stdio: "pipe",
      timeout: 5000,
    }).toString();
  } catch (err) {
    const msg = err instanceof Error ? sanitise(err.message) : "unknown error";
    throw new Error(`[pi-cerebro] cerebro --version failed: ${msg}`);
  }

  // Stale-shim defence: version string must contain /\d+\.\d+/
  if (!VERSION_PATTERN.test(versionOut)) {
    const safeOut = sanitise(versionOut);
    throw new Error(
      `[pi-cerebro] cerebro --version output did not match expected pattern (stale shim?): "${safeOut}"`
    );
  }
}

// ---------------------------------------------------------------------------
// Cached binary path — resolved once at module init.
// Exported for test inspection only.
// ---------------------------------------------------------------------------
let _resolvedBinaryPath: string | null = null;
let _initError: Error | null = null;

/**
 * Initialise the module-level binary resolution. Called once from
 * src/index.ts at extension load time. Idempotent (second call is a no-op).
 *
 * After a successful init, getCerebroPath() returns the resolved path.
 * After a failed init, getCerebroPath() returns null (no binary).
 *
 * @internal exported for test injection.
 */
export function initCerebroPath(): void {
  if (_resolvedBinaryPath !== null || _initError !== null) return; // idempotent
  try {
    const p = resolveCerebroPath();
    validateCerebroCli(p);
    _resolvedBinaryPath = p;
  } catch (err) {
    _initError = err instanceof Error ? err : new Error(String(err));
    // sanitise err.message before logging; the message already carries a [pi-cerebro] prefix
    // from resolveCerebroPath() / validateCerebroCli() — do not double-prefix.
    const safeMsg = sanitise(_initError.message);
    console.error(`${safeMsg} — tools will not be registered.`);
  }
}

/** Returns the resolved cerebro binary path, or null if init failed. */
export function getCerebroPath(): string | null {
  return _resolvedBinaryPath;
}

/**
 * Reset module state — for test isolation ONLY.
 * Production code must never call this.
 * @internal
 */
export function __resetCerebroPathForTests(): void {
  _resolvedBinaryPath = null;
  _initError = null;
}

// ---------------------------------------------------------------------------
// Spawn wrappers
// ---------------------------------------------------------------------------

/**
 * Run `cerebro recall <query> -p <projectDir> --limit <limit> --format md`.
 *
 * SECURITY: argv-array form only. No shell interpolation. `query` and
 * `projectDir` are positional/flag arguments in the argv array — they
 * are never concatenated into a shell string.
 *
 * @param query       Free-text semantic query.
 * @param projectDir  Absolute project dir for the -p flag.
 *                    Must be non-empty (caller's responsibility).
 * @param limit       Max results (default 10).
 * @param binaryPath  Path to the cerebro binary (from getCerebroPath()).
 */
export function runRecall(
  query: string,
  projectDir: string,
  limit: number = 10,
  binaryPath: string
): RecallResult {
  try {
    const stdout = execFileSync(
      binaryPath,
      ["recall", query, "-p", projectDir, "--limit", String(limit), "--format", "md"],
      { stdio: "pipe", timeout: 15000 }
    ).toString();
    return { stdout, exitCode: 0 };
  } catch (err: unknown) {
    const spawnErr = err as NodeJS.ErrnoException & { status?: number; stderr?: Buffer };
    const exitCode = spawnErr.status ?? null;
    const stderr = spawnErr.stderr?.toString() ?? "";
    throw new Error(
      `[pi-cerebro] cerebro recall failed (exit ${exitCode}): ${sanitise(stderr || (err instanceof Error ? err.message : String(err)))}`
    );
  }
}

/**
 * Run `cerebro add <content> -p <projectDir> --type <type> --importance <imp>
 *      [--beads-id <beadsId>]`.
 *
 * Parses the new node ID from stdout (format: `<hex-id> [type] ...`).
 * Security: nodeId captured by a bounded hex regex to prevent injection
 * from a malicious subprocess output (Security S-N3).
 *
 * @param content     Content to store.
 * @param projectDir  Absolute project dir for the -p flag.
 * @param type        Node type (default "episode").
 * @param importance  Importance weight 0.0–1.0 (default 0.75).
 * @param binaryPath  Path to the cerebro binary.
 * @param beadsId     Optional beads task id for forensic linkage (HS-039).
 *                    When non-null and non-empty, appended as ["--beads-id", beadsId]
 *                    via argv-array discipline (TL-N2 — NOT inline-concat form).
 *                    Trim+validation is the Go CLI layer's responsibility (N-S1);
 *                    this belt-and-braces guard skips the flag if beadsId is empty.
 */
export function runAdd(
  content: string,
  projectDir: string,
  type: string = "episode",
  importance: number = 0.75,
  binaryPath: string,
  beadsId: string | null = null
): AddResult {
  // TL-N2: argv-array discipline — build the base argv, then conditionally
  // push ["--beads-id", beadsId] as SEPARATE tokens. Never inline-concat.
  const argv = [
    "add",
    content,
    "-p",
    projectDir,
    "--type",
    type,
    "--importance",
    String(importance),
  ];
  if (beadsId !== null && beadsId !== "") {
    argv.push("--beads-id", beadsId);
  }
  try {
    const stdout = execFileSync(binaryPath, argv, {
      stdio: "pipe",
      timeout: 15000,
    }).toString();

    // S-N3: bounded hex capture — 8 to 64 hex chars, anchored at start of line.
    // Defends against arbitrary output from a compromised or stubbed binary.
    const nodeIdMatch = stdout.match(/^([0-9a-f]{8,64})/m);
    const nodeId = nodeIdMatch?.[1] ?? null;

    return { nodeId, exitCode: 0 };
  } catch (err: unknown) {
    const spawnErr = err as NodeJS.ErrnoException & { status?: number; stderr?: Buffer };
    const exitCode = spawnErr.status ?? null;
    const stderr = spawnErr.stderr?.toString() ?? "";
    throw new Error(
      `[pi-cerebro] cerebro add failed (exit ${exitCode}): ${sanitise(stderr || (err instanceof Error ? err.message : String(err)))}`
    );
  }
}

/**
 * Run `cerebro recall --prime -p <projectDir>` to prime memories at session
 * start.
 *
 * Parses the primed count from the output line matching `primed: N memories`
 * (N5 resolution: parse cerebro's stdout token first; fall back to non-empty
 * line count). This means either cerebro's own log line or plain output lines
 * serve as the count source — whichever cerebro emits.
 *
 * @param projectDir  Absolute project dir for the -p flag.
 * @param binaryPath  Path to the cerebro binary.
 */
export function runBootPrime(projectDir: string, binaryPath: string): BootPrimeResult {
  let stdout: string;
  try {
    stdout = execFileSync(
      binaryPath,
      ["recall", "--prime", "-p", projectDir],
      { stdio: "pipe", timeout: 20000 }
    ).toString();
  } catch (err: unknown) {
    const spawnErr = err as NodeJS.ErrnoException & { status?: number; stderr?: Buffer };
    const exitCode = spawnErr.status ?? null;
    const stderr = spawnErr.stderr?.toString() ?? "";
    throw new Error(
      `[pi-cerebro] cerebro recall --prime failed (exit ${exitCode}): ${sanitise(stderr || (err instanceof Error ? err.message : String(err)))}`
    );
  }

  // N5: parse `primed: N memories` token first (from cerebro's own log line)
  const primedTokenMatch = stdout.match(/primed:\s*(\d+)\s+memor/i);
  if (primedTokenMatch) {
    const count = parseInt(primedTokenMatch[1] ?? "0", 10);
    return { primedCount: count, stdout };
  }

  // Fallback: count non-empty lines (for older cerebro versions or alternate output)
  const nonEmptyLines = stdout.split("\n").filter((l) => l.trim().length > 0).length;
  return { primedCount: nonEmptyLines, stdout };
}
