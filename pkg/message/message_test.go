package message

import "testing"

func TestNewUserMessage(t *testing.T) {
	m := NewUserMessage("hello")
	if m.Role != RoleUser || m.Content != "hello" {
		t.Fatalf("unexpected message: %+v", m)
	}
	if m.HasToolCalls() {
		t.Fatal("user message should not have tool calls")
	}
}

func TestNewAssistantToolCallMessage(t *testing.T) {
	calls := []ToolCall{
		{ID: "call_1", Name: "bash", Arguments: `{"command":"ls"}`},
		{ID: "call_2", Name: "file_read", Arguments: `{"path":"foo.txt"}`},
	}
	m := NewAssistantToolCallMessage("", calls)
	if m.Role != RoleAssistant {
		t.Fatalf("expected assistant role, got %s", m.Role)
	}
	if !m.HasToolCalls() {
		t.Fatal("expected tool calls")
	}
	if len(m.ToolCalls) != 2 {
		t.Fatalf("expected 2 tool calls, got %d", len(m.ToolCalls))
	}
}

func TestNewToolResultMessage(t *testing.T) {
	m := NewToolResultMessage("call_1", "bash", "output")
	if m.Role != RoleTool || m.ToolCallID != "call_1" || m.Name != "bash" || m.Content != "output" {
		t.Fatalf("unexpected message: %+v", m)
	}
}
