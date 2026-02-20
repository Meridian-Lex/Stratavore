package parsers

import (
	"strings"
	"testing"
	"time"
)

func TestParseProjectMap_ValidMarkdown(t *testing.T) {
	md := `# Project Map

## Project Registry

| Project Name | Status | Path | Started | Priority | Notes |
|-------------|--------|------|---------|----------|-------|
| test-proj | ACTIVE | ~/projects/test | 2026-01-01 | HIGH | Test project |
| another-proj | ARCHIVED | ~/projects/another | 2026-02-15 | MEDIUM | Another test |
`

	projects, err := ParseProjectMap(md)
	if err != nil {
		t.Fatalf("ParseProjectMap failed: %v", err)
	}

	if len(projects) != 2 {
		t.Fatalf("Expected 2 projects, got %d", len(projects))
	}

	// Verify first project
	if projects[0].Name != "test-proj" {
		t.Errorf("Expected name 'test-proj', got '%s'", projects[0].Name)
	}
	if projects[0].Status != "ACTIVE" {
		t.Errorf("Expected status 'ACTIVE', got '%s'", projects[0].Status)
	}
	if !strings.HasSuffix(projects[0].Path, "/projects/test") {
		t.Errorf("Expected path to end with '/projects/test', got '%s'", projects[0].Path)
	}
	if projects[0].Priority != "HIGH" {
		t.Errorf("Expected priority 'HIGH', got '%s'", projects[0].Priority)
	}
	if projects[0].Notes != "Test project" {
		t.Errorf("Expected notes 'Test project', got '%s'", projects[0].Notes)
	}

	// Verify started date
	expectedDate := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if !projects[0].Started.Equal(expectedDate) {
		t.Errorf("Expected started date %v, got %v", expectedDate, projects[0].Started)
	}
}

func TestParseProjectMap_EmptyContent(t *testing.T) {
	projects, err := ParseProjectMap("")
	if err != nil {
		t.Fatalf("ParseProjectMap failed on empty content: %v", err)
	}

	if len(projects) != 0 {
		t.Errorf("Expected 0 projects from empty content, got %d", len(projects))
	}
}

func TestParseProjectMap_NoTables(t *testing.T) {
	md := `# Project Map

This is just some text without any tables.

## Section Header

More text here.
`

	projects, err := ParseProjectMap(md)
	if err != nil {
		t.Fatalf("ParseProjectMap failed: %v", err)
	}

	if len(projects) != 0 {
		t.Errorf("Expected 0 projects from content without tables, got %d", len(projects))
	}
}

func TestParseProjectMap_MultipleTables(t *testing.T) {
	md := `# Project Map

## Active Projects

| Project Name | Status | Path | Started | Priority | Notes |
|-------------|--------|------|---------|----------|-------|
| active-one | ACTIVE | ~/active/one | 2026-01-01 | HIGH | Active project |

## Archived Projects

| Project Name | Status | Path | Started | Priority | Notes |
|-------------|--------|------|---------|----------|-------|
| archived-one | ARCHIVED | ~/archived/one | 2025-12-01 | - | Old project |
| archived-two | ARCHIVED | ~/archived/two | 2025-11-15 | LOW | Another old one |
`

	projects, err := ParseProjectMap(md)
	if err != nil {
		t.Fatalf("ParseProjectMap failed: %v", err)
	}

	if len(projects) != 3 {
		t.Fatalf("Expected 3 projects from multiple tables, got %d", len(projects))
	}

	// Verify we got all three
	names := make(map[string]bool)
	for _, p := range projects {
		names[p.Name] = true
	}

	expected := []string{"active-one", "archived-one", "archived-two"}
	for _, name := range expected {
		if !names[name] {
			t.Errorf("Expected to find project '%s', but it was not parsed", name)
		}
	}
}

func TestParseProjectMap_MissingDate(t *testing.T) {
	md := `| Project Name | Status | Path | Started | Priority | Notes |
|-------------|--------|------|---------|----------|-------|
| no-date | ACTIVE | ~/projects/nodate | - | HIGH | No start date |
`

	projects, err := ParseProjectMap(md)
	if err != nil {
		t.Fatalf("ParseProjectMap failed: %v", err)
	}

	if len(projects) != 1 {
		t.Fatalf("Expected 1 project, got %d", len(projects))
	}

	// Zero time for missing date
	if !projects[0].Started.IsZero() {
		t.Errorf("Expected zero time for missing date, got %v", projects[0].Started)
	}
}

