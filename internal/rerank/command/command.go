// Package command implements a Reranker backed by a local operator-supplied
// subprocess (agentic-2ixw). cerebro writes one JSON request to the child's
// stdin and reads one JSON response from its stdout:
//
//	stdin:  {"query": "...", "documents": ["doc0", "doc1", ...]}
//	stdout: {"scores": [s0, s1, ...]}   // len(scores) == len(documents), index-aligned
//
// Security discipline (Step 2.5 Security review):
//   - argv-array exec only — the command string is tokenized with strings.Fields
//     (first token = binary, rest = literal args). NEVER a shell ("sh -c"); shell
//     metacharacters in operator content are passed through verbatim, never
//     evaluated (S-LOW-3 / CWE-78). If you need a pipeline, wrap it in your own
//     script and point the command at that script.
//   - exec.CommandContext with an explicit per-call timeout kills a hung child
//     (S-LOW-2 / CWE-400); caller context cancellation also aborts the child.
//   - untrusted content (query + documents) flows on stdin as JSON, never argv.
//   - child stdout is read through an io.LimitReader bound (S-LOW-1 / CWE-1284).
//   - non-finite scores (NaN/±Inf) are rejected so the sort comparator stays a
//     strict weak ordering (S-LOW-1 / CWE-20). On any failure Rerank returns an
//     error and the caller degrades to the pre-rerank composite order.
package command

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os/exec"
	"strings"
	"time"

	"github.com/coetzeevs/cerebro/internal/rerank"
)

// defaultTimeout bounds a single reranker invocation. Cross-encoder scoring of
// ≤50 short documents is sub-second once the model is warm; this generous
// ceiling guards against a child that hangs on first-run model download.
const defaultTimeout = 30 * time.Second

// maxStdoutBytes bounds the child's stdout read. A response is a single JSON
// object holding ≤50 float scores; 4 MiB is far more than enough and caps a
// runaway/buggy child's output without risking OOM.
const maxStdoutBytes = 4 << 20 // 4 MiB

// request is the JSON object written to the child's stdin.
type request struct {
	Query     string   `json:"query"`
	Documents []string `json:"documents"`
}

// response is the JSON object read from the child's stdout.
type response struct {
	Scores []float64 `json:"scores"`
}

// Reranker runs an operator-supplied subprocess to score candidates.
type Reranker struct {
	command string
	env     []string // nil = inherit the parent environment
	timeout time.Duration
}

// Option configures a Reranker.
type Option func(*Reranker)

// WithEnv sets the child environment explicitly (nil inherits the parent's).
func WithEnv(env []string) Option {
	return func(r *Reranker) { r.env = env }
}

// WithTimeout overrides the per-call subprocess timeout.
func WithTimeout(d time.Duration) Option {
	return func(r *Reranker) {
		if d > 0 {
			r.timeout = d
		}
	}
}

// New creates a subprocess reranker for the given command string. The string is
// tokenized argv-style at call time; an empty string yields a reranker whose
// Rerank always errors (so the caller degrades to composite order).
func New(command string, opts ...Option) *Reranker {
	r := &Reranker{command: command, timeout: defaultTimeout}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// Name returns the reranker identifier, including the child binary name.
func (r *Reranker) Name() string {
	fields := strings.Fields(r.command)
	if len(fields) == 0 {
		return "command:(unset)"
	}
	return "command:" + fields[0]
}

// Rerank invokes the subprocess and returns index-aligned scores.
func (r *Reranker) Rerank(ctx context.Context, query string, cands []rerank.Candidate) ([]float64, error) {
	fields := strings.Fields(r.command)
	if len(fields) == 0 {
		return nil, errors.New("rerank command is empty")
	}

	documents := make([]string, len(cands))
	for i := range cands {
		documents[i] = cands[i].Content
	}

	payload, err := json.Marshal(request{Query: query, Documents: documents})
	if err != nil {
		return nil, fmt.Errorf("marshaling rerank request: %w", err)
	}

	// Explicit per-call deadline derived from the caller's context, so both an
	// internal timeout and an external cancellation kill the child.
	runCtx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	// argv-array exec — NEVER a shell. fields[0] is the binary; the rest are
	// literal arguments passed verbatim (no metacharacter interpretation).
	cmd := exec.CommandContext(runCtx, fields[0], fields[1:]...) //nolint:gosec // argv-array, no shell; operator-supplied local command (Model B)
	if r.env != nil {
		cmd.Env = r.env
	}
	cmd.Stdin = bytes.NewReader(payload)

	var stdout bytes.Buffer
	// Bound the stdout read so a runaway child cannot exhaust memory.
	cmd.Stdout = &limitedWriter{w: &stdout, remaining: maxStdoutBytes}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if runCtx.Err() != nil {
			return nil, fmt.Errorf("rerank subprocess aborted: %w", runCtx.Err())
		}
		return nil, fmt.Errorf("rerank subprocess failed: %w (stderr: %s)", err, strings.TrimSpace(stderr.String()))
	}

	var resp response
	dec := json.NewDecoder(io.LimitReader(&stdout, maxStdoutBytes))
	if err := dec.Decode(&resp); err != nil {
		return nil, fmt.Errorf("decoding rerank response: %w", err)
	}

	if len(resp.Scores) != len(cands) {
		return nil, fmt.Errorf("rerank score count %d != candidate count %d", len(resp.Scores), len(cands))
	}

	// Reject non-finite scores: NaN breaks the sort comparator's strict weak
	// ordering; ±Inf would pin a candidate. Either is a corrupt ranking signal.
	if err := validateFiniteScores(resp.Scores); err != nil {
		return nil, err
	}

	return resp.Scores, nil
}

// validateFiniteScores returns an error if any score is NaN or ±Inf. A NaN
// comparator is non-transitive (Go's sort produces an undefined order); ±Inf
// would pin a candidate. Rejecting them lets the caller degrade to composite
// order rather than corrupt the ranking (S-LOW-1 / CWE-20).
func validateFiniteScores(scores []float64) error {
	for i, s := range scores {
		if math.IsNaN(s) || math.IsInf(s, 0) {
			return fmt.Errorf("rerank score %d is non-finite (%v)", i, s)
		}
	}
	return nil
}

// limitedWriter caps the number of bytes written; once the cap is exceeded it
// returns an error, so an unbounded child stdout surfaces as a Rerank failure
// (which degrades to composite order) rather than unbounded memory growth.
type limitedWriter struct {
	w         *bytes.Buffer
	remaining int
}

func (lw *limitedWriter) Write(p []byte) (int, error) {
	if len(p) > lw.remaining {
		return 0, fmt.Errorf("rerank stdout exceeded %d bytes", maxStdoutBytes)
	}
	n, err := lw.w.Write(p)
	lw.remaining -= n
	return n, err
}
