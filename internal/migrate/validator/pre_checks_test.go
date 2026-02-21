package validator

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCheckV2FilesExist(t *testing.T) {
	// Create temporary test directory
	tmpDir := t.TempDir()
	stateDir := filepath.Join(tmpDir, "lex-internal", "state")
	configDir := filepath.Join(tmpDir, "lex-internal", "config")
	directivesDir := filepath.Join(tmpDir, "lex-internal", "directives")

	os.MkdirAll(stateDir, 0755)
	os.MkdirAll(configDir, 0755)
	os.MkdirAll(directivesDir, 0755)

	// Create required files
	os.WriteFile(filepath.Join(stateDir, "PROJECT-MAP.md"), []byte("test"), 0644)
	os.WriteFile(filepath.Join(stateDir, "time_sessions.jsonl"), []byte("test"), 0644)
	os.WriteFile(filepath.Join(configDir, "LEX-CONFIG.yaml"), []byte("test"), 0644)
	os.WriteFile(filepath.Join(directivesDir, "rank-status.jsonl"), []byte("test"), 0644)
	os.WriteFile(filepath.Join(directivesDir, "behavioral-directives.jsonl"), []byte("test"), 0644)

	checks := &PreMigrationChecks{
		V2Dir: stateDir,
	}

	err := checks.CheckV2FilesExist()
	if err != nil {
		t.Errorf("Expected no error when all files exist, got: %v", err)
	}
}

func TestCheckV2FilesExist_MissingFile(t *testing.T) {
	tmpDir := t.TempDir()
	stateDir := filepath.Join(tmpDir, "lex-internal", "state")
	os.MkdirAll(stateDir, 0755)

	// Only create PROJECT-MAP.md, missing time_sessions.jsonl
	os.WriteFile(filepath.Join(stateDir, "PROJECT-MAP.md"), []byte("test"), 0644)

	checks := &PreMigrationChecks{
		V2Dir: stateDir,
	}

	err := checks.CheckV2FilesExist()
	if err == nil {
		t.Error("Expected error for missing file, got nil")
	}

	if _, ok := err.(*ValidationError); !ok {
		t.Errorf("Expected ValidationError, got %T", err)
	}
}

func TestCheckV2FilesReadable(t *testing.T) {
	tmpDir := t.TempDir()
	stateDir := filepath.Join(tmpDir, "lex-internal", "state")
	configDir := filepath.Join(tmpDir, "lex-internal", "config")
	directivesDir := filepath.Join(tmpDir, "lex-internal", "directives")

	os.MkdirAll(stateDir, 0755)
	os.MkdirAll(configDir, 0755)
	os.MkdirAll(directivesDir, 0755)

	// Create files with read permissions
	os.WriteFile(filepath.Join(stateDir, "PROJECT-MAP.md"), []byte("test"), 0644)
	os.WriteFile(filepath.Join(stateDir, "time_sessions.jsonl"), []byte("test"), 0644)
	os.WriteFile(filepath.Join(configDir, "LEX-CONFIG.yaml"), []byte("test"), 0644)
	os.WriteFile(filepath.Join(directivesDir, "rank-status.jsonl"), []byte("test"), 0644)
	os.WriteFile(filepath.Join(directivesDir, "behavioral-directives.jsonl"), []byte("test"), 0644)

	checks := &PreMigrationChecks{
		V2Dir: stateDir,
	}

	err := checks.CheckV2FilesReadable()
	if err != nil {
		t.Errorf("Expected no error for readable files, got: %v", err)
	}
}

func TestCheckSufficientDiskSpace(t *testing.T) {
	tmpDir := t.TempDir()

	checks := &PreMigrationChecks{
		V2Dir: tmpDir,
	}

	err := checks.CheckSufficientDiskSpace()
	if err != nil {
		t.Errorf("Expected no error for sufficient disk space, got: %v", err)
	}
}

func TestCheckSufficientDiskSpace_InvalidDir(t *testing.T) {
	checks := &PreMigrationChecks{
		V2Dir: "/nonexistent/directory/that/does/not/exist",
	}

	err := checks.CheckSufficientDiskSpace()
	if err == nil {
		t.Error("Expected error for nonexistent directory, got nil")
	}

	if _, ok := err.(*ValidationError); !ok {
		t.Errorf("Expected ValidationError, got %T", err)
	}
}

