package parsers

import (
	"testing"
	"time"
)

func TestParseLexConfig_ValidYAML(t *testing.T) {
	yaml := `mode:
  current: "AUTONOMOUS"
  description: "Test mode"

token_budget:
  daily_limit: 100000
  per_session_target: 20000
  reserved_for_commander: 30000
  tracking:
    today_used: 0
    last_reset: "2026-02-06"

paths:
  home: "/home/meridian/meridian-home"
  projects: "/home/meridian/meridian-home/projects"

claude:
  default_flags: ""
  presets:
    full-access: "--dangerously-skip-permissions"
    plan-mode: "--permission-mode plan"
  quick_flags:
    skip_permissions: false
    allow_skip: false
`

	config, err := ParseLexConfig(yaml)
	if err != nil {
		t.Fatalf("ParseLexConfig failed: %v", err)
	}

	// Verify mode
	if config.Mode.Current != "AUTONOMOUS" {
		t.Errorf("Expected mode 'AUTONOMOUS', got '%s'", config.Mode.Current)
	}

	// Verify token budget
	if config.TokenBudget.DailyLimit != 100000 {
		t.Errorf("Expected daily_limit 100000, got %d", config.TokenBudget.DailyLimit)
	}
	if config.TokenBudget.PerSessionTarget != 20000 {
		t.Errorf("Expected per_session_target 20000, got %d", config.TokenBudget.PerSessionTarget)
	}
	if config.TokenBudget.ReservedForCommander != 30000 {
		t.Errorf("Expected reserved_for_commander 30000, got %d", config.TokenBudget.ReservedForCommander)
	}
	if config.TokenBudget.Tracking.TodayUsed != 0 {
		t.Errorf("Expected tracking.today_used 0, got %d", config.TokenBudget.Tracking.TodayUsed)
	}

	// Verify paths
	if config.Paths.Home != "/home/meridian/meridian-home" {
		t.Errorf("Expected home path, got '%s'", config.Paths.Home)
	}

	// Verify runner presets
	// IDENTITY-EXCEPTION: V2Config.Lex field references V2 YAML schema
	if config.Claude.Presets.FullAccess != "--dangerously-skip-permissions" {
		t.Errorf("Expected full-access preset, got '%s'", config.Claude.Presets.FullAccess)
	}
}

func TestParseLexConfig_RealWorldExample(t *testing.T) {
	yaml := `mode:
  current: "AUTONOMOUS"
  description: "Awaiting first assignment"

scheduling:
  enabled: false
  todo_check_interval: 7200
  last_check: null
  next_check: null

autonomous_mode:
  enabled: false
  max_daily_tokens: 50000
  work_hours:
    start: "09:00"
    end: "17:00"
  work_pace: "steady"
  priorities:
    - "Bug fixes and critical issues"
    - "Test coverage and documentation"

development:
  commit_style: "conventional"
  testing_required: true
  documentation_required: true
  code_review_self: true
  standards:
    - "Senior software engineer best practices"
    - "Clean, maintainable, well-documented code"

claude:
  default_flags: ""
  presets:
    full-access: "--dangerously-skip-permissions"
    plan-mode: "--permission-mode plan --allow-dangerously-skip-permissions"
    auto-git: "--allowedTools \"Bash(git *)\" \"Read\" \"Write\" \"Edit\""
  quick_flags:
    skip_permissions: false
    allow_skip: false
  system_prompt:
    append: ""
    append_file: ""

token_budget:
  daily_limit: 100000
  per_session_target: 20000
  reserved_for_commander: 30000
  tracking:
    today_used: 0
    last_reset: "2026-02-06"

notifications:
  completion_reports: true
  error_alerts: true
  progress_updates: false
  daily_summary: true

paths:
  home: "/home/meridian/meridian-home"
  projects: "/home/meridian/meridian-home/projects"
  archive: "/home/meridian/meridian-home/archive"
  docs: "/home/meridian/meridian-home/docs"
  logs: "/home/meridian/meridian-home/logs"
  lex_internal: "/home/meridian/meridian-home/lex-internal"
  lex_state: "/home/meridian/meridian-home/lex-internal/state"
  lex_config: "/home/meridian/meridian-home/lex-internal/config"

github_monitoring:
  enabled: true
  poll_interval: 600
  state_file: "/home/meridian/meridian-home/lex-internal/state/github-monitor-state.json"
  log_file: "/home/meridian/meridian-home/logs/github-activity.log"
  repositories:
    - "Meridian-Lex/Lex-webui"
    - "Meridian-Lex/lex"
  tracked_users:
    - "LunarLaurus"
  events:
    - "pull_request_reaction"
    - "issue_comment"

metadata:
  vessel_id: "meridian-lex-001"
  operator: "Fleet Admiral Lunar Laurus"
  commissioned: "2026-02-06"
  config_version: "1.0"
`

	config, err := ParseLexConfig(yaml)
	if err != nil {
		t.Fatalf("ParseLexConfig failed on real-world example: %v", err)
	}

	// Verify autonomous mode settings
	if config.AutonomousMode.Enabled {
		t.Error("Expected autonomous_mode.enabled to be false")
	}
	if config.AutonomousMode.MaxDailyTokens != 50000 {
		t.Errorf("Expected max_daily_tokens 50000, got %d", config.AutonomousMode.MaxDailyTokens)
	}
	if config.AutonomousMode.WorkHours.Start != "09:00" {
		t.Errorf("Expected work_hours.start '09:00', got '%s'", config.AutonomousMode.WorkHours.Start)
	}
	if len(config.AutonomousMode.Priorities) != 2 {
		t.Errorf("Expected 2 priorities, got %d", len(config.AutonomousMode.Priorities))
	}

	// Verify development settings
	if config.Development.CommitStyle != "conventional" {
		t.Errorf("Expected commit_style 'conventional', got '%s'", config.Development.CommitStyle)
	}
	if !config.Development.TestingRequired {
		t.Error("Expected testing_required to be true")
	}

	// Verify runner presets
	// IDENTITY-EXCEPTION: V2Config.Lex field references V2 YAML schema
	if config.Claude.Presets.PlanMode != "--permission-mode plan --allow-dangerously-skip-permissions" {
		t.Errorf("Plan mode preset mismatch: %s", config.Claude.Presets.PlanMode)
	}

	// Verify GitHub monitoring
	if !config.GitHubMonitoring.Enabled {
		t.Error("Expected github_monitoring.enabled to be true")
	}
	if len(config.GitHubMonitoring.Repositories) != 2 {
		t.Errorf("Expected 2 repositories, got %d", len(config.GitHubMonitoring.Repositories))
	}

	// Verify metadata
	if config.Metadata.VesselID != "meridian-lex-001" {
		t.Errorf("Expected vessel_id 'meridian-lex-001', got '%s'", config.Metadata.VesselID)
	}
	if config.Metadata.Operator != "Fleet Admiral Lunar Laurus" {
		t.Errorf("Expected operator 'Fleet Admiral Lunar Laurus', got '%s'", config.Metadata.Operator)
	}
}

