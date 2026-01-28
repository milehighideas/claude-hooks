package main

import (
	"bytes"
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
			name: "valid edit event",
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
			name:    "invalid json",
			input:   `{invalid}`,
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
					t.Errorf("parseHookEvent() HookEventName = %v, want %v", got.HookEventName, tt.want.HookEventName)
				}
				if got.ToolName != tt.want.ToolName {
					t.Errorf("parseHookEvent() ToolName = %v, want %v", got.ToolName, tt.want.ToolName)
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
			name: "edit on PostToolUse",
			event: &HookEvent{
				HookEventName: "PostToolUse",
				ToolName:      "Edit",
			},
			want: true,
		},
		{
			name: "write on PostToolUse",
			event: &HookEvent{
				HookEventName: "PostToolUse",
				ToolName:      "Write",
			},
			want: true,
		},
		{
			name: "multiedit on PostToolUse",
			event: &HookEvent{
				HookEventName: "PostToolUse",
				ToolName:      "MultiEdit",
			},
			want: true,
		},
		{
			name: "read on PostToolUse",
			event: &HookEvent{
				HookEventName: "PostToolUse",
				ToolName:      "Read",
			},
			want: false,
		},
		{
			name: "edit on PreToolUse",
			event: &HookEvent{
				HookEventName: "PreToolUse",
				ToolName:      "Edit",
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
		setupFunc func(string)
		wantLangs []string
	}{
		{
			name: "go project with go.mod",
			setupFunc: func(dir string) {
				_ = os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test"), 0644)
			},
			wantLangs: []string{"go"},
		},
		{
			name: "python project with setup.py",
			setupFunc: func(dir string) {
				_ = os.WriteFile(filepath.Join(dir, "setup.py"), []byte(""), 0644)
			},
			wantLangs: []string{"python"},
		},
		{
			name: "javascript project with package.json",
			setupFunc: func(dir string) {
				_ = os.WriteFile(filepath.Join(dir, "package.json"), []byte("{}"), 0644)
			},
			wantLangs: []string{"javascript"},
		},
		{
			name: "rust project with Cargo.toml",
			setupFunc: func(dir string) {
				_ = os.WriteFile(filepath.Join(dir, "Cargo.toml"), []byte(""), 0644)
			},
			wantLangs: []string{"rust"},
		},
		{
			name: "multi-language project",
			setupFunc: func(dir string) {
				_ = os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test"), 0644)
				_ = os.WriteFile(filepath.Join(dir, "package.json"), []byte("{}"), 0644)
			},
			wantLangs: []string{"go", "javascript"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp directory
			tmpDir, err := os.MkdirTemp("", "smart-test-*")
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				_ = os.RemoveAll(tmpDir)
			}()

			// Setup test files
			tt.setupFunc(tmpDir)

			// Change to temp directory
			oldDir, _ := os.Getwd()
			_ = os.Chdir(tmpDir)
			defer func() {
				_ = os.Chdir(oldDir)
			}()

			// Detect project type
			pt := detectProjectType()

			// Check results
			if len(pt.Languages) != len(tt.wantLangs) {
				t.Errorf("detectProjectType() returned %d languages, want %d", len(pt.Languages), len(tt.wantLangs))
			}

			langMap := make(map[string]bool)
			for _, lang := range pt.Languages {
				langMap[lang] = true
			}

			for _, want := range tt.wantLangs {
				if !langMap[want] {
					t.Errorf("detectProjectType() missing language %q", want)
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
			name:     "exact match",
			filePath: "test.go",
			patterns: []string{"test.go"},
			want:     true,
		},
		{
			name:     "glob pattern",
			filePath: "test_generated.go",
			patterns: []string{"*_generated.go"},
			want:     true,
		},
		{
			name:     "directory pattern",
			filePath: "vendor/pkg/file.go",
			patterns: []string{"vendor/**"},
			want:     true,
		},
		{
			name:     "no match",
			filePath: "main.go",
			patterns: []string{"test.go", "*.txt"},
			want:     false,
		},
		{
			name:     "basename match",
			filePath: "path/to/test.go",
			patterns: []string{"test.go"},
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

func TestLoadIgnorePatterns(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "smart-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	// Create .git directory to mark as project root
	gitDir := filepath.Join(tmpDir, ".git")
	if err := os.Mkdir(gitDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create ignore file
	ignoreContent := `# Comment
*_generated.go
vendor/**

*.tmp
`
	ignorePath := filepath.Join(tmpDir, ".claude-hooks-ignore")
	if err := os.WriteFile(ignorePath, []byte(ignoreContent), 0644); err != nil {
		t.Fatal(err)
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
	want := []string{"*_generated.go", "vendor/**", "*.tmp"}
	if len(patterns) != len(want) {
		t.Errorf("loadIgnorePatterns() returned %d patterns, want %d", len(patterns), len(want))
	}

	for i, p := range patterns {
		if p != want[i] {
			t.Errorf("loadIgnorePatterns() pattern[%d] = %q, want %q", i, p, want[i])
		}
	}
}

func TestIsTestingEnabled(t *testing.T) {
	tests := []struct {
		name    string
		envVal  string
		wantSet bool
		want    bool
	}{
		{
			name:    "default (not set)",
			wantSet: false,
			want:    true,
		},
		{
			name:    "explicitly enabled",
			envVal:  "true",
			wantSet: true,
			want:    true,
		},
		{
			name:    "explicitly disabled",
			envVal:  "false",
			wantSet: true,
			want:    false,
		},
		{
			name:    "enabled with 1",
			envVal:  "1",
			wantSet: true,
			want:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save original env
			origVal, origSet := os.LookupEnv("CLAUDE_HOOKS_TEST_ON_EDIT")
			defer func() {
				if origSet {
					_ = os.Setenv("CLAUDE_HOOKS_TEST_ON_EDIT", origVal)
				} else {
					_ = os.Unsetenv("CLAUDE_HOOKS_TEST_ON_EDIT")
				}
			}()

			// Set test env
			if tt.wantSet {
				_ = os.Setenv("CLAUDE_HOOKS_TEST_ON_EDIT", tt.envVal)
			} else {
				_ = os.Unsetenv("CLAUDE_HOOKS_TEST_ON_EDIT")
			}

			if got := isTestingEnabled(); got != tt.want {
				t.Errorf("isTestingEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsRaceEnabled(t *testing.T) {
	tests := []struct {
		name    string
		envVal  string
		wantSet bool
		want    bool
	}{
		{
			name:    "default (not set)",
			wantSet: false,
			want:    true,
		},
		{
			name:    "explicitly enabled",
			envVal:  "true",
			wantSet: true,
			want:    true,
		},
		{
			name:    "explicitly disabled",
			envVal:  "false",
			wantSet: true,
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save original env
			origVal, origSet := os.LookupEnv("CLAUDE_HOOKS_ENABLE_RACE")
			defer func() {
				if origSet {
					_ = os.Setenv("CLAUDE_HOOKS_ENABLE_RACE", origVal)
				} else {
					_ = os.Unsetenv("CLAUDE_HOOKS_ENABLE_RACE")
				}
			}()

			// Set test env
			if tt.wantSet {
				_ = os.Setenv("CLAUDE_HOOKS_ENABLE_RACE", tt.envVal)
			} else {
				_ = os.Unsetenv("CLAUDE_HOOKS_ENABLE_RACE")
			}

			if got := isRaceEnabled(); got != tt.want {
				t.Errorf("isRaceEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestErrorCollector(t *testing.T) {
	// Capture stderr
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w
	defer func() {
		os.Stderr = oldStderr
	}()

	ec := &ErrorCollector{}

	// Add errors
	ec.Add("error 1")
	ec.Add("error 2")

	// Check count
	if ec.Count() != 2 {
		t.Errorf("ErrorCollector.Count() = %d, want 2", ec.Count())
	}

	// Close writer and read stderr
	_ = w.Close()
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	output := buf.String()

	// Check stderr output
	if !strings.Contains(output, "error 1") {
		t.Error("ErrorCollector did not write error 1 to stderr")
	}
	if !strings.Contains(output, "error 2") {
		t.Error("ErrorCollector did not write error 2 to stderr")
	}
}

func TestFindProjectRoot(t *testing.T) {
	// Create temp directory structure
	tmpDir, err := os.MkdirTemp("", "smart-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	// Create .git directory to mark as project root
	gitDir := filepath.Join(tmpDir, ".git")
	if err := os.Mkdir(gitDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create nested directory
	nestedDir := filepath.Join(tmpDir, "src", "pkg")
	if err := os.MkdirAll(nestedDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Change to nested directory
	oldDir, _ := os.Getwd()
	_ = os.Chdir(nestedDir)
	defer func() {
		_ = os.Chdir(oldDir)
	}()

	// Find project root
	root, err := findProjectRoot()
	if err != nil {
		t.Errorf("findProjectRoot() error = %v", err)
	}

	// Resolve symlinks for comparison (macOS uses /private/var/folders)
	resolvedRoot, _ := filepath.EvalSymlinks(root)
	resolvedTmpDir, _ := filepath.EvalSymlinks(tmpDir)

	if resolvedRoot != resolvedTmpDir {
		t.Errorf("findProjectRoot() = %q, want %q", root, tmpDir)
	}
}

// Integration test to verify the hook event processing
func TestHookEventProcessing(t *testing.T) {
	event := &HookEvent{
		HookEventName: "PostToolUse",
		ToolName:      "Edit",
		ToolInput: map[string]interface{}{
			"file_path": "/path/to/file.go",
		},
		CWD: "/path/to",
	}

	// Marshal to JSON
	data, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}

	// Parse back
	parsed, err := parseHookEvent(bytes.NewReader(data))
	if err != nil {
		t.Errorf("parseHookEvent() error = %v", err)
	}

	// Verify
	if !shouldProcess(parsed) {
		t.Error("shouldProcess() returned false for valid Edit event")
	}

	filePath, ok := parsed.ToolInput["file_path"].(string)
	if !ok {
		t.Error("file_path not found in ToolInput")
	}
	if filePath != "/path/to/file.go" {
		t.Errorf("file_path = %q, want %q", filePath, "/path/to/file.go")
	}
}
