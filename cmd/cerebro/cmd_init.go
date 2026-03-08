package main

import (
	"fmt"

	"github.com/coetzeevs/cerebro/brain"
	"github.com/spf13/cobra"
)

var initEmbedProvider string
var initEmbedModel string
var initEmbedDims int
var initGlobalFlag bool

func init() {
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize a new brain for the current project",
		RunE:  runInit,
	}
	cmd.Flags().StringVar(&initEmbedProvider, "embed-provider", "ollama", "Embedding provider: ollama, voyage, none")
	cmd.Flags().StringVar(&initEmbedModel, "embed-model", "", "Embedding model (provider-specific)")
	cmd.Flags().IntVar(&initEmbedDims, "embed-dims", 0, "Embedding dimensions (auto-detected from provider if 0)")
	cmd.Flags().BoolVar(&initGlobalFlag, "global", false, "Initialize the global store (~/.cerebro/global.sqlite)")
	rootCmd.AddCommand(cmd)
}

func runInit(cmd *cobra.Command, args []string) error {
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

	return nil
}
