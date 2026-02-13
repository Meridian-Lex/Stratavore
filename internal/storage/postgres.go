package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/meridian-lex/stratavore/pkg/types"
)

// PostgresClient handles PostgreSQL operations
type PostgresClient struct {
	pool *pgxpool.Pool
}

// NewPostgresClient creates a new PostgreSQL client
func NewPostgresClient(ctx context.Context, connString string, maxConns, minConns int) (*PostgresClient, error) {
	config, err := pgxpool.ParseConfig(connString)
	if err != nil {
		return nil, fmt.Errorf("parse connection string: %w", err)
	}

	config.MaxConns = int32(maxConns)
	config.MinConns = int32(minConns)
	config.MaxConnLifetime = time.Hour
	config.MaxConnIdleTime = 30 * time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}

	// Test connection
	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("ping database: %w", err)
	}

	return &PostgresClient{pool: pool}, nil
}

// Close closes the database connection pool
func (c *PostgresClient) Close() {
	c.pool.Close()
}

// BeginTx starts a new transaction
func (c *PostgresClient) BeginTx(ctx context.Context) (pgx.Tx, error) {
	return c.pool.Begin(ctx)
}

// ===== PROJECTS =====

// CreateProject creates a new project
func (c *PostgresClient) CreateProject(ctx context.Context, project *types.Project) error {
	query := `
		INSERT INTO projects (name, path, status, description, tags)
		VALUES ($1, $2, $3, $4, $5)
	`

	_, err := c.pool.Exec(ctx, query,
		project.Name,
		project.Path,
		project.Status,
		project.Description,
		project.Tags,
	)

	return err
}

// GetProject retrieves a project by name
func (c *PostgresClient) GetProject(ctx context.Context, name string) (*types.Project, error) {
	query := `
		SELECT name, path, status, description, tags,
		       total_runners, active_runners, total_sessions, total_tokens,
		       created_at, last_accessed_at, archived_at, updated_at
		FROM projects
		WHERE name = $1
	`

	var project types.Project
	var tags []string
	var lastAccessed, archived sql.NullTime

	err := c.pool.QueryRow(ctx, query, name).Scan(
		&project.Name,
		&project.Path,
		&project.Status,
		&project.Description,
		&tags,
		&project.TotalRunners,
		&project.ActiveRunners,
		&project.TotalSessions,
		&project.TotalTokens,
		&project.CreatedAt,
		&lastAccessed,
		&archived,
		&project.UpdatedAt,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("project not found: %s", name)
		}
		return nil, err
	}

	project.Tags = tags
	if lastAccessed.Valid {
		project.LastAccessedAt = &lastAccessed.Time
	}
	if archived.Valid {
		project.ArchivedAt = &archived.Time
	}

	return &project, nil
}

// ArchiveProject soft-deletes a project by setting its status to archived
func (c *PostgresClient) ArchiveProject(ctx context.Context, name string) error {
	_, err := c.pool.Exec(ctx, `
		UPDATE projects SET status = $1, updated_at = NOW() WHERE name = $2
	`, types.ProjectArchived, name)
	return err
}

// ListProjects returns all projects
func (c *PostgresClient) ListProjects(ctx context.Context, status string) ([]*types.Project, error) {
	query := `
		SELECT name, path, status, description, tags,
		       total_runners, active_runners, total_sessions, total_tokens,
		       created_at, last_accessed_at, archived_at, updated_at
		FROM projects
	`

	args := []interface{}{}
	if status != "" {
		query += " WHERE status = $1"
		args = append(args, status)
	}

	query += " ORDER BY last_accessed_at DESC NULLS LAST, name"

	rows, err := c.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var projects []*types.Project
	for rows.Next() {
		var project types.Project
		var tags []string
		var lastAccessed, archived sql.NullTime

		err := rows.Scan(
			&project.Name,
			&project.Path,
			&project.Status,
			&project.Description,
			&tags,
			&project.TotalRunners,
			&project.ActiveRunners,
			&project.TotalSessions,
			&project.TotalTokens,
			&project.CreatedAt,
			&lastAccessed,
			&archived,
			&project.UpdatedAt,
		)

		if err != nil {
			return nil, err
		}

		project.Tags = tags
		if lastAccessed.Valid {
			project.LastAccessedAt = &lastAccessed.Time
		}
		if archived.Valid {
			project.ArchivedAt = &archived.Time
		}

		projects = append(projects, &project)
	}

	return projects, rows.Err()
}

