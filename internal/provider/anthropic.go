package provider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/vulpeslab/vulpix/pkg/core"
)

type AnthropicProvider struct {
	apiKey      string
	baseURL     string
	model       string
	modelConfig core.ModelConfig
	client      *http.Client
}

func NewAnthropicProvider(apiKey, baseURL, model string, modelConfig core.ModelConfig) *AnthropicProvider {
	if baseURL == "" {
		baseURL = "https://api.anthropic.com/v1"
	}
	return &AnthropicProvider{
		apiKey:      apiKey,
		baseURL:     baseURL,
		model:       model,
		modelConfig: modelConfig,
		client:      &http.Client{},
	}
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"` // string or []contentBlock
}

type contentBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   string          `json:"content,omitempty"`
}

type anthropicTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
}

type anthropicRequest struct {
	Model       string             `json:"model"`
	Messages    []anthropicMessage `json:"messages"`
	Tools       []anthropicTool    `json:"tools,omitempty"`
	Stream      bool               `json:"stream"`
	MaxTokens   int                `json:"max_tokens"`
	Temperature *float64           `json:"temperature,omitempty"`
	System      string             `json:"system,omitempty"`
}

func (p *AnthropicProvider) StreamCompletion(ctx context.Context, messages []core.Message, tools []core.Tool) (<-chan core.StreamEvent, error) {
	var anthropicMessages []anthropicMessage
	var systemPrompt string

	for _, m := range messages {
		if m.Role == "system" {
			systemPrompt += m.Content + "\n"
			continue
		}

		var content any = m.Content
		if len(m.ToolCalls) > 0 {
			// Assistant message with tool calls
			var blocks []contentBlock
			if m.Content != "" {
				blocks = append(blocks, contentBlock{Type: "text", Text: m.Content})
			}
			for _, tc := range m.ToolCalls {
				argsJSON, _ := json.Marshal(tc.Arguments)
				blocks = append(blocks, contentBlock{
					Type:  "tool_use",
					ID:    tc.ID,
					Name:  tc.Name,
					Input: argsJSON,
				})
			}
			content = blocks
		} else if m.Role == "tool" {
			// Tool result
			// Anthropic expects tool results in user messages with type tool_result
			// But core.Message has Role "tool". We need to map this to "user" with content block.
			// Wait, core.Message structure for tool result is: Role="tool", ToolCallID="...", Content="..."
			// Anthropic expects: Role="user", Content=[{type="tool_result", tool_use_id="...", content="..."}]
			// We need to handle this carefully.
			// The loop structure here assumes 1:1 mapping, but we might need to merge consecutive tool results?
			// For now, let's map it to a user message.
			content = []contentBlock{
				{
					Type:      "tool_result",
					ToolUseID: m.ToolCallID,
					Content:   m.Content,
				},
			}
			// Note: Anthropic requires alternating user/assistant messages.
			// If we have multiple tool results, they should probably be in one user message.
			// But core.Message splits them.
			// This is a complexity. For now, let's assume the agent handles this or we just send them as is and hope Anthropic accepts consecutive user messages (it might not).
			// Actually, Anthropic DOES NOT accept consecutive user messages.
			// We might need to coalesce them.
		}

		role := m.Role
		if role == "tool" {
			role = "user"
		}

		anthropicMessages = append(anthropicMessages, anthropicMessage{
			Role:    role,
			Content: content,
		})
	}

	// Coalesce consecutive user messages (including tool results)
	var coalescedMessages []anthropicMessage
	for _, m := range anthropicMessages {
		if len(coalescedMessages) > 0 {
			last := &coalescedMessages[len(coalescedMessages)-1]
			if last.Role == m.Role && last.Role == "user" {
				// Merge content
				// Ensure both are arrays of blocks
				lastBlocks := toBlocks(last.Content)
				currentBlocks := toBlocks(m.Content)
				last.Content = append(lastBlocks, currentBlocks...)
				continue
			}
		}
		coalescedMessages = append(coalescedMessages, m)
	}

	var anthropicTools []anthropicTool
	for _, t := range tools {
		anthropicTools = append(anthropicTools, anthropicTool{
			Name:        t.Name(),
			Description: t.Description(),
			InputSchema: json.RawMessage(t.Schema()),
		})
	}

	maxTokens := 1024
	if p.modelConfig.MaxTokens != nil {
		maxTokens = *p.modelConfig.MaxTokens
	}

	reqBody := anthropicRequest{
		Model:       p.model,
		Messages:    coalescedMessages,
		Tools:       anthropicTools,
		Stream:      true,
		MaxTokens:   maxTokens,
		Temperature: p.modelConfig.Temperature,
		System:      systemPrompt,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/messages", bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, err
	}

	req.Header.Set("x-api-key", p.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("content-type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("anthropic api error: %s", resp.Status)
	}

	ch := make(chan core.StreamEvent)
	go func() {
		defer close(ch)
		defer resp.Body.Close()

		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			data := strings.TrimPrefix(line, "data: ")
			if data == "[DONE]" {
				return
			}

			var event struct {
				Type         string          `json:"type"`
				Delta        json.RawMessage `json:"delta"`
				Index        int             `json:"index"`
				ContentBlock json.RawMessage `json:"content_block"`
				Usage        struct {
					InputTokens  int `json:"input_tokens"`
					OutputTokens int `json:"output_tokens"`
				} `json:"usage"`
				Message struct {
					Usage struct {
						InputTokens int `json:"input_tokens"`
					} `json:"usage"`
				} `json:"message"`
			}
			if err := json.Unmarshal([]byte(data), &event); err != nil {
				continue
			}

			switch event.Type {
			case "message_start":
				if event.Message.Usage.InputTokens > 0 {
					ch <- core.StreamEvent{
						Type: "usage",
						Usage: core.Usage{
							InputTokens: event.Message.Usage.InputTokens,
						},
					}
				}
			case "message_delta":
				if event.Usage.OutputTokens > 0 {
					ch <- core.StreamEvent{
						Type: "usage",
						Usage: core.Usage{
							OutputTokens: event.Usage.OutputTokens,
						},
					}
				}
			case "content_block_delta":
				var delta struct {
					Type        string `json:"type"`
					Text        string `json:"text"`
					PartialJSON string `json:"partial_json"`
				}
				json.Unmarshal(event.Delta, &delta)
				if delta.Type == "text_delta" {
					ch <- core.StreamEvent{Type: "content", Content: delta.Text}
				} else if delta.Type == "input_json_delta" {
					ch <- core.StreamEvent{
						Type: "tool_call",
						ToolCalls: []core.ToolCall{{
							Arguments: map[string]any{"_raw": delta.PartialJSON},
						}},
					}
				}
			case "content_block_start":
				var start struct {
					Type string `json:"type"`
					Name string `json:"name"`
					ID   string `json:"id"`
				}
				json.Unmarshal(event.ContentBlock, &start)
				if start.Type == "tool_use" {
					ch <- core.StreamEvent{
						Type: "tool_call",
						ToolCalls: []core.ToolCall{{
							ID:        start.ID,
							Name:      start.Name,
							Arguments: map[string]any{"_raw": ""},
						}},
					}
				}
			}
		}
	}()

	return ch, nil
}

