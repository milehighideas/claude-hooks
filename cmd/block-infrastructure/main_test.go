package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestExtractFilePathsFromCommand(t *testing.T) {
	tests := []struct {
		name     string
		command  string
		expected []string
	}{
		{
			name:     "redirect with >",
			command:  "echo 'test' > ~/.claude/CLAUDE.md",
			expected: []string{"~/.claude/CLAUDE.md"},
		},
		{
			name:     "redirect with >>",
			command:  "cat file.txt >> ~/.claude/hooks/test.py",
			expected: []string{"~/.claude/hooks/test.py"},
		},
		{
			name:     "sed in-place edit",
			command:  "sed -i 's/old/new/' ~/.claude/CLAUDE.md",
			expected: []string{"~/.claude/CLAUDE.md"},
		},
		{
			name:     "sed in-place edit with empty string",
			command:  "sed -i'' 's/old/new/' ~/.claude/hooks/test.sh",
			expected: []string{"~/.claude/hooks/test.sh"},
		},
		{
			name:     "awk in-place edit",
			command:  "awk -i inplace '{print $1}' ~/.claude/settings.json",
			expected: []string{"~/.claude/settings.json"},
		},
		{
			name:     "mv command",
			command:  "mv file.txt ~/.claude/hooks/backup.py",
			expected: []string{"~/.claude/hooks/backup.py"},
		},
		{
			name:     "cp command",
			command:  "cp -r src/ ~/.claude/hooks/",
			expected: []string{"~/.claude/hooks/"},
		},
		{
			name:     "vim editor",
			command:  "vim ~/.claude/CLAUDE.md",
			expected: []string{"~/.claude/CLAUDE.md"},
		},
		{
			name:     "nano editor",
			command:  "nano ~/.claude/hooks/test.py",
			expected: []string{"~/.claude/hooks/test.py"},
		},
		{
			name:     "heredoc to file",
			command:  "cat << EOF > ~/.claude/CLAUDE.md",
			expected: []string{"~/.claude/CLAUDE.md"},
		},
		{
			name:     "multiple operations",
			command:  "echo 'test' > file1.txt && cat file1.txt >> file2.txt",
			expected: []string{"file1.txt", "file2.txt"},
		},
		{
			name:     "no file operations",
			command:  "ls -la | grep test",
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractFilePathsFromCommand(tt.command)
			if len(result) != len(tt.expected) {
				t.Errorf("expected %d paths, got %d: %v", len(tt.expected), len(result), result)
				return
			}
			for i, expected := range tt.expected {
				if result[i] != expected {
					t.Errorf("path %d: expected %q, got %q", i, expected, result[i])
				}
			}
		})
	}
}

func TestIsProtectedFile(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("failed to get home dir: %v", err)
	}

	// Create a temporary project directory
	tmpDir := t.TempDir()

	tests := []struct {
		name         string
		filePath     string
		cwd          string
		expectBlock  bool
		reasonSubstr string
	}{
		{
			name:         "global CLAUDE.md",
			filePath:     "~/.claude/CLAUDE.md",
			cwd:          tmpDir,
			expectBlock:  true,
			reasonSubstr: "protected global configuration",
		},
		{
			name:         "global hooks directory",
			filePath:     "~/.claude/hooks/test.py",
			cwd:          tmpDir,
			expectBlock:  true,
			reasonSubstr: "protected infrastructure",
		},
		{
			name:         "settings.json",
			filePath:     "~/.claude/settings.json",
			cwd:          tmpDir,
			expectBlock:  true,
			reasonSubstr: "protected global configuration",
		},
		{
			name:         "ast_utils.py",
			filePath:     filepath.Join(home, ".claude/hooks/ast_utils.py"),
			cwd:          tmpDir,
			expectBlock:  true,
			reasonSubstr: "protected infrastructure",
		},
		{
			name:         "project hook config",
			filePath:     filepath.Join(tmpDir, ".claude-hooks-config.sh"),
			cwd:          tmpDir,
			expectBlock:  true,
			reasonSubstr: "Project infrastructure",
		},
		{
			name:         "project hook ignore",
			filePath:     filepath.Join(tmpDir, ".claude-hooks-ignore"),
			cwd:          tmpDir,
			expectBlock:  true,
			reasonSubstr: "Project infrastructure",
		},
		{
			name:         "project hook script .py",
			filePath:     filepath.Join(tmpDir, ".claude/hooks/test.py"),
			cwd:          tmpDir,
			expectBlock:  true,
			reasonSubstr: "Project infrastructure",
		},
		{
			name:         "project hook script .sh",
			filePath:     filepath.Join(tmpDir, ".claude/hooks/test.sh"),
			cwd:          tmpDir,
			expectBlock:  true,
			reasonSubstr: "Project infrastructure",
		},
		{
			name:         "project hook script .js",
			filePath:     filepath.Join(tmpDir, ".claude/hooks/test.js"),
			cwd:          tmpDir,
			expectBlock:  true,
			reasonSubstr: "Project infrastructure",
		},
		{
			name:        "regular project file",
			filePath:    filepath.Join(tmpDir, "src/main.go"),
			cwd:         tmpDir,
			expectBlock: false,
		},
		{
			name:        "regular home file",
			filePath:    "~/Documents/notes.txt",
			cwd:         tmpDir,
			expectBlock: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isProtected, reason := isProtectedFile(tt.filePath, tt.cwd)
			if isProtected != tt.expectBlock {
				t.Errorf("expected block=%v, got %v (reason: %s)", tt.expectBlock, isProtected, reason)
			}
			if tt.expectBlock && tt.reasonSubstr != "" {
				if reason == "" || !contains(reason, tt.reasonSubstr) {
					t.Errorf("expected reason to contain %q, got %q", tt.reasonSubstr, reason)
				}
			}
		})
	}
}

