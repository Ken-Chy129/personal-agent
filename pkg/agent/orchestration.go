package agent

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/Ken-Chy129/personal-agent/pkg/message"
)

// toolCallResult holds the result of a single tool call execution.
type toolCallResult struct {
	ToolCall message.ToolCall
	Result   *ToolResult
	Err      error
}

// batch represents a group of tool calls to be executed together.
type batch struct {
	Concurrent bool
	Calls      []message.ToolCall
}

// partitionToolCalls groups consecutive tool calls by concurrency safety.
// Consecutive ConcurrencySafe calls are batched together for parallel execution.
// Non-ConcurrencySafe calls each form their own serial batch.
func partitionToolCalls(calls []message.ToolCall, tools map[string]Tool) []batch {
	var batches []batch
	var currentConcurrent []message.ToolCall

	flush := func() {
		if len(currentConcurrent) > 0 {
			batches = append(batches, batch{Concurrent: true, Calls: currentConcurrent})
			currentConcurrent = nil
		}
	}

	for _, call := range calls {
		tool, ok := tools[call.Name]
		if !ok || !tool.IsConcurrencySafe() {
			flush()
			batches = append(batches, batch{Concurrent: false, Calls: []message.ToolCall{call}})
		} else {
			currentConcurrent = append(currentConcurrent, call)
		}
	}
	flush()

	return batches
}

// executeBatch runs a batch of tool calls, either concurrently or serially.
func executeBatch(ctx context.Context, b batch, tools map[string]Tool) []toolCallResult {
	if b.Concurrent && len(b.Calls) > 1 {
		return executeConcurrent(ctx, b.Calls, tools)
	}
	return executeSerial(ctx, b.Calls, tools)
}

func executeConcurrent(ctx context.Context, calls []message.ToolCall, tools map[string]Tool) []toolCallResult {
	results := make([]toolCallResult, len(calls))
	var wg sync.WaitGroup

	for i, call := range calls {
		wg.Add(1)
		go func(idx int, tc message.ToolCall) {
			defer wg.Done()
			results[idx] = executeOne(ctx, tc, tools)
		}(i, call)
	}

	wg.Wait()
	return results
}

func executeSerial(ctx context.Context, calls []message.ToolCall, tools map[string]Tool) []toolCallResult {
	results := make([]toolCallResult, 0, len(calls))
	for _, call := range calls {
		results = append(results, executeOne(ctx, call, tools))
	}
	return results
}

func executeOne(ctx context.Context, call message.ToolCall, tools map[string]Tool) toolCallResult {
	tool, ok := tools[call.Name]
	if !ok {
		return toolCallResult{
			ToolCall: call,
			Result:   &ToolResult{Content: "error: unknown tool " + call.Name, IsError: true},
		}
	}

	result, err := tool.Execute(ctx, json.RawMessage(call.Arguments))
	if err != nil {
		return toolCallResult{
			ToolCall: call,
			Result:   &ToolResult{Content: "error: " + err.Error(), IsError: true},
		}
	}

	return toolCallResult{ToolCall: call, Result: result}
}
