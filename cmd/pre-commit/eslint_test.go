package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
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

func TestParseOxlintErrors(t *testing.T) {
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
			name:     "clean run prints nothing at all",
			output:   "\n",
			expected: nil,
		},
		{
			name:   "compact warning strips trailing help text",
			output: "src/steps.test.ts:31:43: warning jest(no-restricted-matchers): Use of toBeTruthy is disallowed help: Weak assertion\n",
			expected: []lintError{
				{
					filePath: "src/steps.test.ts",
					line:     "31",
					column:   "43",
					severity: "warning",
					message:  "Use of toBeTruthy is disallowed",
					rule:     "jest/no-restricted-matchers",
					fullText: "src/steps.test.ts:31:43: warning jest(no-restricted-matchers): Use of toBeTruthy is disallowed help: Weak assertion",
				},
			},
		},
		{
			name:   "compact error without help text",
			output: "components/hooks/useForm.ts:72:21: error react(incompatible-library): Use of incompatible library\n",
			expected: []lintError{
				{
					filePath: "components/hooks/useForm.ts",
					line:     "72",
					column:   "21",
					severity: "error",
					message:  "Use of incompatible library",
					rule:     "react/incompatible-library",
					fullText: "components/hooks/useForm.ts:72:21: error react(incompatible-library): Use of incompatible library",
				},
			},
		},
		{
			name:   "compact keeps a windows absolute path intact",
			output: "C:\\src\\app\\main.ts:4:9: error eslint(no-debugger): Unexpected debugger statement\n",
			expected: []lintError{
				{
					filePath: "C:\\src\\app\\main.ts",
					line:     "4",
					column:   "9",
					severity: "error",
					message:  "Unexpected debugger statement",
					rule:     "eslint/no-debugger",
					fullText: "C:\\src\\app\\main.ts:4:9: error eslint(no-debugger): Unexpected debugger statement",
				},
			},
		},
		{
			name: "graphical error still parsed",
			output: "  x eslint(no-unused-vars): 'foo' is never used\n" +
				"   ,-[src/app.ts:10:5]\n",
			expected: []lintError{
				{
					filePath: "src/app.ts",
					line:     "10",
					column:   "5",
					severity: "error",
					message:  "'foo' is never used",
					rule:     "eslint/no-unused-vars",
					fullText: "   ,-[src/app.ts:10:5]",
				},
			},
		},
		{
			name: "graphical warning still parsed",
			output: "  ! react(purity): Cannot call impure function during render\n" +
				"   ,-[src/Clock.tsx:7:22]\n",
			expected: []lintError{
				{
					filePath: "src/Clock.tsx",
					line:     "7",
					column:   "22",
					severity: "warning",
					message:  "Cannot call impure function during render",
					rule:     "react/purity",
					fullText: "   ,-[src/Clock.tsx:7:22]",
				},
			},
		},
		{
			name: "both formats in one run",
			output: "src/a.ts:1:2: error react(purity): Impure call\n" +
				"  ! react(refs): Cannot access refs during render\n" +
				"   ,-[src/b.tsx:3:4]\n",
			expected: []lintError{
				{
					filePath: "src/a.ts",
					line:     "1",
					column:   "2",
					severity: "error",
					message:  "Impure call",
					rule:     "react/purity",
					fullText: "src/a.ts:1:2: error react(purity): Impure call",
				},
				{
					filePath: "src/b.tsx",
					line:     "3",
					column:   "4",
					severity: "warning",
					message:  "Cannot access refs during render",
					rule:     "react/refs",
					fullText: "   ,-[src/b.tsx:3:4]",
				},
			},
		},
		{
			name:     "output without findings yields nothing",
			output:   "oxlint: using configuration from .oxlintrc.json\n",
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseOxlintErrors(tt.output)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("parseOxlintErrors() = %+v, want %+v", result, tt.expected)
			}
		})
	}
}

