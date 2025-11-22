package config

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/tailscale/hujson"
	"github.com/vulpeslab/vulpix/pkg/core"
)

const CurrentConfigVersion = 2

// GetIndexPath returns the path to the index for the given codebase
func GetIndexPath(codebasePath string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	hash := sha256.Sum256([]byte(codebasePath))
	hashStr := hex.EncodeToString(hash[:])

	dir := filepath.Join(home, ".vulpix", "indexes", hashStr)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}

	return filepath.Join(dir, "vulpix.db"), nil
}

// LoadConfig loads the configuration from ~/.vulpix/config.jsonc
func LoadConfig() (*core.Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	configPath := filepath.Join(home, ".vulpix", "config.jsonc")
	data, err := os.ReadFile(configPath)
	if os.IsNotExist(err) {
		// Create default config file
		if err := createDefaultConfig(configPath); err != nil {
			return nil, err
		}
		return DefaultConfig(), nil
	}
	if err != nil {
		return nil, err
	}

	// Standardize JSONC to JSON
	ast, err := hujson.Parse(data)
	if err != nil {
		return nil, err
	}
	ast.Standardize()
	data = ast.Pack()

	var cfg core.Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	// Migration System
	if cfg.Version < CurrentConfigVersion {
		if migrateConfig(&cfg) {
			if err := saveConfig(configPath, &cfg); err != nil {
				// We continue even if save fails, but maybe we should log it?
				// For now, we just proceed with the migrated config in memory.
			}
		}
	}

	// Merge with defaults
	defaults := DefaultConfig()
	if cfg.Model == "" {
		cfg.Model = defaults.Model
	}
	if cfg.Theme == "" {
		cfg.Theme = defaults.Theme
	}
	if cfg.LogLevel == "" {
		cfg.LogLevel = defaults.LogLevel
	}
	if cfg.CodebaseIndexing.EmbeddingModel == "" {
		cfg.CodebaseIndexing.EmbeddingModel = defaults.CodebaseIndexing.EmbeddingModel
	}

	// Merge providers
	if cfg.Providers == nil {
		cfg.Providers = make(map[string]core.ProviderConfig)
	}
	for name, defProv := range defaults.Providers {
		prov, exists := cfg.Providers[name]
		if !exists {
			cfg.Providers[name] = defProv
			continue
		}
		if prov.BaseURL == "" {
			prov.BaseURL = defProv.BaseURL
		}
		// Ensure Nebius URL is correct if it was manually set to the old one
		// if name == "nebius" && strings.Contains(prov.BaseURL, "tokenfactory.nebius.com") {
		// 	prov.BaseURL = "https://api.studio.nebius.ai/v1/"
		// }
		if prov.Models == nil {
			prov.Models = make(map[string]core.ModelConfig)
		}
		// We don't necessarily merge models here, as the user might want to define their own.
		// But we could merge specific model settings if we had defaults for them.
		cfg.Providers[name] = prov
	}

	return &cfg, nil
}

// DefaultConfig returns the default configuration.
func DefaultConfig() *core.Config {
	return &core.Config{
		Version:     CurrentConfigVersion,
		Model:       "openai/gpt-4-turbo",
		AutoApprove: false,
		Theme:       "default",
		LogLevel:    "info",
		CodebaseIndexing: core.CodebaseIndexingConfig{
			Enabled:        true,
			EmbeddingModel: "openai/text-embedding-3-small",
		},
		CollapseReasoning:    false,
		TruncateToolResponse: true,
		Providers: map[string]core.ProviderConfig{
			"openai": {
				BaseURL: "https://api.openai.com/v1",
				Models: map[string]core.ModelConfig{
					"gpt-4-turbo": {Temperature: ptr(0.7)},
				},
			},
			"anthropic": {
				BaseURL: "https://api.anthropic.com/v1",
			},
			"openrouter": {
				BaseURL: "https://openrouter.ai/api/v1",
			},
			"chutes": {
				BaseURL: "https://llm.chutes.ai/v1",
			},
			"nahcrof": {
				BaseURL: "https://ai.nahcrof.com/v2",
			},
			"nebius": {
				BaseURL: "https://api.studio.nebius.ai/v1/",
			},
		},
	}
}

func ptr[T any](v T) *T {
	return &v
}

func createDefaultConfig(path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	defaultContent := `{
	// Vulpix Configuration
	"version": 1,

	// AI Provider and Model
	// Format: "provider/model"
	// Supported providers: openai, anthropic, openrouter, chutes, nahcrof
	"model": "openai/gpt-4-turbo",

	// Providers Configuration
	"providers": {
		"openai": {
			"api_key": "", // Set here or via OPENAI_API_KEY env var
			"base_url": "https://api.openai.com/v1",
			"models": {
				"gpt-4-turbo": {
					"temperature": 0.7
				}
			}
		},
		"anthropic": {
			"api_key": "", // Set here or via ANTHROPIC_API_KEY env var
			"base_url": "https://api.anthropic.com/v1",
			"models": {
				"claude-3-opus-20240229": {
					"temperature": 0.7
				}
			}
		},
		"openrouter": {
			"api_key": "", // Set here or via OPENROUTER_API_KEY env var
			"base_url": "https://openrouter.ai/api/v1",
			"models": {}
		},
		"chutes": {
			"api_key": "", // Set here or via CHUTES_API_KEY env var
			"base_url": "https://llm.chutes.ai/v1",
			"models": {}
		},
		"nahcrof": {
			"api_key": "", // Set here or via NAHCROF_API_KEY env var
			"base_url": "https://ai.nahcrof.com/v2",
			"models": {}
		},
		"nebius": {
			"api_key": "", // Set here or via NEBIUS_API_KEY env var
			"base_url": "https://api.studio.nebius.ai/v1/",
			"models": {}
		}
	},

	// Auto-approve potentially dangerous actions (use with caution)
	"auto_approve": false,

	// UI Theme
	"theme": "default",

	// Log Level (DEBUG, INFO, WARNING, ERROR)
	"log_level": "INFO",

	// RAG (Retrieval-Augmented Generation) Settings
	"codebase_indexing": {
		"enabled": true,
		"embedding_model": "openai/text-embedding-3-small"
	}
}
`
	return os.WriteFile(path, []byte(defaultContent), 0644)
}

func migrateConfig(cfg *core.Config) bool {
	updated := false
	if cfg.Version < 1 {
		// Migrate v0 -> v1
		// If user has old APIKeys but no Providers, migrate them
		if len(cfg.APIKeys) > 0 && len(cfg.Providers) == 0 {
			cfg.Providers = make(map[string]core.ProviderConfig)
			if key, ok := cfg.APIKeys["openai"]; ok {
				cfg.Providers["openai"] = core.ProviderConfig{
					APIKey:  key,
					BaseURL: "https://api.openai.com/v1",
				}
			}
		}
		cfg.Version = 1
		updated = true
	}

	if cfg.Version < 2 {
		// Migrate v1 -> v2
		// We previously forced a migration from tokenfactory to studio, but some users
		// still use tokenfactory with specific tokens. We should not force this.
		// Just bump version.
		cfg.Version = 2
		updated = true
	}

	return updated
}

// SaveConfig saves the configuration to ~/.vulpix/config.jsonc
func SaveConfig(cfg *core.Config) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	configPath := filepath.Join(home, ".vulpix", "config.jsonc")
	return saveConfig(configPath, cfg)
}

func saveConfig(path string, cfg *core.Config) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
