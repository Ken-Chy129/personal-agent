package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/joho/godotenv"

	"github.com/Ken-Chy129/personal-agent/internal/cli"
	"github.com/Ken-Chy129/personal-agent/internal/tools"
	"github.com/Ken-Chy129/personal-agent/pkg/agent"
	oaiprovider "github.com/Ken-Chy129/personal-agent/pkg/provider/openai"
)

const defaultSystemPrompt = `You are a helpful coding assistant with access to tools.
Use the file_write tool to create or modify files.
Use the bash tool to run shell commands.
Use the file_read tool to read file contents.
Use the image_generate tool to generate images from text prompts.
After creating files, verify your work by running them when appropriate.
Be concise in your responses.`

func main() {
	_ = godotenv.Load()

	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		fmt.Fprintln(os.Stderr, "Error: OPENAI_API_KEY is not set (set in .env or environment)")
		os.Exit(1)
	}
	baseURL := os.Getenv("OPENAI_BASE_URL")
	model := getEnvOr("AGENT_MODEL", "gpt-4o")

	var providerOpts []oaiprovider.Option
	providerOpts = append(providerOpts, oaiprovider.WithModel(model))
	if baseURL != "" {
		providerOpts = append(providerOpts, oaiprovider.WithBaseURL(baseURL))
	}
	p := oaiprovider.NewProvider(apiKey, providerOpts...)

	allTools := []agent.Tool{
		tools.NewBash(),
		tools.NewFileWrite(),
		tools.NewFileRead(),
		tools.NewImageGenerate(apiKey, baseURL),
	}

	cfg := &agent.Config{
		Model:        model,
		SystemPrompt: defaultSystemPrompt,
		MaxTurns:     50,
	}

	a := agent.New(p, allTools, cfg)
	r := cli.New(a)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	if err := r.Run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func getEnvOr(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}
