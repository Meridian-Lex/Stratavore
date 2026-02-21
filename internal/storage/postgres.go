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

// ===== MODEL REGISTRY =====

// ListModels returns all enabled models from the registry.
func (c *PostgresClient) ListModels(ctx context.Context) ([]*types.ModelRegistry, error) {
	rows, err := c.pool.Query(ctx, `
		SELECT id, name, display_name, backend, tier,
		       cost_per_million_input, cost_per_million_output,
		       context_window, max_output_tokens, backend_config, enabled, created_at
		FROM model_registry
		WHERE enabled = true
		ORDER BY tier, name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var models []*types.ModelRegistry
	for rows.Next() {
		m, err := scanModel(rows)
		if err != nil {
			return nil, err
		}
		models = append(models, m)
	}
	return models, rows.Err()
}

// GetModel returns a model by name.
func (c *PostgresClient) GetModel(ctx context.Context, name string) (*types.ModelRegistry, error) {
	row := c.pool.QueryRow(ctx, `
		SELECT id, name, display_name, backend, tier,
		       cost_per_million_input, cost_per_million_output,
		       context_window, max_output_tokens, backend_config, enabled, created_at
		FROM model_registry
		WHERE name = $1
	`, name)
	return scanModel(row)
}

// UpdateModelEnabled sets the enabled flag on a model.
func (c *PostgresClient) UpdateModelEnabled(ctx context.Context, name string, enabled bool) error {
	_, err := c.pool.Exec(ctx, `
		UPDATE model_registry SET enabled = $1 WHERE name = $2
	`, enabled, name)
	return err
}

// UpdateModelConfig updates the backend_config JSONB field for a model.
func (c *PostgresClient) UpdateModelConfig(ctx context.Context, name string, config map[string]interface{}) error {
	cfgJSON, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	_, err = c.pool.Exec(ctx, `
		UPDATE model_registry SET backend_config = $1 WHERE name = $2
	`, cfgJSON, name)
	return err
}

type rowScanner interface {
	Scan(dest ...interface{}) error
}

func scanModel(row rowScanner) (*types.ModelRegistry, error) {
	var m types.ModelRegistry
	var costIn, costOut sql.NullFloat64
	var contextWin, maxOut sql.NullInt32
	var cfgJSON []byte

	err := row.Scan(
		&m.ID, &m.Name, &m.DisplayName, &m.Backend, &m.Tier,
		&costIn, &costOut, &contextWin, &maxOut,
		&cfgJSON, &m.Enabled, &m.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	if costIn.Valid {
		m.CostPerMillionInput = costIn.Float64
	}
	if costOut.Valid {
		m.CostPerMillionOutput = costOut.Float64
	}
	if contextWin.Valid {
		m.ContextWindow = int(contextWin.Int32)
	}
	if maxOut.Valid {
		m.MaxOutputTokens = int(maxOut.Int32)
	}
	if len(cfgJSON) > 0 {
		json.Unmarshal(cfgJSON, &m.BackendConfig)
	}
	return &m, nil
}

// ===== SPRINTS =====

// CreateSprint creates a new sprint and returns its UUID.
func (c *PostgresClient) CreateSprint(ctx context.Context, sprint *types.Sprint) error {
	tagsJSON, _ := json.Marshal(sprint.Tags)
	id := uuid.New().String()

	_, err := c.pool.Exec(ctx, `
		INSERT INTO sprints (id, name, description, project_name, status, created_by, tags)
		VALUES ($1, $2, $3, NULLIF($4,''), $5, $6, $7::jsonb)
	`, id, sprint.Name, sprint.Description, sprint.ProjectName,
		string(SprintStatusPending), sprint.CreatedBy, tagsJSON)
	if err != nil {
		return err
	}
	sprint.ID = id
	return nil
}

// GetSprint retrieves a sprint by ID, optionally loading tasks.
func (c *PostgresClient) GetSprint(ctx context.Context, id string, includeTasks bool) (*types.Sprint, error) {
	var s types.Sprint
	var desc, projectName sql.NullString
	var startedAt, completedAt sql.NullTime
	var tagsJSON []byte

	err := c.pool.QueryRow(ctx, `
		SELECT id, name, description, project_name, status, created_by, tags,
		       created_at, started_at, completed_at, updated_at
		FROM sprints WHERE id = $1
	`, id).Scan(
		&s.ID, &s.Name, &desc, &projectName, &s.Status, &s.CreatedBy,
		&tagsJSON, &s.CreatedAt, &startedAt, &completedAt, &s.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("sprint not found: %s", id)
		}
		return nil, err
	}

	if desc.Valid {
		s.Description = desc.String
	}
	if projectName.Valid {
		s.ProjectName = projectName.String
	}
	if startedAt.Valid {
		s.StartedAt = &startedAt.Time
	}
	if completedAt.Valid {
		s.CompletedAt = &completedAt.Time
	}
	json.Unmarshal(tagsJSON, &s.Tags)

	if includeTasks {
		tasks, err := c.ListSprintTasks(ctx, id)
		if err != nil {
			return nil, err
		}
		s.Tasks = tasks
	}

	return &s, nil
}

// ListSprints returns sprints filtered by optional project name and status.
func (c *PostgresClient) ListSprints(ctx context.Context, projectName, status string) ([]*types.Sprint, error) {
	query := `
		SELECT id, name, description, project_name, status, created_by, tags,
		       created_at, started_at, completed_at, updated_at
		FROM sprints
		WHERE ($1 = '' OR project_name = $1)
		  AND ($2 = '' OR status = $2)
		ORDER BY created_at DESC
		LIMIT 100
	`
	rows, err := c.pool.Query(ctx, query, projectName, status)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sprints []*types.Sprint
	for rows.Next() {
		var s types.Sprint
		var desc, proj sql.NullString
		var startedAt, completedAt sql.NullTime
		var tagsJSON []byte

		err := rows.Scan(
			&s.ID, &s.Name, &desc, &proj, &s.Status, &s.CreatedBy,
			&tagsJSON, &s.CreatedAt, &startedAt, &completedAt, &s.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		if desc.Valid {
			s.Description = desc.String
		}
		if proj.Valid {
			s.ProjectName = proj.String
		}
		if startedAt.Valid {
			s.StartedAt = &startedAt.Time
		}
		if completedAt.Valid {
			s.CompletedAt = &completedAt.Time
		}
		json.Unmarshal(tagsJSON, &s.Tags)
		sprints = append(sprints, &s)
	}
	return sprints, rows.Err()
}

// UpdateSprintStatus sets the status (and optional timestamps) on a sprint.
func (c *PostgresClient) UpdateSprintStatus(ctx context.Context, id string, status types.SprintStatus) error {
	var startClause string
	if status == types.SprintRunning {
		startClause = ", started_at = NOW()"
	}
	var endClause string
	if status == types.SprintCompleted || status == types.SprintFailed || status == types.SprintCancelled {
		endClause = ", completed_at = NOW()"
	}

	_, err := c.pool.Exec(ctx, fmt.Sprintf(`
		UPDATE sprints
		SET status = $1, updated_at = NOW()%s%s
		WHERE id = $2
	`, startClause, endClause), string(status), id)
	return err
}

// SprintStatusPending is the default sprint status string (unexported sentinel).
const SprintStatusPending = "pending"

// ===== SPRINT TASKS =====

// AddSprintTask adds a task to an existing sprint.
func (c *PostgresClient) AddSprintTask(ctx context.Context, task *types.SprintTask) error {
	id := uuid.New().String()
	depsJSON, _ := json.Marshal(task.DependsOn)
	resultJSON, _ := json.Marshal(task.ResultData)

	_, err := c.pool.Exec(ctx, `
		INSERT INTO sprint_tasks (
			id, sprint_id, sequence_number, depends_on, name, description,
			model_name, system_prompt, user_prompt, max_tokens, temperature,
			status, result_data
		) VALUES ($1,$2,$3,$4::jsonb,$5,$6,$7,$8,$9,$10,$11,'pending',$12::jsonb)
	`, id, task.SprintID, task.SequenceNumber, depsJSON,
		task.Name, task.Description, task.ModelName,
		task.SystemPrompt, task.UserPrompt, task.MaxTokens, task.Temperature,
		resultJSON)
	if err != nil {
		return err
	}
	task.ID = id
	return nil
}

// ListSprintTasks returns all tasks for a sprint ordered by sequence.
func (c *PostgresClient) ListSprintTasks(ctx context.Context, sprintID string) ([]types.SprintTask, error) {
	rows, err := c.pool.Query(ctx, `
		SELECT id, sprint_id, sequence_number, depends_on, name, description,
		       model_name, system_prompt, user_prompt, max_tokens, temperature,
		       status, result_summary, result_data, tokens_input, tokens_output,
		       cost_usd, started_at, completed_at, error_message, created_at, updated_at
		FROM sprint_tasks
		WHERE sprint_id = $1
		ORDER BY sequence_number, created_at
	`, sprintID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []types.SprintTask
	for rows.Next() {
		t, err := scanSprintTask(rows)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, t)
	}
	return tasks, rows.Err()
}

// UpdateTaskResult writes the result of a completed or failed task.
func (c *PostgresClient) UpdateTaskResult(ctx context.Context, taskID string, status types.SprintTaskStatus,
	summary string, resultData map[string]interface{}, tokensIn, tokensOut int64, costUSD float64, errMsg string) error {

	resultJSON, _ := json.Marshal(resultData)
	var endClause string
	if status == types.TaskCompleted || status == types.TaskFailed {
		endClause = ", completed_at = NOW()"
	}

	_, err := c.pool.Exec(ctx, fmt.Sprintf(`
		UPDATE sprint_tasks
		SET status = $1, result_summary = $2, result_data = $3::jsonb,
		    tokens_input = $4, tokens_output = $5, cost_usd = $6,
		    error_message = $7, updated_at = NOW()%s
		WHERE id = $8
	`, endClause),
		string(status), summary, resultJSON,
		tokensIn, tokensOut, costUSD, errMsg, taskID)
	return err
}

// UpdateTaskStatus sets only the status (and started_at when transitioning to running).
func (c *PostgresClient) UpdateTaskStatus(ctx context.Context, taskID string, status types.SprintTaskStatus) error {
	startClause := ""
	if status == types.TaskRunning {
		startClause = ", started_at = NOW()"
	}
	_, err := c.pool.Exec(ctx, fmt.Sprintf(`
		UPDATE sprint_tasks SET status = $1, updated_at = NOW()%s WHERE id = $2
	`, startClause), string(status), taskID)
	return err
}

func scanSprintTask(rows pgx.Rows) (types.SprintTask, error) {
	var t types.SprintTask
	var depsJSON, resultJSON []byte
	var desc, systemPrompt, resultSummary, errMsg sql.NullString
	var startedAt, completedAt sql.NullTime

	err := rows.Scan(
		&t.ID, &t.SprintID, &t.SequenceNumber, &depsJSON,
		&t.Name, &desc, &t.ModelName, &systemPrompt, &t.UserPrompt,
		&t.MaxTokens, &t.Temperature, &t.Status,
		&resultSummary, &resultJSON, &t.TokensInput, &t.TokensOutput,
		&t.CostUSD, &startedAt, &completedAt, &errMsg,
		&t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		return t, err
	}
	if desc.Valid {
		t.Description = desc.String
	}
	if systemPrompt.Valid {
		t.SystemPrompt = systemPrompt.String
	}
	if resultSummary.Valid {
		t.ResultSummary = resultSummary.String
	}
	if errMsg.Valid {
		t.ErrorMessage = errMsg.String
	}
	if startedAt.Valid {
		t.StartedAt = &startedAt.Time
	}
	if completedAt.Valid {
		t.CompletedAt = &completedAt.Time
	}
	json.Unmarshal(depsJSON, &t.DependsOn)
	json.Unmarshal(resultJSON, &t.ResultData)
	return t, nil
}

// ===== SPRINT EXECUTIONS =====

// CreateSprintExecution starts a new execution audit record.
func (c *PostgresClient) CreateSprintExecution(ctx context.Context, sprintID, executedBy string, tasksTotal int) (*types.SprintExecution, error) {
	id := uuid.New().String()
	_, err := c.pool.Exec(ctx, `
		INSERT INTO sprint_executions (id, sprint_id, executed_by, status, tasks_total)
		VALUES ($1, $2, $3, 'running', $4)
	`, id, sprintID, executedBy, tasksTotal)
	if err != nil {
		return nil, err
	}
	return &types.SprintExecution{
		ID:          id,
		SprintID:    sprintID,
		ExecutedBy:  executedBy,
		Status:      "running",
		TasksTotal:  tasksTotal,
	}, nil
}

// CompleteSprintExecution finalizes an execution record.
func (c *PostgresClient) CompleteSprintExecution(ctx context.Context, execID, status string,
	completed, failed int, tokensIn, tokensOut int64, costUSD float64, durationMs int64) error {

	_, err := c.pool.Exec(ctx, `
		UPDATE sprint_executions
		SET status = $1, tasks_completed = $2, tasks_failed = $3,
		    total_tokens_input = $4, total_tokens_output = $5,
		    total_cost_usd = $6, duration_ms = $7, completed_at = NOW()
		WHERE id = $8
	`, status, completed, failed, tokensIn, tokensOut, costUSD, durationMs, execID)
	return err
}

// GetOperationalMode retrieves the current operational mode from daemon state
func (c *PostgresClient) GetOperationalMode(ctx context.Context) (string, string, error) {
	var configJSON []byte
	err := c.pool.QueryRow(ctx, `
		SELECT config FROM daemon_state WHERE singleton = true
	`).Scan(&configJSON)

	if err == pgx.ErrNoRows {
		// No daemon state yet, return default
		return "IDLE", "", nil
	}
	if err != nil {
		return "", "", fmt.Errorf("query daemon_state: %w", err)
	}

	var config map[string]interface{}
	if err := json.Unmarshal(configJSON, &config); err != nil {
		return "", "", fmt.Errorf("unmarshal config: %w", err)
	}

	mode, _ := config["operational_mode"].(string)
	description, _ := config["mode_description"].(string)

	if mode == "" {
		mode = "IDLE"
	}

	return mode, description, nil
}

// SetOperationalMode updates the operational mode in daemon state
func (c *PostgresClient) SetOperationalMode(ctx context.Context, mode, description string) error {
	// Upsert daemon state with mode in config JSONB
	_, err := c.pool.Exec(ctx, `
		INSERT INTO daemon_state (singleton, daemon_id, hostname, version, started_at, last_heartbeat, config)
		VALUES (
			true,
			gen_random_uuid(),
			'stratavore-cli',
			'1.4.0',
			NOW(),
			NOW(),
			jsonb_build_object('operational_mode', $1, 'mode_description', $2)
		)
		ON CONFLICT (singleton) DO UPDATE
		SET config = jsonb_set(
			jsonb_set(
				daemon_state.config,
				'{operational_mode}',
				to_jsonb($1::text)
			),
			'{mode_description}',
			to_jsonb($2::text)
		),
		last_heartbeat = NOW()
	`, mode, description)

	return err
}

// ListAllSessions returns all sessions across all projects with their token usage
func (c *PostgresClient) ListAllSessions(ctx context.Context) ([]*types.Session, error) {
	query := `
		SELECT id, runner_id, project_name, started_at, last_message_at,
		       message_count, tokens_used, summary, created_at
		FROM sessions
		ORDER BY started_at DESC
	`

	rows, err := c.pool.Query(ctx, query)
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

// GetDailyTokenBudget retrieves the current daily token budget
func (c *PostgresClient) GetDailyTokenBudget(ctx context.Context) (*types.TokenBudget, error) {
	query := `
		SELECT id, scope, scope_id, limit_tokens, used_tokens,
		       period_granularity, period_start, period_end
		FROM token_budgets
		WHERE scope = 'global'
		  AND period_granularity = 'daily'
		  AND period_end > NOW()
		ORDER BY period_start DESC
		LIMIT 1
	`

	var budget types.TokenBudget
	var scopeIDVal sql.NullString

	err := c.pool.QueryRow(ctx, query).Scan(
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
