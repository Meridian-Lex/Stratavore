package parsers

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

// V2RankStatusFile represents the combined rank state and event history.
// State fields come from rank-status.json; history arrays are populated
// from rank-events.jsonl via ParseRankFiles.
//
// Note: StrikeHistory, Commendations, and RankHistory carry json:"-" tags.
// They are intentionally excluded from direct JSON unmarshalling because
// rank-status.json only holds the current-state snapshot; history is
// reconstructed from rank-events.jsonl by ParseRankFiles. Callers that
// unmarshal V2RankStatusFile directly will receive empty slices for these
// fields unless they call ParseRankFiles.
type V2RankStatusFile struct {
	CurrentRank        string               `json:"current_rank"`
	ProgressTowardNext string               `json:"progress_toward_next"` // e.g., "2/5"
	NextRank           string               `json:"next_rank"`
	Strikes            int                  `json:"strikes"`
	LastUpdated        string               `json:"last_updated"`
	StrikeHistory      []V2StrikeEvent      `json:"-"` // populated from rank-events.jsonl
	Commendations      []V2Commendation     `json:"-"` // populated from rank-events.jsonl
	RankHistory        []V2RankHistoryEvent `json:"-"` // populated from rank-events.jsonl
}

// V2StrikeEvent represents a strike or note in the strike history
type V2StrikeEvent struct {
	Strike      int    `json:"strike,omitempty"`
	Date        string `json:"date"`
	Infraction  string `json:"infraction,omitempty"`
	Evidence    string `json:"evidence,omitempty"`
	Consequence string `json:"consequence,omitempty"`
	Note        string `json:"note,omitempty"`
}

// V2Commendation represents a commendation entry
type V2Commendation struct {
	Date        string `json:"date"`
	Achievement string `json:"achievement,omitempty"`
	Details     string `json:"details"`
	AwardedBy   string `json:"awarded_by"`
	Points      int    `json:"points"`
	Reason      string `json:"reason,omitempty"`
}

// V2RankHistoryEvent represents a rank change event
type V2RankHistoryEvent struct {
	Rank     string `json:"rank"`
	Achieved string `json:"achieved"`
	Lost     string `json:"lost,omitempty"`
	Reason   string `json:"reason,omitempty"`
	Note     string `json:"note,omitempty"`
}

// V2RankEventLine is a tagged-union type for a single line in rank-events.jsonl.
// The "type" field discriminates between event kinds.
type V2RankEventLine struct {
	Type string `json:"type"`

	// strike_event fields (also used for note entries)
	Strike      int    `json:"strike,omitempty"`
	Infraction  string `json:"infraction,omitempty"`
	Evidence    string `json:"evidence,omitempty"`
	Consequence string `json:"consequence,omitempty"`
	Note        string `json:"note,omitempty"`

	// commendation fields
	Achievement string `json:"achievement,omitempty"`
	Reason      string `json:"reason,omitempty"`
	Details     string `json:"details,omitempty"`
	AwardedBy   string `json:"awarded_by,omitempty"`
	Points      int    `json:"points,omitempty"`

	// rank_change / demotion fields
	Rank      string `json:"rank,omitempty"`
	Achieved  string `json:"achieved,omitempty"`
	Lost      string `json:"lost,omitempty"`
	FromRank  string `json:"from_rank,omitempty"`
	ToRank    string `json:"to_rank,omitempty"`
	OrderedBy string `json:"ordered_by,omitempty"`

	// common
	Date string `json:"date,omitempty"`
}

