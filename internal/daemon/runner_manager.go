package daemon

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
	"time"

	"github.com/meridian-lex/stratavore/internal/messaging"
	"github.com/meridian-lex/stratavore/internal/storage"
	"github.com/meridian-lex/stratavore/pkg/types"
	"go.uber.org/zap"
)

// RunnerManager manages Meridian Lex runner lifecycles
type RunnerManager struct {
	db             *storage.PostgresClient
	messaging      *messaging.Client
	logger         *zap.Logger
	activeRunners  map[string]*ManagedRunner
	mu             sync.RWMutex
	tmuxAvailable  bool
}

// ManagedRunner represents an actively managed runner
type ManagedRunner struct {
	Runner     *types.Runner
	Process    *exec.Cmd
	Heartbeats chan *types.Heartbeat
	StopCh     chan struct{}
}

// NewRunnerManager creates a new runner manager
func NewRunnerManager(
	db *storage.PostgresClient,
	messaging *messaging.Client,
	logger *zap.Logger,
) *RunnerManager {
	_, tmuxErr := exec.LookPath("tmux")
	tmuxAvailable := tmuxErr == nil
	if tmuxAvailable {
		logger.Info("tmux detected: runners will be wrapped in named sessions")
	} else {
		logger.Warn("tmux not found: runners will launch directly (no session persistence)")
	}
	return &RunnerManager{
		db:            db,
		messaging:     messaging,
		logger:        logger,
		activeRunners: make(map[string]*ManagedRunner),
		tmuxAvailable: tmuxAvailable,
	}
}

// Launch starts a new runner
func (rm *RunnerManager) Launch(ctx context.Context, req *types.LaunchRequest) (*types.Runner, error) {
	rm.logger.Info("launching runner",
		zap.String("project", req.ProjectName),
		zap.String("runtime", string(req.RuntimeType)))

	// Get project to validate
	project, err := rm.db.GetProject(ctx, req.ProjectName)
	if err != nil {
		return nil, fmt.Errorf("get project: %w", err)
	}

	// Get quota
	quota, err := rm.db.GetResourceQuota(ctx, req.ProjectName)
	if err != nil {
		return nil, fmt.Errorf("get quota: %w", err)
	}

	// Create runner with transactional outbox (atomic with quota check)
	runner, err := rm.db.CreateRunnerTx(ctx, req, quota.MaxConcurrentRunners)
	if err != nil {
		return nil, fmt.Errorf("create runner: %w", err)
	}

	// Start agent wrapper
	managed, err := rm.startAgent(ctx, runner, req)
	if err != nil {
		// Mark as failed
		rm.db.UpdateRunnerStatus(ctx, runner.ID, types.StatusFailed)
		return nil, fmt.Errorf("start agent: %w", err)
	}

	// Register runner
	rm.mu.Lock()
	rm.activeRunners[runner.ID] = managed
	rm.mu.Unlock()

	// Update project access time
	rm.updateProjectAccess(ctx, project.Name)

	rm.logger.Info("runner launched successfully",
		zap.String("runner_id", runner.ID),
		zap.String("project", req.ProjectName))

	return runner, nil
}

