package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/meridian-lex/stratavore/internal/migrate/importers"
	"github.com/meridian-lex/stratavore/internal/migrate/parsers"
	"github.com/spf13/cobra"
)

var (
	syncDBURL string
	syncType  string
)

func init() {
	syncCmd.Flags().StringVar(&syncDBURL, "db-url", "", "PostgreSQL connection URL (default: from STRATAVORE_DB_URL env)")
	syncCmd.Flags().StringVar(&syncType, "type", "all", "Sync type: all, projects, sessions, config, rank")
}

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Synchronize V2 data to V3 database (idempotent)",
	Long: `Synchronizes Lex V2 state files into Stratavore V3 PostgreSQL database.

This command performs UPSERT operations (idempotent) to sync V2 data:
  - Projects: INSERT ... ON CONFLICT UPDATE
  - Sessions: INSERT ... ON CONFLICT UPDATE
  - Config: UPDATE existing budgets/quotas
  - Rank: INSERT new events only (append-only)

Useful for:
  - Ongoing V2→V3 parallel operation
  - Scheduled synchronization (cron jobs)
  - Manual state refresh

Sync types:
  --type=all        Sync all data types (default)
  --type=projects   Sync PROJECT-MAP.md only
  --type=sessions   Sync time_sessions.jsonl only
  --type=config     Sync LEX-CONFIG.yaml only
  --type=rank       Sync rank-status.jsonl only

Example:
  stratavore-migrate sync --v2-dir=/home/meridian/meridian-home/lex-internal/state
  stratavore-migrate sync --type=projects --v2-dir=/path/to/state
`,
	RunE: runSync,
}

func runSync(cmd *cobra.Command, args []string) error {
	// Validate V2 directory
	if v2Dir == "" {
		return fmt.Errorf("--v2-dir flag is required")
	}

	// Get database URL
	dbURL := syncDBURL
	if dbURL == "" {
		dbURL = os.Getenv("STRATAVORE_DB_URL")
		if dbURL == "" {
			return fmt.Errorf("database URL required: set STRATAVORE_DB_URL or use --db-url flag")
		}
	}

	fmt.Println("Lex V2 → Stratavore V3 Synchronization")
	fmt.Println("═══════════════════════════════════════════════════════════")
	fmt.Printf("Sync type: %s\n", syncType)
	fmt.Println()

	// Connect to database
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}
	defer pool.Close()

	// Begin transaction
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Sync based on type
	switch syncType {
	case "all":
		if err := syncAll(ctx, tx); err != nil {
			return err
		}
	case "projects":
		if err := syncProjects(ctx, tx); err != nil {
			return err
		}
	case "sessions":
		if err := syncSessions(ctx, tx); err != nil {
			return err
		}
	case "config":
		if err := syncConfig(ctx, tx); err != nil {
			return err
		}
	case "rank":
		if err := syncRank(ctx, tx); err != nil {
			return err
		}
	default:
		return fmt.Errorf("invalid sync type: %s (must be: all, projects, sessions, config, rank)", syncType)
	}

	// Commit transaction
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	fmt.Println()
	fmt.Println("✓ Synchronization complete")
	return nil
}

func syncAll(ctx context.Context, tx pgx.Tx) error {
	if err := syncProjects(ctx, tx); err != nil {
		return err
	}
	if err := syncSessions(ctx, tx); err != nil {
		return err
	}
	if err := syncConfig(ctx, tx); err != nil {
		return err
	}
	if err := syncRank(ctx, tx); err != nil {
		return err
	}
	return nil
}

func syncProjects(ctx context.Context, tx pgx.Tx) error {
	fmt.Println("[Projects] Syncing PROJECT-MAP.md...")

	projectMapPath := filepath.Join(v2Dir, "PROJECT-MAP.md")
	projects, err := parsers.ParseProjectMapFile(projectMapPath)
	if err != nil {
		return fmt.Errorf("parse PROJECT-MAP.md: %w", err)
	}

	count, err := importers.ImportProjects(ctx, tx, projects)
	if err != nil {
		return fmt.Errorf("sync projects: %w", err)
	}

	fmt.Printf("  ✓ Synced %d projects\n", count)
	return nil
}

func syncSessions(ctx context.Context, tx pgx.Tx) error {
	fmt.Println("[Sessions] Syncing time_sessions.jsonl...")

	sessionsPath := filepath.Join(v2Dir, "time_sessions.jsonl")
	sessions, err := parsers.ParseTimeSessions(sessionsPath)
	if err != nil {
		return fmt.Errorf("parse time_sessions.jsonl: %w", err)
	}

	count, err := importers.ImportSessions(ctx, tx, sessions)
	if err != nil {
		return fmt.Errorf("sync sessions: %w", err)
	}

	fmt.Printf("  ✓ Synced %d sessions\n", count)
	return nil
}

func syncConfig(ctx context.Context, tx pgx.Tx) error {
	fmt.Println("[Config] Syncing LEX-CONFIG.yaml...")

	configPath := filepath.Join(v2Dir, "..", "config", "LEX-CONFIG.yaml")
	config, err := parsers.ParseLexConfigFile(configPath)
	if err != nil {
		return fmt.Errorf("parse LEX-CONFIG.yaml: %w", err)
	}

	budgets, quotas, err := importers.ImportConfig(ctx, tx, config)
	if err != nil {
		return fmt.Errorf("sync config: %w", err)
	}

	fmt.Printf("  ✓ Synced %d budgets, %d quotas\n", budgets, quotas)
	return nil
}

func syncRank(ctx context.Context, tx pgx.Tx) error {
	fmt.Println("[Rank] Syncing rank-status.jsonl...")

	rankPath := filepath.Join(v2Dir, "..", "directives", "rank-status.jsonl")
	rankStatus, err := parsers.ParseRankStatusFile(rankPath)
	if err != nil {
		return fmt.Errorf("parse rank-status.jsonl: %w", err)
	}

	// For rank sync, we only want to add new events (append-only)
	// The ImportRank function already handles idempotency, but for ongoing sync
	// we might want to check last sync state to avoid duplicates.
	// For now, rely on database constraints to prevent duplicates.

	count, err := importers.ImportRank(ctx, tx, rankStatus)
	if err != nil {
		return fmt.Errorf("sync rank: %w", err)
	}

	fmt.Printf("  ✓ Synced %d rank events\n", count)
	return nil
}
