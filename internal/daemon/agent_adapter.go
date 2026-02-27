package daemon

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/meridian-lex/stratavore/pkg/api"
	"go.uber.org/zap"
)

// AgentAdapter translates WebUI's agent model to daemon's runner model.
// This provides backward compatibility for the WebUI without requiring code changes.
type AgentAdapter struct {
	handler *GRPCServer
	logger  *zap.Logger
}

// NewAgentAdapter creates a new agent adapter.
func NewAgentAdapter(handler *GRPCServer, logger *zap.Logger) *AgentAdapter {
	return &AgentAdapter{
		handler: handler,
		logger:  logger,
	}
}

// AgentEnvelope provides the {status, error, data} format expected by WebUI.
type AgentEnvelope struct {
	Status string      `json:"status"`
	Error  string      `json:"error,omitempty"`
	Data   interface{} `json:"data,omitempty"`
}

// AgentSummary represents the agent-focused response for /api/agents.
type AgentSummary struct {
	Total      int               `json:"total"`
	Idle       int               `json:"idle"`
	Working    int               `json:"working"`
	Spawning   int               `json:"spawning"`
	Completed  int               `json:"completed"`
	Error      int               `json:"error"`
	Paused     int               `json:"paused"`
	Agents     []WebUIAgent      `json:"agents"`
	Summary    map[string]int    `json:"summary"`
	ByPersonality map[string]int `json:"by_personality"`
}

// WebUIAgent represents an agent in WebUI format.
type WebUIAgent struct {
	AgentID      string    `json:"agent_id"`
	Personality  string    `json:"personality"`
	Status       string    `json:"status"`
	Thought      string    `json:"thought,omitempty"`
	TaskID       string    `json:"task_id,omitempty"`
	ProjectName  string    `json:"project_name"`
	TokensUsed   int64     `json:"tokens_used"`
	CPUPercent   float64   `json:"cpu_percent"`
	MemoryMB     int64     `json:"memory_mb"`
	StartedAt    time.Time `json:"started_at"`
	LastActivity time.Time `json:"last_activity"`
}

// HandleSpawnAgent handles POST /api/agents/spawn
func (a *AgentAdapter) HandleSpawnAgent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		a.respondEnvelope(w, http.StatusMethodNotAllowed, "error", "Method not allowed", nil)
		return
	}

	var req struct {
		Personality string `json:"personality"`
		TaskID      string `json:"task_id,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		a.respondEnvelope(w, http.StatusBadRequest, "error", "Invalid request body", nil)
		return
	}

	// Map personality to capabilities
	capabilities := a.personalityToCapabilities(req.Personality)

	// Launch runner
	launchReq := &api.LaunchRunnerRequest{
		ProjectName:      "default",
		ProjectPath:      "/tmp/default",
		Capabilities:     capabilities,
		ConversationMode: "new",
		RuntimeType:      "process",
		Environment:      map[string]string{
			"PERSONALITY": req.Personality,
		},
	}

	resp, err := a.handler.LaunchRunner(r.Context(), launchReq)
	if err != nil {
		a.logger.Error("failed to spawn agent", zap.Error(err))
		a.respondEnvelope(w, http.StatusInternalServerError, "error", err.Error(), nil)
		return
	}

	if resp.Error != "" {
		a.respondEnvelope(w, http.StatusInternalServerError, "error", resp.Error, nil)
		return
	}

	// Convert runner to WebUI agent format
	agent := a.runnerToAgent(resp.Runner)
	a.respondEnvelope(w, http.StatusOK, "success", "", agent)
}

// HandleAssignAgent handles POST /api/agents/{id}/assign
func (a *AgentAdapter) HandleAssignAgent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		a.respondEnvelope(w, http.StatusMethodNotAllowed, "error", "Method not allowed", nil)
		return
	}

	// Extract agent ID from path
	agentID := extractIDFromPath(r.URL.Path, "/api/agents/", "/assign")
	if agentID == "" {
		a.respondEnvelope(w, http.StatusBadRequest, "error", "agent_id required", nil)
		return
	}

	var req struct {
		TaskID string `json:"task_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		a.respondEnvelope(w, http.StatusBadRequest, "error", "Invalid request body", nil)
		return
	}

	// Update runner with task assignment (stored in environment)
	// Note: This is a placeholder - full implementation would update runner metadata
	getReq := &api.GetRunnerRequest{RunnerID: agentID}
	getResp, err := a.handler.GetRunner(r.Context(), getReq)
	if err != nil || getResp.Error != "" {
		a.logger.Error("failed to get runner", zap.String("runner_id", agentID), zap.Error(err))
		a.respondEnvelope(w, http.StatusNotFound, "error", "Runner not found", nil)
		return
	}

	agent := a.runnerToAgent(getResp.Runner)
	agent.TaskID = req.TaskID
	a.respondEnvelope(w, http.StatusOK, "success", "", agent)
}