// ParseRankFiles reads rank-status.json and rank-events.jsonl and returns
// a combined V2RankStatusFile suitable for downstream import/sync operations.
func ParseRankFiles(statusPath, eventsPath string) (*V2RankStatusFile, error) {
	// Read current state
	statusData, err := os.ReadFile(statusPath)
	if err != nil {
		return nil, fmt.Errorf("read rank-status.json: %w", err)
	}

	var status V2RankStatusFile
	if err := json.Unmarshal(statusData, &status); err != nil {
		return nil, fmt.Errorf("parse rank-status.json: %w", err)
	}

	// Read event history
	eventsFile, err := os.Open(eventsPath)
	if err != nil {
		return nil, fmt.Errorf("open rank-events.jsonl: %w", err)
	}
	defer eventsFile.Close()

	scanner := bufio.NewScanner(eventsFile)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		var ev V2RankEventLine
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			return nil, fmt.Errorf("rank-events.jsonl line %d: %w", lineNum, err)
		}

		switch ev.Type {
		case "strike_event":
			status.StrikeHistory = append(status.StrikeHistory, V2StrikeEvent{
				Strike:      ev.Strike,
				Date:        ev.Date,
				Infraction:  ev.Infraction,
				Evidence:    ev.Evidence,
				Consequence: ev.Consequence,
				Note:        ev.Note,
			})
		case "commendation":
			status.Commendations = append(status.Commendations, V2Commendation{
				Date:        ev.Date,
				Achievement: ev.Achievement,
				Details:     ev.Details,
				AwardedBy:   ev.AwardedBy,
				Points:      ev.Points,
				Reason:      ev.Reason,
			})
		case "rank_change":
			if ev.Rank == "" || ev.Achieved == "" {
				return nil, fmt.Errorf("rank-events.jsonl line %d: rank_change missing required fields (rank, achieved)", lineNum)
			}
			status.RankHistory = append(status.RankHistory, V2RankHistoryEvent{
				Rank:     ev.Rank,
				Achieved: ev.Achieved,
				Lost:     ev.Lost,
				Reason:   ev.Reason,
				Note:     ev.Note,
			})
		case "demotion":
			// Demotions appear in both rank_history and strike_history for full context.
			// RankHistory records the resulting rank; StrikeHistory carries the full
			// rationale including infraction, evidence, and ordered-by fields.
			// GetRankEvents emits the demotion event from StrikeHistory only, skipping
			// the matching RankHistory entry to avoid duplicates.
			status.RankHistory = append(status.RankHistory, V2RankHistoryEvent{
				Rank:     ev.ToRank,
				Achieved: ev.Date,
				Reason:   fmt.Sprintf("Demotion from %s: %s", ev.FromRank, ev.Infraction),
			})
			status.StrikeHistory = append(status.StrikeHistory, V2StrikeEvent{
				Date:        ev.Date,
				Infraction:  ev.Infraction,
				Evidence:    ev.Evidence,
				Consequence: ev.Consequence,
				Note:        fmt.Sprintf("Demotion from %s to %s: %s / %s (ordered by %s)", ev.FromRank, ev.ToRank, ev.Infraction, ev.Consequence, ev.OrderedBy),
			})
		default:
			return nil, fmt.Errorf("rank-events.jsonl line %d: unknown event type %q", lineNum, ev.Type)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan rank-events.jsonl: %w", err)
	}

	return &status, nil
}

// GetRankEvents converts the rank status file into a flat list of rank_tracking events.
// All emitted event types conform to the rank_tracking.event_type CHECK constraint:
// strike, commendation, promotion, demotion, initial, note.
func (r *V2RankStatusFile) GetRankEvents() []V2RankEvent {
	var events []V2RankEvent

	// Add strike events
	for _, strike := range r.StrikeHistory {
		date, err := time.Parse("2006-01-02", strike.Date)
		if err != nil {
			date, err = time.Parse(time.RFC3339, strike.Date)
			if err != nil {
				date = time.Time{}
			}
		}

		if strike.Strike > 0 {
			events = append(events, V2RankEvent{
				Type:        "strike",
				Date:        date,
				Description: fmt.Sprintf("Strike %d: %s", strike.Strike, strike.Infraction),
				Evidence:    strike.Evidence,
			})
		} else if strike.Note != "" {
			eventType := "note"
			if strings.Contains(strike.Note, "Promoted") {
				eventType = "promotion"
			} else if strings.Contains(strike.Note, "Demoted") || strings.Contains(strike.Note, "Demotion") {
				eventType = "demotion"
			}
			events = append(events, V2RankEvent{
				Type:        eventType,
				Date:        date,
				Description: strike.Note,
				Evidence:    strike.Evidence,
			})
		}
	}

	// Add rank history events. Demotions are skipped here because ParseRankFiles
	// appends them to both RankHistory and StrikeHistory; the StrikeHistory loop
	// above already emits the demotion event, so re-emitting from RankHistory
	// would produce duplicates in rank_tracking.
	//
	// firstRankEmitted tracks whether the first non-demotion rank event has been
	// emitted. Using the raw slice index i==0 is incorrect because leading demotion
	// entries are skipped by the continue below, which would then mislabel the
	// first emitted event as "promotion" rather than "initial".
	firstRankEmitted := false
	for _, rh := range r.RankHistory {
		if strings.Contains(rh.Reason, "Demotion") {
			continue
		}

		date, err := time.Parse("2006-01-02", rh.Achieved)
		if err != nil {
			date, err = time.Parse(time.RFC3339, rh.Achieved)
			if err != nil {
				date = time.Time{}
			}
		}

		eventType := "promotion"
		if !firstRankEmitted {
			eventType = "initial"
			firstRankEmitted = true
		}

		description := rh.Reason
		if description == "" {
			description = rh.Note
		}
		desc := fmt.Sprintf("Rank: %s", rh.Rank)
		if description != "" {
			desc += " — " + description
		}

		events = append(events, V2RankEvent{
			Type:        eventType,
			Date:        date,
			Description: desc,
		})
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
