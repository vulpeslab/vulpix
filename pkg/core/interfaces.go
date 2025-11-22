package core

import (
	"context"
	"time"
)

// Message represents a single unit of communication.
type Message struct {
	ID         string
	Role       string
	Content    string
	ToolCalls  []ToolCall
	ToolCallID string // For tool result messages
	Timestamp  time.Time
}

// ToolCall represents a request to execute a tool.
type ToolCall struct {
	ID        string
	Name      string
	Arguments map[string]any
}

// Session represents a single interactive session.
type Session struct {
	ID        string
	Messages  []Message
	Context   map[string]any
	CreatedAt time.Time
}

// Action represents a decision made by the agent.
type Action struct {
	Type      string // "tool", "message", "stop", "usage"
	Content   string // Message content or reasoning
	ToolCalls []ToolCall
	Usage     Usage
}

// Result represents the outcome of an action.
type Result struct {
	Success bool
	Output  string
	Error   error
}

// Usage represents token usage statistics.
type Usage struct {
	InputTokens  int
	OutputTokens int
	TotalTokens  int
}

// StreamEvent represents a chunk of data from the provider.
type StreamEvent struct {
	Type      string // "content", "tool_call", "reasoning", "usage"
	Content   string
	ToolCalls []ToolCall
	Usage     Usage
}

// Mode represents the operating mode of the agent.
type Mode string

const (
	ModeAgent Mode = "Agent"
	ModePlan  Mode = "Plan"
	ModeAsk   Mode = "Ask"
)

// Provider defines the interface for AI model providers (OpenAI, Anthropic, etc.)
type Provider interface {
	// StreamCompletion streams the response from the LLM.
	// It returns a channel that emits chunks of the response.
	StreamCompletion(ctx context.Context, messages []Message, tools []Tool) (<-chan StreamEvent, error)

	// Embed generates vector embeddings for the given text.
	// Deprecated: Use EmbeddingProvider instead.
	Embed(ctx context.Context, text string) ([]float32, error)

	// GetModels returns a list of available models.
	GetModels(ctx context.Context) ([]string, error)

	// GetContextWindow returns the context window size for the given model.
	// Returns 0 if unknown.
	GetContextWindow(ctx context.Context, model string) (int, error)
}

// EmbeddingProvider defines the interface for embedding providers.
type EmbeddingProvider interface {
	// Embed generates vector embeddings for the given text.
	Embed(ctx context.Context, text string) ([]float32, error)
}

// Tool defines an executable capability.
type Tool interface {
	// Name returns the unique name of the tool.
	Name() string

	// Description returns a human-readable description.
	Description() string

	// Schema returns the JSON schema for the tool's arguments.
	Schema() string

	// Execute runs the tool with the given arguments.
	Execute(ctx context.Context, args map[string]any) (string, error)

	// IsDangerous returns true if the tool has side effects that require user approval.
	IsDangerous() bool
}

// Agent defines the high-level agent logic.
type Agent interface {
	// Think processes the current session state and decides on the next action.
	Think(ctx context.Context, session *Session) (Action, error)

	// Act executes the decided action (e.g., running a tool).
	Act(ctx context.Context, action Action) (Result, error)
}
