package main

import (
	"path/filepath"
	"testing"

	"github.com/coetzeevs/cerebro/brain"
	"github.com/coetzeevs/cerebro/internal/store"
)

// testBrainForConfig creates a temporary brain for config tests.
func testBrainForConfig(t *testing.T) *brain.Brain {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.sqlite")
	b, err := brain.Init(path, brain.EmbedConfig{Provider: "none"})
	if err != nil {
		t.Fatalf("brain.Init: %v", err)
	}
	t.Cleanup(func() { _ = b.Close() })
	return b
}

// --- Registry tests ---

func TestConfigRegistry_KnownKeys(t *testing.T) {
	// All expected keys must be in the registry.
	expected := []string{
		"prime_limit", "gc_threshold", "search_limit",
		"search_threshold", "recall_threshold",
	}
	for _, key := range expected {
		if _, ok := configRegistry[key]; !ok {
			t.Errorf("expected key %q in configRegistry", key)
		}
	}
}

func TestConfigRegistry_UnknownKey(t *testing.T) {
	if _, ok := configRegistry["nonexistent_key"]; ok {
		t.Error("unexpected key 'nonexistent_key' found in registry")
	}
}

// --- Validation tests ---

func TestConfigValidation_PrimeLimit(t *testing.T) {
	p := configRegistry["prime_limit"]

	// Valid values
	for _, v := range []string{"1", "20", "200"} {
		if err := p.Validate(v); err != nil {
			t.Errorf("Validate(%q) unexpected error: %v", v, err)
		}
	}

	// Invalid values
	for _, v := range []string{"0", "-1", "abc", "3.5", ""} {
		if err := p.Validate(v); err == nil {
			t.Errorf("Validate(%q) expected error, got nil", v)
		}
	}
}

func TestConfigValidation_GCThreshold(t *testing.T) {
	p := configRegistry["gc_threshold"]

	for _, v := range []string{"0", "0.01", "0.5", "1.0"} {
		if err := p.Validate(v); err != nil {
			t.Errorf("Validate(%q) unexpected error: %v", v, err)
		}
	}

	for _, v := range []string{"-0.1", "1.1", "abc", ""} {
		if err := p.Validate(v); err == nil {
			t.Errorf("Validate(%q) expected error, got nil", v)
		}
	}
}

func TestConfigValidation_SearchThreshold(t *testing.T) {
	p := configRegistry["search_threshold"]

	if err := p.Validate("0.7"); err != nil {
		t.Errorf("Validate(0.7) unexpected error: %v", err)
	}
	if err := p.Validate("-0.1"); err == nil {
		t.Error("Validate(-0.1) expected error, got nil")
	}
}

func TestConfigValidation_SearchLimit(t *testing.T) {
	p := configRegistry["search_limit"]

	if err := p.Validate("10"); err != nil {
		t.Errorf("Validate(10) unexpected error: %v", err)
	}
	if err := p.Validate("0"); err == nil {
		t.Error("Validate(0) expected error, got nil")
	}
}

func TestConfigValidation_RecallThreshold(t *testing.T) {
	p := configRegistry["recall_threshold"]

	if err := p.Validate("0.3"); err != nil {
		t.Errorf("Validate(0.3) unexpected error: %v", err)
	}
	if err := p.Validate("2.0"); err == nil {
		t.Error("Validate(2.0) expected error, got nil")
	}
}

// --- Resolve tests ---

func TestResolveConfigInt_Default(t *testing.T) {
	b := testBrainForConfig(t)
	// No config set — should return the fallback default.
	got := resolveConfigInt(b.Store(), "prime_limit", 20)
	if got != 20 {
		t.Errorf("resolveConfigInt = %d, want 20", got)
	}
}

func TestResolveConfigInt_Override(t *testing.T) {
	b := testBrainForConfig(t)
	if err := b.Store().SetMeta("config.prime_limit", "30"); err != nil {
		t.Fatalf("SetMeta: %v", err)
	}
	got := resolveConfigInt(b.Store(), "prime_limit", 20)
	if got != 30 {
		t.Errorf("resolveConfigInt = %d, want 30", got)
	}
}

func TestResolveConfigInt_InvalidStoredValue(t *testing.T) {
	b := testBrainForConfig(t)
	// Store a non-integer — should fall back to default.
	if err := b.Store().SetMeta("config.prime_limit", "not_a_number"); err != nil {
		t.Fatalf("SetMeta: %v", err)
	}
	got := resolveConfigInt(b.Store(), "prime_limit", 20)
	if got != 20 {
		t.Errorf("resolveConfigInt with invalid stored = %d, want fallback 20", got)
	}
}

func TestResolveConfigFloat_Default(t *testing.T) {
	b := testBrainForConfig(t)
	got := resolveConfigFloat(b.Store(), "gc_threshold", 0.01)
	if got != 0.01 {
		t.Errorf("resolveConfigFloat = %f, want 0.01", got)
	}
}

