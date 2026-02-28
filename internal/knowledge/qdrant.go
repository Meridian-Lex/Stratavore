package knowledge

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// QdrantClient is a minimal HTTP client for the Qdrant v1 REST API.
type QdrantClient struct {
	baseURL    string
	collection string
	client     *http.Client
}

// NewQdrantClient creates a client for the given host:port and collection name.
func NewQdrantClient(host string, port int, collection string) *QdrantClient {
	if collection == "" {
		collection = QdrantCollection
	}
	return &QdrantClient{
		baseURL:    fmt.Sprintf("http://%s:%d", host, port),
		collection: collection,
		client:     &http.Client{Timeout: 15 * time.Second},
	}
}

// EnsureCollection creates the collection if it does not exist.
// Uses cosine distance with VectorDimension (768) dimensions.
func (q *QdrantClient) EnsureCollection(ctx context.Context) error {
	// Check if collection exists.
	url := fmt.Sprintf("%s/collections/%s", q.baseURL, q.collection)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	resp, err := q.client.Do(req)
	if err != nil {
		return fmt.Errorf("qdrant check collection: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		return nil // already exists
	}

	// Create it.
	payload := map[string]interface{}{
		"vectors": map[string]interface{}{
			"size":     VectorDimension,
			"distance": "Cosine",
		},
	}
	body, _ := json.Marshal(payload)
	req2, _ := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(body))
	req2.Header.Set("Content-Type", "application/json")

	resp2, err := q.client.Do(req2)
	if err != nil {
		return fmt.Errorf("qdrant create collection: %w", err)
	}
	defer resp2.Body.Close()

	if resp2.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp2.Body)
		return fmt.Errorf("qdrant create collection returned %d: %s", resp2.StatusCode, string(b))
	}
	return nil
}

// UpsertPoints inserts or updates chunks in the collection.
// Each chunk must have its ID, vector, and payload fields set.
func (q *QdrantClient) UpsertPoints(ctx context.Context, chunks []*Chunk, vectors [][]float32) error {
	if len(chunks) != len(vectors) {
		return fmt.Errorf("chunks/vectors length mismatch: %d vs %d", len(chunks), len(vectors))
	}
	if len(chunks) == 0 {
		return nil
	}

	type point struct {
		ID      string      `json:"id"`
		Vector  []float32   `json:"vector"`
		Payload interface{} `json:"payload"`
	}

	points := make([]point, len(chunks))
	for i, c := range chunks {
		points[i] = point{
			ID:     c.ID,
			Vector: vectors[i],
			Payload: map[string]interface{}{
				"source_file":  c.SourceFile,
				"section":      c.Section,
				"content":      c.Content,
				"content_hash": c.ContentHash,
				"updated_at":   c.UpdatedAt.Format(time.RFC3339),
			},
		}
	}

	payload := map[string]interface{}{"points": points}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal upsert payload: %w", err)
	}

	url := fmt.Sprintf("%s/collections/%s/points", q.baseURL, q.collection)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := q.client.Do(req)
	if err != nil {
		return fmt.Errorf("qdrant upsert: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("qdrant upsert returned %d: %s", resp.StatusCode, string(b))
	}
	return nil
}

// DeleteBySourceFile removes all points whose payload.source_file == filename.
func (q *QdrantClient) DeleteBySourceFile(ctx context.Context, filename string) error {
	payload := map[string]interface{}{
		"filter": map[string]interface{}{
			"must": []map[string]interface{}{
				{
					"key":   "source_file",
					"match": map[string]string{"value": filename},
				},
			},
		},
	}
	body, _ := json.Marshal(payload)

	url := fmt.Sprintf("%s/collections/%s/points/delete", q.baseURL, q.collection)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := q.client.Do(req)
	if err != nil {
		return fmt.Errorf("qdrant delete: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("qdrant delete returned %d: %s", resp.StatusCode, string(b))
	}
	return nil
}

// Search performs a KNN search and returns the top-k matching chunks.
func (q *QdrantClient) Search(ctx context.Context, vector []float32, k int) ([]*Chunk, error) {
	payload := map[string]interface{}{
		"vector":       vector,
		"limit":        k,
		"with_payload": true,
	}
	body, _ := json.Marshal(payload)

	url := fmt.Sprintf("%s/collections/%s/points/search", q.baseURL, q.collection)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := q.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("qdrant search: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read search response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("qdrant search returned %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Result []struct {
			ID      string             `json:"id"`
			Score   float32            `json:"score"`
			Payload map[string]interface{} `json:"payload"`
		} `json:"result"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("parse search response: %w", err)
	}

	chunks := make([]*Chunk, 0, len(result.Result))
	for _, r := range result.Result {
		c := &Chunk{
			ID:    r.ID,
			Score: r.Score,
		}
		if v, ok := r.Payload["source_file"].(string); ok {
			c.SourceFile = v
		}
		if v, ok := r.Payload["section"].(string); ok {
			c.Section = v
		}
		if v, ok := r.Payload["content"].(string); ok {
			c.Content = v
		}
		if v, ok := r.Payload["content_hash"].(string); ok {
			c.ContentHash = v
		}
		if v, ok := r.Payload["updated_at"].(string); ok {
			c.UpdatedAt, _ = time.Parse(time.RFC3339, v)
		}
		chunks = append(chunks, c)
	}

	return chunks, nil
}

// Ping checks if Qdrant is reachable.
func (q *QdrantClient) Ping(ctx context.Context) error {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, q.baseURL+"/", nil)
	resp, err := q.client.Do(req)
	if err != nil {
		return fmt.Errorf("qdrant unreachable at %s: %w", q.baseURL, err)
	}
	defer resp.Body.Close()
	return nil
}
