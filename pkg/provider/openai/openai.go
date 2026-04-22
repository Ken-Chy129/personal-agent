package openai

import (
	"context"
	"fmt"
	"os"

	oai "github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/shared"

	"github.com/Ken-Chy129/personal-agent/pkg/message"
	"github.com/Ken-Chy129/personal-agent/pkg/provider"
)

const defaultModel = "gpt-4o"

// Provider implements the provider.Provider interface using the OpenAI API.
type Provider struct {
	client  *oai.Client
	model   string
	baseURL string
}

// Option configures the OpenAI provider.
type Option func(*Provider)

func WithBaseURL(url string) Option {
	return func(p *Provider) {
		p.baseURL = url
	}
}

func WithModel(model string) Option {
	return func(p *Provider) {
		p.model = model
	}
}

// NewProvider creates a new OpenAI provider.
// If apiKey is empty, it falls back to the OPENAI_API_KEY environment variable.
func NewProvider(apiKey string, opts ...Option) *Provider {
	if apiKey == "" {
		apiKey = os.Getenv("OPENAI_API_KEY")
	}

	p := &Provider{model: defaultModel}
	for _, opt := range opts {
		opt(p)
	}
	if p.baseURL == "" {
		p.baseURL = os.Getenv("OPENAI_BASE_URL")
	}

	clientOpts := []option.RequestOption{
		option.WithAPIKey(apiKey),
	}
	if p.baseURL != "" {
		clientOpts = append(clientOpts, option.WithBaseURL(p.baseURL))
	}

	c := oai.NewClient(clientOpts...)
	p.client = &c
	return p
}

func (p *Provider) Name() string { return "openai" }

func (p *Provider) Stream(ctx context.Context, req *provider.ChatRequest) (<-chan provider.StreamEvent, error) {
	model := p.model
	if req.Model != "" {
		model = req.Model
	}

	msgs := convertMessagesToOpenAI(req.Messages, req.SystemPrompt)
	tools := convertToolsToOpenAI(req.Tools)

	params := oai.ChatCompletionNewParams{
		Model:    shared.ChatModel(model),
		Messages: msgs,
	}
	if len(tools) > 0 {
		params.Tools = tools
	}
	if req.MaxTokens > 0 {
		params.MaxCompletionTokens = oai.Int(int64(req.MaxTokens))
	}

	stream := p.client.Chat.Completions.NewStreaming(ctx, params)

	ch := make(chan provider.StreamEvent, 32)
	go func() {
		defer close(ch)
		acc := oai.ChatCompletionAccumulator{}

		for stream.Next() {
			chunk := stream.Current()
			acc.AddChunk(chunk)

			// Emit text deltas
			if len(chunk.Choices) > 0 {
				delta := chunk.Choices[0].Delta
				if delta.Content != "" {
					ch <- provider.StreamEvent{
						Type:      "text_delta",
						TextDelta: delta.Content,
					}
				}
			}
		}

		if err := stream.Err(); err != nil {
			ch <- provider.StreamEvent{Type: "error", Error: err}
			return
		}

		// Build final response from accumulated result
		resp := convertResponseFromOpenAI(&oai.ChatCompletion{
			Choices: []oai.ChatCompletionChoice{acc.Choices[0]},
			Usage:   acc.Usage,
			Model:   acc.Model,
		}, model)

		ch <- provider.StreamEvent{Type: "done", Response: resp}
	}()

	return ch, nil
}

func (p *Provider) Chat(ctx context.Context, req *provider.ChatRequest) (*provider.ChatResponse, error) {
	model := p.model
	if req.Model != "" {
		model = req.Model
	}

	msgs := convertMessagesToOpenAI(req.Messages, req.SystemPrompt)
	tools := convertToolsToOpenAI(req.Tools)

	params := oai.ChatCompletionNewParams{
		Model:    shared.ChatModel(model),
		Messages: msgs,
	}
	if len(tools) > 0 {
		params.Tools = tools
	}
	if req.MaxTokens > 0 {
		params.MaxCompletionTokens = oai.Int(int64(req.MaxTokens))
	}

	completion, err := p.client.Chat.Completions.New(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("openai chat: %w", err)
	}

	return convertResponseFromOpenAI(completion, model), nil
}

// convertMessagesToOpenAI converts internal messages to OpenAI SDK format.
func convertMessagesToOpenAI(msgs []message.Message, systemPrompt string) []oai.ChatCompletionMessageParamUnion {
	var result []oai.ChatCompletionMessageParamUnion

	if systemPrompt != "" {
		result = append(result, oai.SystemMessage(systemPrompt))
	}

	for _, msg := range msgs {
		switch msg.Role {
		case message.RoleSystem:
			result = append(result, oai.SystemMessage(msg.Content))
		case message.RoleUser:
			result = append(result, oai.UserMessage(msg.Content))
		case message.RoleAssistant:
			if msg.HasToolCalls() {
				// Assistant message with tool calls
				asst := oai.ChatCompletionAssistantMessageParam{}
				if msg.Content != "" {
					asst.Content.OfString = oai.String(msg.Content)
				}
				asst.ToolCalls = make([]oai.ChatCompletionMessageToolCallParam, len(msg.ToolCalls))
				for i, tc := range msg.ToolCalls {
					asst.ToolCalls[i] = oai.ChatCompletionMessageToolCallParam{
						ID: tc.ID,
						Function: oai.ChatCompletionMessageToolCallFunctionParam{
							Name:      tc.Name,
							Arguments: tc.Arguments,
						},
					}
				}
				result = append(result, oai.ChatCompletionMessageParamUnion{OfAssistant: &asst})
			} else {
				result = append(result, oai.AssistantMessage(msg.Content))
			}
		case message.RoleTool:
			result = append(result, oai.ToolMessage(msg.Content, msg.ToolCallID))
		}
	}
	return result
}

// convertToolsToOpenAI converts internal tool definitions to OpenAI SDK format.
func convertToolsToOpenAI(tools []provider.ToolDefinition) []oai.ChatCompletionToolParam {
	if len(tools) == 0 {
		return nil
	}
	result := make([]oai.ChatCompletionToolParam, len(tools))
	for i, t := range tools {
		result[i] = oai.ChatCompletionToolParam{
			Function: shared.FunctionDefinitionParam{
				Name:        t.Name,
				Description: oai.String(t.Description),
				Parameters:  shared.FunctionParameters(t.Parameters),
			},
		}
	}
	return result
}

// convertResponseFromOpenAI converts an OpenAI response to the internal format.
func convertResponseFromOpenAI(c *oai.ChatCompletion, requestedModel string) *provider.ChatResponse {
	resp := &provider.ChatResponse{
		Usage: provider.Usage{
			InputTokens:  int(c.Usage.PromptTokens),
			OutputTokens: int(c.Usage.CompletionTokens),
			Model:        c.Model,
		},
	}

	if len(c.Choices) == 0 {
		resp.StopReason = "stop"
		return resp
	}

	choice := c.Choices[0]
	resp.Content = choice.Message.Content
	resp.StopReason = choice.FinishReason

	if len(choice.Message.ToolCalls) > 0 {
		resp.StopReason = "tool_calls"
		resp.ToolCalls = make([]message.ToolCall, len(choice.Message.ToolCalls))
		for i, tc := range choice.Message.ToolCalls {
			resp.ToolCalls[i] = message.ToolCall{
				ID:        tc.ID,
				Name:      tc.Function.Name,
				Arguments: tc.Function.Arguments,
			}
		}
	}

	return resp
}
