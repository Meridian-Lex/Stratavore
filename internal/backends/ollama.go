package backends

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// OllamaBackend implements ModelBackend for local Ollama inference.
type OllamaBackend struct {
	endpoint string
}

// NewOllamaBackend creates a backend for Ollama.
// If endpoint is empty, defaults to "http://localhost:11434".
func NewOllamaBackend(endpoint string) *OllamaBackend {
	if endpoint == "" {
		endpoint = "http://localhost:11434"
	}
	return &OllamaBackend{endpoint: endpoint}
}

func (b *OllamaBackend) Name() string {
	return "ollama"
}

// Complete sends a completion request to Ollama.
func (b *OllamaBackend) Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error) {
	if req.MaxTokens <= 0 {
		req.MaxTokens = 2048
	}
	if req.Temperature < 0 {
		req.Temperature = 0.7
	}

	// Combine system and user prompts for Ollama
	prompt := req.SystemPrompt
	if prompt != "" {
		prompt += "\n\n"
	}
	prompt += req.UserPrompt

	payload := map[string]interface{}{
		"model":  req.Model,
		"prompt": prompt,
		"stream": false,
		"options": map[string]interface{}{
			"num_predict": req.MaxTokens,
			"temperature": req.Temperature,
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := b.endpoint + "/api/generate"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("content-type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Ollama returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var apiResp struct {
		Response      string `json:"response"`
		PromptEvalCnt int64  `json:"prompt_eval_count"`
		EvalCount     int64  `json:"eval_count"`
	}

	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &CompletionResponse{
		Content:      apiResp.Response,
		InputTokens:  apiResp.PromptEvalCnt,
		OutputTokens: apiResp.EvalCount,
		StopReason:   "stop",
	}, nil
}

// ListModels queries Ollama for available models.
func (b *OllamaBackend) ListModels(ctx context.Context) ([]ModelInfo, error) {
	url := b.endpoint + "/api/tags"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	client := &http.Client{}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Ollama returned status %d", resp.StatusCode)
	}

	var apiResp struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}

	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	var models []ModelInfo
	for _, m := range apiResp.Models {
		models = append(models, ModelInfo{
			Name:        m.Name,
			DisplayName: strings.Title(strings.ReplaceAll(m.Name, "-", " ")),
		})
	}

	return models, nil
}

// Ping checks if Ollama is accessible.
func (b *OllamaBackend) Ping(ctx context.Context) error {
	url := b.endpoint + "/"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create ping request: %w", err)
	}

	client := &http.Client{}
	resp, err := client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("ping request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Ollama returned status %d on health check", resp.StatusCode)
	}

	return nil
}
