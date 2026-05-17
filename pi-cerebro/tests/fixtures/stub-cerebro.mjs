#!/usr/bin/env node
/**
 * Stub `cerebro` binary for pi-cerebro tests.
 *
 * Behaviour:
 *   cerebro --version   → prints "2.0.0\n" (satisfies /\d+\.\d+/ regex), exits 0
 *   cerebro recall ...  → prints a stub recall result, exits 0
 *   cerebro add ...     → prints a fake node ID line, exits 0
 *   any other args      → prints the args as JSON, exits 0
 *
 * Mode 0755 required (executable bit must be set by the create-fixture script).
 *
 * Opt-in argv logging: when PI_CEREBRO_STUB_ARGV_LOG is set to an absolute
 * path, the stub writes process.argv.slice(2) as a JSON array to that path
 * using flag:"w" (overwrite, no append). The env var must point to an absolute
 * path; if it contains shell metacharacters or is not absolute, the log is
 * silently skipped (TL-N4 / Security advisory). [HS-024]
 */

import { writeFileSync } from "node:fs";
import { isAbsolute } from "node:path";

const args = process.argv.slice(2);
const subcommand = args[0];

// Opt-in argv log (PI_CEREBRO_STUB_ARGV_LOG) — test seam only, never in src/
// Guard: only write when env var is set to a non-empty absolute path without
// shell metacharacters (prevents accidental I/O in tests that don't set it).
const argvLogPath = process.env.PI_CEREBRO_STUB_ARGV_LOG;
if (argvLogPath && isAbsolute(argvLogPath) && !/[;&|`$<>]/.test(argvLogPath)) {
  writeFileSync(argvLogPath, JSON.stringify(args), { flag: "w" });
}

if (subcommand === "--version") {
  process.stdout.write("2.0.0\n");
  process.exit(0);
}

if (subcommand === "recall") {
  const primeFlag = args.includes("--prime");
  if (primeFlag) {
    // Prime output: cerebro recall --prime emits a "primed: N memories" line
    process.stdout.write("primed: 3 memories\n## abc123 [episode] score=0.9\ntest memory 1\n\n## def456 [procedure] score=0.8\ntest memory 2\n");
  } else {
    // Regular recall: emit a couple of markdown memory blocks
    process.stdout.write("## abc123ef [episode] score=0.900\nTest memory content from stub.\n");
  }
  process.exit(0);
}

if (subcommand === "add") {
  // Emit a fake node ID (hex, 8+ chars) on the first line
  process.stdout.write("abcdef12 [episode] added successfully\n");
  process.exit(0);
}

// Default: echo args as JSON
process.stdout.write(JSON.stringify({ args }) + "\n");
process.exit(0);