// startAgent spawns the stratavore-agent process
func (rm *RunnerManager) startAgent(
	ctx context.Context,
	runner *types.Runner,
	req *types.LaunchRequest,
) (*ManagedRunner, error) {
	// Build agent command
	args := []string{
		"--runner-id", runner.ID,
		"--project-name", req.ProjectName,
		"--project-path", req.ProjectPath,
	}

	// Add flags
	for _, flag := range req.Flags {
		args = append(args, "--claude-flag", flag)
	}

	// Build command — wrap in tmux session when available so the process
	// survives SSH disconnects and can be re-attached via stratavore attach.
	var cmd *exec.Cmd
	var sessionName string

	if rm.tmuxAvailable {
		sessionName = "stratavore-" + runner.ID[:8]
		// Resolve agent binary path first
		agentPath, err := resolveAgentPath()
		if err != nil {
			return nil, fmt.Errorf("resolve agent path: %w", err)
		}
		// tmux new-session -d -s <name> -- <agentBin> <args...>
		tmuxArgs := append([]string{"new-session", "-d", "-s", sessionName, "--", agentPath}, args...)
		cmd = exec.CommandContext(ctx, "tmux", tmuxArgs...)
	} else {
		var err error
		cmd, err = launchAgent(ctx, args)
		if err != nil {
			return nil, fmt.Errorf("launch agent: %w", err)
		}
	}

	// Set up logging (could redirect to structured log files)
	// cmd.Stdout = ...
	// cmd.Stderr = ...

	// Start the process
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start process: %w", err)
	}

	pid := cmd.Process.Pid

	// If running under tmux, store the session name in the DB
	if rm.tmuxAvailable && sessionName != "" {
		runner.SessionName = sessionName
		if err := rm.db.UpdateRunnerSessionName(ctx, runner.ID, sessionName); err != nil {
			rm.logger.Warn("failed to store session name",
				zap.String("runner_id", runner.ID),
				zap.String("session_name", sessionName),
				zap.Error(err))
		}
	}

	// Update runner with runtime ID (PID)
	if err := rm.db.UpdateRunnerRuntimeID(ctx, runner.ID, fmt.Sprintf("%d", pid)); err != nil {
		cmd.Process.Kill()
		return nil, fmt.Errorf("update runtime id: %w", err)
	}

	runner.RuntimeID = fmt.Sprintf("%d", pid)

	managed := &ManagedRunner{
		Runner:     runner,
		Process:    cmd,
		Heartbeats: make(chan *types.Heartbeat, 10),
		StopCh:     make(chan struct{}),
	}

	// Monitor process lifecycle
	go rm.monitorProcess(runner.ID, cmd)

	return managed, nil
}

// resolveAgentPath returns the path to the stratavore-agent binary
func resolveAgentPath() (string, error) {
	exeName := "stratavore-agent"
	if runtime.GOOS == "windows" {
		exeName += ".exe"
	}

	// First try same directory as this executable
	exePath, err := os.Executable()
	if err == nil {
		exeDir := filepath.Dir(exePath)
		candidate := filepath.Join(exeDir, exeName)
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}

	// Fallback to PATH
	return exeName, nil
}

// launchAgent returns an exec.Cmd pointing to stratavore-agent
func launchAgent(ctx context.Context, args []string) (*exec.Cmd, error) {
	agentPath, err := resolveAgentPath()
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, agentPath, args...)
	return cmd, nil
}

// monitorProcess watches the agent process and updates status on exit
func (rm *RunnerManager) monitorProcess(runnerID string, cmd *exec.Cmd) {
	err := cmd.Wait()

	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = -1
		}
	}

	ctx := context.Background()

	// Update database
	rm.db.TerminateRunner(ctx, runnerID, exitCode)

	// Remove from active runners
	rm.mu.Lock()
	delete(rm.activeRunners, runnerID)
	rm.mu.Unlock()

	// Publish termination event
	event := map[string]interface{}{
		"runner_id": runnerID,
		"exit_code": exitCode,
		"timestamp": time.Now().Format(time.RFC3339),
	}

	rm.messaging.Publish(ctx, fmt.Sprintf("runner.stopped.%s", runnerID), event)

	rm.logger.Info("runner process exited",
		zap.String("runner_id", runnerID),
		zap.Int("exit_code", exitCode))
}