// ===== RUNNERS WITH TRANSACTIONAL OUTBOX =====

// CreateRunnerTx creates a runner and outbox event in a transaction
func (c *PostgresClient) CreateRunnerTx(ctx context.Context, req *types.LaunchRequest, quotaMax int) (*types.Runner, error) {
	tx, err := c.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Acquire advisory lock per project to avoid race conditions
	_, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hash_project($1))", req.ProjectName)
	if err != nil {
		return nil, fmt.Errorf("acquire lock: %w", err)
	}

	// Check quota
	var activeCount int
	err = tx.QueryRow(ctx, `
		SELECT count(*) FROM runners 
		WHERE project_name = $1 AND status IN ('starting', 'running')
	`, req.ProjectName).Scan(&activeCount)
	if err != nil {
		return nil, fmt.Errorf("check quota: %w", err)
	}

	if activeCount >= quotaMax {
		return nil, fmt.Errorf("quota exceeded: %d/%d runners active", activeCount, quotaMax)
	}

	// Create runner
	runnerID := uuid.New().String()
	runner := &types.Runner{
		ID:                 runnerID,
		RuntimeType:        req.RuntimeType,
		ProjectName:        req.ProjectName,
		ProjectPath:        req.ProjectPath,
		Status:             types.StatusStarting,
		Flags:              req.Flags,
		Capabilities:       req.Capabilities,
		Environment:        req.Environment,
		ConversationMode:   req.ConversationMode,
		SessionID:          req.SessionID,
		MaxRestartAttempts: 3,
		HeartbeatTTL:       30,
		StartedAt:          time.Now(),
		CreatedAt:          time.Now(),
		UpdatedAt:          time.Now(),
	}

	flagsJSON, _ := json.Marshal(runner.Flags)
	capsJSON, _ := json.Marshal(runner.Capabilities)
	envJSON, _ := json.Marshal(runner.Environment)

	_, err = tx.Exec(ctx, `
		INSERT INTO runners (
			id, runtime_type, runtime_id, project_name, project_path, status,
			flags, capabilities, environment, conversation_mode, session_id,
			max_restart_attempts, heartbeat_ttl_seconds, started_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
	`, runnerID, runner.RuntimeType, "", runner.ProjectName, runner.ProjectPath,
		runner.Status, flagsJSON, capsJSON, envJSON, runner.ConversationMode,
		runner.SessionID, runner.MaxRestartAttempts, runner.HeartbeatTTL,
		runner.StartedAt)

	if err != nil {
		return nil, fmt.Errorf("insert runner: %w", err)
	}

	// Create outbox event
	event := map[string]interface{}{
		"type":         "runner.started",
		"runner_id":    runnerID,
		"project_name": req.ProjectName,
		"timestamp":    time.Now().Format(time.RFC3339),
	}

	eventJSON, _ := json.Marshal(event)
	routingKey := fmt.Sprintf("runner.started.%s", req.ProjectName)

	_, err = tx.Exec(ctx, `
		INSERT INTO outbox (
			service_name, event_type, payload, aggregate_type, aggregate_id, routing_key
		) VALUES ($1, $2, $3, $4, $5, $6)
	`, "stratavore", "runner.started", eventJSON, "runner", runnerID, routingKey)

	if err != nil {
		return nil, fmt.Errorf("insert outbox: %w", err)
	}

	// Commit transaction
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit transaction: %w", err)
	}

	return runner, nil
}

// UpdateRunnerRuntimeID sets the runtime ID (PID/container ID) after agent starts
func (c *PostgresClient) UpdateRunnerRuntimeID(ctx context.Context, runnerID, runtimeID string) error {
	_, err := c.pool.Exec(ctx, `
		UPDATE runners SET runtime_id = $1 WHERE id = $2
	`, runtimeID, runnerID)
	return err
}

// UpdateRunnerStatus updates runner status
func (c *PostgresClient) UpdateRunnerStatus(ctx context.Context, runnerID string, status types.RunnerStatus) error {
	_, err := c.pool.Exec(ctx, `
		UPDATE runners SET status = $1 WHERE id = $2
	`, status, runnerID)
	return err
}

