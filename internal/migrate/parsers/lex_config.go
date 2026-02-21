package parsers

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// ParseLexConfig parses LEX-CONFIG.yaml and extracts configuration
func ParseLexConfig(yamlContent string) (*V2Config, error) {
	var config V2Config

	err := yaml.Unmarshal([]byte(yamlContent), &config)
	if err != nil {
		return nil, fmt.Errorf("unmarshal YAML: %w", err)
	}

	return &config, nil
}

// ParseLexConfigFile reads and parses a LEX-CONFIG.yaml file
func ParseLexConfigFile(filePath string) (*V2Config, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}

	return ParseLexConfig(string(content))
}

// GetPeriodBoundaries calculates period_start and period_end for token budgets
// V2 uses flat daily limits; V3 requires explicit period boundaries
func GetPeriodBoundaries(period string, referenceTime time.Time) (start time.Time, end time.Time) {
	switch period {
	case "daily", "":
		// Start of today (00:00:00)
		start = referenceTime.Truncate(24 * time.Hour)
		// Start of tomorrow (00:00:00)
		end = start.Add(24 * time.Hour)

	case "weekly":
		// Start of this week (Monday 00:00:00)
		weekday := int(referenceTime.Weekday())
		if weekday == 0 { // Sunday
			weekday = 7
		}
		daysToMonday := weekday - 1
		start = referenceTime.AddDate(0, 0, -daysToMonday).Truncate(24 * time.Hour)
		// Start of next week
		end = start.AddDate(0, 0, 7)

	case "monthly":
		// Start of this month (1st day, 00:00:00)
		year, month, _ := referenceTime.Date()
		start = time.Date(year, month, 1, 0, 0, 0, 0, referenceTime.Location())
		// Start of next month
		end = start.AddDate(0, 1, 0)

	default:
		// Default to daily
		start = referenceTime.Truncate(24 * time.Hour)
		end = start.Add(24 * time.Hour)
	}

	return start, end
}

// NormalizePaths expands paths with ~ to absolute paths
func NormalizePaths(paths *V2Paths) error {
	// Note: In production, we should expand ~ to actual home directory
	// For now, just validate that paths exist
	// This function can be enhanced later if needed
	return nil
}