func (p *AnthropicProvider) Embed(ctx context.Context, text string) ([]float32, error) {
	return nil, fmt.Errorf("embeddings not supported by Anthropic provider")
}

func (p *AnthropicProvider) GetModels(ctx context.Context) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", p.baseURL+"/models", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("x-api-key", p.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("anthropic api error: %s", resp.Status)
	}

	var result struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	var models []string
	for _, m := range result.Data {
		models = append(models, m.ID)
	}
	return models, nil
}

func (p *AnthropicProvider) GetContextWindow(ctx context.Context, model string) (int, error) {
	if p.modelConfig.ContextWindow != nil {
		return *p.modelConfig.ContextWindow, nil
	}

	// Try to fetch from API
	req, err := http.NewRequestWithContext(ctx, "GET", p.baseURL+"/models", nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("x-api-key", p.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := p.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("anthropic api error: %s", resp.Status)
	}

	var result struct {
		Data []struct {
			ID            string `json:"id"`
			ContextWindow int    `json:"context_window"` // Speculative
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, err
	}

	for _, m := range result.Data {
		if m.ID == p.model || m.ID == model || strings.HasSuffix(model, m.ID) {
			if m.ContextWindow > 0 {
				return m.ContextWindow, nil
			}
		}
	}

	return 0, nil
}

func toBlocks(content any) []contentBlock {
	if s, ok := content.(string); ok {
		return []contentBlock{{Type: "text", Text: s}}
	}
	if blocks, ok := content.([]contentBlock); ok {
		return blocks
	}
	return nil
}
