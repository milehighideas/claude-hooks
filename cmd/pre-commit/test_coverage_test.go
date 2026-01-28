package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestShouldHaveTest(t *testing.T) {
	checker := NewTestCoverageChecker(TestCoverageConfig{
		RequireTestFolders: []string{"hooks", "read", "utils"},
		ExcludeFiles:       []string{"index.ts", "*.types.ts"},
		ExcludePaths:       []string{"__mocks__/"},
	})

	tests := []struct {
		name     string
		path     string
		expected bool
	}{
		{
			name:     "hook file should have test",
			path:     "apps/admin/src/components/routes/users/hooks/useUsers.ts",
			expected: true,
		},
		{
			name:     "read component should have test",
			path:     "apps/admin/src/components/routes/users/read/UserCard.tsx",
			expected: true,
		},
		{
			name:     "utils file should have test",
			path:     "apps/admin/src/components/routes/users/utils/formatters.ts",
			expected: true,
		},
		{
			name:     "index.ts should not require test",
			path:     "apps/admin/src/components/routes/users/hooks/index.ts",
			expected: false,
		},
		{
			name:     "types file should not require test",
			path:     "apps/admin/src/components/routes/users/types/user.types.ts",
			expected: false,
		},
		{
			name:     "screen file should not require test (not in required folders)",
			path:     "apps/admin/src/components/routes/users/screens/UsersScreen.tsx",
			expected: false,
		},
		{
			name:     "test file itself should not require test",
			path:     "apps/admin/src/components/routes/users/hooks/useUsers.test.ts",
			expected: false,
		},
		{
			name:     "mock file should not require test",
			path:     "apps/admin/src/__mocks__/convex.ts",
			expected: false,
		},
		{
			name:     "non-ts file should not require test",
			path:     "apps/admin/src/index.css",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := checker.shouldHaveTest(tt.path)
			if result != tt.expected {
				t.Errorf("shouldHaveTest(%s) = %v, want %v", tt.path, result, tt.expected)
			}
		})
	}
}

func TestGetTestFilePath(t *testing.T) {
	checker := NewTestCoverageChecker(TestCoverageConfig{})

	tests := []struct {
		source   string
		expected string
	}{
		{
			source:   "hooks/useUsers.ts",
			expected: "hooks/useUsers.test.ts",
		},
		{
			source:   "read/UserCard.tsx",
			expected: "read/UserCard.test.tsx",
		},
	}

	for _, tt := range tests {
		t.Run(tt.source, func(t *testing.T) {
			result := checker.getTestFilePath(tt.source)
			if result != tt.expected {
				t.Errorf("getTestFilePath(%s) = %s, want %s", tt.source, result, tt.expected)
			}
		})
	}
}

func TestMatchPattern(t *testing.T) {
	tests := []struct {
		filename string
		pattern  string
		expected bool
	}{
		{"index.ts", "index.ts", true},
		{"useUsers.ts", "index.ts", false},
		{"user.types.ts", "*.types.ts", true},
		{"useUsers.ts", "*.types.ts", false},
		{"UserCard.tsx", "*.tsx", true},
	}

	for _, tt := range tests {
		t.Run(tt.filename+"_"+tt.pattern, func(t *testing.T) {
			result := matchPattern(tt.filename, tt.pattern)
			if result != tt.expected {
				t.Errorf("matchPattern(%s, %s) = %v, want %v", tt.filename, tt.pattern, result, tt.expected)
			}
		})
	}
}

func TestTestCoverageChecker(t *testing.T) {
	// Create temp directory structure
	tempDir := t.TempDir()

	// Create app with proper structure
	hooksDir := filepath.Join(tempDir, "app", "hooks")
	os.MkdirAll(hooksDir, 0755)

	// Create a hook file without test
	os.WriteFile(filepath.Join(hooksDir, "useUsers.ts"), []byte("export function useUsers() {}"), 0644)
	os.WriteFile(filepath.Join(hooksDir, "index.ts"), []byte("export * from './useUsers'"), 0644)

	// Create a hook file with test
	os.WriteFile(filepath.Join(hooksDir, "useSettings.ts"), []byte("export function useSettings() {}"), 0644)
	os.WriteFile(filepath.Join(hooksDir, "useSettings.test.ts"), []byte("test('useSettings', () => {})"), 0644)

	checker := NewTestCoverageChecker(TestCoverageConfig{
		AppPaths:           []string{filepath.Join(tempDir, "app")},
		RequireTestFolders: []string{"hooks"},
		ExcludeFiles:       []string{"index.ts"},
	})

	violations, err := checker.Check()
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}

	// Should have exactly one violation (useUsers.ts missing test)
	if len(violations) != 1 {
		t.Errorf("Expected 1 violation, got %d", len(violations))
	}

	if len(violations) > 0 {
		if !filepath.IsAbs(violations[0].SourceFile) {
			// Make path absolute for comparison
			expectedSource := filepath.Join(hooksDir, "useUsers.ts")
			if violations[0].SourceFile != expectedSource {
				t.Errorf("Expected violation for useUsers.ts, got %s", violations[0].SourceFile)
			}
		}
	}
}
