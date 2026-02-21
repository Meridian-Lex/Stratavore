package parsers

import (
	"testing"
)

func TestParseRankStatus_ValidJSON(t *testing.T) {
	json := `{
  "current_rank": "Ensign",
  "progress_toward_next": "2/5",
  "next_rank": "Lieutenant (JG)",
  "strikes": 2,
  "strike_history": [
    {
      "strike": 1,
      "date": "2026-02-07",
      "infraction": "Test infraction 1",
      "consequence": "Warning"
    },
    {
      "strike": 2,
      "date": "2026-02-08",
      "infraction": "Test infraction 2",
      "evidence": "PR #123",
      "consequence": "Strike 2"
    },
    {
      "note": "Promoted to Ensign",
      "date": "2026-02-10"
    }
  ],
  "commendations": [
    {
      "date": "2026-02-09",
      "achievement": "Good work",
      "details": "Completed task successfully",
      "awarded_by": "Fleet Admiral",
      "points": 1
    }
  ],
  "rank_history": [
    {
      "rank": "Unranked",
      "achieved": "2026-02-06",
      "reason": "Starting rank"
    },
    {
      "rank": "Ensign",
      "achieved": "2026-02-10",
      "reason": "Promoted"
    }
  ],
  "last_updated": "2026-02-10T12:00:00Z"
}`

	rankStatus, err := ParseRankStatus(json)
	if err != nil {
		t.Fatalf("ParseRankStatus failed: %v", err)
	}

	// Verify basic fields
	if rankStatus.CurrentRank != "Ensign" {
		t.Errorf("Expected current_rank 'Ensign', got '%s'", rankStatus.CurrentRank)
	}
	if rankStatus.ProgressTowardNext != "2/5" {
		t.Errorf("Expected progress '2/5', got '%s'", rankStatus.ProgressTowardNext)
	}
	if rankStatus.Strikes != 2 {
		t.Errorf("Expected 2 strikes, got %d", rankStatus.Strikes)
	}

	// Verify strike history
	if len(rankStatus.StrikeHistory) != 3 {
		t.Fatalf("Expected 3 strike history entries, got %d", len(rankStatus.StrikeHistory))
	}

	if rankStatus.StrikeHistory[0].Strike != 1 {
		t.Errorf("Expected strike 1, got %d", rankStatus.StrikeHistory[0].Strike)
	}
	if rankStatus.StrikeHistory[2].Note != "Promoted to Ensign" {
		t.Errorf("Expected note 'Promoted to Ensign', got '%s'", rankStatus.StrikeHistory[2].Note)
	}

	// Verify commendations
	if len(rankStatus.Commendations) != 1 {
		t.Fatalf("Expected 1 commendation, got %d", len(rankStatus.Commendations))
	}
	if rankStatus.Commendations[0].Points != 1 {
		t.Errorf("Expected 1 point, got %d", rankStatus.Commendations[0].Points)
	}

	// Verify rank history
	if len(rankStatus.RankHistory) != 2 {
		t.Fatalf("Expected 2 rank history entries, got %d", len(rankStatus.RankHistory))
	}
}

func TestParseRankStatus_EmptyJSON(t *testing.T) {
	json := `{}`

	rankStatus, err := ParseRankStatus(json)
	if err != nil {
		t.Fatalf("ParseRankStatus failed on empty JSON: %v", err)
	}

	if rankStatus.CurrentRank != "" {
		t.Errorf("Expected empty current_rank, got '%s'", rankStatus.CurrentRank)
	}
	if rankStatus.Strikes != 0 {
		t.Errorf("Expected 0 strikes, got %d", rankStatus.Strikes)
	}
}

func TestParseRankStatus_InvalidJSON(t *testing.T) {
	json := `{"current_rank": invalid json`

	_, err := ParseRankStatus(json)
	if err == nil {
		t.Fatal("Expected error for invalid JSON, got nil")
	}
}

