package rag

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/vulpeslab/vulpix/pkg/core"
)

type MockProvider struct {
	EmbedFunc func(ctx context.Context, text string) ([]float32, error)
}

func (m *MockProvider) StreamCompletion(ctx context.Context, messages []core.Message, tools []core.Tool) (<-chan core.StreamEvent, error) {
	return nil, nil
}

func (m *MockProvider) Embed(ctx context.Context, text string) ([]float32, error) {
	if m.EmbedFunc != nil {
		return m.EmbedFunc(ctx, text)
	}
	return make([]float32, 1536), nil
}

func (m *MockProvider) GetModels(ctx context.Context) ([]string, error) {
	return []string{"mock-model"}, nil
}

func TestEngine_IndexAndSearch(t *testing.T) {
	// t.Skip("Skipping RAG test due to Wasm/sqlite-vec environment issues: i32.atomic.store invalid")
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	// Create a dummy file to index
	dummyFile := filepath.Join(tmpDir, "dummy.txt")
	if err := os.WriteFile(dummyFile, []byte("Hello world content"), 0644); err != nil {
		t.Fatalf("Failed to write dummy file: %v", err)
	}

	provider := &MockProvider{}
	engine, err := NewEngine(dbPath, tmpDir, provider)
	if err != nil {
		t.Fatalf("Failed to create engine: %v", err)
	}
	defer engine.Close()

	// Index the file
	if err := engine.IndexFile(context.Background(), dummyFile); err != nil {
		t.Fatalf("IndexFile failed: %v", err)
	}

	// Search
	results, err := engine.Search(context.Background(), "Hello", 1)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if len(results) == 0 {
		t.Error("Expected results, got 0")
	} else {
		if results[0].FilePath != dummyFile {
			t.Errorf("Expected file path %s, got %s", dummyFile, results[0].FilePath)
		}
	}
}
