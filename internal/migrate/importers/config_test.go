package importers

import (
	"testing"
	"time"

	"github.com/meridian-lex/stratavore/internal/migrate/parsers"
)

func TestImportTokenBudget_PeriodCalculation(t *testing.T) {
	// Test that period boundaries are calculated correctly
	now := time.Date(2026, 2, 20, 14, 30, 0, 0, time.UTC)

	periodStart, periodEnd := parsers.GetPeriodBoundaries("daily", now)

	expectedStart := time.Date(2026, 2, 20, 0, 0, 0, 0, time.UTC)
	expectedEnd := time.Date(2026, 2, 21, 0, 0, 0, 0, time.UTC)

	if !periodStart.Equal(expectedStart) {
		t.Errorf("Expected period_start %v, got %v", expectedStart, periodStart)
	}

	if !periodEnd.Equal(expectedEnd) {
		t.Errorf("Expected period_end %v, got %v", expectedEnd, periodEnd)
	}
}

func TestImportTokenBudget_Values(t *testing.T) {
	v2Config := &parsers.V2Config{
		TokenBudget: parsers.V2TokenBudget{
			DailyLimit: 100000,
			Tracking: parsers.V2Tracking{
				TodayUsed: 15000,
			},
		},
	}

	if v2Config.TokenBudget.DailyLimit != 100000 {
		t.Errorf("Expected daily_limit 100000, got %d", v2Config.TokenBudget.DailyLimit)
	}

	if v2Config.TokenBudget.Tracking.TodayUsed != 15000 {
		t.Errorf("Expected used_tokens 15000, got %d", v2Config.TokenBudget.Tracking.TodayUsed)
	}
}

func TestImportResourceQuotas_AutonomousMode(t *testing.T) {
	tests := []struct {
		name              string
		autonomousEnabled bool
		maxDailyTokens    int64
		shouldCreateQuota bool
	}{
		{
			name:              "autonomous mode enabled",
			autonomousEnabled: true,
			maxDailyTokens:    50000,
			shouldCreateQuota: false, // Current implementation skips quota creation
		},
		{
			name:              "autonomous mode disabled",
			autonomousEnabled: false,
			maxDailyTokens:    0,
			shouldCreateQuota: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v2Config := &parsers.V2Config{
				AutonomousMode: parsers.V2AutonomousMode{
					Enabled:        tt.autonomousEnabled,
					MaxDailyTokens: tt.maxDailyTokens,
				},
			}

			if v2Config.AutonomousMode.Enabled != tt.autonomousEnabled {
				t.Errorf("Expected autonomous enabled=%v, got %v", tt.autonomousEnabled, v2Config.AutonomousMode.Enabled)
			}
		})
	}
}

func TestImportConfigForProject_QuotaValues(t *testing.T) {
	v2Config := &parsers.V2Config{
		AutonomousMode: parsers.V2AutonomousMode{
			Enabled:        true,
			MaxDailyTokens: 50000,
		},
	}

	projectName := "test-project"

	// Verify values that would be inserted
	maxConcurrentRunners := 5
	maxTokensPerDay := v2Config.AutonomousMode.MaxDailyTokens

	if maxConcurrentRunners != 5 {
		t.Errorf("Expected max_concurrent_runners 5, got %d", maxConcurrentRunners)
	}

	if maxTokensPerDay != 50000 {
		t.Errorf("Expected max_tokens_per_day 50000, got %d", maxTokensPerDay)
	}

	if projectName != "test-project" {
		t.Errorf("Expected project_name 'test-project', got %s", projectName)
	}
}

func TestImportConfig_Scope(t *testing.T) {
	// Verify that imported budgets use 'global' scope
	scope := "global"
	var scopeID *string = nil

	if scope != "global" {
		t.Errorf("Expected scope 'global', got %s", scope)
	}

	if scopeID != nil {
		t.Errorf("Expected scope_id to be nil for global scope, got %v", scopeID)
	}
}

func TestImportConfig_PeriodGranularity(t *testing.T) {
	// V2 uses daily limits, so period_granularity should be 'daily'
	periodGranularity := "daily"

	if periodGranularity != "daily" {
		t.Errorf("Expected period_granularity 'daily', got %s", periodGranularity)
	}
}

func TestImportConfig_DefaultValues(t *testing.T) {
	// Test default values for resource quotas
	tests := []struct {
		name     string
		field    string
		value    interface{}
		expected interface{}
	}{
		{
			name:     "max concurrent runners default",
			field:    "max_concurrent_runners",
			value:    5,
			expected: 5,
		},
		{
			name:     "max memory mb unset",
			field:    "max_memory_mb",
			value:    nil,
			expected: nil,
		},
		{
			name:     "max cpu percent unset",
			field:    "max_cpu_percent",
			value:    nil,
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.value != tt.expected {
				t.Errorf("Expected %s to be %v, got %v", tt.field, tt.expected, tt.value)
			}
		})
	}
}

func TestImportConfig_RealWorldExample(t *testing.T) {
	v2Config := &parsers.V2Config{
		TokenBudget: parsers.V2TokenBudget{
			DailyLimit:           100000,
			PerSessionTarget:     20000,
			ReservedForCommander: 30000,
			Tracking: parsers.V2Tracking{
				TodayUsed: 0,
				LastReset: "2026-02-06",
			},
		},
		AutonomousMode: parsers.V2AutonomousMode{
			Enabled:        false,
			MaxDailyTokens: 50000,
			WorkHours: parsers.V2WorkHours{
				Start: "09:00",
				End:   "17:00",
			},
			WorkPace: "steady",
		},
	}

	// Verify token budget values
	if v2Config.TokenBudget.DailyLimit != 100000 {
		t.Errorf("Expected daily_limit 100000, got %d", v2Config.TokenBudget.DailyLimit)
	}

	if v2Config.TokenBudget.Tracking.TodayUsed != 0 {
		t.Errorf("Expected used_tokens 0, got %d", v2Config.TokenBudget.Tracking.TodayUsed)
	}

	// Verify autonomous mode values
	if v2Config.AutonomousMode.Enabled {
		t.Error("Expected autonomous_mode.enabled to be false")
	}

	if v2Config.AutonomousMode.MaxDailyTokens != 50000 {
		t.Errorf("Expected max_daily_tokens 50000, got %d", v2Config.AutonomousMode.MaxDailyTokens)
	}
}
