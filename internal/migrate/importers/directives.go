package importers

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/meridian-lex/stratavore/internal/migrate/parsers"
)

// ImportDirectives imports V2 behavioral directives into the directives table
// Returns the number of directives imported
func ImportDirectives(ctx context.Context, tx pgx.Tx, v2Directives []parsers.V2Directive) (int, error) {
	query := `
		INSERT INTO directives (
			id, severity, trigger_condition, action, directive_text,
			standard_process, enabled, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (id) DO UPDATE SET
			severity = EXCLUDED.severity,
			trigger_condition = EXCLUDED.trigger_condition,
			action = EXCLUDED.action,
			directive_text = EXCLUDED.directive_text,
			standard_process = EXCLUDED.standard_process,
			enabled = EXCLUDED.enabled,
			updated_at = NOW()
	`

	count := 0
	for _, directive := range v2Directives {
		// Convert action map to JSONB
		actionJSON, err := json.Marshal(directive.Action)
		if err != nil {
			return count, fmt.Errorf("marshal action for directive %s: %w", directive.ID, err)
		}

		// Parse timestamp (may be empty for some directives)
		var createdAt time.Time
		if directive.Timestamp != "" {
			createdAt, err = time.Parse(time.RFC3339, directive.Timestamp)
			if err != nil {
				// If parse fails, use current time
				createdAt = time.Now()
			}
		} else {
			createdAt = time.Now()
		}

		// All imported directives are enabled by default
		enabled := true

		_, err = tx.Exec(ctx, query,
			directive.ID,
			directive.Severity,
			directive.TriggerCondition,
			actionJSON, // JSONB
			directive.DirectiveText,
			directive.StandardProcess,
			enabled,
			createdAt,
			createdAt, // updated_at = created_at initially
		)

		if err != nil {
			return count, fmt.Errorf("import directive %s: %w", directive.ID, err)
		}
		count++
	}

	return count, nil
}
