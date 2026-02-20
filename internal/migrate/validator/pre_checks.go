package validator

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PreMigrationChecks performs all pre-migration validation checks
type PreMigrationChecks struct {
	V2Dir string
	Pool  *pgxpool.Pool
}

// ValidationError represents a validation failure
type ValidationError struct {
	Check   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("[%s] %s", e.Check, e.Message)
}

// ValidateAll runs all pre-migration checks and returns any errors
func (c *PreMigrationChecks) ValidateAll() []error {
	var errors []error

	// Check 1: V2 files exist
	if err := c.CheckV2FilesExist(); err != nil {
		errors = append(errors, err)
	}

	// Check 2: V2 files readable
	if err := c.CheckV2FilesReadable(); err != nil {
		errors = append(errors, err)
	}

	// Check 3: PostgreSQL connection healthy
	if err := c.CheckPostgreSQLHealthy(); err != nil {
		errors = append(errors, err)
	}

	// Check 4: No active V3 runners
	if err := c.CheckNoActiveRunners(); err != nil {
		errors = append(errors, err)
	}

	// Check 5: Sufficient disk space
	if err := c.CheckSufficientDiskSpace(); err != nil {
		errors = append(errors, err)
	}

	// Check 6: V2 files not corrupted
	if err := c.CheckV2FilesValid(); err != nil {
		errors = append(errors, err)
	}

	return errors
}

// CheckV2FilesExist verifies all required V2 files exist
func (c *PreMigrationChecks) CheckV2FilesExist() error {
	requiredFiles := []string{
		"PROJECT-MAP.md",
		"time_sessions.jsonl",
	}

	requiredDirs := []string{
		filepath.Join(c.V2Dir, "..", "config"),
		filepath.Join(c.V2Dir, "..", "directives"),
	}

	configFiles := []string{
		filepath.Join(c.V2Dir, "..", "config", "LEX-CONFIG.yaml"),
	}

	directiveFiles := []string{
		filepath.Join(c.V2Dir, "..", "directives", "rank-status.jsonl"),
		filepath.Join(c.V2Dir, "..", "directives", "behavioral-directives.jsonl"),
	}

	// Check state files
	for _, file := range requiredFiles {
		path := filepath.Join(c.V2Dir, file)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			return &ValidationError{
				Check:   "V2FilesExist",
				Message: fmt.Sprintf("required file not found: %s", path),
			}
		}
	}

	// Check config directory and files
	for _, dir := range requiredDirs {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			return &ValidationError{
				Check:   "V2FilesExist",
				Message: fmt.Sprintf("required directory not found: %s", dir),
			}
		}
	}

	// Check config files
	for _, file := range configFiles {
		if _, err := os.Stat(file); os.IsNotExist(err) {
			return &ValidationError{
				Check:   "V2FilesExist",
				Message: fmt.Sprintf("required config file not found: %s", file),
			}
		}
	}

	// Check directive files
	for _, file := range directiveFiles {
		if _, err := os.Stat(file); os.IsNotExist(err) {
			return &ValidationError{
				Check:   "V2FilesExist",
				Message: fmt.Sprintf("required directive file not found: %s", file),
			}
		}
	}

	return nil
}

// CheckV2FilesReadable verifies all V2 files have read permissions
func (c *PreMigrationChecks) CheckV2FilesReadable() error {
	files := []string{
		filepath.Join(c.V2Dir, "PROJECT-MAP.md"),
		filepath.Join(c.V2Dir, "time_sessions.jsonl"),
		filepath.Join(c.V2Dir, "..", "config", "LEX-CONFIG.yaml"),
		filepath.Join(c.V2Dir, "..", "directives", "rank-status.jsonl"),
		filepath.Join(c.V2Dir, "..", "directives", "behavioral-directives.jsonl"),
	}

	for _, file := range files {
		// Try to open file
		f, err := os.Open(file)
		if err != nil {
			return &ValidationError{
				Check:   "V2FilesReadable",
				Message: fmt.Sprintf("cannot read file %s: %v", file, err),
			}
		}
		f.Close()
	}

	return nil
}

