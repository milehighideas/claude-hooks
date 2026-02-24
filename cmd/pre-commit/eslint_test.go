package main

import (
	"reflect"
	"testing"
)

func TestParseEslintErrors(t *testing.T) {
	tests := []struct {
		name     string
		output   string
		expected []lintError
	}{
		{
			name:     "empty output",
			output:   "",
			expected: nil,
		},
		{
			name:     "no errors",
			output:   "No ESLint warnings or errors\n",
			expected: nil,
		},
		{
			name: "single error",
			output: `/path/to/file.tsx
  10:5  error  'foo' is defined but never used  @typescript-eslint/no-unused-vars
`,
			expected: []lintError{
				{
					filePath: "/path/to/file.tsx",
					line:     "10",
					column:   "5",
					severity: "error",
					message:  "'foo' is defined but never used",
					rule:     "@typescript-eslint/no-unused-vars",
					fullText: "  10:5  error  'foo' is defined but never used  @typescript-eslint/no-unused-vars",
				},
			},
		},
		{
			name: "multiple errors same file",
			output: `/path/to/file.tsx
  10:5  error  'foo' is defined but never used  @typescript-eslint/no-unused-vars
  15:10  warning  Unexpected any. Specify a different type  @typescript-eslint/no-explicit-any
`,
			expected: []lintError{
				{
					filePath: "/path/to/file.tsx",
					line:     "10",
					column:   "5",
					severity: "error",
					message:  "'foo' is defined but never used",
					rule:     "@typescript-eslint/no-unused-vars",
					fullText: "  10:5  error  'foo' is defined but never used  @typescript-eslint/no-unused-vars",
				},
				{
					filePath: "/path/to/file.tsx",
					line:     "15",
					column:   "10",
					severity: "warning",
					message:  "Unexpected any. Specify a different type",
					rule:     "@typescript-eslint/no-explicit-any",
					fullText: "  15:10  warning  Unexpected any. Specify a different type  @typescript-eslint/no-explicit-any",
				},
			},
		},
		{
			name: "multiple files",
			output: `/path/to/file1.tsx
  10:5  error  Missing return type  @typescript-eslint/explicit-function-return-type

/path/to/file2.ts
  5:1  error  'bar' is never used  unused-imports/no-unused-vars
`,
			expected: []lintError{
				{
					filePath: "/path/to/file1.tsx",
					line:     "10",
					column:   "5",
					severity: "error",
					message:  "Missing return type",
					rule:     "@typescript-eslint/explicit-function-return-type",
					fullText: "  10:5  error  Missing return type  @typescript-eslint/explicit-function-return-type",
				},
				{
					filePath: "/path/to/file2.ts",
					line:     "5",
					column:   "1",
					severity: "error",
					message:  "'bar' is never used",
					rule:     "unused-imports/no-unused-vars",
					fullText: "  5:1  error  'bar' is never used  unused-imports/no-unused-vars",
				},
			},
		},
		{
			name: "error in test file",
			output: `/path/to/__tests__/utils.test.ts
  10:5  error  Type assertion error  @typescript-eslint/no-unsafe-assignment
`,
			expected: []lintError{
				{
					filePath: "/path/to/__tests__/utils.test.ts",
					line:     "10",
					column:   "5",
					severity: "error",
					message:  "Type assertion error",
					rule:     "@typescript-eslint/no-unsafe-assignment",
					fullText: "  10:5  error  Type assertion error  @typescript-eslint/no-unsafe-assignment",
				},
			},
		},
		{
			name: "mjs file",
			output: `/path/to/config.mjs
  1:1  error  Missing module declaration  import/no-unresolved
`,
			expected: []lintError{
				{
					filePath: "/path/to/config.mjs",
					line:     "1",
					column:   "1",
					severity: "error",
					message:  "Missing module declaration",
					rule:     "import/no-unresolved",
					fullText: "  1:1  error  Missing module declaration  import/no-unresolved",
				},
			},
		},
		{
			name: "cjs file",
			output: `/path/to/config.cjs
  5:10  warning  Require statement  @typescript-eslint/no-require-imports
`,
			expected: []lintError{
				{
					filePath: "/path/to/config.cjs",
					line:     "5",
					column:   "10",
					severity: "warning",
					message:  "Require statement",
					rule:     "@typescript-eslint/no-require-imports",
					fullText: "  5:10  warning  Require statement  @typescript-eslint/no-require-imports",
				},
			},
		},
		{
			name: "output with summary lines",
			output: `/path/to/file.tsx
  10:5  error  Error message  some-rule

1 problem (1 error, 0 warnings)
`,
			expected: []lintError{
				{
					filePath: "/path/to/file.tsx",
					line:     "10",
					column:   "5",
					severity: "error",
					message:  "Error message",
					rule:     "some-rule",
					fullText: "  10:5  error  Error message  some-rule",
				},
			},
		},
		{
			name: "high line and column numbers",
			output: `/path/to/large-file.tsx
  9999:150  error  Some error  some-rule
`,
			expected: []lintError{
				{
					filePath: "/path/to/large-file.tsx",
					line:     "9999",
					column:   "150",
					severity: "error",
					message:  "Some error",
					rule:     "some-rule",
					fullText: "  9999:150  error  Some error  some-rule",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseEslintErrors(tt.output)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("parseEslintErrors() = %+v, want %+v", result, tt.expected)
			}
		})
	}
}

