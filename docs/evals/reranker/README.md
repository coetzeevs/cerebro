# Reference cross-encoder reranker (agentic-2ixw)

`rerank_minilm.py` is a **reference** implementation of the
`CEREBRO_RERANK_COMMAND` contract. It lets QA and operators reproduce the
before/after recall numbers in `../rerank-results.md`.

**cerebro bundles no model.** The reranker is an operator-supplied local
subprocess; this is one such subprocess (a thin wrapper around a MiniLM
cross-encoder). It is dependency-light and model-free until you install it.

## Contract

cerebro writes one JSON object to the script's stdin and reads one from stdout:

```
stdin:  {"query": "<text>", "documents": ["doc0", "doc1", ...]}
stdout: {"scores": [s0, s1, ...]}      # len(scores) == len(documents), index-aligned
```

Higher score = more relevant. Scores must be finite — cerebro rejects NaN/±Inf
and degrades to the composite order. cerebro invokes the command argv-array
style (never a shell); this script takes no arguments and reads only stdin.

## Setup

```bash
# From the cerebro repo root.
python3 -m venv docs/evals/reranker/.venv
docs/evals/reranker/.venv/bin/pip install sentence-transformers
```

The model `cross-encoder/ms-marco-MiniLM-L6-v2` (~22.7M params, ~90MB) is
downloaded **once** by sentence-transformers into the HuggingFace cache on first
run — by this script, not by cerebro. Override with `RERANK_MODEL`.

## Enable in cerebro

```bash
cerebro config set rerank_enabled true -p <brain>
export CEREBRO_RERANK_COMMAND="$(pwd)/docs/evals/reranker/.venv/bin/python $(pwd)/docs/evals/reranker/rerank_minilm.py"
cerebro recall "your query" -p <brain> --limit 10
# ...restore when done:
cerebro config set rerank_enabled false -p <brain>
```

Precedence: the `rerank_command` brain-config key wins over the
`CEREBRO_RERANK_COMMAND` env var (config-wins, env-fallback).

## Graceful degradation

The script exits non-zero (with a one-line stderr message) when:
- stdin is not valid JSON (rc 1),
- `sentence-transformers` is not installed (rc 2),
- the model cannot be loaded — e.g. offline with a cold cache (rc 3),
- the model produces a non-finite score (rc 4).

In every case cerebro logs a one-line warning and falls back to the pre-rerank
composite order, so **recall is never worse than disabled.**

## Smoke test (no model needed)

```bash
echo '{"query":"q","documents":[]}' | python3 rerank_minilm.py   # -> {"scores": []}
```
