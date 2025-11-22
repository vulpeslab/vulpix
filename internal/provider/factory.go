package provider

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/vulpeslab/vulpix/pkg/core"
)

func NewProvider(config *core.Config) (core.Provider, error) {
	// Parse provider/model string
	// Format: "provider/model" e.g. "openai/gpt-4o"
	// If no provider specified, default to openai

	parts := strings.Split(config.Model, "/")
	var providerName, modelName string

	if len(parts) >= 2 {
		providerName = parts[0]
		modelName = strings.Join(parts[1:], "/")
	} else {
		providerName = "openai"
		modelName = config.Model
	}

	provConfig, ok := config.Providers[providerName]
	if !ok {
		return nil, fmt.Errorf("provider %s not configured", providerName)
	}

	apiKey := provConfig.APIKey
	if apiKey == "" {
		// Try env var
		envVar := strings.ToUpper(providerName) + "_API_KEY"
		apiKey = os.Getenv(envVar)
	}
	if apiKey == "" {
		return nil, fmt.Errorf("API key for %s not found in config or environment variable %s", providerName, strings.ToUpper(providerName)+"_API_KEY")
	}

	modelConfig := provConfig.Models[modelName]

	switch providerName {
	case "openai", "openrouter", "chutes", "nahcrof", "nebius":
		return NewOpenAIProvider(apiKey, provConfig.BaseURL, modelName, config.CodebaseIndexing.EmbeddingModel, modelConfig), nil
	case "anthropic":
		return NewAnthropicProvider(apiKey, provConfig.BaseURL, modelName, modelConfig), nil
	default:
		return nil, fmt.Errorf("unsupported provider: %s", providerName)
	}
}

type ErrorProvider struct {
	Err error
}

func NewErrorProvider(err error) *ErrorProvider {
	return &ErrorProvider{Err: err}
}

func (p *ErrorProvider) StreamCompletion(ctx context.Context, messages []core.Message, tools []core.Tool) (<-chan core.StreamEvent, error) {
	return nil, p.Err
}

func (p *ErrorProvider) Embed(ctx context.Context, text string) ([]float32, error) {
	return nil, p.Err
}

func (p *ErrorProvider) GetModels(ctx context.Context) ([]string, error) {
	return nil, p.Err
}

func (p *ErrorProvider) GetContextWindow(ctx context.Context, model string) (int, error) {
	return 0, p.Err
}
