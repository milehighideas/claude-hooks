package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGetAppType(t *testing.T) {
	tests := []struct {
		name     string
		filePath string
		want     string
	}{
		{
			name:     "mobile app",
			filePath: "/project/packages/mobile/src/Component.tsx",
			want:     "mobile",
		},
		{
			name:     "native app",
			filePath: "/project/packages/native/screens/Home.tsx",
			want:     "native",
		},
		{
			name:     "web app",
			filePath: "/project/packages/web/components/Button.tsx",
			want:     "web",
		},
		{
			name:     "portal app",
			filePath: "/project/packages/portal/src/Dashboard.tsx",
			want:     "portal",
		},
		{
			name:     "unknown app",
			filePath: "/project/packages/backend/utils/helper.ts",
			want:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getAppType(tt.filePath)
			if got != tt.want {
				t.Errorf("getAppType() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsTestFile(t *testing.T) {
	tests := []struct {
		name     string
		filePath string
		want     bool
	}{
		{
			name:     "unit test file",
			filePath: "/src/Component.test.tsx",
			want:     true,
		},
		{
			name:     "spec test file",
			filePath: "/src/Component.spec.ts",
			want:     true,
		},
		{
			name:     "e2e test file",
			filePath: "/src/Component.e2e.ts",
			want:     true,
		},
		{
			name:     "maestro test file",
			filePath: "/src/Component.maestro.yaml",
			want:     true,
		},
		{
			name:     "regular component",
			filePath: "/src/Component.tsx",
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isTestFile(tt.filePath)
			if got != tt.want {
				t.Errorf("isTestFile() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsTypeOrBarrelFile(t *testing.T) {
	tests := []struct {
		name     string
		filePath string
		want     bool
	}{
		{
			name:     "index.ts barrel",
			filePath: "/src/components/index.ts",
			want:     true,
		},
		{
			name:     "index.tsx barrel",
			filePath: "/src/screens/index.tsx",
			want:     true,
		},
		{
			name:     "types file",
			filePath: "/src/types/User.ts",
			want:     true,
		},
		{
			name:     "regular component",
			filePath: "/src/components/Button.tsx",
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isTypeOrBarrelFile(tt.filePath)
			if got != tt.want {
				t.Errorf("isTypeOrBarrelFile() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetUnitTestPath(t *testing.T) {
	tests := []struct {
		name     string
		filePath string
		want     string
	}{
		{
			name:     "tsx file",
			filePath: "/src/Component.tsx",
			want:     "/src/Component.test.tsx",
		},
		{
			name:     "ts file",
			filePath: "/src/utils/helper.ts",
			want:     "/src/utils/helper.test.ts",
		},
		{
			name:     "unsupported extension",
			filePath: "/src/data.json",
			want:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getUnitTestPath(tt.filePath)
			if got != tt.want {
				t.Errorf("getUnitTestPath() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetE2ETestPath(t *testing.T) {
	tests := []struct {
		name     string
		filePath string
		appType  string
		want     string
	}{
		{
			name:     "mobile app",
			filePath: "/src/Screen.tsx",
			appType:  "mobile",
			want:     "/src/Screen.maestro.yaml",
		},
		{
			name:     "native app",
			filePath: "/src/Screen.tsx",
			appType:  "native",
			want:     "/src/Screen.maestro.yaml",
		},
		{
			name:     "web app",
			filePath: "/src/Component.tsx",
			appType:  "web",
			want:     "/src/Component.e2e.ts",
		},
		{
			name:     "portal app",
			filePath: "/src/Component.tsx",
			appType:  "portal",
			want:     "/src/Component.e2e.ts",
		},
		{
			name:     "unknown app type",
			filePath: "/src/Component.tsx",
			appType:  "backend",
			want:     "",
		},
		{
			name:     "empty app type",
			filePath: "/src/Component.tsx",
			appType:  "",
			want:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getE2ETestPath(tt.filePath, tt.appType)
			if got != tt.want {
				t.Errorf("getE2ETestPath() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsScreen(t *testing.T) {
	tests := []struct {
		name     string
		filePath string
		want     bool
	}{
		{
			name:     "screen file",
			filePath: "/src/screens/Home.tsx",
			want:     true,
		},
		{
			name:     "nested screen file",
			filePath: "/packages/mobile/src/screens/auth/Login.tsx",
			want:     true,
		},
		{
			name:     "component file",
			filePath: "/src/components/Button.tsx",
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isScreen(tt.filePath)
			if got != tt.want {
				t.Errorf("isScreen() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsCRUDFolder(t *testing.T) {
	tests := []struct {
		name     string
		filePath string
		want     bool
	}{
		{
			name:     "create folder",
			filePath: "/src/create/UserForm.tsx",
			want:     true,
		},
		{
			name:     "update folder",
			filePath: "/src/update/ProfileForm.tsx",
			want:     true,
		},
		{
			name:     "delete folder",
			filePath: "/src/delete/ConfirmDelete.tsx",
			want:     true,
		},
		{
			name:     "regular component",
			filePath: "/src/components/Button.tsx",
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isCRUDFolder(tt.filePath)
			if got != tt.want {
				t.Errorf("isCRUDFolder() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsHookOrUtil(t *testing.T) {
	tests := []struct {
		name     string
		filePath string
		want     bool
	}{
		{
			name:     "hook file",
			filePath: "/src/hooks/useAuth.ts",
			want:     true,
		},
		{
			name:     "util file",
			filePath: "/src/utils/formatter.ts",
			want:     true,
		},
		{
			name:     "component file",
			filePath: "/src/components/Button.tsx",
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isHookOrUtil(tt.filePath)
			if got != tt.want {
				t.Errorf("isHookOrUtil() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsInteractiveComponent(t *testing.T) {
	// Create temporary directory
	tmpDir := t.TempDir()

	tests := []struct {
		name     string
		filePath string
		content  string
		want     bool
		wantErr  bool
	}{
		{
			name:     "component with useState",
			filePath: filepath.Join(tmpDir, "Component1.tsx"),
			content: `import { useState } from 'react';
export const Component = () => {
  const [value, setValue] = useState('');
  return <div>{value}</div>;
};`,
			want:    true,
			wantErr: false,
		},
		{
			name:     "component with useForm",
			filePath: filepath.Join(tmpDir, "Component2.tsx"),
			content: `import { useForm } from 'react-hook-form';
export const Component = () => {
  const form = useForm();
  return <form />;
};`,
			want:    true,
			wantErr: false,
		},
		{
			name:     "component with useMutation",
			filePath: filepath.Join(tmpDir, "Component3.tsx"),
			content: `import { useMutation } from 'convex/react';
export const Component = () => {
  const mutation = useMutation(api.users.create);
  return <button onClick={() => mutation({})}>Save</button>;
};`,
			want:    true,
			wantErr: false,
		},
		{
			name:     "display-only component",
			filePath: filepath.Join(tmpDir, "Component4.tsx"),
			content: `export const Component = ({ title }: { title: string }) => {
  return <h1>{title}</h1>;
};`,
			want:    false,
			wantErr: false,
		},
		{
			name:     "create folder component",
			filePath: filepath.Join(tmpDir, "create", "UserForm.tsx"),
			content: `export const UserForm = () => {
  return <form />;
};`,
			want:    true,
			wantErr: false,
		},
		{
			name:     "update folder component",
			filePath: filepath.Join(tmpDir, "update", "ProfileForm.tsx"),
			content: `export const ProfileForm = () => {
  return <form />;
};`,
			want:    true,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create directory structure if needed
			dir := filepath.Dir(tt.filePath)
			if err := os.MkdirAll(dir, 0755); err != nil {
				t.Fatalf("failed to create directory: %v", err)
			}

			// Write test file
			if err := os.WriteFile(tt.filePath, []byte(tt.content), 0644); err != nil {
				t.Fatalf("failed to write test file: %v", err)
			}

			got, err := isInteractiveComponent(tt.filePath)
			if (err != nil) != tt.wantErr {
				t.Errorf("isInteractiveComponent() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("isInteractiveComponent() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsComponentWriteOperation(t *testing.T) {
	tests := []struct {
		name         string
		data         HookData
		wantIsOp     bool
		wantFilePath string
	}{
		{
			name: "Write operation on component",
			data: HookData{
				ToolName: "Write",
				ToolInput: ToolInput{
					FilePath: "/src/components/Button.tsx",
				},
			},
			wantIsOp:     true,
			wantFilePath: "/src/components/Button.tsx",
		},
		{
			name: "Edit operation on component",
			data: HookData{
				ToolName: "Edit",
				ToolInput: ToolInput{
					FilePath: "/src/components/Input.tsx",
				},
			},
			wantIsOp:     true,
			wantFilePath: "/src/components/Input.tsx",
		},
		{
			name: "Write operation on test file",
			data: HookData{
				ToolName: "Write",
				ToolInput: ToolInput{
					FilePath: "/src/components/Button.test.tsx",
				},
			},
			wantIsOp:     false,
			wantFilePath: "",
		},
		{
			name: "Read operation on component",
			data: HookData{
				ToolName: "Read",
				ToolInput: ToolInput{
					FilePath: "/src/components/Button.tsx",
				},
			},
			wantIsOp:     false,
			wantFilePath: "",
		},
		{
			name: "Write operation on non-component",
			data: HookData{
				ToolName: "Write",
				ToolInput: ToolInput{
					FilePath: "/src/utils/helper.ts",
				},
			},
			wantIsOp:     false,
			wantFilePath: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotIsOp, gotFilePath := isComponentWriteOperation(tt.data)
			if gotIsOp != tt.wantIsOp {
				t.Errorf("isComponentWriteOperation() gotIsOp = %v, want %v", gotIsOp, tt.wantIsOp)
			}
			if gotFilePath != tt.wantFilePath {
				t.Errorf("isComponentWriteOperation() gotFilePath = %v, want %v", gotFilePath, tt.wantFilePath)
			}
		})
	}
}

func TestCheckTestRequirements(t *testing.T) {
	// Create temporary directory with test structure
	tmpDir := t.TempDir()

	// Create a screen component (should need both unit and E2E)
	screenDir := filepath.Join(tmpDir, "packages", "mobile", "src", "screens")
	if err := os.MkdirAll(screenDir, 0755); err != nil {
		t.Fatalf("failed to create screen directory: %v", err)
	}

	screenFile := filepath.Join(screenDir, "Home.tsx")
	screenContent := `export const HomeScreen = () => <div>Home</div>;`
	if err := os.WriteFile(screenFile, []byte(screenContent), 0644); err != nil {
		t.Fatalf("failed to write screen file: %v", err)
	}

	// Create a hook (should need unit test only)
	hookDir := filepath.Join(tmpDir, "packages", "web", "src", "hooks")
	if err := os.MkdirAll(hookDir, 0755); err != nil {
		t.Fatalf("failed to create hook directory: %v", err)
	}

	hookFile := filepath.Join(hookDir, "useAuth.ts")
	hookContent := `export const useAuth = () => { return {}; };`
	if err := os.WriteFile(hookFile, []byte(hookContent), 0644); err != nil {
		t.Fatalf("failed to write hook file: %v", err)
	}

	// Create a display component (should need unit test only)
	componentDir := filepath.Join(tmpDir, "packages", "portal", "src", "components")
	if err := os.MkdirAll(componentDir, 0755); err != nil {
		t.Fatalf("failed to create component directory: %v", err)
	}

	displayFile := filepath.Join(componentDir, "Header.tsx")
	displayContent := `export const Header = ({ title }: { title: string }) => <h1>{title}</h1>;`
	if err := os.WriteFile(displayFile, []byte(displayContent), 0644); err != nil {
		t.Fatalf("failed to write display file: %v", err)
	}

	tests := []struct {
		name           string
		filePath       string
		wantViolations int
		wantUnitTest   bool
		wantE2ETest    bool
		wantErr        bool
	}{
		{
			name:           "screen without tests",
			filePath:       screenFile,
			wantViolations: 2, // missing unit and E2E
			wantUnitTest:   true,
			wantE2ETest:    true,
			wantErr:        false,
		},
		{
			name:           "hook without test",
			filePath:       hookFile,
			wantViolations: 1, // missing unit test
			wantUnitTest:   true,
			wantE2ETest:    false,
			wantErr:        false,
		},
		{
			name:           "display component without test",
			filePath:       displayFile,
			wantViolations: 1, // missing unit test
			wantUnitTest:   true,
			wantE2ETest:    false,
			wantErr:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			violations, err := checkTestRequirements(tt.filePath)
			if (err != nil) != tt.wantErr {
				t.Errorf("checkTestRequirements() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if len(violations) != tt.wantViolations {
				t.Errorf("checkTestRequirements() violations = %v, want %v", len(violations), tt.wantViolations)
			}

			// Check that expected violations are present
			hasUnitTest := false
			hasE2ETest := false
			for _, v := range violations {
				if strings.Contains(v.ExpectedPath, ".test.tsx") || strings.Contains(v.ExpectedPath, ".test.ts") {
					hasUnitTest = true
				}
				if strings.Contains(v.ExpectedPath, ".maestro.yaml") || strings.Contains(v.ExpectedPath, ".e2e.ts") {
					hasE2ETest = true
				}
			}

			if tt.wantUnitTest && !hasUnitTest {
				t.Errorf("checkTestRequirements() expected unit test violation not found. Violations: %+v", violations)
			}
			if tt.wantE2ETest && !hasE2ETest {
				t.Errorf("checkTestRequirements() expected E2E test violation not found. Violations: %+v", violations)
			}
		})
	}
}

func TestCheckDisabled(t *testing.T) {
	// Save original value
	originalValue := os.Getenv("CLAUDE_HOOKS_AST_VALIDATION")
	defer func() {
		if originalValue != "" {
			_ = os.Setenv("CLAUDE_HOOKS_AST_VALIDATION", originalValue)
		} else {
			_ = os.Unsetenv("CLAUDE_HOOKS_AST_VALIDATION")
		}
	}()

	tests := []struct {
		name   string
		envVal string
		want   bool
	}{
		{
			name:   "disabled",
			envVal: "false",
			want:   true,
		},
		{
			name:   "enabled",
			envVal: "true",
			want:   false,
		},
		{
			name:   "not set",
			envVal: "",
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envVal != "" {
				_ = os.Setenv("CLAUDE_HOOKS_AST_VALIDATION", tt.envVal)
			} else {
				_ = os.Unsetenv("CLAUDE_HOOKS_AST_VALIDATION")
			}

			got := checkDisabled()
			if got != tt.want {
				t.Errorf("checkDisabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsTestFileWriteOperation(t *testing.T) {
	tests := []struct {
		name         string
		data         HookData
		wantIsOp     bool
		wantFilePath string
	}{
		{
			name: "Write .test.tsx",
			data: HookData{
				ToolName:  "Write",
				ToolInput: ToolInput{FilePath: "/src/components/Button.test.tsx"},
			},
			wantIsOp:     true,
			wantFilePath: "/src/components/Button.test.tsx",
		},
		{
			name: "Write .test.ts",
			data: HookData{
				ToolName:  "Write",
				ToolInput: ToolInput{FilePath: "/src/hooks/useAuth.test.ts"},
			},
			wantIsOp:     true,
			wantFilePath: "/src/hooks/useAuth.test.ts",
		},
		{
			name: "Edit .test.tsx",
			data: HookData{
				ToolName:  "Edit",
				ToolInput: ToolInput{FilePath: "/src/components/Button.test.tsx"},
			},
			wantIsOp:     true,
			wantFilePath: "/src/components/Button.test.tsx",
		},
		{
			name: "Write non-test file",
			data: HookData{
				ToolName:  "Write",
				ToolInput: ToolInput{FilePath: "/src/components/Button.tsx"},
			},
			wantIsOp:     false,
			wantFilePath: "",
		},
		{
			name: "Read .test.tsx",
			data: HookData{
				ToolName:  "Read",
				ToolInput: ToolInput{FilePath: "/src/components/Button.test.tsx"},
			},
			wantIsOp:     false,
			wantFilePath: "",
		},
		{
			name: ".spec.ts is not targeted",
			data: HookData{
				ToolName:  "Write",
				ToolInput: ToolInput{FilePath: "/src/util.spec.ts"},
			},
			wantIsOp:     false,
			wantFilePath: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotIsOp, gotFilePath := isTestFileWriteOperation(tt.data)
			if gotIsOp != tt.wantIsOp {
				t.Errorf("isTestFileWriteOperation() gotIsOp = %v, want %v", gotIsOp, tt.wantIsOp)
			}
			if gotFilePath != tt.wantFilePath {
				t.Errorf("isTestFileWriteOperation() gotFilePath = %v, want %v", gotFilePath, tt.wantFilePath)
			}
		})
	}
}

func TestIsStubContent(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    bool
	}{
		{
			name: "canonical stub",
			content: `import { describe, it, expect } from "vitest";

describe("Component.test.tsx", () => {
  it("should be defined", () => {
    expect(true).toBe(true);
  });
});`,
			want: true,
		},
		{
			name: "multiple stubs, all placeholder",
			content: `describe("x", () => {
  it("a", () => { expect(true).toBe(true); });
  it("b", () => { expect(true).toBe(true); });
});`,
			want: true,
		},
		{
			name: "stub with whitespace variations",
			content: `it("stub", () => { expect( true ) . toBe( true ); });`,
			want: true,
		},
		{
			name: "real test, no stub at all",
			content: `import { render, screen } from "@testing-library/react";
import { Button } from "./Button";
it("renders label", () => {
  render(<Button label="Hi" />);
  expect(screen.getByText("Hi")).toBeTruthy();
});`,
			want: false,
		},
		{
			name: "mixed: one stub plus one real assertion — NOT a stub",
			content: `it("a", () => { expect(true).toBe(true); });
it("b", () => { expect(value).toBe(42); });`,
			want: false,
		},
		{
			name:    "empty file",
			content: ``,
			want:    false,
		},
		{
			name:    "file with no expect calls",
			content: `describe("x", () => { it("a", () => {}); });`,
			want:    false,
		},
		{
			name: "expect(true).toBe(false) is not the stub pattern",
			content: `it("a", () => { expect(true).toBe(false); });`,
			want:    false,
		},
		{
			name: "comment mentioning expect(true).toBe(true) alongside real tests",
			content: `// avoid expect(true).toBe(true)
it("real", () => { expect(x).toBe(1); });`,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isStubContent(tt.content)
			if got != tt.want {
				t.Errorf("isStubContent() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFindProjectRoot(t *testing.T) {
	tmpDir := t.TempDir()

	// Layout:
	//   tmpDir/
	//     project-a/
	//       .pre-commit.json
	//       apps/web/components/Foo.tsx
	//     project-b/
	//       apps/mobile/screens/Home.tsx     (no marker anywhere)
	//     project-c/
	//       .pre-commit.json
	//       packages/inner/
	//         .pre-commit.json             (nested marker)
	//         src/Button.tsx
	projectA := filepath.Join(tmpDir, "project-a")
	projectAComponent := filepath.Join(projectA, "apps", "web", "components", "Foo.tsx")
	projectB := filepath.Join(tmpDir, "project-b")
	projectBFile := filepath.Join(projectB, "apps", "mobile", "screens", "Home.tsx")
	projectC := filepath.Join(tmpDir, "project-c")
	nestedInner := filepath.Join(projectC, "packages", "inner")
	nestedFile := filepath.Join(nestedInner, "src", "Button.tsx")

	// project-a
	if err := os.MkdirAll(filepath.Dir(projectAComponent), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectA, ".pre-commit.json"), []byte("{}"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.WriteFile(projectAComponent, []byte(""), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	// project-b (no marker)
	if err := os.MkdirAll(filepath.Dir(projectBFile), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(projectBFile, []byte(""), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	// project-c with nested marker
	if err := os.MkdirAll(filepath.Dir(nestedFile), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectC, ".pre-commit.json"), []byte("{}"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.WriteFile(filepath.Join(nestedInner, ".pre-commit.json"), []byte("{}"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.WriteFile(nestedFile, []byte(""), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	tests := []struct {
		name     string
		filePath string
		want     string
	}{
		{
			name:     "marker at project root",
			filePath: projectAComponent,
			want:     projectA,
		},
		{
			name:     "no marker anywhere",
			filePath: projectBFile,
			want:     "",
		},
		{
			name:     "nearest marker wins",
			filePath: nestedFile,
			want:     nestedInner,
		},
		{
			name:     "marker directory is returned when file sits next to it",
			filePath: filepath.Join(projectA, "sibling.tsx"),
			want:     projectA,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := findProjectRoot(tt.filePath)
			if got != tt.want {
				t.Errorf("findProjectRoot(%q) = %q, want %q", tt.filePath, got, tt.want)
			}
		})
	}
}

func TestIsProjectOptedIn(t *testing.T) {
	tmpDir := t.TempDir()

	// Cases covered:
	//   enabled      — features.testFiles = true
	//   disabled     — features.testFiles = false
	//   absent       — features block present, testFiles key absent
	//   noFeatures   — no features block
	//   invalid      — malformed JSON
	//   jsonc        — JSONC comments are supported
	//   noMarker     — no .pre-commit.json in any parent
	enabled := filepath.Join(tmpDir, "enabled")
	disabled := filepath.Join(tmpDir, "disabled")
	absent := filepath.Join(tmpDir, "absent")
	noFeatures := filepath.Join(tmpDir, "no-features")
	invalid := filepath.Join(tmpDir, "invalid")
	jsoncDir := filepath.Join(tmpDir, "jsonc")
	noMarker := filepath.Join(tmpDir, "no-marker")

	for _, dir := range []string{enabled, disabled, absent, noFeatures, invalid, jsoncDir, noMarker} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
	}

	mustWrite := func(dir, content string) {
		if err := os.WriteFile(filepath.Join(dir, ".pre-commit.json"), []byte(content), 0644); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	mustWrite(enabled, `{"features":{"testFiles":true}}`)
	mustWrite(disabled, `{"features":{"testFiles":false}}`)
	mustWrite(absent, `{"features":{"lintTypecheck":true}}`)
	mustWrite(noFeatures, `{}`)
	mustWrite(invalid, `{"features":{`)
	mustWrite(jsoncDir, `{
  // testFiles flips on per-tool-use validation
  "features": {
    "testFiles": true // opt in
  }
}`)

	tests := []struct {
		name     string
		filePath string
		want     bool
	}{
		{
			name:     "feature enabled",
			filePath: filepath.Join(enabled, "apps", "web", "components", "Foo.tsx"),
			want:     true,
		},
		{
			name:     "feature disabled",
			filePath: filepath.Join(disabled, "apps", "web", "components", "Foo.tsx"),
			want:     false,
		},
		{
			name:     "feature key absent",
			filePath: filepath.Join(absent, "src", "Foo.tsx"),
			want:     false,
		},
		{
			name:     "no features block",
			filePath: filepath.Join(noFeatures, "src", "Foo.tsx"),
			want:     false,
		},
		{
			name:     "malformed config",
			filePath: filepath.Join(invalid, "src", "Foo.tsx"),
			want:     false,
		},
		{
			name:     "jsonc comments allowed",
			filePath: filepath.Join(jsoncDir, "src", "Foo.tsx"),
			want:     true,
		},
		{
			name:     "no marker anywhere",
			filePath: filepath.Join(noMarker, "src", "Foo.tsx"),
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isProjectOptedIn(tt.filePath)
			if got != tt.want {
				t.Errorf("isProjectOptedIn(%q) = %v, want %v", tt.filePath, got, tt.want)
			}
		})
	}
}

func TestRunGatesOnProjectOptIn(t *testing.T) {
	// Clear env override so opt-in is the only gate under test.
	t.Setenv("CLAUDE_HOOKS_AST_VALIDATION", "")

	tmpDir := t.TempDir()

	// Helper: write a component file that the validator will flag as missing tests.
	// The file uses useState so isInteractiveComponent returns true → requires unit + E2E.
	interactiveSrc := `import { useState } from 'react';
export const Foo = () => { const [v] = useState(0); return <div>{v}</div>; };`

	// Opted-in project
	optIn := filepath.Join(tmpDir, "opt-in")
	optInComponent := filepath.Join(optIn, "apps", "web", "components", "Foo.tsx")
	if err := os.MkdirAll(filepath.Dir(optInComponent), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(optIn, ".pre-commit.json"), []byte(`{"features":{"testFiles":true}}`), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if err := os.WriteFile(optInComponent, []byte(interactiveSrc), 0644); err != nil {
		t.Fatalf("write component: %v", err)
	}

	// Opted-in project with tests alongside (should pass)
	passing := filepath.Join(tmpDir, "opt-in-passing")
	passingComponent := filepath.Join(passing, "apps", "web", "components", "Bar.tsx")
	if err := os.MkdirAll(filepath.Dir(passingComponent), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(passing, ".pre-commit.json"), []byte(`{"features":{"testFiles":true}}`), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if err := os.WriteFile(passingComponent, []byte(interactiveSrc), 0644); err != nil {
		t.Fatalf("write component: %v", err)
	}
	if err := os.WriteFile(filepath.Join(filepath.Dir(passingComponent), "Bar.test.tsx"), []byte(``), 0644); err != nil {
		t.Fatalf("write unit test: %v", err)
	}
	if err := os.WriteFile(filepath.Join(filepath.Dir(passingComponent), "Bar.e2e.ts"), []byte(``), 0644); err != nil {
		t.Fatalf("write e2e test: %v", err)
	}

	// Project without .pre-commit.json — should be a silent no-op
	noOpt := filepath.Join(tmpDir, "no-opt")
	noOptComponent := filepath.Join(noOpt, "apps", "web", "components", "Foo.tsx")
	if err := os.MkdirAll(filepath.Dir(noOptComponent), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(noOptComponent, []byte(interactiveSrc), 0644); err != nil {
		t.Fatalf("write component: %v", err)
	}

	// Project with features.testFiles=false — also a silent no-op
	optOut := filepath.Join(tmpDir, "opt-out")
	optOutComponent := filepath.Join(optOut, "apps", "web", "components", "Foo.tsx")
	if err := os.MkdirAll(filepath.Dir(optOutComponent), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(optOut, ".pre-commit.json"), []byte(`{"features":{"testFiles":false}}`), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if err := os.WriteFile(optOutComponent, []byte(interactiveSrc), 0644); err != nil {
		t.Fatalf("write component: %v", err)
	}

	// Stub test file inside an opted-in project — should be blocked
	stubProj := filepath.Join(tmpDir, "opt-in-stub")
	stubTestFile := filepath.Join(stubProj, "apps", "web", "components", "Baz.test.tsx")
	if err := os.MkdirAll(filepath.Dir(stubTestFile), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(stubProj, ".pre-commit.json"), []byte(`{"features":{"testFiles":true}}`), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	// Same stub write in a non-opted-in project — should NOT be blocked
	stubNoOpt := filepath.Join(tmpDir, "no-opt-stub")
	stubNoOptFile := filepath.Join(stubNoOpt, "apps", "web", "components", "Baz.test.tsx")
	if err := os.MkdirAll(filepath.Dir(stubNoOptFile), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	stubContent := `import { describe, it, expect } from "vitest";
describe("Baz", () => { it("t", () => { expect(true).toBe(true); }); });`

	tests := []struct {
		name         string
		data         HookData
		wantExit     int
		wantStderrIn string // substring expected in stderr when blocking
	}{
		{
			name: "opted in, missing tests → block",
			data: HookData{
				ToolName:  "Edit",
				ToolInput: ToolInput{FilePath: optInComponent},
			},
			wantExit:     2,
			wantStderrIn: "Test file requirements not met",
		},
		{
			name: "opted in, tests present → allow",
			data: HookData{
				ToolName:  "Edit",
				ToolInput: ToolInput{FilePath: passingComponent},
			},
			wantExit: 0,
		},
		{
			name: "no .pre-commit.json → silent no-op",
			data: HookData{
				ToolName:  "Edit",
				ToolInput: ToolInput{FilePath: noOptComponent},
			},
			wantExit: 0,
		},
		{
			name: "features.testFiles=false → silent no-op",
			data: HookData{
				ToolName:  "Edit",
				ToolInput: ToolInput{FilePath: optOutComponent},
			},
			wantExit: 0,
		},
		{
			name: "opted in, stub test file → block",
			data: HookData{
				ToolName: "Write",
				ToolInput: ToolInput{
					FilePath: stubTestFile,
					Content:  stubContent,
				},
			},
			wantExit:     2,
			wantStderrIn: "Stub test file rejected",
		},
		{
			name: "no opt-in, stub test file → silent no-op",
			data: HookData{
				ToolName: "Write",
				ToolInput: ToolInput{
					FilePath: stubNoOptFile,
					Content:  stubContent,
				},
			},
			wantExit: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			got := run(tt.data, &buf)
			if got != tt.wantExit {
				t.Errorf("run() exit = %d, want %d (stderr: %q)", got, tt.wantExit, buf.String())
			}
			if tt.wantStderrIn != "" && !strings.Contains(buf.String(), tt.wantStderrIn) {
				t.Errorf("run() stderr %q does not contain %q", buf.String(), tt.wantStderrIn)
			}
			if tt.wantExit == 0 && buf.Len() != 0 {
				t.Errorf("run() expected no stderr on exit 0, got %q", buf.String())
			}
		})
	}
}

func TestIsFileInScope(t *testing.T) {
	projectRoot := "/project"

	tests := []struct {
		name     string
		filePath string
		cfg      projectConfig
		want     bool
	}{
		{
			name:     "empty config → everything in scope",
			filePath: "/project/apps/web/components/Foo.tsx",
			cfg:      projectConfig{},
			want:     true,
		},
		{
			name:     "appPaths match → in scope",
			filePath: "/project/apps/web/components/Foo.tsx",
			cfg: projectConfig{
				TestFilesConfig: testFilesConfig{
					AppPaths: []string{"apps/web"},
				},
			},
			want: true,
		},
		{
			name:     "appPaths set but no match → out of scope",
			filePath: "/project/apps/legacy/components/Foo.tsx",
			cfg: projectConfig{
				TestFilesConfig: testFilesConfig{
					AppPaths: []string{"apps/web"},
				},
			},
			want: false,
		},
		{
			name:     "multiple appPaths, one matches",
			filePath: "/project/packages/ui/src/Button.tsx",
			cfg: projectConfig{
				TestFilesConfig: testFilesConfig{
					AppPaths: []string{"apps/web", "packages/ui"},
				},
			},
			want: true,
		},
		{
			name:     "excludePaths wins over appPaths",
			filePath: "/project/apps/web/legacy/Foo.tsx",
			cfg: projectConfig{
				TestFilesConfig: testFilesConfig{
					AppPaths:     []string{"apps/web"},
					ExcludePaths: []string{"apps/web/legacy"},
				},
			},
			want: false,
		},
		{
			name:     "excludePaths with empty appPaths",
			filePath: "/project/apps/legacy/Foo.tsx",
			cfg: projectConfig{
				TestFilesConfig: testFilesConfig{
					ExcludePaths: []string{"apps/legacy"},
				},
			},
			want: false,
		},
		{
			name:     "file outside project root → treat as in scope (degrade gracefully)",
			filePath: "/unrelated/foo/bar.tsx",
			cfg:      projectConfig{},
			want:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isFileInScope(projectRoot, tt.filePath, tt.cfg)
			if got != tt.want {
				t.Errorf("isFileInScope(%q) = %v, want %v", tt.filePath, got, tt.want)
			}
		})
	}
}

func TestRunAppliesScopeFilter(t *testing.T) {
	t.Setenv("CLAUDE_HOOKS_AST_VALIDATION", "")

	tmpDir := t.TempDir()
	projectRoot := filepath.Join(tmpDir, "proj")

	if err := os.MkdirAll(projectRoot, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectRoot, ".pre-commit.json"), []byte(`{
  "features": { "testFiles": true },
  "testFilesConfig": {
    "appPaths": ["apps/web"],
    "excludePaths": ["apps/web/legacy"]
  }
}`), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	interactive := `import { useState } from 'react';
export const X = () => { const [v] = useState(0); return <div>{v}</div>; };`

	inScope := filepath.Join(projectRoot, "apps", "web", "components", "InScope.tsx")
	outOfScope := filepath.Join(projectRoot, "apps", "mobile", "components", "OutOfScope.tsx")
	excluded := filepath.Join(projectRoot, "apps", "web", "legacy", "Excluded.tsx")

	for _, f := range []string{inScope, outOfScope, excluded} {
		if err := os.MkdirAll(filepath.Dir(f), 0755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(f, []byte(interactive), 0644); err != nil {
			t.Fatalf("write: %v", err)
		}
	}

	tests := []struct {
		name     string
		filePath string
		wantExit int
	}{
		{name: "file inside appPaths → validated", filePath: inScope, wantExit: 2},
		{name: "file outside appPaths → no-op", filePath: outOfScope, wantExit: 0},
		{name: "file matches excludePaths → no-op", filePath: excluded, wantExit: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			got := run(HookData{ToolName: "Edit", ToolInput: ToolInput{FilePath: tt.filePath}}, &buf)
			if got != tt.wantExit {
				t.Errorf("run() = %d, want %d (stderr: %q)", got, tt.wantExit, buf.String())
			}
		})
	}
}

func TestRunRespectsEnvDisable(t *testing.T) {
	t.Setenv("CLAUDE_HOOKS_AST_VALIDATION", "false")

	tmpDir := t.TempDir()
	component := filepath.Join(tmpDir, "opt-in", "apps", "web", "components", "Foo.tsx")
	if err := os.MkdirAll(filepath.Dir(component), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "opt-in", ".pre-commit.json"), []byte(`{"features":{"testFiles":true}}`), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.WriteFile(component, []byte(`import { useState } from 'react'; export const X = () => { const [v] = useState(0); return <div>{v}</div> };`), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	got := run(HookData{ToolName: "Edit", ToolInput: ToolInput{FilePath: component}}, io.Discard)
	if got != 0 {
		t.Errorf("run() with env disable = %d, want 0", got)
	}
}

func TestGetResultingTestContent(t *testing.T) {
	tmpDir := t.TempDir()

	// Seed a file for Edit simulation
	existingPath := filepath.Join(tmpDir, "Seed.test.tsx")
	existingContent := `it("a", () => { expect(x).toBe(1); });`
	if err := os.WriteFile(existingPath, []byte(existingContent), 0644); err != nil {
		t.Fatalf("failed to seed file: %v", err)
	}

	tests := []struct {
		name    string
		data    HookData
		want    string
		wantErr bool
	}{
		{
			name: "Write returns content directly",
			data: HookData{
				ToolName: "Write",
				ToolInput: ToolInput{
					FilePath: "/virtual/new.test.tsx",
					Content:  `expect(true).toBe(true);`,
				},
			},
			want:    `expect(true).toBe(true);`,
			wantErr: false,
		},
		{
			name: "Edit applies replacement to existing file",
			data: HookData{
				ToolName: "Edit",
				ToolInput: ToolInput{
					FilePath:  existingPath,
					OldString: `expect(x).toBe(1)`,
					NewString: `expect(true).toBe(true)`,
				},
			},
			want:    `it("a", () => { expect(true).toBe(true); });`,
			wantErr: false,
		},
		{
			name: "Edit on missing file errors",
			data: HookData{
				ToolName: "Edit",
				ToolInput: ToolInput{
					FilePath:  filepath.Join(tmpDir, "does-not-exist.test.tsx"),
					OldString: "a",
					NewString: "b",
				},
			},
			want:    "",
			wantErr: true,
		},
		{
			name: "Unknown tool errors",
			data: HookData{
				ToolName:  "Read",
				ToolInput: ToolInput{FilePath: existingPath},
			},
			want:    "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getResultingTestContent(tt.data)
			if (err != nil) != tt.wantErr {
				t.Errorf("getResultingTestContent() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("getResultingTestContent() = %q, want %q", got, tt.want)
			}
		})
	}
}
