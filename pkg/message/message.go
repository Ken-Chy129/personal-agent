package message

// Role represents the sender of a message.
type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
	RoleSystem    Role = "system"
)

// Message represents a single message in a conversation.
type Message struct {
	Role       Role
	Content    string     // text content
	ToolCalls  []ToolCall // tool calls requested by assistant
	ToolCallID string     // for tool result messages: the ID of the call this result belongs to
	Name       string     // tool name (used in tool result messages)
}

// ToolCall represents a tool invocation requested by the assistant.
type ToolCall struct {
	ID        string // unique ID for this call
	Name      string // tool name
	Arguments string // raw JSON arguments
}

// NewUserMessage creates a user message.
func NewUserMessage(content string) Message {
	return Message{Role: RoleUser, Content: content}
}

// NewSystemMessage creates a system message.
func NewSystemMessage(content string) Message {
	return Message{Role: RoleSystem, Content: content}
}

// NewAssistantMessage creates an assistant text message.
func NewAssistantMessage(content string) Message {
	return Message{Role: RoleAssistant, Content: content}
}

// NewAssistantToolCallMessage creates an assistant message requesting tool calls.
func NewAssistantToolCallMessage(content string, toolCalls []ToolCall) Message {
	return Message{Role: RoleAssistant, Content: content, ToolCalls: toolCalls}
}

// NewToolResultMessage creates a tool result message.
func NewToolResultMessage(toolCallID, name, content string) Message {
	return Message{Role: RoleTool, Content: content, ToolCallID: toolCallID, Name: name}
}

// HasToolCalls returns true if the message contains tool call requests.
func (m Message) HasToolCalls() bool {
	return len(m.ToolCalls) > 0
}
