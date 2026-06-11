#!/usr/bin/env python3
"""Reference cross-encoder reranker for cerebro (agentic-2ixw).

This is a *reference* implementation of the CEREBRO_RERANK_COMMAND contract so
QA/operators can reproduce the before/after recall numbers. cerebro itself
bundles NO model — the operator supplies a reranker subprocess; this is one such
subprocess. It is intentionally tiny and dependency-light.

Contract (read on stdin, write on stdout, both a single JSON object):

    stdin:  {"query": "<query text>", "documents": ["doc0", "doc1", ...]}
    stdout: {"scores": [s0, s1, ...]}   # len(scores) == len(documents)

Higher score = more relevant. Scores must be finite (cerebro rejects NaN/Inf).
argv is never shell-interpreted by cerebro; this script takes no arguments.

Model: cross-encoder/ms-marco-MiniLM-L6-v2 (~22.7M params, ~90MB fp32). It is
downloaded ONCE by sentence-transformers (HuggingFace cache), not by cerebro.
Override with the RERANK_MODEL env var.

Dependencies (install into a venv, NOT system Python):
    python3 -m venv docs/evals/reranker/.venv
    docs/evals/reranker/.venv/bin/pip install sentence-transformers
Then point cerebro at it:
    cerebro config set rerank_enabled true -p <brain>
    export CEREBRO_RERANK_COMMAND="$(pwd)/docs/evals/reranker/.venv/bin/python \
        $(pwd)/docs/evals/reranker/rerank_minilm.py"

Offline / missing-model behaviour: if the model cannot be loaded, this script
exits non-zero with a message on stderr. cerebro then degrades to the
pre-rerank composite order (recall never worse than disabled).
"""

import json
import os
import sys


def main() -> int:
    try:
        req = json.load(sys.stdin)
    except json.JSONDecodeError as exc:
        print(f"rerank: invalid JSON on stdin: {exc}", file=sys.stderr)
        return 1

    query = req.get("query", "")
    documents = req.get("documents", [])
    if not isinstance(documents, list):
        print("rerank: 'documents' must be a list", file=sys.stderr)
        return 1

    # Empty candidate set: emit an empty score list (valid, index-aligned).
    if not documents:
        json.dump({"scores": []}, sys.stdout)
        return 0

    try:
        from sentence_transformers import CrossEncoder
    except ImportError:
        print(
            "rerank: sentence-transformers not installed; "
            "see docs/evals/reranker/README for setup",
            file=sys.stderr,
        )
        return 2

    model_name = os.environ.get("RERANK_MODEL", "cross-encoder/ms-marco-MiniLM-L6-v2")
    try:
        model = CrossEncoder(model_name)
    except Exception as exc:  # noqa: BLE001 — surface any load failure to cerebro
        print(f"rerank: failed to load model {model_name!r}: {exc}", file=sys.stderr)
        return 3

    pairs = [(query, doc) for doc in documents]
    raw_scores = model.predict(pairs)

    # predict() returns numpy float32; cast to plain finite Python floats.
    scores = []
    for s in raw_scores:
        f = float(s)
        if f != f or f in (float("inf"), float("-inf")):  # NaN/Inf guard
            print("rerank: model produced a non-finite score", file=sys.stderr)
            return 4
        scores.append(f)

    json.dump({"scores": scores}, sys.stdout)
    return 0


if __name__ == "__main__":
    sys.exit(main())
