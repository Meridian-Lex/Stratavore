package knowledge

import "github.com/meridian-lex/stratavore/pkg/api"

const (
	// QdrantCollection is the Qdrant collection name for knowledge chunks.
	QdrantCollection = "stratavore_knowledge"
	// VectorDimension is the embedding dimension for granite-embedding:278m.
	VectorDimension = 768
	// DefaultTopK is the default number of results returned by Query.
	DefaultTopK = 5
	// CacheTTLSeconds is the default Redis cache TTL for query results.
	CacheTTLSeconds = 3600
	// CacheKeyPrefix prefixes all knowledge query cache keys.
	CacheKeyPrefix = "stratavore:knowledge:q:"
)

// Chunk and QueryResult are type aliases for the public API types so
// consumers of pkg/client do not need to import internal/knowledge.
type Chunk = api.KnowledgeChunk
type QueryResult = api.KnowledgeQueryResult
