package main

import (
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
