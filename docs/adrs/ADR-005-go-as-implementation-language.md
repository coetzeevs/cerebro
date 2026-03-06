# ADR-005: Go as Implementation Language

## Status
Proposed

## Context

Cerebro needs an implementation language for the CLI tool and core library. Requirements:

- **Single binary distribution** — no runtime dependencies, no package managers, no virtualenvs
- **Performance** — fast startup, low memory overhead, efficient SQLite operations
- **Type safety** — the memory lifecycle (reconciliation, decay, graph traversal) involves complex state management that benefits from compile-time guarantees
- **Ecosystem fit** — good SQLite bindings, HTTP client for Ollama/Voyage API, protobuf/gRPC support for future service mode
- **Maintainability** — reasonable learning curve, readable codebase

## Decision

Implement Cerebro in **Go**.

## Alternatives Considered

### 1. Python
- **Pros:** Fastest to prototype. Best Ollama SDK (`ollama` package). Best SQLite ecosystem (`sqlite3` stdlib, `sqlite-vec` pip package). Largest AI/ML community.
- **Rejected because:** Runtime dependency (Python + pip + virtualenv). Slow startup (~200-500ms for Python process vs ~5ms for Go). Distribution is painful — packaging a Python CLI for end users requires PyInstaller/Nuitka/etc., which are fragile. No compile-time type safety. GIL limits concurrency for background operations (embedding queue, consolidation).

### 2. Rust
- **Pros:** Best performance. Memory safety without GC. Single binary. Excellent SQLite bindings (`rusqlite`). Strong type system.
- **Rejected because:** Higher learning curve. Slower development velocity for a project in design phase where APIs will change frequently. Borrow checker friction for graph data structures. The performance difference vs Go is irrelevant for Cerebro's workload (SQLite I/O bound, not CPU bound).

### 3. TypeScript (Node.js / Bun)
- **Pros:** Wide orchestrator ecosystem compatibility (many agent frameworks are JS/TS). Good developer experience.
- **Rejected because:** Runtime dependency (Node.js or Bun). Larger binary if bundled. SQLite bindings exist (`better-sqlite3`) but sqlite-vec integration is less mature. No protobuf/gRPC without additional tooling.

## Why Go Specifically

### Single Binary
`go build` produces a statically linked binary (with CGO for sqlite-vec). No runtime, no dependencies. `cerebro` is one file that works on any machine of the target OS/arch.

### SQLite + sqlite-vec Integration
Go has mature SQLite bindings via `mattn/go-sqlite3` (CGO-based). sqlite-vec can be loaded as an extension:

```go
import (
    "database/sql"
    _ "github.com/mattn/go-sqlite3"
)

db, err := sql.Open("sqlite3", "brain.sqlite?_journal_mode=WAL")
db.Exec("SELECT load_extension('vec0')")
```

**CGO requirement:** sqlite-vec is a C extension, so the build requires CGO. This means cross-compilation needs a C cross-compiler (e.g., `zig cc`). This is a well-understood constraint with established tooling.

**Alternative:** `modernc.org/sqlite` is a pure-Go SQLite implementation (transpiled from C). However, loading C extensions like sqlite-vec still requires CGO. The pure-Go path is not viable for our use case.

### Ollama / API Provider Integration
Ollama exposes an HTTP API. Go's `net/http` stdlib handles this natively — no SDK dependency needed:

```go
resp, err := http.Post("http://localhost:11434/api/embeddings",
    "application/json",
    bytes.NewReader(payload))
```

Voyage AI and OpenAI similarly expose REST APIs. A thin provider interface:

```go
type EmbeddingProvider interface {
    Embed(ctx context.Context, text string) ([]float32, error)
    Dimensions() int
    Model() string
}
```

### Protobuf / gRPC (Future)
Go has first-class protobuf and gRPC support via `google.golang.org/protobuf` and `google.golang.org/grpc`. When/if Cerebro adds a service mode (section 10.3 of the architecture), the proto definitions compile directly to Go server/client code. No additional tooling beyond `protoc-gen-go`.

### Concurrency Model
Go's goroutines and channels are well-suited for:
- Background embedding queue processing (pending embeddings when provider was unavailable)
- Opportunistic consolidation (run consolidation in background while recall returns immediately)
- Parallel embedding during migration (re-embed N nodes concurrently)

## Project Structure

```
cerebro/
  cmd/cerebro/            # CLI entrypoint (thin wrapper)
  brain/                  # Public Go API (Brain type, operations)
  internal/
    store/                # SQLite + sqlite-vec operations
    embed/                # Embedding provider abstraction
      ollama/             # Ollama provider
      voyage/             # Voyage AI provider
      noop/               # No-op provider (graph-only mode)
    lifecycle/            # Decay, consolidation, eviction logic
    graph/                # Graph traversal and scoring
    reconcile/            # Mem0-style ADD/UPDATE/DELETE/NOOP
  proto/                  # gRPC service definitions (future)
  docs/                   # Architecture docs (existing)
```

The `brain/` package is the stable public API. `internal/` packages are implementation details that can change without breaking consumers.

## Consequences

### Positive
- **Zero runtime dependencies.** Users download one binary. No Python, no Node, no Docker.
- **Fast startup.** ~5ms to launch vs ~200-500ms for Python. Matters when the orchestrator invokes `cerebro recall` on every task dispatch.
- **Type safety.** The memory lifecycle (reconciliation state machine, decay calculations, graph traversal) benefits from compile-time checks. Refactoring is safe.
- **Native concurrency.** Goroutines for background embedding, parallel migration, opportunistic consolidation.
- **Cross-platform builds.** `GOOS=linux GOARCH=amd64 go build` (with CGO cross-compilation via zig).
- **Future gRPC path.** When non-Go orchestrators need native integration, the service mode is straightforward to add.

### Negative
- **CGO required.** sqlite-vec is a C extension. CGO complicates cross-compilation (need C toolchain for each target). Mitigation: use `zig cc` as the C compiler for cross-compilation, or provide pre-built binaries for common platforms via GitHub Actions.
- **Less AI ecosystem.** Go has fewer AI/ML libraries than Python. Not a problem for Cerebro (we call Ollama/API over HTTP, not running models locally), but limits future extensibility if we ever wanted to run models in-process.
- **Slower iteration than Python.** Compile step adds a few seconds per change. Acceptable for a project past the design phase.

### Risks
- **sqlite-vec Go bindings maturity.** sqlite-vec is primarily used via Python and Node. Go integration via `mattn/go-sqlite3` extension loading is less documented. Mitigation: spike the integration early to validate. The underlying mechanism (SQLite `load_extension`) is well-established.
- **CGO cross-compilation friction.** Building for Linux from macOS (or vice versa) with CGO requires extra tooling. Mitigation: CI builds on native runners per platform, or use zig as a cross-compiler.

## References
- [mattn/go-sqlite3](https://github.com/mattn/go-sqlite3) — Go SQLite bindings (CGO)
- [sqlite-vec](https://github.com/asg017/sqlite-vec) — Vector search extension
- [Ollama API](https://github.com/ollama/ollama/blob/main/docs/api.md) — HTTP API for local models
