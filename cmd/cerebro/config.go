package main

import (
	"fmt"
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
	if f < 0 || f > 1 {
		return fmt.Errorf("must be between 0 and 1, got %s", s)
	}
	return nil
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
	return []string{
		"prime_limit",
		"gc_threshold",
		"search_limit",
		"search_threshold",
		"recall_threshold",
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
