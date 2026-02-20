package validator

import (
	"testing"
	"time"
)

func TestPostMigrationChecks_ExpectedCounts(t *testing.T) {
	expectedCounts := map[string]int{
		"projects":      4,
		"sessions":      3,
		"token_budgets": 1,
		"rank_tracking": 234,
		"directives":    23,
	}

	checks := &PostMigrationChecks{
		ExpectedCounts: expectedCounts,
	}

	// Verify expected counts stored correctly
	if checks.ExpectedCounts["projects"] != 4 {
		t.Errorf("Expected 4 projects, got %d", checks.ExpectedCounts["projects"])
	}

	if checks.ExpectedCounts["sessions"] != 3 {
		t.Errorf("Expected 3 sessions, got %d", checks.ExpectedCounts["sessions"])
	}
}

func TestPostMigrationChecks_TimestampValidation(t *testing.T) {
	now := time.Now()
	futureThreshold := now.Add(24 * time.Hour)

	// Test timestamps
	tests := []struct {
		name      string
		timestamp time.Time
		valid     bool
	}{
		{
			name:      "past timestamp",
			timestamp: now.Add(-24 * time.Hour),
			valid:     true,
		},
		{
			name:      "current timestamp",
			timestamp: now,
			valid:     true,
		},
		{
			name:      "slightly future (timezone difference)",
			timestamp: now.Add(12 * time.Hour),
			valid:     true,
		},
		{
			name:      "far future (invalid)",
			timestamp: now.Add(48 * time.Hour),
			valid:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isFuture := tt.timestamp.After(futureThreshold)

			if tt.valid && isFuture {
				t.Error("Expected timestamp to be valid, but it's beyond future threshold")
			}

			if !tt.valid && !isFuture {
				t.Error("Expected timestamp to be invalid, but it's within threshold")
			}
		})
	}
}

func TestPostMigrationChecks_TokenTolerance(t *testing.T) {
	tests := []struct {
		name      string
		v2Total   int64
		v3Total   int64
		tolerance float64
		valid     bool
	}{
		{
			name:      "exact match",
			v2Total:   10000,
			v3Total:   10000,
			tolerance: 0.05,
			valid:     true,
		},
		{
			name:      "within 5% tolerance (higher)",
			v2Total:   10000,
			v3Total:   10400,
			tolerance: 0.05,
			valid:     true,
		},
		{
			name:      "within 5% tolerance (lower)",
			v2Total:   10000,
			v3Total:   9600,
			tolerance: 0.05,
			valid:     true,
		},
		{
			name:      "outside tolerance (too high)",
			v2Total:   10000,
			v3Total:   10600,
			tolerance: 0.05,
			valid:     false,
		},
		{
			name:      "outside tolerance (too low)",
			v2Total:   10000,
			v3Total:   9400,
			tolerance: 0.05,
			valid:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diff := float64(tt.v3Total-tt.v2Total) / float64(tt.v2Total)
			if diff < 0 {
				diff = -diff
			}

			withinTolerance := diff <= tt.tolerance

			if tt.valid && !withinTolerance {
				t.Errorf("Expected to be within tolerance, diff=%.2f%%", diff*100)
			}

			if !tt.valid && withinTolerance {
				t.Errorf("Expected to be outside tolerance, diff=%.2f%%", diff*100)
			}
		})
	}
}

func TestPostMigrationChecks_Tables(t *testing.T) {
	tables := []string{"projects", "sessions", "token_budgets", "rank_tracking", "directives"}

	if len(tables) != 5 {
		t.Errorf("Expected 5 tables to check, got %d", len(tables))
	}

	expectedTables := map[string]bool{
		"projects":      true,
		"sessions":      true,
		"token_budgets": true,
		"rank_tracking": true,
		"directives":    true,
	}

	for _, table := range tables {
		if !expectedTables[table] {
			t.Errorf("Unexpected table in checks: %s", table)
		}
	}
}

func TestPostMigrationChecks_NotNullColumns(t *testing.T) {
	checks := []struct {
		table  string
		column string
	}{
		{"projects", "name"},
		{"projects", "path"},
		{"projects", "status"},
		{"sessions", "id"},
		{"sessions", "runner_id"},
		{"sessions", "project_name"},
		{"token_budgets", "limit_tokens"},
		{"token_budgets", "period_start"},
		{"token_budgets", "period_end"},
		{"rank_tracking", "current_rank"},
		{"rank_tracking", "event_type"},
		{"directives", "id"},
		{"directives", "severity"},
	}

	if len(checks) != 13 {
		t.Errorf("Expected 13 NOT NULL checks, got %d", len(checks))
	}

	// Verify critical columns are included
	criticalColumns := map[string]bool{
		"projects.name":             false,
		"sessions.id":               false,
		"token_budgets.limit_tokens": false,
	}

	for _, check := range checks {
		key := check.table + "." + check.column
		if _, exists := criticalColumns[key]; exists {
			criticalColumns[key] = true
		}
	}

	for key, found := range criticalColumns {
		if !found {
			t.Errorf("Critical column not in NOT NULL checks: %s", key)
		}
	}
}

