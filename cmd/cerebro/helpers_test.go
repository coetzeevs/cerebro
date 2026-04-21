package main

import (
	"os"
	"testing"

	"github.com/coetzeevs/cerebro/brain"
)

func TestResolveProjectDir_FlagWins(t *testing.T) {
	// Flag should win over env var and cwd
	t.Setenv("CLAUDE_PROJECT_DIR", "/env/path")
	old := projectFlag
	projectFlag = "/flag/path"
	t.Cleanup(func() { projectFlag = old })

	got := resolveProjectDir()
	if got != "/flag/path" {
		t.Errorf("resolveProjectDir() = %q, want /flag/path", got)
	}
}

func TestResolveProjectDir_EnvWins(t *testing.T) {
	// Env var should win when no flag is set
	t.Setenv("CLAUDE_PROJECT_DIR", "/env/path")
	old := projectFlag
	projectFlag = ""
	t.Cleanup(func() { projectFlag = old })

	got := resolveProjectDir()
	if got != "/env/path" {
		t.Errorf("resolveProjectDir() = %q, want /env/path", got)
	}
}

func TestResolveProjectDir_FallbackToCwd(t *testing.T) {
	// With no flag and no env var, should fall back to cwd
	t.Setenv("CLAUDE_PROJECT_DIR", "")
	old := projectFlag
	projectFlag = ""
	t.Cleanup(func() { projectFlag = old })

	cwd, _ := os.Getwd()
	got := resolveProjectDir()
	if got != cwd {
		t.Errorf("resolveProjectDir() = %q, want cwd %q", got, cwd)
	}
}

func TestResolveBrainPath_UsesResolveProjectDir(t *testing.T) {
	// resolveBrainPath should return brain.ProjectPath of resolveProjectDir
	t.Setenv("CLAUDE_PROJECT_DIR", "/env/path")
	old := projectFlag
	projectFlag = ""
	t.Cleanup(func() { projectFlag = old })

	got := resolveBrainPath()
	want := brain.ProjectPath("/env/path")
	if got != want {
		t.Errorf("resolveBrainPath() = %q, want %q", got, want)
	}
}

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
