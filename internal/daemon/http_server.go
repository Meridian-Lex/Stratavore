package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/meridian-lex/stratavore/internal/auth"
	"github.com/meridian-lex/stratavore/internal/backends"
	"github.com/meridian-lex/stratavore/internal/dispatch"
	"github.com/meridian-lex/stratavore/internal/sprint"
	"github.com/meridian-lex/stratavore/pkg/api"
	"github.com/meridian-lex/stratavore/pkg/config"
	"github.com/meridian-lex/stratavore/pkg/types"
	"go.uber.org/zap"
)

// HTTPServer provides REST API for CLI communication
type HTTPServer struct {
	server    *http.Server
	handler   *GRPCServer // Reuse gRPC handler logic
	logger    *zap.Logger
	fleet     *FleetHandler
	startedAt time.Time
}

// NewHTTPServer creates HTTP API server.
// It wires JWT auth and per-client rate limiting when the corresponding
// config values are set; both default to disabled/permissive.
func NewHTTPServer(port int, handler *GRPCServer, logger *zap.Logger, cfg *config.SecurityConfig, fleet *FleetHandler) *HTTPServer {
	mux := http.NewServeMux()

	httpServer := &HTTPServer{
		handler:   handler,
		logger:    logger,
		fleet:     fleet,
		startedAt: time.Now(),
	}

	// Register routes
	mux.HandleFunc("/api/v1/runners/launch", httpServer.handleLaunchRunner)
	mux.HandleFunc("/api/v1/runners/stop", httpServer.handleStopRunner)
	mux.HandleFunc("/api/v1/runners/list", httpServer.handleListRunners)
	mux.HandleFunc("/api/v1/runners/get", httpServer.handleGetRunner)
	mux.HandleFunc("/api/v1/projects/create", httpServer.handleCreateProject)
	mux.HandleFunc("/api/v1/projects/list", httpServer.handleListProjects)
	mux.HandleFunc("/api/v1/projects/get", httpServer.handleGetProject)
	mux.HandleFunc("/api/v1/projects/delete", httpServer.handleDeleteProject)
	mux.HandleFunc("/api/v1/sessions/list", httpServer.handleListSessions)
	mux.HandleFunc("/api/v1/sessions/get", httpServer.handleGetSession)
	mux.HandleFunc("/api/v1/metrics", httpServer.handleMetrics)
	mux.HandleFunc("/api/v1/heartbeat", httpServer.handleHeartbeat)
	mux.HandleFunc("/api/v1/status", httpServer.handleStatus)
	mux.HandleFunc("/api/v1/reconcile", httpServer.handleReconcile)
	mux.HandleFunc("/api/v1/health", httpServer.handleHealth)
	mux.HandleFunc("/api/v1/fleet/prs", httpServer.handleFleetPRs)
	mux.HandleFunc("/api/v1/mode/get", httpServer.handleGetMode)
	mux.HandleFunc("/api/v1/mode/set", httpServer.handleSetMode)
	mux.HandleFunc("/api/v1/config", httpServer.handleGetConfig)
	mux.HandleFunc("/api/v1/tokens", httpServer.handleTokens)
	mux.HandleFunc("/api/v1/state", httpServer.handleGetState)

	// Sprint management
	mux.HandleFunc("/api/v1/sprints/create", httpServer.handleCreateSprint)
	mux.HandleFunc("/api/v1/sprints/list", httpServer.handleListSprints)
	mux.HandleFunc("/api/v1/sprints/get", httpServer.handleGetSprint)
	mux.HandleFunc("/api/v1/sprints/task/add", httpServer.handleAddSprintTask)
	mux.HandleFunc("/api/v1/sprints/status", httpServer.handleUpdateSprintStatus)
	mux.HandleFunc("/api/v1/sprints/execute", httpServer.handleExecuteSprint)
	mux.HandleFunc("/api/v1/tasks/result", httpServer.handleUpdateTaskResult)
	// Model registry
	mux.HandleFunc("/api/v1/models", httpServer.handleListModels)
	mux.HandleFunc("/api/v1/models/", httpServer.handleUpdateModel)

	// Build middleware chain: rate-limit → JWT auth → mux
	var handler_ http.Handler = mux

	// JWT auth (disabled when auth_secret is empty)
	if cfg != nil {
		validator := auth.NewValidator(cfg.AuthSecret)
		if validator.Enabled() {
			logger.Info("HTTP API auth enabled")
		} else {
			logger.Info("HTTP API auth disabled (no auth_secret configured)")
		}
		handler_ = auth.Middleware(validator)(handler_)

		// Rate limiting (always active; defaults to 300 req/min, burst 50)
		ratePerMin := cfg.RateLimit.RequestsPerMinute
		if ratePerMin <= 0 {
			ratePerMin = 300
		}
		burst := cfg.RateLimit.Burst
		if burst <= 0 {
			burst = 50
		}
		rl := auth.NewRateLimiter(ratePerMin, time.Minute, burst)
		handler_ = auth.RateLimitMiddleware(rl)(handler_)

		logger.Info("HTTP API rate limiting enabled",
			zap.Int("requests_per_minute", ratePerMin),
			zap.Int("burst", burst))
	}

	httpServer.server = &http.Server{
		Addr:         fmt.Sprintf(":%d", port),
		Handler:      handler_,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	return httpServer
}

// Start begins serving HTTP requests
func (s *HTTPServer) Start() error {
	s.logger.Info("HTTP API server starting", zap.String("addr", s.server.Addr))

	if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("http server error: %w", err)
	}
	return nil
}

