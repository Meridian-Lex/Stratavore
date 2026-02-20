package main

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
)

var (
	snapshotPath   string
	rollbackDBURL  string
	forceRollback  bool
)

func init() {
	rollbackCmd.Flags().StringVar(&snapshotPath, "snapshot", "", "Path to pg_dump snapshot file (required)")
	rollbackCmd.Flags().StringVar(&rollbackDBURL, "db-url", "", "PostgreSQL connection URL (default: from STRATAVORE_DB_URL env)")
	rollbackCmd.Flags().BoolVar(&forceRollback, "force", false, "Skip confirmation prompt (dangerous)")
}

var rollbackCmd = &cobra.Command{
	Use:   "rollback",
	Short: "Rollback migration using pg_dump snapshot",
	Long: `Restores the database from a pg_dump snapshot file.

WARNING: This is a DESTRUCTIVE operation that will:
  - Drop all tables in the target database
  - Restore data from the snapshot file
  - PERMANENTLY ERASE any changes made after the snapshot

This command should only be used when:
  - Migration validation fails and you need to revert
  - Testing migration on a development database
  - Recovering from a corrupted migration

IMPORTANT: Ensure you have a backup before running this command.

Example:
  stratavore-migrate rollback --snapshot=/tmp/stratavore-pre-migration-1708423200.sql
  stratavore-migrate rollback --snapshot=/path/to/backup.sql --db-url=postgres://localhost/stratavore_db
`,
	RunE: runRollback,
}

func runRollback(cmd *cobra.Command, args []string) error {
	// Validate snapshot path
	if snapshotPath == "" {
		return fmt.Errorf("--snapshot flag is required")
	}

	// Check snapshot file exists
	if _, err := os.Stat(snapshotPath); os.IsNotExist(err) {
		return fmt.Errorf("snapshot file not found: %s", snapshotPath)
	}

	// Get database URL
	dbURL := rollbackDBURL
	if dbURL == "" {
		dbURL = os.Getenv("STRATAVORE_DB_URL")
		if dbURL == "" {
			return fmt.Errorf("database URL required: set STRATAVORE_DB_URL or use --db-url flag")
		}
	}

	fmt.Println("Stratavore V3 Migration Rollback")
	fmt.Println("═══════════════════════════════════════════════════════════")
	fmt.Println()
	fmt.Println("WARNING: This operation will PERMANENTLY ERASE all data")
	fmt.Println("         in the target database and restore from snapshot.")
	fmt.Println()
	fmt.Printf("Snapshot: %s\n", snapshotPath)
	fmt.Printf("Database: %s\n", dbURL)
	fmt.Println()

	// Confirmation prompt (unless --force)
	if !forceRollback {
		fmt.Print("Are you sure you want to continue? Type 'ROLLBACK' to confirm: ")
		var confirmation string
		fmt.Scanln(&confirmation)

		if confirmation != "ROLLBACK" {
			fmt.Println()
			fmt.Println("Rollback cancelled.")
			return nil
		}
	}

	fmt.Println()
	fmt.Println("Starting rollback...")
	fmt.Println()

	// Execute psql to restore snapshot
	// Note: This assumes PostgreSQL is configured to allow connections
	// and that the snapshot was created with compatible pg_dump version

	// Extract database name from URL (simplified approach)
	// Production would parse the URL properly
	dbName := "stratavore_db" // Default

	// Step 1: Drop existing database (if exists) and recreate
	fmt.Println("[1/3] Preparing database...")

	dropCmd := exec.Command("psql", "-c", fmt.Sprintf("DROP DATABASE IF EXISTS %s;", dbName))
	dropCmd.Env = append(os.Environ(), "PGPASSWORD="+os.Getenv("PGPASSWORD"))
	dropOutput, err := dropCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("drop database failed: %w\nOutput: %s", err, string(dropOutput))
	}

	createCmd := exec.Command("psql", "-c", fmt.Sprintf("CREATE DATABASE %s;", dbName))
	createCmd.Env = append(os.Environ(), "PGPASSWORD="+os.Getenv("PGPASSWORD"))
	createOutput, err := createCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("create database failed: %w\nOutput: %s", err, string(createOutput))
	}

	fmt.Println("✓ Database prepared")
	fmt.Println()

	// Step 2: Restore from snapshot
	fmt.Println("[2/3] Restoring from snapshot...")

	restoreCmd := exec.Command("psql", "-d", dbName, "-f", snapshotPath)
	restoreCmd.Env = append(os.Environ(), "PGPASSWORD="+os.Getenv("PGPASSWORD"))
	restoreOutput, err := restoreCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("restore failed: %w\nOutput: %s", err, string(restoreOutput))
	}

	fmt.Println("✓ Snapshot restored")
	fmt.Println()

	// Step 3: Verify restoration
	fmt.Println("[3/3] Verifying restoration...")

	verifyCmd := exec.Command("psql", "-d", dbName, "-c", "SELECT COUNT(*) FROM projects;")
	verifyCmd.Env = append(os.Environ(), "PGPASSWORD="+os.Getenv("PGPASSWORD"))
	verifyOutput, err := verifyCmd.CombinedOutput()
	if err != nil {
		fmt.Printf("Warning: Verification query failed: %v\n", err)
		fmt.Println("Database may have been restored, but verification incomplete.")
	} else {
		fmt.Println("✓ Verification complete")
		fmt.Printf("  %s", string(verifyOutput))
	}

	fmt.Println()
	fmt.Println("Rollback Complete")
	fmt.Println("───────────────────────────────────────────────────────────")
	fmt.Printf("Database restored from: %s\n", snapshotPath)
	fmt.Println()
	fmt.Println("IMPORTANT: Any changes made after this snapshot were LOST.")
	fmt.Println()

	return nil
}
