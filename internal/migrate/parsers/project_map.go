package parsers

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ParseProjectMap parses PROJECT-MAP.md and extracts project entries
func ParseProjectMap(mdContent string) ([]V2Project, error) {
	lines := strings.Split(mdContent, "\n")
	var projects []V2Project

	inTable := false
	headerSeen := false

	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Skip empty lines
		if line == "" {
			continue
		}

		// Detect table rows (start with |)
		if !strings.HasPrefix(line, "|") {
			inTable = false
			headerSeen = false
			continue
		}

		// Split on | and trim each cell
		cells := strings.Split(line, "|")
		if len(cells) < 3 { // Must have at least 3 cells (including empty first/last from split)
			continue
		}

		// Trim whitespace from all cells
		for i := range cells {
			cells[i] = strings.TrimSpace(cells[i])
		}

		// Remove empty first and last cells (from splitting "|foo|bar|")
		if len(cells) > 0 && cells[0] == "" {
			cells = cells[1:]
		}
		if len(cells) > 0 && cells[len(cells)-1] == "" {
			cells = cells[:len(cells)-1]
		}

		// Skip if not enough columns (need at least 6: Name, Status, Path, Started, Priority, Notes)
		if len(cells) < 6 {
			continue
		}

		// Detect header row (contains "Project Name" or "Status")
		if strings.Contains(cells[0], "Project Name") || strings.Contains(cells[1], "Status") {
			inTable = true
			headerSeen = true
			continue
		}

		// Detect separator row (contains ----)
		if strings.Contains(cells[0], "---") || strings.Contains(cells[1], "---") {
			continue
		}

		// If we've seen the header and we're in a table, this is a data row
		if headerSeen && inTable {
			project, err := parseProjectRow(cells)
			if err != nil {
				// Log warning but continue parsing other rows
				fmt.Fprintf(os.Stderr, "Warning: failed to parse project row: %v\n", err)
				continue
			}
			projects = append(projects, project)
		}
	}

	return projects, nil
}

// parseProjectRow parses a single project table row
func parseProjectRow(cells []string) (V2Project, error) {
	if len(cells) < 6 {
		return V2Project{}, fmt.Errorf("insufficient columns: got %d, need 6", len(cells))
	}

	name := cells[0]
	status := cells[1]
	path := cells[2]
	startedStr := cells[3]
	priority := cells[4]
	notes := cells[5]

	// Validate required fields
	if name == "" {
		return V2Project{}, fmt.Errorf("project name is empty")
	}

	// Expand path (~/... → /home/meridian/...)
	if strings.HasPrefix(path, "~/") {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return V2Project{}, fmt.Errorf("expand home directory: %w", err)
		}
		path = filepath.Join(homeDir, path[2:])
	}

	// Parse started date (format: YYYY-MM-DD)
	var started time.Time
	if startedStr != "" && startedStr != "-" {
		var err error
		started, err = time.Parse("2006-01-02", startedStr)
		if err != nil {
			// If parse fails, use zero time (will be handled during import)
			started = time.Time{}
		}
	}

	return V2Project{
		Name:     name,
		Status:   status,
		Path:     path,
		Started:  started,
		Priority: priority,
		Notes:    notes,
	}, nil
}

// ParseProjectMapFile reads and parses a PROJECT-MAP.md file
func ParseProjectMapFile(filePath string) ([]V2Project, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}

	return ParseProjectMap(string(content))
}
