package parsers

import (
	"testing"
	"time"
)

func TestParseTimeSessions_ValidJSONL(t *testing.T) {
	jsonl := `{"session_id": "test-1", "job_id": "project-a", "agent": "lex", "description": "test", "status": "completed", "start_time": "2026-02-12T10:00:00Z", "start_timestamp": 1770900000, "end_time": "2026-02-12T11:00:00Z", "end_timestamp": 1770903600, "duration_seconds": 3600, "paused_time": 0, "pauses": [], "notes": "Done", "created_at": "2026-02-12T10:00:00Z"}
{"session_id": "test-2", "job_id": "project-b", "agent": "lex", "description": "test2", "status": "completed", "start_time": "2026-02-12T12:00:00Z", "start_timestamp": 1770907200, "end_time": "2026-02-12T13:30:00Z", "end_timestamp": 1770912600, "duration_seconds": 5400, "paused_time": 300, "pauses": [], "notes": "", "created_at": "2026-02-12T12:00:00Z"}
`

	sessions, err := ParseTimeSessionsContent(jsonl)
	if err != nil {
		t.Fatalf("ParseTimeSessionsContent failed: %v", err)
	}

	if len(sessions) != 2 {
		t.Fatalf("Expected 2 sessions, got %d", len(sessions))
	}

	// Verify first session
	if sessions[0].SessionID != "test-1" {
		t.Errorf("Expected session_id 'test-1', got '%s'", sessions[0].SessionID)
	}
	if sessions[0].ProjectName != "project-a" {
		t.Errorf("Expected project_name 'project-a', got '%s'", sessions[0].ProjectName)
	}
	if sessions[0].Summary != "Done" {
		t.Errorf("Expected summary 'Done', got '%s'", sessions[0].Summary)
	}
	if sessions[0].PausedTime != 0 {
		t.Errorf("Expected paused_time 0, got %d", sessions[0].PausedTime)
	}

	// Verify second session
	if sessions[1].SessionID != "test-2" {
		t.Errorf("Expected session_id 'test-2', got '%s'", sessions[1].SessionID)
	}
	if sessions[1].PausedTime != 300 {
		t.Errorf("Expected paused_time 300, got %d", sessions[1].PausedTime)
	}
}

func TestParseTimeSessions_EmptyFile(t *testing.T) {
	sessions, err := ParseTimeSessionsContent("")
	if err != nil {
		t.Fatalf("ParseTimeSessionsContent failed on empty content: %v", err)
	}

	if len(sessions) != 0 {
		t.Errorf("Expected 0 sessions from empty content, got %d", len(sessions))
	}
}

func TestParseTimeSessions_WithPauses(t *testing.T) {
	jsonl := `{"session_id": "paused-session", "job_id": "test-proj", "agent": "lex", "description": "test", "status": "completed", "start_time": "2026-02-12T10:00:00Z", "start_timestamp": 1770900000, "end_time": "2026-02-12T11:00:00Z", "end_timestamp": 1770903600, "duration_seconds": 3300, "paused_time": 300, "pauses": [{"pause_start": 1770901000, "pause_end": 1770901300}], "notes": "", "created_at": "2026-02-12T10:00:00Z"}
`

	sessions, err := ParseTimeSessionsContent(jsonl)
	if err != nil {
		t.Fatalf("ParseTimeSessionsContent failed: %v", err)
	}

	if len(sessions) != 1 {
		t.Fatalf("Expected 1 session, got %d", len(sessions))
	}

	// Verify paused time was captured
	if sessions[0].PausedTime != 300 {
		t.Errorf("Expected paused_time 300 seconds, got %d", sessions[0].PausedTime)
	}
}

func TestParseTimeSessions_NullEndTime(t *testing.T) {
	jsonl := `{"session_id": "running-session", "job_id": "test-proj", "agent": "lex", "description": "still running", "status": "running", "start_time": "2026-02-12T10:00:00Z", "start_timestamp": 1770900000, "end_time": "", "end_timestamp": null, "duration_seconds": null, "paused_time": 0, "pauses": [], "notes": "", "created_at": "2026-02-12T10:00:00Z"}
`

	sessions, err := ParseTimeSessionsContent(jsonl)
	if err != nil {
		t.Fatalf("ParseTimeSessionsContent failed: %v", err)
	}

	if len(sessions) != 1 {
		t.Fatalf("Expected 1 session, got %d", len(sessions))
	}

	// Verify end time is zero value
	if !sessions[0].EndTime.IsZero() {
		t.Errorf("Expected zero EndTime for running session, got %v", sessions[0].EndTime)
	}
}

func TestParseTimeSessions_InvalidJSON(t *testing.T) {
	jsonl := `{"session_id": "test-1", "job_id": "project-a", invalid json here`

	_, err := ParseTimeSessionsContent(jsonl)
	if err == nil {
		t.Fatal("Expected error for invalid JSON, got nil")
	}
}

