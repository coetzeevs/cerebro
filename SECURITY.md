# Security

## Security Model

Cerebro is a local-first storage system. All data resides in SQLite files on the local filesystem. There are no network listeners, no servers, and no remote data transmission in the core library.

### Data Storage

- **SQLite with WAL mode** — Write-Ahead Logging for crash safety and concurrent read access
- **Single-writer model** — one process writes to a given brain at a time, enforced by SQLite's locking
- **File permissions** — brain files are created with user-only permissions; the calling application is responsible for directory permissions

### Embedding Provider Keys

Cerebro supports external embedding providers (Voyage AI, Ollama). API keys for these providers:

- Are passed via `brain.EmbedConfig` at initialization time
- Are stored in the brain's metadata table (SQLite) for reopening
- Are **not** logged or included in export bundles
- For CLI usage, should be set via environment variables (e.g., `VOYAGE_API_KEY`)

### Export/Import

- `cerebro export` outputs the full brain contents (nodes, edges, metadata) as JSON, SQL, or SQLite copy
- Exported data may contain sensitive information stored in memory nodes
- Import uses conflict resolution (skip or replace) and does not execute any embedded content

### What Cerebro Does NOT Do

- Does not make network requests (except via embedding providers when configured)
- Does not execute code from memory content
- Does not evaluate or interpret memory node content — it is treated as opaque text
- Does not manage authentication or authorization — that is the responsibility of the consuming application

## Reporting Vulnerabilities

If you discover a security vulnerability in Cerebro, please report it responsibly:

1. **Do not** open a public GitHub issue
2. Email the maintainer directly at the address listed in the repository profile
3. Include a description of the vulnerability, steps to reproduce, and potential impact
4. Allow reasonable time for a fix before public disclosure

We aim to acknowledge reports within 48 hours and release fixes promptly.
