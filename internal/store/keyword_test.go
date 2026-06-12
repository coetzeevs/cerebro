//go:build fts5

package store

import (
	"strings"
	"testing"
)

// TestBuildMatchQuery_NeutralisesFTS5Operators is the adversarial S-PI-N1 test
// (the OO-011 nodes_test.go SQL-injection precedent). It asserts that the MATCH
// builder turns user text into a per-token-quoted FTS5 expression so NO operator
// from user input reaches the FTS5 parser as syntax. The exact single-term
// payloads are the Security-review live-proven set; they remain passing after
// the tokenize-OR rework (PART 1 of the agentic-2lak rework).
//
// Contract after the rework: the query is split on whitespace, EACH token is
// wrapped in a double-quoted FTS5 phrase (internal `"` doubled per the FTS5
// phrase-escape), and the tokens are joined with ` OR ` (our operator, never the
// user's). Every term is therefore a literal phrase — `AND/OR/NOT/NEAR/*/:/^/(`
// in user text become literal phrase content, not operators.
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
			assertWellFormedMatch(t, tc.in, q)
		})
	}
}

// TestBuildMatchQuery_MultiWordInjectionStaysSafe (S-PI-N1, multi-word rework) —
// proves that an FTS5 injection payload embedded INSIDE a multi-word query still
// cannot inject an operator: every whitespace token is individually quoted, so
// the user's `"`, `OR`, `*`, `NEAR(...)`, etc. become literal phrase content.
// These are the exact payloads named in the rework brief.
func TestBuildMatchQuery_MultiWordInjectionStaysSafe(t *testing.T) {
	cases := []struct {
		name string
		in   string
	}{
		{"embedded quote-OR break", `foo HS-049" OR x`},
		{"NEAR in multi-word", "a NEAR(b c)"},
		{"prefix star in multi-word", "x* y"},
		{"multi AND OR NOT", "alpha AND beta OR gamma NOT delta"},
		{"trailing stray quote multi", `report HS-049 "`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			q := buildMatchQuery(tc.in)
			assertWellFormedMatch(t, tc.in, q)
			// Every whitespace token must surface as its own quoted phrase joined
			// by ` OR `. A bareword FTS5 operator (AND/OR/NOT/NEAR) must NOT appear
			// unquoted — i.e. it must never sit between two spaces without quotes.
			// We assert the join shape: phrases separated by " OR " and each phrase
			// starts and ends with a double-quote.
			for _, phrase := range strings.Split(q, " OR ") {
				if !strings.HasPrefix(phrase, `"`) || !strings.HasSuffix(phrase, `"`) {
					t.Fatalf("buildMatchQuery(%q) = %q has an unquoted token %q (operator could inject)", tc.in, q, phrase)
				}
			}
		})
	}
}

