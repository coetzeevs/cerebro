package main

import (
	"fmt"
	"math"
	"strconv"

	"github.com/coetzeevs/cerebro/internal/store"
	"github.com/spf13/cobra"
)

// configParam defines a known configuration parameter.
type configParam struct {
	Key         string
	Description string
	Default     string
	Validate    func(string) error
}

// configRegistry is the authoritative set of known config keys.
var configRegistry = map[string]configParam{
	"prime_limit": {
		Key:         "prime_limit",
		Description: "Maximum memories loaded at session start (recall --prime)",
		Default:     "20",
		Validate:    validatePositiveInt,
	},
	"gc_threshold": {
		Key:         "gc_threshold",
		Description: "GC eviction threshold (nodes scoring below this are archived)",
		Default:     "0.01",
		Validate:    validateUnitFloat,
	},
	"search_limit": {
		Key:         "search_limit",
		Description: "Maximum results for the search command",
		Default:     "10",
		Validate:    validatePositiveInt,
	},
	"search_threshold": {
		Key:         "search_threshold",
		Description: "Minimum similarity threshold for the search command",
		Default:     "0.7",
		Validate:    validateUnitFloat,
	},
	"recall_threshold": {
		Key:         "recall_threshold",
		Description: "Minimum similarity threshold for recall query mode",
		Default:     "0.3",
		Validate:    validateUnitFloat,
	},
	"indegree_bonus_enabled": {
		Key:         "indegree_bonus_enabled",
		Description: "In-degree structural baseline in search scoring (agentic-do71); default on, set false to disable (t3c9 A/B seam)",
		Default:     "true",
		Validate:    validateBool,
	},
	"stop_guard_enabled": {
		Key:         "stop_guard_enabled",
		Description: "Enable the stop-guard premature-stop detector (disabled by default; operator ruling 2026-08-25 — the hook must also be wired in settings.json)",
		Default:     "false",
		Validate:    validateBool,
	},
	"rerank_enabled": {
		Key:         "rerank_enabled",
		Description: "Enable local cross-encoder reranking of recall candidates (agentic-2ixw)",
		Default:     "false",
		Validate:    validateBool,
	},
	"rerank_command": {
		Key:         "rerank_command",
		Description: "Local reranker subprocess (JSON stdin/stdout); empty = use CEREBRO_RERANK_COMMAND env or disable",
		Default:     "",
		Validate:    validateAny,
	},
	"rerank_fusion": {
		Key:         "rerank_fusion",
		Description: "Combine mode when rerank is enabled: \"rrf\" (Reciprocal Rank Fusion, default — fuses composite+reranker ranks) or \"reorder\" (legacy pure-reorder by reranker score)",
		Default:     "rrf",
		Validate:    validateRerankFusion,
	},
	"bm25_enabled": {
		Key:         "bm25_enabled",
		Description: "Compose BM25/FTS5 keyword recall into the pipeline (agentic-2lak). Default true (always-on when the binary has the fts5 build tag). Set false ONLY as an eval/diagnostic seam to produce the same-session BM25-disabled floor — not an end-user feature toggle.",
		Default:     "true",
		Validate:    validateBool,
	},
	"expand_threshold": {
		Key:         "expand_threshold",
		Description: "Skip graph expansion when the top-1 vector cosine similarity strictly exceeds this value (agentic-73l6). 0.0 disables the condition.",
		Default:     "0.75",
		Validate:    validateUnitFloat,
	},
	"expand_spread_threshold": {
		Key:         "expand_spread_threshold",
		Description: "Skip graph expansion when the full top-K similarity spread (top-1 minus top-K) is strictly below this value (agentic-73l6). 0.0 (default) disables the condition — on the current brain the spread anti-correlates with confidence, so it ships OFF.",
		Default:     "0.0",
		Validate:    validateUnitFloat,
	},
}

// configMetaPrefix is prepended to config keys when storing in schema_meta.
const configMetaPrefix = "config."

// --- Validators ---

func validatePositiveInt(s string) error {
	n, err := strconv.Atoi(s)
	if err != nil {
		return fmt.Errorf("must be a positive integer, got %q", s)
	}
	if n <= 0 {
		return fmt.Errorf("must be a positive integer (> 0), got %d", n)
	}
	return nil
}