// UpdateRunnerHeartbeat updates runner heartbeat and metrics
func (c *PostgresClient) UpdateRunnerHeartbeat(ctx context.Context, hb *types.Heartbeat) error {
	_, err := c.pool.Exec(ctx, `
		UPDATE runners 
		SET last_heartbeat = $1, cpu_percent = $2, memory_mb = $3, 
		    tokens_used = $4, status = $5, session_id = $6
		WHERE id = $7
	`, hb.Timestamp, hb.CPUPercent, hb.MemoryMB, hb.TokensUsed, hb.Status, hb.SessionID, hb.RunnerID)

	return err
}

// TerminateRunner marks a runner as terminated
func (c *PostgresClient) TerminateRunner(ctx context.Context, runnerID string, exitCode int) error {
	now := time.Now()
	_, err := c.pool.Exec(ctx, `
		UPDATE runners 
		SET status = 'terminated', terminated_at = $1, exit_code = $2
		WHERE id = $3
	`, now, exitCode, runnerID)

	return err
}

// GetRunner retrieves a runner by ID
func (c *PostgresClient) GetRunner(ctx context.Context, runnerID string) (*types.Runner, error) {
	query := `
		SELECT id, runtime_type, runtime_id, node_id, project_name, project_path,
		       status, flags, capabilities, environment, session_id, conversation_mode,
		       tokens_used, cpu_percent, memory_mb, restart_attempts, max_restart_attempts,
		       started_at, last_heartbeat, heartbeat_ttl_seconds, terminated_at, exit_code,
		       created_at, updated_at
		FROM runners WHERE id = $1
	`

	var runner types.Runner
	var flagsJSON, capsJSON, envJSON []byte
	var nodeID, sessionID sql.NullString
	var conversationMode sql.NullString
	var cpuPercent sql.NullFloat64
	var memoryMB, tokensUsed sql.NullInt64
	var lastHeartbeat, terminatedAt sql.NullTime
	var exitCode sql.NullInt32

	err := c.pool.QueryRow(ctx, query, runnerID).Scan(
		&runner.ID, &runner.RuntimeType, &runner.RuntimeID, &nodeID,
		&runner.ProjectName, &runner.ProjectPath, &runner.Status,
		&flagsJSON, &capsJSON, &envJSON, &sessionID, &conversationMode,
		&tokensUsed, &cpuPercent, &memoryMB,
		&runner.RestartAttempts, &runner.MaxRestartAttempts,
		&runner.StartedAt, &lastHeartbeat, &runner.HeartbeatTTL,
		&terminatedAt, &exitCode, &runner.CreatedAt, &runner.UpdatedAt,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("runner not found: %s", runnerID)
		}
		return nil, err
	}

	json.Unmarshal(flagsJSON, &runner.Flags)
	json.Unmarshal(capsJSON, &runner.Capabilities)
	json.Unmarshal(envJSON, &runner.Environment)

	if nodeID.Valid {
		runner.NodeID = nodeID.String
	}
	if sessionID.Valid {
		runner.SessionID = sessionID.String
	}
	if conversationMode.Valid {
		runner.ConversationMode = types.ConversationMode(conversationMode.String)
	}
	if cpuPercent.Valid {
		runner.CPUPercent = cpuPercent.Float64
	}
	if memoryMB.Valid {
		runner.MemoryMB = memoryMB.Int64
	}
	if tokensUsed.Valid {
		runner.TokensUsed = tokensUsed.Int64
	}
	if lastHeartbeat.Valid {
		runner.LastHeartbeat = &lastHeartbeat.Time
	}
	if terminatedAt.Valid {
		runner.TerminatedAt = &terminatedAt.Time
	}
	if exitCode.Valid {
		ec := int(exitCode.Int32)
		runner.ExitCode = &ec
	}

	return &runner, nil
}

