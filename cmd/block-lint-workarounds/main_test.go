package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestProcessHook_IgnoresNonEditWriteTools(t *testing.T) {
	tests := []struct {
		name     string
		toolName string
	}{
		{"Read tool", "Read"},
		{"Bash tool", "Bash"},
		{"Grep tool", "Grep"},
		{"WebFetch tool", "WebFetch"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := &HookInput{
				ToolName: tt.toolName,
				ToolInput: map[string]interface{}{
					"new_string": "import { Foo } from 'bar';",
				},
			}

			output := processHook(input)

			if output.Decision != "approve" {
				t.Errorf("expected approve for %s, got %s", tt.toolName, output.Decision)
			}
			if output.Reason != "" {
				t.Errorf("expected no reason, got %s", output.Reason)
			}
		})
	}
}

func TestProcessHook_EmptyContent(t *testing.T) {
	tests := []struct {
		name      string
		toolInput map[string]interface{}
	}{
		{
			name:      "empty new_string",
			toolInput: map[string]interface{}{"new_string": ""},
		},
		{
			name:      "empty content",
			toolInput: map[string]interface{}{"content": ""},
		},
		{
			name:      "no text fields",
			toolInput: map[string]interface{}{"file_path": "/some/path"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := &HookInput{
				ToolName:  "Edit",
				ToolInput: tt.toolInput,
			}

			output := processHook(input)

			if output.Decision != "approve" {
				t.Errorf("expected approve for empty content, got %s", output.Decision)
			}
		})
	}
}

func TestCheckUnderscorePrefixes_BlocksWorkarounds(t *testing.T) {
	tests := []struct {
		name          string
		textPattern   string
		textArgs      []interface{}
		isConvexFile  bool
		shouldBlock   bool
		expectedMatch string
	}{
		{
			name:          "blocks underscore import alias",
			textPattern:   "import { Foo %s %sFoo } from 'bar';",
			textArgs:      []interface{}{"as", "_"},
			isConvexFile:  false,
			shouldBlock:   true,
			expectedMatch: "as _",
		},
		{
			name:          "blocks underscore destructuring",
			textPattern:   "const { prop%s %sunused } = obj;",
			textArgs:      []interface{}{":", "_"},
			isConvexFile:  false,
			shouldBlock:   true,
			expectedMatch: ": _",
		},
		{
			name:          "blocks type alias with underscore",
			textPattern:   "type MyType %s %sMyType",
			textArgs:      []interface{}{"as", "_"},
			isConvexFile:  false,
			shouldBlock:   true,
			expectedMatch: "as _",
		},
		{
			name:          "blocks underscore variable with extra spaces",
			textPattern:   "import { Thing %s   %sThing } from 'module';",
			textArgs:      []interface{}{"as", "_"},
			isConvexFile:  false,
			shouldBlock:   true,
			expectedMatch: "as   _",
		},
		{
			name:          "allows normal imports",
			textPattern:   "import { Foo } from 'bar';",
			textArgs:      []interface{}{},
			isConvexFile:  false,
			shouldBlock:   false,
			expectedMatch: "",
		},
		{
			name:          "allows normal aliases",
			textPattern:   "import { Foo as Bar } from 'baz';",
			textArgs:      []interface{}{},
			isConvexFile:  false,
			shouldBlock:   false,
			expectedMatch: "",
		},
		{
			name:          "allows underscore in middle of name",
			textPattern:   "import { foo_bar } from 'baz';",
			textArgs:      []interface{}{},
			isConvexFile:  false,
			shouldBlock:   false,
			expectedMatch: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			text := fmt.Sprintf(tt.textPattern, tt.textArgs...)
			output := checkUnderscorePrefixes(text, tt.isConvexFile)

			if tt.shouldBlock {
				if output == nil {
					t.Fatal("expected block decision, got nil")
				}
				if output.Decision != "block" {
					t.Errorf("expected block, got %s", output.Decision)
				}
				if !strings.Contains(output.Reason, tt.expectedMatch) {
					t.Errorf("expected reason to contain %q, got %q", tt.expectedMatch, output.Reason)
				}
				if !strings.Contains(output.Reason, "BLOCKED: Underscore prefix workaround detected") {
					t.Errorf("expected standard block message in reason")
				}
			} else {
				if output != nil {
					t.Errorf("expected no block, got %s with reason: %s", output.Decision, output.Reason)
				}
			}
		})
	}
}

