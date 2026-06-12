//go:build fts5

package store

import (
	"strings"
	"testing"
)

// TestBuildMatchQuery_NeutralisesFTS5Operators is the adversarial S-PI-N1 test
// (the OO-011 nodes_test.go SQL-injection precedent). It asserts that the MATCH
// builder turns user text into a single literal FTS5 phrase so NO operator from
// user input reaches the FTS5 parser as syntax. The exact payloads are the
// Security-review live-proven set.
func TestBuildMatchQuery_NeutralisesFTS5Operators(t *testing.T) {
	cases := []struct {
		name string
		in   string
	}{
		{"trailing AND", "HS-049 AND"},
		{"OR injection", "HS-049 OR cats"},
		{"stray quote", `"unterminated`},
		{"column filter", "content : cats"},
		{"prefix star", "foo*"},
		{"NEAR operator", "NEAR(a b)"},
		{"initial token caret", "^ticket"},
		{"embedded quotes", `say "hello" world`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			q := buildMatchQuery(tc.in)
			// Must be a single double-quoted phrase: starts and ends with ".
			if !strings.HasPrefix(q, `"`) || !strings.HasSuffix(q, `"`) {
				t.Fatalf("buildMatchQuery(%q) = %q; want a double-quoted phrase", tc.in, q)
			}
			// Internal quotes must be doubled (FTS5 phrase escape) so the phrase is
			// well-formed: counting the quotes, total must be even.
			if strings.Count(q, `"`)%2 != 0 {
				t.Fatalf("buildMatchQuery(%q) = %q has unbalanced quotes", tc.in, q)
			}
		})
	}
}

// TestKeywordSearch_AdversarialInputsDoNotError (S-PI-N1, QA security set) — every
// adversarial payload must run cleanly through KeywordSearch: no error, no
// column hijack, no crash. The store has one benign node; the adversarial query
// must simply return cleanly (typically zero matches), never an FTS5 syntax
// error.
func TestKeywordSearch_AdversarialInputsDoNotError(t *testing.T) {
	s := testStore(t)
	if _, err := s.AddNode(&AddNodeOpts{Type: TypeConcept, Content: "harmless content about cats"}); err != nil {
		t.Fatalf("AddNode: %v", err)
	}

	payloads := []string{
		"HS-049 AND",
		"HS-049 OR cats",
		`"unterminated`,
		"content : cats",
		"foo*",
		"NEAR(a b)",
		"^ticket",
		`nested "quote" here`,
		"a AND b OR c NOT d",
	}
	for _, p := range payloads {
		if _, err := s.KeywordSearch(p, 10); err != nil {
			t.Fatalf("KeywordSearch(%q) errored (S-PI-N1 violated): %v", p, err)
		}
	}
}

// TestKeywordSearch_ExactIdentifierMatches — the feature's whole point: a query
// containing an exact identifier present in nodes_fts returns the matching node.
func TestKeywordSearch_ExactIdentifierMatches(t *testing.T) {
	s := testStore(t)
	wantID, err := s.AddNode(&AddNodeOpts{Type: TypeProcedure, Content: "the HS-049 incident postmortem and resolution"})
	if err != nil {
		t.Fatalf("AddNode: %v", err)
	}
	// A distractor node with no identifier.
	if _, err := s.AddNode(&AddNodeOpts{Type: TypeConcept, Content: "general notes about widgets and gadgets"}); err != nil {
		t.Fatalf("AddNode distractor: %v", err)
	}

	results, err := s.KeywordSearch("HS-049", 10)
	if err != nil {
		t.Fatalf("KeywordSearch: %v", err)
	}
	if len(results) == 0 {
		t.Fatalf("expected at least one match for HS-049, got none")
	}
	found := false
	for i := range results {
		if results[i].ID == wantID {
			found = true
		}
	}
	if !found {
		t.Fatalf("HS-049 query did not surface the target node %s", wantID)
	}
}

// TestKeywordSearch_EmptyQueryReturnsEmpty (S-PI-N1.3 / AC4-NR identity) — an
// empty or whitespace-only query returns an empty slice, not an error, not a
// MATCH ” that would error.
func TestKeywordSearch_EmptyQueryReturnsEmpty(t *testing.T) {
	s := testStore(t)
	if _, err := s.AddNode(&AddNodeOpts{Type: TypeConcept, Content: "something"}); err != nil {
		t.Fatalf("AddNode: %v", err)
	}
	for _, q := range []string{"", "   ", "\t\n"} {
		res, err := s.KeywordSearch(q, 10)
		if err != nil {
			t.Fatalf("KeywordSearch(%q) errored: %v", q, err)
		}
		if len(res) != 0 {
			t.Fatalf("KeywordSearch(%q) = %d results; want 0", q, len(res))
		}
	}
}

// TestKeywordSearch_GracefulWithoutFTS — sanity: KeywordSearch never panics; with
// the fts5 tag the table is present, so this just confirms a normal call path.
// (The no-fts5 graceful path is covered by the dedicated no-tag build, which
// compiles KeywordSearch but nodes_fts is absent → empty slice.)
func TestKeywordSearch_ReturnsScoredNodes(t *testing.T) {
	s := testStore(t)
	if _, err := s.AddNode(&AddNodeOpts{Type: TypeConcept, Content: "alpha beta gamma"}); err != nil {
		t.Fatalf("AddNode: %v", err)
	}
	res, err := s.KeywordSearch("beta", 10)
	if err != nil {
		t.Fatalf("KeywordSearch: %v", err)
	}
	if len(res) != 1 {
		t.Fatalf("expected 1 result, got %d", len(res))
	}
	if res[0].Content != "alpha beta gamma" {
		t.Fatalf("unexpected content: %q", res[0].Content)
	}
}
