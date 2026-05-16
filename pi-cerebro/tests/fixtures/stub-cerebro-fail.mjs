#!/usr/bin/env node
/**
 * Failing stub `cerebro` binary for pi-cerebro tests.
 *
 * All invocations exit 1 with an error message on stderr.
 * Used to test error-path handling in cerebro-cli.ts wrappers.
 *
 * Mode 0755 required.
 */

process.stderr.write("stub-cerebro-fail: simulated failure\n");
process.exit(1);
