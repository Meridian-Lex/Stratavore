package importers

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/meridian-lex/stratavore/internal/migrate/parsers"
)

// ImportRank imports V2 rank tracking data into the rank_tracking table
// Returns the number of rank events imported
// Idempotent: Uses ON CONFLICT DO NOTHING to safely handle repeated syncs
func ImportRank(ctx context.Context, tx pgx.Tx, rankStatus *parsers.V2RankStatusFile) (int, error) {
	query := `
		INSERT INTO rank_tracking (
			current_rank, progress, strikes, commendations,
			event_type, event_date, description, evidence, metadata
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (event_type, event_date, COALESCE(description, ''))
		DO NOTHING
	`

	// Get all rank events (strikes, commendations, promotions, demotions)
	events := rankStatus.GetRankEvents()

	count := 0
	for _, event := range events {
		// Extract progress from rankStatus (e.g., "2/5")
		progress := rankStatus.ProgressTowardNext

		// For metadata, store any additional context
		metadata := map[string]interface{}{
			"v2_import": true,
		}

		_, err := tx.Exec(ctx, query,
			rankStatus.CurrentRank,   // current_rank
			progress,                  // progress (e.g., "2/5")
			rankStatus.Strikes,        // strikes (current count)
			len(rankStatus.Commendations), // commendations (total count)
			event.Type,                // event_type
			event.Date,                // event_date
			event.Description,         // description
			event.Evidence,            // evidence
			metadata,                  // metadata (JSONB)
		)

		if err != nil {
			return count, fmt.Errorf("import rank event %s: %w", event.Type, err)
		}
		count++
	}

	// Also import the current state as an "initial" event if no events exist
	if len(events) == 0 {
		metadata := map[string]interface{}{
			"v2_import":     true,
			"current_state": true,
		}

		_, err := tx.Exec(ctx, query,
			rankStatus.CurrentRank,
			rankStatus.ProgressTowardNext,
			rankStatus.Strikes,
			len(rankStatus.Commendations),
			"initial",
			rankStatus.LastUpdated,
			"V2 migration: current rank state",
			"",
			metadata,
		)

		if err != nil {
			return count, fmt.Errorf("import current rank state: %w", err)
		}
		count++
	}

	return count, nil
}

// parseProgress extracts numeric progress from "2/5" format
func parseProgress(progressStr string) (current int, total int, err error) {
	parts := strings.Split(progressStr, "/")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid progress format: %s", progressStr)
	}

	current, err = strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		return 0, 0, fmt.Errorf("parse current: %w", err)
	}

	total, err = strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil {
		return 0, 0, fmt.Errorf("parse total: %w", err)
	}

	return current, total, nil
}

// ImportRankAndDirectives imports both rank tracking and directives in one call
// Returns (rank_events_count, directives_count, error)
func ImportRankAndDirectives(ctx context.Context, tx pgx.Tx, rankStatus *parsers.V2RankStatusFile, directives []parsers.V2Directive) (int, int, error) {
	// Import rank tracking
	rankCount, err := ImportRank(ctx, tx, rankStatus)
	if err != nil {
		return 0, 0, fmt.Errorf("import rank: %w", err)
	}

	// Import directives
	directivesCount, err := ImportDirectives(ctx, tx, directives)
	if err != nil {
		return rankCount, 0, fmt.Errorf("import directives: %w", err)
	}

	return rankCount, directivesCount, nil
}