func TestParseTimeSessions_RealWorldExample(t *testing.T) {
	// This is actual data from the real time_sessions.jsonl file
	jsonl := `{"session_id": "test-job_1770927124", "job_id": "test-job", "agent": "meridian-lex", "description": "smoke test", "status": "cancelled", "start_time": "2026-02-12T20:12:04.687585Z", "start_timestamp": 1770927124.6875756, "end_time": "2026-02-12T20:12:23.278493Z", "end_timestamp": null, "duration_seconds": null, "paused_time": 0, "pauses": [], "notes": "", "created_at": "2026-02-12T20:12:04.687608Z"}
{"session_id": "test-job2_1770927133", "job_id": "test-job2", "agent": "meridian-lex", "description": "smoke test", "status": "completed", "start_time": "2026-02-12T20:12:13.196119Z", "start_timestamp": 1770927133.1961093, "end_time": "2026-02-12T20:12:16.322972Z", "end_timestamp": 1770927136.3229623, "duration_seconds": 2.084792137145996, "paused_time": 1.0420608520507812, "pauses": [{"pause_start": 1770927134.2397175, "pause_end": 1770927135.2817783}], "notes": "test complete", "created_at": "2026-02-12T20:12:13.196135Z"}
{"session_id": "session-2026-02-13-morning_1770973951", "job_id": "session-2026-02-13-morning", "agent": "Meridian Lex", "description": "Morning startup — directive load, state confirmation, readiness for Task 30", "status": "completed", "start_time": "2026-02-13T09:12:31.400978Z", "start_timestamp": 1770973951.400969, "end_time": "2026-02-13T09:53:23.407547Z", "end_timestamp": 1770976403.4075372, "duration_seconds": 2452.0065681934357, "paused_time": 0, "pauses": [], "notes": "Task 30 Phase 1 complete — Lex-webui v3 Stratavore integration, PR #8 open", "created_at": "2026-02-13T09:12:31.400994Z"}
`

	sessions, err := ParseTimeSessionsContent(jsonl)
	if err != nil {
		t.Fatalf("ParseTimeSessionsContent failed on real-world data: %v", err)
	}

	if len(sessions) != 3 {
		t.Fatalf("Expected 3 sessions, got %d", len(sessions))
	}

	// Verify third session (the long morning session)
	morningSession := sessions[2]
	if morningSession.SessionID != "session-2026-02-13-morning_1770973951" {
		t.Errorf("Expected session_id for morning session, got '%s'", morningSession.SessionID)
	}
	if morningSession.ProjectName != "session-2026-02-13-morning" {
		t.Errorf("Expected project_name 'session-2026-02-13-morning', got '%s'", morningSession.ProjectName)
	}
	if morningSession.Summary != "Task 30 Phase 1 complete — Lex-webui v3 Stratavore integration, PR #8 open" {
		t.Errorf("Summary mismatch: got '%s'", morningSession.Summary)
	}

	// Verify start time parsing
	expectedStart := time.Date(2026, 2, 13, 9, 12, 31, 400978000, time.UTC)
	if !morningSession.StartTime.Equal(expectedStart) {
		t.Errorf("Expected start time %v, got %v", expectedStart, morningSession.StartTime)
	}

	// Verify second session has paused time
	if sessions[1].PausedTime != 1 { // Truncated from 1.042 to 1
		t.Errorf("Expected paused_time 1 second (truncated), got %d", sessions[1].PausedTime)
	}
}

func TestParseTimeSessions_BlankLines(t *testing.T) {
	jsonl := `{"session_id": "test-1", "job_id": "proj", "agent": "lex", "description": "", "status": "completed", "start_time": "2026-02-12T10:00:00Z", "start_timestamp": 1770900000, "end_time": "2026-02-12T11:00:00Z", "end_timestamp": 1770903600, "duration_seconds": 3600, "paused_time": 0, "pauses": [], "notes": "", "created_at": "2026-02-12T10:00:00Z"}

{"session_id": "test-2", "job_id": "proj2", "agent": "lex", "description": "", "status": "completed", "start_time": "2026-02-12T12:00:00Z", "start_timestamp": 1770907200, "end_time": "2026-02-12T13:00:00Z", "end_timestamp": 1770910800, "duration_seconds": 3600, "paused_time": 0, "pauses": [], "notes": "", "created_at": "2026-02-12T12:00:00Z"}
`

	sessions, err := ParseTimeSessionsContent(jsonl)
	if err != nil {
		t.Fatalf("ParseTimeSessionsContent failed with blank lines: %v", err)
	}

	if len(sessions) != 2 {
		t.Errorf("Expected 2 sessions (blank lines should be skipped), got %d", len(sessions))
	}
}
