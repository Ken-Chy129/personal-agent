package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/Ken-Chy129/personal-agent/pkg/agent"
	"github.com/Ken-Chy129/personal-agent/pkg/message"
)

const maxToolResultDisplay = 500

// REPL provides an interactive command-line interface for the agent.
type REPL struct {
	agent   *agent.Agent
	history []message.Message
}

// New creates a new REPL.
func New(a *agent.Agent) *REPL {
	return &REPL{agent: a}
}

// Run starts the REPL loop. It blocks until the user exits or the context is cancelled.
func (r *REPL) Run(ctx context.Context) error {
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024) // 1MB buffer for long inputs

	fmt.Println("Personal Agent (type /exit to quit, /clear to reset)")
	fmt.Println()

	for {
		fmt.Print("> ")
		if !scanner.Scan() {
			break
		}

		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}

		switch input {
		case "/exit", "/quit":
			fmt.Println("Goodbye!")
			return nil
		case "/clear":
			r.history = nil
			fmt.Println("Conversation cleared.")
			continue
		}

		r.history = append(r.history, message.NewUserMessage(input))

		events := r.agent.Run(ctx, r.history)
		var lastAssistantContent string
		streaming := false

		for ev := range events {
			switch ev.Type {
			case agent.EventTextDelta:
				if !streaming {
					fmt.Println()
					streaming = true
				}
				fmt.Print(ev.Content)

			case agent.EventAssistantMessage:
				lastAssistantContent = ev.Content
				if !streaming {
					// Only print if we didn't already stream it
					fmt.Printf("\n%s\n", ev.Content)
				} else {
					fmt.Println() // newline after streamed text
					streaming = false
				}

			case agent.EventToolUseStart:
				if streaming {
					fmt.Println()
					streaming = false
				}
				fmt.Printf("\n  [Tool: %s] %s\n", ev.ToolName, truncate(ev.ToolInput, 200))

			case agent.EventToolUseResult:
				if ev.ToolResult != nil {
					if ev.ToolResult.IsError {
						fmt.Printf("  [Error] %s\n", truncate(ev.ToolResult.Content, maxToolResultDisplay))
					} else {
						fmt.Printf("  [Result] %s\n", truncate(ev.ToolResult.Content, maxToolResultDisplay))
					}
				}

			case agent.EventError:
				if streaming {
					fmt.Println()
					streaming = false
				}
				fmt.Printf("\n  Error: %s\n", ev.Content)

			case agent.EventDone:
				if streaming {
					fmt.Println()
					streaming = false
				}
				if ev.TotalUsage.InputTokens > 0 || ev.TotalUsage.OutputTokens > 0 {
					fmt.Printf("\n  [tokens: in=%d out=%d]\n", ev.TotalUsage.InputTokens, ev.TotalUsage.OutputTokens)
				}
			}
		}

		// Append the final assistant message to history for multi-turn conversation
		if lastAssistantContent != "" {
			r.history = append(r.history, message.NewAssistantMessage(lastAssistantContent))
		}

		fmt.Println()
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("reading input: %w", err)
	}
	return nil
}

func truncate(s string, maxLen int) string {
	s = strings.TrimSpace(s)
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
