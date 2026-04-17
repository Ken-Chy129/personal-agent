package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/Ken-Chy129/personal-agent/internal/cli"
	"github.com/Ken-Chy129/personal-agent/internal/tools"
	"github.com/Ken-Chy129/personal-agent/pkg/agent"
	oaiprovider "github.com/Ken-Chy129/personal-agent/pkg/provider/openai"
)

const defaultSystemPrompt = `You are a helpful coding assistant with access to tools.
Use the file_write tool to create or modify files.
Use the bash tool to run shell commands.
Use the file_read tool to read file contents.
After creating files, verify your work by running them when appropriate.
Be concise in your responses.`

func main() {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		fmt.Fprintln(os.Stderr, "Error: OPENAI_API_KEY environment variable is not set")
		os.Exit(1)
	}

	model := getEnvOr("AGENT_MODEL", "gpt-4o")

	p := oaiprovider.NewProvider(apiKey, oaiprovider.WithModel(model))

	allTools := []agent.Tool{
		tools.NewBash(),
		tools.NewFileWrite(),
		tools.NewFileRead(),
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
