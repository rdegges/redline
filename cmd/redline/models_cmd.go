package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

const (
	defaultOllamaModel    = "qwen3:30b"
	defaultEmbeddingModel = "nomic-embed-text"
)

func modelsListRun() error {
	fmt.Printf("configured LLM model:       %s\n", defaultOllamaModel)
	fmt.Printf("configured embedding model: %s\n", defaultEmbeddingModel)
	return nil
}

func modelsRecommendRun(c *cobra.Command) error {
	out := c.OutOrStdout()
	fmt.Fprintln(out, "Recommended Ollama models by RAM tier:")
	fmt.Fprintln(out, "  8GB   LLM=qwen3:4b      embed=nomic-embed-text")
	fmt.Fprintln(out, "  16GB  LLM=qwen3:30b     embed=nomic-embed-text  (default)")
	fmt.Fprintln(out, "  32GB  LLM=qwen3:30b     embed=mxbai-embed-large")
	fmt.Fprintln(out, "  48GB+ LLM=qwen3:30b     embed=mxbai-embed-large")
	return nil
}