func TestHandleBashTool(t *testing.T) {
	tests := []struct {
		name        string
		input       HookInput
		expectBlock bool
	}{
		{
			name: "block redirect to CLAUDE.md",
			input: HookInput{
				ToolName: "Bash",
				ToolInput: map[string]interface{}{
					"command": "echo 'test' > ~/.claude/CLAUDE.md",
				},
				Cwd: "/tmp",
			},
			expectBlock: true,
		},
		{
			name: "block vim on hook script",
			input: HookInput{
				ToolName: "Bash",
				ToolInput: map[string]interface{}{
					"command": "vim ~/.claude/hooks/test.py",
				},
				Cwd: "/tmp",
			},
			expectBlock: true,
		},
		{
			name: "allow regular file edit",
			input: HookInput{
				ToolName: "Bash",
				ToolInput: map[string]interface{}{
					"command": "echo 'test' > /tmp/test.txt",
				},
				Cwd: "/tmp",
			},
			expectBlock: false,
		},
		{
			name: "allow non-edit commands",
			input: HookInput{
				ToolName: "Bash",
				ToolInput: map[string]interface{}{
					"command": "cat ~/.claude/CLAUDE.md",
				},
				Cwd: "/tmp",
			},
			expectBlock: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exitCode := runHookWithInput(t, tt.input)
			if tt.expectBlock && exitCode != 2 {
				t.Errorf("expected block (exit 2), got exit %d", exitCode)
			}
			if !tt.expectBlock && exitCode != 0 {
				t.Errorf("expected allow (exit 0), got exit %d", exitCode)
			}
		})
	}
}

func TestHandleEditTool(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name        string
		input       HookInput
		expectBlock bool
	}{
		{
			name: "block Edit on CLAUDE.md",
			input: HookInput{
				ToolName: "Edit",
				ToolInput: map[string]interface{}{
					"file_path": "~/.claude/CLAUDE.md",
				},
				Cwd: tmpDir,
			},
			expectBlock: true,
		},
		{
			name: "block Write on hook script",
			input: HookInput{
				ToolName: "Write",
				ToolInput: map[string]interface{}{
					"file_path": "~/.claude/hooks/test.py",
				},
				Cwd: tmpDir,
			},
			expectBlock: true,
		},
		{
			name: "block NotebookEdit on hook script",
			input: HookInput{
				ToolName: "NotebookEdit",
				ToolInput: map[string]interface{}{
					"notebook_path": "~/.claude/hooks/test.ipynb",
				},
				Cwd: tmpDir,
			},
			expectBlock: true,
		},
		{
			name: "block project hook config",
			input: HookInput{
				ToolName: "Edit",
				ToolInput: map[string]interface{}{
					"file_path": filepath.Join(tmpDir, ".claude-hooks-config.sh"),
				},
				Cwd: tmpDir,
			},
			expectBlock: true,
		},
		{
			name: "allow regular file edit",
			input: HookInput{
				ToolName: "Edit",
				ToolInput: map[string]interface{}{
					"file_path": filepath.Join(tmpDir, "src/main.go"),
				},
				Cwd: tmpDir,
			},
			expectBlock: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exitCode := runHookWithInput(t, tt.input)
			if tt.expectBlock && exitCode != 2 {
				t.Errorf("expected block (exit 2), got exit %d", exitCode)
			}
			if !tt.expectBlock && exitCode != 0 {
				t.Errorf("expected allow (exit 0), got exit %d", exitCode)
			}
		})
	}
}

