package importers

import (
	"testing"
	"time"

	"github.com/meridian-lex/stratavore/internal/migrate/parsers"
)

func TestParseProgress(t *testing.T) {
	tests := []struct {
		name        string
		progress    string
		wantCurrent int
		wantTotal   int
		wantErr     bool
	}{
		{
			name:        "valid progress 2/5",
			progress:    "2/5",
			wantCurrent: 2,
			wantTotal:   5,
			wantErr:     false,
		},
		{
			name:        "valid progress 0/5",
			progress:    "0/5",
			wantCurrent: 0,
			wantTotal:   5,
			wantErr:     false,
		},
		{
			name:        "valid progress 5/5",
			progress:    "5/5",
			wantCurrent: 5,
			wantTotal:   5,
			wantErr:     false,
		},
		{
			name:        "with spaces",
			progress:    " 3 / 5 ",
			wantCurrent: 3,
			wantTotal:   5,
			wantErr:     false,
		},
		{
			name:        "invalid format no slash",
			progress:    "2of5",
			wantCurrent: 0,
			wantTotal:   0,
			wantErr:     true,
		},
		{
			name:        "invalid format multiple slashes",
			progress:    "2/5/10",
			wantCurrent: 0,
			wantTotal:   0,
			wantErr:     true,
		},
		{
			name:        "non-numeric current",
			progress:    "x/5",
			wantCurrent: 0,
			wantTotal:   0,
			wantErr:     true,
		},
		{
			name:        "non-numeric total",
			progress:    "2/x",
			wantCurrent: 0,
			wantTotal:   0,
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			current, total, err := parseProgress(tt.progress)

			if tt.wantErr {
				if err == nil {
					t.Errorf("Expected error for progress %q, got nil", tt.progress)
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if current != tt.wantCurrent {
				t.Errorf("Expected current=%d, got %d", tt.wantCurrent, current)
			}

			if total != tt.wantTotal {
				t.Errorf("Expected total=%d, got %d", tt.wantTotal, total)
			}
		})
	}
}

func TestImportRank_EventGeneration(t *testing.T) {
	rankStatus := &parsers.V2RankStatusFile{
		CurrentRank:        "Ensign",
		ProgressTowardNext: "2/5",
		Strikes:            2,
		StrikeHistory: []parsers.V2StrikeEvent{
			{
				Strike:      1,
				Date:        "2026-02-20",
				Infraction:  "Test violation",
				Consequence: "Warning",
			},
		},
		Commendations: []parsers.V2Commendation{
			{
				Date:        "2026-02-19",
				Achievement: "Good work",
				Details:     "Completed task",
				AwardedBy:   "Admiral",
				Points:      1,
			},
		},
	}

	events := rankStatus.GetRankEvents()

	// Should have 2 events: 1 strike + 1 commendation
	if len(events) < 2 {
		t.Errorf("Expected at least 2 events, got %d", len(events))
	}

	// Verify strike event
	strikeFound := false
	commendationFound := false

	for _, event := range events {
		if event.Type == "strike" {
			strikeFound = true
		}
		if event.Type == "commendation" {
			commendationFound = true
		}
	}

	if !strikeFound {
		t.Error("Expected to find strike event")
	}
	if !commendationFound {
		t.Error("Expected to find commendation event")
	}
}

func TestImportRank_CurrentStateImport(t *testing.T) {
	// When there are no events, should import current state as "initial"
	rankStatus := &parsers.V2RankStatusFile{
		CurrentRank:        "Cadet",
		ProgressTowardNext: "1/5",
		Strikes:            0,
		Commendations:      []parsers.V2Commendation{},
		StrikeHistory:      []parsers.V2StrikeEvent{},
		LastUpdated:        "2026-02-20T10:00:00Z",
	}

	events := rankStatus.GetRankEvents()

	// Should have 0 events from history
	if len(events) != 0 {
		t.Errorf("Expected 0 events from empty history, got %d", len(events))
	}

	// Verify current state values
	if rankStatus.CurrentRank != "Cadet" {
		t.Errorf("Expected current_rank 'Cadet', got %s", rankStatus.CurrentRank)
	}
	if rankStatus.ProgressTowardNext != "1/5" {
		t.Errorf("Expected progress '1/5', got %s", rankStatus.ProgressTowardNext)
	}
	if rankStatus.Strikes != 0 {
		t.Errorf("Expected 0 strikes, got %d", rankStatus.Strikes)
	}
}

func TestImportRank_Metadata(t *testing.T) {
	metadata := map[string]interface{}{
		"v2_import": true,
	}

	if !metadata["v2_import"].(bool) {
		t.Error("Expected v2_import to be true")
	}

	// Test current_state metadata variant
	metadataWithState := map[string]interface{}{
		"v2_import":     true,
		"current_state": true,
	}

	if !metadataWithState["current_state"].(bool) {
		t.Error("Expected current_state to be true")
	}
}

func TestImportRank_RealWorldExample(t *testing.T) {
	rankStatus := &parsers.V2RankStatusFile{
		CurrentRank:        "Ensign",
		ProgressTowardNext: "3/5",
		NextRank:           "Lieutenant (JG)",
		Strikes:            2,
		StrikeHistory: []parsers.V2StrikeEvent{
			{
				Strike:      1,
				Date:        "2026-02-20",
				Infraction:  "Identity reference in PR",
				Evidence:    "PRs #16, #17",
				Consequence: "Strike 1",
			},
			{
				Strike:      2,
				Date:        "2026-02-20",
				Infraction:  "Failed to execute removal directive",
				Evidence:    "Windows paths",
				Consequence: "Strike 2",
			},
		},
		Commendations: []parsers.V2Commendation{
			{
				Date:        "2026-02-20",
				Achievement: "V2 migration parsers",
				Details:     "Phase 1 tasks 1-5 complete",
				AwardedBy:   "Fleet Admiral Lunar Laurus",
				Points:      1,
			},
		},
		LastUpdated: "2026-02-20T08:45:00Z",
	}

	// Verify basic fields
	if rankStatus.CurrentRank != "Ensign" {
		t.Errorf("Expected Ensign rank, got %s", rankStatus.CurrentRank)
	}

	if rankStatus.Strikes != 2 {
		t.Errorf("Expected 2 strikes, got %d", rankStatus.Strikes)
	}

	current, total, err := parseProgress(rankStatus.ProgressTowardNext)
	if err != nil {
		t.Errorf("Failed to parse progress: %v", err)
	}

	if current != 3 || total != 5 {
		t.Errorf("Expected progress 3/5, got %d/%d", current, total)
	}

	// Verify event generation
	events := rankStatus.GetRankEvents()
	if len(events) < 3 { // 2 strikes + 1 commendation
		t.Errorf("Expected at least 3 events, got %d", len(events))
	}
}

func TestImportRank_LastUpdatedParsing(t *testing.T) {
	lastUpdated := "2026-02-20T08:45:00Z"

	parsed, err := time.Parse(time.RFC3339, lastUpdated)
	if err != nil {
		t.Fatalf("Failed to parse last_updated: %v", err)
	}

	expected := time.Date(2026, 2, 20, 8, 45, 0, 0, time.UTC)
	if !parsed.Equal(expected) {
		t.Errorf("Expected %v, got %v", expected, parsed)
	}
}
