package tools

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	oai "github.com/openai/openai-go"
	"github.com/openai/openai-go/option"

	"github.com/Ken-Chy129/personal-agent/pkg/agent"
)

type imageGenerateInput struct {
	Prompt  string `json:"prompt"`
	Path    string `json:"path"`    // output file path (e.g. "output.png")
	Model   string `json:"model"`   // "gpt-image-1" (default), "gpt-image-1-mini", "dall-e-3", etc.
	Size    string `json:"size"`    // "1024x1024", "1536x1024", "1024x1536", "auto"
	Quality string `json:"quality"` // "low", "medium", "high", "auto"
}

// ImageGenerateTool generates images using OpenAI's gpt-image-1 model.
type ImageGenerateTool struct {
	client *oai.Client
}

func NewImageGenerate(apiKey string) *ImageGenerateTool {
	if apiKey == "" {
		apiKey = os.Getenv("OPENAI_API_KEY")
	}
	c := oai.NewClient(option.WithAPIKey(apiKey))
	return &ImageGenerateTool{client: &c}
}

func (t *ImageGenerateTool) Name() string        { return "image_generate" }
func (t *ImageGenerateTool) Description() string {
	return "Generate an image from a text prompt using GPT Image (gpt-image-1). Saves the image to the specified path."
}
func (t *ImageGenerateTool) IsReadOnly() bool       { return false }
func (t *ImageGenerateTool) IsConcurrencySafe() bool { return true }
func (t *ImageGenerateTool) IsDestructive() bool     { return false }

func (t *ImageGenerateTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"prompt": map[string]any{
				"type":        "string",
				"description": "A text description of the desired image",
			},
			"path": map[string]any{
				"type":        "string",
				"description": "Output file path for the generated image (e.g. 'output.png'). Supported formats: png, jpeg, webp",
			},
			"model": map[string]any{
				"type":        "string",
				"description": "Image model to use. Default: 'gpt-image-1'. Options: 'gpt-image-1', 'dall-e-3', etc.",
			},
			"size": map[string]any{
				"type":        "string",
				"description": "Image size: '1024x1024' (square), '1536x1024' (landscape), '1024x1536' (portrait), or 'auto' (default)",
				"enum":        []any{"auto", "1024x1024", "1536x1024", "1024x1536"},
			},
			"quality": map[string]any{
				"type":        "string",
				"description": "Image quality: 'low', 'medium', 'high', or 'auto' (default)",
				"enum":        []any{"auto", "low", "medium", "high"},
			},
		},
		"required": []any{"prompt", "path"},
	}
}

func (t *ImageGenerateTool) Execute(ctx context.Context, input json.RawMessage) (*agent.ToolResult, error) {
	var in imageGenerateInput
	if err := json.Unmarshal(input, &in); err != nil {
		return nil, fmt.Errorf("parse image_generate input: %w", err)
	}
	if in.Prompt == "" {
		return &agent.ToolResult{Content: "error: prompt is empty", IsError: true}, nil
	}
	if in.Path == "" {
		return &agent.ToolResult{Content: "error: path is empty", IsError: true}, nil
	}

	// Determine output format from file extension
	outputFormat := oai.ImageGenerateParamsOutputFormat("png")
	ext := strings.ToLower(filepath.Ext(in.Path))
	switch ext {
	case ".png":
		outputFormat = "png"
	case ".jpg", ".jpeg":
		outputFormat = "jpeg"
	case ".webp":
		outputFormat = "webp"
	default:
		return &agent.ToolResult{
			Content: fmt.Sprintf("error: unsupported file extension %q, use .png, .jpg, or .webp", ext),
			IsError: true,
		}, nil
	}

	model := oai.ImageModel("gpt-image-1")
	if in.Model != "" {
		model = oai.ImageModel(in.Model)
	}

	params := oai.ImageGenerateParams{
		Prompt:       in.Prompt,
		Model:        model,
		OutputFormat: outputFormat,
	}
	if in.Size != "" && in.Size != "auto" {
		params.Size = oai.ImageGenerateParamsSize(in.Size)
	}
	if in.Quality != "" && in.Quality != "auto" {
		params.Quality = oai.ImageGenerateParamsQuality(in.Quality)
	}

	resp, err := t.client.Images.Generate(ctx, params)
	if err != nil {
		return &agent.ToolResult{
			Content: fmt.Sprintf("error generating image: %s", err),
			IsError: true,
		}, nil
	}

	if len(resp.Data) == 0 {
		return &agent.ToolResult{Content: "error: no image data returned", IsError: true}, nil
	}

	// Decode base64 image
	imgData, err := base64.StdEncoding.DecodeString(resp.Data[0].B64JSON)
	if err != nil {
		return &agent.ToolResult{
			Content: fmt.Sprintf("error decoding image data: %s", err),
			IsError: true,
		}, nil
	}

	// Create parent directories
	dir := filepath.Dir(in.Path)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return &agent.ToolResult{
				Content: fmt.Sprintf("error creating directory: %s", err),
				IsError: true,
			}, nil
		}
	}

	if err := os.WriteFile(in.Path, imgData, 0644); err != nil {
		return &agent.ToolResult{
			Content: fmt.Sprintf("error writing image file: %s", err),
			IsError: true,
		}, nil
	}

	return &agent.ToolResult{
		Content: fmt.Sprintf("Image generated and saved to %s (%d bytes, %s)", in.Path, len(imgData), resp.Size),
	}, nil
}
