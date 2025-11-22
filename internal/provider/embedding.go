package provider

import (
	"context"
	"fmt"
	"os"
	"strings"

	openai "github.com/sashabaranov/go-openai"
	"github.com/vulpeslab/vulpix/pkg/core"
)

type OpenAIEmbeddingProvider struct {
	client *openai.Client
	model  string
}

func NewOpenAIEmbeddingProvider(apiKey, baseURL, model string) *OpenAIEmbeddingProvider {
	config := openai.DefaultConfig(apiKey)
	if baseURL != "" {
		config.BaseURL = strings.TrimSuffix(baseURL, "/")
	}
	config.HTTPClient = NewLoggingClient()
	return &OpenAIEmbeddingProvider{
		client: openai.NewClientWithConfig(config),
		model:  model,
	}
}

func (p *OpenAIEmbeddingProvider) Embed(ctx context.Context, text string) ([]float32, error) {
	model := p.model
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

func NewEmbeddingProvider(config *core.Config) (core.EmbeddingProvider, error) {
	// Format: "provider/model" e.g. "openai/text-embedding-3-small"
	embeddingModel := config.CodebaseIndexing.EmbeddingModel
	parts := strings.Split(embeddingModel, "/")
	var providerName, modelName string

	if len(parts) >= 2 {
		providerName = parts[0]
		modelName = strings.Join(parts[1:], "/")
	} else {
		providerName = "openai"
		modelName = embeddingModel
	}

	provConfig, ok := config.Providers[providerName]
	if !ok {
		// Fallback for nebius if not explicitly in providers map but requested?
		// The user said "The API-Keys from ... providers should be used".
		// If the user adds "nebius" to providers in config, it will be there.
		// If not, we can't really proceed without an API key unless we have a default or env var.
		// Let's check env var as fallback like in factory.go
	}

	apiKey := ""
	baseURL := ""

	if ok {
		apiKey = provConfig.APIKey
		baseURL = provConfig.BaseURL
	}

	if apiKey == "" {
		envVar := strings.ToUpper(providerName) + "_API_KEY"
		apiKey = os.Getenv(envVar)
	}

	if apiKey == "" {
		return nil, fmt.Errorf("API key for %s not found", providerName)
	}

	// Override BaseURL for specific providers if not set in config
	if baseURL == "" {
		switch providerName {
		case "openai":
			baseURL = "https://api.openai.com/v1"
		case "openrouter":
			baseURL = "https://openrouter.ai/api/v1"
		case "nebius":
			baseURL = "https://api.tokenfactory.nebius.com/v1"
		}
	}

	// Ensure no trailing slash
	baseURL = strings.TrimSuffix(baseURL, "/")

	switch providerName {
	case "openai", "openrouter", "nebius":
		return NewOpenAIEmbeddingProvider(apiKey, baseURL, modelName), nil
	default:
		return nil, fmt.Errorf("unsupported embedding provider: %s", providerName)
	}
}
