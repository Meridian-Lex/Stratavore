-- Fleet Agent Identity & Career Progression System
-- Enables autonomous agents to develop persistent personalities and rank up through the fleet

-- Agent personality and identity
CREATE TABLE agent_personalities (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Identity
    agent_name TEXT NOT NULL UNIQUE,
    specialization TEXT,  -- Develops organically through mission history
    personality_traits JSONB DEFAULT '{}',

    -- Career progression
    current_rank INTEGER NOT NULL DEFAULT 0,  -- 0=Unranked, 1=Cadet, ..., 10=Admiral
    rank_progress INTEGER NOT NULL DEFAULT 0,  -- Commendation points toward next rank (0-4, promotes at 5)
    strikes INTEGER NOT NULL DEFAULT 0,  -- Strike count (demoted at 3)

    -- Constraints
    CHECK (current_rank >= 0 AND current_rank <= 10),
    CHECK (rank_progress >= 0 AND rank_progress < 5),
    CHECK (strikes >= 0 AND strikes < 3),

    -- Service record
    total_missions INTEGER DEFAULT 0,
    successful_missions INTEGER DEFAULT 0,
    failed_missions INTEGER DEFAULT 0,

    -- Statistics
    total_tokens_used BIGINT DEFAULT 0,
    total_runtime_hours DECIMAL(10,2) DEFAULT 0,

    -- Metadata
    created_at TIMESTAMPTZ DEFAULT NOW(),
    last_active_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_agent_personalities_rank ON agent_personalities(current_rank DESC);
CREATE INDEX idx_agent_personalities_name ON agent_personalities(agent_name);
CREATE INDEX idx_agent_personalities_active ON agent_personalities(last_active_at DESC);

-- Agent mission history (full audit trail)
CREATE TABLE agent_missions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_id UUID NOT NULL,

    -- Mission identification
    mission_type TEXT NOT NULL,  -- 'feature', 'bugfix', 'infrastructure', 'research', etc.
    mission_name TEXT NOT NULL,
    mission_description TEXT,

    -- Execution context
    project_name TEXT,
    runner_id UUID,
    session_id TEXT,

    -- Outcome
    status TEXT NOT NULL,  -- 'pending', 'in_progress', 'success', 'failed', 'abandoned'
    result_summary TEXT,

    -- Metrics
    tokens_used BIGINT DEFAULT 0,
    runtime_hours DECIMAL(10,2) DEFAULT 0,

    -- Timestamps
    started_at TIMESTAMPTZ DEFAULT NOW(),
    completed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW(),

    FOREIGN KEY (agent_id) REFERENCES agent_personalities(id) ON DELETE CASCADE,
    FOREIGN KEY (runner_id) REFERENCES runners(id) ON DELETE SET NULL,

    -- Constraints
    CHECK (status IN ('pending', 'in_progress', 'success', 'failed', 'abandoned')),
    CHECK (tokens_used >= 0),
    CHECK (runtime_hours >= 0)
);

CREATE INDEX idx_agent_missions_agent ON agent_missions(agent_id);
CREATE INDEX idx_agent_missions_status ON agent_missions(status);
CREATE INDEX idx_agent_missions_completed ON agent_missions(completed_at DESC);
CREATE INDEX idx_agent_missions_project ON agent_missions(project_name);

-- Agent rank events (promotions, demotions, commendations, strikes)
CREATE TABLE agent_rank_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_id UUID NOT NULL,

    -- Event classification
    event_type TEXT NOT NULL,  -- 'commendation', 'strike', 'promotion', 'demotion'

    -- Rank context
    rank_before INTEGER,
    rank_after INTEGER,
    points_awarded INTEGER DEFAULT 0,  -- For commendations

    -- Details
    achievement TEXT,  -- What earned the commendation/promotion
    infraction TEXT,  -- What caused the strike/demotion
    awarded_by TEXT,  -- Usually 'Fleet Admiral Lunar Laurus' or 'System'

    -- Context
    mission_id UUID,
    related_pr TEXT,  -- GitHub PR URL if applicable

    -- Metadata
    created_at TIMESTAMPTZ DEFAULT NOW(),

    FOREIGN KEY (agent_id) REFERENCES agent_personalities(id) ON DELETE CASCADE,
    FOREIGN KEY (mission_id) REFERENCES agent_missions(id) ON DELETE SET NULL,

    -- Constraints
    CHECK (event_type IN ('commendation', 'strike', 'promotion', 'demotion')),
    CHECK (points_awarded >= 0),
    CHECK (rank_before >= 0 AND rank_before <= 10),
    CHECK (rank_after >= 0 AND rank_after <= 10)
);

