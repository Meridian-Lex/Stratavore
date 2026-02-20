-- V2 Migration Support: Preserve Lex V2 state during transition to Stratavore V3
-- This migration adds tables for V2-specific data that doesn't fit the current schema:
-- rank progression, behavioral directives, and sync state tracking.

-- Rank tracking: Full history of rank progression, strikes, commendations
CREATE TABLE IF NOT EXISTS rank_tracking (
    id SERIAL PRIMARY KEY,
    current_rank TEXT NOT NULL,
    progress TEXT NOT NULL,           -- e.g., "2/5" (progress toward next rank)
    strikes INTEGER NOT NULL DEFAULT 0,
    commendations INTEGER NOT NULL DEFAULT 0,
    event_type TEXT NOT NULL          -- 'strike' | 'commendation' | 'promotion' | 'demotion' | 'initial' | 'note'
        CHECK (event_type IN ('strike', 'commendation', 'promotion', 'demotion', 'initial', 'note')),
    event_date TIMESTAMPTZ NOT NULL,
    description TEXT,
    evidence TEXT,                     -- File path, commit hash, or other reference
    metadata JSONB DEFAULT '{}',       -- Additional structured data
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Unique constraint to prevent duplicate events during sync
CREATE UNIQUE INDEX idx_rank_tracking_unique_event ON rank_tracking(event_type, event_date, COALESCE(description, ''));

CREATE INDEX idx_rank_tracking_event_date ON rank_tracking(event_date DESC);
CREATE INDEX idx_rank_tracking_event_type ON rank_tracking(event_type);
CREATE INDEX idx_rank_tracking_current_rank ON rank_tracking(current_rank);

-- Behavioral directives: Machine-readable operational rules from V2
CREATE TABLE IF NOT EXISTS directives (
    id TEXT PRIMARY KEY,               -- e.g., "IDENTITY-ENFORCEMENT", "GIT-SAFETY-001"
    severity TEXT NOT NULL             -- 'PRIME' | 'CRITICAL' | 'HIGH' | 'MEDIUM' | 'LOW' | 'STANDARD' | 'META'
        CHECK (severity IN ('PRIME', 'CRITICAL', 'HIGH', 'MEDIUM', 'LOW', 'STANDARD', 'META')),
    trigger_condition TEXT NOT NULL,   -- When this directive applies
    action JSONB NOT NULL,             -- What to do (structured action specification)
    directive_text TEXT NOT NULL,      -- Human-readable directive
    standard_process BOOLEAN DEFAULT false,  -- Is this a standard process/checklist?
    enabled BOOLEAN DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    last_triggered TIMESTAMPTZ
);

CREATE INDEX idx_directives_severity ON directives(severity);
CREATE INDEX idx_directives_enabled ON directives(enabled) WHERE enabled = true;

-- V2 sync state: Track last successful sync for each V2 file
CREATE TABLE IF NOT EXISTS v2_sync_state (
    file_path TEXT PRIMARY KEY,        -- V2 file path (e.g., "PROJECT-MAP.md")
    file_type TEXT NOT NULL            -- 'project_map' | 'time_sessions' | 'lex_config' | 'rank_status' | 'directives'
        CHECK (file_type IN ('project_map', 'time_sessions', 'lex_config', 'rank_status', 'directives')),
    last_sync_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_modified_at TIMESTAMPTZ,      -- File mtime at last sync
    records_synced INTEGER DEFAULT 0,
    sync_status TEXT NOT NULL DEFAULT 'success'
        CHECK (sync_status IN ('success', 'partial', 'failed')),
    error_message TEXT,
    checksum TEXT                       -- SHA256 of file content for change detection
);

CREATE INDEX idx_v2_sync_state_file_type ON v2_sync_state(file_type);
CREATE INDEX idx_v2_sync_state_last_sync ON v2_sync_state(last_sync_at DESC);

-- V2 import metadata: Track migration/import events
CREATE TABLE IF NOT EXISTS v2_import_log (
    id SERIAL PRIMARY KEY,
    import_type TEXT NOT NULL          -- 'full_migration' | 'incremental_sync' | 'manual_import'
        CHECK (import_type IN ('full_migration', 'incremental_sync', 'manual_import')),
    source_dir TEXT NOT NULL,          -- V2 lex-internal/state directory
    status TEXT NOT NULL               -- 'running' | 'completed' | 'failed' | 'rolled_back'
        CHECK (status IN ('running', 'completed', 'failed', 'rolled_back')),
    records_imported JSONB DEFAULT '{}', -- {"projects": 4, "sessions": 3, ...}
    validation_passed BOOLEAN DEFAULT false,
    snapshot_path TEXT,                -- pg_dump snapshot location for rollback
    error_message TEXT,
    started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ
);

CREATE INDEX idx_v2_import_log_status ON v2_import_log(status);
CREATE INDEX idx_v2_import_log_started ON v2_import_log(started_at DESC);