func validateUnitFloat(s string) error {
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return fmt.Errorf("must be a number between 0 and 1, got %q", s)
	}
	// S-1 (agentic-73l6): ParseFloat accepts "NaN" (case-insensitive), and NaN
	// compares false to everything, so `f < 0 || f > 1` alone never rejects it.
	if math.IsNaN(f) || f < 0 || f > 1 {
		return fmt.Errorf("must be between 0 and 1, got %s", s)
	}
	return nil
}

// validateBool accepts exactly "true" or "false" (lowercase). Anything else,
// including "1"/"0"/"True"/"yes", is rejected so the boolean gate is
// unambiguous (M2, agentic-2ixw).
func validateBool(s string) error {
	if s == "true" || s == "false" {
		return nil
	}
	return fmt.Errorf("must be \"true\" or \"false\", got %q", s)
}

// validateAny is a permissive validator for free-form string config values
// (e.g. a reranker command line). It never rejects (M2, agentic-2ixw). The
// stored string is consumed as an argv-array (never a shell) at the use site.
func validateAny(string) error { return nil }

// validateRerankFusion accepts exactly "rrf" or "reorder" (lowercase) so the
// combine-mode gate is unambiguous. The brain-side resolver
// (brain.resolveRerankFusion) treats any non-"reorder" value as the RRF default;
// this validator additionally rejects typos at config-set time (agentic-2ixw).
func validateRerankFusion(s string) error {
	if s == "rrf" || s == "reorder" {
		return nil
	}
	return fmt.Errorf("must be \"rrf\" or \"reorder\", got %q", s)
}

// --- Resolve helpers ---

// resolveConfigInt reads a config key from the store. If not set or unparseable,
// returns the fallback value.
func resolveConfigInt(s *store.Store, key string, fallback int) int {
	val, _ := s.GetMeta(configMetaPrefix + key)
	if val == "" {
		return fallback
	}
	n, err := strconv.Atoi(val)
	if err != nil {
		return fallback
	}
	return n
}

// resolveConfigFloat reads a config key from the store. If not set or unparseable,
// returns the fallback value.
func resolveConfigFloat(s *store.Store, key string, fallback float64) float64 {
	val, _ := s.GetMeta(configMetaPrefix + key)
	if val == "" {
		return fallback
	}
	f, err := strconv.ParseFloat(val, 64)
	if err != nil {
		return fallback
	}
	return f
}

// --- Cobra commands ---

func init() {
	configCmd := &cobra.Command{
		Use:   "config",
		Short: "View and modify brain configuration",
		Long: `Manage per-brain configuration values that override compiled defaults.

Precedence: CLI flag (explicit) > brain config > compiled default.

Use 'cerebro config list' to see all available keys and their current values.`,
	}

	configCmd.AddCommand(configSetCmd())
	configCmd.AddCommand(configGetCmd())
	configCmd.AddCommand(configListCmd())
	configCmd.AddCommand(configResetCmd())

	rootCmd.AddCommand(configCmd)
}

func configSetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "set <key> <value>",
		Short: "Set a configuration value",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			key, value := args[0], args[1]

			param, ok := configRegistry[key]
			if !ok {
				return fmt.Errorf("unknown config key %q — run 'cerebro config list' to see available keys", key)
			}

			if err := param.Validate(value); err != nil {
				return fmt.Errorf("invalid value for %s: %w", key, err)
			}

			b, err := openBrain()
			if err != nil {
				return err
			}
			defer func() { _ = b.Close() }()

			prev := resolveConfigString(b.Store(), key)

			if err := b.Store().SetMeta(configMetaPrefix+key, value); err != nil {
				return fmt.Errorf("setting config: %w", err)
			}

			if formatFlag == "json" {
				outputJSON(map[string]string{
					"key":      key,
					"value":    value,
					"previous": prev,
				})
				return nil
			}

			if !quietFlag {
				fmt.Printf("Set %s = %s (was: %s)\n", key, value, prev)
			}
			return nil
		},
	}
}

func configGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <key>",
		Short: "Get a configuration value",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			key := args[0]

			param, ok := configRegistry[key]
			if !ok {
				return fmt.Errorf("unknown config key %q — run 'cerebro config list' to see available keys", key)
			}

			b, err := openBrain()
			if err != nil {
				return err
			}
			defer func() { _ = b.Close() }()

			stored, _ := b.Store().GetMeta(configMetaPrefix + key)
			source := "brain"
			value := stored
			if stored == "" {
				source = "default"
				value = param.Default
			}

			if formatFlag == "json" {
				outputJSON(map[string]string{
					"key":     key,
					"value":   value,
					"source":  source,
					"default": param.Default,
				})
				return nil
			}

			if source == "brain" {
				fmt.Printf("%s = %s (default: %s)\n", key, value, param.Default)
			} else {
				fmt.Printf("%s = %s (default)\n", key, value)
			}
			return nil
		},
	}
}

func configListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all configuration keys and their values",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			b, err := openBrain()
			if err != nil {
				return err
			}
			defer func() { _ = b.Close() }()

			type entry struct {
				Key         string `json:"key"`
				Value       string `json:"value"`
				Source      string `json:"source"`
				Default     string `json:"default"`
				Description string `json:"description"`
			}

			// Collect entries in a deterministic order.
			keys := configRegistryKeys()
			entries := make([]entry, 0, len(keys))

			for _, key := range keys {
				param := configRegistry[key]
				stored, _ := b.Store().GetMeta(configMetaPrefix + key)
				source := "brain"
				value := stored
				if stored == "" {
					source = "default"
					value = param.Default
				}
				entries = append(entries, entry{
					Key:         key,
					Value:       value,
					Source:      source,
					Default:     param.Default,
					Description: param.Description,
				})
			}

			if formatFlag == "json" {
				outputJSON(entries)
				return nil
			}

			// Markdown table.
			fmt.Println("# Cerebro Configuration")
			fmt.Println()
			for _, e := range entries {
				if e.Source == "brain" {
					fmt.Printf("  %-20s = %-8s (override, default: %s)\n", e.Key, e.Value, e.Default)
				} else {
					fmt.Printf("  %-20s = %-8s (default)\n", e.Key, e.Value)
				}
			}
			fmt.Println()
			fmt.Println("Set with: cerebro config set <key> <value>")
			return nil
		},
	}
}

var configResetAllFlag bool

func configResetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "reset [key]",
		Short: "Reset a configuration value to its default",
		Long:  `Remove a brain config override, reverting to the compiled default. Use --all to reset all config.`,
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !configResetAllFlag && len(args) == 0 {
				return fmt.Errorf("provide a key to reset, or use --all to reset all config")
			}

			b, err := openBrain()
			if err != nil {
				return err
			}
			defer func() { _ = b.Close() }()

			if configResetAllFlag {
				for key := range configRegistry {
					_ = b.Store().DeleteMeta(configMetaPrefix + key)
				}
				if !quietFlag {
					fmt.Println("Reset all config to defaults.")
				}
				return nil
			}

			key := args[0]
			if _, ok := configRegistry[key]; !ok {
				return fmt.Errorf("unknown config key %q — run 'cerebro config list' to see available keys", key)
			}

			if err := b.Store().DeleteMeta(configMetaPrefix + key); err != nil {
				return fmt.Errorf("resetting config: %w", err)
			}

			if !quietFlag {
				fmt.Printf("Reset %s to default (%s).\n", key, configRegistry[key].Default)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&configResetAllFlag, "all", false, "Reset all config values to defaults")
	return cmd
}

// --- Helpers ---

// resolveConfigString reads a raw config value from the store.
// Returns the stored value, or the registry default if not set.
func resolveConfigString(s *store.Store, key string) string {
	val, _ := s.GetMeta(configMetaPrefix + key)
	if val != "" {
		return val
	}
	if p, ok := configRegistry[key]; ok {
		return p.Default
	}
	return ""
}

// configRegistryKeys returns registry keys in sorted order for deterministic output.
func configRegistryKeys() []string {
	// Explicit order for readability.
	// NOTE: this slice is hand-maintained, NOT derived from configRegistry —
	// any new registry key MUST also be appended here or `cerebro config list`
	// silently omits it (M3, agentic-2ixw).
	return []string{
		"prime_limit",
		"gc_threshold",
		"search_limit",
		"search_threshold",
		"recall_threshold",
		"indegree_bonus_enabled",
		"stop_guard_enabled",
		"rerank_enabled",
		"rerank_command",
		"rerank_fusion",
		"bm25_enabled",
		"expand_threshold",
		"expand_spread_threshold",
	}
}

// applyConfigFlag overrides a Cobra flag's default with the brain's config value,
// but only if the flag was not explicitly set by the user on the command line.
func applyConfigFlag(cmd *cobra.Command, s *store.Store, flagName, configKey string) {
	if cmd.Flags().Changed(flagName) {
		return // explicit CLI flag wins
	}
	val, _ := s.GetMeta(configMetaPrefix + configKey)
	if val == "" {
		return // no brain config — compiled default stands
	}
	_ = cmd.Flags().Set(flagName, val)
}