// HandleUpdateAgentStatus handles POST /api/agents/{id}/status
func (a *AgentAdapter) HandleUpdateAgentStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		a.respondEnvelope(w, http.StatusMethodNotAllowed, "error", "Method not allowed", nil)
		return
	}

	agentID := extractIDFromPath(r.URL.Path, "/api/agents/", "/status")
	if agentID == "" {
		a.respondEnvelope(w, http.StatusBadRequest, "error", "agent_id required", nil)
		return
	}

	var req struct {
		Status  string `json:"status"`
		Thought string `json:"thought,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		a.respondEnvelope(w, http.StatusBadRequest, "error", "Invalid request body", nil)
		return
	}

	// Map WebUI status to runner status
	runnerStatus := a.webuiStatusToRunnerStatus(req.Status)

	// Get current runner
	getReq := &api.GetRunnerRequest{RunnerID: agentID}
	getResp, err := a.handler.GetRunner(r.Context(), getReq)
	if err != nil || getResp.Error != "" {
		a.logger.Error("failed to get runner", zap.String("runner_id", agentID), zap.Error(err))
		a.respondEnvelope(w, http.StatusNotFound, "error", "Runner not found", nil)
		return
	}

	// Update runner status (this is a simplified version)
	// In production, this would call a proper UpdateRunner API
	agent := a.runnerToAgent(getResp.Runner)
	agent.Status = req.Status
	agent.Thought = req.Thought

	a.respondEnvelope(w, http.StatusOK, "success", "", agent)
	a.logger.Info("agent status updated",
		zap.String("agent_id", agentID),
		zap.String("status", req.Status),
		zap.String("runner_status", runnerStatus))
}

// HandleKillAgent handles POST /api/agents/{id}/kill
func (a *AgentAdapter) HandleKillAgent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		a.respondEnvelope(w, http.StatusMethodNotAllowed, "error", "Method not allowed", nil)
		return
	}

	agentID := extractIDFromPath(r.URL.Path, "/api/agents/", "/kill")
	if agentID == "" {
		a.respondEnvelope(w, http.StatusBadRequest, "error", "agent_id required", nil)
		return
	}

	// Stop runner
	stopReq := &api.StopRunnerRequest{
		RunnerID:       agentID,
		Force:          true,
		TimeoutSeconds: 10,
	}

	resp, err := a.handler.StopRunner(r.Context(), stopReq)
	if err != nil {
		a.logger.Error("failed to kill agent", zap.String("agent_id", agentID), zap.Error(err))
		a.respondEnvelope(w, http.StatusInternalServerError, "error", err.Error(), nil)
		return
	}

	if resp.Error != "" {
		a.respondEnvelope(w, http.StatusInternalServerError, "error", resp.Error, nil)
		return
	}

	a.respondEnvelope(w, http.StatusOK, "success", "", map[string]string{
		"agent_id": agentID,
		"status":   "terminated",
	})
}

// HandleListAgents handles GET /api/agents
func (a *AgentAdapter) HandleListAgents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		a.respondEnvelope(w, http.StatusMethodNotAllowed, "error", "Method not allowed", nil)
		return
	}

	// List all runners
	listReq := &api.ListRunnersRequest{}
	listResp, err := a.handler.ListRunners(r.Context(), listReq)
	if err != nil {
		a.logger.Error("failed to list runners", zap.Error(err))
		a.respondEnvelope(w, http.StatusInternalServerError, "error", err.Error(), nil)
		return
	}

	if listResp.Error != "" {
		a.respondEnvelope(w, http.StatusInternalServerError, "error", listResp.Error, nil)
		return
	}

	// Convert runners to agents and build summary
	summary := &AgentSummary{
		Summary:       make(map[string]int),
		ByPersonality: make(map[string]int),
		Agents:        make([]WebUIAgent, 0, len(listResp.Runners)),
	}

	for _, runner := range listResp.Runners {
		agent := a.runnerToAgent(runner)
		summary.Agents = append(summary.Agents, agent)
		summary.Total++

		// Increment status counters
		switch agent.Status {
		case "idle":
			summary.Idle++
		case "working":
			summary.Working++
		case "spawning":
			summary.Spawning++
		case "completed":
			summary.Completed++
		case "error":
			summary.Error++
		case "paused":
			summary.Paused++
		}

		summary.Summary[agent.Status]++
		summary.ByPersonality[agent.Personality]++
	}

	a.respondEnvelope(w, http.StatusOK, "success", "", summary)
}

// HandleGetAgent handles GET /api/agents/{id}
func (a *AgentAdapter) HandleGetAgent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		a.respondEnvelope(w, http.StatusMethodNotAllowed, "error", "Method not allowed", nil)
		return
	}

	agentID := extractIDFromPath(r.URL.Path, "/api/agents/", "")
	if agentID == "" {
		a.respondEnvelope(w, http.StatusBadRequest, "error", "agent_id required", nil)
		return
	}

	getReq := &api.GetRunnerRequest{RunnerID: agentID}
	getResp, err := a.handler.GetRunner(r.Context(), getReq)
	if err != nil || getResp.Error != "" {
		a.logger.Error("failed to get runner", zap.String("runner_id", agentID), zap.Error(err))
		a.respondEnvelope(w, http.StatusNotFound, "error", "Runner not found", nil)
		return
	}

	agent := a.runnerToAgent(getResp.Runner)
	a.respondEnvelope(w, http.StatusOK, "success", "", agent)
}

// Legacy endpoint handlers (accept body parameters instead of path parameters)

// HandleAssignAgentLegacy handles POST /api/assign-agent with body parameters
func (a *AgentAdapter) HandleAssignAgentLegacy(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		a.respondEnvelope(w, http.StatusMethodNotAllowed, "error", "Method not allowed", nil)
		return
	}

	var req struct {
		AgentID string `json:"agent_id"`
		TaskID  string `json:"task_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		a.respondEnvelope(w, http.StatusBadRequest, "error", "Invalid request body", nil)
		return
	}

	if req.AgentID == "" {
		a.respondEnvelope(w, http.StatusBadRequest, "error", "agent_id required", nil)
		return
	}

	// Get current runner
	getReq := &api.GetRunnerRequest{RunnerID: req.AgentID}
	getResp, err := a.handler.GetRunner(r.Context(), getReq)
	if err != nil || getResp.Error != "" {
		a.logger.Error("failed to get runner", zap.String("runner_id", req.AgentID), zap.Error(err))
		a.respondEnvelope(w, http.StatusNotFound, "error", "Runner not found", nil)
		return
	}

	agent := a.runnerToAgent(getResp.Runner)
	agent.TaskID = req.TaskID
	a.respondEnvelope(w, http.StatusOK, "success", "", agent)
}

