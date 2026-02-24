package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type HookInput struct {
	ToolName  string                 `json:"tool_name"`
	ToolInput map[string]interface{} `json:"tool_input"`
}

// Extensions that Prettier supports and we want to auto-format.
var formatExtensions = map[string]bool{
	".ts":   true,
	".tsx":  true,
	".js":   true,
	".jsx":  true,
	".json": true,
	".md":   true,
	".css":  true,
}

func main() {
	if err := run(os.Stdin, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "format-on-save: %v\n", err)
	}
	// Always exit 0 — formatting is non-blocking.
	os.Exit(0)
}

func run(stdin io.Reader, stderr io.Writer) error {
	input, err := readInput(stdin)
	if err != nil {
		return err
	}

	filePath := getFilePath(input.ToolInput)
	if filePath == "" {
		return nil
	}

	if !shouldFormat(filePath) {
		return nil
	}

	return runPrettier(filePath, stderr)
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

func shouldFormat(filePath string) bool {
	ext := strings.ToLower(filepath.Ext(filePath))
	return formatExtensions[ext]
}

// runPrettier finds and runs prettier on the file.
// Checks for a local project install first, then falls back to a global one.
func runPrettier(filePath string, stderr io.Writer) error {
	// Walk up from the file to find the nearest node_modules/.bin/prettier.
	dir := filepath.Dir(filePath)
	for {
		candidate := filepath.Join(dir, "node_modules", ".bin", "prettier")
		if _, err := os.Stat(candidate); err == nil {
			return execPrettier(candidate, filePath, stderr)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	// Fall back to global prettier.
	if path, err := exec.LookPath("prettier"); err == nil {
		return execPrettier(path, filePath, stderr)
	}

	return nil // No prettier found — silently skip.
}

func execPrettier(prettierPath, filePath string, stderr io.Writer) error {
	cmd := exec.Command(prettierPath, "--write", filePath)
	cmd.Stderr = stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("prettier failed: %w", err)
	}

	return nil
}