func TestShouldFilterLintError(t *testing.T) {
	tests := []struct {
		name         string
		err          lintError
		rules        []string
		excludePaths []string
		expected     bool
	}{
		{
			name: "filter by rule",
			err: lintError{
				filePath: "/path/to/file.tsx",
				rule:     "@typescript-eslint/no-explicit-any",
			},
			rules:        []string{"@typescript-eslint/no-explicit-any"},
			excludePaths: []string{},
			expected:     true,
		},
		{
			name: "filter by unused-vars rule",
			err: lintError{
				filePath: "/path/to/file.tsx",
				rule:     "unused-imports/no-unused-vars",
			},
			rules:        []string{"unused-imports/no-unused-vars"},
			excludePaths: []string{},
			expected:     true,
		},
		{
			name: "do not filter unmatched rule",
			err: lintError{
				filePath: "/path/to/file.tsx",
				rule:     "@typescript-eslint/no-unsafe-call",
			},
			rules:        []string{"@typescript-eslint/no-explicit-any"},
			excludePaths: []string{},
			expected:     false,
		},
		{
			name: "filter by __tests__ path",
			err: lintError{
				filePath: "/path/to/__tests__/utils.test.ts",
				rule:     "some-rule",
			},
			rules:        []string{},
			excludePaths: []string{"__tests__/"},
			expected:     true,
		},
		{
			name: "filter by .test. path",
			err: lintError{
				filePath: "/path/to/utils.test.ts",
				rule:     "some-rule",
			},
			rules:        []string{},
			excludePaths: []string{".test."},
			expected:     true,
		},
		{
			name: "filter by .spec. path",
			err: lintError{
				filePath: "/path/to/component.spec.tsx",
				rule:     "some-rule",
			},
			rules:        []string{},
			excludePaths: []string{".spec."},
			expected:     true,
		},
		{
			name: "do not filter production file",
			err: lintError{
				filePath: "/path/to/component.tsx",
				rule:     "@typescript-eslint/no-unsafe-call",
			},
			rules:        []string{"@typescript-eslint/no-explicit-any"},
			excludePaths: []string{"__tests__/", ".test.", ".spec."},
			expected:     false,
		},
		{
			name: "empty filter lists do not filter",
			err: lintError{
				filePath: "/path/to/file.tsx",
				rule:     "@typescript-eslint/no-explicit-any",
			},
			rules:        []string{},
			excludePaths: []string{},
			expected:     false,
		},
		{
			name: "rule filter takes priority",
			err: lintError{
				filePath: "/path/to/production.tsx",
				rule:     "@typescript-eslint/no-explicit-any",
			},
			rules:        []string{"@typescript-eslint/no-explicit-any"},
			excludePaths: []string{"__tests__/"},
			expected:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := shouldFilterLintError(tt.err, tt.rules, tt.excludePaths, false)
			if result != tt.expected {
				t.Errorf("shouldFilterLintError() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestDefaultLintExcludePaths(t *testing.T) {
	// Defaults are empty - no filtering unless explicitly configured
	expected := []string{}
	if !reflect.DeepEqual(DefaultLintExcludePaths, expected) {
		t.Errorf("DefaultLintExcludePaths = %v, want %v", DefaultLintExcludePaths, expected)
	}
}

func TestShouldFilterLintErrorWithDefaults(t *testing.T) {
	// With empty defaults, nothing is filtered unless explicitly configured
	tests := []struct {
		name     string
		err      lintError
		expected bool
	}{
		{
			name: "default does not filter __tests__ path",
			err: lintError{
				filePath: "/path/to/__tests__/foo.ts",
				rule:     "some-rule",
			},
			expected: false,
		},
		{
			name: "default does not filter .test. path",
			err: lintError{
				filePath: "/path/to/utils.test.ts",
				rule:     "some-rule",
			},
			expected: false,
		},
		{
			name: "default does not filter .spec. path",
			err: lintError{
				filePath: "/path/to/component.spec.tsx",
				rule:     "some-rule",
			},
			expected: false,
		},
		{
			name: "default does not filter production file",
			err: lintError{
				filePath: "/path/to/component.tsx",
				rule:     "some-rule",
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// With default exclude paths (empty) and no rule filters
			result := shouldFilterLintError(tt.err, []string{}, DefaultLintExcludePaths, false)
			if result != tt.expected {
				t.Errorf("shouldFilterLintError() with defaults = %v, want %v", result, tt.expected)
			}
		})
	}
}
