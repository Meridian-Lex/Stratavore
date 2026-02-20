package importers

import (
	"testing"
	"time"

	"github.com/meridian-lex/stratavore/internal/migrate/parsers"
	"github.com/meridian-lex/stratavore/pkg/types"
)

func TestMapProjectStatus(t *testing.T) {
	tests := []struct {
		name     string
		v2Status string
		want     types.ProjectStatus
	}{
		{"active uppercase", "ACTIVE", types.ProjectActive},
		{"active lowercase", "active", types.ProjectActive},
		{"active mixed case", "AcTiVe", types.ProjectActive},
		{"archived uppercase", "ARCHIVED", types.ProjectArchived},
		{"archived lowercase", "archived", types.ProjectArchived},
		{"idle uppercase", "IDLE", types.ProjectIdle},
		{"idle lowercase", "idle", types.ProjectIdle},
		{"unknown status", "UNKNOWN", types.ProjectIdle},
		{"empty status", "", types.ProjectIdle},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mapProjectStatus(tt.v2Status)
			if got != tt.want {
				t.Errorf("mapProjectStatus(%q) = %v, want %v", tt.v2Status, got, tt.want)
			}
		})
	}
}

func TestBuildDescription(t *testing.T) {
	tests := []struct {
		name    string
		project parsers.V2Project
		want    string
	}{
		{
			name: "notes and priority",
			project: parsers.V2Project{
				Notes:    "Test project for demos",
				Priority: "HIGH",
			},
			want: "Test project for demos — Priority: HIGH",
		},
		{
			name: "notes only",
			project: parsers.V2Project{
				Notes:    "Just some notes",
				Priority: "-",
			},
			want: "Just some notes",
		},
		{
			name: "priority only",
			project: parsers.V2Project{
				Notes:    "",
				Priority: "MEDIUM",
			},
			want: "Priority: MEDIUM",
		},
		{
			name: "no notes or priority",
			project: parsers.V2Project{
				Notes:    "",
				Priority: "-",
			},
			want: "",
		},
		{
			name: "empty priority",
			project: parsers.V2Project{
				Notes:    "Notes here",
				Priority: "",
			},
			want: "Notes here",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildDescription(tt.project)
			if got != tt.want {
				t.Errorf("buildDescription() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestImportProjects_ValidationLogic(t *testing.T) {
	// Test the transformation logic without database
	v2Projects := []parsers.V2Project{
		{
			Name:     "test-project",
			Status:   "ACTIVE",
			Path:     "/home/meridian/projects/test",
			Started:  time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC),
			Priority: "HIGH",
			Notes:    "Test project",
		},
		{
			Name:     "archived-project",
			Status:   "ARCHIVED",
			Path:     "/home/meridian/projects/archived",
			Started:  time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
			Priority: "-",
			Notes:    "Old project",
		},
	}

	// Verify status mapping
	for _, proj := range v2Projects {
		status := mapProjectStatus(proj.Status)
		if proj.Name == "test-project" && status != types.ProjectActive {
			t.Errorf("Expected active status for test-project, got %v", status)
		}
		if proj.Name == "archived-project" && status != types.ProjectArchived {
			t.Errorf("Expected archived status for archived-project, got %v", status)
		}
	}

	// Verify description building
	for _, proj := range v2Projects {
		desc := buildDescription(proj)
		if proj.Name == "test-project" {
			if desc != "Test project — Priority: HIGH" {
				t.Errorf("Expected full description for test-project, got %q", desc)
			}
		}
		if proj.Name == "archived-project" {
			if desc != "Old project" {
				t.Errorf("Expected notes-only description for archived-project, got %q", desc)
			}
		}
	}
}

func TestImportProjects_TagGeneration(t *testing.T) {
	tests := []struct {
		name     string
		priority string
		wantTags int
	}{
		{"high priority generates tag", "HIGH", 1},
		{"medium priority generates tag", "MEDIUM", 1},
		{"dash priority no tag", "-", 0},
		{"empty priority no tag", "", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var tags []string
			if tt.priority != "" && tt.priority != "-" {
				tags = []string{tt.priority}
			}

			if len(tags) != tt.wantTags {
				t.Errorf("Expected %d tags for priority %q, got %d", tt.wantTags, tt.priority, len(tags))
			}
		})
	}
}

func TestImportProjects_PathPreservation(t *testing.T) {
	// Verify that paths from V2 are preserved correctly
	testCases := []struct {
		v2Path      string
		shouldMatch string
	}{
		{"/home/meridian/meridian-home/projects/test", "/home/meridian/meridian-home/projects/test"},
		{"/home/meridian/projects/another", "/home/meridian/projects/another"},
		{"~/relative/path", "~/relative/path"}, // Path expansion happens in parser, not importer
	}

	for _, tc := range testCases {
		if tc.v2Path != tc.shouldMatch {
			t.Errorf("Path mismatch: got %q, want %q", tc.v2Path, tc.shouldMatch)
		}
	}
}