func TestGetRankEvents(t *testing.T) {
	json := `{
  "current_rank": "Ensign",
  "progress_toward_next": "2/5",
  "next_rank": "Lieutenant (JG)",
  "strikes": 1,
  "strike_history": [
    {
      "strike": 1,
      "date": "2026-02-07",
      "infraction": "Test violation",
      "consequence": "Warning"
    },
    {
      "note": "Promoted to Ensign",
      "date": "2026-02-10"
    }
  ],
  "commendations": [
    {
      "date": "2026-02-09",
      "achievement": "Excellence",
      "details": "Great work",
      "awarded_by": "Admiral",
      "points": 2
    }
  ],
  "rank_history": [],
  "last_updated": "2026-02-10T12:00:00Z"
}`

	rankStatus, err := ParseRankStatus(json)
	if err != nil {
		t.Fatalf("ParseRankStatus failed: %v", err)
	}

	events := rankStatus.GetRankEvents()

	// Should have: 1 strike + 1 promotion note + 1 commendation = 3 events
	if len(events) != 3 {
		t.Fatalf("Expected 3 rank events, got %d", len(events))
	}

	// Verify event types
	strikeFound := false
	promotionFound := false
	commendationFound := false

	for _, event := range events {
		switch event.Type {
		case "strike":
			strikeFound = true
		case "promotion":
			promotionFound = true
		case "commendation":
			commendationFound = true
		}
	}

	if !strikeFound {
		t.Error("Expected to find strike event")
	}
	if !promotionFound {
		t.Error("Expected to find promotion event")
	}
	if !commendationFound {
		t.Error("Expected to find commendation event")
	}
}

func TestParseRankStatus_RealWorldExample(t *testing.T) {
	// Simplified version of actual rank-status.jsonl
	json := `{
  "current_rank": "Ensign",
  "progress_toward_next": "2/5",
  "next_rank": "Lieutenant (JG)",
  "strikes": 2,
  "strike_history": [
    {
      "strike": 1,
      "date": "2026-02-07",
      "infraction": "Direct edit to lex system files",
      "consequence": "Warning issued"
    },
    {
      "note": "Promoted to Ensign (0 strikes carried forward)",
      "date": "2026-02-13"
    },
    {
      "strike": 1,
      "date": "2026-02-13",
      "infraction": "Merged PR without heart react approval",
      "evidence": "Stratavore PR #5",
      "consequence": "Strike 1 issued"
    }
  ],
  "commendations": [
    {
      "date": "2026-02-07",
      "achievement": "Comprehensive UI enhancements",
      "details": "Implemented search, filters, error handling",
      "awarded_by": "Fleet Admiral Lunar Laurus",
      "points": 1
    },
    {
      "date": "2026-02-12",
      "achievement": "Clean Stratavore acquisition",
      "details": "Forked, configured remotes, authored project documentation",
      "awarded_by": "Fleet Admiral Lunar Laurus",
      "points": 1
    }
  ],
  "rank_history": [
    {
      "rank": "Unranked",
      "achieved": "2026-02-07",
      "reason": "Starting rank"
    },
    {
      "rank": "Ensign",
      "achieved": "2026-02-13",
      "reason": "5 commendation points accumulated"
    }
  ],
  "last_updated": "2026-02-20T02:00:00Z"
}`

	rankStatus, err := ParseRankStatus(json)
	if err != nil {
		t.Fatalf("ParseRankStatus failed on real-world example: %v", err)
	}

	if rankStatus.CurrentRank != "Ensign" {
		t.Errorf("Expected Ensign rank, got '%s'", rankStatus.CurrentRank)
	}

	if rankStatus.Strikes != 2 {
		t.Errorf("Expected 2 strikes, got %d", rankStatus.Strikes)
	}

	if len(rankStatus.Commendations) != 2 {
		t.Errorf("Expected 2 commendations, got %d", len(rankStatus.Commendations))
	}

	// Verify GetRankEvents processes correctly
	events := rankStatus.GetRankEvents()
	if len(events) < 3 { // At least 2 strikes + 2 commendations
		t.Errorf("Expected at least 4 events, got %d", len(events))
	}
}
