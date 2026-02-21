package importers

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/meridian-lex/stratavore/internal/migrate/parsers"
)

// ImportSessions imports V2 sessions into the sessions and session_blobs tables
// V2 sessions are not resumable in V3, so resumable=false for all imports
// Returns the number of sessions processed
// Sessions without project_name are skipped (schema mismatch in test data)
func ImportSessions(ctx context.Context, tx pgx.Tx, v2Sessions []parsers.V2Session) (int, error) {
	runnerQuery := `
		INSERT INTO runners (
			id, runtime_type, runtime_id, project_name, project_path,
			status, conversation_mode, flags, environment,
			started_at, last_heartbeat, terminated_at,
			cpu_percent, memory_mb, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
		ON CONFLICT (id) DO NOTHING
	`

	sessionQuery := `
		INSERT INTO sessions (
			id, runner_id, project_name,
			started_at, ended_at, last_message_at,
			message_count, tokens_used,
			resumable, summary,
			created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		ON CONFLICT (id) DO UPDATE SET
			ended_at = EXCLUDED.ended_at,
			tokens_used = EXCLUDED.tokens_used,
			summary = EXCLUDED.summary
	`

	blobQuery := `
		INSERT INTO session_blobs (session_id, blob_type, storage_key, size_bytes, created_at)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT DO NOTHING
	`

	count := 0
	for _, v2sess := range v2Sessions {
		// Skip sessions with no project_name (test sessions or incomplete data)
		if v2sess.ProjectName == "" {
			fmt.Printf("  ⚠️  Skipping session %s: no project_name\n", v2sess.SessionID)
			continue
		}

		// Generate synthetic runner UUID for V2 sessions
		// Use deterministic UUID v5 based on session_id to ensure idempotency
		runnerID := uuid.NewSHA1(uuid.NameSpaceURL, []byte("v2-session:"+v2sess.SessionID))

		// Calculate duration and set ended_at
		var endedAt *time.Time
		if !v2sess.EndTime.IsZero() {
			endedAt = &v2sess.EndTime
		}

		// Create synthetic runner record for V2 session
		// V2 sessions have no runner metadata, so use minimal values
		var err error
		var terminatedAt *time.Time
		if endedAt != nil {
			terminatedAt = endedAt
		}

		_, err = tx.Exec(ctx, runnerQuery,
			runnerID,            // id
			"process",           // runtime_type
			"v2-synthetic",      // runtime_id
			v2sess.ProjectName,  // project_name
			"unknown",           // project_path (V2 doesn't track this)
			"terminated",        // status (V2 sessions are historical)
			"new",               // conversation_mode
			"[]",                // flags (empty JSON array)
			"{}",                // environment (empty JSON object)
			v2sess.StartTime,    // started_at
			v2sess.StartTime,    // last_heartbeat
			terminatedAt,        // terminated_at
			0.0,                 // cpu_percent
			int64(0),            // memory_mb
			v2sess.StartTime,    // created_at
		)

		if err != nil {
			return count, fmt.Errorf("create synthetic runner for session %s: %w", v2sess.SessionID, err)
		}

		// V2 sessions don't track message count, default to 0
		messageCount := 0

		// V2 sessions don't track last_message_at, use ended_at if available
		var lastMessageAt *time.Time
		if endedAt != nil {
			lastMessageAt = endedAt
		}

		// Insert session
		_, err = tx.Exec(ctx, sessionQuery,
			v2sess.SessionID,      // id
			runnerID,              // runner_id (synthetic)
			v2sess.ProjectName,    // project_name
			v2sess.StartTime,      // started_at
			endedAt,               // ended_at
			lastMessageAt,         // last_message_at
			messageCount,          // message_count
			v2sess.TokensUsed,     // tokens_used
			false,                 // resumable (V2 sessions not resumable in V3)
			v2sess.Summary,        // summary
			v2sess.StartTime,      // created_at (use start time)
		)

		if err != nil {
			return count, fmt.Errorf("import session %s: %w", v2sess.SessionID, err)
		}

		// Store V2 metadata as blob
		v2Metadata := map[string]interface{}{
			"paused_time_seconds": v2sess.PausedTime,
			"v2_format":           true,
			"imported_from":       "time_sessions.jsonl",
		}

		metadataJSON, err := json.Marshal(v2Metadata)
		if err != nil {
			return count, fmt.Errorf("marshal V2 metadata for session %s: %w", v2sess.SessionID, err)
		}

		storageKey := fmt.Sprintf("v2-migration/sessions/%s/metadata.json", v2sess.SessionID)
		sizeBytes := int64(len(metadataJSON))

		_, err = tx.Exec(ctx, blobQuery,
			v2sess.SessionID,  // session_id
			"v2_metadata",     // blob_type
			storageKey,        // storage_key
			sizeBytes,         // size_bytes
			v2sess.StartTime,  // created_at
		)

		if err != nil {
			return count, fmt.Errorf("import session blob for %s: %w", v2sess.SessionID, err)
		}
		count++
	}

	return count, nil
}
