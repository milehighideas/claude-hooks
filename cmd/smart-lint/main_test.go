package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseHookEvent(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    *HookEvent
		wantErr bool
	}{
		{
			name: "valid PostToolUse Edit event",
			input: `{
				"hook_event_name": "PostToolUse",
				"tool_name": "Edit",
				"tool_input": {"file_path": "/path/to/file.go"},
				"cwd": "/path/to"
			}`,
			want: &HookEvent{
				HookEventName: "PostToolUse",
				ToolName:      "Edit",
				ToolInput:     map[string]interface{}{"file_path": "/path/to/file.go"},
				CWD:           "/path/to",
			},
			wantErr: false,
		},
		{
			name: "valid PostToolUse Write event",
			input: `{
				"hook_event_name": "PostToolUse",
				"tool_name": "Write",
				"tool_input": {"file_path": "/path/to/file.py"},
				"cwd": "/path/to"
			}`,
			want: &HookEvent{
				HookEventName: "PostToolUse",
				ToolName:      "Write",
				ToolInput:     map[string]interface{}{"file_path": "/path/to/file.py"},
				CWD:           "/path/to",
			},
			wantErr: false,
		},
		{
			name:    "invalid JSON",
			input:   `{invalid json}`,
			want:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := strings.NewReader(tt.input)
			got, err := parseHookEvent(reader)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseHookEvent() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if got.HookEventName != tt.want.HookEventName {
					t.Errorf("HookEventName = %v, want %v", got.HookEventName, tt.want.HookEventName)
				}
				if got.ToolName != tt.want.ToolName {
					t.Errorf("ToolName = %v, want %v", got.ToolName, tt.want.ToolName)
				}
			}
		})
	}
}

func TestShouldProcess(t *testing.T) {
	tests := []struct {
		name  string
		event *HookEvent
		want  bool
	}{
		{
			name: "PostToolUse Edit should process",
			event: &HookEvent{
				HookEventName: "PostToolUse",
				ToolName:      "Edit",
			},
			want: true,
		},
		{
			name: "PostToolUse Write should process",
			event: &HookEvent{
				HookEventName: "PostToolUse",
				ToolName:      "Write",
			},
			want: true,
		},
		{
			name: "PostToolUse MultiEdit should process",
			event: &HookEvent{
				HookEventName: "PostToolUse",
				ToolName:      "MultiEdit",
			},
			want: true,
		},
		{
			name: "PreToolUse should not process",
			event: &HookEvent{
				HookEventName: "PreToolUse",
				ToolName:      "Edit",
			},
			want: false,
		},
		{
			name: "PostToolUse Read should not process",
			event: &HookEvent{
				HookEventName: "PostToolUse",
				ToolName:      "Read",
			},
			want: false,
		},
		{
			name: "PostToolUse Bash should not process",
			event: &HookEvent{
				HookEventName: "PostToolUse",
				ToolName:      "Bash",
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldProcess(tt.event); got != tt.want {
				t.Errorf("shouldProcess() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDetectProjectType(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(dir string)
		wantLangs []string
	}{
		{
			name: "Go project with go.mod",
			setup: func(dir string) {
				_ = os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test"), 0644)
			},
			wantLangs: []string{"go"},
		},
		{
			name: "Python project with pyproject.toml",
			setup: func(dir string) {
				_ = os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte("[tool.poetry]"), 0644)
			},
			wantLangs: []string{"python"},
		},
		{
			name: "JavaScript project with package.json",
			setup: func(dir string) {
				_ = os.WriteFile(filepath.Join(dir, "package.json"), []byte("{}"), 0644)
			},
			wantLangs: []string{"javascript"},
		},
		{
			name: "Rust project with Cargo.toml",
			setup: func(dir string) {
				_ = os.WriteFile(filepath.Join(dir, "Cargo.toml"), []byte("[package]"), 0644)
			},
			wantLangs: []string{"rust"},
		},
		{
			name: "Mixed Go and Python project",
			setup: func(dir string) {
				_ = os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test"), 0644)
				_ = os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte("[tool.poetry]"), 0644)
			},
			wantLangs: []string{"go", "python"},
		},
		{
			name: "Unknown project type",
			setup: func(dir string) {
				// No project files
			},
			wantLangs: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp directory
			tmpDir, err := os.MkdirTemp("", "smart-lint-test-*")
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				_ = os.RemoveAll(tmpDir)
			}()

			// Setup test files
			tt.setup(tmpDir)

			// Change to temp directory
			oldDir, _ := os.Getwd()
			_ = os.Chdir(tmpDir)
			defer func() {
				_ = os.Chdir(oldDir)
			}()

			// Run detection
			pt := detectProjectType()

			// Check languages
			if len(pt.Languages) != len(tt.wantLangs) {
				t.Errorf("got %d languages, want %d", len(pt.Languages), len(tt.wantLangs))
			}

			// Check each expected language is present
			for _, wantLang := range tt.wantLangs {
				found := false
				for _, gotLang := range pt.Languages {
					if gotLang == wantLang {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected language %s not found in %v", wantLang, pt.Languages)
				}
			}
		})
	}
}

func TestLoadIgnorePatterns(t *testing.T) {
	tests := []struct {
		name         string
		ignoreFile   string
		wantPatterns []string
	}{
		{
			name: "basic patterns",
			ignoreFile: `# Comment
*.test.go
vendor/**
node_modules

test_*.py`,
			wantPatterns: []string{"*.test.go", "vendor/**", "node_modules", "test_*.py"},
		},
		{
			name:         "empty file",
			ignoreFile:   "",
			wantPatterns: []string{},
		},
		{
			name: "only comments",
			ignoreFile: `# Comment 1
# Comment 2`,
			wantPatterns: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp directory with .git to mark as project root
			tmpDir, err := os.MkdirTemp("", "smart-lint-test-*")
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				_ = os.RemoveAll(tmpDir)
			}()

			// Create .git directory to mark as project root
			_ = os.Mkdir(filepath.Join(tmpDir, ".git"), 0755)

			// Write ignore file
			if tt.ignoreFile != "" {
				err = os.WriteFile(filepath.Join(tmpDir, ".claude-hooks-ignore"), []byte(tt.ignoreFile), 0644)
				if err != nil {
					t.Fatal(err)
				}
			}

			// Change to temp directory
			oldDir, _ := os.Getwd()
			_ = os.Chdir(tmpDir)
			defer func() {
				_ = os.Chdir(oldDir)
			}()

			// Load patterns
			patterns, err := loadIgnorePatterns()
			if err != nil {
				t.Errorf("loadIgnorePatterns() error = %v", err)
			}

			// Check patterns
			if len(patterns) != len(tt.wantPatterns) {
				t.Errorf("got %d patterns, want %d", len(patterns), len(tt.wantPatterns))
			}

			for i, want := range tt.wantPatterns {
				if i >= len(patterns) {
					break
				}
				if patterns[i] != want {
					t.Errorf("pattern[%d] = %s, want %s", i, patterns[i], want)
				}
			}
		})
	}
}

