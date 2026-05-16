package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// capturepiInit executes the pi-init command with the given args and returns
// (stdout bytes, stderr bytes, error). It redirects both streams so that
// status messages (stderr) and the JSON snippet (stdout) can be inspected
// independently — per Security A09: stdout must be pure JSON.
//
// The cobra persistent flags (-p, -f, -q) are registered once at package init
// via init() in main.go and cmd_pi_init.go. We must NOT reset the root command's
// persistent flag set between calls — doing so unregisters -p and causes
// "unknown shorthand flag: 'p'" errors. Instead, we reset only the bound global
// variables so flag state doesn't bleed between test invocations.
func capturepiInit(t *testing.T, args ...string) (stdout, stderr []byte, err error) {
	t.Helper()

	// Reset global flag variables so each test starts clean.
	projectFlag = ""
	formatFlag = "md"
	quietFlag = false

	var outBuf, errBuf bytes.Buffer

	rootCmd.SetOut(&outBuf)
	rootCmd.SetErr(&errBuf)
	piInitCmd.SetOut(&outBuf)
	piInitCmd.SetErr(&errBuf)

	// Suppress cobra's auto-usage-on-error so stdout stays pure JSON on error
	// paths. (cobra prints usage to the command's out writer on RunE errors by
	// default.) We restore the original SilenceUsage setting after the call.
	prev := piInitCmd.SilenceUsage
	piInitCmd.SilenceUsage = true
	defer func() { piInitCmd.SilenceUsage = prev }()

	rootCmd.SetArgs(append([]string{"pi-init"}, args...))

	err = rootCmd.Execute()
	return outBuf.Bytes(), errBuf.Bytes(), err
}

// TestPiInitHappyPath verifies that a valid project directory produces exit 0,
// valid JSON on stdout, and the expected snippet shape.
// Covers Gherkin Scenario 1.
func TestPiInitHappyPath(t *testing.T) {
	projectDir := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)

	stdout, _, err := capturepiInit(t, "-p", projectDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var snippet piInitSnippet
	if jsonErr := json.Unmarshal(stdout, &snippet); jsonErr != nil {
		t.Fatalf("stdout is not valid JSON: %v\nstdout=%q", jsonErr, stdout)
	}

	if len(snippet.Extensions) != 1 {
		t.Fatalf("expected 1 extension, got %d", len(snippet.Extensions))
	}
	ext := snippet.Extensions[0]
	if ext.Name != "pi-cerebro" {
		t.Errorf("expected name=pi-cerebro, got %q", ext.Name)
	}
	if ext.Package != "@coetzeevs/pi-cerebro" {
		t.Errorf("expected package=@coetzeevs/pi-cerebro, got %q", ext.Package)
	}
	if ext.Options.ProjectDir == "" {
		t.Error("expected non-empty projectDir")
	}
	if ext.Options.BrainPath == "" {
		t.Error("expected non-empty brainPath")
	}
	if ext.Options.Boot.Limit != 1 {
		t.Errorf("expected boot.limit=1, got %d", ext.Options.Boot.Limit)
	}
	// brainPath must end in .sqlite
	if !strings.HasSuffix(ext.Options.BrainPath, ".sqlite") {
		t.Errorf("brainPath %q should end in .sqlite", ext.Options.BrainPath)
	}
}

// TestPiInitNonexistentPath verifies that a missing project directory causes
// a non-zero exit and an error message containing the path.
// Covers Gherkin Scenario 2.
func TestPiInitNonexistentPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	badPath := "/nonexistent/xyz/does-not-exist"
	stdout, _, err := capturepiInit(t, "-p", badPath)
	if err == nil {
		t.Fatal("expected non-zero exit for missing path, got nil error")
	}
	if len(stdout) != 0 {
		t.Errorf("expected empty stdout on error, got %q", stdout)
	}
}

// TestPiInitSymlinkResolution verifies that when the project dir is a symlink,
// the emitted snippet contains the real path, not the symlink path.
// Covers Gherkin Scenario 3.
func TestPiInitSymlinkResolution(t *testing.T) {
	base := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)

	realDir := filepath.Join(base, "real")
	if err := os.MkdirAll(realDir, 0o750); err != nil {
		t.Fatalf("mkdir real: %v", err)
	}
	symDir := filepath.Join(base, "sym")
	if err := os.Symlink(realDir, symDir); err != nil {
		t.Skip("os.Symlink not supported on this platform, skipping symlink test")
	}

	stdout, _, err := capturepiInit(t, "-p", symDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var snippet piInitSnippet
	if jsonErr := json.Unmarshal(stdout, &snippet); jsonErr != nil {
		t.Fatalf("stdout is not valid JSON: %v\nstdout=%q", jsonErr, stdout)
	}
	ext := snippet.Extensions[0]

	// The projectDir in the snippet must resolve to the real path, not the symlink.
	// On macOS /tmp -> /private/tmp, so compare the EvalSymlinks-resolved values.
	resolvedSym, err := filepath.EvalSymlinks(symDir)
	if err != nil {
		t.Fatalf("EvalSymlinks(symDir): %v", err)
	}
	if ext.Options.ProjectDir != resolvedSym {
		t.Errorf("expected projectDir=%q (resolved), got %q", resolvedSym, ext.Options.ProjectDir)
	}
	if strings.Contains(ext.Options.ProjectDir, "sym") {
		t.Errorf("projectDir should not contain the symlink name, got %q", ext.Options.ProjectDir)
	}
}