// ProcessHeartbeat handles a heartbeat from an agent
func (rm *RunnerManager) ProcessHeartbeat(ctx context.Context, hb *types.Heartbeat) error {
	rm.mu.RLock()
	managed, exists := rm.activeRunners[hb.RunnerID]
	rm.mu.RUnlock()

	if !exists {
		return fmt.Errorf("runner not found: %s", hb.RunnerID)
	}

	// Update database
	if err := rm.db.UpdateRunnerHeartbeat(ctx, hb); err != nil {
		return fmt.Errorf("update heartbeat: %w", err)
	}

	// Forward to channel for monitoring
	select {
	case managed.Heartbeats <- hb:
	default:
		// Channel full, skip
	}

	return nil
}

// StopRunner gracefully stops a runner
func (rm *RunnerManager) StopRunner(ctx context.Context, runnerID string) error {
	rm.mu.RLock()
	managed, exists := rm.activeRunners[runnerID]
	rm.mu.RUnlock()

	if !exists {
		return fmt.Errorf("runner not active: %s", runnerID)
	}

	rm.logger.Info("stopping runner", zap.String("runner_id", runnerID))

	// Kill tmux session if present — this terminates the runner cleanly
	if managed.Runner.SessionName != "" {
		if err := exec.Command("tmux", "kill-session", "-t", managed.Runner.SessionName).Run(); err != nil {
			rm.logger.Warn("failed to kill tmux session",
				zap.String("session_name", managed.Runner.SessionName),
				zap.Error(err))
		}
	}

	// Signal stop
	close(managed.StopCh)

	// Send SIGTERM to process
	if managed.Process != nil && managed.Process.Process != nil {

		managed.Process.Process.Signal(syscall.SIGTERM)

		// Wait for graceful shutdown with timeout
		done := make(chan struct{})
		go func() {
			managed.Process.Wait()
			close(done)
		}()

		select {
		case <-done:
			// Process exited gracefully
		case <-time.After(10 * time.Second):
			// Force kill
			rm.logger.Warn("runner did not exit gracefully, killing",
				zap.String("runner_id", runnerID))
			managed.Process.Process.Kill()
		}
	}

	return nil
}

// GetActiveRunners returns all active runners
func (rm *RunnerManager) GetActiveRunners() []*types.Runner {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	runners := make([]*types.Runner, 0, len(rm.activeRunners))
	for _, managed := range rm.activeRunners {
		runners = append(runners, managed.Runner)
	}

	return runners
}

// ReconcileRunners checks for stale runners and marks them as failed
func (rm *RunnerManager) ReconcileRunners(ctx context.Context) error {
	failedIDs, err := rm.db.ReconcileStaleRunners(ctx, 30)
	if err != nil {
		return fmt.Errorf("reconcile stale runners: %w", err)
	}

	if len(failedIDs) > 0 {
		rm.logger.Warn("marked stale runners as failed",
			zap.Int("count", len(failedIDs)),
			zap.Strings("runner_ids", failedIDs))

		// Publish failed events
		for _, id := range failedIDs {
			event := map[string]interface{}{
				"runner_id": id,
				"reason":    "heartbeat_timeout",
				"timestamp": time.Now().Format(time.RFC3339),
			}
			rm.messaging.Publish(ctx, fmt.Sprintf("runner.failed.%s", id), event)
		}
	}

	return nil
}

// updateProjectAccess updates the last accessed timestamp
func (rm *RunnerManager) updateProjectAccess(ctx context.Context, projectName string) {
	// This would be a simple UPDATE query
	// Omitted for brevity - add to storage layer
}

// Shutdown gracefully stops all runners
func (rm *RunnerManager) Shutdown(ctx context.Context) error {
	rm.logger.Info("shutting down runner manager")

	rm.mu.RLock()
	runnerIDs := make([]string, 0, len(rm.activeRunners))
	for id := range rm.activeRunners {
		runnerIDs = append(runnerIDs, id)
	}
	rm.mu.RUnlock()

	// Stop all runners
	for _, id := range runnerIDs {
		if err := rm.StopRunner(ctx, id); err != nil {
			rm.logger.Error("error stopping runner during shutdown",
				zap.String("runner_id", id),
				zap.Error(err))
		}
	}

	return nil
}
