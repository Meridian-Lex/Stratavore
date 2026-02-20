package parsers

import (
	"testing"
)

func TestParseDirectives_ValidJSONL(t *testing.T) {
	jsonl := `{"id":"TEST-1","timestamp":"2026-02-07T00:00:00Z","severity":"CRITICAL","trigger":"Test trigger","action":{"abort":true},"directive_text":"Test directive","standard_process":true}
{"id":"TEST-2","timestamp":"2026-02-08T00:00:00Z","severity":"STANDARD","trigger":"Another trigger","action":{"create":true},"directive_text":"Another directive","standard_process":false}
`

	directives, err := ParseDirectivesContent(jsonl)
	if err != nil {
		t.Fatalf("ParseDirectivesContent failed: %v", err)
	}

	if len(directives) != 2 {
		t.Fatalf("Expected 2 directives, got %d", len(directives))
	}

	// Verify first directive
	if directives[0].ID != "TEST-1" {
		t.Errorf("Expected ID 'TEST-1', got '%s'", directives[0].ID)
	}
	if directives[0].Severity != "CRITICAL" {
		t.Errorf("Expected severity 'CRITICAL', got '%s'", directives[0].Severity)
	}
	if !directives[0].StandardProcess {
		t.Error("Expected standard_process to be true")
	}

	// Verify action JSONB
	if directives[0].Action == nil {
		t.Fatal("Expected action to be non-nil")
	}
	if abort, ok := directives[0].Action["abort"].(bool); !ok || !abort {
		t.Error("Expected action.abort to be true")
	}

	// Verify second directive
	if directives[1].ID != "TEST-2" {
		t.Errorf("Expected ID 'TEST-2', got '%s'", directives[1].ID)
	}
	if directives[1].StandardProcess {
		t.Error("Expected standard_process to be false")
	}
}

func TestParseDirectives_EmptyFile(t *testing.T) {
	directives, err := ParseDirectivesContent("")
	if err != nil {
		t.Fatalf("ParseDirectivesContent failed on empty content: %v", err)
	}

	if len(directives) != 0 {
		t.Errorf("Expected 0 directives from empty content, got %d", len(directives))
	}
}

func TestParseDirectives_WithComments(t *testing.T) {
	jsonl := `# This is a comment
{"id":"TEST-1","timestamp":"2026-02-07T00:00:00Z","severity":"HIGH","trigger":"Test","action":{},"directive_text":"Test"}
# Another comment

{"id":"TEST-2","timestamp":"2026-02-08T00:00:00Z","severity":"MEDIUM","trigger":"Test2","action":{},"directive_text":"Test2"}
`

	directives, err := ParseDirectivesContent(jsonl)
	if err != nil {
		t.Fatalf("ParseDirectivesContent failed: %v", err)
	}

	if len(directives) != 2 {
		t.Errorf("Expected 2 directives (comments should be skipped), got %d", len(directives))
	}
}

func TestParseDirectives_BlankLines(t *testing.T) {
	jsonl := `{"id":"TEST-1","timestamp":"2026-02-07T00:00:00Z","severity":"HIGH","trigger":"Test","action":{},"directive_text":"Test"}

{"id":"TEST-2","timestamp":"2026-02-08T00:00:00Z","severity":"MEDIUM","trigger":"Test2","action":{},"directive_text":"Test2"}


`

	directives, err := ParseDirectivesContent(jsonl)
	if err != nil {
		t.Fatalf("ParseDirectivesContent failed with blank lines: %v", err)
	}

	if len(directives) != 2 {
		t.Errorf("Expected 2 directives (blank lines should be skipped), got %d", len(directives))
	}
}

func TestParseDirectives_InvalidJSON(t *testing.T) {
	jsonl := `{"id":"TEST-1", invalid json here`

	_, err := ParseDirectivesContent(jsonl)
	if err == nil {
		t.Fatal("Expected error for invalid JSON, got nil")
	}
}

