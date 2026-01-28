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

func TestCheckDirectConvexImports(t *testing.T) {
	tests := []struct {
		name         string
		filePath     string
		code         string
		wantViolated bool
	}{
		{
			name:         "convex import in data-layer",
			filePath:     "/app/packages/data-layer/hooks.tsx",
			code:         `import { useQuery } from 'convex/react';`,
			wantViolated: false,
		},
		{
			name:         "convex import in component",
			filePath:     "/app/components/UserProfile.tsx",
			code:         `import { useQuery } from 'convex/react';`,
			wantViolated: true,
		},
		{
			name:         "allowed convex import (Preloaded)",
			filePath:     "/app/components/UserProfile.tsx",
			code:         `import { Preloaded, usePreloadedQuery } from 'convex/react';`,
			wantViolated: false,
		},
		{
			name:         "generated api import",
			filePath:     "/app/components/UserProfile.tsx",
			code:         `import { api } from '../_generated/api';`,
			wantViolated: true,
		},
		{
			name:         "no convex imports",
			filePath:     "/app/components/Button.tsx",
			code:         `import React from 'react';`,
			wantViolated: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			analysis := analyzeCode(tt.code, tt.filePath)
			violations := checkDirectConvexImports(analysis, tt.filePath)
			hasError := len(violations) > 0
			if hasError != tt.wantViolated {
				t.Errorf("checkDirectConvexImports() violated = %v, want %v", hasError, tt.wantViolated)
			}
		})
	}
}

func TestCheckStateInScreens(t *testing.T) {
	tests := []struct {
		name         string
		filePath     string
		code         string
		wantViolated bool
	}{
		{
			name:         "screen with state",
			filePath:     "/app/screens/HomeScreen.tsx",
			code:         `const [count, setCount] = useState(0);`,
			wantViolated: true,
		},
		{
			name:         "screen without state",
			filePath:     "/app/screens/HomeScreen.tsx",
			code:         `export const HomeScreen = () => <div />;`,
			wantViolated: false,
		},
		{
			name:         "component with state (not screen)",
			filePath:     "/app/components/Counter.tsx",
			code:         `const [count, setCount] = useState(0);`,
			wantViolated: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			analysis := analyzeCode(tt.code, tt.filePath)
			violations := checkStateInScreens(analysis, tt.filePath)
			hasError := len(violations) > 0
			if hasError != tt.wantViolated {
				t.Errorf("checkStateInScreens() violated = %v, want %v", hasError, tt.wantViolated)
			}
		})
	}
}

func TestCheckMultipleExports(t *testing.T) {
	tests := []struct {
		name         string
		filePath     string
		code         string
		wantViolated bool
	}{
		{
			name:         "CRUD component with single export",
			filePath:     "/app/features/users/create/CreateUserForm.tsx",
			code:         `export const CreateUserForm = () => <div />;`,
			wantViolated: false,
		},
		{
			name:     "CRUD component with multiple exports",
			filePath: "/app/features/users/create/CreateUserForm.tsx",
			code: `export const CreateUserForm = () => <div />;
export const helper = () => {};`,
			wantViolated: true,
		},
		{
			name:     "non-CRUD component with multiple exports",
			filePath: "/app/components/ui/Button.tsx",
			code: `export const Button = () => <div />;
export const IconButton = () => <div />;`,
			wantViolated: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			analysis := analyzeCode(tt.code, tt.filePath)
			violations := checkMultipleExports(analysis, tt.filePath)
			hasError := len(violations) > 0
			if hasError != tt.wantViolated {
				t.Errorf("checkMultipleExports() violated = %v, want %v", hasError, tt.wantViolated)
			}
		})
	}
}

func TestCheckFileSize(t *testing.T) {
	tests := []struct {
		name        string
		filePath    string
		lineCount   int
		wantWarning bool
	}{
		{
			name:        "small screen",
			filePath:    "/app/screens/HomeScreen.tsx",
			lineCount:   50,
			wantWarning: false,
		},
		{
			name:        "large screen",
			filePath:    "/app/screens/HomeScreen.tsx",
			lineCount:   150,
			wantWarning: true,
		},
		{
			name:        "large hook",
			filePath:    "/app/hooks/useAuth.tsx",
			lineCount:   200,
			wantWarning: true,
		},
		{
			name:        "large component",
			filePath:    "/app/components/UserProfile.tsx",
			lineCount:   250,
			wantWarning: true,
		},
		{
			name:        "large script (allowed)",
			filePath:    "/app/scripts/generate.tsx",
			lineCount:   500,
			wantWarning: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			analysis := &ASTAnalysis{
				FilePath:  tt.filePath,
				LineCount: tt.lineCount,
			}
			violations := checkFileSize(analysis, tt.filePath)
			hasWarning := len(violations) > 0
			if hasWarning != tt.wantWarning {
				t.Errorf("checkFileSize() warning = %v, want %v", hasWarning, tt.wantWarning)
			}
		})
	}
}

func TestCheckTypeExportsLocation(t *testing.T) {
	tests := []struct {
		name         string
		filePath     string
		code         string
		wantViolated bool
	}{
		{
			name:         "type export in types folder",
			filePath:     "/app/types/User.tsx",
			code:         `export type User = { id: string; name: string; };`,
			wantViolated: false,
		},
		{
			name:         "type export outside types folder",
			filePath:     "/app/components/UserProfile.tsx",
			code:         `export type User = { id: string; name: string; };`,
			wantViolated: true,
		},
		{
			name:         "interface export outside types folder",
			filePath:     "/app/components/UserProfile.tsx",
			code:         `export interface UserProps { id: string; }`,
			wantViolated: true,
		},
		{
			name:         "regular export outside types folder",
			filePath:     "/app/components/Button.tsx",
			code:         `export const Button = () => <div />;`,
			wantViolated: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			analysis := analyzeCode(tt.code, tt.filePath)
			violations := checkTypeExportsLocation(analysis, tt.filePath)
			hasError := len(violations) > 0
			if hasError != tt.wantViolated {
				t.Errorf("checkTypeExportsLocation() violated = %v, want %v", hasError, tt.wantViolated)
			}
		})
	}
}

func TestCheckMixedConcerns(t *testing.T) {
	tests := []struct {
		name        string
		code        string
		wantWarning bool
	}{
		{
			name: "mixed concerns (data + UI + state)",
			code: `import { useUser } from '@dashtag/data-layer/generated-hooks';
import { Button } from '@/components/ui/button';
import { useState } from 'react';
const [count, setCount] = useState(0);`,
			wantWarning: true,
		},
		{
			name: "only data layer",
			code: `import { useUser } from '@dashtag/data-layer/generated-hooks';
export const UserData = () => <div />;`,
			wantWarning: false,
		},
		{
			name: "data + UI (no state)",
			code: `import { useUser } from '@dashtag/data-layer/generated-hooks';
import { Button } from '@/components/ui/button';`,
			wantWarning: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			analysis := analyzeCode(tt.code, "test.tsx")
			violations := checkMixedConcerns(analysis, "test.tsx")
			hasWarning := len(violations) > 0
			if hasWarning != tt.wantWarning {
				t.Errorf("checkMixedConcerns() warning = %v, want %v", hasWarning, tt.wantWarning)
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
