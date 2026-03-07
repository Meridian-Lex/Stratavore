package validator

import (
	"os"
	"path/filepath"
	"testing"
)

// createTestStructure creates the standard V2 directory layout in a temp dir.
// Returns stateDir (V2Dir) and directivesDir.
func createTestStructure(t *testing.T) (stateDir, directivesDir string) {
	t.Helper()
	tmpDir := t.TempDir()
	stateDir = filepath.Join(tmpDir, "lex-internal", "state")
	configDir := filepath.Join(tmpDir, "lex-internal", "config")
	directivesDir = filepath.Join(tmpDir, "lex-internal", "directives")

	for _, dir := range []string{stateDir, configDir, directivesDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("createTestStructure: %v", err)
		}
	}
	return stateDir, directivesDir
}

// writeFixture writes a file; fatals on error.
func writeFixture(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("writeFixture %s: %v", path, err)
	}
}

func TestCheckV2FilesExist(t *testing.T) {
	stateDir, directivesDir := createTestStructure(t)
	configDir := filepath.Join(stateDir, "..", "config")

	writeFixture(t, filepath.Join(stateDir, "PROJECT-MAP.md"), "test")
	writeFixture(t, filepath.Join(stateDir, "time_sessions.jsonl"), "test")
	writeFixture(t, filepath.Join(configDir, "LEX-CONFIG.yaml"), "test")
	writeFixture(t, filepath.Join(directivesDir, "rank-status.json"), `{"current_rank":"Ensign"}`)
	writeFixture(t, filepath.Join(directivesDir, "rank-events.jsonl"), "")
	writeFixture(t, filepath.Join(directivesDir, "behavioral-directives.jsonl"), "{}")

	checks := &PreMigrationChecks{V2Dir: stateDir}

	if err := checks.CheckV2FilesExist(); err != nil {
		t.Errorf("Expected no error when all files exist, got: %v", err)
	}
}

func TestCheckV2FilesExist_MissingFile(t *testing.T) {
	stateDir, _ := createTestStructure(t)

	// Only create PROJECT-MAP.md, missing time_sessions.jsonl
	writeFixture(t, filepath.Join(stateDir, "PROJECT-MAP.md"), "test")

	checks := &PreMigrationChecks{V2Dir: stateDir}

	err := checks.CheckV2FilesExist()
	if err == nil {
		t.Error("Expected error for missing file, got nil")
	}

	if _, ok := err.(*ValidationError); !ok {
		t.Errorf("Expected ValidationError, got %T", err)
	}
}

func TestCheckV2FilesReadable(t *testing.T) {
	stateDir, directivesDir := createTestStructure(t)
	configDir := filepath.Join(stateDir, "..", "config")

	writeFixture(t, filepath.Join(stateDir, "PROJECT-MAP.md"), "test")
	writeFixture(t, filepath.Join(stateDir, "time_sessions.jsonl"), "test")
	writeFixture(t, filepath.Join(configDir, "LEX-CONFIG.yaml"), "test")
	writeFixture(t, filepath.Join(directivesDir, "rank-status.json"), `{"current_rank":"Ensign"}`)
	writeFixture(t, filepath.Join(directivesDir, "rank-events.jsonl"), "")
	writeFixture(t, filepath.Join(directivesDir, "behavioral-directives.jsonl"), "{}")

	checks := &PreMigrationChecks{V2Dir: stateDir}

	if err := checks.CheckV2FilesReadable(); err != nil {
		t.Errorf("Expected no error for readable files, got: %v", err)
	}
}

