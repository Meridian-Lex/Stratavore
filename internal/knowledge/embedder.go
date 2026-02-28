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

// Embedder generates vector embeddings via the Ollama REST API.
type Embedder struct {
	endpoint string
	model    string
	client   *http.Client
}

// NewEmbedder creates an Embedder pointing at ollamaHost using the given model.
func NewEmbedder(ollamaHost, model string) *Embedder {
	if ollamaHost == "" {
		ollamaHost = "http://localhost:11434"
	}
	if model == "" {
		model = "granite-embedding:278m"
	}
	return &Embedder{
		endpoint: ollamaHost,
		model:    model,
		client: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        10,
				MaxIdleConnsPerHost: 5,
				IdleConnTimeout:     90 * time.Second,
			},
		},
	}
}

// Embed returns a 768-dimensional vector for the given text.
// Returns an error if Ollama is unreachable or returns a non-200 status.
func (e *Embedder) Embed(ctx context.Context, text string) ([]float32, error) {
	payload := map[string]string{
		"model":  e.model,
		"prompt": text,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal embed request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		e.endpoint+"/api/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create embed request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama embed request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read embed response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ollama returned %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Embedding []float32 `json:"embedding"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("parse embed response: %w", err)
	}

	if len(result.Embedding) != VectorDimension {
		return nil, fmt.Errorf("unexpected embedding dimension: got %d, want %d", len(result.Embedding), VectorDimension)
	}

	return result.Embedding, nil
}

// Ping checks if the Ollama endpoint is reachable.
func (e *Embedder) Ping(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, e.endpoint+"/", nil)
	if err != nil {
		return err
	}
	resp, err := e.client.Do(req)
	if err != nil {
		return fmt.Errorf("ollama unreachable at %s: %w", e.endpoint, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ollama health check returned %d", resp.StatusCode)
	}
	return nil
}