func TestCheckUnderscorePrefixes_ConvexExceptions(t *testing.T) {
	// Build test strings dynamically to avoid triggering the hook on this file
	underscore := "_"
	colon := ":"

	tests := []struct {
		name         string
		text         string
		isConvexFile bool
		shouldBlock  bool
	}{
		{
			name:         "allows convex id field in convex file",
			text:         fmt.Sprintf("Key%s %sid", colon, underscore),
			isConvexFile: true,
			shouldBlock:  false,
		},
		{
			name:         "allows convex creationTime field in convex file",
			text:         fmt.Sprintf("Key%s %screationTime", colon, underscore),
			isConvexFile: true,
			shouldBlock:  false,
		},
		{
			name:         "blocks convex id field in non-convex file",
			text:         fmt.Sprintf("Key%s %sid", colon, underscore),
			isConvexFile: false,
			shouldBlock:  true,
		},
		{
			name:         "blocks convex creationTime field in non-convex file",
			text:         fmt.Sprintf("Key%s %screationTime", colon, underscore),
			isConvexFile: false,
			shouldBlock:  true,
		},
		{
			name:         "blocks other underscore prefixes in convex file",
			text:         fmt.Sprintf("const { prop%s %sunused } = obj;", colon, underscore),
			isConvexFile: true,
			shouldBlock:  true,
		},
		{
			name:         "property access not blocked (no colon pattern)",
			text:         fmt.Sprintf("sortKey: (doc) => doc.%sid", underscore),
			isConvexFile: false,
			shouldBlock:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := checkUnderscorePrefixes(tt.text, tt.isConvexFile)

			if tt.shouldBlock {
				if output == nil {
					t.Fatal("expected block decision, got nil")
				}
				if output.Decision != "block" {
					t.Errorf("expected block, got %s", output.Decision)
				}
			} else {
				if output != nil {
					t.Errorf("expected no block, got %s with reason: %s", output.Decision, output.Reason)
				}
			}
		})
	}
}

func TestIsInConvexDirectory(t *testing.T) {
	tests := []struct {
		name     string
		filePath string
		expected bool
	}{
		{
			name:     "convex directory",
			filePath: "/project/convex/schema.ts",
			expected: true,
		},
		{
			name:     "convex subdirectory",
			filePath: "/project/convex/stories/storyMutations.ts",
			expected: true,
		},
		{
			name:     "packages backend convex",
			filePath: "/Volumes/Developer/code/project/packages/backend/convex/model/stories.ts",
			expected: true,
		},
		{
			name:     "non-convex directory",
			filePath: "/project/src/components/Button.tsx",
			expected: false,
		},
		{
			name:     "empty path",
			filePath: "",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isInConvexDirectory(tt.filePath)
			if result != tt.expected {
				t.Errorf("expected %v, got %v for path %s", tt.expected, result, tt.filePath)
			}
		})
	}
}

func TestCheckESLintDisable_BlocksComments(t *testing.T) {
	tests := []struct {
		name          string
		textPattern   string
		textArgs      []interface{}
		shouldBlock   bool
		expectedMatch string
	}{
		{
			name:          "blocks inline eslint-disable",
			textPattern:   "// %s%s-next-line",
			textArgs:      []interface{}{"eslint", "-disable"},
			shouldBlock:   true,
			expectedMatch: "//",
		},
		{
			name:          "blocks block eslint-disable",
			textPattern:   "/* %s%s */",
			textArgs:      []interface{}{"eslint", "-disable"},
			shouldBlock:   true,
			expectedMatch: "/*",
		},
		{
			name:          "blocks eslint-disable with extra spaces",
			textPattern:   "//  %s%s-line",
			textArgs:      []interface{}{"eslint", "-disable"},
			shouldBlock:   true,
			expectedMatch: "//",
		},
		{
			name:          "blocks multiline eslint-disable",
			textPattern:   "/*  %s%s-next-line */",
			textArgs:      []interface{}{"eslint", "-disable"},
			shouldBlock:   true,
			expectedMatch: "/*",
		},
		{
			name:          "allows normal comments",
			textPattern:   "// This is a regular comment",
			textArgs:      []interface{}{},
			shouldBlock:   false,
			expectedMatch: "",
		},
		{
			name:          "allows code without eslint",
			textPattern:   "const foo = 'bar';",
			textArgs:      []interface{}{},
			shouldBlock:   false,
			expectedMatch: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			text := fmt.Sprintf(tt.textPattern, tt.textArgs...)
			output := checkESLintDisable(text)

			if tt.shouldBlock {
				if output == nil {
					t.Fatal("expected block decision, got nil")
				}
				if output.Decision != "block" {
					t.Errorf("expected block, got %s", output.Decision)
				}
				if !strings.Contains(output.Reason, tt.expectedMatch) {
					t.Errorf("expected reason to contain %q, got %q", tt.expectedMatch, output.Reason)
				}
				if !strings.Contains(output.Reason, "BLOCKED: ESLint suppression comment detected") {
					t.Errorf("expected standard block message in reason")
				}
			} else {
				if output != nil {
					t.Errorf("expected no block, got %s with reason: %s", output.Decision, output.Reason)
				}
			}
		})
	}
}

