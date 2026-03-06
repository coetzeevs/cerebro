# Cerebro

Local-first persistent memory system for AI agents. SQLite-backed with vector search (sqlite-vec).

## Development

```bash
# Build
go build ./cmd/cerebro

# Test
go test ./... -race

# Test with coverage
go test ./... -race -coverprofile=coverage.out
go tool cover -func=coverage.out

# Lint
golangci-lint run
```

## Project Structure

```
cmd/cerebro/       CLI (Cobra commands)
brain/             Public API (Brain type)
internal/store/    SQLite storage, schema, CRUD, vector search
internal/embed/    Embedding provider interface + implementations
```

## Key Patterns

- **CGO required**: mattn/go-sqlite3 needs a C compiler (`xcode-select --install` on Mac)
- **TDD (strict)**: Always write a failing test before implementing. Red → Green → Refactor. No production code without a covering test.
- **Functional options**: `brain.WithImportance(0.8)`, `brain.WithContent("updated")`
- **Building-block CLI**: Low-level commands composed by the calling agent
- **Pre-commit hooks**: Install with `pre-commit install` (requires [pre-commit](https://pre-commit.com/))

## Conventions

- Keep test fixtures in `testdata/` directories
- Use `t.TempDir()` for test databases — no cleanup needed
- Node types: `episode`, `concept`, `procedure`, `reflection`
- Format flag: `--format md` (default) or `--format json`
