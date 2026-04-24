package main

import (
	"encoding/json"
	"strings"
	"testing"
)

// mockRead returns a readFileFn that serves a fixed payload for every path.
func mockRead(payload string) readFileFn {
	return func(string) ([]byte, error) {
		return []byte(payload), nil
	}
}

func editInput(path, oldStr, newStr string) *hookInput {
	return &hookInput{
		ToolName: "Edit",
		ToolInput: map[string]interface{}{
			"file_path":  path,
			"old_string": oldStr,
			"new_string": newStr,
		},
	}
}

func writeInput(path, content string) *hookInput {
	return &hookInput{
		ToolName: "Write",
		ToolInput: map[string]interface{}{
			"file_path": path,
			"content":   content,
		},
	}
}

func TestEvaluate_IgnoresNonTargetFile(t *testing.T) {
	in := editInput("/repo/src/foo.ts", "a", "b")
	out := evaluate(in, mockRead(""))
	if out.Decision != "approve" {
		t.Fatalf("expected approve for unrelated file, got %+v", out)
	}
}

func TestEvaluate_IgnoresNonEditTools(t *testing.T) {
	in := &hookInput{
		ToolName:  "Read",
		ToolInput: map[string]interface{}{"file_path": "/repo/.pre-commit.json"},
	}
	out := evaluate(in, mockRead(""))
	if out.Decision != "approve" {
		t.Fatalf("expected approve for Read tool, got %+v", out)
	}
}

func TestEvaluate_BlocksNewExcludePathEntry(t *testing.T) {
	original := `{
  "missingTestsCheckConfig": {
    "excludePaths": ["generated-types/", "stories/"]
  }
}`
	after := `{
  "missingTestsCheckConfig": {
    "excludePaths": ["generated-types/", "stories/", "packages/backend/slack/"]
  }
}`

	in := editInput("/repo/.pre-commit.json",
		`["generated-types/", "stories/"]`,
		`["generated-types/", "stories/", "packages/backend/slack/"]`,
	)
	out := evaluate(in, mockRead(original))
	if out.Decision != "block" {
		t.Fatalf("expected block, got %+v", out)
	}
	if !strings.Contains(out.Reason, "packages/backend/slack/") {
		t.Errorf("expected reason to mention the added path, got: %s", out.Reason)
	}

	// And via Write tool on the full file.
	out = evaluate(writeInput("/repo/.pre-commit.json", after), mockRead(original))
	if out.Decision != "block" {
		t.Fatalf("expected block for Write, got %+v", out)
	}
}

func TestEvaluate_AllowsReorderOrRemoval(t *testing.T) {
	original := `{
  "missingTestsCheckConfig": {
    "excludePaths": ["generated-types/", "stories/", "e2e/"]
  }
}`
	// Remove an entry and reorder the others. No new entries.
	reordered := `{
  "missingTestsCheckConfig": {
    "excludePaths": ["stories/", "generated-types/"]
  }
}`
	out := evaluate(writeInput("/repo/.pre-commit.json", reordered), mockRead(original))
	if out.Decision != "approve" {
		t.Fatalf("expected approve for removal/reorder, got %+v", out)
	}
}

func TestEvaluate_BlocksExcludePathInNestedConfig(t *testing.T) {
	// The excludePaths array shows up in multiple configs (stubTestCheck,
	// missingTestsCheck, typecheck, ...). Any of them should trip the gate.
	original := `{
  "stubTestCheckConfig": { "excludePaths": ["generated-types/"] }
}`
	after := `{
  "stubTestCheckConfig": { "excludePaths": ["generated-types/", "apps/demo/"] }
}`
	out := evaluate(writeInput("/repo/.pre-commit.json", after), mockRead(original))
	if out.Decision != "block" {
		t.Fatalf("expected block for nested excludePaths add, got %+v", out)
	}
	if !strings.Contains(out.Reason, "apps/demo/") {
		t.Errorf("reason missing added path: %s", out.Reason)
	}
}

