package main

// cmd_origin_test.go — TDD tests for origin identity at the CLI (agentic-goc7)
// and the typed-relation registry commands (agentic-8l2g).
//
// Origin derivation contract: the CLI stamps only what it OBSERVES —
//   channel  = "cli" (the write demonstrably came through the CLI)
//   host     = os.Hostname() (the machine the write ran on)
//   session  = $CEREBRO_ORIGIN_SESSION, else $CLAUDE_SESSION_ID, else unset
//   actor    = $CEREBRO_ORIGIN_ACTOR, else unset (NOT observable — never guessed)
// --origin-* flags override every derived value. An explicit empty flag value
// clears the derived default to NULL.

import (
	"os"
	"strings"
	"testing"

	"github.com/coetzeevs/cerebro/brain"
	"github.com/coetzeevs/cerebro/internal/store"
)

func setupOriginTest(t *testing.T) (projectDir string) {
	t.Helper()
	projectDir = setupAddTest(t)
	// Origin env must be deterministic regardless of the operator's shell.
	t.Setenv("CEREBRO_ORIGIN_ACTOR", "")
	t.Setenv("CEREBRO_ORIGIN_SESSION", "")
	t.Setenv("CLAUDE_SESSION_ID", "")
	return projectDir
}

func nodeByContent(t *testing.T, projectDir, content string) *store.Node {
	t.Helper()
	nodes := listNodesForProject(t, projectDir)
	for i := range nodes {
		if nodes[i].Content == content {
			return &nodes[i]
		}
	}
	t.Fatalf("no stored node with content %q", content)
	return nil
}

