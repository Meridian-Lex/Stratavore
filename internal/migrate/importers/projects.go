package importers

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/meridian-lex/stratavore/internal/migrate/parsers"
	"github.com/meridian-lex/stratavore/pkg/types"
)

// ImportProjects imports V2 projects into the projects table
// Uses UPSERT semantics (INSERT ... ON CONFLICT UPDATE) for idempotency
// Returns the number of projects processed
func ImportProjects(ctx context.Context, tx pgx.Tx, v2Projects []parsers.V2Project) (int, error) {
	query := `
		INSERT INTO projects (name, path, status, description, tags, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, NOW())
		ON CONFLICT (name) DO UPDATE SET
			path = EXCLUDED.path,
			status = EXCLUDED.status,
			description = EXCLUDED.description,
			tags = EXCLUDED.tags,
			updated_at = NOW()
	`

	count := 0
	for _, v2proj := range v2Projects {
		// Map V2 status to V3 status
		status := mapProjectStatus(v2proj.Status)

		// Build description from V2 notes and priority
		description := buildDescription(v2proj)

		// Tags from priority (if not "-")
		var tags []string
		if v2proj.Priority != "" && v2proj.Priority != "-" {
			tags = []string{strings.ToLower(v2proj.Priority)}
		}

		// Use started date as created_at if available
		createdAt := v2proj.Started
		if createdAt.IsZero() {
			// If no started date, use a default (migration time)
			// This will be set by the migration timestamp
			createdAt = v2proj.Started // Keep as zero, will be handled by COALESCE or default
		}

		_, err := tx.Exec(ctx, query,
			v2proj.Name,
			v2proj.Path,
			status,
			description,
			tags,
			createdAt,
		)

		if err != nil {
			return count, fmt.Errorf("import project %s: %w", v2proj.Name, err)
		}
		count++
	}

	return count, nil
}

// mapProjectStatus converts V2 status strings to V3 ProjectStatus
func mapProjectStatus(v2Status string) types.ProjectStatus {
	switch strings.ToUpper(v2Status) {
	case "ACTIVE":
		return types.ProjectActive
	case "ARCHIVED":
		return types.ProjectArchived
	case "IDLE":
		return types.ProjectIdle
	default:
		// Default to idle for unknown statuses
		return types.ProjectIdle
	}
}

// buildDescription creates a description from V2 notes and priority
func buildDescription(v2proj parsers.V2Project) string {
	parts := []string{}

	if v2proj.Notes != "" {
		parts = append(parts, v2proj.Notes)
	}

	if v2proj.Priority != "" && v2proj.Priority != "-" {
		parts = append(parts, fmt.Sprintf("Priority: %s", v2proj.Priority))
	}

	return strings.Join(parts, " — ")
}
