package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// HookInput represents the JSON input from Claude Code
type HookInput struct {
	ToolName  string                 `json:"tool_name"`
	ToolInput map[string]interface{} `json:"tool_input"`
	Cwd       string                 `json:"cwd"`
}

// Protected paths relative to home directory
var protectedPaths = []string{
	"~/.claude/hooks/",
	"~/.claude/CLAUDE.md",
	"~/.claude/settings.json",
}

// Project-level protected patterns (regex)
var projectProtectedPatterns = []*regexp.Regexp{
	regexp.MustCompile(`\.claude-hooks-config\.sh$`),
	regexp.MustCompile(`\.claude-hooks-ignore$`),
	regexp.MustCompile(`\.claude/hooks/.*\.py$`),
	regexp.MustCompile(`\.claude/hooks/.*\.sh$`),
	regexp.MustCompile(`\.claude/hooks/.*\.js$`),
}

// Bash command patterns for extracting file paths
var (
	// Pattern 1: Redirects (cat > file, echo >> file)
	redirectPattern = regexp.MustCompile(`(?:>>?)\s+([^\s;&|<>]+)`)

	// Pattern 3: Move/copy destination (mv src dest, cp src dest)
	moveCopyPattern = regexp.MustCompile(`(?:mv|cp)\s+(?:-\w+\s+)*\S+\s+([^\s;&|<>]+)`)

	// Pattern 4: Text editors (vim file, nano file, emacs file)
	editorPattern = regexp.MustCompile(`(?:vim?|nano|emacs|vi)\s+([^\s;&|<>]+)`)

	// Pattern 5: cat with heredoc (cat << EOF > file)
	heredocPattern = regexp.MustCompile(`cat\s+<<.*?>\s*([^\s;&|<>]+)`)
)

func main() {
	var input HookInput
	if err := json.NewDecoder(os.Stdin).Decode(&input); err != nil {
		// Allow if we can't parse input
		os.Exit(0)
	}

	// Handle different tool types
	switch input.ToolName {
	case "Bash":
		handleBashTool(input)
	case "Edit", "Write", "NotebookEdit":
		handleEditTool(input)
	default:
		os.Exit(0)
	}
}

func handleBashTool(input HookInput) {
	command, ok := input.ToolInput["command"].(string)
	if !ok || command == "" {
		os.Exit(0)
	}

	cwd := input.Cwd
	if cwd == "" {
		cwd, _ = os.Getwd()
	}

	filePaths := extractFilePathsFromCommand(command)
	if len(filePaths) == 0 {
		os.Exit(0)
	}

	for _, filePath := range filePaths {
		if isProtected, reason := isProtectedFile(filePath, cwd); isProtected {
			blockBashEdit(command, filePath, reason)
		}
	}

	os.Exit(0)
}

func handleEditTool(input HookInput) {
	cwd := input.Cwd
	if cwd == "" {
		cwd, _ = os.Getwd()
	}

	// Get file path from tool input
	var filePath string
	if fp, ok := input.ToolInput["file_path"].(string); ok {
		filePath = fp
	} else if np, ok := input.ToolInput["notebook_path"].(string); ok {
		filePath = np
	}

	if filePath == "" {
		os.Exit(0)
	}

	if isProtected, reason := isProtectedFile(filePath, cwd); isProtected {
		blockFileEdit(filePath, reason)
	}

	os.Exit(0)
}

func extractFilePathsFromCommand(command string) []string {
	var filePaths []string
	seen := make(map[string]bool)

	// Helper to add unique paths
	addPath := func(path string) {
		if path != "" && !seen[path] {
			filePaths = append(filePaths, path)
			seen[path] = true
		}
	}

	// Pattern 1: Redirects
	for _, match := range redirectPattern.FindAllStringSubmatch(command, -1) {
		addPath(match[1])
	}

	// Pattern 2: In-place edits (sed -i, awk -i)
	if strings.Contains(command, "sed") || strings.Contains(command, "awk") {
		if strings.Contains(command, "-i") {
			parts := strings.Fields(command)
			// Find the file argument (usually last non-flag, non-script argument)
			for i := len(parts) - 1; i >= 0; i-- {
				if !strings.HasPrefix(parts[i], "-") && !strings.HasPrefix(parts[i], "s/") {
					addPath(parts[i])
					break
				}
			}
		}
	}

	// Pattern 3: Move/copy
	for _, match := range moveCopyPattern.FindAllStringSubmatch(command, -1) {
		addPath(match[1])
	}

	// Pattern 4: Text editors
	for _, match := range editorPattern.FindAllStringSubmatch(command, -1) {
		addPath(match[1])
	}

	// Pattern 5: Heredoc
	for _, match := range heredocPattern.FindAllStringSubmatch(command, -1) {
		addPath(match[1])
	}

	return filePaths
}

