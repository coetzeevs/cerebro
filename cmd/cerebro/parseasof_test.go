package main

import (
	"testing"
	"time"
)

// TestParseAsOf_RFC3339 verifies RFC3339 input parses and normalizes to UTC.
func TestParseAsOf_RFC3339(t *testing.T) {
	got, err := parseAsOf("2026-06-17T14:30:00Z")
	if err != nil {
		t.Fatalf("parseAsOf RFC3339: %v", err)
	}
	want := time.Date(2026, 6, 17, 14, 30, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("got %v want %v", got, want)
	}
	if got.Location() != time.UTC {
		t.Errorf("result not UTC-normalized: %v", got.Location())
	}
}

// TestParseAsOf_RFC3339WithOffsetNormalizesToUTC verifies a non-Z offset is
// converted to UTC.
func TestParseAsOf_RFC3339WithOffsetNormalizesToUTC(t *testing.T) {
	got, err := parseAsOf("2026-06-17T14:30:00+02:00")
	if err != nil {
		t.Fatalf("parseAsOf offset: %v", err)
	}
	// 14:30+02:00 == 12:30 UTC.
	want := time.Date(2026, 6, 17, 12, 30, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("got %v want %v", got, want)
	}
	if got.Location() != time.UTC {
		t.Errorf("result not UTC-normalized: %v", got.Location())
	}
}

// TestParseAsOf_DateOnly verifies date-only input is interpreted as midnight UTC.
func TestParseAsOf_DateOnly(t *testing.T) {
	got, err := parseAsOf("2026-06-17")
	if err != nil {
		t.Fatalf("parseAsOf date-only: %v", err)
	}
	want := time.Date(2026, 6, 17, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("got %v want %v", got, want)
	}
	if got.Location() != time.UTC {
		t.Errorf("result not UTC-normalized: %v", got.Location())
	}
}

// TestParseAsOf_RejectsBadInput verifies garbage/empty/partial/out-of-range
// input returns an error and NEVER panics (Security guardrail).
func TestParseAsOf_RejectsBadInput(t *testing.T) {
	bad := []string{
		"",                               // empty
		"   ",                            // whitespace
		"not-a-date",                     // garbage
		"2026-13-40",                     // out-of-range month/day
		"2026-06",                        // partial (year-month only)
		"2026/06/17",                     // wrong separators
		"17-06-2026",                     // wrong order
		"2026-06-17 14",                  // partial time, unsupported layout
		"2026-06-17T25:00:00Z",           // out-of-range hour
		"garbage'); DROP TABLE edges;--", // SQL-ish payload must be rejected, not bound
	}
	for _, in := range bad {
		t.Run(in, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("parseAsOf panicked on %q: %v", in, r)
				}
			}()
			_, err := parseAsOf(in)
			if err == nil {
				t.Errorf("parseAsOf(%q) expected error, got nil", in)
			}
		})
	}
}
