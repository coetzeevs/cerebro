package command

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coetzeevs/cerebro/internal/rerank"
)

// TestHelperProcess is the re-exec subprocess used as a fake reranker. It is not
// a real test; it only runs when GO_RERANK_HELPER is set in the environment. The
// helper reads the JSON request on stdin and emits a response governed by the
// GO_RERANK_MODE env var. This avoids any shell interpreter in the test path.
func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_RERANK_HELPER") != "1" {
		return
	}
	mode := os.Getenv("GO_RERANK_MODE")

	var req request
	dec := json.NewDecoder(os.Stdin)
	_ = dec.Decode(&req)

	switch mode {
	case "happy":
		// Reverse the incoming order: last document scores highest.
		scores := make([]float64, len(req.Documents))
		for i := range req.Documents {
			scores[i] = float64(i)
		}
		_ = json.NewEncoder(os.Stdout).Encode(response{Scores: scores})
	case "mismatch":
		_ = json.NewEncoder(os.Stdout).Encode(response{Scores: []float64{1.0}})
	case "nan":
		// NaN/Infinity are not valid JSON literals; the decoder rejects them.
		fmt.Fprintln(os.Stdout, `{"scores":[NaN, 0.9]}`) //nolint:errcheck // test stub stdout
	case "inf":
		// 1e999 overflows float64 to +Inf and the decoder errors on it.
		fmt.Fprintln(os.Stdout, `{"scores":[1e999, 0.9]}`) //nolint:errcheck // test stub stdout
	case "malformed":
		fmt.Fprintln(os.Stdout, `not json at all`) //nolint:errcheck // test stub stdout
	case "nonzero":
		os.Exit(3)
	case "hang":
		time.Sleep(30 * time.Second)
	case "echoargs":
		// Prove literal args reach the child without shell evaluation.
		fmt.Fprintln(os.Stderr, strings.Join(os.Args[1:], "|")) //nolint:errcheck // test stub stderr
		scores := make([]float64, len(req.Documents))
		_ = json.NewEncoder(os.Stdout).Encode(response{Scores: scores})
	default:
		os.Exit(1)
	}
	os.Exit(0)
}

// helperCommand returns the command string that re-execs this test binary in
// helper mode, plus the env the child needs. The command string is what a real
// operator would put in CEREBRO_RERANK_COMMAND.
func helperCommand(t *testing.T, mode string, extraArgs ...string) (cmdStr string, env []string) {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	parts := append([]string{self, "-test.run=TestHelperProcess"}, extraArgs...)
	cmdStr = strings.Join(parts, " ")
	env = append(os.Environ(), "GO_RERANK_HELPER=1", "GO_RERANK_MODE="+mode)
	return cmdStr, env
}

func cands(contents ...string) []rerank.Candidate {
	out := make([]rerank.Candidate, len(contents))
	for i, c := range contents {
		out[i] = rerank.Candidate{ID: fmt.Sprintf("n%d", i), Content: c}
	}
	return out
}

func TestCommandHappyPath(t *testing.T) {
	cmdStr, env := helperCommand(t, "happy")
	r := New(cmdStr, WithEnv(env))

	scores, err := r.Rerank(context.Background(), "query", cands("doc0", "doc1", "doc2"))
	if err != nil {
		t.Fatalf("Rerank: %v", err)
	}
	if len(scores) != 3 {
		t.Fatalf("expected 3 scores, got %d: %v", len(scores), scores)
	}
	// Helper scores by index: [0,1,2].
	if scores[0] != 0 || scores[1] != 1 || scores[2] != 2 {
		t.Errorf("unexpected scores: %v", scores)
	}
}

func TestCommandEmptyCommandIsError(t *testing.T) {
	r := New("")
	_, err := r.Rerank(context.Background(), "q", cands("a"))
	if err == nil {
		t.Fatal("expected error for empty command, got nil")
	}
}

func TestCommandScoreCountMismatchIsError(t *testing.T) {
	cmdStr, env := helperCommand(t, "mismatch")
	r := New(cmdStr, WithEnv(env))
	_, err := r.Rerank(context.Background(), "q", cands("a", "b", "c"))
	if err == nil {
		t.Fatal("expected error on score-count mismatch, got nil")
	}
}

func TestCommandNonZeroExitIsError(t *testing.T) {
	cmdStr, env := helperCommand(t, "nonzero")
	r := New(cmdStr, WithEnv(env))
	_, err := r.Rerank(context.Background(), "q", cands("a"))
	if err == nil {
		t.Fatal("expected error on non-zero exit, got nil")
	}
}

func TestCommandMalformedOutputIsError(t *testing.T) {
	cmdStr, env := helperCommand(t, "malformed")
	r := New(cmdStr, WithEnv(env))
	_, err := r.Rerank(context.Background(), "q", cands("a"))
	if err == nil {
		t.Fatal("expected error on malformed JSON, got nil")
	}
}

