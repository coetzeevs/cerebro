package metrics

import "testing"

func TestSparkline_Empty(t *testing.T) {
	got := Sparkline(nil, 10)
	if got != "" {
		t.Errorf("Sparkline(nil) = %q, want empty", got)
	}
}

func TestSparkline_SingleValue(t *testing.T) {
	got := Sparkline([]float64{5.0}, 10)
	if got != "█" {
		t.Errorf("Sparkline([5.0]) = %q, want █", got)
	}
}

func TestSparkline_AllSame(t *testing.T) {
	got := Sparkline([]float64{3.0, 3.0, 3.0}, 10)
	if got != "███" {
		t.Errorf("Sparkline(all same) = %q, want ███", got)
	}
}

func TestSparkline_AllZero(t *testing.T) {
	got := Sparkline([]float64{0, 0, 0}, 10)
	if got != "___" {
		t.Errorf("Sparkline(all zero) = %q, want ___", got)
	}
}

func TestSparkline_MinMax(t *testing.T) {
	// With values [0, 1], should produce _ and █
	got := Sparkline([]float64{0, 1}, 10)
	if len([]rune(got)) != 2 {
		t.Fatalf("expected 2 chars, got %d: %q", len([]rune(got)), got)
	}
	runes := []rune(got)
	if runes[0] != '_' {
		t.Errorf("first char = %c, want _", runes[0])
	}
	if runes[1] != '█' {
		t.Errorf("second char = %c, want █", runes[1])
	}
}

func TestSparkline_GradientAscending(t *testing.T) {
	values := []float64{0, 1, 2, 3, 4, 5, 6, 7, 8}
	got := Sparkline(values, 20)
	runes := []rune(got)
	if len(runes) != 9 {
		t.Fatalf("expected 9 chars, got %d: %q", len(runes), got)
	}
	// Each subsequent character should be >= the previous.
	for i := 1; i < len(runes); i++ {
		if runes[i] < runes[i-1] {
			t.Errorf("non-ascending at position %d: %c < %c", i, runes[i], runes[i-1])
		}
	}
}

func TestSparkline_WidthTruncation(t *testing.T) {
	values := []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	got := Sparkline(values, 5)
	runes := []rune(got)
	// Should show only the last 5 values.
	if len(runes) != 5 {
		t.Errorf("expected 5 chars (width truncation), got %d: %q", len(runes), got)
	}
}

func TestSparkline_WidthLargerThanData(t *testing.T) {
	values := []float64{1, 2, 3}
	got := Sparkline(values, 20)
	runes := []rune(got)
	// Should show all 3 values (no padding).
	if len(runes) != 3 {
		t.Errorf("expected 3 chars, got %d: %q", len(runes), got)
	}
}

func TestBoolSparkline_Empty(t *testing.T) {
	got := BoolSparkline(nil, 10, '*')
	if got != "" {
		t.Errorf("BoolSparkline(nil) = %q, want empty", got)
	}
}

func TestBoolSparkline_Mixed(t *testing.T) {
	values := []bool{false, false, true, false, true}
	got := BoolSparkline(values, 10, '*')
	if got != "__*_*" {
		t.Errorf("BoolSparkline = %q, want __*_*", got)
	}
}

func TestBoolSparkline_WidthTruncation(t *testing.T) {
	values := []bool{true, false, false, true, false, true}
	got := BoolSparkline(values, 3, '*')
	// Last 3 of [true, false, false, true, false, true] = [true, false, true].
	if got != "*_*" {
		t.Errorf("BoolSparkline truncated = %q, want *_* (last 3)", got)
	}
}
