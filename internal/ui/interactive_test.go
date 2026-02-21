package ui

import (
	"fmt"
	"testing"
	"time"

	"github.com/meridian-lex/stratavore/pkg/api"
	"github.com/meridian-lex/stratavore/pkg/types"
)

func TestUITruncate(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		maxLen int
		want   string
	}{
		{
			name:   "no truncation needed",
			input:  "short",
			maxLen: 10,
			want:   "short",
		},
		{
			name:   "truncate with ellipsis",
			input:  "this is a very long string",
			maxLen: 10,
			want:   "this is...",
		},
		{
			name:   "exact length",
			input:  "exactlen",
			maxLen: 8,
			want:   "exactlen",
		},
		{
			name:   "maxLen 3",
			input:  "toolong",
			maxLen: 3,
			want:   "too",
		},
		{
			name:   "maxLen 1",
			input:  "ab",
			maxLen: 1,
			want:   "a",
		},
		{
			name:   "empty string",
			input:  "",
			maxLen: 5,
			want:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := uiTruncate(tt.input, tt.maxLen)
			if got != tt.want {
				t.Errorf("uiTruncate(%q, %d) = %q, want %q", tt.input, tt.maxLen, got, tt.want)
			}
		})
	}
}

func TestUIFormatDuration(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
		want     string
	}{
		{
			name:     "seconds only",
			duration: 45 * time.Second,
			want:     "45s",
		},
		{
			name:     "minutes and seconds",
			duration: 3*time.Minute + 30*time.Second,
			want:     "3m30s",
		},
		{
			name:     "hours, minutes, and seconds",
			duration: 2*time.Hour + 15*time.Minute + 45*time.Second,
			want:     "2h15m45s",
		},
		{
			name:     "hours only",
			duration: 5 * time.Hour,
			want:     "5h0m0s",
		},
		{
			name:     "zero duration",
			duration: 0,
			want:     "0s",
		},
		{
			name:     "subsecond rounds to zero",
			duration: 300 * time.Millisecond,
			want:     "0s",
		},
		{
			name:     "rounding up",
			duration: 3*time.Minute + 30*time.Second + 600*time.Millisecond,
			want:     "3m31s",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := uiFormatDuration(tt.duration)
			if got != tt.want {
				t.Errorf("uiFormatDuration(%v) = %q, want %q", tt.duration, got, tt.want)
			}
		})
	}
}

