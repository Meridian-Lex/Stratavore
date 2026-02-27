package daemon

import (
	"testing"

	"github.com/meridian-lex/stratavore/pkg/api"
)

func TestRunnerStatusToWebUIStatus(t *testing.T) {
	adapter := &AgentAdapter{}

	tests := []struct {
		runnerStatus string
		expected     string
	}{
		{"starting", "spawning"},
		{"running", "working"},
		{"paused", "paused"},
		{"terminated", "completed"},
		{"failed", "error"},
		{"unknown", "idle"},
	}

	for _, tt := range tests {
		t.Run(tt.runnerStatus, func(t *testing.T) {
			result := adapter.runnerStatusToWebUIStatus(tt.runnerStatus)
			if result != tt.expected {
				t.Errorf("runnerStatusToWebUIStatus(%q) = %q; want %q", tt.runnerStatus, result, tt.expected)
			}
		})
	}
}

func TestWebUIStatusToRunnerStatus(t *testing.T) {
	adapter := &AgentAdapter{}

	tests := []struct {
		webuiStatus string
		expected    string
	}{
		{"spawning", "starting"},
		{"working", "running"},
		{"paused", "paused"},
		{"completed", "terminated"},
		{"error", "failed"},
		{"idle", "running"},
		{"unknown", "running"},
	}

	for _, tt := range tests {
		t.Run(tt.webuiStatus, func(t *testing.T) {
			result := adapter.webuiStatusToRunnerStatus(tt.webuiStatus)
			if result != tt.expected {
				t.Errorf("webuiStatusToRunnerStatus(%q) = %q; want %q", tt.webuiStatus, result, tt.expected)
			}
		})
	}
}

func TestPersonalityToCapabilities(t *testing.T) {
	adapter := &AgentAdapter{}

	tests := []struct {
		personality string
		expected    []string
	}{
		{"cadet", []string{"basic"}},
		{"senior", []string{"advanced", "code-review"}},
		{"specialist", []string{"specialized"}},
		{"researcher", []string{"research", "analysis"}},
		{"debugger", []string{"debugging", "troubleshooting"}},
		{"optimizer", []string{"optimization", "performance"}},
		{"unknown", []string{"basic"}},
	}

	for _, tt := range tests {
		t.Run(tt.personality, func(t *testing.T) {
			result := adapter.personalityToCapabilities(tt.personality)
			if len(result) != len(tt.expected) {
				t.Errorf("personalityToCapabilities(%q) length = %d; want %d", tt.personality, len(result), len(tt.expected))
				return
			}
			for i := range result {
				if result[i] != tt.expected[i] {
					t.Errorf("personalityToCapabilities(%q)[%d] = %q; want %q", tt.personality, i, result[i], tt.expected[i])
				}
			}
		})
	}
}

func TestExtractIDFromPath(t *testing.T) {
	tests := []struct {
		path     string
		prefix   string
		suffix   string
		expected string
	}{
		{"/api/agents/123/status", "/api/agents/", "/status", "123"},
		{"/api/agents/abc-def/kill", "/api/agents/", "/kill", "abc-def"},
		{"/api/agents/xyz", "/api/agents/", "", "xyz"},
		{"/api/agents/", "/api/agents/", "", ""},
		{"/api/agents/123/assign", "/api/agents/", "/assign", "123"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			result := extractIDFromPath(tt.path, tt.prefix, tt.suffix)
			if result != tt.expected {
				t.Errorf("extractIDFromPath(%q, %q, %q) = %q; want %q", tt.path, tt.prefix, tt.suffix, result, tt.expected)
			}
		})
	}
}

func TestRunnerToAgent(t *testing.T) {
	adapter := &AgentAdapter{}

	runner := &api.Runner{
		ID:            "test-123",
		ProjectName:   "test-project",
		Status:        "running",
		TokensUsed:    1000,
		CPUPercent:    25.5,
		MemoryMB:      512,
		StartedAt:     "2026-02-21T12:00:00Z",
		LastHeartbeat: "2026-02-21T12:05:00Z",
		Environment: map[string]string{
			"PERSONALITY": "senior",
		},
	}

	agent := adapter.runnerToAgent(runner)

	if agent.AgentID != "test-123" {
		t.Errorf("AgentID = %q; want test-123", agent.AgentID)
	}
	if agent.Personality != "senior" {
		t.Errorf("Personality = %q; want senior", agent.Personality)
	}
	if agent.Status != "working" {
		t.Errorf("Status = %q; want working", agent.Status)
	}
	if agent.ProjectName != "test-project" {
		t.Errorf("ProjectName = %q; want test-project", agent.ProjectName)
	}
	if agent.TokensUsed != 1000 {
		t.Errorf("TokensUsed = %d; want 1000", agent.TokensUsed)
	}
}
