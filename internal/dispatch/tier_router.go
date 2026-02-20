package dispatch

import (
	"context"
	"fmt"

	"github.com/meridian-lex/stratavore/internal/backends"
	"github.com/meridian-lex/stratavore/internal/storage"
	"github.com/meridian-lex/stratavore/pkg/types"
	"go.uber.org/zap"
)

// TierRouter routes sprint tasks to the correct backend based on model_name.
type TierRouter struct {
	db       *storage.PostgresClient
	backends *backends.BackendRegistry
	logger   *zap.Logger
}

// NewTierRouter creates a new tier router.
func NewTierRouter(db *storage.PostgresClient, backendReg *backends.BackendRegistry, logger *zap.Logger) *TierRouter {
	return &TierRouter{
		db:       db,
		backends: backendReg,
		logger:   logger,
	}
}

// RouteTask executes a sprint task by:
// 1. Looking up the model in model_registry to get its backend
// 2. Getting the backend implementation from the registry
// 3. Calling Complete() with the task's prompts
// 4. Calculating cost from token counts and model pricing
// Returns the completion result.
func (r *TierRouter) RouteTask(ctx context.Context, task *types.SprintTask) (*backends.CompletionResponse, float64, error) {
	// Look up model in registry
	model, err := r.db.GetModel(ctx, task.ModelName)
	if err != nil {
		return nil, 0, fmt.Errorf("get model %s: %w", task.ModelName, err)
	}

	// Get backend
	backend, err := r.backends.Get(model.Backend)
	if err != nil {
		return nil, 0, fmt.Errorf("get backend %s: %w", model.Backend, err)
	}

	r.logger.Info("routing task to backend",
		zap.String("task_id", task.ID),
		zap.String("model", task.ModelName),
		zap.String("backend", backend.Name()))

	// Build completion request
	req := &backends.CompletionRequest{
		Model:        task.ModelName,
		SystemPrompt: task.SystemPrompt,
		UserPrompt:   task.UserPrompt,
		MaxTokens:    task.MaxTokens,
		Temperature:  task.Temperature,
	}

	// Call backend
	resp, err := backend.Complete(ctx, req)
	if err != nil {
		return nil, 0, fmt.Errorf("backend complete: %w", err)
	}

	// Calculate cost: (input_tokens / 1M) * cost_per_million_input + (output_tokens / 1M) * cost_per_million_output
	costUSD := 0.0
	if model.CostPerMillionInput > 0 && resp.InputTokens > 0 {
		costUSD += (float64(resp.InputTokens) / 1_000_000.0) * model.CostPerMillionInput
	}
	if model.CostPerMillionOutput > 0 && resp.OutputTokens > 0 {
		costUSD += (float64(resp.OutputTokens) / 1_000_000.0) * model.CostPerMillionOutput
	}

	return resp, costUSD, nil
}