func TestParseProjectMap_InvalidDate(t *testing.T) {
	md := `| Project Name | Status | Path | Started | Priority | Notes |
|-------------|--------|------|---------|----------|-------|
| bad-date | ACTIVE | ~/projects/baddate | not-a-date | HIGH | Invalid date format |
`

	projects, err := ParseProjectMap(md)
	if err != nil {
		t.Fatalf("ParseProjectMap failed: %v", err)
	}

	if len(projects) != 1 {
		t.Fatalf("Expected 1 project (should handle invalid date gracefully), got %d", len(projects))
	}

	// Should use zero time for unparseable date
	if !projects[0].Started.IsZero() {
		t.Errorf("Expected zero time for invalid date, got %v", projects[0].Started)
	}
}

func TestParseProjectMap_RealWorldExample(t *testing.T) {
	// This is the actual format from the real PROJECT-MAP.md
	md := `# Meridian Lex - Project Map

**Last Updated**: 2026-02-13
**Status**: Operational

---

## Project Registry

| Project Name | Status | Path | Started | Priority | Notes |
|-------------|--------|------|---------|----------|-------|
| lex | ARCHIVED | ~/meridian-home/projects/lex | 2026-02-06 | - | Superseded by Stratavore (v3). Archived 2026-02-13. |
| setup-agentos | ACTIVE | ~/meridian-home/projects/setup-agentos | 2026-02-06 | HIGH | Agent OS integration utilities |
| Gantry | ACTIVE | ~/meridian-home/projects/lex-docker | 2026-02-07 | MEDIUM | Docker infrastructure suite |
| meridian-lex-setup | ARCHIVED | ~/lex/meridian-lex-setup | 2026-02-06 | - | Initial vessel setup (complete) |

---

## Archived Projects

| Project Name | Archived Date | Notes |
|-------------|---------------|-------|
| lex | 2026-02-13 | V1/V2 bash-based launcher. |
`

	projects, err := ParseProjectMap(md)
	if err != nil {
		t.Fatalf("ParseProjectMap failed on real-world example: %v", err)
	}

	// Should parse the main table (4 projects), not the "Archived Projects" table (different format)
	if len(projects) != 4 {
		t.Fatalf("Expected 4 projects from main registry table, got %d", len(projects))
	}

	// Verify specific projects
	foundLex := false
	foundSetup := false
	for _, p := range projects {
		if p.Name == "lex" {
			foundLex = true
			if p.Status != "ARCHIVED" {
				t.Errorf("Expected lex status 'ARCHIVED', got '%s'", p.Status)
			}
		}
		if p.Name == "setup-agentos" {
			foundSetup = true
			if p.Status != "ACTIVE" {
				t.Errorf("Expected setup-agentos status 'ACTIVE', got '%s'", p.Status)
			}
			if p.Priority != "HIGH" {
				t.Errorf("Expected setup-agentos priority 'HIGH', got '%s'", p.Priority)
			}
		}
	}

	if !foundLex {
		t.Error("Did not find 'lex' project in parsed results")
	}
	if !foundSetup {
		t.Error("Did not find 'setup-agentos' project in parsed results")
	}
}

func TestParseProjectMap_ExtraWhitespace(t *testing.T) {
	md := `|   Project Name   |  Status  |    Path     | Started    |  Priority  |   Notes   |
|------------------|----------|-------------|------------|------------|-----------|
|  spaced-proj     |  ACTIVE  | ~/proj/test | 2026-01-01 |    HIGH    | Has spaces |
`

	projects, err := ParseProjectMap(md)
	if err != nil {
		t.Fatalf("ParseProjectMap failed: %v", err)
	}

	if len(projects) != 1 {
		t.Fatalf("Expected 1 project, got %d", len(projects))
	}

	// Should trim whitespace correctly
	if projects[0].Name != "spaced-proj" {
		t.Errorf("Expected trimmed name 'spaced-proj', got '%s'", projects[0].Name)
	}
	if projects[0].Status != "ACTIVE" {
		t.Errorf("Expected trimmed status 'ACTIVE', got '%s'", projects[0].Status)
	}
}
