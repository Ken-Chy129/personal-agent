package agent

import (
	"context"
	"encoding/json"
)

// Tool defines a tool that can be invoked by the agent.
type Tool interface {
	Name() string
	Description() string
	InputSchema() map[string]any
	Execute(ctx context.Context, input json.RawMessage) (*ToolResult, error)
	IsReadOnly() bool
	IsConcurrencySafe() bool
	IsDestructive() bool
}

// ToolResult is the result of a tool execution.
type ToolResult struct {
	Content string
	IsError bool
}
