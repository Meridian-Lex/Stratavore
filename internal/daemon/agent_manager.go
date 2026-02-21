package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/meridian-lex/stratavore/internal/storage"
	"github.com/meridian-lex/stratavore/pkg/api"
	"go.uber.org/zap"
)

// AgentManager handles fleet agent personality and career progression operations
type AgentManager struct {
	db     *storage.PostgresClient
	logger *zap.Logger
}

// NewAgentManager creates a new agent manager
func NewAgentManager(db *storage.PostgresClient, logger *zap.Logger) *AgentManager {
	return &AgentManager{
		db:     db,
		logger: logger,
	}
}

// RegisterAgent creates a new agent personality with default rank (Unranked = 0)
func (m *AgentManager) RegisterAgent(ctx context.Context, req *api.RegisterAgentRequest) (*api.AgentPersonality, error) {
	if req.AgentName == "" {
		return nil, fmt.Errorf("agent_name required")
	}

	// Check if agent name already exists
	exists, err := m.db.CheckAgentNameExists(ctx, req.AgentName)
	if err != nil {
		return nil, fmt.Errorf("failed to check agent name: %w", err)
	}
	if exists {
		return nil, fmt.Errorf("agent name already registered: %s", req.AgentName)
	}

	agentID := uuid.New().String()
	now := time.Now()

	// Marshal personality traits to JSON
	var traitsJSON []byte
	if req.PersonalityTraits != nil && len(req.PersonalityTraits) > 0 {
		traitsJSON, err = json.Marshal(req.PersonalityTraits)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal personality traits: %w", err)
		}
	} else {
		traitsJSON = []byte("{}")
	}

	// Insert agent personality using transaction for consistency
	tx, err := m.db.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx, `
		INSERT INTO agent_personalities (
			id, agent_name, personality_traits, current_rank, rank_progress, strikes,
			created_at, last_active_at, updated_at
		) VALUES ($1, $2, $3, 0, 0, 0, $4, $4, $4)
	`, agentID, req.AgentName, traitsJSON, now)

	if err != nil {
		return nil, fmt.Errorf("failed to register agent: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	m.logger.Info("agent registered",
		zap.String("agent_id", agentID),
		zap.String("agent_name", req.AgentName))

	return m.GetAgent(ctx, agentID)
}

// GetAgent retrieves an agent personality by ID
func (m *AgentManager) GetAgent(ctx context.Context, agentID string) (*api.AgentPersonality, error) {
	// Use a transaction for consistent read
	tx, err := m.db.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	row := tx.QueryRow(ctx, `
		SELECT id, agent_name, specialization, personality_traits,
		       current_rank, rank_progress, strikes,
		       total_missions, successful_missions, failed_missions,
		       total_tokens_used, total_runtime_hours,
		       created_at, last_active_at, updated_at
		FROM agent_personalities
		WHERE id = $1
	`, agentID)

	agent := &api.AgentPersonality{}
	var specialization *string
	var lastActiveAt *time.Time
	var personalityTraits []byte

	err = row.Scan(
		&agent.ID,
		&agent.AgentName,
		&specialization,
		&personalityTraits,
		&agent.CurrentRank,
		&agent.RankProgress,
		&agent.Strikes,
		&agent.TotalMissions,
		&agent.SuccessfulMissions,
		&agent.FailedMissions,
		&agent.TotalTokensUsed,
		&agent.TotalRuntimeHours,
		&agent.CreatedAt,
		&lastActiveAt,
		&agent.UpdatedAt,
	)

	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("agent not found: %s", agentID)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get agent: %w", err)
	}

	if specialization != nil {
		agent.Specialization = *specialization
	}
	if lastActiveAt != nil {
		agent.LastActiveAt = lastActiveAt.String()
	}

	// Unmarshal personality traits from JSON byte array
	if len(personalityTraits) > 0 {
		err = json.Unmarshal(personalityTraits, &agent.PersonalityTraits)
		if err != nil {
			m.logger.Warn("failed to unmarshal personality traits",
				zap.String("agent_id", agentID),
				zap.Error(err))
			// Continue - don't fail the entire operation for malformed JSON
			agent.PersonalityTraits = make(map[string]interface{})
		}
	} else {
		agent.PersonalityTraits = make(map[string]interface{})
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return agent, nil
}

