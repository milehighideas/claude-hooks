package main

import (
	"reflect"
	"testing"
)

func TestParseTypeScriptErrors(t *testing.T) {
	tests := []struct {
		name     string
		output   string
		expected []tsError
	}{
		{
			name:     "empty output",
			output:   "",
			expected: nil,
		},
		{
			name:     "no errors",
			output:   "Compilation complete.\n",
			expected: nil,
		},
		{
			name:   "single line error",
			output: "src/index.ts(10,5): error TS2322: Type 'string' is not assignable to type 'number'.\n",
			expected: []tsError{
				{
					filePath:  "src/index.ts",
					errorCode: "TS2322",
					fullText:  "src/index.ts(10,5): error TS2322: Type 'string' is not assignable to type 'number'.",
				},
			},
		},
		{
			name: "multiple single line errors",
			output: `src/foo.ts(1,1): error TS2589: Type instantiation is excessively deep and possibly infinite.
src/bar.tsx(25,10): error TS2742: The inferred type cannot be named without a reference.
`,
			expected: []tsError{
				{
					filePath:  "src/foo.ts",
					errorCode: "TS2589",
					fullText:  "src/foo.ts(1,1): error TS2589: Type instantiation is excessively deep and possibly infinite.",
				},
				{
					filePath:  "src/bar.tsx",
					errorCode: "TS2742",
					fullText:  "src/bar.tsx(25,10): error TS2742: The inferred type cannot be named without a reference.",
				},
			},
		},
		{
			name: "multi-line error with space indentation",
			output: `src/component.tsx(15,3): error TS2322: Type '{ name: string; }' is not assignable to type 'Props'.
  Property 'id' is missing in type '{ name: string; }' but required in type 'Props'.
`,
			expected: []tsError{
				{
					filePath:  "src/component.tsx",
					errorCode: "TS2322",
					fullText: `src/component.tsx(15,3): error TS2322: Type '{ name: string; }' is not assignable to type 'Props'.
  Property 'id' is missing in type '{ name: string; }' but required in type 'Props'.`,
				},
			},
		},
		{
			name: "multi-line error with tab indentation",
			output: "src/utils.ts(50,12): error TS2345: Argument of type 'string' is not assignable.\n\tExpected type 'number'.\n",
			expected: []tsError{
				{
					filePath:  "src/utils.ts",
					errorCode: "TS2345",
					fullText:  "src/utils.ts(50,12): error TS2345: Argument of type 'string' is not assignable.\n\tExpected type 'number'.",
				},
			},
		},
		{
			name: "multi-line error with multiple continuation lines",
			output: `packages/backend/src/api.ts(100,1): error TS2322: Type 'A' is not assignable to type 'B'.
  Type 'A' is missing the following properties from type 'B':
    property1
    property2
    property3
`,
			expected: []tsError{
				{
					filePath:  "packages/backend/src/api.ts",
					errorCode: "TS2322",
					fullText: `packages/backend/src/api.ts(100,1): error TS2322: Type 'A' is not assignable to type 'B'.
  Type 'A' is missing the following properties from type 'B':
    property1
    property2
    property3`,
				},
			},
		},
		{
			name: "mixed single and multi-line errors",
			output: `src/a.ts(1,1): error TS2589: Type too deep.
src/b.ts(2,2): error TS2322: Type mismatch.
  Expected: string
  Got: number
src/c.ts(3,3): error TS2742: Cannot infer.
`,
			expected: []tsError{
				{
					filePath:  "src/a.ts",
					errorCode: "TS2589",
					fullText:  "src/a.ts(1,1): error TS2589: Type too deep.",
				},
				{
					filePath:  "src/b.ts",
					errorCode: "TS2322",
					fullText:  "src/b.ts(2,2): error TS2322: Type mismatch.\n  Expected: string\n  Got: number",
				},
				{
					filePath:  "src/c.ts",
					errorCode: "TS2742",
					fullText:  "src/c.ts(3,3): error TS2742: Cannot infer.",
				},
			},
		},
		{
			name: "error with path containing parentheses in filename",
			output: "src/components/Button(old).tsx(5,10): error TS2304: Cannot find name 'Props'.\n",
			expected: []tsError{
				{
					filePath:  "src/components/Button(old).tsx",
					errorCode: "TS2304",
					fullText:  "src/components/Button(old).tsx(5,10): error TS2304: Cannot find name 'Props'.",
				},
			},
		},
		{
			name: "error with Windows-style path",
			output: "C:\\Users\\dev\\project\\src\\index.ts(10,5): error TS2322: Type error.\n",
			expected: []tsError{
				{
					filePath:  "C:\\Users\\dev\\project\\src\\index.ts",
					errorCode: "TS2322",
					fullText:  "C:\\Users\\dev\\project\\src\\index.ts(10,5): error TS2322: Type error.",
				},
			},
		},
		{
			name: "output with non-error lines interspersed",
			output: `Starting compilation...
src/index.ts(1,1): error TS2322: Type error.
Found 1 error.
`,
			expected: []tsError{
				{
					filePath:  "src/index.ts",
					errorCode: "TS2322",
					fullText:  "src/index.ts(1,1): error TS2322: Type error.",
				},
			},
		},
		{
			name: "error in test file path",
			output: "src/__tests__/utils.test.ts(10,5): error TS2322: Type error.\n",
			expected: []tsError{
				{
					filePath:  "src/__tests__/utils.test.ts",
					errorCode: "TS2322",
					fullText:  "src/__tests__/utils.test.ts(10,5): error TS2322: Type error.",
				},
			},
		},
		{
			name: "error with high line and column numbers",
			output: "src/large-file.ts(9999,150): error TS2551: Property 'foo' does not exist.\n",
			expected: []tsError{
				{
					filePath:  "src/large-file.ts",
					errorCode: "TS2551",
					fullText:  "src/large-file.ts(9999,150): error TS2551: Property 'foo' does not exist.",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseTypeScriptErrors(tt.output)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("parseTypeScriptErrors() = %+v, want %+v", result, tt.expected)
			}
		})
	}
}

