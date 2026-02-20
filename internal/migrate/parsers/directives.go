package parsers

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// ParseDirectives reads and parses behavioral-directives.jsonl
func ParseDirectives(jsonlPath string) ([]V2Directive, error) {
	file, err := os.Open(jsonlPath)
	if err != nil {
		return nil, fmt.Errorf("open file: %w", err)
	}
	defer file.Close()

	var directives []V2Directive
	scanner := bufio.NewScanner(file)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		// Skip empty lines and comments
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		var directive V2Directive
		if err := json.Unmarshal([]byte(line), &directive); err != nil {
			return nil, fmt.Errorf("parse line %d: %w", lineNum, err)
		}

		directives = append(directives, directive)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}

	return directives, nil
}

// ParseDirectivesContent parses JSONL content directly (for testing)
func ParseDirectivesContent(jsonlContent string) ([]V2Directive, error) {
	var directives []V2Directive
	lineNum := 0

	lines := strings.Split(jsonlContent, "\n")
	for _, line := range lines {
		lineNum++
		line = strings.TrimSpace(line)

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		var directive V2Directive
		if err := json.Unmarshal([]byte(line), &directive); err != nil {
			return nil, fmt.Errorf("parse line %d: %w", lineNum, err)
		}

		directives = append(directives, directive)
	}

	return directives, nil
}