// ListAgents retrieves all agent personalities, ordered by rank descending
func (m *AgentManager) ListAgents(ctx context.Context, limit, offset int32) ([]*api.AgentPersonality, int32, error) {
	if limit <= 0 {
		limit = 50
	}

	// Use transaction for consistent read
	tx, err := m.db.BeginTx(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Get total count
	var total int32
	err = tx.QueryRow(ctx, "SELECT COUNT(*) FROM agent_personalities").Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count agents: %w", err)
	}

	// Get agents
	rows, err := tx.Query(ctx, `
		SELECT id, agent_name, specialization, personality_traits,
		       current_rank, rank_progress, strikes,
		       total_missions, successful_missions, failed_missions,
		       total_tokens_used, total_runtime_hours,
		       created_at, last_active_at, updated_at
		FROM agent_personalities
		ORDER BY current_rank DESC, rank_progress DESC, created_at ASC
		LIMIT $1 OFFSET $2
	`, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list agents: %w", err)
	}
	defer rows.Close()

	agents := []*api.AgentPersonality{}
	for rows.Next() {
		agent := &api.AgentPersonality{}
		var specialization *string
		var lastActiveAt *time.Time
		var personalityTraits []byte

		err := rows.Scan(
			&agent.ID,
			&agent.AgentName,
			&specialization,
			&personalityTraits,
			&agent.CurrentRank,
			&agent.RankProgress,
			&agent.Strikes,
			&agent.TotalMissions,
			&agent.SuccessfulMissions,
			&agent.FailedMissions,
			&agent.TotalTokensUsed,
			&agent.TotalRuntimeHours,
			&agent.CreatedAt,
			&lastActiveAt,
			&agent.UpdatedAt,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan agent: %w", err)
		}

		if specialization != nil {
			agent.Specialization = *specialization
		}
		if lastActiveAt != nil {
			agent.LastActiveAt = lastActiveAt.String()
		}

		// Unmarshal personality traits from JSON byte array
		if len(personalityTraits) > 0 {
			err = json.Unmarshal(personalityTraits, &agent.PersonalityTraits)
			if err != nil {
				m.logger.Warn("failed to unmarshal personality traits for agent",
					zap.String("agent_id", agent.ID),
					zap.Error(err))
				agent.PersonalityTraits = make(map[string]interface{})
			}
		} else {
			agent.PersonalityTraits = make(map[string]interface{})
		}

		agents = append(agents, agent)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, 0, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return agents, total, nil
}

// UpdateAgent updates agent personality traits or specialization
func (m *AgentManager) UpdateAgent(ctx context.Context, req *api.UpdateAgentRequest) (*api.AgentPersonality, error) {
	// Verify agent exists
	_, err := m.GetAgent(ctx, req.AgentID)
	if err != nil {
		return nil, err
	}

	// Use transaction to ensure atomicity of multiple updates
	tx, err := m.db.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Build dynamic update query
	if req.PersonalityTraits != nil {
		traitsJSON, err := json.Marshal(req.PersonalityTraits)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal personality traits: %w", err)
		}
		_, err = tx.Exec(ctx, `
			UPDATE agent_personalities
			SET personality_traits = $1, updated_at = NOW()
			WHERE id = $2
		`, traitsJSON, req.AgentID)
		if err != nil {
			return nil, fmt.Errorf("failed to update personality traits: %w", err)
		}
	}

	if req.Specialization != nil {
		_, err = tx.Exec(ctx, `
			UPDATE agent_personalities
			SET specialization = $1, updated_at = NOW()
			WHERE id = $2
		`, *req.Specialization, req.AgentID)
		if err != nil {
			return nil, fmt.Errorf("failed to update specialization: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return m.GetAgent(ctx, req.AgentID)
}

// CommendAgent awards commendation points and checks for promotion
func (m *AgentManager) CommendAgent(ctx context.Context, req *api.CommendAgentRequest) (bool, int, error) {
	if req.Achievement == "" {
		return false, 0, fmt.Errorf("achievement required")
	}
	if req.Points <= 0 {
		req.Points = 1 // Default to 1 point
	}

	tx, err := m.db.BeginTx(ctx)
	if err != nil {
		return false, 0, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Get current agent state
	var agentID, agentName string
	var currentRank, rankProgress int
	err = tx.QueryRow(ctx, `
		SELECT id, agent_name, current_rank, rank_progress
		FROM agent_personalities
		WHERE id = $1
		FOR UPDATE
	`, req.AgentID).Scan(&agentID, &agentName, &currentRank, &rankProgress)

	if err == pgx.ErrNoRows {
		return false, 0, fmt.Errorf("agent not found: %s", req.AgentID)
	}
	if err != nil {
		return false, 0, fmt.Errorf("failed to get agent: %w", err)
	}

	// Award commendation points
	newProgress := rankProgress + req.Points
	_, err = tx.Exec(ctx, `
		UPDATE agent_personalities
		SET rank_progress = rank_progress + $1, updated_at = NOW()
		WHERE id = $2
	`, req.Points, req.AgentID)
	if err != nil {
		return false, 0, fmt.Errorf("failed to update rank progress: %w", err)
	}

	// Record commendation event
	awardedBy := req.AwardedBy
	if awardedBy == "" {
		awardedBy = "System"
	}

	var missionID *string
	if req.MissionID != "" {
		missionID = &req.MissionID
	}
	var relatedPR *string
	if req.RelatedPR != "" {
		relatedPR = &req.RelatedPR
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO agent_rank_events (
			agent_id, event_type, rank_before, points_awarded,
			achievement, awarded_by, mission_id, related_pr, created_at
		) VALUES ($1, 'commendation', $2, $3, $4, $5, $6, $7, NOW())
	`, req.AgentID, currentRank, req.Points, req.Achievement, awardedBy, missionID, relatedPR)

	if err != nil {
		return false, 0, fmt.Errorf("failed to record commendation: %w", err)
	}

	// Check for promotion (5 points triggers promotion)
	promoted := false
	newRank := currentRank
	if newProgress >= 5 && currentRank < 10 {
		promoted = true
		newRank = currentRank + 1
		// Preserve remainder points after promotion (e.g., 7 points -> rank up with 2 points progress)
		remainderProgress := newProgress - 5

		_, err = tx.Exec(ctx, `
			UPDATE agent_personalities
			SET current_rank = $1, rank_progress = $2, updated_at = NOW()
			WHERE id = $3
		`, newRank, remainderProgress, req.AgentID)
		if err != nil {
			return false, 0, fmt.Errorf("failed to promote agent: %w", err)
		}

		// Record promotion event
		_, err = tx.Exec(ctx, `
			INSERT INTO agent_rank_events (
				agent_id, event_type, rank_before, rank_after,
				achievement, awarded_by, created_at
			) VALUES ($1, 'promotion', $2, $3, $4, $5, NOW())
		`, req.AgentID, currentRank, newRank, fmt.Sprintf("Promoted from rank %d to rank %d", currentRank, newRank), awardedBy)
		if err != nil {
			return false, 0, fmt.Errorf("failed to record promotion: %w", err)
		}

		m.logger.Info("agent promoted",
			zap.String("agent_id", agentID),
			zap.String("agent_name", agentName),
			zap.Int("old_rank", currentRank),
			zap.Int("new_rank", newRank))
	}

	if err := tx.Commit(ctx); err != nil {
		return false, 0, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return promoted, newRank, nil
}

// StrikeAgent issues a strike and checks for demotion (3 strikes = demotion)
func (m *AgentManager) StrikeAgent(ctx context.Context, req *api.StrikeAgentRequest) (bool, int, error) {
	if req.Infraction == "" {
		return false, 0, fmt.Errorf("infraction required")
	}

	tx, err := m.db.BeginTx(ctx)
	if err != nil {
		return false, 0, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Get current agent state
	var agentID, agentName string
	var currentRank, strikes int
	err = tx.QueryRow(ctx, `
		SELECT id, agent_name, current_rank, strikes
		FROM agent_personalities
		WHERE id = $1
		FOR UPDATE
	`, req.AgentID).Scan(&agentID, &agentName, &currentRank, &strikes)

	if err == pgx.ErrNoRows {
		return false, 0, fmt.Errorf("agent not found: %s", req.AgentID)
	}
	if err != nil {
		return false, 0, fmt.Errorf("failed to get agent: %w", err)
	}

	// Increment strike count
	newStrikes := strikes + 1
	_, err = tx.Exec(ctx, `
		UPDATE agent_personalities
		SET strikes = strikes + 1, updated_at = NOW()
		WHERE id = $1
	`, req.AgentID)
	if err != nil {
		return false, 0, fmt.Errorf("failed to update strikes: %w", err)
	}

	// Record strike event
	var missionID *string
	if req.MissionID != "" {
		missionID = &req.MissionID
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO agent_rank_events (
			agent_id, event_type, rank_before, infraction, awarded_by, mission_id, created_at
		) VALUES ($1, 'strike', $2, $3, 'System', $4, NOW())
	`, req.AgentID, currentRank, req.Infraction, missionID)

	if err != nil {
		return false, 0, fmt.Errorf("failed to record strike: %w", err)
	}

	// Check for demotion (3 strikes triggers demotion)
	demoted := false
	newRank := currentRank
	if newStrikes >= 3 && currentRank > 0 {
		demoted = true
		newRank = currentRank - 1

		_, err = tx.Exec(ctx, `
			UPDATE agent_personalities
			SET current_rank = $1, strikes = 0, updated_at = NOW()
			WHERE id = $2
		`, newRank, req.AgentID)
		if err != nil {
			return false, 0, fmt.Errorf("failed to demote agent: %w", err)
		}

		// Record demotion event
		_, err = tx.Exec(ctx, `
			INSERT INTO agent_rank_events (
				agent_id, event_type, rank_before, rank_after,
				infraction, awarded_by, created_at
			) VALUES ($1, 'demotion', $2, $3, $4, 'System', NOW())
		`, req.AgentID, currentRank, newRank, fmt.Sprintf("Demoted from rank %d to rank %d due to 3 strikes", currentRank, newRank))
		if err != nil {
			return false, 0, fmt.Errorf("failed to record demotion: %w", err)
		}

		m.logger.Warn("agent demoted",
			zap.String("agent_id", agentID),
			zap.String("agent_name", agentName),
			zap.Int("old_rank", currentRank),
			zap.Int("new_rank", newRank),
			zap.String("infraction", req.Infraction))
	}

	if err := tx.Commit(ctx); err != nil {
		return false, 0, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return demoted, newRank, nil
}

// scanMission populates an AgentMission from row scan results
func (m *AgentManager) scanMission(mission *api.AgentMission, missionDesc, projectName, runnerID, sessionID, resultSummary, completedAt *string) {
	if missionDesc != nil {
		mission.MissionDescription = *missionDesc
	}
	if projectName != nil {
		mission.ProjectName = *projectName
	}
	if runnerID != nil {
		mission.RunnerID = *runnerID
	}
	if sessionID != nil {
		mission.SessionID = *sessionID
	}
	if resultSummary != nil {
		mission.ResultSummary = *resultSummary
	}
	if completedAt != nil {
		mission.CompletedAt = *completedAt
	}
}

// ListAgentMissions retrieves mission history for an agent
func (m *AgentManager) ListAgentMissions(ctx context.Context, agentID string, limit, offset int32) ([]*api.AgentMission, int32, error) {
	if limit <= 0 {
		limit = 50
	}

	// Use transaction for consistent read
	tx, err := m.db.BeginTx(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Get total count
	var total int32
	err = tx.QueryRow(ctx, "SELECT COUNT(*) FROM agent_missions WHERE agent_id = $1", agentID).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count missions: %w", err)
	}

	// Get missions
	rows, err := tx.Query(ctx, `
		SELECT id, agent_id, mission_type, mission_name, mission_description,
		       project_name, runner_id, session_id, status, result_summary,
		       tokens_used, runtime_hours, started_at, completed_at, created_at
		FROM agent_missions
		WHERE agent_id = $1
		ORDER BY started_at DESC
		LIMIT $2 OFFSET $3
	`, agentID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list missions: %w", err)
	}
	defer rows.Close()

	missions := []*api.AgentMission{}
	for rows.Next() {
		mission := &api.AgentMission{}
		var missionDesc, projectName, runnerID, sessionID, resultSummary, completedAt *string

		err := rows.Scan(
			&mission.ID,
			&mission.AgentID,
			&mission.MissionType,
			&mission.MissionName,
			&missionDesc,
			&projectName,
			&runnerID,
			&sessionID,
			&mission.Status,
			&resultSummary,
			&mission.TokensUsed,
			&mission.RuntimeHours,
			&mission.StartedAt,
			&completedAt,
			&mission.CreatedAt,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan mission: %w", err)
		}

		m.scanMission(mission, missionDesc, projectName, runnerID, sessionID, resultSummary, completedAt)
		missions = append(missions, mission)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, 0, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return missions, total, nil
}

// CreateMission records a new mission for an agent
func (m *AgentManager) CreateMission(ctx context.Context, req *api.CreateMissionRequest) (*api.AgentMission, error) {
	if req.MissionType == "" {
		return nil, fmt.Errorf("mission_type required")
	}
	if req.MissionName == "" {
		return nil, fmt.Errorf("mission_name required")
	}

	// Verify agent exists
	_, err := m.GetAgent(ctx, req.AgentID)
	if err != nil {
		return nil, err
	}

	missionID := uuid.New().String()
	now := time.Now()

	var missionDescPtr *string
	if req.MissionDescription != "" {
		missionDescPtr = &req.MissionDescription
	}
	var projectNamePtr *string
	if req.ProjectName != "" {
		projectNamePtr = &req.ProjectName
	}
	var runnerIDPtr *string
	if req.RunnerID != "" {
		runnerIDPtr = &req.RunnerID
	}
	var sessionIDPtr *string
	if req.SessionID != "" {
		sessionIDPtr = &req.SessionID
	}

	// Use transaction to ensure atomicity of mission insertion and counter update
	tx, err := m.db.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx, `
		INSERT INTO agent_missions (
			id, agent_id, mission_type, mission_name, mission_description,
			project_name, runner_id, session_id, status, started_at, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'in_progress', $9, $9)
	`,
		missionID,
		req.AgentID,
		req.MissionType,
		req.MissionName,
		missionDescPtr,
		projectNamePtr,
		runnerIDPtr,
		sessionIDPtr,
		now,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to create mission: %w", err)
	}

	// Increment total_missions counter
	_, err = tx.Exec(ctx, `
		UPDATE agent_personalities
		SET total_missions = total_missions + 1, last_active_at = NOW(), updated_at = NOW()
		WHERE id = $1
	`, req.AgentID)
	if err != nil {
		return nil, fmt.Errorf("failed to update agent mission count: %w", err)
	}

	// Retrieve and return the created mission BEFORE committing
	row := tx.QueryRow(ctx, `
		SELECT id, agent_id, mission_type, mission_name, mission_description,
		       project_name, runner_id, session_id, status, result_summary,
		       tokens_used, runtime_hours, started_at, completed_at, created_at
		FROM agent_missions
		WHERE id = $1
	`, missionID)

	mission := &api.AgentMission{}
	var missionDesc, projectName, runnerID, sessionID, resultSummary, completedAt *string

	err = row.Scan(
		&mission.ID,
		&mission.AgentID,
		&mission.MissionType,
		&mission.MissionName,
		&missionDesc,
		&projectName,
		&runnerID,
		&sessionID,
		&mission.Status,
		&resultSummary,
		&mission.TokensUsed,
		&mission.RuntimeHours,
		&mission.StartedAt,
		&completedAt,
		&mission.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve created mission: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	m.logger.Info("mission created",
		zap.String("mission_id", missionID),
		zap.String("agent_id", req.AgentID),
		zap.String("mission_type", req.MissionType),
		zap.String("mission_name", req.MissionName))

	m.scanMission(mission, missionDesc, projectName, runnerID, sessionID, resultSummary, completedAt)
	return mission, nil
}
