package main

import (
	"os"
	"testing"
)

func TestIsTypeScriptFile(t *testing.T) {
	tests := []struct {
		name     string
		filePath string
		want     bool
	}{
		{"tsx file", "Component.tsx", true},
		{"ts file", "utils.ts", true},
		{"d.ts file", "types.d.ts", false},
		{"js file", "script.js", false},
		{"jsx file", "Component.jsx", false},
		{"txt file", "readme.txt", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isTypeScriptFile(tt.filePath)
			if got != tt.want {
				t.Errorf("isTypeScriptFile(%q) = %v, want %v", tt.filePath, got, tt.want)
			}
		})
	}
}

func TestExtractBashFileWrite(t *testing.T) {
	tests := []struct {
		name        string
		command     string
		wantPath    string
		wantContent string
	}{
		{
			name: "heredoc with single quotes",
			command: `cat > Component.tsx << 'EOF'
export const Component = () => <div>Hello</div>;
EOF`,
			wantPath:    "Component.tsx",
			wantContent: "export const Component = () => <div>Hello</div>;",
		},
		{
			name:        "echo redirect",
			command:     `echo "export const Foo = 1;" > file.tsx`,
			wantPath:    "file.tsx",
			wantContent: "export const Foo = 1;",
		},
		{
			name: "tee heredoc",
			command: `tee Component.tsx << 'EOF'
export const Component = () => <div>Test</div>;
EOF`,
			wantPath:    "Component.tsx",
			wantContent: "export const Component = () => <div>Test</div>;",
		},
		{
			name:        "no file write",
			command:     "ls -la",
			wantPath:    "",
			wantContent: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotPath, gotContent := extractBashFileWrite(tt.command)
			if gotPath != tt.wantPath {
				t.Errorf("extractBashFileWrite() path = %q, want %q", gotPath, tt.wantPath)
			}
			if gotContent != tt.wantContent {
				t.Errorf("extractBashFileWrite() content = %q, want %q", gotContent, tt.wantContent)
			}
		})
	}
}

func TestAnalyzeCode(t *testing.T) {
	tests := []struct {
		name      string
		code      string
		wantCheck func(*ASTAnalysis) error
	}{
		{
			name: "basic imports",
			code: `import React from 'react';
import { useState } from 'react';
import { api } from 'convex/react';`,
			wantCheck: func(a *ASTAnalysis) error {
				if len(a.Imports) != 3 {
					return errorf("expected 3 imports, got %d", len(a.Imports))
				}
				if a.Imports[0].Source != "react" {
					return errorf("expected first import from 'react', got %q", a.Imports[0].Source)
				}
				if a.Imports[2].Source != "convex/react" {
					return errorf("expected third import from 'convex/react', got %q", a.Imports[2].Source)
				}
				return nil
			},
		},
		{
			name: "exports",
			code: `export const Component = () => <div />;
export function helper() {}
export type Foo = string;`,
			wantCheck: func(a *ASTAnalysis) error {
				if len(a.Exports) != 3 {
					return errorf("expected 3 exports, got %d", len(a.Exports))
				}
				// Check for type export
				hasTypeExport := false
				for _, exp := range a.Exports {
					if exp.IsTypeOnly {
						hasTypeExport = true
						break
					}
				}
				if !hasTypeExport {
					return errorf("expected to find type export")
				}
				return nil
			},
		},
		{
			name: "state management",
			code: `import { useState, useReducer } from 'react';
const [state, setState] = useState(0);
const [state2, dispatch] = useReducer(reducer, initialState);`,
			wantCheck: func(a *ASTAnalysis) error {
				if len(a.StateManagement) != 2 {
					return errorf("expected 2 state hooks, got %d", len(a.StateManagement))
				}
				if a.StateManagement[0].Hook != "useState" {
					return errorf("expected first hook to be useState, got %q", a.StateManagement[0].Hook)
				}
				if a.StateManagement[1].Hook != "useReducer" {
					return errorf("expected second hook to be useReducer, got %q", a.StateManagement[1].Hook)
				}
				return nil
			},
		},
		{
			name: "responsibility comment",
			code: `/** Single Responsibility: This component handles user authentication */
export const AuthComponent = () => <div />;`,
			wantCheck: func(a *ASTAnalysis) error {
				if !a.HasResponsibilityComment {
					return errorf("expected to find responsibility comment")
				}
				return nil
			},
		},
		{
			name: "line count",
			code: `line 1
line 2
line 3`,
			wantCheck: func(a *ASTAnalysis) error {
				if a.LineCount != 3 {
					return errorf("expected 3 lines, got %d", a.LineCount)
				}
				return nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			analysis := analyzeCode(tt.code, "test.tsx")
			if analysis == nil {
				t.Fatal("analyzeCode returned nil")
			}
			if err := tt.wantCheck(analysis); err != nil {
				t.Error(err)
			}
		})
	}
}

