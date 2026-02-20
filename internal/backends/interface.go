package backends

import "context"

// ModelBackend is the ports-and-adapters interface for all LLM backends.
// Implementations: MessagesAPIBackend (remote LLM API), OllamaBackend (local).
type ModelBackend interface {
	Name() string
	Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error)
	ListModels(ctx context.Context) ([]ModelInfo, error)
	Ping(ctx context.Context) error
}

type CompletionRequest struct {
	Model        string
	SystemPrompt string
	UserPrompt   string
	MaxTokens    int
	Temperature  float64
}

type CompletionResponse struct {
	Content      string
	InputTokens  int64
	OutputTokens int64
	StopReason   string
}

type ModelInfo struct {
	Name        string
	DisplayName string
}