func TestCheckSufficientDiskSpace(t *testing.T) {
	tmpDir := t.TempDir()

	checks := &PreMigrationChecks{V2Dir: tmpDir}

	if err := checks.CheckSufficientDiskSpace(); err != nil {
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
	stateDir, directivesDir := createTestStructure(t)

	writeFixture(t, filepath.Join(stateDir, "PROJECT-MAP.md"), "| Project | Status |\n|---------|--------|\n| test    | ACTIVE |\n")
	writeFixture(t, filepath.Join(stateDir, "time_sessions.jsonl"), `{"session_id":"test","project":"test"}`+"\n")
	writeFixture(t, filepath.Join(directivesDir, "rank-status.json"), `{"current_rank":"Ensign","strikes":0}`)
	writeFixture(t, filepath.Join(directivesDir, "rank-events.jsonl"), `{"type":"strike_event","date":"2026-01-01"}`+"\n")

	checks := &PreMigrationChecks{V2Dir: stateDir}

	if err := checks.CheckV2FilesValid(); err != nil {
		t.Errorf("Expected no error for valid files, got: %v", err)
	}
}

func TestCheckV2FilesValid_InvalidMarkdown(t *testing.T) {
	stateDir, directivesDir := createTestStructure(t)

	writeFixture(t, filepath.Join(stateDir, "PROJECT-MAP.md"), "Just plain text, no table")
	writeFixture(t, filepath.Join(stateDir, "time_sessions.jsonl"), "{}")
	writeFixture(t, filepath.Join(directivesDir, "rank-status.json"), `{"current_rank":"Ensign"}`)
	writeFixture(t, filepath.Join(directivesDir, "rank-events.jsonl"), "")

	checks := &PreMigrationChecks{V2Dir: stateDir}

	if err := checks.CheckV2FilesValid(); err == nil {
		t.Error("Expected error for invalid markdown table, got nil")
	}
}

func TestCheckV2FilesValid_InvalidJSON(t *testing.T) {
	stateDir, directivesDir := createTestStructure(t)

	writeFixture(t, filepath.Join(stateDir, "PROJECT-MAP.md"), "| A | B |")
	writeFixture(t, filepath.Join(stateDir, "time_sessions.jsonl"), "not json at all")
	writeFixture(t, filepath.Join(directivesDir, "rank-status.json"), `{"current_rank":"Ensign"}`)
	writeFixture(t, filepath.Join(directivesDir, "rank-events.jsonl"), "")

	checks := &PreMigrationChecks{V2Dir: stateDir}

	if err := checks.CheckV2FilesValid(); err == nil {
		t.Error("Expected error for invalid JSON, got nil")
	}
}

func TestCheckV2FilesValid_InvalidRankStatus(t *testing.T) {
	stateDir, directivesDir := createTestStructure(t)

	writeFixture(t, filepath.Join(stateDir, "PROJECT-MAP.md"), "| A | B |")
	writeFixture(t, filepath.Join(stateDir, "time_sessions.jsonl"), "{}")
	writeFixture(t, filepath.Join(directivesDir, "rank-status.json"), `{invalid json`)
	writeFixture(t, filepath.Join(directivesDir, "rank-events.jsonl"), "")

	checks := &PreMigrationChecks{V2Dir: stateDir}

	if err := checks.CheckV2FilesValid(); err == nil {
		t.Error("Expected error for invalid rank-status.json, got nil")
	}
}

func TestCheckV2FilesValid_InvalidRankEvents(t *testing.T) {
	stateDir, directivesDir := createTestStructure(t)

	writeFixture(t, filepath.Join(stateDir, "PROJECT-MAP.md"), "| A | B |")
	writeFixture(t, filepath.Join(stateDir, "time_sessions.jsonl"), "{}")
	writeFixture(t, filepath.Join(directivesDir, "rank-status.json"), `{"current_rank":"Ensign"}`)
	writeFixture(t, filepath.Join(directivesDir, "rank-events.jsonl"), "not valid json\n")

	checks := &PreMigrationChecks{V2Dir: stateDir}

	if err := checks.CheckV2FilesValid(); err == nil {
		t.Error("Expected error for invalid rank-events.jsonl, got nil")
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
	stateDir, directivesDir := createTestStructure(t)
	configDir := filepath.Join(stateDir, "..", "config")

	writeFixture(t, filepath.Join(stateDir, "PROJECT-MAP.md"), "| A | B |")
	writeFixture(t, filepath.Join(stateDir, "time_sessions.jsonl"), "{}")
	writeFixture(t, filepath.Join(configDir, "LEX-CONFIG.yaml"), "test: value")
	writeFixture(t, filepath.Join(directivesDir, "rank-status.json"), `{"current_rank":"Ensign","strikes":0}`)
	writeFixture(t, filepath.Join(directivesDir, "rank-events.jsonl"), "")
	writeFixture(t, filepath.Join(directivesDir, "behavioral-directives.jsonl"), "{}")

	checks := &PreMigrationChecks{
		V2Dir: stateDir,
		Pool:  nil, // Skip PostgreSQL checks
	}

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
