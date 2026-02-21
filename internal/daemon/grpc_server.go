package daemon

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/meridian-lex/stratavore/internal/storage"
	"github.com/meridian-lex/stratavore/pkg/api"
	"github.com/meridian-lex/stratavore/pkg/types"
	"go.uber.org/zap"
	"google.golang.org/grpc"
)

// GRPCServer implements the Stratavore gRPC API
type GRPCServer struct {
	runnerManager *RunnerManager
	storage       *storage.PostgresClient
	logger        *zap.Logger
	server        *grpc.Server
	port          int
}

// NewGRPCServer creates a new gRPC server
func NewGRPCServer(
	runnerManager *RunnerManager,
	storage *storage.PostgresClient,
	logger *zap.Logger,
	port int,
) *GRPCServer {
	return &GRPCServer{
		runnerManager: runnerManager,
		storage:       storage,
		logger:        logger,
		port:          port,
	}
}

func (s *GRPCServer) Start() error {
	addr := fmt.Sprintf(":%d", s.port)
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		s.logger.Error("failed to listen on port", zap.Int("port", s.port), zap.Error(err))
		return fmt.Errorf("failed to listen: %w", err)
	}

	s.server = grpc.NewServer()
	// api.RegisterStratavoreServiceServer(s.server, s)

	s.logger.Info("gRPC server starting", zap.String("address", addr))
	if err := s.server.Serve(lis); err != nil {
		s.logger.Error("gRPC server failed", zap.Error(err))
		return fmt.Errorf("gRPC server failed: %w", err)
	}
	return nil
}

func (s *GRPCServer) Stop() {
	if s.server != nil {
		s.logger.Info("stopping gRPC server")
		s.server.GracefulStop()
	}
}

// LaunchRunner handles runner launch requests
func (s *GRPCServer) LaunchRunner(ctx context.Context, req *api.LaunchRunnerRequest) (*api.LaunchRunnerResponse, error) {
	s.logger.Info("launch runner request",
		zap.String("project", req.ProjectName),
		zap.String("runtime", req.RuntimeType))

	// Convert to internal request
	launchReq := &types.LaunchRequest{
		ProjectName:      req.ProjectName,
		ProjectPath:      req.ProjectPath,
		Flags:            req.Flags,
		Capabilities:     req.Capabilities,
		Environment:      req.Environment,
		ConversationMode: types.ConversationMode(req.ConversationMode),
		SessionID:        req.SessionID,
		RuntimeType:      types.RuntimeType(req.RuntimeType),
	}

	// Launch runner
	runner, err := s.runnerManager.Launch(ctx, launchReq)
	if err != nil {
		s.logger.Error("failed to launch runner", zap.Error(err))
		return &api.LaunchRunnerResponse{
			Error: err.Error(),
		}, nil
	}

	return &api.LaunchRunnerResponse{
		Runner: convertRunnerToAPI(runner),
	}, nil
}

// StopRunner handles runner stop requests
func (s *GRPCServer) StopRunner(ctx context.Context, req *api.StopRunnerRequest) (*api.StopRunnerResponse, error) {
	s.logger.Info("stop runner request",
		zap.String("runner_id", req.RunnerID),
		zap.Bool("force", req.Force))

	err := s.runnerManager.StopRunner(ctx, req.RunnerID)
	if err != nil {
		s.logger.Error("failed to stop runner", zap.Error(err))
		return &api.StopRunnerResponse{
			Success: false,
			Error:   err.Error(),
		}, nil
	}

	return &api.StopRunnerResponse{
		Success: true,
	}, nil
}

// GetRunner retrieves runner details
func (s *GRPCServer) GetRunner(ctx context.Context, req *api.GetRunnerRequest) (*api.GetRunnerResponse, error) {
	runner, err := s.storage.GetRunner(ctx, req.RunnerID)
	if err != nil {
		return &api.GetRunnerResponse{
			Error: err.Error(),
		}, nil
	}

	return &api.GetRunnerResponse{
		Runner: convertRunnerToAPI(runner),
	}, nil
}

