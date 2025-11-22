package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	openai "github.com/sashabaranov/go-openai"
	"github.com/vulpeslab/vulpix/pkg/core"
)

type OpenAIProvider struct {
	client         *openai.Client
	httpClient     *http.Client
	apiKey         string
	baseURL        string
	model          string
	embeddingModel string
	modelConfig    core.ModelConfig
}

func NewOpenAIProvider(apiKey, baseURL, model, embeddingModel string, modelConfig core.ModelConfig) *OpenAIProvider {
	config := openai.DefaultConfig(apiKey)
	if baseURL != "" {
		config.BaseURL = baseURL
	}
	httpClient := NewLoggingClient()
	config.HTTPClient = httpClient
	return &OpenAIProvider{
		client:         openai.NewClientWithConfig(config),
		httpClient:     httpClient,
		apiKey:         apiKey,
		baseURL:        config.BaseURL,
		model:          model,
		embeddingModel: embeddingModel,
		modelConfig:    modelConfig,
	}
}

func (p *OpenAIProvider) StreamCompletion(ctx context.Context, messages []core.Message, tools []core.Tool) (<-chan core.StreamEvent, error) {
	reqMessages := make([]openai.ChatCompletionMessage, len(messages))
	for i, m := range messages {
		reqMessages[i] = openai.ChatCompletionMessage{
			Role:       m.Role,
			Content:    m.Content,
			ToolCallID: m.ToolCallID,
		}
		if len(m.ToolCalls) > 0 {
			reqMessages[i].ToolCalls = make([]openai.ToolCall, len(m.ToolCalls))
			for j, tc := range m.ToolCalls {
				argsJSON, _ := json.Marshal(tc.Arguments)
				reqMessages[i].ToolCalls[j] = openai.ToolCall{
					ID:   tc.ID,
					Type: openai.ToolTypeFunction,
					Function: openai.FunctionCall{
						Name:      tc.Name,
						Arguments: string(argsJSON),
					},
				}
			}
		}
	}

	var toolDefs []openai.Tool
	for _, t := range tools {
		toolDefs = append(toolDefs, openai.Tool{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        t.Name(),
				Description: t.Description(),
				Parameters:  json.RawMessage(t.Schema()),
			},
		})
	}

	req := openai.ChatCompletionRequest{
		Model:    p.model,
		Messages: reqMessages,
		Tools:    toolDefs,
		Stream:   true,
		StreamOptions: &openai.StreamOptions{
			IncludeUsage: true,
		},
	}

	if p.modelConfig.Temperature != nil {
		t := float32(*p.modelConfig.Temperature)
		req.Temperature = t
	}
	if p.modelConfig.MaxTokens != nil {
		req.MaxTokens = *p.modelConfig.MaxTokens
	}
	// ReasoningEffort is for o1 models, check if supported by library or use generic map if needed
	// sashabaranov/go-openai might not support ReasoningEffort yet directly, but let's check if we can pass it.
	// For now, we'll skip ReasoningEffort if not supported by the struct.

	stream, err := p.client.CreateChatCompletionStream(ctx, req)
	if err != nil {
		return nil, err
	}

	ch := make(chan core.StreamEvent)
	go func() {
		defer close(ch)
		defer stream.Close()

		for {
			response, err := stream.Recv()
			if errors.Is(err, io.EOF) {
				return
			}
			if err != nil {
				// TODO: Log error?
				return
			}

			if response.Usage != nil {
				ch <- core.StreamEvent{
					Type: "usage",
					Usage: core.Usage{
						InputTokens:  response.Usage.PromptTokens,
						OutputTokens: response.Usage.CompletionTokens,
						TotalTokens:  response.Usage.TotalTokens,
					},
				}
			}

			if len(response.Choices) > 0 {
				delta := response.Choices[0].Delta
				if delta.ReasoningContent != "" {
					ch <- core.StreamEvent{
						Type:    "reasoning",
						Content: delta.ReasoningContent,
					}
				}
				if delta.Content != "" {
					ch <- core.StreamEvent{
						Type:    "content",
						Content: delta.Content,
					}
				}
				if len(delta.ToolCalls) > 0 {
					var tcs []core.ToolCall
					for _, tc := range delta.ToolCalls {
						tcs = append(tcs, core.ToolCall{
							ID:   tc.ID,
							Name: tc.Function.Name,
							// Note: Arguments come as partial strings in streaming.
							// We need to accumulate them in the engine or here.
							// For simplicity, let's pass the partial string in Arguments map with a special key?
							// Or better: Just pass the raw string in a special field in ToolCall?
							// core.ToolCall has map[string]any.
							// Let's change core.ToolCall to have ArgumentsRaw string for streaming?
							// Or just pass it as "arguments": string
							Arguments: map[string]any{"_raw": tc.Function.Arguments},
						})
					}
					ch <- core.StreamEvent{
						Type:      "tool_call",
						ToolCalls: tcs,
					}
				}
			}
		}
	}()

	return ch, nil
}

func (p *OpenAIProvider) Embed(ctx context.Context, text string) ([]float32, error) {
	model := p.embeddingModel
	if model == "" {
		model = string(openai.SmallEmbedding3)
	}

	resp, err := p.client.CreateEmbeddings(ctx, openai.EmbeddingRequest{
		Input: []string{text},
		Model: openai.EmbeddingModel(model),
	})
	if err != nil {
		return nil, err
	}
	return resp.Data[0].Embedding, nil
}

func (p *OpenAIProvider) GetModels(ctx context.Context) ([]string, error) {
	list, err := p.client.ListModels(ctx)
	if err != nil {
		return nil, err
	}
	var models []string
	for _, m := range list.Models {
		models = append(models, m.ID)
	}
	return models, nil
}

func (p *OpenAIProvider) GetContextWindow(ctx context.Context, model string) (int, error) {
	if p.modelConfig.ContextWindow != nil {
		return *p.modelConfig.ContextWindow, nil
	}

	// Try to fetch from API
	url := p.baseURL + "/models"
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("failed to list models: %s", resp.Status)
	}

	var result struct {
		Data []struct {
			ID            string `json:"id"`
			ContextWindow int    `json:"context_window"` // Nebius?
			ContextLength int    `json:"context_length"` // OpenRouter?
			MaxContext    int    `json:"max_context"`    // Others?
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, err
	}

	for _, m := range result.Data {
		// Match against p.model (the configured model ID) or the passed model argument
		// We prefer p.model because it's what we initialized the provider with (stripped of prefix)
		if m.ID == p.model || m.ID == model || strings.HasSuffix(model, m.ID) {
			if m.ContextWindow > 0 {
				return m.ContextWindow, nil
			}
			if m.ContextLength > 0 {
				return m.ContextLength, nil
			}
			if m.MaxContext > 0 {
				return m.MaxContext, nil
			}
		}
	}

	return 0, nil
}
