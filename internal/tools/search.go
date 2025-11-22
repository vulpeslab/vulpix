package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/vulpeslab/vulpix/internal/rag"
)

type SearchTool struct {
	engine *rag.Engine
}

func NewSearchTool(engine *rag.Engine) *SearchTool {
	return &SearchTool{engine: engine}
}

func (t *SearchTool) Name() string {
	return "search_codebase"
}

func (t *SearchTool) Description() string {
	return "Search the codebase for relevant code snippets using semantic search."
}

func (t *SearchTool) Schema() string {
	return `{
		"type": "object",
		"properties": {
			"query": {
				"type": "string",
				"description": "The search query describing what you are looking for."
			}
		},
		"required": ["query"]
	}`
}

func (t *SearchTool) IsDangerous() bool {
	return false
}

func (t *SearchTool) Execute(ctx context.Context, args map[string]any) (string, error) {
	query, ok := args["query"].(string)
	if !ok {
		return "", fmt.Errorf("missing or invalid 'query' argument")
	}

	results, err := t.engine.Search(ctx, query, 5)
	if err != nil {
		return "", fmt.Errorf("search failed: %w", err)
	}

	if len(results) == 0 {
		return "No relevant code found.", nil
	}

	var sb strings.Builder
	sb.WriteString("Found the following relevant code snippets:\n\n")
	for i, res := range results {
		sb.WriteString(fmt.Sprintf("--- Result %d (File: %s) ---\n%s\n\n", i+1, res.FilePath, res.Content))
	}
	return sb.String(), nil
}
