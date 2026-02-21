package main

import (
	"fmt"
	"path/filepath"

	"github.com/meridian-lex/stratavore/internal/migrate/parsers"
	"github.com/spf13/cobra"
)

var analyzeCmd = &cobra.Command{
	Use:   "analyze",
	Short: "Analyze V2 data and show what will be imported",
	Long: `Analyzes Lex V2 state files and displays a summary of data to be imported.

This command does NOT modify any data - it only reads and analyzes V2 files.

Example:
  stratavore-migrate analyze --v2-dir=/home/meridian/meridian-home/lex-internal/state
`,
	RunE: runAnalyze,
}

func runAnalyze(cmd *cobra.Command, args []string) error {
	// Validate V2 directory
	if v2Dir == "" {
		return fmt.Errorf("--v2-dir flag is required")
	}

	fmt.Println("Lex V2 Data Analysis")
	fmt.Println("═══════════════════════════════════════════════════════════")
	fmt.Println()

	// Parse PROJECT-MAP.md
	projectMapPath := filepath.Join(v2Dir, "PROJECT-MAP.md")
	fmt.Printf("Reading %s...\n", projectMapPath)

	projects, err := parsers.ParseProjectMapFile(projectMapPath)
	if err != nil {
		return fmt.Errorf("parse PROJECT-MAP.md: %w", err)
	}

	fmt.Printf("✓ Projects: %d found\n", len(projects))
	for _, proj := range projects {
		fmt.Printf("  - %s (%s)\n", proj.Name, proj.Status)
	}
	fmt.Println()

	// Parse time_sessions.jsonl
	sessionsPath := filepath.Join(v2Dir, "time_sessions.jsonl")
	fmt.Printf("Reading %s...\n", sessionsPath)

	sessions, err := parsers.ParseTimeSessions(sessionsPath)
	if err != nil {
		return fmt.Errorf("parse time_sessions.jsonl: %w", err)
	}

	fmt.Printf("✓ Sessions: %d found\n", len(sessions))
	var totalDuration int64
	for _, sess := range sessions {
		duration := sess.EndTime.Sub(sess.StartTime).Seconds() - float64(sess.PausedTime)
		totalDuration += int64(duration)
		fmt.Printf("  - %s: %.0fs duration\n", sess.SessionID, duration)
	}
	fmt.Println()

	// Parse LEX-CONFIG.yaml
	configPath := filepath.Join(v2Dir, "..", "config", "LEX-CONFIG.yaml")
	fmt.Printf("Reading %s...\n", configPath)

	config, err := parsers.ParseLexConfigFile(configPath)
	if err != nil {
		return fmt.Errorf("parse LEX-CONFIG.yaml: %w", err)
	}

	fmt.Printf("✓ Token Budgets:\n")
	fmt.Printf("  - Daily limit: %d\n", config.TokenBudget.DailyLimit)
	fmt.Printf("  - Today used: %d\n", config.TokenBudget.Tracking.TodayUsed)
	fmt.Printf("  - Per-session target: %d\n", config.TokenBudget.PerSessionTarget)
	fmt.Println()

	// Parse rank-status.jsonl
	rankPath := filepath.Join(v2Dir, "..", "directives", "rank-status.jsonl")
	fmt.Printf("Reading %s...\n", rankPath)

	rankStatus, err := parsers.ParseRankStatusFile(rankPath)
	if err != nil {
		return fmt.Errorf("parse rank-status.jsonl: %w", err)
	}

	fmt.Printf("✓ Rank: %s (%s progress, %d strikes)\n",
		rankStatus.CurrentRank,
		rankStatus.ProgressTowardNext,
		rankStatus.Strikes)

	rankEvents := rankStatus.GetRankEvents()
	fmt.Printf("  - Total rank events: %d\n", len(rankEvents))
	fmt.Println()

	// Parse behavioral-directives.jsonl
	directivesPath := filepath.Join(v2Dir, "..", "directives", "behavioral-directives.jsonl")
	fmt.Printf("Reading %s...\n", directivesPath)

	directives, err := parsers.ParseDirectives(directivesPath)
	if err != nil {
		return fmt.Errorf("parse behavioral-directives.jsonl: %w", err)
	}

	fmt.Printf("✓ Directives: %d rules found\n", len(directives))
	severityCounts := make(map[string]int)
	for _, d := range directives {
		severityCounts[d.Severity]++
	}
	for severity, count := range severityCounts {
		fmt.Printf("  - %s: %d\n", severity, count)
	}
	fmt.Println()

	// Expected import summary
	fmt.Println("Expected Import:")
	fmt.Println("───────────────────────────────────────────────────────────")
	fmt.Printf("  - %d projects → projects table\n", len(projects))
	fmt.Printf("  - %d sessions → sessions table\n", len(sessions))
	fmt.Printf("  - 1 token budget → token_budgets table\n")
	fmt.Printf("  - %d rank events → rank_tracking table\n", len(rankEvents))
	fmt.Printf("  - %d directives → directives table\n", len(directives))
	fmt.Println()

	// Status
	totalItems := len(projects) + len(sessions) + 1 + len(rankEvents) + len(directives)
	fmt.Printf("Status: Ready for import (%d total items)\n", totalItems)
	fmt.Println()
	fmt.Println("Next step: Run `stratavore-migrate import --v2-dir=<path>` to execute migration")

	return nil
}
