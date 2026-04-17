package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/Ken-Chy129/personal-agent/pkg/agent"
)

type fileReadInput struct {
	Path   string `json:"path"`
	Offset *int   `json:"offset,omitempty"` // 0-based line offset
	Limit  *int   `json:"limit,omitempty"`  // max lines to read
}

type FileReadTool struct{}

func NewFileRead() *FileReadTool { return &FileReadTool{} }

func (t *FileReadTool) Name() string        { return "file_read" }
func (t *FileReadTool) Description() string { return "Read the contents of a file. Optionally specify line offset and limit." }
func (t *FileReadTool) IsReadOnly() bool       { return true }
func (t *FileReadTool) IsConcurrencySafe() bool { return true }
func (t *FileReadTool) IsDestructive() bool     { return false }

func (t *FileReadTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "The file path to read",
			},
			"offset": map[string]any{
				"type":        "integer",
				"description": "Line offset to start reading from (0-based). Default: 0",
			},
			"limit": map[string]any{
				"type":        "integer",
				"description": "Maximum number of lines to read. Default: all lines",
			},
		},
		"required": []any{"path"},
	}
}

func (t *FileReadTool) Execute(_ context.Context, input json.RawMessage) (*agent.ToolResult, error) {
	var in fileReadInput
	if err := json.Unmarshal(input, &in); err != nil {
		return nil, fmt.Errorf("parse file_read input: %w", err)
	}
	if in.Path == "" {
		return &agent.ToolResult{Content: "error: path is empty", IsError: true}, nil
	}

	data, err := os.ReadFile(in.Path)
	if err != nil {
		return &agent.ToolResult{
			Content: fmt.Sprintf("error reading file: %s", err),
			IsError: true,
		}, nil
	}

	lines := strings.Split(string(data), "\n")

	offset := 0
	if in.Offset != nil && *in.Offset > 0 {
		offset = *in.Offset
	}
	if offset > len(lines) {
		offset = len(lines)
	}

	end := len(lines)
	if in.Limit != nil && *in.Limit > 0 {
		end = offset + *in.Limit
		if end > len(lines) {
			end = len(lines)
		}
	}

	lines = lines[offset:end]

	// Add line numbers
	var sb strings.Builder
	for i, line := range lines {
		fmt.Fprintf(&sb, "%d\t%s\n", offset+i+1, line)
	}

	return &agent.ToolResult{Content: sb.String()}, nil
}
