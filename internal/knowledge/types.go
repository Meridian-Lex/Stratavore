package knowledge

import "time"

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
	// IndexKeyPrefix prefixes per-file Qdrant point ID lists.
	IndexKeyPrefix = "stratavore:knowledge:idx:"
)

// Chunk is a single indexed unit of knowledge — a section of a markdown file.
type Chunk struct {
	ID          string    `json:"id"`
	SourceFile  string    `json:"source_file"`
	Section     string    `json:"section"`
	Content     string    `json:"content"`
	ContentHash string    `json:"content_hash"`
	UpdatedAt   time.Time `json:"updated_at"`
	Score       float32   `json:"score,omitempty"`
}

// QueryResult holds the response to a knowledge query.
type QueryResult struct {
	Chunks []*Chunk `json:"chunks"`
	Query  string   `json:"query"`
	K      int      `json:"k"`
	Cached bool     `json:"cached"`
}
