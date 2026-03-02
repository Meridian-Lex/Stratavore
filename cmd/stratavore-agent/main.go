package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"github.com/meridian-lex/stratavore/internal/procmetrics"
	"go.uber.org/zap"
)

var (
	Version     = "1.6.4"
	BuildTime   = "unknown"
	Commit      = "unknown"

	runnerID    string
	projectName string
	projectPath string
	apiURL      string
	claudeFlags []string
)

func main() {
	// Parse flags
	flag.StringVar(&runnerID, "runner-id", "", "Runner ID")
	flag.StringVar(&projectName, "project-name", "", "Project name")
	flag.StringVar(&projectPath, "project-path", "", "Project path")
	flag.StringVar(&apiURL, "api-url", "http://localhost:8080", "Stratavore daemon HTTP API base URL")
	flag.Parse()
	
	if runnerID == "" || projectName == "" || projectPath == "" {
		fmt.Fprintf(os.Stderr, "Missing required flags\n")
		os.Exit(1)
	}
	
	// Setup logger
	logger, _ := zap.NewProduction()
	defer logger.Sync()
	
	logger.Info("stratavore-agent starting",
		zap.String("version", Version),
		zap.String("runner_id", runnerID),
		zap.String("project_name", projectName),
		zap.String("project_path", projectPath))
	
	// Create context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	
	// Start heartbeat goroutine
	go sendHeartbeats(ctx, runnerID, apiURL, logger)
	
	// Build Meridian Lex command
	args := []string{"--project", projectPath}
	
	// Add custom flags
	for _, f := range claudeFlags {
		args = append(args, f)
	}
	
	// Start Meridian Lex
 // IDENTITY-EXCEPTION: functional internal reference — not for public exposure
	cmd := exec.CommandContext(ctx, "claude", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Dir = projectPath
	
	logger.Info("starting claude code", zap.Strings("args", args))
	
	if err := cmd.Start(); err != nil {
		logger.Error("failed to start claude code", zap.Error(err))
		os.Exit(1)
	}
	
	pid := cmd.Process.Pid
	logger.Info("claude code started", zap.Int("pid", pid))
	
	// Handle signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	
	// Wait for process or signal
	errCh := make(chan error, 1)
	go func() {
		errCh <- cmd.Wait()
	}()
	
	select {
	case err := <-errCh:
		exitCode := 0
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				exitCode = exitErr.ExitCode()
			} else {
				exitCode = -1
			}
		}
		logger.Info("claude code exited",
			zap.Int("exit_code", exitCode))
		os.Exit(exitCode)
		
	case sig := <-sigCh:
		logger.Info("received signal, terminating",
			zap.String("signal", sig.String()))
		
		// Forward signal to Meridian Lex
		cmd.Process.Signal(sig)
		
		// Wait with timeout
		select {
		case <-errCh:
			logger.Info("claude code terminated gracefully")
		case <-time.After(10 * time.Second):
			logger.Warn("claude code did not exit, killing")
			cmd.Process.Kill()
		}
	}
}

func sendHeartbeats(ctx context.Context, runnerID string, baseURL string, logger *zap.Logger) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	client := &http.Client{Timeout: 5 * time.Second}
	heartbeatURL := baseURL + "/api/v1/heartbeat"
	hostname, _ := os.Hostname()

	// pid is not known yet at startup; we'll discover it lazily.
	// The process sampler is initialised once we know the PID.
	var sampler *procmetrics.Sampler

	for {
		select {
		case <-ticker.C:
			// Collect CPU / memory for the current process (the agent itself).
			// If the agent is wrapping a claude subprocess, callers can pass the
			// child PID via the --pid flag in a future enhancement; for now we
			// report the agent's own resource usage which is a reasonable proxy.
			cpuPercent := 0.0
			var memoryMB int64

			if sampler == nil {
				sampler = procmetrics.NewSampler(os.Getpid())
			}
			if s, err := sampler.Sample(); err == nil {
				cpuPercent = s.CPUPercent
				memoryMB = s.MemoryMB
			} else {
				logger.Debug("procmetrics sample failed", zap.Error(err))
			}

			// Create heartbeat request
			hb := map[string]interface{}{
				"runner_id":     runnerID,
				"status":        "running",
				"cpu_percent":   cpuPercent,
				"memory_mb":     memoryMB,
				"tokens_used":   0,
				"session_id":    "",
				"agent_version": Version,
				"hostname":      hostname,
			}

			data, err := json.Marshal(hb)
			if err != nil {
				logger.Error("failed to marshal heartbeat", zap.Error(err))
				continue
			}

			resp, err := client.Post(heartbeatURL, "application/json", bytes.NewReader(data))
			if err != nil {
				logger.Debug("heartbeat failed (daemon may be restarting)", zap.Error(err))
				continue
			}
			resp.Body.Close()

			logger.Debug("heartbeat sent",
				zap.String("runner_id", runnerID),
				zap.Float64("cpu_pct", cpuPercent),
				zap.Int64("mem_mb", memoryMB))

		case <-ctx.Done():
			// Send final heartbeat
			finalHB := map[string]interface{}{
				"runner_id":     runnerID,
				"status":        "stopped",
				"agent_version": Version,
				"hostname":      hostname,
			}
			data, _ := json.Marshal(finalHB)
			client.Post(heartbeatURL, "application/json", bytes.NewReader(data))
			return
		}
	}
}