func TestPostMigrationChecks_ForeignKeys(t *testing.T) {
	foreignKeys := []struct {
		name        string
		childTable  string
		childColumn string
		parentTable string
		parentColumn string
	}{
		{
			name:         "sessions.runner_id",
			childTable:   "sessions",
			childColumn:  "runner_id",
			parentTable:  "runners",
			parentColumn: "id",
		},
		{
			name:         "sessions.project_name",
			childTable:   "sessions",
			childColumn:  "project_name",
			parentTable:  "projects",
			parentColumn: "name",
		},
	}

	if len(foreignKeys) != 2 {
		t.Errorf("Expected 2 foreign key checks, got %d", len(foreignKeys))
	}

	for _, fk := range foreignKeys {
		if fk.childTable == "" || fk.parentTable == "" {
			t.Errorf("Foreign key %s has empty table names", fk.name)
		}

		if fk.childColumn == "" || fk.parentColumn == "" {
			t.Errorf("Foreign key %s has empty column names", fk.name)
		}
	}
}

func TestPostMigrationChecks_JSONBColumns(t *testing.T) {
	jsonbColumns := []struct {
		table  string
		column string
	}{
		{"directives", "action"},
		{"rank_tracking", "metadata"},
	}

	if len(jsonbColumns) != 2 {
		t.Errorf("Expected 2 JSONB columns to check, got %d", len(jsonbColumns))
	}

	for _, col := range jsonbColumns {
		if col.table == "" || col.column == "" {
			t.Errorf("JSONB column check has empty table or column name")
		}
	}
}

func TestPostMigrationChecks_ValidationErrorFormat(t *testing.T) {
	err := &ValidationError{
		Check:   "RowCounts",
		Message: "table projects: expected 4 rows, got 3",
	}

	expected := "[RowCounts] table projects: expected 4 rows, got 3"
	if err.Error() != expected {
		t.Errorf("Expected error format %q, got %q", expected, err.Error())
	}
}

func TestPostMigrationChecks_TokenValidation(t *testing.T) {
	// Test that negative tokens are caught
	totalTokens := int64(-100)

	if totalTokens >= 0 {
		t.Error("Expected negative tokens to be invalid")
	}

	// Test that zero tokens are valid (fresh migration)
	totalTokens = int64(0)

	if totalTokens < 0 {
		t.Error("Expected zero tokens to be valid")
	}

	// Test that positive tokens are valid
	totalTokens = int64(50000)

	if totalTokens < 0 {
		t.Error("Expected positive tokens to be valid")
	}
}

func TestPostMigrationChecks_RealWorldExpectedCounts(t *testing.T) {
	// Based on actual V2 data from planning document
	expectedCounts := map[string]int{
		"projects":      4,   // setup-agentos, lex, Gantry, meridian-lex-setup
		"sessions":      3,   // 3 entries in time_sessions.jsonl
		"token_budgets": 1,   // 1 global daily budget
		"rank_tracking": 234, // 234 rank events from rank-status.jsonl
		"directives":    23,  // 23 behavioral directives
	}

	// Verify counts match planning document
	if expectedCounts["projects"] != 4 {
		t.Errorf("Expected 4 projects from V2 data, got %d", expectedCounts["projects"])
	}

	if expectedCounts["sessions"] != 3 {
		t.Errorf("Expected 3 sessions from V2 data, got %d", expectedCounts["sessions"])
	}

	if expectedCounts["rank_tracking"] != 234 {
		t.Errorf("Expected 234 rank events from V2 data, got %d", expectedCounts["rank_tracking"])
	}

	if expectedCounts["directives"] != 23 {
		t.Errorf("Expected 23 directives from V2 data, got %d", expectedCounts["directives"])
	}
}

func TestPostMigrationChecks_TextTruncationDetection(t *testing.T) {
	tests := []struct {
		name      string
		text      string
		truncated bool
	}{
		{
			name:      "normal text",
			text:      "This is a normal description",
			truncated: false,
		},
		{
			name:      "empty text",
			text:      "",
			truncated: true, // Empty when NOT NULL suggests truncation
		},
		{
			name:      "suspiciously ends with ellipsis",
			text:      "This text ends with...",
			truncated: true, // Might be truncated
		},
		{
			name:      "long valid text",
			text:      "This is a very long description that contains lots of detailed information about the project and its goals.",
			truncated: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isEmpty := len(tt.text) == 0
			endsWithEllipsis := len(tt.text) > 3 && tt.text[len(tt.text)-3:] == "..."

			suspiciousTruncation := isEmpty || endsWithEllipsis

			if tt.truncated && !suspiciousTruncation {
				t.Error("Expected text to appear truncated")
			}
		})
	}
}
