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
 */

const args = process.argv.slice(2);
const subcommand = args[0];

if (subcommand === "--version") {
  process.stdout.write("2.0.0\n");
  process.exit(0);
}

if (subcommand === "recall") {
  const bootFlag = args.includes("--boot");
  if (bootFlag) {
    // Boot prime output: cerebro recall --boot emits a "primed: N memories" line
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