// GetActiveRunners returns all active runners for a project
func (c *PostgresClient) GetActiveRunners(ctx context.Context, projectName string) ([]*types.Runner, error) {
	query := `
		SELECT id, runtime_type, runtime_id, project_name, status, started_at, tokens_used
		FROM runners
		WHERE project_name = $1 AND status IN ('starting', 'running', 'paused')
		ORDER BY started_at DESC
	`

	rows, err := c.pool.Query(ctx, query, projectName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var runners []*types.Runner
	for rows.Next() {
		var r types.Runner
		var tokensUsed sql.NullInt64

		err := rows.Scan(&r.ID, &r.RuntimeType, &r.RuntimeID, &r.ProjectName,
			&r.Status, &r.StartedAt, &tokensUsed)
		if err != nil {
			return nil, err
		}

		if tokensUsed.Valid {
			r.TokensUsed = tokensUsed.Int64
		}

		runners = append(runners, &r)
	}

	return runners, rows.Err()
}

// ReconcileStaleRunners marks stale runners as failed
func (c *PostgresClient) ReconcileStaleRunners(ctx context.Context, ttlSeconds int) ([]string, error) {
	query := `
		SELECT reconcile_stale_runners($1)
	`

	rows, err := c.pool.Query(ctx, query, ttlSeconds)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var failedIDs []string
	for rows.Next() {
		var id, unused string
		if err := rows.Scan(&id, &unused); err != nil {
			return nil, err
		}
		failedIDs = append(failedIDs, id)
	}

	return failedIDs, rows.Err()
}

// ===== OUTBOX =====

// GetPendingOutboxEntries retrieves undelivered outbox entries
func (c *PostgresClient) GetPendingOutboxEntries(ctx context.Context, limit int) ([]*types.OutboxEntry, error) {
	query := `
		SELECT id, created_at, event_id, service_name, aggregate_type, aggregate_id,
		       event_type, payload, metadata, routing_key, attempts, max_attempts
		FROM outbox
		WHERE delivered = false 
		  AND (next_retry_at IS NULL OR next_retry_at <= NOW())
		ORDER BY created_at
		LIMIT $1
		FOR UPDATE SKIP LOCKED
	`

	rows, err := c.pool.Query(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []*types.OutboxEntry
	for rows.Next() {
		var entry types.OutboxEntry
		var payloadJSON, metadataJSON []byte
		var aggregateType, aggregateID sql.NullString

		err := rows.Scan(
			&entry.ID, &entry.CreatedAt, &entry.EventID, &entry.ServiceName,
			&aggregateType, &aggregateID, &entry.EventType,
			&payloadJSON, &metadataJSON, &entry.RoutingKey,
			&entry.Attempts, &entry.MaxAttempts,
		)
		if err != nil {
			return nil, err
		}

		json.Unmarshal(payloadJSON, &entry.Payload)
		json.Unmarshal(metadataJSON, &entry.Metadata)

		if aggregateType.Valid {
			entry.AggregateType = aggregateType.String
		}
		if aggregateID.Valid {
			entry.AggregateID = aggregateID.String
		}

		entries = append(entries, &entry)
	}

	return entries, rows.Err()
}

// MarkOutboxDelivered marks an outbox entry as delivered
func (c *PostgresClient) MarkOutboxDelivered(ctx context.Context, id int64) error {
	_, err := c.pool.Exec(ctx, `
		UPDATE outbox 
		SET delivered = true, delivered_at = NOW()
		WHERE id = $1
	`, id)
	return err
}

// IncrementOutboxAttempts increments retry attempts and schedules next retry
func (c *PostgresClient) IncrementOutboxAttempts(ctx context.Context, id int64, errMsg string) error {
	_, err := c.pool.Exec(ctx, `
		UPDATE outbox 
		SET attempts = attempts + 1,
		    last_attempt_at = NOW(),
		    next_retry_at = NOW() + (POWER(2, attempts) || ' seconds')::INTERVAL,
		    error = $1
		WHERE id = $2
	`, errMsg, id)
	return err
}

// ===== RESOURCE QUOTAS =====

// GetResourceQuota retrieves resource quota for a project
func (c *PostgresClient) GetResourceQuota(ctx context.Context, projectName string) (*types.ResourceQuota, error) {
	query := `
		SELECT project_name, max_concurrent_runners, max_memory_mb, max_cpu_percent, max_tokens_per_day
		FROM resource_quotas
		WHERE project_name = $1
	`

	var quota types.ResourceQuota
	var maxMemory, maxTokens sql.NullInt64
	var maxCPU sql.NullInt32

	err := c.pool.QueryRow(ctx, query, projectName).Scan(
		&quota.ProjectName, &quota.MaxConcurrentRunners,
		&maxMemory, &maxCPU, &maxTokens,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			// Return default quota
			return &types.ResourceQuota{
				ProjectName:          projectName,
				MaxConcurrentRunners: 5,
			}, nil
		}
		return nil, err
	}

	if maxMemory.Valid {
		quota.MaxMemoryMB = maxMemory.Int64
	}
	if maxCPU.Valid {
		quota.MaxCPUPercent = int(maxCPU.Int32)
	}
	if maxTokens.Valid {
		quota.MaxTokensPerDay = maxTokens.Int64
	}

	return &quota, nil
}

// ===== SESSIONS =====

// CreateSession creates a new session
func (c *PostgresClient) CreateSession(ctx context.Context, session *types.Session) error {
	query := `
		INSERT INTO sessions (id, runner_id, project_name, started_at, resumable)
		VALUES ($1, $2, $3, $4, $5)
	`

	_, err := c.pool.Exec(ctx, query,
		session.ID,
		session.RunnerID,
		session.ProjectName,
		session.StartedAt,
		session.Resumable,
	)

	return err
}

// GetSession retrieves a session by ID
func (c *PostgresClient) GetSession(ctx context.Context, sessionID string) (*types.Session, error) {
	query := `
		SELECT id, runner_id, project_name, started_at, ended_at, last_message_at,
		       message_count, tokens_used, resumable, resumed_from, summary,
		       transcript_s3_key, transcript_size_bytes, created_at
		FROM sessions
		WHERE id = $1
	`

	var session types.Session
	var endedAt, lastMessageAt sql.NullTime
	var resumedFrom, summary, transcriptKey sql.NullString
	var transcriptSize sql.NullInt64

	err := c.pool.QueryRow(ctx, query, sessionID).Scan(
		&session.ID,
		&session.RunnerID,
		&session.ProjectName,
		&session.StartedAt,
		&endedAt,
		&lastMessageAt,
		&session.MessageCount,
		&session.TokensUsed,
		&session.Resumable,
		&resumedFrom,
		&summary,
		&transcriptKey,
		&transcriptSize,
		&session.CreatedAt,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("session not found: %s", sessionID)
		}
		return nil, err
	}

	if endedAt.Valid {
		session.EndedAt = &endedAt.Time
	}
	if lastMessageAt.Valid {
		session.LastMessageAt = &lastMessageAt.Time
	}
	if resumedFrom.Valid {
		session.ResumedFrom = resumedFrom.String
	}
	if summary.Valid {
		session.Summary = summary.String
	}
	if transcriptKey.Valid {
		session.TranscriptS3Key = transcriptKey.String
	}
	if transcriptSize.Valid {
		session.TranscriptSizeBytes = transcriptSize.Int64
	}

	return &session, nil
}

