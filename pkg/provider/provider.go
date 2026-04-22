package provider

import (
	"context"

	"github.com/Ken-Chy129/personal-agent/pkg/message"
)

// Provider is the abstraction for LLM API calls.
type Provider interface {
	Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error)
	Stream(ctx context.Context, req *ChatRequest) (<-chan StreamEvent, error)
	Name() string
}

// StreamEvent represents a single event from a streaming LLM response.
type StreamEvent struct {
	Type string // "text_delta", "tool_call_delta", "done", "error"

	// For text_delta: the incremental text
	TextDelta string

	// For done: the final accumulated response
	Response *ChatResponse

	// For error
	Error error
}

// ChatRequest represents a request to the LLM.
type ChatRequest struct {
	Model        string
	Messages     []message.Message
	Tools        []ToolDefinition
	SystemPrompt string
	MaxTokens    int
}

// ToolDefinition describes a tool for the LLM.
type ToolDefinition struct {
	Name        string
	Description string
	Parameters  map[string]any // JSON Schema
}

// ChatResponse represents the LLM's response.
type ChatResponse struct {
	Content    string             // text content
	ToolCalls  []message.ToolCall // tool calls requested
	Usage      Usage
	StopReason string // "stop", "tool_calls"
}

// Usage tracks token consumption.
type Usage struct {
	InputTokens  int
	OutputTokens int
	Model        string
}
