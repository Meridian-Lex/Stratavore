package importers

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/meridian-lex/stratavore/internal/migrate/parsers"
)

// ImportConfig imports V2 config (token budgets and resource quotas) into V3 tables
// Returns (budgets_count, quotas_count, error)
func ImportConfig(ctx context.Context, tx pgx.Tx, v2Config *parsers.V2Config) (int, int, error) {
	// Import global token budget
	budgetCount, err := importTokenBudget(ctx, tx, v2Config)
	if err != nil {
		return 0, 0, fmt.Errorf("import token budget: %w", err)
	}

	// Import resource quotas (derived from V2 autonomous mode settings)
	quotaCount, err := importResourceQuotas(ctx, tx, v2Config)
	if err != nil {
		return budgetCount, 0, fmt.Errorf("import resource quotas: %w", err)
	}

	return budgetCount, quotaCount, nil
}

// importTokenBudget creates a global token budget from V2 config
// Returns the number of budgets created
func importTokenBudget(ctx context.Context, tx pgx.Tx, v2Config *parsers.V2Config) (int, error) {
	// Calculate period boundaries for today
	now := time.Now()
	periodStart, periodEnd := parsers.GetPeriodBoundaries("daily", now)

	query := `
		INSERT INTO token_budgets (
			scope, scope_id,
			limit_tokens, used_tokens,
			period_granularity, period_start, period_end,
			created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, NOW(), NOW())
		ON CONFLICT (scope, scope_id, period_start) DO UPDATE SET
			limit_tokens = EXCLUDED.limit_tokens,
			updated_at = NOW()
	`

	_, err := tx.Exec(ctx, query,
		"global",                       // scope
		nil,                            // scope_id (NULL for global)
		v2Config.TokenBudget.DailyLimit, // limit_tokens
		v2Config.TokenBudget.Tracking.TodayUsed, // used_tokens
		"daily",                        // period_granularity
		periodStart,                    // period_start
		periodEnd,                      // period_end
	)

	if err != nil {
		return 0, fmt.Errorf("insert token budget: %w", err)
	}

	return 1, nil
}

// importResourceQuotas creates default resource quotas
// V2 doesn't have explicit per-project quotas, so we use sensible defaults
// Returns the number of quotas created
func importResourceQuotas(ctx context.Context, tx pgx.Tx, v2Config *parsers.V2Config) (int, error) {
	// V2 doesn't have project-specific quotas, but we can create a default quota
	// Only create if there's autonomous mode configuration
	if !v2Config.AutonomousMode.Enabled {
		// No quotas to import if autonomous mode not enabled
		return 0, nil
	}

	// NOTE: In a real implementation, we'd want to get project names from the
	// imported projects. For now, we'll skip creating resource_quotas since
	// they're optional and V2 doesn't have them.

	// Alternative: Create a global default that can be referenced
	// This is left as a no-op for now since resource_quotas are project-specific

	return 0, nil
}

// ImportConfigForProject creates resource quotas for a specific project
// This is a helper function for use during full migration when project names are known
func ImportConfigForProject(ctx context.Context, tx pgx.Tx, projectName string, v2Config *parsers.V2Config) error {
	query := `
		INSERT INTO resource_quotas (
			project_name,
			max_concurrent_runners,
			max_memory_mb,
			max_cpu_percent,
			max_tokens_per_day
		) VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (project_name) DO UPDATE SET
			max_concurrent_runners = EXCLUDED.max_concurrent_runners,
			max_tokens_per_day = EXCLUDED.max_tokens_per_day,
			updated_at = NOW()
	`

	// Derive reasonable defaults from V2 autonomous mode config
	maxConcurrentRunners := 5 // Default
	var maxTokensPerDay *int64
	if v2Config.AutonomousMode.Enabled {
		maxTokensPerDay = &v2Config.AutonomousMode.MaxDailyTokens
	}

	_, err := tx.Exec(ctx, query,
		projectName,
		maxConcurrentRunners,
		nil, // max_memory_mb (V2 doesn't track)
		nil, // max_cpu_percent (V2 doesn't track)
		maxTokensPerDay,
	)

	if err != nil {
		return fmt.Errorf("insert resource quota for project %s: %w", projectName, err)
	}

	return nil
}
