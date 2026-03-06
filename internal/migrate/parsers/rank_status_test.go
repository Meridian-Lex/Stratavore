package parsers

import (
	"os"
	"path/filepath"
	"testing"
)

// writeTemp writes content to a temp file and returns the path.
func writeTemp(t *testing.T, name, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("writeTemp: %v", err)
	}
	return path
}

const statusJSON = `{
  "current_rank": "Ensign",
  "progress_toward_next": "2/5",
  "next_rank": "Lieutenant (JG)",
  "strikes": 2,
  "last_updated": "2026-02-10T12:00:00Z"
}`

const eventsJSONL = `{"type":"strike_event","strike":1,"date":"2026-02-07","infraction":"Test infraction 1","consequence":"Warning"}
{"type":"strike_event","strike":2,"date":"2026-02-08","infraction":"Test infraction 2","evidence":"PR #123","consequence":"Strike 2"}
{"type":"strike_event","note":"Promoted to Ensign","date":"2026-02-10"}
{"type":"commendation","date":"2026-02-09","achievement":"Good work","details":"Completed task successfully","awarded_by":"Fleet Admiral","points":1}
{"type":"rank_change","rank":"Unranked","achieved":"2026-02-06","reason":"Starting rank"}
{"type":"rank_change","rank":"Ensign","achieved":"2026-02-10","reason":"Promoted"}
`

func TestParseRankFiles_ValidInput(t *testing.T) {
	statusPath := writeTemp(t, "rank-status.json", statusJSON)
	eventsPath := writeTemp(t, "rank-events.jsonl", eventsJSONL)

	rs, err := ParseRankFiles(statusPath, eventsPath)
	if err != nil {
		t.Fatalf("ParseRankFiles failed: %v", err)
	}

	if rs.CurrentRank != "Ensign" {
		t.Errorf("expected current_rank 'Ensign', got '%s'", rs.CurrentRank)
	}
	if rs.ProgressTowardNext != "2/5" {
		t.Errorf("expected progress '2/5', got '%s'", rs.ProgressTowardNext)
	}
	if rs.Strikes != 2 {
		t.Errorf("expected 2 strikes, got %d", rs.Strikes)
	}
	if len(rs.StrikeHistory) != 3 {
		t.Fatalf("expected 3 strike_history entries, got %d", len(rs.StrikeHistory))
	}
	if rs.StrikeHistory[0].Strike != 1 {
		t.Errorf("expected strike 1, got %d", rs.StrikeHistory[0].Strike)
	}
	if rs.StrikeHistory[2].Note != "Promoted to Ensign" {
		t.Errorf("expected note 'Promoted to Ensign', got '%s'", rs.StrikeHistory[2].Note)
	}
	if len(rs.Commendations) != 1 {
		t.Fatalf("expected 1 commendation, got %d", len(rs.Commendations))
	}
	if rs.Commendations[0].Points != 1 {
		t.Errorf("expected 1 point, got %d", rs.Commendations[0].Points)
	}
	if len(rs.RankHistory) != 2 {
		t.Fatalf("expected 2 rank_history entries, got %d", len(rs.RankHistory))
	}
}

func TestParseRankFiles_EmptyEvents(t *testing.T) {
	statusPath := writeTemp(t, "rank-status.json", `{"current_rank":"Ensign","strikes":0}`)
	eventsPath := writeTemp(t, "rank-events.jsonl", "")

	rs, err := ParseRankFiles(statusPath, eventsPath)
	if err != nil {
		t.Fatalf("ParseRankFiles failed on empty events: %v", err)
	}
	if rs.CurrentRank != "Ensign" {
		t.Errorf("expected 'Ensign', got '%s'", rs.CurrentRank)
	}
	if len(rs.StrikeHistory) != 0 {
		t.Errorf("expected empty StrikeHistory")
	}
}