// Stop gracefully stops the server
func (s *HTTPServer) Stop(ctx context.Context) error {
	s.logger.Info("stopping HTTP API server")
	return s.server.Shutdown(ctx)
}

func (s *HTTPServer) handleLaunchRunner(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req api.LaunchRunnerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	resp, err := s.handler.LaunchRunner(r.Context(), &req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.respondJSON(w, resp)
}

func (s *HTTPServer) handleStopRunner(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req api.StopRunnerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	resp, err := s.handler.StopRunner(r.Context(), &req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.respondJSON(w, resp)
}

func (s *HTTPServer) handleListRunners(w http.ResponseWriter, r *http.Request) {
	projectName := r.URL.Query().Get("project")

	req := &api.ListRunnersRequest{
		ProjectName: projectName,
	}

	resp, err := s.handler.ListRunners(r.Context(), req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.respondJSON(w, resp)
}

func (s *HTTPServer) handleGetRunner(w http.ResponseWriter, r *http.Request) {
	runnerID := r.URL.Query().Get("id")
	if runnerID == "" {
		http.Error(w, "runner_id required", http.StatusBadRequest)
		return
	}

	req := &api.GetRunnerRequest{RunnerID: runnerID}
	resp, err := s.handler.GetRunner(r.Context(), req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.respondJSON(w, resp)
}

func (s *HTTPServer) handleCreateProject(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req api.CreateProjectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	resp, err := s.handler.CreateProject(r.Context(), &req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.respondJSON(w, resp)
}

func (s *HTTPServer) handleListProjects(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")

	req := &api.ListProjectsRequest{Status: status}
	resp, err := s.handler.ListProjects(r.Context(), req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.respondJSON(w, resp)
}

func (s *HTTPServer) handleHeartbeat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req api.HeartbeatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	resp, err := s.handler.SendHeartbeat(r.Context(), &req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.respondJSON(w, resp)
}

func (s *HTTPServer) handleStatus(w http.ResponseWriter, r *http.Request) {
	req := &api.GetStatusRequest{}
	resp, err := s.handler.GetStatus(r.Context(), req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.respondJSON(w, resp)
}

func (s *HTTPServer) handleReconcile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	req := &api.TriggerReconciliationRequest{}
	resp, err := s.handler.TriggerReconciliation(r.Context(), req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.respondJSON(w, resp)
}

func (s *HTTPServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

func (s *HTTPServer) handleGetProject(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if name == "" {
		http.Error(w, "name required", http.StatusBadRequest)
		return
	}

	req := &api.GetProjectRequest{Name: name}
	resp, err := s.handler.GetProject(r.Context(), req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.respondJSON(w, resp)
}

func (s *HTTPServer) handleDeleteProject(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req api.DeleteProjectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.Name == "" {
		http.Error(w, "name required", http.StatusBadRequest)
		return
	}

	if err := s.handler.storage.ArchiveProject(r.Context(), req.Name); err != nil {
		s.respondJSON(w, &api.DeleteProjectResponse{Success: false, Error: err.Error()})
		return
	}

	s.respondJSON(w, &api.DeleteProjectResponse{Success: true})
}

func (s *HTTPServer) handleListSessions(w http.ResponseWriter, r *http.Request) {
	project := r.URL.Query().Get("project")
	if project == "" {
		http.Error(w, "project required", http.StatusBadRequest)
		return
	}

	sessions, err := s.handler.storage.GetResumableSessions(r.Context(), project)
	if err != nil {
		s.respondJSON(w, &api.ListSessionsResponse{Error: err.Error()})
		return
	}

	s.respondJSON(w, &api.ListSessionsResponse{Sessions: sessions})
}

func (s *HTTPServer) handleGetSession(w http.ResponseWriter, r *http.Request) {
	sessionID := r.URL.Query().Get("session_id")
	if sessionID == "" {
		http.Error(w, "session_id required", http.StatusBadRequest)
		return
	}

	session, err := s.handler.storage.GetSession(r.Context(), sessionID)
	if err != nil {
		s.respondJSON(w, &api.GetSessionResponse{Error: err.Error()})
		return
	}

	s.respondJSON(w, &api.GetSessionResponse{Session: session})
}

func (s *HTTPServer) handleMetrics(w http.ResponseWriter, r *http.Request) {
	resp, err := s.handler.GetStatus(r.Context(), &api.GetStatusRequest{})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.respondJSON(w, resp.Metrics)
}

func (s *HTTPServer) handleFleetPRs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.fleet == nil {
		http.Error(w, "fleet handler not configured (github.token missing?)", http.StatusServiceUnavailable)
		return
	}

	refresh := r.URL.Query().Get("refresh") == "true"
	prs, cachedAt, err := s.fleet.GetPRs(r.Context(), refresh)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.respondJSON(w, &api.FleetPRsResponse{
		PRs:      prs,
		CachedAt: cachedAt,
		Total:    len(prs),
	})
}

func (s *HTTPServer) respondJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

// ===== Phase 2 Command Handlers =====

func (s *HTTPServer) handleGetMode(w http.ResponseWriter, r *http.Request) {
	mode, description, err := s.handler.storage.GetOperationalMode(r.Context())
	if err != nil {
		s.respondJSON(w, &api.GetModeResponse{Error: err.Error()})
		return
	}

	s.respondJSON(w, &api.GetModeResponse{
		Mode:        mode,
		Description: description,
	})
}

func (s *HTTPServer) handleSetMode(w http.ResponseWriter, r *http.Request) {
	var req api.SetModeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondJSON(w, &api.SetModeResponse{Error: "invalid request body"})
		return
	}

	// Validate mode
	validModes := map[string]bool{
		"IDLE":           true,
		"AUTONOMOUS":     true,
		"DIRECTED":       true,
		"COLLABORATIVE":  true,
	}

	if !validModes[req.Mode] {
		s.respondJSON(w, &api.SetModeResponse{
			Success: false,
			Error:   "invalid mode: must be one of IDLE, AUTONOMOUS, DIRECTED, COLLABORATIVE",
		})
		return
	}

	if err := s.handler.storage.SetOperationalMode(r.Context(), req.Mode, req.Description); err != nil {
		s.respondJSON(w, &api.SetModeResponse{Error: err.Error()})
		return
	}

	s.respondJSON(w, &api.SetModeResponse{
		Success: true,
		Mode:    req.Mode,
	})
}

func (s *HTTPServer) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	cfg, err := config.LoadConfig()
	if err != nil {
		s.respondJSON(w, &api.GetConfigResponse{Error: err.Error()})
		return
	}

	resp := &api.GetConfigResponse{
		Database: api.DatabaseDisplayConfig{
			Host:     cfg.Database.PostgreSQL.Host,
			Port:     cfg.Database.PostgreSQL.Port,
			Database: cfg.Database.PostgreSQL.Database,
		},
		Daemon: api.DaemonDisplayConfig{
			HTTPPort: cfg.Daemon.Port_HTTP,
			GRPCPort: cfg.Daemon.Port_GRPC,
		},
		Observability: api.ObservabilityDisplayConfig{
			LogLevel: cfg.Observability.LogLevel,
		},
	}

	s.respondJSON(w, resp)
}

func (s *HTTPServer) handleTokens(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	req := &api.GetTokensRequest{}
	resp, err := s.handler.GetTokens(r.Context(), req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.respondJSON(w, resp)
}

func (s *HTTPServer) handleGetState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx := r.Context()

	// Get operational mode
	mode, _, err := s.handler.storage.GetOperationalMode(ctx)
	if err != nil {
		s.logger.Error("failed to get operational mode", zap.Error(err))
		mode = "UNKNOWN"
	}

	// Get active runners count
	activeRunners := len(s.handler.runnerManager.GetActiveRunners())

	// Get total projects count
	projects, err := s.handler.storage.ListProjects(ctx, "")
	totalProjects := int32(0)
	if err != nil {
		s.logger.Error("failed to list projects", zap.Error(err))
	} else {
		totalProjects = int32(len(projects))
	}

	// Get total sessions count and tokens used from database
	totalSessions := int32(0)
	tokensUsed := int64(0)

	// Query sessions and accumulate counts
	if projects != nil {
		for _, proj := range projects {
			sessions, err := s.handler.storage.GetResumableSessions(ctx, proj.Name)
			if err != nil {
				s.logger.Warn("failed to get sessions for project", zap.String("project", proj.Name), zap.Error(err))
				continue
			}
			totalSessions += int32(len(sessions))
			for _, sess := range sessions {
				tokensUsed += sess.TokensUsed
			}
		}
	}

	// Calculate uptime
	uptime := formatUptimeDuration(time.Since(s.startedAt))

	resp := &api.GetStateResponse{
		OperationalMode: mode,
		DaemonStatus:    "Running",
		Uptime:          uptime,
		ActiveRunners:   int32(activeRunners),
		TotalProjects:   totalProjects,
		TotalSessions:   totalSessions,
		TokensUsed:      tokensUsed,
	}

	s.respondJSON(w, resp)
}

// formatUptimeDuration formats a duration as a human-readable uptime string
func formatUptimeDuration(d time.Duration) string {
	d = d.Round(time.Second)
	h := d / time.Hour
	d -= h * time.Hour
	m := d / time.Minute
	d -= m * time.Minute
	s := d / time.Second

	if h > 0 {
		return fmt.Sprintf("%dh %dm", h, m)
	} else if m > 0 {
		return fmt.Sprintf("%dm %ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}

// ===== Sprint Management Handlers =====

func (s *HTTPServer) handleCreateSprint(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req api.CreateSprintRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.Name == "" {
		http.Error(w, "name required", http.StatusBadRequest)
		return
	}

	// Map API request to types.Sprint
	sprint := &types.Sprint{
		ID:          uuid.New().String(),
		Name:        req.Name,
		Description: req.Description,
		ProjectName: req.ProjectName,
		CreatedBy:   req.CreatedBy,
		Tags:        req.Tags,
		Status:      types.SprintPending,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	if err := s.handler.storage.CreateSprint(r.Context(), sprint); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.respondJSON(w, &api.CreateSprintResponse{SprintID: sprint.ID})
}

func (s *HTTPServer) handleListSprints(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	projectName := r.URL.Query().Get("project_name")
	status := r.URL.Query().Get("status")

	sprints, err := s.handler.storage.ListSprints(r.Context(), projectName, status)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.respondJSON(w, sprints)
}

func (s *HTTPServer) handleGetSprint(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	sprintID := r.URL.Query().Get("sprint_id")
	if sprintID == "" {
		http.Error(w, "sprint_id required", http.StatusBadRequest)
		return
	}

	includeTasks := r.URL.Query().Get("include_tasks") == "true"

	sprint, err := s.handler.storage.GetSprint(r.Context(), sprintID, includeTasks)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.respondJSON(w, sprint)
}

func (s *HTTPServer) handleAddSprintTask(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req api.AddSprintTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.SprintID == "" {
		http.Error(w, "sprint_id required", http.StatusBadRequest)
		return
	}

	if req.ModelName == "" {
		http.Error(w, "model_name required", http.StatusBadRequest)
		return
	}

	if req.UserPrompt == "" {
		http.Error(w, "user_prompt required", http.StatusBadRequest)
		return
	}

	// Map API request to types.SprintTask
	task := &types.SprintTask{
		ID:             uuid.New().String(),
		SprintID:       req.SprintID,
		SequenceNumber: req.SequenceNumber,
		DependsOn:      req.DependsOn,
		Name:           req.Name,
		Description:    req.Description,
		ModelName:      req.ModelName,
		SystemPrompt:   req.SystemPrompt,
		UserPrompt:     req.UserPrompt,
		MaxTokens:      req.MaxTokens,
		Temperature:    req.Temperature,
		Status:         types.TaskPending,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	if task.MaxTokens <= 0 {
		task.MaxTokens = 2048
	}
	if task.Temperature < 0 {
		task.Temperature = 0.7
	}

	if err := s.handler.storage.AddSprintTask(r.Context(), task); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.respondJSON(w, map[string]string{"task_id": task.ID})
}

func (s *HTTPServer) handleUpdateSprintStatus(w http.ResponseWriter, r *http.Request) {
	// Accept both PATCH and POST for flexibility
	if r.Method != http.MethodPatch && r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req api.UpdateSprintStatusRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.SprintID == "" {
		http.Error(w, "sprint_id required", http.StatusBadRequest)
		return
	}

	if req.Status == "" {
		http.Error(w, "status required", http.StatusBadRequest)
		return
	}

	status := types.SprintStatus(req.Status)
	if err := s.handler.storage.UpdateSprintStatus(r.Context(), req.SprintID, status); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.respondJSON(w, map[string]string{"status": "updated"})
}

func (s *HTTPServer) handleUpdateTaskResult(w http.ResponseWriter, r *http.Request) {
	// Accept both PATCH and POST for flexibility
	if r.Method != http.MethodPatch && r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req api.UpdateTaskResultRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.TaskID == "" {
		http.Error(w, "task_id required", http.StatusBadRequest)
		return
	}

	if req.Status == "" {
		http.Error(w, "status required", http.StatusBadRequest)
		return
	}

	taskStatus := types.SprintTaskStatus(req.Status)
	if err := s.handler.storage.UpdateTaskResult(r.Context(), req.TaskID, taskStatus, req.ResultSummary, req.ResultData, req.TokensInput, req.TokensOutput, req.CostUSD, req.ErrorMessage); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.respondJSON(w, map[string]string{"status": "updated"})
}

func (s *HTTPServer) handleListModels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	models, err := s.handler.storage.ListModels(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.respondJSON(w, models)
}

func (s *HTTPServer) handleUpdateModel(w http.ResponseWriter, r *http.Request) {
	// Accept both PATCH and POST for flexibility
	if r.Method != http.MethodPatch && r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse model name from URL path /api/v1/models/{name}
	modelName := r.URL.Path[len("/api/v1/models/"):]
	if modelName == "" {
		http.Error(w, "model name required in path", http.StatusBadRequest)
		return
	}

	var req api.UpdateModelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Update enabled state if provided
	if req.Enabled != nil {
		if err := s.handler.storage.UpdateModelEnabled(r.Context(), modelName, *req.Enabled); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	// Update config if provided
	if req.Config != nil {
		if err := s.handler.storage.UpdateModelConfig(r.Context(), modelName, req.Config); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	s.respondJSON(w, map[string]string{"status": "updated"})
}

func (s *HTTPServer) handleExecuteSprint(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		SprintID   string `json:"sprint_id"`
		ExecutedBy string `json:"executed_by,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.ExecutedBy == "" {
		req.ExecutedBy = "lex"
	}

	// Initialize backends (in production, this would be wired via DI)
	registry := backends.NewBackendRegistry()

	messagesAPI, err := backends.NewMessagesAPIBackend()
	if err == nil && messagesAPI != nil {
		registry.Register(messagesAPI)
	}

	ollama := backends.NewOllamaBackend("http://localhost:11434")
	registry.Register(ollama)

	router := dispatch.NewTierRouter(s.handler.storage, registry, s.logger)
	executor := sprint.NewSprintExecutor(s.handler.storage, router, s.logger)

	exec, err := executor.ExecuteSprint(r.Context(), req.SprintID, req.ExecutedBy)
	if err != nil {
		s.logger.Error("sprint execution failed", zap.Error(err))
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(exec)
}
