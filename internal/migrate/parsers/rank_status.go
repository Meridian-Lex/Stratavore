package parsers

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// V2RankStatusFile represents the complete rank-status.jsonl file structure
// Note: Despite the .jsonl extension, this file contains a single JSON object
type V2RankStatusFile struct {
	CurrentRank       string             `json:"current_rank"`
	ProgressTowardNext string            `json:"progress_toward_next"` // e.g., "2/5"
	NextRank          string             `json:"next_rank"`
	Strikes           int                `json:"strikes"`
	StrikeHistory     []V2StrikeEvent    `json:"strike_history"`
	Commendations     []V2Commendation   `json:"commendations"`
	RankHistory       []V2RankHistoryEvent `json:"rank_history"`
	LastUpdated       string             `json:"last_updated"`
}

// V2StrikeEvent represents a strike or note in the strike history
type V2StrikeEvent struct {
	Strike      int    `json:"strike,omitempty"` // Strike number (1, 2, 3) or 0 for notes
	Date        string `json:"date"`
	Infraction  string `json:"infraction,omitempty"`
	Evidence    string `json:"evidence,omitempty"`
	Consequence string `json:"consequence,omitempty"`
	Note        string `json:"note,omitempty"` // For non-strike entries (promotions, etc.)
}

// V2Commendation represents a commendation entry
type V2Commendation struct {
	Date       string `json:"date"`
	Achievement string `json:"achievement,omitempty"`
	Details    string `json:"details"`
	AwardedBy  string `json:"awarded_by"`
	Points     int    `json:"points"`
	Reason     string `json:"reason,omitempty"` // Alternative to achievement
}

// V2RankHistoryEvent represents a rank change event
type V2RankHistoryEvent struct {
	Rank     string `json:"rank"`
	Achieved string `json:"achieved"`
	Lost     string `json:"lost,omitempty"`
	Reason   string `json:"reason,omitempty"`
	Note     string `json:"note,omitempty"`
}

// ParseRankStatus parses rank-status.jsonl and extracts all rank tracking data
func ParseRankStatus(jsonContent string) (*V2RankStatusFile, error) {
	var rankStatus V2RankStatusFile

	err := json.Unmarshal([]byte(jsonContent), &rankStatus)
	if err != nil {
		return nil, fmt.Errorf("unmarshal JSON: %w", err)
	}

	return &rankStatus, nil
}

// ParseRankStatusFile reads and parses a rank-status.jsonl file
func ParseRankStatusFile(filePath string) (*V2RankStatusFile, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}

	// Strip comment lines (starting with #) which are present in the file
	cleaned := stripComments(string(content))

	return ParseRankStatus(cleaned)
}

// stripComments removes lines starting with # from JSON content
func stripComments(content string) string {
	lines := splitLines(content)
	var result []byte

	for _, line := range lines {
		trimmed := trimSpace(line)
		if trimmed == "" || trimmed[0] == '#' {
			continue // Skip comment and empty lines
		}
		result = append(result, []byte(line)...)
		result = append(result, '\n')
	}

	return string(result)
}

// GetRankEvents converts the rank status file into a flat list of rank_tracking events
func (r *V2RankStatusFile) GetRankEvents() []V2RankEvent {
	var events []V2RankEvent

	// Add strike events
	for _, strike := range r.StrikeHistory {
		// Parse date
		date, err := time.Parse("2006-01-02", strike.Date)
		if err != nil {
			// Try RFC3339 format
			date, err = time.Parse(time.RFC3339, strike.Date)
			if err != nil {
				// Use zero time if parse fails
				date = time.Time{}
			}
		}

		if strike.Strike > 0 {
			// This is an actual strike
			events = append(events, V2RankEvent{
				Type:        "strike",
				Date:        date,
				Description: fmt.Sprintf("Strike %d: %s", strike.Strike, strike.Infraction),
				Evidence:    strike.Evidence,
			})
		} else if strike.Note != "" {
			// This is a note (promotion, demotion, etc.)
			// Determine type from note content
			eventType := "note"
			if contains(strike.Note, "Promoted") {
				eventType = "promotion"
			} else if contains(strike.Note, "Demoted") || contains(strike.Note, "Demotion") {
				eventType = "demotion"
			}

			events = append(events, V2RankEvent{
				Type:        eventType,
				Date:        date,
				Description: strike.Note,
				Evidence:    "",
			})
		}
	}

	// Add commendations as events
	for _, comm := range r.Commendations {
		date, err := time.Parse("2006-01-02", comm.Date)
		if err != nil {
			date = time.Time{}
		}

		achievement := comm.Achievement
		if achievement == "" {
			achievement = comm.Reason
		}

		events = append(events, V2RankEvent{
			Type:        "commendation",
			Date:        date,
			Description: fmt.Sprintf("%s (%d points)", achievement, comm.Points),
			Evidence:    comm.Details,
		})
	}

	return events
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && s != "" && substr != "" &&
		(s == substr || findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		match := true
		for j := 0; j < len(substr); j++ {
			if s[i+j] != substr[j] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}