func TestParseRankFiles_InvalidStatusJSON(t *testing.T) {
	statusPath := writeTemp(t, "rank-status.json", `{"current_rank": invalid`)
	eventsPath := writeTemp(t, "rank-events.jsonl", "")

	_, err := ParseRankFiles(statusPath, eventsPath)
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

func TestParseRankFiles_InvalidEventsLine(t *testing.T) {
	statusPath := writeTemp(t, "rank-status.json", `{"current_rank":"Ensign","strikes":0}`)
	eventsPath := writeTemp(t, "rank-events.jsonl", `{"type":"commendation","points":1}
not valid json
`)

	_, err := ParseRankFiles(statusPath, eventsPath)
	if err == nil {
		t.Fatal("expected error for invalid JSONL line, got nil")
	}
}

func TestGetRankEvents(t *testing.T) {
	statusPath := writeTemp(t, "rank-status.json", statusJSON)
	eventsPath := writeTemp(t, "rank-events.jsonl", `{"type":"strike_event","strike":1,"date":"2026-02-07","infraction":"Test violation","consequence":"Warning"}
{"type":"strike_event","note":"Promoted to Ensign","date":"2026-02-10"}
{"type":"commendation","date":"2026-02-09","achievement":"Excellence","details":"Great work","awarded_by":"Admiral","points":2}
`)

	rs, err := ParseRankFiles(statusPath, eventsPath)
	if err != nil {
		t.Fatalf("ParseRankFiles failed: %v", err)
	}

	events := rs.GetRankEvents()
	// 1 strike + 1 promotion note + 1 commendation = 3 events
	if len(events) != 3 {
		t.Fatalf("expected 3 rank events, got %d", len(events))
	}

	strikeFound, promotionFound, commendationFound := false, false, false
	for _, ev := range events {
		switch ev.Type {
		case "strike":
			strikeFound = true
		case "promotion":
			promotionFound = true
		case "commendation":
			commendationFound = true
		}
	}
	if !strikeFound {
		t.Error("expected to find strike event")
	}
	if !promotionFound {
		t.Error("expected to find promotion event")
	}
	if !commendationFound {
		t.Error("expected to find commendation event")
	}
}

func TestParseRankFiles_DemotionType(t *testing.T) {
	statusPath := writeTemp(t, "rank-status.json", `{"current_rank":"Lieutenant (JG)","strikes":0}`)
	eventsPath := writeTemp(t, "rank-events.jsonl", `{"type":"demotion","date":"2026-03-05","from_rank":"Lieutenant Commander","to_rank":"Lieutenant","ordered_by":"Fleet Admiral Lunar Laurus","infraction":"Security posture violation","consequence":"Demotion 1 of 2"}
`)

	rs, err := ParseRankFiles(statusPath, eventsPath)
	if err != nil {
		t.Fatalf("ParseRankFiles failed: %v", err)
	}
	if len(rs.RankHistory) != 1 {
		t.Fatalf("expected 1 rank_history entry from demotion, got %d", len(rs.RankHistory))
	}
	if rs.RankHistory[0].Rank != "Lieutenant" {
		t.Errorf("expected rank 'Lieutenant', got '%s'", rs.RankHistory[0].Rank)
	}

	// Verify demotion also appears in StrikeHistory
	if len(rs.StrikeHistory) != 1 {
		t.Fatalf("expected 1 strike_history entry from demotion, got %d", len(rs.StrikeHistory))
	}

	// Verify GetRankEvents emits the demotion event
	events := rs.GetRankEvents()
	demotionFound := false
	for _, ev := range events {
		if ev.Type == "demotion" {
			demotionFound = true
			if ev.Date.IsZero() {
				t.Error("expected non-zero date on demotion event")
			}
		}
	}
	if !demotionFound {
		t.Errorf("expected demotion event in GetRankEvents(), got types: %v", func() []string {
			var types []string
			for _, ev := range events {
				types = append(types, ev.Type)
			}
			return types
		}())
	}
}
