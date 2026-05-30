// auto-lingui-extract is a Claude Code PostToolUse hook. When a source file
// that contains a Lingui macro is edited, it runs the owning package's
// `lingui:extract` command so the message catalogs (.po / compiled .ts) stay
// in sync with the source — the lingui analogue of auto-convex-gen.
//
// Generic: each project configures its packages in .lingui-gen.json. The hook
// runs at most one command per edit (the first target whose include path
// matches the edited file), and only when the file actually contains a macro
// marker, so routine edits to non-translatable files are a no-op.
//
// .lingui-gen.json schema:
//
//	{
//	  "macroMarkers": ["@lingui/core/macro", "@lingui/react/macro"],
//	  "extensions": [".ts", ".tsx"],
//	  "targets": [
//	    {
//	      "include": ["apps/mobile/src/", "apps/mobile/components/", "apps/mobile/app/"],
//	      "exclude": ["apps/mobile/src/locales/"],
//	      "command": ["bun", "--filter", "@dashtag/mobile", "lingui:extract"]
//	    }
//	  ]
//	}
//
// macroMarkers and extensions are optional; the defaults above are used when
// they are omitted.
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

const configFileName = ".lingui-gen.json"

var (
	defaultMacroMarkers = []string{"@lingui/core/macro", "@lingui/react/macro"}
	defaultExtensions   = []string{".ts", ".tsx"}
	// skipSuffixes are never translatable sources; lingui excludes them too.
	skipSuffixes = []string{".test.ts", ".test.tsx", ".spec.ts", ".spec.tsx", ".d.ts"}
)

// HookInput is the PostToolUse JSON delivered on stdin.
type HookInput struct {
	ToolName  string                 `json:"tool_name"`
	ToolInput map[string]interface{} `json:"tool_input"`
}

// Target maps a set of source directories to the extract command for the
// package that owns them.
type Target struct {
	Include []string `json:"include"`
	Exclude []string `json:"exclude"`
	Command []string `json:"command"`
}

// Config mirrors .lingui-gen.json.
type Config struct {
	MacroMarkers []string `json:"macroMarkers"`
	Extensions   []string `json:"extensions"`
	Targets      []Target `json:"targets"`
}

func main() {
	if err := run(os.Stdin, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "auto-lingui-extract: %v\n", err)
	}
	// Always exit 0 — this is a non-blocking PostToolUse hook.
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
	if err != nil || len(config.Targets) == 0 {
		return nil
	}
	applyDefaults(config)

	relPath, err := filepath.Rel(projectRoot, filePath)
	if err != nil {
		return nil
	}
	relPath = filepath.ToSlash(relPath)

	if !hasExtension(relPath, config.Extensions) || isSkipped(relPath) {
		return nil
	}

	target := matchTarget(relPath, config.Targets)
	if target == nil {
		return nil
	}

	// Only regenerate when the edited file actually defines translatable
	// strings — keeps the hook quiet for the common non-i18n edit.
	if !fileContainsMarker(filePath, config.MacroMarkers) {
		return nil
	}

	return runCommand(projectRoot, target.Command, stderr)
}

// applyDefaults fills in optional config fields.
func applyDefaults(cfg *Config) {
	if len(cfg.MacroMarkers) == 0 {
		cfg.MacroMarkers = defaultMacroMarkers
	}
	if len(cfg.Extensions) == 0 {
		cfg.Extensions = defaultExtensions
	}
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

// findProjectRoot walks up from the edited file looking for .lingui-gen.json.
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
		return nil, fmt.Errorf("parsing %s: %w", configFileName, err)
	}
	return &cfg, nil
}

func hasExtension(relPath string, extensions []string) bool {
	for _, ext := range extensions {
		if strings.HasSuffix(relPath, ext) {
			return true
		}
	}
	return false
}

func isSkipped(relPath string) bool {
	for _, suffix := range skipSuffixes {
		if strings.HasSuffix(relPath, suffix) {
			return true
		}
	}
	return false
}

// matchTarget returns the first target whose include prefix matches relPath
// without also matching an exclude prefix. Returns nil when nothing matches.
func matchTarget(relPath string, targets []Target) *Target {
	for i := range targets {
		t := &targets[i]
		if !matchesAny(relPath, t.Include) || matchesAny(relPath, t.Exclude) {
			continue
		}
		if len(t.Command) == 0 {
			continue
		}
		return t
	}
	return nil
}

func matchesAny(relPath string, prefixes []string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(relPath, filepath.ToSlash(p)) {
			return true
		}
	}
	return false
}

func fileContainsMarker(filePath string, markers []string) bool {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return false
	}
	content := string(data)
	for _, m := range markers {
		if strings.Contains(content, m) {
			return true
		}
	}
	return false
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
