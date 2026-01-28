package main

import (
	"errors"
	"testing"
)

func TestConsoleChecker(t *testing.T) {
	tests := []struct {
		name         string
		files        []string
		allowedFiles []string
		fileContents map[string]string
		wantErr      bool
	}{
		{
			name:         "detects console.log",
			files:        []string{"src/app.ts"},
			allowedFiles: nil,
			fileContents: map[string]string{
				"src/app.ts": `function test() { console.log("debug"); }`,
			},
			wantErr: true,
		},
		{
			name:         "detects console.warn",
			files:        []string{"src/utils.tsx"},
			allowedFiles: nil,
			fileContents: map[string]string{
				"src/utils.tsx": `console.warn("warning message");`,
			},
			wantErr: true,
		},
		{
			name:         "detects console.error",
			files:        []string{"src/handler.js"},
			allowedFiles: nil,
			fileContents: map[string]string{
				"src/handler.js": `console.error("error occurred");`,
			},
			wantErr: true,
		},
		{
			name:         "detects console.info",
			files:        []string{"src/service.jsx"},
			allowedFiles: nil,
			fileContents: map[string]string{
				"src/service.jsx": `console.info("info message");`,
			},
			wantErr: true,
		},
		{
			name:         "detects console.debug",
			files:        []string{"src/debug.ts"},
			allowedFiles: nil,
			fileContents: map[string]string{
				"src/debug.ts": `console.debug("debug info");`,
			},
			wantErr: true,
		},
		{
			name:         "allowed files are skipped",
			files:        []string{"src/logger.ts", "src/app.ts"},
			allowedFiles: []string{"src/logger.ts"},
			fileContents: map[string]string{
				"src/logger.ts": `console.log("this is allowed");`,
				"src/app.ts":    `function clean() { return 42; }`,
			},
			wantErr: false,
		},
		{
			name:         "non-JS/TS files are skipped",
			files:        []string{"config.json", "README.md", "styles.css", "data.yaml"},
			allowedFiles: nil,
			fileContents: map[string]string{
				"config.json": `{"debug": "console.log"}`,
				"README.md":   `Use console.log for debugging`,
				"styles.css":  `.console { log: none; }`,
				"data.yaml":   `console: log`,
			},
			wantErr: false,
		},
		{
			name:         "clean files pass",
			files:        []string{"src/clean.ts", "src/pure.tsx"},
			allowedFiles: nil,
			fileContents: map[string]string{
				"src/clean.ts":  `export function add(a: number, b: number) { return a + b; }`,
				"src/pure.tsx":  `export const Component = () => <div>Hello</div>;`,
			},
			wantErr: false,
		},
		{
			name:         "multiple violations detected",
			files:        []string{"src/a.ts", "src/b.tsx", "src/c.js"},
			allowedFiles: nil,
			fileContents: map[string]string{
				"src/a.ts":  `console.log("a");`,
				"src/b.tsx": `console.warn("b");`,
				"src/c.js":  `console.error("c");`,
			},
			wantErr: true,
		},
		{
			name:         "mixed allowed and violations",
			files:        []string{"src/allowed.ts", "src/violation.ts"},
			allowedFiles: []string{"src/allowed.ts"},
			fileContents: map[string]string{
				"src/allowed.ts":   `console.log("allowed");`,
				"src/violation.ts": `console.log("not allowed");`,
			},
			wantErr: true,
		},
		{
			name:         "empty file list",
			files:        []string{},
			allowedFiles: nil,
			fileContents: map[string]string{},
			wantErr:      false,
		},
		{
			name:         "console in string literal not matched without parens",
			files:        []string{"src/test.ts"},
			allowedFiles: nil,
			fileContents: map[string]string{
				"src/test.ts": `const msg = "console.log is a method";`,
			},
			wantErr: false,
		},
		{
			name:         "console.log with parentheses in string is detected",
			files:        []string{"src/test.ts"},
			allowedFiles: nil,
			fileContents: map[string]string{
				"src/test.ts": `const msg = "console.log(x)";`,
			},
			wantErr: true,
		},
		{
			name:         "all file extensions checked",
			files:        []string{"a.ts", "b.tsx", "c.js", "d.jsx"},
			allowedFiles: nil,
			fileContents: map[string]string{
				"a.ts":  `console.log("ts");`,
				"b.tsx": `console.log("tsx");`,
				"c.js":  `console.log("js");`,
				"d.jsx": `console.log("jsx");`,
			},
			wantErr: true,
		},
		{
			name:         "git show error is handled gracefully",
			files:        []string{"missing.ts"},
			allowedFiles: nil,
			fileContents: map[string]string{}, // File not in map simulates git show error
			wantErr:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checker := &ConsoleChecker{
				gitShowFunc: func(file string) ([]byte, error) {
					content, ok := tt.fileContents[file]
					if !ok {
						return nil, errors.New("file not found")
					}
					return []byte(content), nil
				},
			}

			err := checker.Check("test", tt.files, tt.allowedFiles)
			if (err != nil) != tt.wantErr {
				t.Errorf("Check() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestIsCheckableFile(t *testing.T) {
	checker := NewConsoleChecker()

	tests := []struct {
		file string
		want bool
	}{
		{"src/app.ts", true},
		{"src/component.tsx", true},
		{"src/util.js", true},
		{"src/widget.jsx", true},
		{"config.json", false},
		{"README.md", false},
		{"styles.css", false},
		{"Makefile", false},
		{"script.sh", false},
		{".ts", true},  // Edge case: just extension
		{"", false},    // Edge case: empty string
		{"file.typescript", false}, // Not a valid extension
		{"file.ts.bak", false}, // Backup file
	}

	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			got := checker.isCheckableFile(tt.file)
			if got != tt.want {
				t.Errorf("isCheckableFile(%q) = %v, want %v", tt.file, got, tt.want)
			}
		})
	}
}

func TestIsAllowedFile(t *testing.T) {
	checker := NewConsoleChecker()

	tests := []struct {
		name         string
		file         string
		allowedFiles []string
		want         bool
	}{
		{
			name:         "file in allowed list",
			file:         "src/logger.ts",
			allowedFiles: []string{"src/logger.ts", "src/debug.ts"},
			want:         true,
		},
		{
			name:         "file not in allowed list",
			file:         "src/app.ts",
			allowedFiles: []string{"src/logger.ts"},
			want:         false,
		},
		{
			name:         "empty allowed list",
			file:         "src/app.ts",
			allowedFiles: []string{},
			want:         false,
		},
		{
			name:         "nil allowed list",
			file:         "src/app.ts",
			allowedFiles: nil,
			want:         false,
		},
		{
			name:         "substring pattern matches",
			file:         "src/logger.ts",
			allowedFiles: []string{"logger.ts"},
			want:         true,
		},
		{
			name:         "directory pattern matches",
			file:         "scripts/build.ts",
			allowedFiles: []string{"scripts/"},
			want:         true,
		},
		{
			name:         "directory pattern matches nested",
			file:         "apps/native/scripts/test.ts",
			allowedFiles: []string{"scripts/"},
			want:         true,
		},
		{
			name:         "demo pattern matches",
			file:         "src/features/demo/index.tsx",
			allowedFiles: []string{"/demo/"},
			want:         true,
		},
		{
			name:         "demo file pattern matches",
			file:         "src/components/button.demo.tsx",
			allowedFiles: []string{".demo."},
			want:         true,
		},
		{
			name:         "pattern does not match different path",
			file:         "src/app.ts",
			allowedFiles: []string{"scripts/", "/demo/"},
			want:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := checker.isAllowedFile(tt.file, tt.allowedFiles)
			if got != tt.want {
				t.Errorf("isAllowedFile(%q, %v) = %v, want %v", tt.file, tt.allowedFiles, got, tt.want)
			}
		})
	}
}

func TestHasConsoleStatements(t *testing.T) {
	checker := NewConsoleChecker()

	tests := []struct {
		name    string
		content string
		want    bool
	}{
		{"console.log", `console.log("test")`, true},
		{"console.warn", `console.warn("warning")`, true},
		{"console.error", `console.error("error")`, true},
		{"console.info", `console.info("info")`, true},
		{"console.debug", `console.debug("debug")`, true},
		{"no console", `function test() { return 42; }`, false},
		{"console without parens", `const x = console.log;`, false},
		{"console.trace not matched", `console.trace("trace")`, false},
		{"console.table not matched", `console.table(data)`, false},
		{"multiline with console", "line1\nconsole.log(x)\nline3", true},
		{"empty content", "", false},
		{"console in comment style", "// console.log(x)", true}, // Pattern matches in comments too
		{"nested console", `obj.console.log("not real")`, true}, // Pattern matches this too
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := checker.hasConsoleStatements([]byte(tt.content))
			if got != tt.want {
				t.Errorf("hasConsoleStatements(%q) = %v, want %v", tt.content, got, tt.want)
			}
		})
	}
}