// A linter run whose output the parser cannot read must fail the gate rather
// than pass as a clean lint — that silent-zero is what let real errors through.
func TestUnparsedLintOutput(t *testing.T) {
	const finding = "a.ts:1:1: error eslint(no-debugger): Unexpected debugger statement"

	tests := []struct {
		name   string
		errors []lintError
		output string
		want   bool
	}{
		{
			name:   "clean run produces no output",
			errors: nil,
			output: "",
			want:   false,
		},
		{
			name:   "findings parsed normally",
			errors: []lintError{{filePath: "a.ts", line: "1", column: "1"}},
			output: finding,
			want:   false,
		},
		{
			name:   "findings present but none parsed",
			errors: nil,
			output: finding,
			want:   true,
		},
		{
			name:   "eslint positions present but none parsed",
			errors: nil,
			output: "/path/to/file.tsx\n  10:5  error  something  some-rule\n",
			want:   true,
		},
		{
			name:   "notice without positions is not a parse failure",
			errors: nil,
			output: "oxlint: using configuration from .oxlintrc.json\n",
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := unparsedLintOutput(tt.errors, tt.output); got != tt.want {
				t.Errorf("unparsedLintOutput() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestErrUnparsedLintOutputQuotesFirstLine(t *testing.T) {
	err := errUnparsedLintOutput("oxlint", "\n\n  a.ts:1:1: error react(purity): Impure call  \n b.ts:2:2: error x(y): z\n")
	if err == nil {
		t.Fatal("errUnparsedLintOutput() returned nil, want an error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "a.ts:1:1: error react(purity): Impure call") {
		t.Errorf("error should quote the first non-empty line, got: %s", msg)
	}
	if strings.Contains(msg, "b.ts") {
		t.Errorf("error should quote only the first line, got: %s", msg)
	}
	if !strings.Contains(msg, "oxlint") {
		t.Errorf("error should name the linter, got: %s", msg)
	}
}

func TestFirstNonEmptyLine(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "empty string", input: "", want: ""},
		{name: "only blank lines", input: "\n  \n\t\n", want: ""},
		{name: "first line wins", input: "alpha\nbeta\n", want: "alpha"},
		{name: "skips leading blanks and trims", input: "\n\n   padded   \nnext\n", want: "padded"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := firstNonEmptyLine(tt.input); got != tt.want {
				t.Errorf("firstNonEmptyLine(%q) = %q, want %q", tt.input, got, tt.want)
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

func TestResolveNodeBin(t *testing.T) {
	writeBin := func(t *testing.T, dir, tool string) string {
		t.Helper()
		bin := filepath.Join(dir, "node_modules", ".bin")
		if err := os.MkdirAll(bin, 0o755); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(bin, tool)
		if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
		return path
	}

	// No local install anywhere → returns the bare tool name, found=false so the
	// caller skips rather than running a global/fetched copy.
	t.Run("not found when no local install", func(t *testing.T) {
		got, found := resolveNodeBin(t.TempDir(), "tsgo")
		if got != "tsgo" || found {
			t.Errorf("resolveNodeBin() = (%q, %v), want (%q, false)", got, found, "tsgo")
		}
	})

	// Local install in the app dir itself is preferred.
	t.Run("prefers app-local node_modules/.bin", func(t *testing.T) {
		root := t.TempDir()
		want := writeBin(t, root, "oxlint")
		got, found := resolveNodeBin(root, "oxlint")
		if got != want || !found {
			t.Errorf("resolveNodeBin() = (%q, %v), want (%q, true)", got, found, want)
		}
	})

	// A monorepo app with no local install walks up to the workspace-root install.
	t.Run("walks up to a parent node_modules install", func(t *testing.T) {
		root := t.TempDir()
		want := writeBin(t, root, "eslint")
		appPath := filepath.Join(root, "apps", "story")
		if err := os.MkdirAll(appPath, 0o755); err != nil {
			t.Fatal(err)
		}
		got, found := resolveNodeBin(appPath, "eslint")
		if got != want || !found {
			t.Errorf("resolveNodeBin() = (%q, %v), want (%q, true)", got, found, want)
		}
	})

	// A directory (not a file) named like the tool — e.g. a half-written install
	// — must be ignored, not returned as the binary.
	t.Run("ignores a directory named like the tool", func(t *testing.T) {
		root := t.TempDir()
		if err := os.MkdirAll(filepath.Join(root, "node_modules", ".bin", "tsc"), 0o755); err != nil {
			t.Fatal(err)
		}
		got, found := resolveNodeBin(root, "tsc")
		if got != "tsc" || found {
			t.Errorf("resolveNodeBin() = (%q, %v), want (%q, false)", got, found, "tsc")
		}
	})
}
