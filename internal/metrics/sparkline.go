package metrics

import "strings"

// Unicode block elements for sparklines: 8 levels from lowest to highest.
var sparkBlocks = []rune{'▁', '▂', '▃', '▄', '▅', '▆', '▇', '█'}

// Sparkline renders a slice of float64 values as a Unicode sparkline string.
// Values are scaled to min/max range across the input. Zero values render as '_'.
// If width < len(values), only the most recent (rightmost) values are shown.
func Sparkline(values []float64, width int) string {
	if len(values) == 0 {
		return ""
	}

	// Truncate to width (keep most recent).
	if width > 0 && len(values) > width {
		values = values[len(values)-width:]
	}

	// Find lo/hi for scaling.
	lo, hi := values[0], values[0]
	for _, v := range values {
		if v < lo {
			lo = v
		}
		if v > hi {
			hi = v
		}
	}

	var b strings.Builder
	span := hi - lo

	for _, v := range values {
		if v == 0 && lo >= 0 {
			b.WriteRune('_')
			continue
		}

		if span == 0 {
			// All values are the same non-zero value.
			b.WriteRune(sparkBlocks[len(sparkBlocks)-1])
			continue
		}

		// Scale to 0..7 index.
		idx := int((v - lo) / span * float64(len(sparkBlocks)-1))
		if idx < 0 {
			idx = 0
		}
		if idx >= len(sparkBlocks) {
			idx = len(sparkBlocks) - 1
		}
		b.WriteRune(sparkBlocks[idx])
	}

	return b.String()
}

// BoolSparkline renders a boolean series as '_' (false) and the given marker rune (true).
// If width < len(values), only the most recent values are shown.
func BoolSparkline(values []bool, width int, marker rune) string {
	if len(values) == 0 {
		return ""
	}

	if width > 0 && len(values) > width {
		values = values[len(values)-width:]
	}

	var b strings.Builder
	for _, v := range values {
		if v {
			b.WriteRune(marker)
		} else {
			b.WriteRune('_')
		}
	}

	return b.String()
}