// HandleUpdateAgentStatusLegacy handles POST /api/agent-status with body parameters
func (a *AgentAdapter) HandleUpdateAgentStatusLegacy(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		a.respondEnvelope(w, http.StatusMethodNotAllowed, "error", "Method not allowed", nil)
		return
	}

	var req struct {
		AgentID string `json:"agent_id"`
		Status  string `json:"status"`
		Thought string `json:"thought,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		a.respondEnvelope(w, http.StatusBadRequest, "error", "Invalid request body", nil)
		return
	}

	if req.AgentID == "" {
		a.respondEnvelope(w, http.StatusBadRequest, "error", "agent_id required", nil)
		return
	}

	// Map WebUI status to runner status
	runnerStatus := a.webuiStatusToRunnerStatus(req.Status)

	// Get current runner
	getReq := &api.GetRunnerRequest{RunnerID: req.AgentID}
	getResp, err := a.handler.GetRunner(r.Context(), getReq)
	if err != nil || getResp.Error != "" {
		a.logger.Error("failed to get runner", zap.String("runner_id", req.AgentID), zap.Error(err))
		a.respondEnvelope(w, http.StatusNotFound, "error", "Runner not found", nil)
		return
	}

	agent := a.runnerToAgent(getResp.Runner)
	agent.Status = req.Status
	agent.Thought = req.Thought

	a.respondEnvelope(w, http.StatusOK, "success", "", agent)
	a.logger.Info("agent status updated",
		zap.String("agent_id", req.AgentID),
		zap.String("status", req.Status),
		zap.String("runner_status", runnerStatus))
}

// HandleKillAgentLegacy handles POST /api/kill-agent with body parameters
func (a *AgentAdapter) HandleKillAgentLegacy(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		a.respondEnvelope(w, http.StatusMethodNotAllowed, "error", "Method not allowed", nil)
		return
	}

	var req struct {
		AgentID string `json:"agent_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		a.respondEnvelope(w, http.StatusBadRequest, "error", "Invalid request body", nil)
		return
	}

	if req.AgentID == "" {
		a.respondEnvelope(w, http.StatusBadRequest, "error", "agent_id required", nil)
		return
	}

	// Stop runner
	stopReq := &api.StopRunnerRequest{
		RunnerID:       req.AgentID,
		Force:          true,
		TimeoutSeconds: 10,
	}

	resp, err := a.handler.StopRunner(r.Context(), stopReq)
	if err != nil {
		a.logger.Error("failed to kill agent", zap.String("agent_id", req.AgentID), zap.Error(err))
		a.respondEnvelope(w, http.StatusInternalServerError, "error", err.Error(), nil)
		return
	}

	if resp.Error != "" {
		a.respondEnvelope(w, http.StatusInternalServerError, "error", resp.Error, nil)
		return
	}

	a.respondEnvelope(w, http.StatusOK, "success", "", map[string]string{
		"agent_id": req.AgentID,
		"status":   "terminated",
	})
}