func TestResolveConfigFloat_Override(t *testing.T) {
	b := testBrainForConfig(t)
	if err := b.Store().SetMeta("config.gc_threshold", "0.05"); err != nil {
		t.Fatalf("SetMeta: %v", err)
	}
	got := resolveConfigFloat(b.Store(), "gc_threshold", 0.01)
	if got != 0.05 {
		t.Errorf("resolveConfigFloat = %f, want 0.05", got)
	}
}

func TestResolveConfigFloat_InvalidStoredValue(t *testing.T) {
	b := testBrainForConfig(t)
	if err := b.Store().SetMeta("config.gc_threshold", "bad"); err != nil {
		t.Fatalf("SetMeta: %v", err)
	}
	got := resolveConfigFloat(b.Store(), "gc_threshold", 0.01)
	if got != 0.01 {
		t.Errorf("resolveConfigFloat with invalid stored = %f, want fallback 0.01", got)
	}
}

// --- Set/Get round-trip via store ---

func TestConfigSetGetRoundTrip(t *testing.T) {
	b := testBrainForConfig(t)

	// Set a config value.
	if err := b.Store().SetMeta("config.prime_limit", "42"); err != nil {
		t.Fatalf("SetMeta: %v", err)
	}

	// Read it back.
	val, err := b.Store().GetMeta("config.prime_limit")
	if err != nil {
		t.Fatalf("GetMeta: %v", err)
	}
	if val != "42" {
		t.Errorf("GetMeta = %q, want 42", val)
	}
}

func TestConfigReset(t *testing.T) {
	b := testBrainForConfig(t)

	// Set then delete.
	if err := b.Store().SetMeta("config.prime_limit", "42"); err != nil {
		t.Fatalf("SetMeta: %v", err)
	}

	if err := b.Store().DeleteMeta("config.prime_limit"); err != nil {
		t.Fatalf("DeleteMeta: %v", err)
	}

	// Should return empty (no row).
	val, err := b.Store().GetMeta("config.prime_limit")
	if err != nil {
		t.Fatalf("GetMeta after delete: %v", err)
	}
	if val != "" {
		t.Errorf("GetMeta after delete = %q, want empty", val)
	}

	// Resolve should return default.
	got := resolveConfigInt(b.Store(), "prime_limit", 20)
	if got != 20 {
		t.Errorf("resolveConfigInt after reset = %d, want 20", got)
	}
}

// --- Backward compatibility ---

func TestConfigBackwardCompat_OldBrainNoConfigRows(t *testing.T) {
	// A brain created before config support has no config.* rows.
	// All resolves should return defaults.
	b := testBrainForConfig(t)

	if got := resolveConfigInt(b.Store(), "prime_limit", 20); got != 20 {
		t.Errorf("prime_limit = %d, want 20", got)
	}
	if got := resolveConfigFloat(b.Store(), "gc_threshold", 0.01); got != 0.01 {
		t.Errorf("gc_threshold = %f, want 0.01", got)
	}
	if got := resolveConfigFloat(b.Store(), "search_threshold", 0.7); got != 0.7 {
		t.Errorf("search_threshold = %f, want 0.7", got)
	}
	if got := resolveConfigInt(b.Store(), "search_limit", 10); got != 10 {
		t.Errorf("search_limit = %d, want 10", got)
	}
	if got := resolveConfigFloat(b.Store(), "recall_threshold", 0.3); got != 0.3 {
		t.Errorf("recall_threshold = %f, want 0.3", got)
	}
}

// --- Export/import portability ---

func TestConfigSurvivesExportImport(t *testing.T) {
	b := testBrainForConfig(t)

	// Set a config value.
	if err := b.Store().SetMeta("config.prime_limit", "42"); err != nil {
		t.Fatalf("SetMeta: %v", err)
	}

	// Export.
	bundle, err := b.Store().Export()
	if err != nil {
		t.Fatalf("Export: %v", err)
	}

	// Verify config key is in meta.
	if bundle.Meta["config.prime_limit"] != "42" {
		t.Errorf("exported meta config.prime_limit = %q, want 42", bundle.Meta["config.prime_limit"])
	}

	// Import into a fresh brain.
	dir := t.TempDir()
	path2 := filepath.Join(dir, "imported.sqlite")
	b2, err := brain.Init(path2, brain.EmbedConfig{Provider: "none"})
	if err != nil {
		t.Fatalf("brain.Init for import: %v", err)
	}
	defer func() { _ = b2.Close() }()

	if _, err := b2.Store().Import(bundle, store.ImportOptions{OnConflict: store.ConflictSkip}); err != nil {
		t.Fatalf("Import: %v", err)
	}

	// Verify config survived.
	got := resolveConfigInt(b2.Store(), "prime_limit", 20)
	if got != 42 {
		t.Errorf("resolveConfigInt after import = %d, want 42", got)
	}
}