func TestEvaluate_BlocksFeatureFlippedFalse(t *testing.T) {
	original := `{
  "features": { "missingTestsCheck": true, "stubTestCheck": true }
}`
	after := `{
  "features": { "missingTestsCheck": false, "stubTestCheck": true }
}`
	out := evaluate(writeInput("/repo/.pre-commit.json", after), mockRead(original))
	if out.Decision != "block" {
		t.Fatalf("expected block for feature disable, got %+v", out)
	}
	if !strings.Contains(out.Reason, "missingTestsCheck") {
		t.Errorf("reason missing feature name: %s", out.Reason)
	}
}

func TestEvaluate_AllowsFeatureFlippedTrue(t *testing.T) {
	original := `{
  "features": { "missingTestsCheck": false }
}`
	after := `{
  "features": { "missingTestsCheck": true }
}`
	out := evaluate(writeInput("/repo/.pre-commit.json", after), mockRead(original))
	if out.Decision != "approve" {
		t.Fatalf("expected approve for feature enable, got %+v", out)
	}
}

func TestEvaluate_AllowsUnrelatedChanges(t *testing.T) {
	original := `{
  "features": { "missingTestsCheck": true },
  "missingTestsCheckConfig": { "mode": "staged", "excludePaths": ["generated-types/"] }
}`
	// Changing mode is fine — no excludePaths change, no feature disable.
	after := `{
  "features": { "missingTestsCheck": true },
  "missingTestsCheckConfig": { "mode": "all", "excludePaths": ["generated-types/"] }
}`
	out := evaluate(writeInput("/repo/.pre-commit.json", after), mockRead(original))
	if out.Decision != "approve" {
		t.Fatalf("expected approve for mode flip, got %+v", out)
	}
}

func TestEvaluate_NewFileWithExcludePaths_IsBlocked(t *testing.T) {
	// .pre-commit.json does not yet exist. Creating one with excludePaths
	// populated should be blocked — the whole point is that exceptions
	// require a human.
	readFile := func(string) ([]byte, error) {
		return nil, nil
	}
	content := `{
  "missingTestsCheckConfig": { "excludePaths": ["packages/backend/slack/"] }
}`
	out := evaluate(writeInput("/repo/.pre-commit.json", content), readFile)
	if out.Decision != "block" {
		t.Fatalf("expected block for new file with excludePaths, got %+v", out)
	}
}

func TestEvaluate_HandlesJSONCComments(t *testing.T) {
	original := `{
  // pre-existing exclusions
  "missingTestsCheckConfig": { "excludePaths": ["generated-types/"] }
}`
	after := `{
  // pre-existing exclusions
  // plus a shortcut the agent wants to sneak in
  "missingTestsCheckConfig": { "excludePaths": ["generated-types/", "packages/backend/slack/"] }
}`
	out := evaluate(writeInput("/repo/.pre-commit.json", after), mockRead(original))
	if out.Decision != "block" {
		t.Fatalf("expected block despite JSONC comments, got %+v", out)
	}
}

func TestEvaluate_MalformedJSONApproves(t *testing.T) {
	original := `{ "missingTestsCheckConfig": { "excludePaths": [] } }`
	out := evaluate(writeInput("/repo/.pre-commit.json", "{ not json"), mockRead(original))
	if out.Decision != "approve" {
		t.Fatalf("expected approve for malformed JSON (let other tools flag it), got %+v", out)
	}
}

func TestEvaluate_EditStringReplacement(t *testing.T) {
	// Exercise the Edit-specific path: old_string must exist in the original
	// file content, and we compute the post-edit content via string replace.
	original := `{
  "missingTestsCheckConfig": {
    "excludePaths": [
      "generated-types/"
    ]
  }
}`
	oldStr := `      "generated-types/"`
	newStr := `      "generated-types/",
      "packages/backend/slack/"`
	out := evaluate(editInput("/repo/.pre-commit.json", oldStr, newStr), mockRead(original))
	if out.Decision != "block" {
		t.Fatalf("expected block for Edit-inserted path, got %+v", out)
	}
}

// Double-check the output marshals to JSON — the contract with Claude Code
// requires a valid JSON object on stdout.
func TestOutputMarshalsCleanly(t *testing.T) {
	out := blockExcludePaths([]string{"packages/x/"})
	data, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	var round map[string]interface{}
	if err := json.Unmarshal(data, &round); err != nil {
		t.Fatalf("unmarshal round-trip failed: %v", err)
	}
	if round["decision"] != "block" {
		t.Errorf("expected decision=block, got %v", round["decision"])
	}
}