// HandleCompleteTask handles POST /api/complete-task
func (a *AgentAdapter) HandleCompleteTask(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		a.respondEnvelope(w, http.StatusMethodNotAllowed, "error", "Method not allowed", nil)
		return
	}

	var req struct {
		AgentID string `json:"agent_id"`
		Success bool   `json:"success"`
		Notes   string `json:"notes,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		a.respondEnvelope(w, http.StatusBadRequest, "error", "Invalid request body", nil)
		return
	}

	if req.AgentID == "" {
		a.respondEnvelope(w, http.StatusBadRequest, "error", "agent_id required", nil)
		return
	}

	// Get current runner
	getReq := &api.GetRunnerRequest{RunnerID: req.AgentID}
	getResp, err := a.handler.GetRunner(r.Context(), getReq)
	if err != nil || getResp.Error != "" {
		a.logger.Error("failed to get runner", zap.String("runner_id", req.AgentID), zap.Error(err))
		a.respondEnvelope(w, http.StatusNotFound, "error", "Runner not found", nil)
		return
	}

	// Mark as completed or failed based on success flag
	status := "completed"
	if !req.Success {
		status = "error"
	}

	agent := a.runnerToAgent(getResp.Runner)
	agent.Status = status
	agent.Thought = req.Notes

	a.respondEnvelope(w, http.StatusOK, "success", "", agent)
	a.logger.Info("task completed",
		zap.String("agent_id", req.AgentID),
		zap.Bool("success", req.Success),
		zap.String("notes", req.Notes))
}

// Helper functions

func (a *AgentAdapter) runnerToAgent(runner *api.Runner) WebUIAgent {
	personality := "cadet" // default
	if p, ok := runner.Environment["PERSONALITY"]; ok {
		personality = p
	}

	status := a.runnerStatusToWebUIStatus(runner.Status)

	startedAt, _ := time.Parse(time.RFC3339, runner.StartedAt)
	lastActivity := time.Now()
	if runner.LastHeartbeat != "" {
		lastActivity, _ = time.Parse(time.RFC3339, runner.LastHeartbeat)
	}

	return WebUIAgent{
		AgentID:      runner.ID,
		Personality:  personality,
		Status:       status,
		ProjectName:  runner.ProjectName,
		TokensUsed:   runner.TokensUsed,
		CPUPercent:   runner.CPUPercent,
		MemoryMB:     runner.MemoryMB,
		StartedAt:    startedAt,
		LastActivity: lastActivity,
	}
}

func (a *AgentAdapter) runnerStatusToWebUIStatus(runnerStatus string) string {
	switch runnerStatus {
	case "starting":
		return "spawning"
	case "running":
		return "working"
	case "paused":
		return "paused"
	case "terminated":
		return "completed"
	case "failed":
		return "error"
	default:
		return "idle"
	}
}

func (a *AgentAdapter) webuiStatusToRunnerStatus(webuiStatus string) string {
	switch webuiStatus {
	case "spawning":
		return "starting"
	case "working":
		return "running"
	case "paused":
		return "paused"
	case "completed":
		return "terminated"
	case "error":
		return "failed"
	case "idle":
		return "running"
	default:
		return "running"
	}
}

func (a *AgentAdapter) personalityToCapabilities(personality string) []string {
	switch personality {
	case "cadet":
		return []string{"basic"}
	case "senior":
		return []string{"advanced", "code-review"}
	case "specialist":
		return []string{"specialized"}
	case "researcher":
		return []string{"research", "analysis"}
	case "debugger":
		return []string{"debugging", "troubleshooting"}
	case "optimizer":
		return []string{"optimization", "performance"}
	default:
		return []string{"basic"}
	}
}

func (a *AgentAdapter) respondEnvelope(w http.ResponseWriter, statusCode int, status, errMsg string, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	envelope := AgentEnvelope{
		Status: status,
		Error:  errMsg,
		Data:   data,
	}

	json.NewEncoder(w).Encode(envelope)
}

// extractIDFromPath extracts an ID from a URL path.
// Example: extractIDFromPath("/api/agents/123/status", "/api/agents/", "/status") -> "123"
func extractIDFromPath(path, prefix, suffix string) string {
	if len(path) < len(prefix) {
		return ""
	}

	path = path[len(prefix):]
	if suffix != "" {
		if idx := len(path) - len(suffix); idx >= 0 && path[idx:] == suffix {
			path = path[:idx]
		}
	}

	return path
}
