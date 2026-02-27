package daemon

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/meridian-lex/stratavore/pkg/api"
	"go.uber.org/zap"
)

// CompatibilityStatusHandler provides WebUI-compatible /api/status endpoint.
// This aggregates daemon data into the format expected by legacy WebUI.
type CompatibilityStatusHandler struct {
	handler *GRPCServer
	logger  *zap.Logger
	adapter *AgentAdapter

	// Cache for compiled status response
	cacheMutex     sync.RWMutex
	cachedResponse *WebUIStatusResponse
	cacheExpiry    time.Time
	cacheTTL       time.Duration
}

// WebUIStatusResponse represents the full status response expected by WebUI.
type WebUIStatusResponse struct {
	Status       string               `json:"status"`
	Timestamp    time.Time            `json:"timestamp"`
	Jobs         []WebUIJob           `json:"jobs"`
	Agents       map[string]WebUIAgent `json:"agents"`
	AgentTodos   []WebUIAgentTodo     `json:"agent_todos"`
	TimeSessions []WebUITimeSession   `json:"time_sessions"`
	Progress     WebUIProgress        `json:"progress"`
}

// WebUIJob represents a job in WebUI format.
type WebUIJob struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Status      string    `json:"status"`
	Priority    string    `json:"priority"`
	AssignedTo  string    `json:"assigned_to,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// WebUIAgentTodo represents an agent todo item.
type WebUIAgentTodo struct {
	ID          string    `json:"id"`
	AgentID     string    `json:"agent_id"`
	Task        string    `json:"task"`
	Status      string    `json:"status"`
	Priority    string    `json:"priority"`
	CreatedAt   time.Time `json:"created_at"`
}

// WebUITimeSession represents a time tracking session.
type WebUITimeSession struct {
	ID          string    `json:"id"`
	ProjectName string    `json:"project_name"`
	StartedAt   time.Time `json:"started_at"`
	EndedAt     *time.Time `json:"ended_at,omitempty"`
	Duration    int64     `json:"duration_seconds"`
	TokensUsed  int64     `json:"tokens_used"`
}

// WebUIProgress represents overall progress metrics.
type WebUIProgress struct {
	TotalJobs      int     `json:"total_jobs"`
	CompletedJobs  int     `json:"completed_jobs"`
	PendingJobs    int     `json:"pending_jobs"`
	ActiveAgents   int     `json:"active_agents"`
	TotalSessions  int     `json:"total_sessions"`
	TokensUsed     int64   `json:"tokens_used"`
	CompletionRate float64 `json:"completion_rate"`
}

// NewCompatibilityStatusHandler creates a new compatibility status handler.
func NewCompatibilityStatusHandler(handler *GRPCServer, adapter *AgentAdapter, logger *zap.Logger) *CompatibilityStatusHandler {
	return &CompatibilityStatusHandler{
		handler:  handler,
		logger:   logger,
		adapter:  adapter,
		cacheTTL: 5 * time.Second, // 5 second cache
	}
}

// HandleStatus handles GET /api/status
func (c *CompatibilityStatusHandler) HandleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Check cache first
	c.cacheMutex.RLock()
	if c.cachedResponse != nil && time.Now().Before(c.cacheExpiry) {
		response := c.cachedResponse
		c.cacheMutex.RUnlock()
		c.respondJSON(w, response)
		return
	}
	c.cacheMutex.RUnlock()

	// Build fresh response
	ctx := r.Context()
	response, err := c.buildStatusResponse(ctx)
	if err != nil {
		c.logger.Error("failed to build status response", zap.Error(err))
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Update cache
	c.cacheMutex.Lock()
	c.cachedResponse = response
	c.cacheExpiry = time.Now().Add(c.cacheTTL)
	c.cacheMutex.Unlock()

	c.respondJSON(w, response)
}

// buildStatusResponse compiles the full status response from daemon data.
func (c *CompatibilityStatusHandler) buildStatusResponse(ctx context.Context) (*WebUIStatusResponse, error) {
	response := &WebUIStatusResponse{
		Status:       "ok",
		Timestamp:    time.Now(),
		Jobs:         make([]WebUIJob, 0),
		Agents:       make(map[string]WebUIAgent),
		AgentTodos:   make([]WebUIAgentTodo, 0),
		TimeSessions: make([]WebUITimeSession, 0),
	}

	// Get runners (mapped to agents)
	listReq := &api.ListRunnersRequest{}
	listResp, err := c.handler.ListRunners(ctx, listReq)
	if err != nil {
		c.logger.Error("failed to list runners", zap.Error(err))
	} else if listResp.Error == "" {
		for _, runner := range listResp.Runners {
			agent := c.adapter.runnerToAgent(runner)
			response.Agents[agent.AgentID] = agent
		}
	}

	// Get sessions (mapped to time sessions)
	projects, err := c.handler.storage.ListProjects(ctx, "")
	if err != nil {
		c.logger.Warn("failed to list projects for sessions", zap.Error(err))
	} else {
		for _, proj := range projects {
			sessions, err := c.handler.storage.GetResumableSessions(ctx, proj.Name)
			if err != nil {
				c.logger.Warn("failed to get sessions", zap.String("project", proj.Name), zap.Error(err))
				continue
			}

			for _, sess := range sessions {
				timeSession := WebUITimeSession{
					ID:          sess.ID,
					ProjectName: sess.ProjectName,
					StartedAt:   sess.StartedAt,
					EndedAt:     sess.EndedAt,
					TokensUsed:  sess.TokensUsed,
				}

				if sess.EndedAt != nil {
					duration := sess.EndedAt.Sub(sess.StartedAt)
					timeSession.Duration = int64(duration.Seconds())
				} else {
					duration := time.Since(sess.StartedAt)
					timeSession.Duration = int64(duration.Seconds())
				}

				response.TimeSessions = append(response.TimeSessions, timeSession)
			}
		}
	}

	// Build progress metrics
	response.Progress = WebUIProgress{
		TotalJobs:     len(response.Jobs),
		CompletedJobs: 0,
		PendingJobs:   0,
		ActiveAgents:  len(response.Agents),
		TotalSessions: len(response.TimeSessions),
		TokensUsed:    0,
	}

	// Calculate token usage
	for _, agent := range response.Agents {
		response.Progress.TokensUsed += agent.TokensUsed
	}

	// Calculate completion rate
	if response.Progress.TotalJobs > 0 {
		response.Progress.CompletionRate = float64(response.Progress.CompletedJobs) / float64(response.Progress.TotalJobs)
	}

	return response, nil
}

// InvalidateCache forces cache refresh on next request.
func (c *CompatibilityStatusHandler) InvalidateCache() {
	c.cacheMutex.Lock()
	defer c.cacheMutex.Unlock()
	c.cachedResponse = nil
	c.cacheExpiry = time.Time{}
}

func (c *CompatibilityStatusHandler) respondJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}