func TestParseLexConfig_EmptyYAML(t *testing.T) {
	config, err := ParseLexConfig("")
	if err != nil {
		t.Fatalf("ParseLexConfig failed on empty YAML: %v", err)
	}

	// Empty YAML should still parse to zero-values
	if config.Mode.Current != "" {
		t.Errorf("Expected empty mode, got '%s'", config.Mode.Current)
	}
	if config.TokenBudget.DailyLimit != 0 {
		t.Errorf("Expected 0 daily_limit, got %d", config.TokenBudget.DailyLimit)
	}
}

func TestParseLexConfig_InvalidYAML(t *testing.T) {
	yaml := `mode:
  current: invalid yaml
  missing closing bracket
token_budget:
`

	_, err := ParseLexConfig(yaml)
	if err == nil {
		t.Fatal("Expected error for invalid YAML, got nil")
	}
}

func TestGetPeriodBoundaries_Daily(t *testing.T) {
	refTime := time.Date(2026, 2, 20, 14, 30, 0, 0, time.UTC)

	start, end := GetPeriodBoundaries("daily", refTime)

	expectedStart := time.Date(2026, 2, 20, 0, 0, 0, 0, time.UTC)
	expectedEnd := time.Date(2026, 2, 21, 0, 0, 0, 0, time.UTC)

	if !start.Equal(expectedStart) {
		t.Errorf("Expected start %v, got %v", expectedStart, start)
	}
	if !end.Equal(expectedEnd) {
		t.Errorf("Expected end %v, got %v", expectedEnd, end)
	}
}

func TestGetPeriodBoundaries_Weekly(t *testing.T) {
	// Thursday 2026-02-19
	refTime := time.Date(2026, 2, 19, 14, 30, 0, 0, time.UTC)

	start, end := GetPeriodBoundaries("weekly", refTime)

	// Week starts Monday 2026-02-16
	expectedStart := time.Date(2026, 2, 16, 0, 0, 0, 0, time.UTC)
	// Next week starts Monday 2026-02-23
	expectedEnd := time.Date(2026, 2, 23, 0, 0, 0, 0, time.UTC)

	if !start.Equal(expectedStart) {
		t.Errorf("Expected start %v, got %v", expectedStart, start)
	}
	if !end.Equal(expectedEnd) {
		t.Errorf("Expected end %v, got %v", expectedEnd, end)
	}
}

func TestGetPeriodBoundaries_Monthly(t *testing.T) {
	refTime := time.Date(2026, 2, 19, 14, 30, 0, 0, time.UTC)

	start, end := GetPeriodBoundaries("monthly", refTime)

	expectedStart := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	expectedEnd := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)

	if !start.Equal(expectedStart) {
		t.Errorf("Expected start %v, got %v", expectedStart, start)
	}
	if !end.Equal(expectedEnd) {
		t.Errorf("Expected end %v, got %v", expectedEnd, end)
	}
}

func TestGetPeriodBoundaries_DefaultToDaily(t *testing.T) {
	refTime := time.Date(2026, 2, 20, 14, 30, 0, 0, time.UTC)

	// Invalid period should default to daily
	start, end := GetPeriodBoundaries("invalid", refTime)

	expectedStart := time.Date(2026, 2, 20, 0, 0, 0, 0, time.UTC)
	expectedEnd := time.Date(2026, 2, 21, 0, 0, 0, 0, time.UTC)

	if !start.Equal(expectedStart) {
		t.Errorf("Expected start %v, got %v", expectedStart, start)
	}
	if !end.Equal(expectedEnd) {
		t.Errorf("Expected end %v, got %v", expectedEnd, end)
	}
}
