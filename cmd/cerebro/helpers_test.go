package main

import "testing"

func TestParseNodeType(t *testing.T) {
	tests := []struct {
		input   string
		want    string
		wantErr bool
	}{
		{"episode", "episode", false},
		{"concept", "concept", false},
		{"procedure", "procedure", false},
		{"reflection", "reflection", false},
		{"Episode", "episode", false},
		{"CONCEPT", "concept", false},
		{"invalid", "", true},
		{"", "", true},
	}

	for _, tt := range tests {
		got, err := parseNodeType(tt.input)
		if tt.wantErr {
			if err == nil {
				t.Errorf("parseNodeType(%q): expected error", tt.input)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseNodeType(%q): unexpected error: %v", tt.input, err)
			continue
		}
		if string(got) != tt.want {
			t.Errorf("parseNodeType(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
