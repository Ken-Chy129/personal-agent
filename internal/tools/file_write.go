package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Ken-Chy129/personal-agent/pkg/agent"
)

type fileWriteInput struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type FileWriteTool struct{}

func NewFileWrite() *FileWriteTool { return &FileWriteTool{} }

func (t *FileWriteTool) Name() string        { return "file_write" }
func (t *FileWriteTool) Description() string { return "Write content to a file, creating it if it doesn't exist." }
func (t *FileWriteTool) IsReadOnly() bool       { return false }
func (t *FileWriteTool) IsConcurrencySafe() bool { return true }
func (t *FileWriteTool) IsDestructive() bool     { return false }

func (t *FileWriteTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "The file path to write to",
			},
			"content": map[string]any{
				"type":        "string",
				"description": "The content to write to the file",
			},
		},
		"required": []any{"path", "content"},
	}
}

func (t *FileWriteTool) Execute(_ context.Context, input json.RawMessage) (*agent.ToolResult, error) {
	var in fileWriteInput
	if err := json.Unmarshal(input, &in); err != nil {
		return nil, fmt.Errorf("parse file_write input: %w", err)
	}
	if in.Path == "" {
		return &agent.ToolResult{Content: "error: path is empty", IsError: true}, nil
	}

	dir := filepath.Dir(in.Path)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return &agent.ToolResult{
				Content: fmt.Sprintf("error creating directory %s: %s", dir, err),
				IsError: true,
			}, nil
		}
	}

	if err := os.WriteFile(in.Path, []byte(in.Content), 0644); err != nil {
		return &agent.ToolResult{
			Content: fmt.Sprintf("error writing file: %s", err),
			IsError: true,
		}, nil
	}

	return &agent.ToolResult{Content: fmt.Sprintf("Successfully wrote %d bytes to %s", len(in.Content), in.Path)}, nil
}
