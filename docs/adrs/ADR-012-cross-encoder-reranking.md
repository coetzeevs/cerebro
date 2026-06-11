# ADR-012: Cross-Encoder Reranking via Local Subprocess

**Date:** 2026-06-11
**Status:** Accepted
**Ticket:** agentic-2ixw
**Supersedes:** —
**Superseded by:** —

## Context

Cerebro retrieves recall candidates by composite score
(`0.35·relevance + 0.25·importance + 0.25·recency + 0.15·structural`,
`internal/store/search.go:compositeScore`). The recall-quality epic (`agentic-0ndn`,
ruler shipped in `agentic-x183` ADR-011) identifies cross-encoder
*rerank-after-retrieve* as the highest-evidence recall lever: over-retrieve a
wider candidate set by composite score, rerank with a cross-encoder, cut to the
final limit. The Distracting Effect (Levi et al., ACL 2025) and Cohere Rerank 4
(+17.2pp MRR@3, +12.1pp Recall@5 over unreranked hybrid) motivate it.

The load-bearing constraint is cerebro's distribution character: a **single
codesigned binary** shipped via a Homebrew cask, with `CGO_ENABLED=1` only for
sqlite-vec and **no bundled shared libraries** (`.goreleaser.yml`:
`binaries: [cerebro]`, post-install xattr quarantine strip only). Model B
forbids runtime cloud LLM calls. So the mechanism question is: how does cerebro
run a cross-encoder without (a) breaking the single-binary distribution, (b)
bundling a model, or (c) calling a cloud API?

## Decision: a local operator-supplied subprocess (`CEREBRO_RERANK_COMMAND`), default OFF

A `rerank.Reranker` interface (mirroring `embed.Provider`) with two
implementations: `noop` (identity order, the disabled-path no-op) and `command`
(a local subprocess invoked over a JSON-on-stdin / JSON-on-stdout contract).
Reranking is gated by the `rerank_enabled` brain config key (default `false`).
When disabled, the recall path is structurally identical to today's
`VectorSearch → ExpandGraph → filter`; the subprocess is never spawned.

The contract:

```
stdin:  {"query": "<text>", "documents": ["doc0", ...]}
stdout: {"scores": [s0, ...]}   # len == len(documents), index-aligned
```

cerebro bundles **no model**. The operator supplies the reranker command (e.g. a
small Python wrapper around a MiniLM cross-encoder; a reference one ships under
`docs/evals/reranker/`). This is the symmetric extension of an already-validated
pattern: cerebro already delegates its heavy ML (embeddings) to a local external
process — Ollama, an HTTP daemon.

## Rejected: in-process ONNX (`yalue/onnxruntime_go`)

Validated via context7 (`/yalue/onnxruntime_go` Requirements): onnxruntime_go
needs the `onnxruntime` **shared library at runtime**, loaded via `dlopen` on
macOS/Linux through `ort.SetSharedLibraryPath(...)`, plus cgo. cerebro's cask
ships a *bare* binary with no bundled libs. Adopting ONNX would mean:

- a second native runtime dependency alongside sqlite-vec;
- new goreleaser/cask plumbing to download, stage, and codesign a ~15–30MB
  `.dylib` for both `darwin/arm64` and `darwin/amd64`;
- a cgo/build-tag entanglement that risks repeating the FTS5 build-tag footgun
  (`agentic-2lak`: a CGO feature silently gated behind a tag set nowhere);
- shipping or downloading a model file.

This converts cerebro from "one codesigned binary" into "binary + shared lib +
model asset" — precisely the property the single-binary constraint forbids.
**Rejected on first-principles footprint grounds, for a default-OFF lever.**

## Rejected: pure-Go cross-encoder

No mature, production-grade pure-Go cross-encoder inference path exists (absent
from `go.mod`/`go.sum`; context7 surfaced only Python sentence-transformers and
the cgo-bound onnxruntime_go). Building tokenization + transformer inference in
pure Go is out of scope for one ticket. Rejected on feasibility.

## Rejected: the existing Ollama daemon

context7 (`/ollama/ollama`) confirms Ollama has **no rerank/cross-encoder
endpoint** — its API is embed/generate/chat only. Reranking cannot be delegated
to the existing embedding daemon (true negative).

## Footprint justification (DoD item 4)

- **Binary size delta: 0 bytes.** No new Go dependency, no linked native lib, no
  embedded model. `internal/rerank/` is pure Go (exec + JSON). `go.mod`/`go.sum`
  and `.goreleaser.yml` are unchanged.
- **Runtime memory (cerebro process): negligible.** One `exec.Cmd`, a JSON
  marshal of ≤50 `{query, document}` pairs, a JSON unmarshal of ≤50 scores. The
  reranker model's memory lives in the operator-supplied subprocess, outside
  cerebro's address space and distribution. When disabled (default), nothing is
  spawned — a true zero-cost no-op.
- **First-run model download: none in cerebro's path.** cerebro bundles no
  model. The operator's subprocess owns any download. Offline/missing-reranker
  behaviour: if enabled but the command is unset/missing/crashing or returns
  malformed/short/non-finite output, the reranker **degrades to the pre-rerank
  composite order** with a one-line stderr warning — recall is never worse than
  disabled.

## Model-choice ruling

`bge-reranker-v2-m3` (~568MB) is rejected as a bundled/default — unacceptable
near a brew binary, and the subprocess design means cerebro bundles no model
regardless. The documented recommended reranker is a MiniLM-class English
cross-encoder (`cross-encoder/ms-marco-MiniLM-L6-v2`, ~22–90MB); bge-m3 is noted
as an optional higher-quality multilingual choice the operator may wire if they
accept the size. cerebro prescribes the *contract*, not the model.

## Security disposition (Step 2.5 review: 0 CRITICAL / 0 HIGH / 0 MEDIUM / 3 LOW / 3 INFO)

The subprocess is the only new attack surface. The `command` implementation:

- execs argv-array only via `strings.Fields` tokenization — **never a shell**
  (`sh -c`); shell metacharacters in operator content are passed verbatim, not
  evaluated (S-LOW-3 / CWE-78). A test asserts zero `sh`/`-c`/`bash` literals.
- uses `exec.CommandContext` with an explicit 30s timeout that kills a hung
  child (S-LOW-2 / CWE-400); caller-context cancellation also aborts it.
- passes untrusted content (query + documents) on **stdin as JSON**, never argv.
- bounds the stdout read (S-LOW-1 / CWE-1284) and **rejects non-finite scores**
  (NaN/±Inf) so the sort comparator stays a strict weak ordering (CWE-20).
- reads `CEREBRO_RERANK_COMMAND` lazily, only on the enabled branch (S-INFO-2),
  and keeps the *enable gate* env-free (S-INFO-3): an actor who can set env but
  not brain config cannot enable subprocess spawning.

Trust is operator→operator on a single-operator workstation (the operator owns
both the brain and the command). Re-grades to MEDIUM/HIGH if cerebro becomes
multi-tenant or `rerank_command` becomes non-owner-settable (S-INFO-1).

## Consequences

- New `internal/rerank/` package; `rerank_enabled`/`rerank_command` config keys;
  the reranker composes inside `Brain.Search`/`SearchWithGlobal`, so `cerebro
  eval` and `cerebro recall` exercise it with no command-layer change.
- `compositeScore` weights are untouched (out of scope) — the reranker reorders
  the already-scored candidate set; it does not recompute any composite score.
- Single-shot exec per query (no persistent reranker daemon) is accepted for a
  recall-quality lever; a daemon optimisation is a future ticket if latency hurts.
