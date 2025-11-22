package rag

import (
	"context"
	"fmt"

	"github.com/philippgille/chromem-go"
	"github.com/vulpeslab/vulpix/internal/logger"
	"github.com/vulpeslab/vulpix/pkg/core"
)

type Engine struct {
	db                *chromem.DB
	collection        *chromem.Collection
	embeddingProvider core.EmbeddingProvider
	indexer           *Indexer
	walker            *Walker
}

func NewEngine(dbPath, rootPath string, embeddingProvider core.EmbeddingProvider) (*Engine, error) {
	logger.Log.Debug("Initializing RAG Engine", "dbPath", dbPath, "rootPath", rootPath)
	// Use persistent DB
	db, err := chromem.NewPersistentDB(dbPath, false)
	if err != nil {
		logger.Log.Error("Failed to create persistent DB", "error", err)
		return nil, err
	}

	// Create collection. We don't pass an embedding function because we'll generate embeddings manually
	// using our provider.
	collection, err := db.GetOrCreateCollection("codebase", nil, nil)
	if err != nil {
		logger.Log.Error("Failed to get or create collection", "error", err)
		return nil, err
	}

	walker, err := NewWalker(rootPath)
	if err != nil {
		logger.Log.Warn("Failed to initialize walker", "error", err)
	}

	e := &Engine{
		db:                db,
		collection:        collection,
		embeddingProvider: embeddingProvider,
		indexer:           NewIndexer(),
		walker:            walker,
	}

	return e, nil
}

func (e *Engine) Close() error {
	// chromem-go doesn't strictly require closing, but we can ensure persistence if needed.
	// The persistent DB saves on every write by default if not configured otherwise?
	// Actually NewPersistentDB takes a 'safe' bool. If false, it might need explicit save or just relies on OS.
	// Looking at docs, it seems it persists on operations.
	return nil
}

func (e *Engine) IndexFile(ctx context.Context, path string) error {
	logger.Log.Debug("Indexing file", "path", path)
	chunks, err := e.indexer.ChunkFile(path)
	if err != nil {
		logger.Log.Error("Failed to chunk file", "path", path, "error", err)
		return err
	}
	logger.Log.Debug("File chunked", "path", path, "chunks", len(chunks))

	// Prepare batch data
	ids := make([]string, 0, len(chunks))
	embeddings := make([][]float32, 0, len(chunks))
	metadatas := make([]map[string]string, 0, len(chunks))
	contents := make([]string, 0, len(chunks))

	for i, chunk := range chunks {
		emb, err := e.embeddingProvider.Embed(ctx, chunk)
		if err != nil {
			logger.Log.Error("Failed to embed chunk", "path", path, "chunk_index", i, "error", err)
			return fmt.Errorf("failed to embed chunk %d of %s: %w", i, path, err)
		}

		id := fmt.Sprintf("%s-%d", path, i)
		ids = append(ids, id)
		embeddings = append(embeddings, emb)
		metadatas = append(metadatas, map[string]string{"path": path})
		contents = append(contents, chunk)
	}

	if len(ids) > 0 {
		// Add batch
		logger.Log.Debug("Adding batch to collection", "count", len(ids))
		err = e.collection.Add(ctx, ids, embeddings, metadatas, contents)
		if err != nil {
			logger.Log.Error("Failed to add batch to collection", "error", err)
			return err
		}
	}
	return nil
}

func (e *Engine) Index(ctx context.Context, rootPath string, progressChan chan<- string) error {
	logger.Log.Info("Starting indexing", "rootPath", rootPath)
	// Use walker if available, otherwise fallback
	if e.walker == nil {
		var err error
		e.walker, err = NewWalker(rootPath)
		if err != nil {
			logger.Log.Error("Failed to create walker", "error", err)
			return err
		}
	}

	return e.walker.Walk(func(path string) error {
		if progressChan != nil {
			progressChan <- fmt.Sprintf("Indexing %s", path)
		}
		return e.IndexFile(ctx, path)
	})
}

type SearchResult struct {
	Content  string
	FilePath string
}

func (e *Engine) Search(ctx context.Context, query string, limit int) ([]SearchResult, error) {
	logger.Log.Debug("Searching", "query", query, "limit", limit)
	embedding, err := e.embeddingProvider.Embed(ctx, query)
	if err != nil {
		logger.Log.Error("Failed to embed query", "error", err)
		return nil, err
	}

	// Query the collection
	// nResults, where, whereDocument
	results, err := e.collection.QueryEmbedding(ctx, embedding, limit, nil, nil)
	if err != nil {
		logger.Log.Error("Failed to query collection", "error", err)
		return nil, err
	}

	logger.Log.Debug("Search results found", "count", len(results))

	var searchResults []SearchResult
	for _, res := range results {
		path := ""
		if p, ok := res.Metadata["path"]; ok {
			path = p
		}
		searchResults = append(searchResults, SearchResult{
			Content:  res.Content,
			FilePath: path,
		})
	}

	return searchResults, nil
}