func TestShouldFilterError(t *testing.T) {
	tests := []struct {
		name         string
		err          tsError
		errorCodes   []string
		excludePaths []string
		expected     bool
	}{
		{
			name: "filter TS2589 error",
			err: tsError{
				filePath:  "src/index.ts",
				errorCode: "TS2589",
				fullText:  "src/index.ts(1,1): error TS2589: Type too deep.",
			},
			errorCodes:   []string{"TS2589", "TS2742"},
			excludePaths: []string{},
			expected:     true,
		},
		{
			name: "filter TS2742 error",
			err: tsError{
				filePath:  "packages/backend/api.ts",
				errorCode: "TS2742",
				fullText:  "packages/backend/api.ts(10,5): error TS2742: Cannot infer.",
			},
			errorCodes:   []string{"TS2589", "TS2742"},
			excludePaths: []string{},
			expected:     true,
		},
		{
			name: "do not filter TS2322 error",
			err: tsError{
				filePath:  "src/component.tsx",
				errorCode: "TS2322",
				fullText:  "src/component.tsx(15,3): error TS2322: Type mismatch.",
			},
			errorCodes:   []string{"TS2589", "TS2742"},
			excludePaths: []string{},
			expected:     false,
		},
		{
			name: "filter error from __tests__ directory",
			err: tsError{
				filePath:  "src/__tests__/utils.test.ts",
				errorCode: "TS2322",
				fullText:  "src/__tests__/utils.test.ts(10,5): error TS2322: Type error.",
			},
			errorCodes:   []string{},
			excludePaths: []string{"__tests__/"},
			expected:     true,
		},
		{
			name: "filter error from .test. file",
			err: tsError{
				filePath:  "src/utils.test.ts",
				errorCode: "TS2322",
				fullText:  "src/utils.test.ts(10,5): error TS2322: Type error.",
			},
			errorCodes:   []string{},
			excludePaths: []string{".test."},
			expected:     true,
		},
		{
			name: "filter error from .spec. file",
			err: tsError{
				filePath:  "src/component.spec.tsx",
				errorCode: "TS2345",
				fullText:  "src/component.spec.tsx(5,1): error TS2345: Argument error.",
			},
			errorCodes:   []string{},
			excludePaths: []string{".spec."},
			expected:     true,
		},
		{
			name: "do not filter non-test file with excluded error code absent",
			err: tsError{
				filePath:  "src/api/handler.ts",
				errorCode: "TS2304",
				fullText:  "src/api/handler.ts(50,12): error TS2304: Cannot find name.",
			},
			errorCodes:   []string{"TS2589", "TS2742"},
			excludePaths: []string{"__tests__/", ".test.", ".spec."},
			expected:     false,
		},
		{
			name: "empty filter lists do not filter anything",
			err: tsError{
				filePath:  "src/index.ts",
				errorCode: "TS2589",
				fullText:  "src/index.ts(1,1): error TS2589: Type too deep.",
			},
			errorCodes:   []string{},
			excludePaths: []string{},
			expected:     false,
		},
		{
			name: "filter by error code takes priority over path",
			err: tsError{
				filePath:  "src/production-code.ts",
				errorCode: "TS2589",
				fullText:  "src/production-code.ts(1,1): error TS2589: Type too deep.",
			},
			errorCodes:   []string{"TS2589"},
			excludePaths: []string{"__tests__/"},
			expected:     true,
		},
		{
			name: "path pattern matching is substring based",
			err: tsError{
				filePath:  "packages/backend/__tests__/api.test.ts",
				errorCode: "TS2322",
				fullText:  "packages/backend/__tests__/api.test.ts(1,1): error TS2322: Type error.",
			},
			errorCodes:   []string{},
			excludePaths: []string{"__tests__/"},
			expected:     true,
		},
		{
			name: "case sensitive error code matching",
			err: tsError{
				filePath:  "src/index.ts",
				errorCode: "TS2589",
				fullText:  "src/index.ts(1,1): error TS2589: Type too deep.",
			},
			errorCodes:   []string{"ts2589"},
			excludePaths: []string{},
			expected:     false,
		},
		{
			name: "case sensitive path matching",
			err: tsError{
				filePath:  "src/__Tests__/utils.ts",
				errorCode: "TS2322",
				fullText:  "src/__Tests__/utils.ts(1,1): error TS2322: Type error.",
			},
			errorCodes:   []string{},
			excludePaths: []string{"__tests__/"},
			expected:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := shouldFilterError(tt.err, tt.errorCodes, tt.excludePaths)
			if result != tt.expected {
				t.Errorf("shouldFilterError() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestDefaultErrorCodes(t *testing.T) {
	expected := []string{"TS2589", "TS2742"}
	if !reflect.DeepEqual(DefaultErrorCodes, expected) {
		t.Errorf("DefaultErrorCodes = %v, want %v", DefaultErrorCodes, expected)
	}
}

func TestDefaultExcludePaths(t *testing.T) {
	expected := []string{"__tests__/", ".test.", ".spec."}
	if !reflect.DeepEqual(DefaultExcludePaths, expected) {
		t.Errorf("DefaultExcludePaths = %v, want %v", DefaultExcludePaths, expected)
	}
}

func TestShouldFilterErrorWithDefaults(t *testing.T) {
	// Test that default values work correctly when applied
	tests := []struct {
		name     string
		err      tsError
		expected bool
	}{
		{
			name: "default filters TS2589",
			err: tsError{
				filePath:  "src/index.ts",
				errorCode: "TS2589",
				fullText:  "src/index.ts(1,1): error TS2589: Type too deep.",
			},
			expected: true,
		},
		{
			name: "default filters TS2742",
			err: tsError{
				filePath:  "src/api.ts",
				errorCode: "TS2742",
				fullText:  "src/api.ts(1,1): error TS2742: Cannot infer.",
			},
			expected: true,
		},
		{
			name: "default filters __tests__ path",
			err: tsError{
				filePath:  "src/__tests__/foo.ts",
				errorCode: "TS2322",
				fullText:  "src/__tests__/foo.ts(1,1): error TS2322: Type error.",
			},
			expected: true,
		},
		{
			name: "default filters .test. path",
			err: tsError{
				filePath:  "src/utils.test.ts",
				errorCode: "TS2322",
				fullText:  "src/utils.test.ts(1,1): error TS2322: Type error.",
			},
			expected: true,
		},
		{
			name: "default filters .spec. path",
			err: tsError{
				filePath:  "src/component.spec.tsx",
				errorCode: "TS2322",
				fullText:  "src/component.spec.tsx(1,1): error TS2322: Type error.",
			},
			expected: true,
		},
		{
			name: "default does not filter regular production error",
			err: tsError{
				filePath:  "src/component.tsx",
				errorCode: "TS2322",
				fullText:  "src/component.tsx(1,1): error TS2322: Type error.",
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := shouldFilterError(tt.err, DefaultErrorCodes, DefaultExcludePaths)
			if result != tt.expected {
				t.Errorf("shouldFilterError() with defaults = %v, want %v", result, tt.expected)
			}
		})
	}
}