func isProtectedFile(filePath, cwd string) (bool, string) {
	normalizedPath, err := normalizePath(filePath)
	if err != nil {
		// Invalid path, let it through (will fail naturally)
		return false, ""
	}

	// Check absolute protected paths
	for _, protected := range protectedPaths {
		normalizedProtected, err := normalizePath(protected)
		if err != nil {
			continue
		}

		// Check if file is in a protected directory
		if strings.HasSuffix(protected, "/") {
			rel, err := filepath.Rel(normalizedProtected, normalizedPath)
			if err == nil && !strings.HasPrefix(rel, "..") {
				return true, fmt.Sprintf("Files in %s are protected infrastructure", protected)
			}
		} else if normalizedPath == normalizedProtected {
			// Exact match
			return true, fmt.Sprintf("%s is protected global configuration", protected)
		}
	}

	// Check project-level protected patterns
	normalizedCwd, err := normalizePath(cwd)
	if err == nil {
		rel, err := filepath.Rel(normalizedCwd, normalizedPath)
		if err == nil && !strings.HasPrefix(rel, "..") {
			// File is within project directory
			for _, pattern := range projectProtectedPatterns {
				if pattern.MatchString(rel) {
					return true, fmt.Sprintf("Project infrastructure files matching %s are protected", pattern.String())
				}
			}
		}
	}

	return false, ""
}

func normalizePath(path string) (string, error) {
	// Expand home directory
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		path = filepath.Join(home, path[2:])
	} else if path == "~" {
		return os.UserHomeDir()
	}

	// Clean and get absolute path
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}

	// Clean the path to remove any .. or . components
	cleanPath := filepath.Clean(absPath)

	// Try to evaluate symlinks for the full path first
	evalPath, err := filepath.EvalSymlinks(cleanPath)
	if err == nil {
		return evalPath, nil
	}

	// If the full path doesn't exist, try to evaluate symlinks for ancestors
	// Walk up the directory tree until we find a directory that exists
	dir := filepath.Dir(cleanPath)
	remaining := []string{filepath.Base(cleanPath)}

	for dir != "/" && dir != "." {
		evalDir, err := filepath.EvalSymlinks(dir)
		if err == nil {
			// Found an existing ancestor, reconstruct the path
			result := evalDir
			for i := len(remaining) - 1; i >= 0; i-- {
				result = filepath.Join(result, remaining[i])
			}
			return result, nil
		}

		// Move up one level
		remaining = append(remaining, filepath.Base(dir))
		dir = filepath.Dir(dir)
	}

	// If no existing ancestor found, just return the clean path
	return cleanPath, nil
}

func blockBashEdit(command, filePath, reason string) {
	msg := fmt.Sprintf(`❌ BLOCKED: Cannot modify infrastructure file via Bash

Command: %s

File: %s
Reason: %s

Infrastructure files are protected from modification via shell commands.

If you need to modify this file:
1. Ask the user to make the change manually
2. Or ask the user to temporarily disable this hook

Protected file categories:
- Hook scripts (~/.claude/hooks/*.py, *.sh, *.js)
- Global instructions (~/.claude/CLAUDE.md)
- Settings (~/.claude/settings.json)
- Project hook configuration (.claude-hooks-config.sh, .claude-hooks-ignore)

This protection ensures agents cannot circumvent quality controls.
`, command, filePath, reason)

	fmt.Fprintln(os.Stderr, msg)
	os.Exit(2)
}

func blockFileEdit(filePath, reason string) {
	msg := fmt.Sprintf(`❌ BLOCKED: Cannot edit infrastructure file

File: %s
Reason: %s

Infrastructure files are protected to prevent agents from modifying their own
constraints or critical configuration files.

If you need to modify this file:
1. Ask the user to make the change manually
2. Or ask the user to temporarily disable this hook

Protected file categories:
- Hook scripts (~/.claude/hooks/*.py, *.sh, *.js)
- Hook utilities (ast_utils.py, srp_validators.py, etc.)
- Global instructions (~/.claude/CLAUDE.md)
- Project hook configuration (.claude-hooks-config.sh, .claude-hooks-ignore)

This protection ensures agents cannot circumvent quality controls or
modify their own behavior without user approval.
`, filePath, reason)

	fmt.Fprintln(os.Stderr, msg)
	os.Exit(2)
}
