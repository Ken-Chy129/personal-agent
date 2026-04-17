package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"time"

	"github.com/Ken-Chy129/personal-agent/pkg/agent"
)

const bashTimeout = 120 * time.Second

type bashInput struct {
	Command string `json:"command"`
}

type BashTool struct{}

func NewBash() *BashTool { return &BashTool{} }

func (t *BashTool) Name() string        { return "bash" }
func (t *BashTool) Description() string { return "Execute a shell command and return its output." }
func (t *BashTool) IsReadOnly() bool       { return false }
func (t *BashTool) IsConcurrencySafe() bool { return false }
func (t *BashTool) IsDestructive() bool     { return true }

func (t *BashTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"command": map[string]any{
				"type":        "string",
				"description": "The shell command to execute",
			},
		},
		"required": []any{"command"},
	}
}

func (t *BashTool) Execute(ctx context.Context, input json.RawMessage) (*agent.ToolResult, error) {
	var in bashInput
	if err := json.Unmarshal(input, &in); err != nil {
		return nil, fmt.Errorf("parse bash input: %w", err)
	}
	if in.Command == "" {
		return &agent.ToolResult{Content: "error: command is empty", IsError: true}, nil
	}

	ctx, cancel := context.WithTimeout(ctx, bashTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", "-c", in.Command)
	out, err := cmd.CombinedOutput()

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return &agent.ToolResult{
				Content: fmt.Sprintf("command timed out after %s\n%s", bashTimeout, string(out)),
				IsError: true,
			}, nil
		}
		return &agent.ToolResult{
			Content: fmt.Sprintf("%s\n%s", string(out), err.Error()),
			IsError: true,
		}, nil
	}

	return &agent.ToolResult{Content: string(out)}, nil
}
