package agent

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"

	"github.com/Ken-Chy129/personal-agent/pkg/message"
	"github.com/Ken-Chy129/personal-agent/pkg/provider"
)

// mockProvider returns scripted responses in order.
type mockProvider struct {
	responses []*provider.ChatResponse
	callIdx   int
}

func (m *mockProvider) Name() string { return "mock" }

func (m *mockProvider) Chat(_ context.Context, _ *provider.ChatRequest) (*provider.ChatResponse, error) {
	if m.callIdx >= len(m.responses) {
		return &provider.ChatResponse{Content: "done", StopReason: "stop"}, nil
	}
	resp := m.responses[m.callIdx]
	m.callIdx++
	return resp, nil
}

// mockTool is a simple tool for testing.
type mockTool struct {
	name            string
	concurrencySafe bool
	execFn          func(ctx context.Context, input json.RawMessage) (*ToolResult, error)
	execCount       atomic.Int32
}

func (m *mockTool) Name() string                   { return m.name }
func (m *mockTool) Description() string            { return "mock tool" }
func (m *mockTool) InputSchema() map[string]any    { return map[string]any{"type": "object"} }
func (m *mockTool) IsReadOnly() bool               { return true }
func (m *mockTool) IsConcurrencySafe() bool        { return m.concurrencySafe }
func (m *mockTool) IsDestructive() bool             { return false }

func (m *mockTool) Execute(ctx context.Context, input json.RawMessage) (*ToolResult, error) {
	m.execCount.Add(1)
	if m.execFn != nil {
		return m.execFn(ctx, input)
	}
	return &ToolResult{Content: "ok"}, nil
}

func collectEvents(ch <-chan AgentEvent) []AgentEvent {
	var events []AgentEvent
	for ev := range ch {
		events = append(events, ev)
	}
	return events
}

func TestAgent_SimpleTextResponse(t *testing.T) {
	p := &mockProvider{
		responses: []*provider.ChatResponse{
			{Content: "Hello!", StopReason: "stop"},
		},
	}
	a := New(p, nil, &Config{MaxTurns: 10})

	events := collectEvents(a.Run(context.Background(), []message.Message{
		message.NewUserMessage("hi"),
	}))

	var hasText, hasDone bool
	for _, ev := range events {
		if ev.Type == EventAssistantMessage && ev.Content == "Hello!" {
			hasText = true
		}
		if ev.Type == EventDone {
			hasDone = true
		}
	}
	if !hasText {
		t.Fatal("expected assistant_message event")
	}
	if !hasDone {
		t.Fatal("expected done event")
	}
}

func TestAgent_ToolCallLoop(t *testing.T) {
	tool := &mockTool{name: "echo"}

	p := &mockProvider{
		responses: []*provider.ChatResponse{
			{
				StopReason: "tool_calls",
				ToolCalls: []message.ToolCall{
					{ID: "call_1", Name: "echo", Arguments: `{}`},
				},
			},
			{Content: "All done!", StopReason: "stop"},
		},
	}

	a := New(p, []Tool{tool}, &Config{MaxTurns: 10})
	events := collectEvents(a.Run(context.Background(), []message.Message{
		message.NewUserMessage("do something"),
	}))

	var hasToolStart, hasToolResult, hasFinalText bool
	for _, ev := range events {
		if ev.Type == EventToolUseStart && ev.ToolName == "echo" {
			hasToolStart = true
		}
		if ev.Type == EventToolUseResult && ev.ToolName == "echo" {
			hasToolResult = true
		}
		if ev.Type == EventAssistantMessage && ev.Content == "All done!" {
			hasFinalText = true
		}
	}
	if !hasToolStart {
		t.Fatal("expected tool_use_start event")
	}
	if !hasToolResult {
		t.Fatal("expected tool_use_result event")
	}
	if !hasFinalText {
		t.Fatal("expected final assistant_message")
	}
	if tool.execCount.Load() != 1 {
		t.Fatalf("expected tool to be called once, got %d", tool.execCount.Load())
	}
}

func TestAgent_MaxTurnsExceeded(t *testing.T) {
	// Provider always returns tool calls
	p := &mockProvider{
		responses: []*provider.ChatResponse{
			{StopReason: "tool_calls", ToolCalls: []message.ToolCall{{ID: "c1", Name: "t", Arguments: `{}`}}},
			{StopReason: "tool_calls", ToolCalls: []message.ToolCall{{ID: "c2", Name: "t", Arguments: `{}`}}},
			{StopReason: "tool_calls", ToolCalls: []message.ToolCall{{ID: "c3", Name: "t", Arguments: `{}`}}},
			{StopReason: "tool_calls", ToolCalls: []message.ToolCall{{ID: "c4", Name: "t", Arguments: `{}`}}},
		},
	}
	tool := &mockTool{name: "t"}
	a := New(p, []Tool{tool}, &Config{MaxTurns: 3})

	events := collectEvents(a.Run(context.Background(), []message.Message{
		message.NewUserMessage("loop"),
	}))

	var hasError bool
	for _, ev := range events {
		if ev.Type == EventError && ev.Error != nil {
			hasError = true
		}
	}
	if !hasError {
		t.Fatal("expected error event for max turns exceeded")
	}
}

func TestAgent_ConcurrentToolExecution(t *testing.T) {
	tool1 := &mockTool{name: "read1", concurrencySafe: true}
	tool2 := &mockTool{name: "read2", concurrencySafe: true}

	p := &mockProvider{
		responses: []*provider.ChatResponse{
			{
				StopReason: "tool_calls",
				ToolCalls: []message.ToolCall{
					{ID: "c1", Name: "read1", Arguments: `{}`},
					{ID: "c2", Name: "read2", Arguments: `{}`},
				},
			},
			{Content: "done", StopReason: "stop"},
		},
	}

	a := New(p, []Tool{tool1, tool2}, &Config{MaxTurns: 10})
	events := collectEvents(a.Run(context.Background(), []message.Message{
		message.NewUserMessage("read both"),
	}))

	if tool1.execCount.Load() != 1 || tool2.execCount.Load() != 1 {
		t.Fatalf("expected each tool called once, got read1=%d read2=%d",
			tool1.execCount.Load(), tool2.execCount.Load())
	}

	// Verify we got results for both
	resultCount := 0
	for _, ev := range events {
		if ev.Type == EventToolUseResult {
			resultCount++
		}
	}
	if resultCount != 2 {
		t.Fatalf("expected 2 tool results, got %d", resultCount)
	}
}

func TestAgent_UnknownTool(t *testing.T) {
	p := &mockProvider{
		responses: []*provider.ChatResponse{
			{
				StopReason: "tool_calls",
				ToolCalls: []message.ToolCall{
					{ID: "c1", Name: "nonexistent", Arguments: `{}`},
				},
			},
			{Content: "ok", StopReason: "stop"},
		},
	}

	a := New(p, nil, &Config{MaxTurns: 10})
	events := collectEvents(a.Run(context.Background(), []message.Message{
		message.NewUserMessage("call unknown"),
	}))

	var hasErrorResult bool
	for _, ev := range events {
		if ev.Type == EventToolUseResult && ev.ToolResult != nil && ev.ToolResult.IsError {
			hasErrorResult = true
		}
	}
	if !hasErrorResult {
		t.Fatal("expected error result for unknown tool")
	}
}
