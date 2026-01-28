package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestRunCommandWithOutput(t *testing.T) {
	tests := []struct {
		name       string
		command    string
		args       []string
		wantErr    bool
		wantOutput string
		contains   string // substring match for output
	}{
		{
			name:       "echo simple text",
			command:    "echo",
			args:       []string{"hello"},
			wantErr:    false,
			wantOutput: "hello\n",
		},
		{
			name:       "echo multiple args",
			command:    "echo",
			args:       []string{"hello", "world"},
			wantErr:    false,
			wantOutput: "hello world\n",
		},
		{
			name:    "command not found",
			command: "nonexistent-command-12345",
			args:    []string{},
			wantErr: true,
		},
		{
			name:    "command fails with exit code",
			command: "false",
			args:    []string{},
			wantErr: true,
		},
		{
			name:     "pwd returns current directory",
			command:  "pwd",
			args:     []string{},
			wantErr:  false,
			contains: "/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, err := runCommandWithOutput(tt.command, tt.args...)

			if tt.wantErr {
				if err == nil {
					t.Errorf("runCommandWithOutput() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("runCommandWithOutput() unexpected error: %v", err)
				return
			}

			if tt.wantOutput != "" && output != tt.wantOutput {
				t.Errorf("runCommandWithOutput() output = %q, want %q", output, tt.wantOutput)
			}

			if tt.contains != "" && !strings.Contains(output, tt.contains) {
				t.Errorf("runCommandWithOutput() output = %q, want to contain %q", output, tt.contains)
			}
		})
	}
}

func TestRunCommand(t *testing.T) {
	tests := []struct {
		name    string
		command string
		args    []string
		wantErr bool
	}{
		{
			name:    "successful command",
			command: "true",
			args:    []string{},
			wantErr: false,
		},
		{
			name:    "failing command",
			command: "false",
			args:    []string{},
			wantErr: true,
		},
		{
			name:    "echo command",
			command: "echo",
			args:    []string{"test"},
			wantErr: false,
		},
		{
			name:    "command not found",
			command: "nonexistent-command-67890",
			args:    []string{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := runCommand(tt.command, tt.args...)

			if tt.wantErr {
				if err == nil {
					t.Errorf("runCommand() expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("runCommand() unexpected error: %v", err)
				}
			}
		})
	}
}

func TestRunCommandInDir(t *testing.T) {
	// Create a temp directory for testing
	tempDir := t.TempDir()

	// Create a subdirectory
	subDir := filepath.Join(tempDir, "subdir")
	if err := os.Mkdir(subDir, 0755); err != nil {
		t.Fatalf("failed to create subdir: %v", err)
	}

	tests := []struct {
		name    string
		dir     string
		command string
		args    []string
		wantErr bool
		setup   func() error
		verify  func(t *testing.T)
	}{
		{
			name:    "run in temp directory",
			dir:     tempDir,
			command: "pwd",
			args:    []string{},
			wantErr: false,
		},
		{
			name:    "run in subdirectory",
			dir:     subDir,
			command: "pwd",
			args:    []string{},
			wantErr: false,
		},
		{
			name:    "command fails in directory",
			dir:     tempDir,
			command: "false",
			args:    []string{},
			wantErr: true,
		},
		{
			name:    "nonexistent directory",
			dir:     "/nonexistent-dir-12345",
			command: "pwd",
			args:    []string{},
			wantErr: true,
		},
		{
			name:    "touch file in directory",
			dir:     tempDir,
			command: "touch",
			args:    []string{"testfile.txt"},
			wantErr: false,
			verify: func(t *testing.T) {
				// Verify file was created in the correct directory
				filePath := filepath.Join(tempDir, "testfile.txt")
				if _, err := os.Stat(filePath); os.IsNotExist(err) {
					t.Errorf("file was not created in the expected directory")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setup != nil {
				if err := tt.setup(); err != nil {
					t.Fatalf("setup failed: %v", err)
				}
			}

			err := runCommandInDir(tt.dir, tt.command, tt.args...)

			if tt.wantErr {
				if err == nil {
					t.Errorf("runCommandInDir() expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("runCommandInDir() unexpected error: %v", err)
				}
			}

			if tt.verify != nil {
				tt.verify(t)
			}
		})
	}
}

func TestResolveCommand(t *testing.T) {
	tests := []struct {
		name        string
		command     string
		wantErr     bool
		wantAbsPath bool
	}{
		{
			name:        "resolve echo",
			command:     "echo",
			wantErr:     false,
			wantAbsPath: true,
		},
		{
			name:        "resolve ls",
			command:     "ls",
			wantErr:     false,
			wantAbsPath: true,
		},
		{
			name:    "nonexistent command",
			command: "nonexistent-cmd-99999",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path, err := resolveCommand(tt.command)

			if tt.wantErr {
				if err == nil {
					t.Errorf("resolveCommand() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("resolveCommand() unexpected error: %v", err)
				return
			}

			if tt.wantAbsPath {
				if !filepath.IsAbs(path) {
					t.Errorf("resolveCommand() returned non-absolute path: %s", path)
				}
			}

			// Verify the resolved path exists
			if _, err := os.Stat(path); os.IsNotExist(err) {
				t.Errorf("resolveCommand() returned non-existent path: %s", path)
			}
		})
	}
}

func TestRunCommandWithOutputCapturesStderr(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping test on windows")
	}

	// Run a command that writes to stderr and fails
	output, err := runCommandWithOutput("sh", "-c", "echo 'error message' >&2 && exit 1")

	if err == nil {
		t.Error("expected command to fail")
	}

	// The function returns stderr on error
	if !strings.Contains(output, "error message") {
		t.Errorf("expected stderr in output, got: %q", output)
	}
}

func TestRunCommandInDirVerifiesWorkingDirectory(t *testing.T) {
	tempDir := t.TempDir()

	// Create a marker file in temp directory
	markerFile := "marker-file.txt"
	markerPath := filepath.Join(tempDir, markerFile)
	if err := os.WriteFile(markerPath, []byte("test"), 0644); err != nil {
		t.Fatalf("failed to create marker file: %v", err)
	}

	// Use ls to verify we're in the right directory
	output, err := runCommandWithOutputInDir(tempDir, "ls")
	if err != nil {
		t.Fatalf("ls failed: %v", err)
	}

	if !strings.Contains(output, markerFile) {
		t.Errorf("expected to find %s in directory listing, got: %s", markerFile, output)
	}
}

// runCommandWithOutputInDir is a helper for testing that combines dir and output capture
func runCommandWithOutputInDir(dir, name string, args ...string) (string, error) {
	cmd := execCommand(name, args...)
	cmd.Dir = dir
	output, err := cmd.Output()
	return string(output), err
}

// execCommand wraps exec.Command for testability
func execCommand(name string, args ...string) *exec.Cmd {
	return exec.Command(name, args...)
}
