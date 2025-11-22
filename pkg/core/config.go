package core

// Config represents the user configuration.
type Config struct {
	Version              int                       `json:"version"`
	Model                string                    `json:"model"`
	APIKeys              map[string]string         `json:"api_keys"` // Deprecated: use Providers
	Providers            map[string]ProviderConfig `json:"providers"`
	AutoApprove          bool                      `json:"auto_approve"`
	Theme                string                    `json:"theme"`
	LogLevel             string                    `json:"log_level"`
	CodebaseIndexing     CodebaseIndexingConfig    `json:"codebase_indexing"`
	CollapseReasoning    bool                      `json:"collapse_reasoning"`
	TruncateToolResponse bool                      `json:"truncate_tool_response"`
}

// ProviderConfig represents configuration for an AI provider.
type ProviderConfig struct {
	APIKey  string                 `json:"api_key"`
	BaseURL string                 `json:"base_url"`
	Models  map[string]ModelConfig `json:"models"`
}

// ModelConfig represents configuration for a specific model.
type ModelConfig struct {
	Temperature     *float64 `json:"temperature,omitempty"`
	ReasoningEffort *string  `json:"reasoning_effort,omitempty"`
	MaxTokens       *int     `json:"max_tokens,omitempty"`
	ContextWindow   *int     `json:"context_window,omitempty"`
}

// CodebaseIndexingConfig configuration for the RAG engine.
type CodebaseIndexingConfig struct {
	Enabled        bool   `json:"enabled"`
	EmbeddingModel string `json:"embedding_model"`
}
