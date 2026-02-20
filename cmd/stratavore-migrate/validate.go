package main

import (
	"context"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/meridian-lex/stratavore/internal/migrate/validator"
	"github.com/spf13/cobra"
)

var (
	validateDBURL string
)

func init() {
	validateCmd.Flags().StringVar(&validateDBURL, "db-url", "", "PostgreSQL connection URL (default: from STRATAVORE_DB_URL env)")
}

var validateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate migrated data integrity",
	Long: `Runs post-migration validation checks on the current database state.

This command verifies data integrity without modifying the database:
  - Row counts and expected data presence
  - No NULL values in NOT NULL columns
  - Foreign key integrity (no orphaned references)
  - Timestamps within valid ranges
  - Token totals preserved (±5% tolerance)
  - Text fields not truncated
  - JSONB fields valid

Useful for:
  - Verifying successful migration
  - Post-import data quality checks
  - Troubleshooting migration issues

Example:
  stratavore-migrate validate --db-url=postgres://localhost/stratavore_db
`,
	RunE: runValidate,
}

func runValidate(cmd *cobra.Command, args []string) error {
	// Get database URL
	dbURL := validateDBURL
	if dbURL == "" {
		dbURL = os.Getenv("STRATAVORE_DB_URL")
		if dbURL == "" {
			return fmt.Errorf("database URL required: set STRATAVORE_DB_URL or use --db-url flag")
		}
	}

	fmt.Println("Stratavore V3 Data Validation")
	fmt.Println("═══════════════════════════════════════════════════════════")
	fmt.Println()

	// Connect to database
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}
	defer pool.Close()

	// Begin read-only transaction for validation
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Run validation checks
	fmt.Println("Running validation checks...")
	fmt.Println()

	postChecks := &validator.PostMigrationChecks{
		Tx: tx,
		ExpectedCounts: nil, // No expected counts for standalone validation
	}

	errs := postChecks.ValidateAll()

	// Report results
	if len(errs) == 0 {
		fmt.Println("✓ All validation checks passed")
		fmt.Println()
		fmt.Println("Database integrity verified:")
		fmt.Println("  - Row counts valid")
		fmt.Println("  - No NULL constraint violations")
		fmt.Println("  - All foreign keys resolve")
		fmt.Println("  - Timestamps within valid range")
		fmt.Println("  - Token totals consistent")
		fmt.Println("  - No text truncation detected")
		fmt.Println("  - All JSONB fields valid")
		fmt.Println()
		return nil
	}

	// Validation failed
	fmt.Printf("✗ Validation failed with %d error(s):\n", len(errs))
	fmt.Println()

	for i, err := range errs {
		fmt.Printf("%d. %v\n", i+1, err)
	}

	fmt.Println()
	fmt.Println("Recommendation: Review errors above and investigate database state.")
	fmt.Println()

	return fmt.Errorf("validation failed with %d error(s)", len(errs))
}