// Derived defaults: channel and host are observed facts and are stamped even
// with no flags; actor stays unset (unobservable).
func TestAddOrigin_DerivedDefaults(t *testing.T) {
	projectDir := setupOriginTest(t)

	rootCmd.SetArgs([]string{"add", "derived origin memory"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("add: %v", err)
	}

	n := nodeByContent(t, projectDir, "derived origin memory")
	if n.OriginChannel != "cli" {
		t.Errorf("origin_channel: got %q want \"cli\"", n.OriginChannel)
	}
	hostname, _ := os.Hostname()
	if hostname != "" && n.OriginHost != hostname {
		t.Errorf("origin_host: got %q want %q", n.OriginHost, hostname)
	}
	if n.OriginActor != "" {
		t.Errorf("origin_actor should be unset without env/flag, got %q", n.OriginActor)
	}
}

// Env vars supply actor and session.
func TestAddOrigin_EnvDerivation(t *testing.T) {
	projectDir := setupOriginTest(t)
	t.Setenv("CEREBRO_ORIGIN_ACTOR", "claude-code")
	t.Setenv("CLAUDE_SESSION_ID", "sess-env-1")

	rootCmd.SetArgs([]string{"add", "env origin memory"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("add: %v", err)
	}

	n := nodeByContent(t, projectDir, "env origin memory")
	if n.OriginActor != "claude-code" {
		t.Errorf("origin_actor: got %q want \"claude-code\"", n.OriginActor)
	}
	if n.OriginSession != "sess-env-1" {
		t.Errorf("origin_session: got %q want \"sess-env-1\"", n.OriginSession)
	}
}

// Flags override every derived value.
func TestAddOrigin_FlagsOverride(t *testing.T) {
	projectDir := setupOriginTest(t)
	t.Setenv("CEREBRO_ORIGIN_ACTOR", "env-actor")

	rootCmd.SetArgs([]string{"add", "flag origin memory",
		"--origin-actor", "flag-actor", "--origin-channel", "hook",
		"--origin-session", "sess-flag", "--origin-host", "elsewhere"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("add: %v", err)
	}

	n := nodeByContent(t, projectDir, "flag origin memory")
	if n.OriginActor != "flag-actor" || n.OriginChannel != "hook" ||
		n.OriginSession != "sess-flag" || n.OriginHost != "elsewhere" {
		t.Errorf("flags did not override derivation: %+v", n)
	}
}

// Supersede stamps the superseder's identity the same way.
func TestSupersedeOrigin_Stamped(t *testing.T) {
	projectDir := setupOriginTest(t)

	rootCmd.SetArgs([]string{"add", "original memory"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("add: %v", err)
	}
	oldID := nodeByContent(t, projectDir, "original memory").ID

	rootCmd.SetArgs([]string{"supersede", oldID, "replacement memory",
		"--origin-actor", "superseder"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("supersede: %v", err)
	}

	n := nodeByContent(t, projectDir, "replacement memory")
	if n.OriginActor != "superseder" || n.OriginChannel != "cli" {
		t.Errorf("supersede origin not stamped: %+v", n)
	}
}

// ---- 8l2g: cerebro relation add/list/rm ----

func TestRelationCommand_CRUD(t *testing.T) {
	projectDir := setupOriginTest(t)

	rootCmd.SetArgs([]string{"relation", "add", "blocks", "--class", "structural"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("relation add: %v", err)
	}

	brainPath := brain.ProjectPath(projectDir)
	b, err := brain.Open(brainPath)
	if err != nil {
		t.Fatalf("brain.Open: %v", err)
	}
	rels, err := b.ListRelations()
	if err != nil {
		t.Fatalf("ListRelations: %v", err)
	}
	_ = b.Close()
	found := false
	for _, r := range rels {
		if r.Name == "blocks" && r.TraversalClass == "structural" {
			found = true
		}
	}
	if !found {
		t.Fatalf("relation add did not register blocks/structural: %+v", rels)
	}

	rootCmd.SetArgs([]string{"relation", "rm", "blocks"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("relation rm: %v", err)
	}
	b2, _ := brain.Open(brainPath)
	ok, _ := b2.RelationRegistered("blocks")
	_ = b2.Close()
	if ok {
		t.Error("relation rm did not remove blocks")
	}

	// list must at least run and include the seeds.
	rootCmd.SetArgs([]string{"relation", "list"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("relation list: %v", err)
	}
}

// Edge creation with an unregistered relation succeeds but warns on stderr
// (warn-not-error: the registry is advisory).
func TestEdge_UnregisteredRelationWarns(t *testing.T) {
	projectDir := setupOriginTest(t)

	rootCmd.SetArgs([]string{"add", "edge src"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("add src: %v", err)
	}
	rootCmd.SetArgs([]string{"add", "edge dst"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("add dst: %v", err)
	}
	srcID := nodeByContent(t, projectDir, "edge src").ID
	dstID := nodeByContent(t, projectDir, "edge dst").ID

	// Capture stderr around the edge command.
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	rootCmd.SetArgs([]string{"edge", srcID, dstID, "totally_novel_rel"})
	execErr := rootCmd.Execute()

	_ = w.Close()
	os.Stderr = oldStderr
	var sb strings.Builder
	buf := make([]byte, 4096)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			sb.Write(buf[:n])
		}
		if err != nil {
			break
		}
	}

	if execErr != nil {
		t.Fatalf("edge with unregistered relation must SUCCEED (warn-not-error), got: %v", execErr)
	}
	if !strings.Contains(sb.String(), "totally_novel_rel") || !strings.Contains(sb.String(), "not registered") {
		t.Errorf("expected unregistered-relation warning on stderr, got: %q", sb.String())
	}

	// A registered relation must NOT warn.
	oldStderr = os.Stderr
	r2, w2, _ := os.Pipe()
	os.Stderr = w2
	rootCmd.SetArgs([]string{"edge", srcID, dstID, "supports"})
	execErr = rootCmd.Execute()
	_ = w2.Close()
	os.Stderr = oldStderr
	var sb2 strings.Builder
	for {
		n, err := r2.Read(buf)
		if n > 0 {
			sb2.Write(buf[:n])
		}
		if err != nil {
			break
		}
	}
	if execErr != nil {
		t.Fatalf("edge with registered relation: %v", execErr)
	}
	if strings.Contains(sb2.String(), "not registered") {
		t.Errorf("registered relation must not warn, stderr: %q", sb2.String())
	}
}
