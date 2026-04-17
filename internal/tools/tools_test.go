package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBashTool_Echo(t *testing.T) {
	tool := NewBash()
	result, err := tool.Execute(context.Background(), json.RawMessage(`{"command":"echo hello"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %s", result.Content)
	}
	if strings.TrimSpace(result.Content) != "hello" {
		t.Fatalf("expected 'hello', got %q", result.Content)
	}
}

func TestBashTool_FailingCommand(t *testing.T) {
	tool := NewBash()
	result, err := tool.Execute(context.Background(), json.RawMessage(`{"command":"exit 1"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for failing command")
	}
}

func TestBashTool_EmptyCommand(t *testing.T) {
	tool := NewBash()
	result, err := tool.Execute(context.Background(), json.RawMessage(`{"command":""}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for empty command")
	}
}

func TestFileWriteTool(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	input := map[string]string{"path": path, "content": "hello world"}
	data, _ := json.Marshal(input)

	tool := NewFileWrite()
	result, err := tool.Execute(context.Background(), data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %s", result.Content)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read written file: %v", err)
	}
	if string(content) != "hello world" {
		t.Fatalf("expected 'hello world', got %q", string(content))
	}
}

func TestFileWriteTool_CreatesParentDirs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "dir", "test.txt")
	input := map[string]string{"path": path, "content": "nested"}
	data, _ := json.Marshal(input)

	tool := NewFileWrite()
	result, err := tool.Execute(context.Background(), data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %s", result.Content)
	}

	content, _ := os.ReadFile(path)
	if string(content) != "nested" {
		t.Fatalf("expected 'nested', got %q", string(content))
	}
}

func TestFileReadTool(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, []byte("line1\nline2\nline3"), 0644)

	input := map[string]string{"path": path}
	data, _ := json.Marshal(input)

	tool := NewFileRead()
	result, err := tool.Execute(context.Background(), data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %s", result.Content)
	}
	if !strings.Contains(result.Content, "1\tline1") {
		t.Fatalf("expected line numbers, got %q", result.Content)
	}
	if !strings.Contains(result.Content, "3\tline3") {
		t.Fatalf("expected line 3, got %q", result.Content)
	}
}

func TestFileReadTool_WithOffsetAndLimit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, []byte("a\nb\nc\nd\ne"), 0644)

	offset := 1
	limit := 2
	input := map[string]any{"path": path, "offset": offset, "limit": limit}
	data, _ := json.Marshal(input)

	tool := NewFileRead()
	result, err := tool.Execute(context.Background(), data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(result.Content), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d: %q", len(lines), result.Content)
	}
	if !strings.Contains(lines[0], "2\tb") {
		t.Fatalf("expected line 2 with 'b', got %q", lines[0])
	}
}

func TestFileReadTool_NotFound(t *testing.T) {
	tool := NewFileRead()
	result, err := tool.Execute(context.Background(), json.RawMessage(`{"path":"/nonexistent/file.txt"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for nonexistent file")
	}
}

func TestToolAttributes(t *testing.T) {
	bash := NewBash()
	if bash.IsReadOnly() {
		t.Fatal("bash should not be read-only")
	}
	if bash.IsConcurrencySafe() {
		t.Fatal("bash should not be concurrency-safe")
	}
	if !bash.IsDestructive() {
		t.Fatal("bash should be destructive")
	}

	fr := NewFileRead()
	if !fr.IsReadOnly() {
		t.Fatal("file_read should be read-only")
	}
	if !fr.IsConcurrencySafe() {
		t.Fatal("file_read should be concurrency-safe")
	}

	fw := NewFileWrite()
	if fw.IsReadOnly() {
		t.Fatal("file_write should not be read-only")
	}
	if !fw.IsConcurrencySafe() {
		t.Fatal("file_write should be concurrency-safe")
	}
}
