package rag

import (
	"os"

	"github.com/vulpeslab/vulpix/internal/logger"
)

type Indexer struct {
	// parsers map[string]*sitter.Parser
}

func NewIndexer() *Indexer {
	return &Indexer{}
}

func (i *Indexer) ChunkFile(path string) ([]string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	logger.Log.Debug("Chunking file", "path", path, "size", len(content))

	// Fallback to simple chunking to ensure stability without CGO
	return chunkText(string(content), 1000, 200), nil
}

func chunkText(text string, chunkSize, overlap int) []string {
	if len(text) <= chunkSize {
		return []string{text}
	}
	var chunks []string
	runes := []rune(text)
	for i := 0; i < len(runes); i += (chunkSize - overlap) {
		end := i + chunkSize
		if end > len(runes) {
			end = len(runes)
		}
		chunks = append(chunks, string(runes[i:end]))
		if end == len(runes) {
			break
		}
	}
	return chunks
}
