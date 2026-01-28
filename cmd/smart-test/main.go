package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// HookEvent represents the JSON input from Claude Code
type HookEvent struct {
	HookEventName string                 `json:"hook_event_name"`
	ToolName      string                 `json:"tool_name"`
	ToolInput     map[string]interface{} `json:"tool_input"`
	CWD           string                 `json:"cwd"`
}

// ProjectConfig represents .claude-hooks.json configuration
type ProjectConfig struct {
	Lint      string `json:"lint"`      // Custom lint command
	Test      string `json:"test"`      // Custom test command (e.g., "pnpm turbo test")
	Typecheck string `json:"typecheck"` // Custom typecheck command
}

// ProjectType represents detected project languages
type ProjectType struct {
	Languages []string
}

// ErrorCollector tracks test failures
type ErrorCollector struct {
	errors []string
}

func (ec *ErrorCollector) Add(msg string) {
	ec.errors = append(ec.errors, msg)
	fmt.Fprintf(os.Stderr, "❌ %s\n", msg)
}

func (ec *ErrorCollector) Count() int {
	return len(ec.errors)
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	// Check if testing is enabled
	if !isTestingEnabled() {
		os.Exit(0)
	}

	// Read JSON from stdin
	event, err := parseHookEvent(os.Stdin)
	if err != nil {
		return fmt.Errorf("failed to parse hook event: %w", err)
	}

	// Only process PostToolUse events for Edit/Write/MultiEdit tools
	if !shouldProcess(event) {
		os.Exit(0)
	}

	// Get the file path that was edited
	filePath, ok := event.ToolInput["file_path"].(string)
	if !ok || filePath == "" {
		os.Exit(0)
	}

	// Change to the directory containing the edited file
	fileDir := filepath.Dir(filePath)
	if err := os.Chdir(fileDir); err != nil {
		return fmt.Errorf("failed to change directory: %w", err)
	}

	// Find project root first (needed for config and ignore patterns)
	projectRoot, _ := findProjectRoot()

	// Load ignore patterns
	ignorePatterns, err := loadIgnorePatterns()
	if err != nil {
		return fmt.Errorf("failed to load ignore patterns: %w", err)
	}

	// Check if file should be skipped
	if shouldSkipFile(filePath, ignorePatterns) {
		os.Exit(0)
	}

	// Collect errors
	errorCollector := &ErrorCollector{}

	// Check for project-level config first (.claude-hooks.json)
	if projectRoot != "" {
		config, err := loadProjectConfig(projectRoot)
		if err == nil && config.Test != "" {
			// Change to project root to run the command
			if err := os.Chdir(projectRoot); err != nil {
				return fmt.Errorf("failed to change to project root: %w", err)
			}
			runCustomCommand(config.Test, errorCollector)
			return exitWithResult(errorCollector)
		}
	}

	// Detect project type
	projectType := detectProjectType()
	if len(projectType.Languages) == 0 {
		// No recognized project type, exit silently
		os.Exit(0)
	}

	// Try project commands (make test or scripts/test.sh)
	if tryProjectCommand(filePath, ignorePatterns, errorCollector) {
		// Project command handled testing
		return exitWithResult(errorCollector)
	}

	// Fall back to language-specific test runners
	for _, lang := range projectType.Languages {
		runLanguageTests(lang, filePath, ignorePatterns, errorCollector)
	}

	return exitWithResult(errorCollector)
}

func parseHookEvent(r io.Reader) (*HookEvent, error) {
	var event HookEvent
	decoder := json.NewDecoder(r)
	if err := decoder.Decode(&event); err != nil {
		return nil, err
	}
	return &event, nil
}

func shouldProcess(event *HookEvent) bool {
	if event.HookEventName != "PostToolUse" {
		return false
	}
	validTools := map[string]bool{
		"Edit":      true,
		"Write":     true,
		"MultiEdit": true,
	}
	return validTools[event.ToolName]
}