func TestCheckTSIgnore_WarnsButAllows(t *testing.T) {
	tests := []struct {
		name          string
		textPattern   string
		textArgs      []interface{}
		shouldWarn    bool
		expectedMatch string
	}{
		{
			name:          "warns on ts-ignore",
			textPattern:   "// %sts-%s",
			textArgs:      []interface{}{"@", "ignore"},
			shouldWarn:    true,
			expectedMatch: "// @",
		},
		{
			name:          "warns on ts-expect-error",
			textPattern:   "// %sts-expect-%s",
			textArgs:      []interface{}{"@", "error"},
			shouldWarn:    true,
			expectedMatch: "// @",
		},
		{
			name:          "warns on ts-nocheck",
			textPattern:   "// %sts-%s",
			textArgs:      []interface{}{"@", "nocheck"},
			shouldWarn:    true,
			expectedMatch: "// @",
		},
		{
			name:          "warns with extra spaces",
			textPattern:   "//  %sts-%s - reason",
			textArgs:      []interface{}{"@", "ignore"},
			shouldWarn:    true,
			expectedMatch: "//  @",
		},
		{
			name:          "allows normal code",
			textPattern:   "const foo: string = 'bar';",
			textArgs:      []interface{}{},
			shouldWarn:    false,
			expectedMatch: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			text := fmt.Sprintf(tt.textPattern, tt.textArgs...)
			output := checkTSIgnore(text)

			if tt.shouldWarn {
				if output == nil {
					t.Fatal("expected warning output, got nil")
				}
				if output.Decision != "approve" {
					t.Errorf("expected approve (with warning), got %s", output.Decision)
				}
				if !strings.Contains(output.Reason, tt.expectedMatch) {
					t.Errorf("expected reason to contain %q, got %q", tt.expectedMatch, output.Reason)
				}
				if !strings.Contains(output.Reason, "WARNING: TypeScript suppression comment detected") {
					t.Errorf("expected warning message in reason")
				}
			} else {
				if output != nil {
					t.Errorf("expected no output, got %s with reason: %s", output.Decision, output.Reason)
				}
			}
		})
	}
}

