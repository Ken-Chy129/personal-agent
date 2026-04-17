package agent

import (
	"context"
	"fmt"

	"github.com/Ken-Chy129/personal-agent/pkg/message"
	"github.com/Ken-Chy129/personal-agent/pkg/provider"
)

const defaultMaxTurns = 50

// Config holds agent configuration.
type Config struct {
	Model        string
	SystemPrompt string
	MaxTurns     int
}

// EventType identifies the kind of agent event.
type EventType string

const (
	EventAssistantMessage EventType = "assistant_message"
	EventToolUseStart     EventType = "tool_use_start"
	EventToolUseResult    EventType = "tool_use_result"
	EventError            EventType = "error"
	EventDone             EventType = "done"
)

// AgentEvent is an event emitted by the agent loop.
type AgentEvent struct {
	Type       EventType
	Content    string      // text for assistant_message / error
	ToolName   string      // for tool events
	ToolInput  string      // raw JSON input for tool_use_start
	ToolResult *ToolResult // for tool_use_result
	Error      error       // for error events
	// Usage accumulated so far in this run
	TotalUsage provider.Usage
}

// Agent ties a provider and tools together to form an autonomous agent.
type Agent struct {
	provider provider.Provider
	tools    map[string]Tool
	config   *Config
}

// New creates a new Agent.
func New(p provider.Provider, tools []Tool, cfg *Config) *Agent {
	toolMap := make(map[string]Tool, len(tools))
	for _, t := range tools {
		toolMap[t.Name()] = t
	}
	if cfg.MaxTurns <= 0 {
		cfg.MaxTurns = defaultMaxTurns
	}
	return &Agent{
		provider: p,
		tools:    toolMap,
		config:   cfg,
	}
}

// Run executes the agent loop and returns a channel of events.
// The channel is closed when the agent is done.
func (a *Agent) Run(ctx context.Context, messages []message.Message) <-chan AgentEvent {
	ch := make(chan AgentEvent, 16)
	go a.run(ctx, messages, ch)
	return ch
}

func (a *Agent) run(ctx context.Context, messages []message.Message, ch chan<- AgentEvent) {
	defer close(ch)

	// Build tool definitions for the provider
	toolDefs := make([]provider.ToolDefinition, 0, len(a.tools))
	for _, t := range a.tools {
		toolDefs = append(toolDefs, provider.ToolDefinition{
			Name:        t.Name(),
			Description: t.Description(),
			Parameters:  t.InputSchema(),
		})
	}

	// Copy messages to avoid mutating the caller's slice
	msgs := make([]message.Message, len(messages))
	copy(msgs, messages)

	var totalUsage provider.Usage

	for turn := 0; turn < a.config.MaxTurns; turn++ {
		select {
		case <-ctx.Done():
			ch <- AgentEvent{Type: EventError, Error: ctx.Err(), Content: "interrupted"}
			ch <- AgentEvent{Type: EventDone, TotalUsage: totalUsage}
			return
		default:
		}

		req := &provider.ChatRequest{
			Model:        a.config.Model,
			Messages:     msgs,
			Tools:        toolDefs,
			SystemPrompt: a.config.SystemPrompt,
		}

		resp, err := a.provider.Chat(ctx, req)
		if err != nil {
			ch <- AgentEvent{Type: EventError, Error: err, Content: err.Error()}
			ch <- AgentEvent{Type: EventDone, TotalUsage: totalUsage}
			return
		}

		// Accumulate usage
		totalUsage.InputTokens += resp.Usage.InputTokens
		totalUsage.OutputTokens += resp.Usage.OutputTokens
		totalUsage.Model = resp.Usage.Model

		// Emit assistant text if present
		if resp.Content != "" {
			ch <- AgentEvent{Type: EventAssistantMessage, Content: resp.Content}
		}

		// If no tool calls, we're done
		if resp.StopReason != "tool_calls" || len(resp.ToolCalls) == 0 {
			ch <- AgentEvent{Type: EventDone, TotalUsage: totalUsage}
			return
		}

		// Append the assistant message (with tool calls) to conversation
		msgs = append(msgs, message.NewAssistantToolCallMessage(resp.Content, resp.ToolCalls))

		// Execute tools
		batches := partitionToolCalls(resp.ToolCalls, a.tools)
		for _, b := range batches {
			// Emit start events
			for _, call := range b.Calls {
				ch <- AgentEvent{Type: EventToolUseStart, ToolName: call.Name, ToolInput: call.Arguments}
			}

			results := executeBatch(ctx, b, a.tools)

			for _, r := range results {
				ch <- AgentEvent{
					Type:       EventToolUseResult,
					ToolName:   r.ToolCall.Name,
					ToolResult: r.Result,
				}
				// Append tool result to conversation
				content := r.Result.Content
				if r.Result.IsError {
					content = fmt.Sprintf("Error: %s", content)
				}
				msgs = append(msgs, message.NewToolResultMessage(r.ToolCall.ID, r.ToolCall.Name, content))
			}
		}
	}

	// Exceeded max turns
	ch <- AgentEvent{
		Type:    EventError,
		Error:   fmt.Errorf("exceeded maximum turns (%d)", a.config.MaxTurns),
		Content: fmt.Sprintf("exceeded maximum turns (%d)", a.config.MaxTurns),
	}
	ch <- AgentEvent{Type: EventDone, TotalUsage: totalUsage}
}