func isTestingEnabled() bool {
	val := os.Getenv("CLAUDE_HOOKS_TEST_ON_EDIT")
	if val == "" {
		return true // Default: enabled
	}
	return val == "true" || val == "1"
}

func isRaceEnabled() bool {
	val := os.Getenv("CLAUDE_HOOKS_ENABLE_RACE")
	if val == "" {
		return true // Default: enabled
	}
	return val == "true" || val == "1"
}

func detectProjectType() *ProjectType {
	pt := &ProjectType{Languages: []string{}}

	// Go project
	if fileExists("go.mod") || fileExists("go.sum") || hasFilesWithExtension(".go", 3) {
		pt.Languages = append(pt.Languages, "go")
	}

	// Python project
	if fileExists("pyproject.toml") || fileExists("setup.py") || fileExists("requirements.txt") || hasFilesWithExtension(".py", 3) {
		pt.Languages = append(pt.Languages, "python")
	}

	// JavaScript/TypeScript project
	if fileExists("package.json") || fileExists("tsconfig.json") || hasFilesWithExtensions([]string{".js", ".ts", ".jsx", ".tsx"}, 3) {
		pt.Languages = append(pt.Languages, "javascript")
	}

	// Rust project
	if fileExists("Cargo.toml") || hasFilesWithExtension(".rs", 3) {
		pt.Languages = append(pt.Languages, "rust")
	}

	// Shell scripts
	if hasFilesWithExtensions([]string{".sh", ".bash"}, 3) {
		pt.Languages = append(pt.Languages, "shell")
	}

	return pt
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func hasFilesWithExtension(ext string, maxDepth int) bool {
	found := false
	_ = filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if found {
			return filepath.SkipAll
		}
		if info.IsDir() {
			depth := strings.Count(path, string(os.PathSeparator))
			if depth > maxDepth {
				return filepath.SkipDir
			}
		}
		if !info.IsDir() && filepath.Ext(path) == ext {
			found = true
			return filepath.SkipAll
		}
		return nil
	})
	return found
}

func hasFilesWithExtensions(exts []string, maxDepth int) bool {
	for _, ext := range exts {
		if hasFilesWithExtension(ext, maxDepth) {
			return true
		}
	}
	return false
}

func loadIgnorePatterns() ([]string, error) {
	patterns := []string{}

	// Find project root
	root, err := findProjectRoot()
	if err != nil {
		// No project root found, return empty patterns
		return patterns, nil
	}

	ignorePath := filepath.Join(root, ".claude-hooks-ignore")
	file, err := os.Open(ignorePath)
	if err != nil {
		// No ignore file, return empty patterns
		return patterns, nil
	}
	defer func() {
		_ = file.Close()
	}()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		// Skip comments and empty lines
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		patterns = append(patterns, line)
	}

	return patterns, scanner.Err()
}

func loadProjectConfig(projectRoot string) (*ProjectConfig, error) {
	configPath := filepath.Join(projectRoot, ".claude-hooks.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}

	var config ProjectConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	return &config, nil
}

func runCustomCommand(command string, ec *ErrorCollector) {
	// Parse the command string into parts
	parts := parseCommand(command)
	if len(parts) == 0 {
		return
	}

	cmd := exec.Command(parts[0], parts[1:]...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		ec.Add(fmt.Sprintf("test command failed: %s", command))
		if len(output) > 0 {
			fmt.Fprint(os.Stderr, string(output))
		}
	}
}

func parseCommand(command string) []string {
	// Simple command parsing - splits on spaces but respects quotes
	var parts []string
	var current strings.Builder
	inQuote := false
	quoteChar := rune(0)

	for _, r := range command {
		switch {
		case r == '"' || r == '\'':
			if !inQuote {
				inQuote = true
				quoteChar = r
			} else if r == quoteChar {
				inQuote = false
				quoteChar = 0
			} else {
				current.WriteRune(r)
			}
		case r == ' ' && !inQuote:
			if current.Len() > 0 {
				parts = append(parts, current.String())
				current.Reset()
			}
		default:
			current.WriteRune(r)
		}
	}

	if current.Len() > 0 {
		parts = append(parts, current.String())
	}

	return parts
}

func findProjectRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for {
		// Check for project markers
		markers := []string{".git", "go.mod", "package.json", "Cargo.toml", "setup.py", "pyproject.toml"}
		for _, marker := range markers {
			path := filepath.Join(dir, marker)
			if fileExists(path) {
				return dir, nil
			}
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached root directory
			return "", fmt.Errorf("project root not found")
		}
		dir = parent
	}
}