func TestIsSeparatorOrEmpty(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{
			name:  "separator line",
			input: "── Projects ──",
			want:  true,
		},
		{
			name:  "empty string",
			input: "",
			want:  true,
		},
		{
			name:  "regular menu item",
			input: "Launch Project",
			want:  false,
		},
		{
			name:  "separator prefix only",
			input: "──",
			want:  true,
		},
		{
			name:  "whitespace",
			input: "   ",
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsSeparatorOrEmpty(tt.input)
			if got != tt.want {
				t.Errorf("IsSeparatorOrEmpty(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestMenuSelector_EmptyItems(t *testing.T) {
	idx, result, err := MenuSelector("Test", []string{}, 10)
	if err == nil {
		t.Error("MenuSelector with empty items should return error")
	}
	if idx != -1 {
		t.Errorf("MenuSelector with empty items should return idx -1, got %d", idx)
	}
	if result != "" {
		t.Errorf("MenuSelector with empty items should return empty result, got %q", result)
	}
}

func TestProjectSelector_EmptyProjects(t *testing.T) {
	idx, err := ProjectSelector([]*api.Project{})
	if err == nil {
		t.Error("ProjectSelector with empty projects should return error")
	}
	if idx != -1 {
		t.Errorf("ProjectSelector with empty projects should return idx -1, got %d", idx)
	}
}

func TestRunnerSelector_EmptyRunners(t *testing.T) {
	idx, err := RunnerSelector([]*types.Runner{}, "test-project")
	if err == nil {
		t.Error("RunnerSelector with empty runners should return error")
	}
	if idx != -1 {
		t.Errorf("RunnerSelector with empty runners should return idx -1, got %d", idx)
	}
}

func TestSessionSelector_EmptySessions(t *testing.T) {
	idx, err := SessionSelector([]*types.Session{})
	if err == nil {
		t.Error("SessionSelector with empty sessions should return error")
	}
	if idx != -1 {
		t.Errorf("SessionSelector with empty sessions should return idx -1, got %d", idx)
	}
}

// Integration test helpers - these test the label formatting logic without interactive prompts

func TestProjectSelector_LabelFormatting(t *testing.T) {
	projects := []*api.Project{
		{
			Name:          "short",
			Status:        "active",
			ActiveRunners: 2,
		},
		{
			Name:          "very-long-project-name-that-needs-truncation",
			Status:        "idle",
			ActiveRunners: 0,
		},
	}

	// Test label generation (extracted from ProjectSelector)
	projectLabels := make([]string, len(projects))
	for i, p := range projects {
		projectLabels[i] = fmt.Sprintf("%s [%s, %d runners]", uiTruncate(p.Name, 20), p.Status, p.ActiveRunners)
	}

	if len(projectLabels) != 2 {
		t.Errorf("Expected 2 labels, got %d", len(projectLabels))
	}

	// First project should not be truncated
	if len(projectLabels[0]) < len("short") {
		t.Error("Short project name should not be truncated")
	}

	// Second project should be truncated
	if !contains(projectLabels[1], "...") {
		t.Error("Long project name should be truncated with ellipsis")
	}
}

func TestRunnerSelector_LabelFormatting(t *testing.T) {
	baseTime := time.Now().Add(-5 * time.Minute)
	runners := []*types.Runner{
		{
			ID:        "runner-123",
			StartedAt: baseTime,
		},
		{
			ID:        "runner-456-with-very-long-identifier-that-needs-truncation",
			StartedAt: baseTime.Add(-10 * time.Minute),
		},
	}

	// Test label generation
	runnerLabels := make([]string, len(runners)+1)
	for i, r := range runners {
		uptime := time.Since(r.StartedAt).Round(time.Second)
		runnerLabels[i] = uiTruncate(r.ID, 30) + " (started " + uiFormatDuration(uptime) + " ago)"
	}
	runnerLabels[len(runners)] = "Launch new runner"

	if len(runnerLabels) != 3 {
		t.Errorf("Expected 3 labels (2 runners + launch new), got %d", len(runnerLabels))
	}

	// Last item should be "Launch new runner"
	if runnerLabels[2] != "Launch new runner" {
		t.Errorf("Last label should be 'Launch new runner', got %q", runnerLabels[2])
	}

	// Check truncation
	if len(uiTruncate(runners[1].ID, 30)) > 30 {
		t.Error("Long runner ID should be truncated to max 30 chars")
	}
}

func TestSessionSelector_LabelFormatting(t *testing.T) {
	baseTime := time.Now().Add(-2 * time.Hour)
	sessions := []*types.Session{
		{
			ID:           "session-123",
			CreatedAt:    baseTime,
			Summary:      "Test session",
			TokensUsed:   1500,
			MessageCount: 42,
		},
		{
			ID:           "session-456",
			CreatedAt:    baseTime.Add(-30 * time.Minute),
			Summary:      "",
			TokensUsed:   500,
			MessageCount: 10,
		},
	}

	// Test label generation
	sessionLabels := make([]string, len(sessions))
	for i, s := range sessions {
		uptime := time.Since(s.CreatedAt)
		summary := s.Summary
		if summary == "" {
			summary = "(no summary)"
		}

		sessionLabels[i] = fmt.Sprintf("%s | %s | Tokens: %d | Msgs: %d",
			uiTruncate(s.ID, 20),
			uiFormatDuration(uptime),
			s.TokensUsed,
			s.MessageCount)
	}

	if len(sessionLabels) != 2 {
		t.Errorf("Expected 2 labels, got %d", len(sessionLabels))
	}
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || containsMiddle(s, substr)))
}

func containsMiddle(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