func TestCheckV2FilesValid(t *testing.T) {
	tmpDir := t.TempDir()

	// Create valid PROJECT-MAP.md with markdown table
	os.WriteFile(filepath.Join(tmpDir, "PROJECT-MAP.md"), []byte(`
| Project | Status |
|---------|--------|
| test    | ACTIVE |
`), 0644)

	// Create valid time_sessions.jsonl
	os.WriteFile(filepath.Join(tmpDir, "time_sessions.jsonl"), []byte(`{"session_id":"test","project":"test"}
`), 0644)

	checks := &PreMigrationChecks{
		V2Dir: tmpDir,
	}

	err := checks.CheckV2FilesValid()
	if err != nil {
		t.Errorf("Expected no error for valid files, got: %v", err)
	}
}

func TestCheckV2FilesValid_InvalidMarkdown(t *testing.T) {
	tmpDir := t.TempDir()

	// Create PROJECT-MAP.md without table (no pipe characters)
	os.WriteFile(filepath.Join(tmpDir, "PROJECT-MAP.md"), []byte("Just plain text, no table"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "time_sessions.jsonl"), []byte("{}"), 0644)

	checks := &PreMigrationChecks{
		V2Dir: tmpDir,
	}

	err := checks.CheckV2FilesValid()
	if err == nil {
		t.Error("Expected error for invalid markdown table, got nil")
	}
}

func TestCheckV2FilesValid_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()

	// Create valid PROJECT-MAP.md
	os.WriteFile(filepath.Join(tmpDir, "PROJECT-MAP.md"), []byte("| A | B |"), 0644)

	// Create time_sessions.jsonl without JSON braces
	os.WriteFile(filepath.Join(tmpDir, "time_sessions.jsonl"), []byte("not json at all"), 0644)

	checks := &PreMigrationChecks{
		V2Dir: tmpDir,
	}

	err := checks.CheckV2FilesValid()
	if err == nil {
		t.Error("Expected error for invalid JSON, got nil")
	}
}

func TestValidationError_Error(t *testing.T) {
	err := &ValidationError{
		Check:   "TestCheck",
		Message: "Test message",
	}

	expected := "[TestCheck] Test message"
	if err.Error() != expected {
		t.Errorf("Expected error message %q, got %q", expected, err.Error())
	}
}

func TestValidateAll_AllChecksPass(t *testing.T) {
	tmpDir := t.TempDir()
	stateDir := filepath.Join(tmpDir, "lex-internal", "state")
	configDir := filepath.Join(tmpDir, "lex-internal", "config")
	directivesDir := filepath.Join(tmpDir, "lex-internal", "directives")

	os.MkdirAll(stateDir, 0755)
	os.MkdirAll(configDir, 0755)
	os.MkdirAll(directivesDir, 0755)

	// Create all required files with valid content
	os.WriteFile(filepath.Join(stateDir, "PROJECT-MAP.md"), []byte("| A | B |"), 0644)
	os.WriteFile(filepath.Join(stateDir, "time_sessions.jsonl"), []byte("{}"), 0644)
	os.WriteFile(filepath.Join(configDir, "LEX-CONFIG.yaml"), []byte("test: value"), 0644)
	os.WriteFile(filepath.Join(directivesDir, "rank-status.jsonl"), []byte("{}"), 0644)
	os.WriteFile(filepath.Join(directivesDir, "behavioral-directives.jsonl"), []byte("{}"), 0644)

	checks := &PreMigrationChecks{
		V2Dir: stateDir,
		Pool:  nil, // Skip PostgreSQL checks
	}

	// Run checks except PostgreSQL-dependent ones
	var errors []error

	if err := checks.CheckV2FilesExist(); err != nil {
		errors = append(errors, err)
	}
	if err := checks.CheckV2FilesReadable(); err != nil {
		errors = append(errors, err)
	}
	if err := checks.CheckSufficientDiskSpace(); err != nil {
		errors = append(errors, err)
	}
	if err := checks.CheckV2FilesValid(); err != nil {
		errors = append(errors, err)
	}

	if len(errors) > 0 {
		t.Errorf("Expected no errors, got %d: %v", len(errors), errors)
	}
}

func TestContainsByte(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		b    byte
		want bool
	}{
		{"contains pipe", []byte("hello | world"), '|', true},
		{"contains brace", []byte("{\"key\":\"value\"}"), '{', true},
		{"does not contain", []byte("hello world"), '|', false},
		{"empty data", []byte(""), 'x', false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := containsByte(tt.data, tt.b)
			if got != tt.want {
				t.Errorf("containsByte() = %v, want %v", got, tt.want)
			}
		})
	}
}
