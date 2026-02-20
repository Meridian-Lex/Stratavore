package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/meridian-lex/stratavore/internal/migrate/importers"
	"github.com/meridian-lex/stratavore/internal/migrate/parsers"
	"github.com/meridian-lex/stratavore/internal/migrate/validator"
	"github.com/spf13/cobra"
)

var (
	dbURL      string
	skipBackup bool
)

func init() {
	importCmd.Flags().StringVar(&dbURL, "db-url", "", "PostgreSQL connection URL (default: from STRATAVORE_DB_URL env)")
	importCmd.Flags().BoolVar(&skipBackup, "skip-backup", false, "Skip pg_dump backup before import (not recommended)")
}

var importCmd = &cobra.Command{
	Use:   "import",
	Short: "Import V2 data into Stratavore V3",
	Long: `Imports Lex V2 state files into Stratavore V3 PostgreSQL database.

This command performs a transactional import with pre/post validation and automatic rollback on failure.

Example:
  stratavore-migrate import --v2-dir=/home/meridian/meridian-home/lex-internal/state
  stratavore-migrate import --v2-dir=/path/to/state --db-url=postgres://localhost/stratavore_db
`,
	RunE: runImport,
}

func runImport(cmd *cobra.Command, args []string) error {
	// Validate V2 directory
	if v2Dir == "" {
		return fmt.Errorf("--v2-dir flag is required")
	}

	// Get database URL
	if dbURL == "" {
		dbURL = os.Getenv("STRATAVORE_DB_URL")
		if dbURL == "" {
			return fmt.Errorf("database URL required: set STRATAVORE_DB_URL or use --db-url flag")
		}
	}

	fmt.Println("Lex V2 → Stratavore V3 Migration")
	fmt.Println("═══════════════════════════════════════════════════════════")
	fmt.Println()

	// Connect to database
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}
	defer pool.Close()

	// Step 1: Pre-migration validation
	fmt.Println("[1/8] Running pre-migration validation...")
	preChecks := &validator.PreMigrationChecks{
		V2Dir: v2Dir,
		Pool:  pool,
	}
	if errs := preChecks.ValidateAll(); len(errs) > 0 {
		fmt.Println("Pre-migration validation failed:")
		for _, err := range errs {
			fmt.Printf("  - %v\n", err)
		}
		return fmt.Errorf("pre-migration validation failed with %d error(s)", len(errs))
	}
	fmt.Println("✓ Pre-migration checks passed")
	fmt.Println()

	// Step 2: Create database snapshot
	var snapshotPath string
	if !skipBackup {
		fmt.Println("[2/8] Creating database snapshot...")
		snapshotPath, err = createDatabaseSnapshot()
		if err != nil {
			return fmt.Errorf("create database snapshot: %w", err)
		}
		fmt.Printf("✓ Database snapshot created: %s\n", snapshotPath)
		fmt.Println()
	} else {
		fmt.Println("[2/8] Skipping database snapshot (--skip-backup enabled)")
		fmt.Println()
	}

	// Step 3: Begin transaction
	fmt.Println("[3/8] Starting import transaction...")
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx) // Auto-rollback on error/panic

	// Step 4: Parse all V2 files
	fmt.Println("[4/8] Parsing V2 data files...")

	// Parse PROJECT-MAP.md
	projectMapPath := filepath.Join(v2Dir, "PROJECT-MAP.md")
	projects, err := parsers.ParseProjectMapFile(projectMapPath)
	if err != nil {
		return fmt.Errorf("parse PROJECT-MAP.md: %w", err)
	}
	fmt.Printf("  ✓ Parsed PROJECT-MAP.md: %d projects\n", len(projects))

	// Parse time_sessions.jsonl
	sessionsPath := filepath.Join(v2Dir, "time_sessions.jsonl")
	sessions, err := parsers.ParseTimeSessions(sessionsPath)
	if err != nil {
		return fmt.Errorf("parse time_sessions.jsonl: %w", err)
	}
	fmt.Printf("  ✓ Parsed time_sessions.jsonl: %d sessions\n", len(sessions))

	// Parse LEX-CONFIG.yaml
	configPath := filepath.Join(v2Dir, "..", "config", "LEX-CONFIG.yaml")
	config, err := parsers.ParseLexConfigFile(configPath)
	if err != nil {
		return fmt.Errorf("parse LEX-CONFIG.yaml: %w", err)
	}
	fmt.Printf("  ✓ Parsed LEX-CONFIG.yaml: token budgets + quotas\n")

	// Parse rank-status.jsonl
	rankPath := filepath.Join(v2Dir, "..", "directives", "rank-status.jsonl")
	rankStatus, err := parsers.ParseRankStatusFile(rankPath)
	if err != nil {
		return fmt.Errorf("parse rank-status.jsonl: %w", err)
	}
	rankEvents := rankStatus.GetRankEvents()
	fmt.Printf("  ✓ Parsed rank-status.jsonl: %d events\n", len(rankEvents))

	// Parse behavioral-directives.jsonl
	directivesPath := filepath.Join(v2Dir, "..", "directives", "behavioral-directives.jsonl")
	directives, err := parsers.ParseDirectives(directivesPath)
	if err != nil {
		return fmt.Errorf("parse behavioral-directives.jsonl: %w", err)
	}
	fmt.Printf("  ✓ Parsed behavioral-directives.jsonl: %d directives\n", len(directives))
	fmt.Println()

	// Step 5: Import data
	fmt.Println("[5/8] Importing data into PostgreSQL...")

	// Import projects
	projectsImported, err := importers.ImportProjects(ctx, tx, projects)
	if err != nil {
		return fmt.Errorf("import projects: %w", err)
	}
	fmt.Printf("  ✓ Imported projects: %d rows\n", projectsImported)

	// Import sessions
	sessionsImported, err := importers.ImportSessions(ctx, tx, sessions)
	if err != nil {
		return fmt.Errorf("import sessions: %w", err)
	}
	fmt.Printf("  ✓ Imported sessions: %d rows\n", sessionsImported)

	// Import config (token budgets + quotas)
	budgetsImported, quotasImported, err := importers.ImportConfig(ctx, tx, config)
	if err != nil {
		return fmt.Errorf("import config: %w", err)
	}
	fmt.Printf("  ✓ Imported token budgets: %d rows\n", budgetsImported)
	fmt.Printf("  ✓ Imported resource quotas: %d rows\n", quotasImported)

	// Import rank & directives
	rankImported, directivesImported, err := importers.ImportRankAndDirectives(ctx, tx, rankStatus, directives)
	if err != nil {
		return fmt.Errorf("import rank/directives: %w", err)
	}
	fmt.Printf("  ✓ Imported rank events: %d rows\n", rankImported)
	fmt.Printf("  ✓ Imported directives: %d rows\n", directivesImported)
	fmt.Println()

	// Step 6: Post-migration validation
	fmt.Println("[6/8] Running post-migration validation...")
	postChecks := &validator.PostMigrationChecks{
		Tx: tx,
		ExpectedCounts: map[string]int{
			"projects":      len(projects),
			"sessions":      len(sessions),
			"rank_tracking": len(rankEvents),
			"directives":    len(directives),
		},
	}
	if errs := postChecks.ValidateAll(); len(errs) > 0 {
		fmt.Println("✗ Post-migration validation failed:")
		for _, err := range errs {
			fmt.Printf("  - %v\n", err)
		}
		fmt.Println()
		fmt.Println("Transaction will be rolled back. Database unchanged.")
		if snapshotPath != "" {
			fmt.Printf("Database snapshot available at: %s\n", snapshotPath)
		}
		return fmt.Errorf("post-migration validation failed with %d error(s)", len(errs))
	}
	fmt.Println("✓ Post-migration validation passed")
	fmt.Println()

	// Step 7: Commit transaction
	fmt.Println("[7/8] Committing transaction...")
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	fmt.Println("✓ Transaction committed")
	fmt.Println()

	// Step 8: Log import record
	fmt.Println("[8/8] Recording import metadata...")
	if err := recordImport(ctx, pool, snapshotPath); err != nil {
		// Non-fatal - import succeeded but logging failed
		fmt.Printf("Warning: failed to record import metadata: %v\n", err)
	} else {
		fmt.Println("✓ Import metadata recorded")
	}
	fmt.Println()

	// Success summary
	fmt.Println("Migration Complete")
	fmt.Println("───────────────────────────────────────────────────────────")
	fmt.Printf("  Projects:        %d\n", projectsImported)
	fmt.Printf("  Sessions:        %d\n", sessionsImported)
	fmt.Printf("  Token Budgets:   %d\n", budgetsImported)
	fmt.Printf("  Resource Quotas: %d\n", quotasImported)
	fmt.Printf("  Rank Events:     %d\n", rankImported)
	fmt.Printf("  Directives:      %d\n", directivesImported)
	fmt.Println()
	fmt.Println("All V2 data successfully imported into Stratavore V3.")
	if snapshotPath != "" {
		fmt.Printf("Pre-import snapshot: %s\n", snapshotPath)
	}
	fmt.Println()

	return nil
}

// createDatabaseSnapshot creates a pg_dump snapshot of the current database
func createDatabaseSnapshot() (string, error) {
	timestamp := time.Now().Unix()
	snapshotPath := fmt.Sprintf("/tmp/stratavore-pre-migration-%d.sql", timestamp)

	// Extract database name from URL for pg_dump
	// Note: This is a simplified approach. Production would parse the URL properly.
	dbName := "stratavore_db" // Default database name

	cmd := exec.Command("pg_dump", "-f", snapshotPath, dbName)
	cmd.Env = append(os.Environ(), "PGPASSWORD="+os.Getenv("PGPASSWORD"))

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("pg_dump failed: %w\nOutput: %s", err, string(output))
	}

	return snapshotPath, nil
}

// recordImport logs the import event to v2_import_log table
func recordImport(ctx context.Context, pool *pgxpool.Pool, snapshotPath string) error {
	query := `
		INSERT INTO v2_import_log (
			import_date,
			snapshot_path,
			status
		) VALUES ($1, $2, $3)
	`

	_, err := pool.Exec(ctx, query, time.Now(), snapshotPath, "success")
	return err
}
