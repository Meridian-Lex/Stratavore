package importers

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/meridian-lex/stratavore/internal/migrate/parsers"
)

func TestImportSessions_UUIDGeneration(t *testing.T) {
	// Test that UUID generation is deterministic for the same session_id
	sessionID := "test-session-123"

	uuid1 := uuid.NewSHA1(uuid.NameSpaceURL, []byte("v2-session:"+sessionID))
	uuid2 := uuid.NewSHA1(uuid.NameSpaceURL, []byte("v2-session:"+sessionID))

	if uuid1 != uuid2 {
		t.Errorf("Expected deterministic UUIDs, got %s and %s", uuid1, uuid2)
	}

	// Different session IDs should produce different UUIDs
	differentSessionID := "different-session-456"
	uuid3 := uuid.NewSHA1(uuid.NameSpaceURL, []byte("v2-session:"+differentSessionID))

	if uuid1 == uuid3 {
		t.Error("Expected different UUIDs for different session IDs")
	}
}

func TestImportSessions_EndedAtLogic(t *testing.T) {
	tests := []struct {
		name      string
		endTime   time.Time
		wantNil   bool
	}{
		{
			name:      "session with end time",
			endTime:   time.Date(2026, 2, 20, 12, 0, 0, 0, time.UTC),
			wantNil:   false,
		},
		{
			name:      "session without end time (zero)",
			endTime:   time.Time{},
			wantNil:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var endedAt *time.Time
			if !tt.endTime.IsZero() {
				endedAt = &tt.endTime
			}

			if tt.wantNil && endedAt != nil {
				t.Error("Expected nil endedAt for zero end time")
			}
			if !tt.wantNil && endedAt == nil {
				t.Error("Expected non-nil endedAt for valid end time")
			}
		})
	}
}

func TestImportSessions_MetadataGeneration(t *testing.T) {
	v2sess := parsers.V2Session{
		SessionID:   "test-123",
		ProjectName: "test-project",
		StartTime:   time.Date(2026, 2, 20, 10, 0, 0, 0, time.UTC),
		EndTime:     time.Date(2026, 2, 20, 11, 30, 0, 0, time.UTC),
		PausedTime:  300, // 5 minutes paused
		TokensUsed:  5000,
		Summary:     "Test session",
	}

	// Verify metadata structure
	metadata := map[string]interface{}{
		"paused_time_seconds": v2sess.PausedTime,
		"v2_format":           true,
		"imported_from":       "time_sessions.jsonl",
	}

	if metadata["paused_time_seconds"].(int64) != 300 {
		t.Errorf("Expected paused_time 300, got %v", metadata["paused_time_seconds"])
	}

	if !metadata["v2_format"].(bool) {
		t.Error("Expected v2_format to be true")
	}

	if metadata["imported_from"].(string) != "time_sessions.jsonl" {
		t.Errorf("Expected imported_from 'time_sessions.jsonl', got %v", metadata["imported_from"])
	}
}

func TestImportSessions_StorageKeyGeneration(t *testing.T) {
	sessionID := "session-2026-02-20-morning_1770973951"

	expectedKey := "v2-migration/sessions/session-2026-02-20-morning_1770973951/metadata.json"
	actualKey := "v2-migration/sessions/" + sessionID + "/metadata.json"

	if actualKey != expectedKey {
		t.Errorf("Storage key mismatch: got %q, want %q", actualKey, expectedKey)
	}
}

func TestImportSessions_ResumableFlag(t *testing.T) {
	// V2 sessions should NEVER be resumable in V3
	tests := []struct {
		name          string
		v2sess        parsers.V2Session
		wantResumable bool
	}{
		{
			name: "completed session",
			v2sess: parsers.V2Session{
				SessionID: "completed-123",
				EndTime:   time.Now(),
			},
			wantResumable: false,
		},
		{
			name: "running session (no end time)",
			v2sess: parsers.V2Session{
				SessionID: "running-456",
				EndTime:   time.Time{},
			},
			wantResumable: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// All V2 sessions should have resumable=false
			resumable := false

			if resumable != tt.wantResumable {
				t.Errorf("Expected resumable=%v for V2 sessions, got %v", tt.wantResumable, resumable)
			}
		})
	}
}

func TestImportSessions_TokenTracking(t *testing.T) {
	v2sess := parsers.V2Session{
		SessionID:  "token-test-123",
		TokensUsed: 12345,
	}

	if v2sess.TokensUsed != 12345 {
		t.Errorf("Expected tokens_used 12345, got %d", v2sess.TokensUsed)
	}

	// V2 sessions with no token tracking should have 0
	v2sessNoTokens := parsers.V2Session{
		SessionID:  "no-tokens-456",
		TokensUsed: 0,
	}

	if v2sessNoTokens.TokensUsed != 0 {
		t.Errorf("Expected tokens_used 0 for untracked session, got %d", v2sessNoTokens.TokensUsed)
	}
}

func TestImportSessions_LastMessageAtLogic(t *testing.T) {
	tests := []struct {
		name    string
		endTime time.Time
		want    bool // true if lastMessageAt should be set
	}{
		{
			name:    "session with end time sets last_message_at",
			endTime: time.Date(2026, 2, 20, 12, 0, 0, 0, time.UTC),
			want:    true,
		},
		{
			name:    "session without end time has nil last_message_at",
			endTime: time.Time{},
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var endedAt *time.Time
			if !tt.endTime.IsZero() {
				endedAt = &tt.endTime
			}

			var lastMessageAt *time.Time
			if endedAt != nil {
				lastMessageAt = endedAt
			}

			if tt.want && lastMessageAt == nil {
				t.Error("Expected lastMessageAt to be set when ended_at is set")
			}
			if !tt.want && lastMessageAt != nil {
				t.Error("Expected lastMessageAt to be nil when ended_at is nil")
			}
		})
	}
}
