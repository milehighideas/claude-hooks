// auto-tiers-gen is a Claude Code PostToolUse hook. When a watched config
// file is edited (e.g. packages/tiers/src/config.ts), it shells out to a
// project-defined command to regenerate derived files. Generic — each
// project configures the watch path and command in .tiers-gen.json.
//
// .tiers-gen.json schema:
//
//	{
//	  "watchFile": "packages/tiers/src/config.ts",
//	  "command": ["bun", "tiers:gen"]
//	}
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
)

const configFileName = ".tiers-gen.json"

type HookInput struct {
	ToolName  string                 `json:"tool_name"`
	ToolInput map[string]interface{} `json:"tool_input"`
}

type Config struct {
	WatchFile string   `json:"watchFile"`
	Command   []string `json:"command"`
}

func main() {
	if err := run(os.Stdin, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "auto-tiers-gen: %v\n", err)
	}
	// Always exit 0 — non-blocking PostToolUse hook.
	os.Exit(0)
}

func run(stdin io.Reader, stderr io.Writer) error {
	input, err := readInput(stdin)
	if err != nil {
		return err
	}

	if input.ToolName != "Edit" && input.ToolName != "Write" {
		return nil
	}

	filePath := getFilePath(input.ToolInput)
	if filePath == "" {
		return nil
	}

	projectRoot := findProjectRoot(filePath)
	if projectRoot == "" {
		return nil
	}

	config, err := loadConfig(projectRoot)
	if err != nil || config.WatchFile == "" || len(config.Command) == 0 {
		return nil
	}

	watchAbs, err := filepath.Abs(filepath.Join(projectRoot, config.WatchFile))
	if err != nil {
		return nil
	}

	fileAbs, err := filepath.Abs(filePath)
	if err != nil {
		return nil
	}

	if fileAbs != watchAbs {
		return nil
	}

	return runCommand(projectRoot, config.Command, stderr)
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

// findProjectRoot walks up from the edited file looking for .tiers-gen.json.
func findProjectRoot(filePath string) string {
	dir := filepath.Dir(filePath)
	for {
		if _, err := os.Stat(filepath.Join(dir, configFileName)); err == nil {
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
	data, err := os.ReadFile(filepath.Join(projectRoot, configFileName))
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func runCommand(projectRoot string, command []string, stderr io.Writer) error {
	resolved, err := exec.LookPath(command[0])
	if err != nil {
		return fmt.Errorf("command not found: %s", command[0])
	}
	cmd := exec.Command(resolved, command[1:]...)
	cmd.Dir = projectRoot
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("command failed: %w", err)
	}
	return nil
}
