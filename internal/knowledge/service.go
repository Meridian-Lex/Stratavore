package knowledge

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"go.uber.org/zap"
)

// Service ties together the embedder, Qdrant client, and Redis cache
// to provide knowledge indexing and querying capabilities.
type Service struct {
	embedder  *Embedder
	qdrant    *QdrantClient
	cache     *KnowledgeCache
	indexer   *Indexer
	knownDir  string
	topK      int
	logger    *zap.Logger
	available bool // false if Qdrant was unreachable at init
}

// Config holds all configuration needed to construct a Service.
type Config struct {
	Enabled      bool
	KnowledgeDir string
	OllamaHost   string
	OllamaModel  string
	QdrantHost   string
	QdrantPort   int
	QdrantCollection string
	RedisHost    string
	RedisPort    int
	CacheTTL     int
	TopK         int
}

// NewService constructs a Service and validates connectivity.
// If Ollama is unreachable, the service is created in degraded mode
// (Query returns empty results, indexer is a no-op) rather than failing.
func NewService(cfg Config, logger *zap.Logger) (*Service, error) {
	embedder := NewEmbedder(cfg.OllamaHost, cfg.OllamaModel)
	qdrantClient := NewQdrantClient(cfg.QdrantHost, cfg.QdrantPort, cfg.QdrantCollection)
	cache := NewKnowledgeCache(cfg.RedisHost, cfg.RedisPort, cfg.CacheTTL)

	topK := cfg.TopK
	if topK <= 0 {
		topK = DefaultTopK
	}

	svc := &Service{
		embedder: embedder,
		qdrant:   qdrantClient,
		cache:    cache,
		knownDir: cfg.KnowledgeDir,
		topK:     topK,
		logger:   logger,
		available: true,
	}

	// Check Redis (warn but continue — queries fall through to Qdrant without cache)
	pingCtx := context.Background()
	if err := cache.Ping(pingCtx); err != nil {
		logger.Warn("knowledge cache (redis) unavailable — queries will not be cached",
			zap.Error(err))
	}

	// Check Qdrant
	if err := qdrantClient.Ping(pingCtx); err != nil {
		logger.Warn("qdrant unavailable — knowledge queries will return empty results",
			zap.Error(err))
		svc.available = false
		return svc, nil // degraded, not fatal
	}

	// Ensure collection exists
	if err := qdrantClient.EnsureCollection(pingCtx); err != nil {
		logger.Warn("failed to ensure qdrant collection",
			zap.String("collection", QdrantCollection),
			zap.Error(err))
		svc.available = false
		return svc, nil
	}

	// Check Ollama — indexer requires it, but Qdrant queries work without it
	if err := embedder.Ping(pingCtx); err != nil {
		logger.Warn("ollama unavailable — knowledge indexing disabled (queries will use existing index)",
			zap.Error(err))
		// Keep available=true: the existing Qdrant index is still queryable.
		// The indexer is not created so no embedding attempts will be made.
		return svc, nil
	}

	svc.indexer = NewIndexer(svc, logger)
	return svc, nil
}

// Start launches the file watcher goroutine and performs an initial full index.
// It blocks until ctx is cancelled.
func (s *Service) Start(ctx context.Context) {
	if s.indexer == nil {
		s.logger.Info("knowledge indexer not started (service in degraded mode)")
		return
	}
	s.logger.Info("knowledge service starting",
		zap.String("knowledge_dir", s.knownDir),
		zap.Bool("qdrant_available", s.available))

	// Initial full index pass
	s.indexer.IndexAll(ctx)

	// Watch for changes
	s.indexer.Watch(ctx, s.knownDir)
}

// Query embeds the query text, checks the cache, searches Qdrant, and
// returns the top-k most relevant knowledge chunks.
// Returns an empty result (not an error) if the service is in degraded mode.
func (s *Service) Query(ctx context.Context, query string, k int) (*QueryResult, error) {
	if k <= 0 {
		k = s.topK
	}

	result := &QueryResult{Query: query, K: k}

	if !s.available {
		return result, nil
	}

	// Cache check
	cached, err := s.cache.Get(ctx, query, k)
	if err != nil {
		s.logger.Warn("cache get failed", zap.Error(err))
	} else if cached != nil {
		result.Chunks = cached
		result.Cached = true
		return result, nil
	}

	// Embed the query
	vector, err := s.embedder.Embed(ctx, query)
	if err != nil {
		s.logger.Warn("failed to embed query — returning empty results",
			zap.Int("query_len", len(query)),
			zap.Error(err))
		return result, nil
	}

	// KNN search
	chunks, err := s.qdrant.Search(ctx, vector, k)
	if err != nil {
		return nil, fmt.Errorf("knowledge search: %w", err)
	}

	result.Chunks = chunks

	// Cache the result (best-effort)
	if err := s.cache.Set(ctx, query, k, chunks); err != nil {
		s.logger.Warn("failed to cache knowledge result", zap.Error(err))
	}

	return result, nil
}

// IndexFile chunks, embeds, and upserts a single file into Qdrant.
// Old chunks for the file are deleted first.
func (s *Service) IndexFile(ctx context.Context, path string) error {
	filename := filepath.Base(path)

	// Only process markdown files
	if !strings.HasSuffix(strings.ToLower(filename), ".md") {
		return nil
	}

	chunks, err := ChunkFile(path)
	if err != nil {
		return fmt.Errorf("chunk %s: %w", filename, err)
	}
	if len(chunks) == 0 {
		return nil
	}

	// Embed all chunks
	vectors := make([][]float32, len(chunks))
	for i, c := range chunks {
		vec, err := s.embedder.Embed(ctx, c.Content)
		if err != nil {
			return fmt.Errorf("embed chunk %d of %s: %w", i, filename, err)
		}
		vectors[i] = vec
	}

	// Delete stale points for this file
	if err := s.qdrant.DeleteBySourceFile(ctx, filename); err != nil {
		s.logger.Warn("failed to delete stale qdrant points",
			zap.String("file", filename), zap.Error(err))
	}
	// Invalidate cache entries for this file
	if err := s.cache.InvalidateFile(ctx); err != nil {
		s.logger.Warn("failed to invalidate query cache after re-index",
			zap.String("file", filename), zap.Error(err))
	}

	// Upsert new points
	if err := s.qdrant.UpsertPoints(ctx, chunks, vectors); err != nil {
		return fmt.Errorf("upsert %s: %w", filename, err)
	}

	s.logger.Info("indexed knowledge file",
		zap.String("file", filename),
		zap.Int("chunks", len(chunks)))
	return nil
}

// KnowledgeDir returns the monitored directory path.
func (s *Service) KnowledgeDir() string {
	return s.knownDir
}