func TestIsComponentWriteOperation(t *testing.T) {
	tests := []struct {
		name        string
		toolData    ToolData
		wantIsTS    bool
		wantPath    string
		wantContent string
	}{
		{
			name: "Write tool with tsx file",
			toolData: ToolData{
				ToolName: "Write",
				ToolInput: ToolInput{
					FilePath: "/app/Component.tsx",
					Content:  "export const Component = () => <div />;",
				},
			},
			wantIsTS:    true,
			wantPath:    "/app/Component.tsx",
			wantContent: "export const Component = () => <div />;",
		},
		{
			name: "Edit tool with ts file",
			toolData: ToolData{
				ToolName: "Edit",
				ToolInput: ToolInput{
					FilePath: "/app/utils.ts",
					Content:  "export const helper = () => {};",
				},
			},
			wantIsTS:    true,
			wantPath:    "/app/utils.ts",
			wantContent: "export const helper = () => {};",
		},
		{
			name: "Write tool with js file",
			toolData: ToolData{
				ToolName: "Write",
				ToolInput: ToolInput{
					FilePath: "/app/script.js",
					Content:  "console.log('hello');",
				},
			},
			wantIsTS:    false,
			wantPath:    "",
			wantContent: "",
		},
		{
			name: "Bash tool with heredoc",
			toolData: ToolData{
				ToolName: "Bash",
				ToolInput: ToolInput{
					Command: `cat > Component.tsx << 'EOF'
export const Component = () => <div />;
EOF`,
				},
			},
			wantIsTS:    true,
			wantPath:    "Component.tsx",
			wantContent: "export const Component = () => <div />;",
		},
		{
			name: "Read tool (not a write operation)",
			toolData: ToolData{
				ToolName: "Read",
				ToolInput: ToolInput{
					FilePath: "/app/Component.tsx",
				},
			},
			wantIsTS:    false,
			wantPath:    "",
			wantContent: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotIsTS, gotPath, gotContent := isComponentWriteOperation(tt.toolData)
			if gotIsTS != tt.wantIsTS {
				t.Errorf("isComponentWriteOperation() isTS = %v, want %v", gotIsTS, tt.wantIsTS)
			}
			if gotPath != tt.wantPath {
				t.Errorf("isComponentWriteOperation() path = %q, want %q", gotPath, tt.wantPath)
			}
			if gotContent != tt.wantContent {
				t.Errorf("isComponentWriteOperation() content = %q, want %q", gotContent, tt.wantContent)
			}
		})
	}
}

func TestValidateSRPComplianceOptIn(t *testing.T) {
	// Test opt-in behavior
	if err := os.Setenv("CLAUDE_HOOKS_AST_VALIDATION", "false"); err != nil {
		t.Fatalf("failed to set env: %v", err)
	}
	defer func() {
		_ = os.Unsetenv("CLAUDE_HOOKS_AST_VALIDATION")
	}()

	code := `import { useQuery } from 'convex/react';` // This would normally be a violation
	analysis := analyzeCode(code, "/app/Component.tsx")
	violations := validateSRPCompliance(analysis, "/app/Component.tsx")

	if len(violations) != 0 {
		t.Errorf("expected no violations when validation is disabled, got %d", len(violations))
	}
}

// Helper function for test error messages
func errorf(format string, args ...interface{}) error {
	return &testError{msg: sprintf(format, args...)}
}

type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}

func sprintf(format string, args ...interface{}) string {
	// Simple sprintf implementation for tests
	result := format
	for _, arg := range args {
		switch v := arg.(type) {
		case string:
			result = replaceFirst(result, "%q", "\""+v+"\"")
			result = replaceFirst(result, "%s", v)
		case int:
			result = replaceFirst(result, "%d", itoa(v))
		case bool:
			result = replaceFirst(result, "%v", btoa(v))
		}
	}
	return result
}

func replaceFirst(s, old, new string) string {
	i := 0
	for i < len(s) {
		if i+len(old) <= len(s) && s[i:i+len(old)] == old {
			return s[:i] + new + s[i+len(old):]
		}
		i++
	}
	return s
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	negative := n < 0
	if negative {
		n = -n
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	if negative {
		digits = append([]byte{'-'}, digits...)
	}
	return string(digits)
}

func btoa(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
