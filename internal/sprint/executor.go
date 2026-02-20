package sprint

import (
	"context"
	"fmt"
	"time"

	"github.com/meridian-lex/stratavore/internal/dispatch"
	"github.com/meridian-lex/stratavore/internal/storage"
	"github.com/meridian-lex/stratavore/pkg/types"
	"go.uber.org/zap"
)

// SprintExecutor runs a sprint by executing its tasks in sequence order, respecting dependencies.
type SprintExecutor struct {
	db     *storage.PostgresClient
	router *dispatch.TierRouter
	logger *zap.Logger
}

// NewSprintExecutor creates a new sprint executor.
func NewSprintExecutor(db *storage.PostgresClient, router *dispatch.TierRouter, logger *zap.Logger) *SprintExecutor {
	return &SprintExecutor{
		db:     db,
		router: router,
		logger: logger,
	}
}

// ExecuteSprint runs all tasks in a sprint.
// Returns the execution record or error.
func (e *SprintExecutor) ExecuteSprint(ctx context.Context, sprintID, executedBy string) (*types.SprintExecution, error) {
	startTime := time.Now()

	// Load sprint with tasks
	sprint, err := e.db.GetSprint(ctx, sprintID, true)
	if err != nil {
		return nil, fmt.Errorf("get sprint: %w", err)
	}

	if len(sprint.Tasks) == 0 {
		return nil, fmt.Errorf("sprint has no tasks")
	}

	e.logger.Info("starting sprint execution",
		zap.String("sprint_id", sprintID),
		zap.String("sprint_name", sprint.Name),
		zap.Int("task_count", len(sprint.Tasks)))

	// Create execution record
	exec, err := e.db.CreateSprintExecution(ctx, sprintID, executedBy, len(sprint.Tasks))
	if err != nil {
		return nil, fmt.Errorf("create execution: %w", err)
	}

	// Update sprint status to running
	if err := e.db.UpdateSprintStatus(ctx, sprintID, types.SprintRunning); err != nil {
		e.logger.Error("failed to update sprint status to running", zap.Error(err))
	}

	// Execute tasks in sequence_number order
	// TODO: respect depends_on — for now, just iterate in order
	completed := 0
	failed := 0
	var totalTokensIn, totalTokensOut int64
	var totalCost float64

	for _, task := range sprint.Tasks {
		e.logger.Info("executing task",
			zap.String("task_id", task.ID),
			zap.String("task_name", task.Name),
			zap.Int("sequence", task.SequenceNumber))

		// Mark task as running
		if err := e.db.UpdateTaskStatus(ctx, task.ID, types.TaskRunning); err != nil {
			e.logger.Error("failed to mark task running", zap.Error(err))
		}

		// Route and execute
		resp, cost, err := e.router.RouteTask(ctx, &task)
		if err != nil {
			e.logger.Error("task execution failed",
				zap.String("task_id", task.ID),
				zap.Error(err))

			// Mark task as failed
			resultData := map[string]interface{}{"error": err.Error()}
			e.db.UpdateTaskResult(ctx, task.ID, types.TaskFailed, "", resultData, 0, 0, 0, err.Error())
			failed++
			continue
		}

		// Success — write result
		resultData := map[string]interface{}{
			"content":     resp.Content,
			"stop_reason": resp.StopReason,
		}
		summary := fmt.Sprintf("Completed: %d input tokens, %d output tokens, $%.6f", resp.InputTokens, resp.OutputTokens, cost)

		if err := e.db.UpdateTaskResult(ctx, task.ID, types.TaskCompleted, summary, resultData, resp.InputTokens, resp.OutputTokens, cost, ""); err != nil {
			e.logger.Error("failed to write task result", zap.Error(err))
		}

		totalTokensIn += resp.InputTokens
		totalTokensOut += resp.OutputTokens
		totalCost += cost
		completed++
	}

	// Finalize execution
	durationMs := time.Since(startTime).Milliseconds()
	status := "completed"
	if failed > 0 {
		status = "failed"
	}

	if err := e.db.CompleteSprintExecution(ctx, exec.ID, status, completed, failed, totalTokensIn, totalTokensOut, totalCost, durationMs); err != nil {
		e.logger.Error("failed to finalize execution", zap.Error(err))
	}

	// Update sprint status
	finalStatus := types.SprintCompleted
	if failed > 0 {
		finalStatus = types.SprintFailed
	}
	if err := e.db.UpdateSprintStatus(ctx, sprintID, finalStatus); err != nil {
		e.logger.Error("failed to update sprint final status", zap.Error(err))
	}

	e.logger.Info("sprint execution complete",
		zap.String("sprint_id", sprintID),
		zap.Int("completed", completed),
		zap.Int("failed", failed),
		zap.Float64("total_cost_usd", totalCost),
		zap.Int64("duration_ms", durationMs))

	exec.Status = status
	exec.TasksCompleted = completed
	exec.TasksFailed = failed
	exec.TotalTokensIn = totalTokensIn
	exec.TotalTokensOut = totalTokensOut
	exec.TotalCostUSD = totalCost
	exec.DurationMs = durationMs

	return exec, nil
}
