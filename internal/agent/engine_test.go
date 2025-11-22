package agent

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/vulpeslab/vulpix/pkg/core"
)

type MockProvider struct {
	StreamFunc func(ctx context.Context, messages []core.Message, tools []core.Tool) (<-chan core.StreamEvent, error)
}

func (m *MockProvider) StreamCompletion(ctx context.Context, messages []core.Message, tools []core.Tool) (<-chan core.StreamEvent, error) {
	return m.StreamFunc(ctx, messages, tools)
}

func (m *MockProvider) Embed(ctx context.Context, text string) ([]float32, error) {
	return []float32{0.1, 0.2}, nil
}

func (m *MockProvider) GetModels(ctx context.Context) ([]string, error) {
	return []string{"mock-model"}, nil
}

func (m *MockProvider) GetContextWindow(ctx context.Context, model string) (int, error) {
	return 4096, nil
}

type MockTool struct {
	NameVal      string
	DangerousVal bool
	ExecuteFunc  func(ctx context.Context, args map[string]any) (string, error)
}

func (m *MockTool) Name() string        { return m.NameVal }
func (m *MockTool) Description() string { return "Mock Tool" }
func (m *MockTool) Schema() string      { return "{}" }
func (m *MockTool) Execute(ctx context.Context, args map[string]any) (string, error) {
	if m.ExecuteFunc != nil {
		return m.ExecuteFunc(ctx, args)
	}
	return "success", nil
}
func (m *MockTool) IsDangerous() bool { return m.DangerousVal }

func TestEngine_Chat_SimpleMessage(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	provider := &MockProvider{
		StreamFunc: func(ctx context.Context, messages []core.Message, tools []core.Tool) (<-chan core.StreamEvent, error) {
			ch := make(chan core.StreamEvent)
			go func() {
				ch <- core.StreamEvent{Type: "content", Content: "Hello"}
				close(ch)
			}()
			return ch, nil
		},
	}

	engine := NewEngine(provider, nil, logger)
	ctx := context.Background()

	actionChan, err := engine.Chat(ctx, "Hi", core.ModeAgent)
	if err != nil {
		t.Fatalf("Chat failed: %v", err)
	}

	var actions []core.Action
	for action := range actionChan {
		actions = append(actions, action)
	}

	if len(actions) != 1 {
		t.Errorf("Expected 1 action, got %d", len(actions))
	}
	if actions[0].Content != "Hello" {
		t.Errorf("Expected content 'Hello', got '%s'", actions[0].Content)
	}
}

func TestEngine_Chat_ToolExecution(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	tool := &MockTool{NameVal: "test_tool", DangerousVal: false}

	provider := &MockProvider{
		StreamFunc: func(ctx context.Context, messages []core.Message, tools []core.Tool) (<-chan core.StreamEvent, error) {
			ch := make(chan core.StreamEvent)
			go func() {
				lastMsg := messages[len(messages)-1]
				switch lastMsg.Role {
				case "user":
					ch <- core.StreamEvent{
						Type: "tool_call",
						ToolCalls: []core.ToolCall{
							{ID: "call_1", Name: "test_tool", Arguments: map[string]any{"_raw": "{}"}},
						},
					}
				case "tool":
					ch <- core.StreamEvent{Type: "content", Content: "Tool executed"}
				}
				close(ch)
			}()
			return ch, nil
		},
	}

	engine := NewEngine(provider, []core.Tool{tool}, logger)
	ctx := context.Background()

	actionChan, err := engine.Chat(ctx, "Run tool", core.ModeAgent)
	if err != nil {
		t.Fatalf("Chat failed: %v", err)
	}

	var actions []core.Action
	for action := range actionChan {
		actions = append(actions, action)
	}

	foundToolStart := false
	foundToolResult := false
	foundMessage := false

	for _, a := range actions {
		if a.Type == "tool_start" {
			foundToolStart = true
		}
		if a.Type == "tool_result" {
			foundToolResult = true
		}
		if a.Type == "message" && a.Content == "Tool executed" {
			foundMessage = true
		}
	}

	if !foundToolStart {
		t.Error("Missing tool_start action")
	}
	if !foundToolResult {
		t.Error("Missing tool_result action")
	}
	if !foundMessage {
		t.Error("Missing final message action")
	}
}
