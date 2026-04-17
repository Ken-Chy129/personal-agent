package openai

import (
	"testing"

	"github.com/Ken-Chy129/personal-agent/pkg/message"
	"github.com/Ken-Chy129/personal-agent/pkg/provider"
)

func TestConvertMessagesToOpenAI(t *testing.T) {
	msgs := []message.Message{
		message.NewUserMessage("hello"),
		message.NewAssistantMessage("hi there"),
		message.NewAssistantToolCallMessage("", []message.ToolCall{
			{ID: "call_1", Name: "bash", Arguments: `{"command":"ls"}`},
		}),
		message.NewToolResultMessage("call_1", "bash", "file1.txt\nfile2.txt"),
	}

	result := convertMessagesToOpenAI(msgs, "You are helpful")
	// system prompt + 4 messages = 5
	if len(result) != 5 {
		t.Fatalf("expected 5 messages, got %d", len(result))
	}
}

func TestConvertToolsToOpenAI(t *testing.T) {
	tools := []provider.ToolDefinition{
		{
			Name:        "bash",
			Description: "Run a shell command",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"command": map[string]any{
						"type":        "string",
						"description": "The command to run",
					},
				},
				"required": []any{"command"},
			},
		},
	}

	result := convertToolsToOpenAI(tools)
	if len(result) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(result))
	}
	if result[0].Function.Name != "bash" {
		t.Fatalf("expected tool name 'bash', got %s", result[0].Function.Name)
	}
}

func TestConvertToolsToOpenAI_Empty(t *testing.T) {
	result := convertToolsToOpenAI(nil)
	if result != nil {
		t.Fatal("expected nil for empty tools")
	}
}
