# Contributing to Cerebro

## Getting Started

1. Fork the repository
2. Clone your fork: `git clone https://github.com/<you>/cerebro.git`
3. Install prerequisites: Go 1.24+, C compiler (`xcode-select --install` on macOS)
4. Install pre-commit hooks: `pre-commit install`
5. Create a feature branch: `git checkout -b feat/your-feature`

## Development Workflow

### Test-Driven Development (Strict)

All changes follow TDD. No exceptions.

1. Write a failing test
2. Write the minimum code to make it pass
3. Refactor
4. Repeat

### Running Checks

```bash
make test         # Run tests with race detector
make lint         # Run golangci-lint
make test-cover   # Tests + coverage report
```

All of these must pass before submitting a PR.

### Pre-commit Hooks

The following run automatically on every commit:

- `golangci-lint` — static analysis
- `gofmt` — formatting
- `go mod tidy` — dependency hygiene
- `go build` — compilation check
- `go test -race -short` — fast test suite

Install with: `pre-commit install`

## Commit Messages

Follow [Conventional Commits](https://www.conventionalcommits.org/):

```
feat: add vector search support for embeddings
fix: correct cosine distance calculation in vec0
docs: update README with export command examples
chore: upgrade golangci-lint to v2
```

Prefix types: `feat`, `fix`, `docs`, `chore`, `test`, `refactor`

## Pull Requests

1. One logical change per PR
2. All CI checks must pass (lint, test, test-short, govulncheck, goreleaser check)
3. Update tests for any changed behavior
4. Update documentation if the public API changes
5. Keep PRs focused — don't bundle unrelated changes

## Code Conventions

- Test fixtures go in `testdata/` directories per package
- Use `t.TempDir()` for test databases — no cleanup needed
- Node types: `episode`, `concept`, `procedure`, `reflection`
- Functional options pattern for `brain.Add()` and `brain.Update()`
- `internal/store/` is not importable by external modules — use `brain/` types

## Architecture

See [docs/architecture/system-architecture.md](docs/architecture/system-architecture.md) and the [ADRs](docs/adrs/) for design decisions.

The key constraint: Cerebro is **pure storage** (Model B per ADR-006). It does not contain LLM reasoning, tool execution, or agent logic. That belongs in consuming applications like [QraftWorx CLI](https://github.com/coetzeevs/qraftworx-cli).
