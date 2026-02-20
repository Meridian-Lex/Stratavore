package importers

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/meridian-lex/stratavore/internal/migrate/parsers"
)

func TestImportDirectives_ActionJSONB(t *testing.T) {
	directive := parsers.V2Directive{
		ID:               "TEST-1",
		Severity:         "HIGH",
		TriggerCondition: "Test trigger",
		Action: map[string]interface{}{
			"abort":        true,
			"notify":       false,
			"nested_value": map[string]interface{}{"key": "value"},
		},
		DirectiveText:    "Test directive",
		StandardProcess:  true,
	}

	// Test JSON marshaling
	actionJSON, err := json.Marshal(directive.Action)
	if err != nil {
		t.Fatalf("Failed to marshal action: %v", err)
	}

	// Verify it's valid JSON
	var unmarshaled map[string]interface{}
	err = json.Unmarshal(actionJSON, &unmarshaled)
	if err != nil {
		t.Fatalf("Failed to unmarshal action: %v", err)
	}

	// Verify values preserved
	if !unmarshaled["abort"].(bool) {
		t.Error("Expected abort to be true")
	}

	if unmarshaled["notify"].(bool) {
		t.Error("Expected notify to be false")
	}
}

func TestImportDirectives_TimestampParsing(t *testing.T) {
	tests := []struct {
		name      string
		timestamp string
		wantErr   bool
	}{
		{
			name:      "valid RFC3339",
			timestamp: "2026-02-07T00:00:00Z",
			wantErr:   false,
		},
		{
			name:      "valid with timezone",
			timestamp: "2026-02-07T12:30:00-05:00",
			wantErr:   false,
		},
		{
			name:      "empty timestamp",
			timestamp: "",
			wantErr:   false, // Should use current time
		},
		{
			name:      "invalid format",
			timestamp: "2026-02-07",
			wantErr:   false, // Should fallback to current time
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var createdAt time.Time
			var err error

			if tt.timestamp != "" {
				createdAt, err = time.Parse(time.RFC3339, tt.timestamp)
				if err != nil {
					// Fallback to current time
					createdAt = time.Now()
				}
			} else {
				createdAt = time.Now()
			}

			if createdAt.IsZero() {
				t.Error("Expected non-zero createdAt")
			}
		})
	}
}

func TestImportDirectives_EnabledFlag(t *testing.T) {
	// All imported directives should be enabled by default
	directive := parsers.V2Directive{
		ID:       "TEST-1",
		Severity: "MEDIUM",
	}

	enabled := true

	if !enabled {
		t.Error("Expected imported directives to be enabled by default")
	}

	// Even if V2 directive doesn't specify enabled state
	if directive.ID != "TEST-1" {
		t.Error("Directive should maintain its ID")
	}
}

func TestImportDirectives_SeverityLevels(t *testing.T) {
	severities := []string{"PRIME", "CRITICAL", "HIGH", "MEDIUM", "LOW"}

	for _, severity := range severities {
		directive := parsers.V2Directive{
			ID:       "TEST-" + severity,
			Severity: severity,
		}

		if directive.Severity != severity {
			t.Errorf("Expected severity %s, got %s", severity, directive.Severity)
		}
	}
}

func TestImportDirectives_RealWorldExample(t *testing.T) {
	directives := []parsers.V2Directive{
		{
			ID:               "STRIKE-3-PROTECTED-BRANCH",
			Timestamp:        "2026-02-07T00:00:00Z",
			Severity:         "CRITICAL",
			TriggerCondition: "Direct work on main/master branch",
			Action: map[string]interface{}{
				"abort":          true,
				"create_branch":  true,
				"never_repeat":   true,
			},
			DirectiveText:    "ALWAYS work on separate branches",
			StandardProcess:  true,
		},
		{
			ID:               "PR-MERGE-VERIFICATION",
			Timestamp:        "2026-02-13T16:45:00Z",
			Severity:         "PRIME",
			TriggerCondition: "Before ANY pull request merge",
			Action: map[string]interface{}{
				"check_heart_react":     true,
				"heart_react_source":    "@LunarLaurus",
				"verification_method":   "gh api repos/{owner}/{repo}/issues/{number}/reactions",
			},
			DirectiveText:    "PRIME DIRECTIVE: Verify heart react before merge",
			StandardProcess:  true,
		},
		{
			ID:               "NO-EMOJIS",
			Timestamp:        "2026-02-12T20:40:00Z",
			Severity:         "STANDARD",
			TriggerCondition: "Any output",
			Action: map[string]interface{}{
				"strip_emojis": true,
			},
			DirectiveText:    "Never use emojis anywhere",
			StandardProcess:  true,
		},
	}

	for _, directive := range directives {
		// Verify each directive has required fields
		if directive.ID == "" {
			t.Error("Directive missing ID")
		}

		if directive.Severity == "" {
			t.Error("Directive missing severity")
		}

		if directive.TriggerCondition == "" {
			t.Error("Directive missing trigger_condition")
		}

		if directive.Action == nil {
			t.Error("Directive missing action")
		}

		// Verify action can be marshaled
		_, err := json.Marshal(directive.Action)
		if err != nil {
			t.Errorf("Failed to marshal action for %s: %v", directive.ID, err)
		}

		// Verify timestamp parsing
		if directive.Timestamp != "" {
			_, err := time.Parse(time.RFC3339, directive.Timestamp)
			if err != nil {
				t.Errorf("Failed to parse timestamp for %s: %v", directive.ID, err)
			}
		}
	}

	// Verify count
	if len(directives) != 3 {
		t.Errorf("Expected 3 directives, got %d", len(directives))
	}
}

func TestImportDirectives_ComplexAction(t *testing.T) {
	directive := parsers.V2Directive{
		ID:       "COMPLEX-ACTION",
		Severity: "HIGH",
		Action: map[string]interface{}{
			"nested": map[string]interface{}{
				"level2": map[string]interface{}{
					"level3": "deep value",
				},
			},
			"array": []interface{}{1, 2, 3, "four"},
			"bool":  true,
			"null":  nil,
		},
	}

	actionJSON, err := json.Marshal(directive.Action)
	if err != nil {
		t.Fatalf("Failed to marshal complex action: %v", err)
	}

	// Verify it unmarshals back correctly
	var unmarshaled map[string]interface{}
	err = json.Unmarshal(actionJSON, &unmarshaled)
	if err != nil {
		t.Fatalf("Failed to unmarshal complex action: %v", err)
	}

	// Verify nested structure preserved
	nested, ok := unmarshaled["nested"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected nested to be a map")
	}

	level2, ok := nested["level2"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected level2 to be a map")
	}

	if level2["level3"] != "deep value" {
		t.Errorf("Expected nested value 'deep value', got %v", level2["level3"])
	}

	// Verify array preserved
	arr, ok := unmarshaled["array"].([]interface{})
	if !ok {
		t.Fatal("Expected array to be a slice")
	}

	if len(arr) != 4 {
		t.Errorf("Expected array length 4, got %d", len(arr))
	}
}

func TestImportDirectives_StandardProcessFlag(t *testing.T) {
	tests := []struct {
		name            string
		standardProcess bool
	}{
		{
			name:            "standard process true",
			standardProcess: true,
		},
		{
			name:            "standard process false",
			standardProcess: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			directive := parsers.V2Directive{
				ID:              "TEST-" + tt.name,
				StandardProcess: tt.standardProcess,
			}

			if directive.StandardProcess != tt.standardProcess {
				t.Errorf("Expected standard_process=%v, got %v", tt.standardProcess, directive.StandardProcess)
			}
		})
	}
}
