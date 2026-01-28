package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFilterTypeScriptFiles(t *testing.T) {
	tests := []struct {
		name     string
		files    []string
		expected []string
	}{
		{
			name:     "only TypeScript files",
			files:    []string{"app.ts", "component.tsx"},
			expected: []string{"app.ts", "component.tsx"},
		},
		{
			name:     "mixed files",
			files:    []string{"app.ts", "styles.css", "component.tsx", "README.md"},
			expected: []string{"app.ts", "component.tsx"},
		},
		{
			name:     "no TypeScript files",
			files:    []string{"styles.css", "README.md", "package.json"},
			expected: nil,
		},
		{
			name:     "empty list",
			files:    []string{},
			expected: nil,
		},
		{
			name:     "JavaScript files excluded",
			files:    []string{"app.js", "component.jsx", "util.ts"},
			expected: []string{"util.ts"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := filterTypeScriptFiles(tt.files)
			if len(result) != len(tt.expected) {
				t.Errorf("expected %d files, got %d", len(tt.expected), len(result))
				return
			}
			for i, f := range result {
				if f != tt.expected[i] {
					t.Errorf("expected %s at index %d, got %s", tt.expected[i], i, f)
				}
			}
		})
	}
}

func TestIncrementalTypecheck_ToRelativePaths(t *testing.T) {
	// Create a temporary directory structure
	tmpDir := t.TempDir()
	projectDir := filepath.Join(tmpDir, "project")
	os.MkdirAll(projectDir, 0755)

	// Create test files
	testFile := filepath.Join(projectDir, "app.ts")
	os.WriteFile(testFile, []byte(""), 0644)

	outsideFile := filepath.Join(tmpDir, "outside.ts")
	os.WriteFile(outsideFile, []byte(""), 0644)

	it := NewIncrementalTypecheck(projectDir, nil, TypecheckFilter{})

	tests := []struct {
		name     string
		files    []string
		expected int
	}{
		{
			name:     "file in project",
			files:    []string{testFile},
			expected: 1,
		},
		{
			name:     "file outside project",
			files:    []string{outsideFile},
			expected: 0,
		},
		{
			name:     "mixed files",
			files:    []string{testFile, outsideFile},
			expected: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := it.toRelativePaths(tt.files)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if len(result) != tt.expected {
				t.Errorf("expected %d paths, got %d", tt.expected, len(result))
			}
		})
	}
}

func TestIncrementalTypecheck_FindTypeDefinitionFiles(t *testing.T) {
	// Create a temporary directory structure
	tmpDir := t.TempDir()

	// Create type definition files
	os.WriteFile(filepath.Join(tmpDir, "uniwind-types.d.ts"), []byte(""), 0644)
	os.WriteFile(filepath.Join(tmpDir, "expo-env.d.ts"), []byte(""), 0644)

	it := NewIncrementalTypecheck(tmpDir, nil, TypecheckFilter{})
	typeDefFiles := it.findTypeDefinitionFiles()

	if len(typeDefFiles) != 2 {
		t.Errorf("expected 2 type definition files, got %d", len(typeDefFiles))
	}

	// Verify specific files are found
	found := make(map[string]bool)
	for _, f := range typeDefFiles {
		found[f] = true
	}

	if !found["uniwind-types.d.ts"] {
		t.Error("expected to find uniwind-types.d.ts")
	}
	if !found["expo-env.d.ts"] {
		t.Error("expected to find expo-env.d.ts")
	}
}

func TestIncrementalTypecheck_FilterErrorsToChangedFiles(t *testing.T) {
	it := NewIncrementalTypecheck(".", nil, TypecheckFilter{})

	tests := []struct {
		name         string
		output       string
		changedFiles []string
		expectedLen  int
	}{
		{
			name: "error in changed file",
			output: `app.ts(10,5): error TS2322: Type 'string' is not assignable to type 'number'.
  The expected type comes from property 'count'`,
			changedFiles: []string{"app.ts"},
			expectedLen:  1,
		},
		{
			name: "error in unchanged file",
			output: `other.ts(10,5): error TS2322: Type 'string' is not assignable to type 'number'.
  The expected type comes from property 'count'`,
			changedFiles: []string{"app.ts"},
			expectedLen:  0,
		},
		{
			name: "multiple errors mixed",
			output: `app.ts(10,5): error TS2322: Type 'string' is not assignable to type 'number'.
  The expected type comes from property 'count'
other.ts(20,3): error TS2345: Argument of type 'string' is not assignable.
component.tsx(5,1): error TS2304: Cannot find name 'React'.`,
			changedFiles: []string{"app.ts", "component.tsx"},
			expectedLen:  2,
		},
		{
			name:         "no errors",
			output:       "",
			changedFiles: []string{"app.ts"},
			expectedLen:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := it.filterErrorsToChangedFiles(tt.output, tt.changedFiles)
			if len(result) != tt.expectedLen {
				t.Errorf("expected %d errors, got %d: %v", tt.expectedLen, len(result), result)
			}
		})
	}
}

func TestIncrementalTypecheck_IsChangedFile(t *testing.T) {
	it := NewIncrementalTypecheck(".", nil, TypecheckFilter{})

	changedSet := map[string]bool{
		"app.ts":           true,
		"components/ui.ts": true,
		"ui.ts":            true, // basename
	}

	tests := []struct {
		name      string
		errorFile string
		expected  bool
	}{
		{
			name:      "exact match",
			errorFile: "app.ts",
			expected:  true,
		},
		{
			name:      "path match",
			errorFile: "components/ui.ts",
			expected:  true,
		},
		{
			name:      "basename match",
			errorFile: "src/components/ui.ts",
			expected:  true,
		},
		{
			name:      "no match",
			errorFile: "other.ts",
			expected:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := it.isChangedFile(tt.errorFile, changedSet)
			if result != tt.expected {
				t.Errorf("expected %v for %s, got %v", tt.expected, tt.errorFile, result)
			}
		})
	}
}