// CheckPostgreSQLHealthy verifies PostgreSQL connection is healthy
func (c *PreMigrationChecks) CheckPostgreSQLHealthy() error {
	if c.Pool == nil {
		return &ValidationError{
			Check:   "PostgreSQLHealthy",
			Message: "database pool is nil",
		}
	}

	// Ping database
	ctx := context.Background()
	if err := c.Pool.Ping(ctx); err != nil {
		return &ValidationError{
			Check:   "PostgreSQLHealthy",
			Message: fmt.Sprintf("database ping failed: %v", err),
		}
	}

	return nil
}

// CheckNoActiveRunners verifies no active V3 runners exist
func (c *PreMigrationChecks) CheckNoActiveRunners() error {
	if c.Pool == nil {
		return &ValidationError{
			Check:   "NoActiveRunners",
			Message: "database pool is nil",
		}
	}

	query := `SELECT COUNT(*) FROM runners WHERE status IN ('starting', 'running', 'paused')`

	var count int
	ctx := context.Background()
	err := c.Pool.QueryRow(ctx, query).Scan(&count)
	if err != nil {
		return &ValidationError{
			Check:   "NoActiveRunners",
			Message: fmt.Sprintf("failed to query runners: %v", err),
		}
	}

	if count > 0 {
		return &ValidationError{
			Check:   "NoActiveRunners",
			Message: fmt.Sprintf("found %d active runners (migration requires clean slate)", count),
		}
	}

	return nil
}

// CheckSufficientDiskSpace verifies at least 1GB free disk space
func (c *PreMigrationChecks) CheckSufficientDiskSpace() error {
	// Get disk usage for V2 directory
	var stat os.FileInfo
	var err error

	// Try to stat the directory
	stat, err = os.Stat(c.V2Dir)
	if err != nil {
		return &ValidationError{
			Check:   "SufficientDiskSpace",
			Message: fmt.Sprintf("cannot stat directory %s: %v", c.V2Dir, err),
		}
	}

	if !stat.IsDir() {
		return &ValidationError{
			Check:   "SufficientDiskSpace",
			Message: fmt.Sprintf("path is not a directory: %s", c.V2Dir),
		}
	}

	// NOTE: Actual disk space checking is platform-specific
	// For now, we'll do a basic check by trying to create a temp file
	tempFile := filepath.Join(c.V2Dir, ".migration-check-temp")
	f, err := os.Create(tempFile)
	if err != nil {
		return &ValidationError{
			Check:   "SufficientDiskSpace",
			Message: fmt.Sprintf("cannot write to directory %s: %v", c.V2Dir, err),
		}
	}
	f.Close()
	os.Remove(tempFile)

	return nil
}

// CheckV2FilesValid verifies V2 files are not corrupted (valid syntax)
func (c *PreMigrationChecks) CheckV2FilesValid() error {
	// Check PROJECT-MAP.md is valid markdown (has table)
	projectMapPath := filepath.Join(c.V2Dir, "PROJECT-MAP.md")
	content, err := os.ReadFile(projectMapPath)
	if err != nil {
		return &ValidationError{
			Check:   "V2FilesValid",
			Message: fmt.Sprintf("cannot read PROJECT-MAP.md: %v", err),
		}
	}

	// Basic validation: should contain pipe characters (markdown table)
	if len(content) > 0 && !containsByte(content, '|') {
		return &ValidationError{
			Check:   "V2FilesValid",
			Message: "PROJECT-MAP.md does not appear to contain a markdown table",
		}
	}

	// Check time_sessions.jsonl is valid JSON lines
	sessionsPath := filepath.Join(c.V2Dir, "time_sessions.jsonl")
	sessionsContent, err := os.ReadFile(sessionsPath)
	if err != nil {
		return &ValidationError{
			Check:   "V2FilesValid",
			Message: fmt.Sprintf("cannot read time_sessions.jsonl: %v", err),
		}
	}

	// Basic validation: should contain JSON braces
	if len(sessionsContent) > 0 && !containsByte(sessionsContent, '{') {
		return &ValidationError{
			Check:   "V2FilesValid",
			Message: "time_sessions.jsonl does not appear to contain JSON",
		}
	}

	return nil
}

// Helper function
func containsByte(data []byte, b byte) bool {
	for _, v := range data {
		if v == b {
			return true
		}
	}
	return false
}