// ListRunners lists active runners
func (s *GRPCServer) ListRunners(ctx context.Context, req *api.ListRunnersRequest) (*api.ListRunnersResponse, error) {
	var runners []*types.Runner
	var err error

	if req.ProjectName != "" {
		runners, err = s.storage.GetActiveRunners(ctx, req.ProjectName)
	} else {
		runners = s.runnerManager.GetActiveRunners()
	}

	if err != nil {
		return &api.ListRunnersResponse{
			Error: err.Error(),
		}, nil
	}

	apiRunners := make([]*api.Runner, len(runners))
	for i, r := range runners {
		apiRunners[i] = convertRunnerToAPI(r)
	}

	return &api.ListRunnersResponse{
		Runners: apiRunners,
		Total:   int32(len(runners)),
	}, nil
}

// CreateProject creates a new project
func (s *GRPCServer) CreateProject(ctx context.Context, req *api.CreateProjectRequest) (*api.CreateProjectResponse, error) {
	project := &types.Project{
		Name:        req.Name,
		Path:        req.Path,
		Description: req.Description,
		Tags:        req.Tags,
		Status:      types.ProjectIdle,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	err := s.storage.CreateProject(ctx, project)
	if err != nil {
		return &api.CreateProjectResponse{
			Error: err.Error(),
		}, nil
	}

	return &api.CreateProjectResponse{
		Project: convertProjectToAPI(project),
	}, nil
}

// GetProject retrieves project details
func (s *GRPCServer) GetProject(ctx context.Context, req *api.GetProjectRequest) (*api.GetProjectResponse, error) {
	project, err := s.storage.GetProject(ctx, req.Name)
	if err != nil {
		return &api.GetProjectResponse{
			Error: err.Error(),
		}, nil
	}

	return &api.GetProjectResponse{
		Project: convertProjectToAPI(project),
	}, nil
}

// ListProjects lists all projects
func (s *GRPCServer) ListProjects(ctx context.Context, req *api.ListProjectsRequest) (*api.ListProjectsResponse, error) {
	projects, err := s.storage.ListProjects(ctx, req.Status)
	if err != nil {
		return &api.ListProjectsResponse{
			Error: err.Error(),
		}, nil
	}

	apiProjects := make([]*api.Project, len(projects))
	for i, p := range projects {
		apiProjects[i] = convertProjectToAPI(p)
	}

	return &api.ListProjectsResponse{
		Projects: apiProjects,
	}, nil
}

// SendHeartbeat processes heartbeat from agent
func (s *GRPCServer) SendHeartbeat(ctx context.Context, req *api.HeartbeatRequest) (*api.HeartbeatResponse, error) {
	hb := &types.Heartbeat{
		RunnerID:     req.RunnerID,
		Status:       types.RunnerStatus(req.Status),
		Timestamp:    time.Now(),
		CPUPercent:   req.CPUPercent,
		MemoryMB:     req.MemoryMB,
		TokensUsed:   req.TokensUsed,
		SessionID:    req.SessionID,
		AgentVersion: req.AgentVersion,
		Hostname:     req.Hostname,
	}

	err := s.runnerManager.ProcessHeartbeat(ctx, hb)
	if err != nil {
		s.logger.Error("heartbeat processing failed",
			zap.String("runner_id", req.RunnerID),
			zap.Error(err))
		return &api.HeartbeatResponse{
			Success: false,
			Error:   err.Error(),
		}, nil
	}

	return &api.HeartbeatResponse{
		Success: true,
	}, nil
}

// GetStatus returns daemon status
func (s *GRPCServer) GetStatus(ctx context.Context, req *api.GetStatusRequest) (*api.GetStatusResponse, error) {
	activeRunners := len(s.runnerManager.GetActiveRunners())

	metrics := &api.GlobalMetrics{
		ActiveRunners: int32(activeRunners),
	}

	daemonStatus := &api.DaemonStatus{
		Healthy:       true,
		LastHeartbeat: time.Now().Format(time.RFC3339),
	}

	return &api.GetStatusResponse{
		Daemon:  daemonStatus,
		Metrics: metrics,
	}, nil
}

// TriggerReconciliation manually triggers stale runner cleanup
func (s *GRPCServer) TriggerReconciliation(ctx context.Context, req *api.TriggerReconciliationRequest) (*api.TriggerReconciliationResponse, error) {
	s.logger.Info("manual reconciliation triggered")

	err := s.runnerManager.ReconcileRunners(ctx)
	if err != nil {
		return &api.TriggerReconciliationResponse{
			Error: err.Error(),
		}, nil
	}

	return &api.TriggerReconciliationResponse{
		ReconciledCount: 0,
	}, nil
}

// GetTokens returns token usage metrics
func (s *GRPCServer) GetTokens(ctx context.Context, req *api.GetTokensRequest) (*api.GetTokensResponse, error) {
	// Query all sessions to get token usage by project
	sessions, err := s.storage.ListAllSessions(ctx)
	if err != nil {
		s.logger.Error("failed to list sessions", zap.Error(err))
		return &api.GetTokensResponse{
			Error: err.Error(),
		}, nil
	}

	// Aggregate tokens by project
	totalTokens := int64(0)
	tokensByProject := make(map[string]int64)

	for _, session := range sessions {
		totalTokens += session.TokensUsed
		tokensByProject[session.ProjectName] += session.TokensUsed
	}

	// Get daily limit from token_budgets table
	dailyLimit := int64(0)
	budget, err := s.storage.GetDailyTokenBudget(ctx)
	if err == nil && budget != nil {
		dailyLimit = budget.LimitTokens
	}

	// Calculate usage percentage
	usagePercentage := 0.0
	if dailyLimit > 0 {
		usagePercentage = (float64(totalTokens) / float64(dailyLimit)) * 100
	}

	return &api.GetTokensResponse{
		TotalTokensUsed: totalTokens,
		TokensByProject: tokensByProject,
		DailyLimit:      dailyLimit,
		UsagePercentage: usagePercentage,
	}, nil
}

// Helper functions to convert between types

func convertRunnerToAPI(r *types.Runner) *api.Runner {
	apiRunner := &api.Runner{
		ID:                 r.ID,
		RuntimeType:        string(r.RuntimeType),
		RuntimeID:          r.RuntimeID,
		NodeID:             r.NodeID,
		ProjectName:        r.ProjectName,
		ProjectPath:        r.ProjectPath,
		Status:             string(r.Status),
		Flags:              r.Flags,
		Capabilities:       r.Capabilities,
		Environment:        r.Environment,
		SessionID:          r.SessionID,
		ConversationMode:   string(r.ConversationMode),
		TokensUsed:         r.TokensUsed,
		CPUPercent:         r.CPUPercent,
		MemoryMB:           r.MemoryMB,
		RestartAttempts:    int32(r.RestartAttempts),
		MaxRestartAttempts: int32(r.MaxRestartAttempts),
		StartedAt:          api.FormatTime(r.StartedAt),
		HeartbeatTTL:       int32(r.HeartbeatTTL),
		CreatedAt:          api.FormatTime(r.CreatedAt),
		UpdatedAt:          api.FormatTime(r.UpdatedAt),
	}

	if r.LastHeartbeat != nil {
		apiRunner.LastHeartbeat = api.FormatTime(*r.LastHeartbeat)
	}
	if r.TerminatedAt != nil {
		apiRunner.TerminatedAt = api.FormatTime(*r.TerminatedAt)
	}
	if r.ExitCode != nil {
		apiRunner.ExitCode = int32(*r.ExitCode)
	}

	return apiRunner
}

func convertProjectToAPI(p *types.Project) *api.Project {
	apiProject := &api.Project{
		Name:          p.Name,
		Path:          p.Path,
		Status:        string(p.Status),
		Description:   p.Description,
		Tags:          p.Tags,
		TotalRunners:  int32(p.TotalRunners),
		ActiveRunners: int32(p.ActiveRunners),
		TotalSessions: int32(p.TotalSessions),
		TotalTokens:   p.TotalTokens,
		CreatedAt:     api.FormatTime(p.CreatedAt),
		UpdatedAt:     api.FormatTime(p.UpdatedAt),
	}

	if p.LastAccessedAt != nil {
		apiProject.LastAccessedAt = api.FormatTime(*p.LastAccessedAt)
	}
	if p.ArchivedAt != nil {
		apiProject.ArchivedAt = api.FormatTime(*p.ArchivedAt)
	}

	return apiProject
}
