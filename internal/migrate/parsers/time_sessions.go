package parsers

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// V2TimeSession represents the actual JSONL format from V2 time_sessions.jsonl
type V2TimeSession struct {
	SessionID       string      `json:"session_id"`
	JobID           string      `json:"job_id"` // Maps to project_name
	Agent           string      `json:"agent"`
	Description     string      `json:"description"`
	Status          string      `json:"status"` // "completed" | "cancelled" | "running"
	StartTime       string      `json:"start_time"`
	StartTimestamp  float64     `json:"start_timestamp"`
	EndTime         string      `json:"end_time"`
	EndTimestamp    *float64    `json:"end_timestamp"` // Can be null
	DurationSeconds *float64    `json:"duration_seconds"` // Can be null
	PausedTime      float64     `json:"paused_time"` // In seconds
	Pauses          []V2Pause   `json:"pauses"`
	Notes           string      `json:"notes"`
	CreatedAt       string      `json:"created_at"`
}

// V2Pause represents a pause event in a session
type V2Pause struct {
	PauseStart float64 `json:"pause_start"`
	PauseEnd   float64 `json:"pause_end"`
}

// ToV2Session converts V2TimeSession to the simplified V2Session format
func (v *V2TimeSession) ToV2Session() (V2Session, error) {
	// Parse start time
	startTime, err := time.Parse(time.RFC3339, v.StartTime)
	if err != nil {
		return V2Session{}, fmt.Errorf("parse start time: %w", err)
	}

	// Parse end time (may be empty or null)
	var endTime time.Time
	if v.EndTime != "" {
		endTime, err = time.Parse(time.RFC3339, v.EndTime)
		if err != nil {
			// Use zero time if parse fails
			endTime = time.Time{}
		}
	}

	// Calculate tokens used (not stored in V2 JSONL, will be 0)
	var tokensUsed int64 = 0

	return V2Session{
		SessionID:   v.SessionID,
		ProjectName: v.JobID, // job_id maps to project name
		StartTime:   startTime,
		EndTime:     endTime,
		PausedTime:  int64(v.PausedTime), // Convert float seconds to int
		TokensUsed:  tokensUsed,
		Summary:     v.Notes,
	}, nil
}

// ParseTimeSessions reads and parses a time_sessions.jsonl file
func ParseTimeSessions(jsonlPath string) ([]V2Session, error) {
	file, err := os.Open(jsonlPath)
	if err != nil {
		return nil, fmt.Errorf("open file: %w", err)
	}
	defer file.Close()

	var sessions []V2Session
	scanner := bufio.NewScanner(file)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		// Skip empty lines
		if line == "" {
			continue
		}

		var timeSession V2TimeSession
		if err := json.Unmarshal([]byte(line), &timeSession); err != nil {
			return nil, fmt.Errorf("parse line %d: %w", lineNum, err)
		}

		session, err := timeSession.ToV2Session()
		if err != nil {
			return nil, fmt.Errorf("convert session at line %d: %w", lineNum, err)
		}

		sessions = append(sessions, session)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}

	return sessions, nil
}

// ParseTimeSessionsContent parses JSONL content directly (for testing)
func ParseTimeSessionsContent(jsonlContent string) ([]V2Session, error) {
	var sessions []V2Session
	lineNum := 0

	for _, line := range splitLines(jsonlContent) {
		lineNum++
		line = trimSpace(line)

		// Skip empty lines
		if line == "" {
			continue
		}

		var timeSession V2TimeSession
		if err := json.Unmarshal([]byte(line), &timeSession); err != nil {
			return nil, fmt.Errorf("parse line %d: %w", lineNum, err)
		}

		session, err := timeSession.ToV2Session()
		if err != nil {
			return nil, fmt.Errorf("convert session at line %d: %w", lineNum, err)
		}

		sessions = append(sessions, session)
	}

	return sessions, nil
}