// EndSession marks a session as ended
func (c *PostgresClient) EndSession(ctx context.Context, sessionID string, endedAt time.Time) error {
	_, err := c.pool.Exec(ctx, `
		UPDATE sessions SET ended_at = $1 WHERE id = $2
	`, endedAt, sessionID)
	return err
}

// UpdateSessionMessage updates session message stats
func (c *PostgresClient) UpdateSessionMessage(ctx context.Context, sessionID string, lastMessageAt time.Time, tokensUsed int64) error {
	_, err := c.pool.Exec(ctx, `
		UPDATE sessions 
		SET last_message_at = $1, 
		    message_count = message_count + 1,
		    tokens_used = tokens_used + $2
		WHERE id = $3
	`, lastMessageAt, tokensUsed, sessionID)
	return err
}

// GetResumableSessions returns resumable sessions for a project
func (c *PostgresClient) GetResumableSessions(ctx context.Context, projectName string) ([]*types.Session, error) {
	query := `
		SELECT id, runner_id, project_name, started_at, last_message_at,
		       message_count, tokens_used, summary, created_at
		FROM sessions
		WHERE project_name = $1 AND resumable = true AND ended_at IS NULL
		ORDER BY last_message_at DESC NULLS LAST
		LIMIT 10
	`

	rows, err := c.pool.Query(ctx, query, projectName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []*types.Session
	for rows.Next() {
		var s types.Session
		var lastMessageAt sql.NullTime
		var summary sql.NullString

		err := rows.Scan(
			&s.ID,
			&s.RunnerID,
			&s.ProjectName,
			&s.StartedAt,
			&lastMessageAt,
			&s.MessageCount,
			&s.TokensUsed,
			&summary,
			&s.CreatedAt,
		)
		if err != nil {
			return nil, err
		}

		if lastMessageAt.Valid {
			s.LastMessageAt = &lastMessageAt.Time
		}
		if summary.Valid {
			s.Summary = summary.String
		}

		sessions = append(sessions, &s)
	}

	return sessions, rows.Err()
}

// MarkSessionNonResumable marks a session as not resumable
func (c *PostgresClient) MarkSessionNonResumable(ctx context.Context, sessionID string) error {
	_, err := c.pool.Exec(ctx, `
		UPDATE sessions SET resumable = false WHERE id = $1
	`, sessionID)
	return err
}

// SaveTranscriptMetadata saves transcript metadata
func (c *PostgresClient) SaveTranscriptMetadata(ctx context.Context, sessionID, s3Key string, sizeBytes int64) error {
	_, err := c.pool.Exec(ctx, `
		UPDATE sessions 
		SET transcript_s3_key = $1, transcript_size_bytes = $2
		WHERE id = $3
	`, s3Key, sizeBytes, sessionID)
	return err
}

// ===== TOKEN BUDGETS =====

// GetTokenBudget retrieves active token budget for scope
func (c *PostgresClient) GetTokenBudget(ctx context.Context, scope, scopeID string) (*types.TokenBudget, error) {
	query := `
		SELECT id, scope, scope_id, limit_tokens, used_tokens, 
		       period_granularity, period_start, period_end
		FROM token_budgets
		WHERE scope = $1 
		  AND (scope_id = $2 OR ($2 = '' AND scope_id IS NULL))
		  AND period_end > NOW()
		ORDER BY period_start DESC
		LIMIT 1
	`

	var budget types.TokenBudget
	var scopeIDVal sql.NullString

	err := c.pool.QueryRow(ctx, query, scope, scopeID).Scan(
		&budget.ID,
		&budget.Scope,
		&scopeIDVal,
		&budget.LimitTokens,
		&budget.UsedTokens,
		&budget.PeriodGranularity,
		&budget.PeriodStart,
		&budget.PeriodEnd,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil // No budget configured
		}
		return nil, err
	}

	if scopeIDVal.Valid {
		budget.ScopeID = scopeIDVal.String
	}

	return &budget, nil
}