func shouldSkipFile(filePath string, patterns []string) bool {
	// Get basename for matching
	basename := filepath.Base(filePath)

	for _, pattern := range patterns {
		// Directory pattern (ends with /**)
		if strings.HasSuffix(pattern, "/**") {
			dirPattern := strings.TrimSuffix(pattern, "/**")
			if strings.HasPrefix(filePath, dirPattern+"/") {
				return true
			}
		}

		// Glob pattern
		if strings.ContainsAny(pattern, "*?") {
			matched, _ := filepath.Match(pattern, filePath)
			if matched {
				return true
			}
			matched, _ = filepath.Match(pattern, basename)
			if matched {
				return true
			}
		}

		// Exact match
		if filePath == pattern || basename == pattern {
			return true
		}
	}

	return false
}

func tryProjectCommand(filePath string, ignorePatterns []string, ec *ErrorCollector) bool {
	// Try make test
	if fileExists("Makefile") {
		if commandExists("make") && makeTargetExists("test") {
			output, err := exec.Command("make", "test").CombinedOutput()
			if err != nil {
				ec.Add("make test failed")
				if len(output) > 0 {
					fmt.Fprint(os.Stderr, string(output))
				}
			}
			return true
		}
	}

	// Try scripts/test.sh
	if fileExists("scripts/test.sh") || fileExists("scripts/test") {
		scriptPath := "scripts/test.sh"
		if !fileExists(scriptPath) {
			scriptPath = "scripts/test"
		}
		output, err := exec.Command(scriptPath).CombinedOutput()
		if err != nil {
			ec.Add("scripts/test failed")
			if len(output) > 0 {
				fmt.Fprint(os.Stderr, string(output))
			}
		}
		return true
	}

	return false
}

func makeTargetExists(target string) bool {
	cmd := exec.Command("make", "-n", target)
	cmd.Stdout = nil
	cmd.Stderr = nil
	err := cmd.Run()
	return err == nil
}

func commandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func runLanguageTests(lang string, filePath string, ignorePatterns []string, ec *ErrorCollector) {
	switch lang {
	case "go":
		testGo(filePath, ignorePatterns, ec)
	case "python":
		testPython(filePath, ignorePatterns, ec)
	case "javascript":
		testJavaScript(filePath, ignorePatterns, ec)
	case "rust":
		testRust(filePath, ignorePatterns, ec)
	case "shell":
		testShell(filePath, ignorePatterns, ec)
	}
}

func testGo(filePath string, ignorePatterns []string, ec *ErrorCollector) {
	// Find Go files
	files := findFiles([]string{".go"}, ignorePatterns)
	if len(files) == 0 {
		return
	}

	if !commandExists("go") {
		return
	}

	// Build test command
	args := []string{"test"}
	if isRaceEnabled() {
		args = append(args, "-race")
	}
	args = append(args, "./...")

	// Run tests
	output, err := exec.Command("go", args...).CombinedOutput()
	if err != nil {
		ec.Add("go test failed")
		if len(output) > 0 {
			fmt.Fprint(os.Stderr, string(output))
		}
	}
}