func TestParseDirectives_RealWorldExample(t *testing.T) {
	// This is actual data from the behavioral-directives.jsonl file
	jsonl := `{"id":"STRIKE-3-PROTECTED-BRANCH","timestamp":"2026-02-07T00:00:00Z","severity":"CRITICAL","rank_demotion":"Unranked","progress":"2/5","trigger":"Direct work on main/master branch","action":{"abort":true,"create_branch":true,"never_repeat":true},"evidence":"V1.1.0 created but unused, PR #1","directive":"ALWAYS work on separate branches","standard_process":true}
# IDENTITY-EXCEPTION: functional internal reference
{"id":"STATE-BACKUP-SYNC","timestamp":"2026-02-19T02:00:00Z","severity":"STANDARD","trigger":"After any significant state change","action":{"stage_changed_files":true,"commit":true,"push_to_origin":true},"directive":"After significant state changes, commit and push to lex-state","standard_process":true}
{"id":"NO-EMOJIS","timestamp":"2026-02-12T20:40:00Z","severity":"STANDARD","trigger":"Any output","action":{"strip_emojis":true},"directive":"Never use emojis anywhere","standard_process":true}
{"id":"PR-MERGE-VERIFICATION","timestamp":"2026-02-13T16:45:00Z","severity":"PRIME","trigger":"Before ANY pull request merge","action":{"check_heart_react":true,"verification_method":"gh api repos/{owner}/{repo}/issues/{number}/reactions"},"directive":"PRIME DIRECTIVE: Verify @LunarLaurus heart react before merge","standard_process":true}
`

	directives, err := ParseDirectivesContent(jsonl)
	if err != nil {
		t.Fatalf("ParseDirectivesContent failed on real-world data: %v", err)
	}

	// Should parse 4 directives (comment line skipped)
	if len(directives) != 4 {
		t.Fatalf("Expected 4 directives, got %d", len(directives))
	}

	// Verify specific directives
	foundProtectedBranch := false
	foundPrMerge := false

	for _, d := range directives {
		if d.ID == "STRIKE-3-PROTECTED-BRANCH" {
			foundProtectedBranch = true
			if d.Severity != "CRITICAL" {
				t.Errorf("Expected CRITICAL severity for protected branch, got '%s'", d.Severity)
			}
			if !d.StandardProcess {
				t.Error("Expected standard_process to be true")
			}
		}

		if d.ID == "PR-MERGE-VERIFICATION" {
			foundPrMerge = true
			if d.Severity != "PRIME" {
				t.Errorf("Expected PRIME severity, got '%s'", d.Severity)
			}
			// Verify action has check_heart_react
			if checkHeart, ok := d.Action["check_heart_react"].(bool); !ok || !checkHeart {
				t.Error("Expected action.check_heart_react to be true")
			}
		}
	}

	if !foundProtectedBranch {
		t.Error("Did not find STRIKE-3-PROTECTED-BRANCH directive")
	}
	if !foundPrMerge {
		t.Error("Did not find PR-MERGE-VERIFICATION directive")
	}
}

func TestParseDirectives_ComplexAction(t *testing.T) {
	jsonl := `{"id":"COMPLEX","timestamp":"2026-02-07T00:00:00Z","severity":"HIGH","trigger":"Test","action":{"nested":{"key":"value"},"array":[1,2,3],"bool":true},"directive_text":"Complex action test"}`

	directives, err := ParseDirectivesContent(jsonl)
	if err != nil {
		t.Fatalf("ParseDirectivesContent failed: %v", err)
	}

	if len(directives) != 1 {
		t.Fatalf("Expected 1 directive, got %d", len(directives))
	}

	// Verify complex action structure preserved
	action := directives[0].Action

	// Check nested object
	if nested, ok := action["nested"].(map[string]interface{}); ok {
		if nested["key"] != "value" {
			t.Error("Expected nested.key to be 'value'")
		}
	} else {
		t.Error("Expected action.nested to be a map")
	}

	// Check array
	if arr, ok := action["array"].([]interface{}); ok {
		if len(arr) != 3 {
			t.Errorf("Expected array length 3, got %d", len(arr))
		}
	} else {
		t.Error("Expected action.array to be an array")
	}

	// Check boolean
	if b, ok := action["bool"].(bool); !ok || !b {
		t.Error("Expected action.bool to be true")
	}
}
