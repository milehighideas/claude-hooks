package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// HookInput represents the PostToolUse JSON input from stdin.
type HookInput struct {
	ToolName  string                 `json:"tool_name"`
	ToolInput map[string]interface{} `json:"tool_input"`
}

// Config mirrors the .convex-gen.json structure (only the fields we need).
type Config struct {
	Convex struct {
		Path string `json:"path"`
	} `json:"convex"`
	Skip struct {
		Directories []string `json:"directories"`
		Patterns    []string `json:"patterns"`
	} `json:"skip"`
}

func main() {
	if err := run(os.Stdin, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "auto-convex-gen: %v\n", err)
	}
	// Always exit 0 — this is a non-blocking PostToolUse hook.
	os.Exit(0)
}

func run(stdin io.Reader, stderr io.Writer) error {
	input, err := readInput(stdin)
	if err != nil {
		return err
	}

	// Only react to Edit and Write.
	if input.ToolName != "Edit" && input.ToolName != "Write" {
		return nil
	}

	filePath := getFilePath(input.ToolInput)
	if filePath == "" {
		return nil
	}

	// Find the project root (directory containing .convex-gen.json).
	projectRoot := findProjectRoot(filePath)
	if projectRoot == "" {
		return nil
	}

	config, err := loadConfig(projectRoot)
	if err != nil {
		return nil // No config or bad config — silently skip.
	}

	// Check if the edited file is inside the Convex source directory.
	convexAbsPath, err := filepath.Abs(filepath.Join(projectRoot, config.Convex.Path))
	if err != nil {
		return nil
	}

	if !strings.HasPrefix(filePath, convexAbsPath+"/") && filePath != convexAbsPath {
		return nil
	}

	// Check if this specific file would be processed by convex-gen
	// (i.e. it's not in a skipped directory or matching a skip pattern).
	relPath, err := filepath.Rel(convexAbsPath, filePath)
	if err != nil {
		return nil
	}

	if shouldSkip(relPath, config) {
		return nil
	}

	// File is relevant — run convex-gen.
	return runConvexGen(projectRoot, stderr)
}

func readInput(r io.Reader) (*HookInput, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("reading stdin: %w", err)
	}

	var input HookInput
	if err := json.Unmarshal(data, &input); err != nil {
		return nil, fmt.Errorf("unmarshaling JSON: %w", err)
	}

	return &input, nil
}

func getFilePath(toolInput map[string]interface{}) string {
	if fp, ok := toolInput["file_path"].(string); ok {
		return fp
	}
	return ""
}

// findProjectRoot walks up from the file looking for .convex-gen.json.
func findProjectRoot(filePath string) string {
	dir := filepath.Dir(filePath)
	for {
		if _, err := os.Stat(filepath.Join(dir, ".convex-gen.json")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

func loadConfig(projectRoot string) (*Config, error) {
	data, err := os.ReadFile(filepath.Join(projectRoot, ".convex-gen.json"))
	if err != nil {
		return nil, err
	}

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	if config.Convex.Path == "" {
		config.Convex.Path = "packages/backend"
	}

	return &config, nil
}

// shouldSkip returns true if the file (relative to the convex dir) matches
// a skip directory or skip pattern from the config.
func shouldSkip(relPath string, config *Config) bool {
	// Only process .ts files.
	if !strings.HasSuffix(relPath, ".ts") {
		return true
	}

	// Skip .d.ts files.
	if strings.HasSuffix(relPath, ".d.ts") {
		return true
	}

	// Check directory components against skip directories.
	parts := strings.Split(filepath.Dir(relPath), string(filepath.Separator))
	for _, part := range parts {
		for _, skipDir := range config.Skip.Directories {
			if part == skipDir {
				return true
			}
		}
	}

	// Check the filename against skip patterns.
	fileName := filepath.Base(relPath)
	for _, pattern := range config.Skip.Patterns {
		re, err := regexp.Compile(pattern)
		if err != nil {
			continue
		}
		if re.MatchString(fileName) {
			return true
		}
	}

	return false
}

// runConvexGen executes the convex-gen binary from the project root.
func runConvexGen(projectRoot string, stderr io.Writer) error {
	// Look for convex-gen binary next to this binary first.
	selfPath, err := os.Executable()
	if err == nil {
		binDir := filepath.Dir(selfPath)
		candidate := filepath.Join(binDir, "convex-gen")
		if _, err := os.Stat(candidate); err == nil {
			return execBinary(candidate, projectRoot, stderr)
		}
	}

	// Fall back to PATH.
	path, err := exec.LookPath("convex-gen")
	if err != nil {
		return fmt.Errorf("convex-gen binary not found")
	}

	return execBinary(path, projectRoot, stderr)
}

func execBinary(binaryPath, projectRoot string, stderr io.Writer) error {
	cmd := exec.Command(binaryPath)
	cmd.Dir = projectRoot
	cmd.Stderr = stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("convex-gen failed: %w", err)
	}

	return nil
}