func testPython(filePath string, ignorePatterns []string, ec *ErrorCollector) {
	files := findFiles([]string{".py"}, ignorePatterns)
	if len(files) == 0 {
		return
	}

	// Try pytest first
	if commandExists("pytest") {
		output, err := exec.Command("pytest").CombinedOutput()
		if err != nil {
			ec.Add("pytest failed")
			if len(output) > 0 {
				fmt.Fprint(os.Stderr, string(output))
			}
		}
		return
	}

	// Fall back to unittest
	if commandExists("python") {
		output, err := exec.Command("python", "-m", "unittest", "discover").CombinedOutput()
		if err != nil {
			ec.Add("python unittest failed")
			if len(output) > 0 {
				fmt.Fprint(os.Stderr, string(output))
			}
		}
	}
}

func testJavaScript(filePath string, ignorePatterns []string, ec *ErrorCollector) {
	files := findFiles([]string{".js", ".ts", ".jsx", ".tsx"}, ignorePatterns)
	if len(files) == 0 {
		return
	}

	// Run npm test if package.json exists
	if fileExists("package.json") && commandExists("npm") {
		output, err := exec.Command("npm", "test").CombinedOutput()
		if err != nil {
			ec.Add("npm test failed")
			if len(output) > 0 {
				fmt.Fprint(os.Stderr, string(output))
			}
		}
	}
}

func testRust(filePath string, ignorePatterns []string, ec *ErrorCollector) {
	files := findFiles([]string{".rs"}, ignorePatterns)
	if len(files) == 0 {
		return
	}

	if !commandExists("cargo") {
		return
	}

	// Run cargo test
	output, err := exec.Command("cargo", "test").CombinedOutput()
	if err != nil {
		ec.Add("cargo test failed")
		if len(output) > 0 {
			fmt.Fprint(os.Stderr, string(output))
		}
	}
}

func testShell(filePath string, ignorePatterns []string, ec *ErrorCollector) {
	// Check if edited file is a shell script
	ext := filepath.Ext(filePath)
	if ext != ".sh" && ext != ".bash" {
		return
	}

	// Look for corresponding test file
	base := strings.TrimSuffix(filepath.Base(filePath), ext)
	testFiles := []string{
		filepath.Join(filepath.Dir(filePath), base+"_test.sh"),
		filepath.Join(filepath.Dir(filePath), "test_"+base+".sh"),
	}

	for _, testFile := range testFiles {
		if fileExists(testFile) {
			output, err := exec.Command("bash", testFile).CombinedOutput()
			if err != nil {
				ec.Add(fmt.Sprintf("shell test %s failed", testFile))
				if len(output) > 0 {
					fmt.Fprint(os.Stderr, string(output))
				}
			}
		}
	}
}

func findFiles(extensions []string, ignorePatterns []string) []string {
	files := []string{}

	extMap := make(map[string]bool)
	for _, ext := range extensions {
		extMap[ext] = true
	}

	_ = filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		// Skip hidden directories and common ignore patterns
		if info.IsDir() {
			base := filepath.Base(path)
			if base == ".git" || base == "node_modules" || base == "venv" || base == ".venv" || base == "target" || base == "dist" || base == "build" {
				return filepath.SkipDir
			}
		}

		if !info.IsDir() && extMap[filepath.Ext(path)] {
			if !shouldSkipFile(path, ignorePatterns) {
				files = append(files, path)
			}
		}

		return nil
	})

	return files
}

func exitWithResult(ec *ErrorCollector) error {
	if ec.Count() > 0 {
		fmt.Fprintf(os.Stderr, "\n❌ Tests failed with %d error(s)\n", ec.Count())
		fmt.Fprintf(os.Stderr, "⛔ BLOCKING: Fix ALL test failures above before continuing\n")
		os.Exit(2)
	}

	// Success - exit with code 2 to show continuation message
	fmt.Fprintf(os.Stderr, "✅ All tests passed. Continue with your task.\n")
	os.Exit(2)
	return nil
}
