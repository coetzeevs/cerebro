package main

import (
	"fmt"
	"os"

	"github.com/coetzeevs/cerebro/brain"
	"github.com/spf13/cobra"
)

var initEmbedProvider string
var initEmbedModel string
var initEmbedDims int
var initGlobalFlag bool
var initSkipIntegration bool

func init() {
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize a new brain and scaffold Claude Code integration",
		Long: `Initialize a new Cerebro brain for the current project (or global store).

By default, this also scaffolds Claude Code integration files:
  - .claude/settings.json  (session hooks)
  - .claude/skills/        (remember, recall, consolidate skills)
  - CLAUDE.md              (behavioral instructions section)

Use --skip-integration to create only the database without integration files.`,
		RunE: runInit,
	}
	cmd.Flags().StringVar(&initEmbedProvider, "embed-provider", "ollama", "Embedding provider: ollama, voyage, none")
	cmd.Flags().StringVar(&initEmbedModel, "embed-model", "", "Embedding model (provider-specific)")
	cmd.Flags().IntVar(&initEmbedDims, "embed-dims", 0, "Embedding dimensions (auto-detected from provider if 0)")
	cmd.Flags().BoolVar(&initGlobalFlag, "global", false, "Initialize the global store (~/.cerebro/global.sqlite)")
	cmd.Flags().BoolVar(&initSkipIntegration, "skip-integration", false, "Skip Claude Code integration file generation")
	rootCmd.AddCommand(cmd)
}

func runInit(_ *cobra.Command, _ []string) error {
	path := resolveBrainPath()
	if initGlobalFlag {
		path = brain.GlobalPath()
	}

	cfg := brain.EmbedConfig{
		Provider:   initEmbedProvider,
		Model:      initEmbedModel,
		Dimensions: initEmbedDims,
	}

	b, err := brain.Init(path, cfg)
	if err != nil {
		return err
	}
	defer func() { _ = b.Close() }()

	if !quietFlag {
		if initGlobalFlag {
			fmt.Printf("Initialized global brain at %s\n", path)
		} else {
			fmt.Printf("Initialized brain at %s\n", path)
		}
		fmt.Printf("Embedding provider: %s\n", initEmbedProvider)
	}

	// Skip integration scaffolding for global store or when explicitly requested
	if initGlobalFlag || initSkipIntegration {
		return nil
	}

	// Resolve the project directory for scaffolding
	projectDir := projectFlag
	if projectDir == "" {
		projectDir, _ = os.Getwd()
	}

	if !quietFlag {
		fmt.Println()
	}

	// Scaffold settings.json hooks
	if settingsCreated, err := scaffoldSettings(projectDir); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not scaffold settings.json: %v\n", err)
	} else if settingsCreated && !quietFlag {
		fmt.Println("Created .claude/settings.json (session hooks)")
	} else if !settingsCreated && !quietFlag {
		fmt.Println("Skipped .claude/settings.json (cerebro hooks already present)")
	}

	// Scaffold skill files
	if skillsCreated, err := scaffoldSkills(projectDir); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not scaffold skills: %v\n", err)
	} else if skillsCreated > 0 && !quietFlag {
		fmt.Printf("Created %d skill files in .claude/skills/\n", skillsCreated)
	} else if !quietFlag {
		fmt.Println("Skipped skills (already present)")
	}

	// Scaffold CLAUDE.md section
	if claudeCreated, err := scaffoldCLAUDEMD(projectDir); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not scaffold CLAUDE.md: %v\n", err)
	} else if claudeCreated && !quietFlag {
		fmt.Println("Updated CLAUDE.md (added Cerebro Memory System section)")
	} else if !quietFlag {
		fmt.Println("Skipped CLAUDE.md (Cerebro section already present)")
	}

	// Check Ollama if using ollama provider
	if initEmbedProvider == "ollama" {
		model := initEmbedModel
		if model == "" {
			model = "nomic-embed-text"
		}
		status := checkOllama(model)
		switch {
		case !status.Installed:
			fmt.Fprintf(os.Stderr, "\nNote: Ollama is not installed. Install it to enable embeddings:\n")
			fmt.Fprintf(os.Stderr, "  brew install ollama\n")
			fmt.Fprintf(os.Stderr, "  ollama pull %s\n", model)
		case !status.Running:
			fmt.Fprintf(os.Stderr, "\nNote: Ollama is installed but not running. Start it:\n")
			fmt.Fprintf(os.Stderr, "  brew services start ollama\n")
			fmt.Fprintf(os.Stderr, "  ollama pull %s\n", model)
		case !status.ModelReady:
			fmt.Fprintf(os.Stderr, "\nNote: Ollama is running but model %q not found. Pull it:\n", model)
			fmt.Fprintf(os.Stderr, "  ollama pull %s\n", model)
		}
	}

	return nil
}