// CreateTokenBudget creates a new token budget
func (c *PostgresClient) CreateTokenBudget(ctx context.Context, budget *types.TokenBudget) error {
	var scopeID interface{}
	if budget.ScopeID == "" {
		scopeID = nil
	} else {
		scopeID = budget.ScopeID
	}

	_, err := c.pool.Exec(ctx, `
		INSERT INTO token_budgets (
			scope, scope_id, limit_tokens, used_tokens,
			period_granularity, period_start, period_end
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, budget.Scope, scopeID, budget.LimitTokens, budget.UsedTokens,
		budget.PeriodGranularity, budget.PeriodStart, budget.PeriodEnd)

	return err
}

// IncrementTokenUsage increments token usage for a budget
func (c *PostgresClient) IncrementTokenUsage(ctx context.Context, scope, scopeID string, tokens int64) error {
	var scopeIDVal interface{}
	if scopeID == "" {
		scopeIDVal = nil
	} else {
		scopeIDVal = scopeID
	}

	_, err := c.pool.Exec(ctx, `
		UPDATE token_budgets
		SET used_tokens = used_tokens + $1
		WHERE scope = $2
		  AND (scope_id = $3 OR ($3 IS NULL AND scope_id IS NULL))
		  AND period_end > NOW()
	`, tokens, scope, scopeIDVal)

	return err
}

// GetExpiredBudgets returns budgets that need rollover
func (c *PostgresClient) GetExpiredBudgets(ctx context.Context, now time.Time) ([]*types.TokenBudget, error) {
	query := `
		SELECT id, scope, scope_id, limit_tokens, used_tokens,
		       period_granularity, period_start, period_end
		FROM token_budgets
		WHERE period_end <= $1
		ORDER BY period_end
	`

	rows, err := c.pool.Query(ctx, query, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var budgets []*types.TokenBudget
	for rows.Next() {
		var budget types.TokenBudget
		var scopeIDVal sql.NullString

		err := rows.Scan(
			&budget.ID,
			&budget.Scope,
			&scopeIDVal,
			&budget.LimitTokens,
			&budget.UsedTokens,
			&budget.PeriodGranularity,
			&budget.PeriodStart,
			&budget.PeriodEnd,
		)
		if err != nil {
			return nil, err
		}

		if scopeIDVal.Valid {
			budget.ScopeID = scopeIDVal.String
		}

		budgets = append(budgets, &budget)
	}

	return budgets, rows.Err()
}