// TestBuildMatchQuery_TokenizesOnWhitespace pins the exact tokenize-OR output for
// representative inputs — the load-bearing change of the rework. A multi-word
// query must become a disjunction of per-token phrases (so a rare identifier
// token matches inside a multi-word query, which the old whole-query phrase
// quoting prevented).
func TestBuildMatchQuery_TokenizesOnWhitespace(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"single identifier unchanged", "HS-049", `"HS-049"`},
		{"multi-word disjunction", "OO-015 determinism wire", `"OO-015" OR "determinism" OR "wire"`},
		{"user OR becomes literal", "cats OR dogs", `"cats" OR "OR" OR "dogs"`},
		{"collapses internal whitespace", "  a   b  ", `"a" OR "b"`},
		{"doubles internal quote", `say "hi"`, `"say" OR """hi"""`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := buildMatchQuery(tc.in); got != tc.want {
				t.Fatalf("buildMatchQuery(%q) = %q; want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestBuildMatchQuery_EmptyTokensProduceEmpty — empty / all-whitespace input has
// no tokens, so the builder returns "" (the KeywordSearch empty-query graceful
// path handles this before it would ever reach MATCH).
func TestBuildMatchQuery_EmptyTokensProduceEmpty(t *testing.T) {
	for _, in := range []string{"", "   ", "\t\n  "} {
		if got := buildMatchQuery(in); got != "" {
			t.Fatalf("buildMatchQuery(%q) = %q; want empty string", in, got)
		}
	}
}

// assertWellFormedMatch checks the structural invariants every buildMatchQuery
// output must satisfy: a non-empty input yields a `"`-delimited expression with
// an even number of double-quotes (every phrase opened is closed; internal
// quotes are doubled).
func assertWellFormedMatch(t *testing.T, in, q string) {
	t.Helper()
	if !strings.HasPrefix(q, `"`) || !strings.HasSuffix(q, `"`) {
		t.Fatalf("buildMatchQuery(%q) = %q; want a double-quoted phrase expression", in, q)
	}
	if strings.Count(q, `"`)%2 != 0 {
		t.Fatalf("buildMatchQuery(%q) = %q has unbalanced quotes", in, q)
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

// TestKeywordSearch_MultiWordSurfacesIdentifier — the rework's whole point: a
// MULTI-WORD query containing an exact identifier now yields a non-empty keyword
// lane that includes the identifier node. Under the old whole-query phrase
// quoting this returned ZERO matches (the 4 terms had to be adjacent); under the
// tokenize-OR rework the identifier term matches on its own and bm25() floats it
// up.
func TestKeywordSearch_MultiWordSurfacesIdentifier(t *testing.T) {
	s := testStore(t)
	wantID, err := s.AddNode(&AddNodeOpts{Type: TypeProcedure, Content: "OO-015 determinism wire enforcement decision record"})
	if err != nil {
		t.Fatalf("AddNode: %v", err)
	}
	// Distractors that share SOME of the multi-word terms but not the identifier.
	if _, err := s.AddNode(&AddNodeOpts{Type: TypeConcept, Content: "general notes about widgets and gadgets"}); err != nil {
		t.Fatalf("AddNode distractor: %v", err)
	}
	if _, err := s.AddNode(&AddNodeOpts{Type: TypeConcept, Content: "a wire was tested for determinism elsewhere"}); err != nil {
		t.Fatalf("AddNode distractor2: %v", err)
	}

	// A realistic multi-word query a user/agent would actually type.
	results, err := s.KeywordSearch("what did OO-015 decide about determinism wire enforcement", 10)
	if err != nil {
		t.Fatalf("KeywordSearch: %v", err)
	}
	if len(results) == 0 {
		t.Fatalf("multi-word query returned EMPTY keyword lane (the defect); want the OO-015 node")
	}
	found := false
	for i := range results {
		if results[i].ID == wantID {
			found = true
		}
	}
	if !found {
		t.Fatalf("multi-word query did not surface the OO-015 node %s; got %d results", wantID, len(results))
	}
}

// TestKeywordSearch_MultiWordInjectionDoesNotError — the multi-word injection
// payloads from the rework brief must run cleanly through KeywordSearch (no FTS5
// syntax error, no crash) even when an injection attempt is embedded among other
// words.
func TestKeywordSearch_MultiWordInjectionDoesNotError(t *testing.T) {
	s := testStore(t)
	if _, err := s.AddNode(&AddNodeOpts{Type: TypeConcept, Content: "harmless content about cats and HS-049"}); err != nil {
		t.Fatalf("AddNode: %v", err)
	}
	payloads := []string{
		`foo HS-049" OR x`,
		"a NEAR(b c)",
		"x* y",
		"alpha AND beta OR gamma NOT delta",
		`report HS-049 "`,
		"prefix* OR suffix*",
	}
	for _, p := range payloads {
		if _, err := s.KeywordSearch(p, 10); err != nil {
			t.Fatalf("KeywordSearch(%q) errored (S-PI-N1 multi-word violated): %v", p, err)
		}
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