CREATE INDEX idx_agent_rank_events_agent ON agent_rank_events(agent_id);
CREATE INDEX idx_agent_rank_events_type ON agent_rank_events(event_type);
CREATE INDEX idx_agent_rank_events_created ON agent_rank_events(created_at DESC);

-- Trigger to keep agent_personalities.updated_at current
CREATE TRIGGER agent_personalities_updated_at BEFORE UPDATE ON agent_personalities
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();

-- RESERVED FOR FUTURE USE: Database-level utilities for agent management
-- These functions could be invoked directly from database clients without Go overhead.
-- Currently, equivalent logic is implemented in Go for consistency with application layer.

-- Function to calculate agent specialization from mission history
-- Specialization develops after 3+ successful missions of a consistent type
CREATE OR REPLACE FUNCTION calculate_agent_specialization(p_agent_id UUID)
RETURNS TEXT AS $$
DECLARE
    v_specialization TEXT;
    v_mission_count INTEGER;
BEGIN
    -- Find most common successful mission type with 3+ occurrences
    SELECT mission_type, COUNT(*) INTO v_specialization, v_mission_count
    FROM agent_missions
    WHERE agent_id = p_agent_id
      AND status = 'success'
    GROUP BY mission_type
    ORDER BY COUNT(*) DESC
    LIMIT 1;

    -- Only assign specialization if pattern is strong (3+ missions)
    IF v_mission_count >= 3 THEN
        RETURN v_specialization;
    ELSE
        RETURN NULL;
    END IF;
END;
$$ LANGUAGE plpgsql;

-- Function to apply rank promotion rules
-- Returns new rank and whether promotion occurred
CREATE OR REPLACE FUNCTION apply_rank_promotion(p_agent_id UUID)
RETURNS TABLE(promoted BOOLEAN, new_rank INTEGER) AS $$
DECLARE
    v_current_rank INTEGER;
    v_progress INTEGER;
BEGIN
    SELECT current_rank, rank_progress INTO v_current_rank, v_progress
    FROM agent_personalities
    WHERE id = p_agent_id;

    -- Promotion occurs at 5 points
    IF v_progress >= 5 AND v_current_rank < 10 THEN
        -- Promote: rank up, reset progress, keep strikes
        UPDATE agent_personalities
        SET current_rank = current_rank + 1,
            rank_progress = 0
        WHERE id = p_agent_id;

        RETURN QUERY SELECT true, v_current_rank + 1;
    ELSE
        RETURN QUERY SELECT false, v_current_rank;
    END IF;
END;
$$ LANGUAGE plpgsql;

-- Function to apply demotion rules (3 strikes)
CREATE OR REPLACE FUNCTION apply_rank_demotion(p_agent_id UUID)
RETURNS TABLE(demoted BOOLEAN, new_rank INTEGER) AS $$
DECLARE
    v_current_rank INTEGER;
    v_strikes INTEGER;
BEGIN
    SELECT current_rank, strikes INTO v_current_rank, v_strikes
    FROM agent_personalities
    WHERE id = p_agent_id;

    -- Demotion occurs at 3 strikes
    IF v_strikes >= 3 AND v_current_rank > 0 THEN
        -- Demote: rank down, clear strikes, keep points
        UPDATE agent_personalities
        SET current_rank = current_rank - 1,
            strikes = 0
        WHERE id = p_agent_id;

        RETURN QUERY SELECT true, v_current_rank - 1;
    ELSE
        RETURN QUERY SELECT false, v_current_rank;
    END IF;
END;
$$ LANGUAGE plpgsql;