func TestGetTextToCheck(t *testing.T) {
	tests := []struct {
		name      string
		toolInput map[string]interface{}
		expected  string
	}{
		{
			name: "prefers new_string over content",
			toolInput: map[string]interface{}{
				"new_string": "new text",
				"content":    "old text",
			},
			expected: "new text",
		},
		{
			name: "uses content when new_string absent",
			toolInput: map[string]interface{}{
				"content": "content text",
			},
			expected: "content text",
		},
		{
			name: "returns empty when new_string is empty",
			toolInput: map[string]interface{}{
				"new_string": "",
				"content":    "content text",
			},
			expected: "content text",
		},
		{
			name: "returns empty when both absent",
			toolInput: map[string]interface{}{
				"file_path": "/some/path",
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getTextToCheck(tt.toolInput)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestIntegration_EditTool(t *testing.T) {
	tests := []struct {
		name           string
		newString      string
		expectDecision string
		expectBlock    bool
	}{
		{
			name:           "blocks underscore import in Edit",
			newString:      fmt.Sprintf("import { Foo %s %sFoo } from 'bar';", "as", "_"),
			expectDecision: "block",
			expectBlock:    true,
		},
		{
			name:           "blocks eslint-disable in Edit",
			newString:      fmt.Sprintf("// %s%s-next-line\nconst x = 1;", "eslint", "-disable"),
			expectDecision: "block",
			expectBlock:    true,
		},
		{
			name:           "warns but allows ts-ignore in Edit",
			newString:      fmt.Sprintf("// %sts-%s\nconst x = 1;", "@", "ignore"),
			expectDecision: "approve",
			expectBlock:    false,
		},
		{
			name:           "approves normal code in Edit",
			newString:      "import { Foo } from 'bar';\nconst x = 1;",
			expectDecision: "approve",
			expectBlock:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := &HookInput{
				ToolName: "Edit",
				ToolInput: map[string]interface{}{
					"file_path":  "/some/file.ts",
					"new_string": tt.newString,
				},
			}

			output := processHook(input)

			if output.Decision != tt.expectDecision {
				t.Errorf("expected decision %s, got %s", tt.expectDecision, output.Decision)
			}

			if tt.expectBlock && output.Reason == "" {
				t.Error("expected reason for block decision")
			}
		})
	}
}

func TestIntegration_WriteTool(t *testing.T) {
	tests := []struct {
		name           string
		content        string
		expectDecision string
		expectBlock    bool
	}{
		{
			name:           "blocks underscore import in Write",
			content:        fmt.Sprintf("import { Foo %s %sFoo } from 'bar';", "as", "_"),
			expectDecision: "block",
			expectBlock:    true,
		},
		{
			name:           "blocks eslint-disable in Write",
			content:        fmt.Sprintf("/* %s%s */\nconst x = 1;", "eslint", "-disable"),
			expectDecision: "block",
			expectBlock:    true,
		},
		{
			name:           "approves normal code in Write",
			content:        "import { Foo } from 'bar';\nconst x = 1;",
			expectDecision: "approve",
			expectBlock:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := &HookInput{
				ToolName: "Write",
				ToolInput: map[string]interface{}{
					"file_path": "/some/file.ts",
					"content":   tt.content,
				},
			}

			output := processHook(input)

			if output.Decision != tt.expectDecision {
				t.Errorf("expected decision %s, got %s", tt.expectDecision, output.Decision)
			}

			if tt.expectBlock && output.Reason == "" {
				t.Error("expected reason for block decision")
			}
		})
	}
}

func TestRun_EndToEnd(t *testing.T) {
	underscorePattern := fmt.Sprintf("%s %sX", "as", "_")
	tests := []struct {
		name           string
		inputJSON      string
		expectDecision string
		expectError    bool
	}{
		{
			name: "complete flow with block",
			inputJSON: fmt.Sprintf(`{
				"tool_name": "Edit",
				"tool_input": {
					"file_path": "/test.ts",
					"new_string": "import { X %s } from 'y';"
				}
			}`, underscorePattern),
			expectDecision: "block",
			expectError:    false,
		},
		{
			name: "complete flow with approve",
			inputJSON: `{
				"tool_name": "Edit",
				"tool_input": {
					"file_path": "/test.ts",
					"new_string": "import { X } from 'y';"
				}
			}`,
			expectDecision: "approve",
			expectError:    false,
		},
		{
			name:           "invalid JSON",
			inputJSON:      `{invalid json}`,
			expectDecision: "",
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdin := strings.NewReader(tt.inputJSON)
			stdout := &bytes.Buffer{}

			err := run(stdin, stdout)

			if tt.expectError {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			var output HookOutput
			if err := json.Unmarshal(stdout.Bytes(), &output); err != nil {
				t.Fatalf("failed to unmarshal output: %v", err)
			}

			if output.Decision != tt.expectDecision {
				t.Errorf("expected decision %s, got %s", tt.expectDecision, output.Decision)
			}
		})
	}
}

func TestReadHookInput(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expectError bool
	}{
		{
			name:        "valid JSON",
			input:       `{"tool_name": "Edit", "tool_input": {}}`,
			expectError: false,
		},
		{
			name:        "invalid JSON",
			input:       `{invalid}`,
			expectError: true,
		},
		{
			name:        "empty input",
			input:       ``,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := strings.NewReader(tt.input)
			_, err := readHookInput(reader)

			if tt.expectError && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestWriteHookOutput(t *testing.T) {
	tests := []struct {
		name   string
		output *HookOutput
	}{
		{
			name: "approve without reason",
			output: &HookOutput{
				Decision: "approve",
			},
		},
		{
			name: "block with reason",
			output: &HookOutput{
				Decision: "block",
				Reason:   "Test reason",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := &bytes.Buffer{}
			err := writeHookOutput(buf, tt.output)

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			var result HookOutput
			if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
				t.Fatalf("failed to unmarshal output: %v", err)
			}

			if result.Decision != tt.output.Decision {
				t.Errorf("expected decision %s, got %s", tt.output.Decision, result.Decision)
			}
			if result.Reason != tt.output.Reason {
				t.Errorf("expected reason %s, got %s", tt.output.Reason, result.Reason)
			}
		})
	}
}