func TestShouldSkipFile(t *testing.T) {
	tests := []struct {
		name     string
		filePath string
		patterns []string
		want     bool
	}{
		{
			name:     "exact match basename",
			filePath: "path/to/file.txt",
			patterns: []string{"file.txt"},
			want:     true,
		},
		{
			name:     "exact match full path",
			filePath: "path/to/file.txt",
			patterns: []string{"path/to/file.txt"},
			want:     true,
		},
		{
			name:     "glob pattern match",
			filePath: "test_helper.go",
			patterns: []string{"test_*.go"},
			want:     true,
		},
		{
			name:     "directory pattern match",
			filePath: "vendor/lib/file.go",
			patterns: []string{"vendor/**"},
			want:     true,
		},
		{
			name:     "no match",
			filePath: "src/main.go",
			patterns: []string{"*.test.go", "vendor/**"},
			want:     false,
		},
		{
			name:     "wildcard extension match",
			filePath: "file.test.go",
			patterns: []string{"*.test.go"},
			want:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldSkipFile(tt.filePath, tt.patterns); got != tt.want {
				t.Errorf("shouldSkipFile() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFindProjectRoot(t *testing.T) {
	// Create temp directory structure
	tmpDir, err := os.MkdirTemp("", "smart-lint-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	// Create nested directories
	subDir := filepath.Join(tmpDir, "src", "pkg")
	_ = os.MkdirAll(subDir, 0755)

	// Create go.mod at root
	_ = os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte("module test"), 0644)

	// Change to subdirectory
	oldDir, _ := os.Getwd()
	_ = os.Chdir(subDir)
	defer func() {
		_ = os.Chdir(oldDir)
	}()

	// Find project root
	root, err := findProjectRoot()
	if err != nil {
		t.Errorf("findProjectRoot() error = %v", err)
	}

	// Use EvalSymlinks to handle /var vs /private/var on macOS
	rootEvaled, _ := filepath.EvalSymlinks(root)
	tmpDirEvaled, _ := filepath.EvalSymlinks(tmpDir)

	if rootEvaled != tmpDirEvaled {
		t.Errorf("findProjectRoot() = %v, want %v", rootEvaled, tmpDirEvaled)
	}
}

func TestCommandExists(t *testing.T) {
	tests := []struct {
		name    string
		command string
		want    bool
	}{
		{
			name:    "ls should exist",
			command: "ls",
			want:    true,
		},
		{
			name:    "nonexistent command",
			command: "this-command-does-not-exist-12345",
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := commandExists(tt.command); got != tt.want {
				t.Errorf("commandExists(%s) = %v, want %v", tt.command, got, tt.want)
			}
		})
	}
}

func TestErrorCollector(t *testing.T) {
	ec := &ErrorCollector{}

	if ec.Count() != 0 {
		t.Errorf("initial count = %d, want 0", ec.Count())
	}

	ec.Add("error 1")
	if ec.Count() != 1 {
		t.Errorf("count after one error = %d, want 1", ec.Count())
	}

	ec.Add("error 2")
	if ec.Count() != 2 {
		t.Errorf("count after two errors = %d, want 2", ec.Count())
	}

	if len(ec.errors) != 2 {
		t.Errorf("errors slice length = %d, want 2", len(ec.errors))
	}
}

func TestIntegrationWithValidJSON(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "smart-lint-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	// Create a test Go file
	testFile := filepath.Join(tmpDir, "test.go")
	_ = os.WriteFile(testFile, []byte("package main\n"), 0644)

	// Create go.mod
	_ = os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte("module test"), 0644)

	// Create hook event JSON
	event := HookEvent{
		HookEventName: "PostToolUse",
		ToolName:      "Edit",
		ToolInput: map[string]interface{}{
			"file_path": testFile,
		},
		CWD: tmpDir,
	}

	jsonData, _ := json.Marshal(event)
	reader := strings.NewReader(string(jsonData))

	// Parse event
	parsed, err := parseHookEvent(reader)
	if err != nil {
		t.Errorf("parseHookEvent() error = %v", err)
	}

	// Should process
	if !shouldProcess(parsed) {
		t.Error("shouldProcess() = false, want true")
	}

	// Change to temp directory and detect project
	oldDir, _ := os.Getwd()
	_ = os.Chdir(tmpDir)
	defer func() {
		_ = os.Chdir(oldDir)
	}()

	pt := detectProjectType()
	if len(pt.Languages) != 1 || pt.Languages[0] != "go" {
		t.Errorf("detected languages = %v, want [go]", pt.Languages)
	}
}

func TestFindFiles(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "smart-lint-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	// Create test files
	_ = os.WriteFile(filepath.Join(tmpDir, "main.go"), []byte("package main"), 0644)
	_ = os.WriteFile(filepath.Join(tmpDir, "helper.go"), []byte("package main"), 0644)
	_ = os.WriteFile(filepath.Join(tmpDir, "test.txt"), []byte("text"), 0644)

	// Create subdirectory with more files
	subDir := filepath.Join(tmpDir, "pkg")
	_ = os.MkdirAll(subDir, 0755)
	_ = os.WriteFile(filepath.Join(subDir, "lib.go"), []byte("package pkg"), 0644)

	// Change to temp directory
	oldDir, _ := os.Getwd()
	_ = os.Chdir(tmpDir)
	defer func() {
		_ = os.Chdir(oldDir)
	}()

	// Find .go files
	files := findFiles([]string{".go"}, []string{})

	if len(files) != 3 {
		t.Errorf("found %d files, want 3", len(files))
	}

	// Verify all found files are .go files
	for _, file := range files {
		if filepath.Ext(file) != ".go" {
			t.Errorf("found non-.go file: %s", file)
		}
	}
}

func TestFindFilesWithIgnore(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "smart-lint-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	// Create test files
	_ = os.WriteFile(filepath.Join(tmpDir, "main.go"), []byte("package main"), 0644)
	_ = os.WriteFile(filepath.Join(tmpDir, "test.go"), []byte("package main"), 0644)

	// Change to temp directory
	oldDir, _ := os.Getwd()
	_ = os.Chdir(tmpDir)
	defer func() {
		_ = os.Chdir(oldDir)
	}()

	// Find .go files with ignore pattern
	files := findFiles([]string{".go"}, []string{"test.go"})

	if len(files) != 1 {
		t.Errorf("found %d files, want 1", len(files))
	}

	if len(files) > 0 && filepath.Base(files[0]) != "main.go" {
		t.Errorf("found file = %s, want main.go", files[0])
	}
}
