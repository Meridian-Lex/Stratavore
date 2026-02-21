package backends

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

// MessagesAPIBackend implements ModelBackend for remote Messages API.
type MessagesAPIBackend struct {
	apiKey   string
	endpoint string
}

// NewMessagesAPIBackend creates a backend for the Messages API.
// API key is read from LLM_API_KEY env var, or attempts to parse from ~/.config/secrets.yaml.
func NewMessagesAPIBackend() (*MessagesAPIBackend, error) {
	apiKey := os.Getenv("LLM_API_KEY")

	// If not in env var, try to read from secrets.yaml
	if apiKey == "" {
		key, err := readAPIKeyFromSecrets()
		if err == nil {
			apiKey = key
		}
	}

	if apiKey == "" {
		return nil, fmt.Errorf("LLM_API_KEY not set and no API key found in secrets")
	}

	return &MessagesAPIBackend{
		apiKey:   apiKey,
		endpoint: "https://api..com/v1/messages",
	}, nil
}

// readAPIKeyFromSecrets attempts to parse the API key from ~/.config/secrets.yaml
func readAPIKeyFromSecrets() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home dir: %w", err)
	}

	data, err := os.ReadFile(home + "/.config/secrets.yaml")
	if err != nil {
		return "", fmt.Errorf("failed to read secrets.yaml: %w", err)
	}

	content := string(data)
	// Simple string-based parsing for YAML structure:
	// api_keys:
	// :
	//     api_key: sk-ant-...
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "api_key:") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				return strings.TrimSpace(parts[1]), nil
			}
		}
	}

	return "", fmt.Errorf("api_key not found in secrets.yaml")
}

func (b *MessagesAPIBackend) Name() string {
	return "messages-api"
}

// Complete sends a completion request to the Messages API.
func (b *MessagesAPIBackend) Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error) {
	if req.MaxTokens <= 0 {
		req.MaxTokens = 4096
	}
	if req.Temperature < 0 {
		req.Temperature = 0.7
	}

	payload := map[string]interface{}{
		"model":      req.Model,
		"max_tokens": req.MaxTokens,
		"system":     req.SystemPrompt,
		"messages": []map[string]string{
			{
				"role":    "user",
				"content": req.UserPrompt,
			},
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, b.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("x-api-key", b.apiKey)
	httpReq.Header.Set("-version", "2023-06-01")
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
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var apiResp struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
		Usage struct {
			InputTokens  int64 `json:"input_tokens"`
			OutputTokens int64 `json:"output_tokens"`
		} `json:"usage"`
		StopReason string `json:"stop_reason"`
	}

	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if len(apiResp.Content) == 0 {
		return nil, fmt.Errorf("empty content in response")
	}

	return &CompletionResponse{
		Content:      apiResp.Content[0].Text,
		InputTokens:  apiResp.Usage.InputTokens,
		OutputTokens: apiResp.Usage.OutputTokens,
		StopReason:   apiResp.StopReason,
	}, nil
}

// ListModels returns a static list of known models.
func (b *MessagesAPIBackend) ListModels(ctx context.Context) ([]ModelInfo, error) {
	return []ModelInfo{
		{Name: "claude-opus-4.6", DisplayName: "Lex Opus 4.6"},
		{Name: "claude-sonnet-4.5", DisplayName: "Lex Sonnet 4.5"},
		{Name: "claude-haiku-4.5", DisplayName: "Lex Haiku 4.5"},
	}, nil
}

// Ping sends a minimal request to verify the backend is accessible.
func (b *MessagesAPIBackend) Ping(ctx context.Context) error {
	payload := map[string]interface{}{
		"model":      "claude-haiku-4.5",
		"max_tokens": 1,
		"system":     "You are a helpful assistant.",
		"messages": []map[string]string{
			{
				"role":    "user",
				"content": "Hi",
			},
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal ping request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, b.endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create ping request: %w", err)
	}

	httpReq.Header.Set("x-api-key", b.apiKey)
	httpReq.Header.Set("-version", "2023-06-01")
	httpReq.Header.Set("content-type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("ping request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ping returned status %d", resp.StatusCode)
	}

	return nil
}