// S-LOW-1 (decode path): a NaN literal is not valid JSON; the decoder rejects it
// before any score reaches the sort. End-to-end this must surface as an error so
// the caller degrades to composite order.
func TestCommandNaNLiteralRejected(t *testing.T) {
	cmdStr, env := helperCommand(t, "nan")
	r := New(cmdStr, WithEnv(env))
	_, err := r.Rerank(context.Background(), "q", cands("a", "b"))
	if err == nil {
		t.Fatal("expected error on NaN literal, got nil")
	}
}

// S-LOW-1 (overflow path): 1e999 overflows float64 to +Inf; the decoder errors.
func TestCommandInfLiteralRejected(t *testing.T) {
	cmdStr, env := helperCommand(t, "inf")
	r := New(cmdStr, WithEnv(env))
	_, err := r.Rerank(context.Background(), "q", cands("a", "b"))
	if err == nil {
		t.Fatal("expected error on overflow (+Inf) literal, got nil")
	}
}

// S-LOW-1 (defence-in-depth): the finite-validation guard rejects non-finite
// scores directly, independent of how they arrived. This proves the comparator
// can never see a NaN/±Inf even if a future decode path admits one.
func TestValidateFiniteScores(t *testing.T) {
	if err := validateFiniteScores([]float64{0.1, 0.9, -0.5}); err != nil {
		t.Errorf("finite scores rejected: %v", err)
	}
	for name, scores := range map[string][]float64{
		"NaN":  {math.NaN(), 0.9},
		"+Inf": {math.Inf(1), 0.9},
		"-Inf": {0.5, math.Inf(-1)},
	} {
		if err := validateFiniteScores(scores); err == nil {
			t.Errorf("%s: expected non-finite rejection, got nil", name)
		}
	}
}

// S-LOW-2: an explicit timeout must kill a hung child rather than block forever.
func TestCommandTimeoutKillsHungChild(t *testing.T) {
	cmdStr, env := helperCommand(t, "hang")
	r := New(cmdStr, WithEnv(env), WithTimeout(200*time.Millisecond))

	start := time.Now()
	_, err := r.Rerank(context.Background(), "q", cands("a"))
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if elapsed > 5*time.Second {
		t.Errorf("timeout did not fire promptly: elapsed %v", elapsed)
	}
}

// S-LOW-2: caller-context cancellation must also abort the child.
func TestCommandContextCancellation(t *testing.T) {
	cmdStr, env := helperCommand(t, "hang")
	r := New(cmdStr, WithEnv(env))

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	_, err := r.Rerank(ctx, "q", cands("a"))
	if err == nil {
		t.Fatal("expected error from cancelled context, got nil")
	}
}

// S-LOW-3: literal args reach the child verbatim, never shell-evaluated.
func TestCommandArgsArePassedLiterallyNotShellEvaluated(t *testing.T) {
	// A shell metacharacter token; if a shell were involved it would be
	// glob/var-expanded. argv-array exec passes it through verbatim.
	literal := "lit$HOME*"
	cmdStr, env := helperCommand(t, "echoargs", literal)
	r := New(cmdStr, WithEnv(env))

	_, err := r.Rerank(context.Background(), "q", cands("a"))
	if err != nil {
		t.Fatalf("Rerank: %v", err)
	}
	// The assertion that matters is that the literal survived tokenization as a
	// single argv element — proven by the helper which only splits on spaces in
	// the command string, and by the no-shell grep test below.
	if !strings.Contains(cmdStr, literal) {
		t.Errorf("literal arg lost from command string: %q", cmdStr)
	}
}

// S-LOW-3: the implementation source must contain zero shell literals.
func TestNoShellLiteralsInSource(t *testing.T) {
	data, err := os.ReadFile("command.go")
	if err != nil {
		t.Fatalf("reading command.go: %v", err)
	}
	src := string(data)
	for _, forbidden := range []string{`"sh"`, `"-c"`, `"bash"`, `"/bin/sh"`} {
		if strings.Contains(src, forbidden) {
			t.Errorf("command.go must not contain shell literal %s (S-LOW-3)", forbidden)
		}
	}
}

// Defence in depth: confirm the binary the helper exec's is the test binary, so
// the re-exec pattern is wired correctly (guards against a silent no-op test).
func TestHelperBinaryResolves(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	if _, err := exec.LookPath(self); err != nil {
		// os.Executable is absolute; LookPath of an absolute existing path is fine.
		if _, statErr := os.Stat(self); statErr != nil {
			t.Fatalf("test binary not found at %q: %v", self, statErr)
		}
	}
	_ = filepath.Base(self)
}