func TestOtherTools(t *testing.T) {
	input := HookInput{
		ToolName: "Read",
		ToolInput: map[string]interface{}{
			"file_path": "~/.claude/CLAUDE.md",
		},
		Cwd: "/tmp",
	}

	exitCode := runHookWithInput(t, input)
	if exitCode != 0 {
		t.Errorf("expected allow (exit 0) for non-edit tool, got exit %d", exitCode)
	}
}

// Helper function to run the hook logic and return exit code
func runHookWithInput(t *testing.T, input HookInput) int {
	// This is a test helper that simulates the main function's behavior
	// without actually calling os.Exit

	if input.ToolName == "Bash" {
		command, ok := input.ToolInput["command"].(string)
		if !ok || command == "" {
			return 0
		}

		cwd := input.Cwd
		if cwd == "" {
			cwd, _ = os.Getwd()
		}

		filePaths := extractFilePathsFromCommand(command)
		if len(filePaths) == 0 {
			return 0
		}

		for _, filePath := range filePaths {
			if isProtected, _ := isProtectedFile(filePath, cwd); isProtected {
				return 2
			}
		}
		return 0
	}

	if input.ToolName == "Edit" || input.ToolName == "Write" || input.ToolName == "NotebookEdit" {
		cwd := input.Cwd
		if cwd == "" {
			cwd, _ = os.Getwd()
		}

		var filePath string
		if fp, ok := input.ToolInput["file_path"].(string); ok {
			filePath = fp
		} else if np, ok := input.ToolInput["notebook_path"].(string); ok {
			filePath = np
		}

		if filePath == "" {
			return 0
		}

		if isProtected, _ := isProtectedFile(filePath, cwd); isProtected {
			return 2
		}
		return 0
	}

	return 0
}

func TestNormalizePath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("failed to get home dir: %v", err)
	}

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "tilde expansion",
			input:    "~/test.txt",
			expected: filepath.Join(home, "test.txt"),
		},
		{
			name:     "just tilde",
			input:    "~",
			expected: home,
		},
		{
			name:     "tilde in path",
			input:    "~/.claude/CLAUDE.md",
			expected: filepath.Join(home, ".claude/CLAUDE.md"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := normalizePath(tt.input)
			if err != nil {
				t.Fatalf("normalizePath failed: %v", err)
			}
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestMainFunction(t *testing.T) {
	// Test that main function properly reads JSON and routes to handlers
	tests := []struct {
		name        string
		input       string
		expectBlock bool
	}{
		{
			name:        "bash block",
			input:       `{"tool_name":"Bash","tool_input":{"command":"echo test > ~/.claude/CLAUDE.md"},"cwd":"/tmp"}`,
			expectBlock: true,
		},
		{
			name:        "edit block",
			input:       `{"tool_name":"Edit","tool_input":{"file_path":"~/.claude/CLAUDE.md"},"cwd":"/tmp"}`,
			expectBlock: true,
		},
		{
			name:        "invalid json",
			input:       `{invalid json`,
			expectBlock: false,
		},
		{
			name:        "other tool",
			input:       `{"tool_name":"Read","tool_input":{"file_path":"~/.claude/CLAUDE.md"},"cwd":"/tmp"}`,
			expectBlock: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var input HookInput
			err := json.Unmarshal([]byte(tt.input), &input)

			var exitCode int
			if err != nil {
				exitCode = 0 // Allow on parse error
			} else {
				exitCode = runHookWithInput(t, input)
			}

			if tt.expectBlock && exitCode != 2 {
				t.Errorf("expected block (exit 2), got exit %d", exitCode)
			}
			if !tt.expectBlock && exitCode != 0 {
				t.Errorf("expected allow (exit 0), got exit %d", exitCode)
			}
		})
	}
}

func TestJSONParsing(t *testing.T) {
	tests := []struct {
		name      string
		jsonInput string
		expectErr bool
	}{
		{
			name:      "valid bash input",
			jsonInput: `{"tool_name":"Bash","tool_input":{"command":"ls"},"cwd":"/tmp"}`,
			expectErr: false,
		},
		{
			name:      "valid edit input",
			jsonInput: `{"tool_name":"Edit","tool_input":{"file_path":"test.txt"},"cwd":"/tmp"}`,
			expectErr: false,
		},
		{
			name:      "invalid json",
			jsonInput: `{invalid`,
			expectErr: true,
		},
		{
			name:      "empty input",
			jsonInput: ``,
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var input HookInput
			err := json.NewDecoder(bytes.NewBufferString(tt.jsonInput)).Decode(&input)
			if tt.expectErr && err == nil {
				t.Errorf("expected error but got none")
			}
			if !tt.expectErr && err != nil {
				t.Errorf("expected no error but got: %v", err)
			}
		})
	}
}

// Helper function
func contains(s, substr string) bool {
	return bytes.Contains([]byte(s), []byte(substr))
}