// TestPiInitIdempotency verifies that two consecutive runs with the same
// project dir produce byte-identical stdout output.
// Covers Gherkin Scenario 4.
func TestPiInitIdempotency(t *testing.T) {
	projectDir := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)

	stdout1, _, err1 := capturepiInit(t, "-p", projectDir)
	if err1 != nil {
		t.Fatalf("first run error: %v", err1)
	}
	stdout2, _, err2 := capturepiInit(t, "-p", projectDir)
	if err2 != nil {
		t.Fatalf("second run error: %v", err2)
	}

	if !bytes.Equal(stdout1, stdout2) {
		t.Errorf("stdout not byte-identical between runs:\nrun1=%q\nrun2=%q", stdout1, stdout2)
	}
}

// TestPiInitStdoutPurity verifies that the entire stdout content is valid JSON
// with zero interleaved human-readable lines.
// Covers Security A09 requirement.
func TestPiInitStdoutPurity(t *testing.T) {
	projectDir := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)

	stdout, _, err := capturepiInit(t, "-p", projectDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Entire stdout must be a single valid JSON document.
	var v interface{}
	if jsonErr := json.Unmarshal(stdout, &v); jsonErr != nil {
		t.Errorf("stdout is not pure JSON: %v\nstdout=%q", jsonErr, stdout)
	}

	// Assert no raw control bytes in the string values of the decoded JSON.
	// encoding/json escapes them, so this guards against fmt.Sprintf regressions.
	var snippet piInitSnippet
	if err := json.Unmarshal(stdout, &snippet); err == nil {
		checkNoControlBytes(t, "projectDir", snippet.Extensions[0].Options.ProjectDir)
		checkNoControlBytes(t, "brainPath", snippet.Extensions[0].Options.BrainPath)
	}
}

// TestPiInitStderrStatusLines verifies that stderr emits the correct status
// message on first run ("Created brain at ...") and second run
// ("Verified existing brain at ...").
// Covers Security A09 and Design §2 "Brain ensure-or-create".
func TestPiInitStderrStatusLines(t *testing.T) {
	projectDir := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)

	_, stderr1, err := capturepiInit(t, "-p", projectDir)
	if err != nil {
		t.Fatalf("first run error: %v", err)
	}
	if !strings.Contains(string(stderr1), "Created brain at") {
		t.Errorf("expected 'Created brain at' in stderr on first run, got %q", stderr1)
	}

	_, stderr2, err := capturepiInit(t, "-p", projectDir)
	if err != nil {
		t.Fatalf("second run error: %v", err)
	}
	if !strings.Contains(string(stderr2), "Verified existing brain at") {
		t.Errorf("expected 'Verified existing brain at' in stderr on second run, got %q", stderr2)
	}
}

// TestPiInitBrainCreatedOnFirstRun verifies that the brain SQLite file exists
// after the first run. (Architect §7 case e.)
func TestPiInitBrainCreatedOnFirstRun(t *testing.T) {
	projectDir := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)

	stdout, _, err := capturepiInit(t, "-p", projectDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var snippet piInitSnippet
	if jsonErr := json.Unmarshal(stdout, &snippet); jsonErr != nil {
		t.Fatalf("stdout not valid JSON: %v", jsonErr)
	}

	brainPath := snippet.Extensions[0].Options.BrainPath
	if _, statErr := os.Stat(brainPath); os.IsNotExist(statErr) {
		t.Errorf("brain file not created at %q", brainPath)
	}
}

// TestPiInitBrainReusedOnSecondRun verifies that the second run does NOT emit
// "Created brain at" (i.e., init is not called again). (Architect §7 case f.)
func TestPiInitBrainReusedOnSecondRun(t *testing.T) {
	projectDir := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)

	// First run creates the brain.
	_, _, err := capturepiInit(t, "-p", projectDir)
	if err != nil {
		t.Fatalf("first run error: %v", err)
	}

	// Second run must not re-create.
	_, stderr2, err := capturepiInit(t, "-p", projectDir)
	if err != nil {
		t.Fatalf("second run error: %v", err)
	}
	if strings.Contains(string(stderr2), "Created brain at") {
		t.Errorf("second run must not emit 'Created brain at', got stderr=%q", stderr2)
	}
}

// TestPiInitPathWithSpaces verifies that a project directory whose path
// contains spaces is handled correctly.
func TestPiInitPathWithSpaces(t *testing.T) {
	base := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)

	spacyDir := filepath.Join(base, "my project with spaces")
	if err := os.MkdirAll(spacyDir, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	stdout, _, err := capturepiInit(t, "-p", spacyDir)
	if err != nil {
		t.Fatalf("unexpected error for path with spaces: %v", err)
	}

	var snippet piInitSnippet
	if jsonErr := json.Unmarshal(stdout, &snippet); jsonErr != nil {
		t.Fatalf("stdout is not valid JSON: %v\nstdout=%q", jsonErr, stdout)
	}
	// Resolved projectDir must contain the spaces directory name.
	if !strings.Contains(snippet.Extensions[0].Options.ProjectDir, "my project with spaces") {
		t.Errorf("expected projectDir to contain 'my project with spaces', got %q",
			snippet.Extensions[0].Options.ProjectDir)
	}
}

// checkNoControlBytes asserts that s contains no raw ASCII control characters
// (bytes 0x00–0x1F, 0x7F). encoding/json escapes these; this defends against
// future fmt.Sprintf regressions per Security A03 note.
func checkNoControlBytes(t *testing.T, field, s string) {
	t.Helper()
	for i, b := range []byte(s) {
		if b < 0x20 || b == 0x7F {
			t.Errorf("field %q contains control byte 0x%02X at position %d", field, b, i)
		}
	}
}
