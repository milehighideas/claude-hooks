package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/milehighideas/claude-hooks/internal/jsonc"
	"github.com/milehighideas/claude-hooks/internal/srp"
)

// CLI flags
var (
	pathFlag    string
	fileFlag    string
	helpFlag    bool
	verboseFlag bool
)

// screenHooksConfig holds the resolved set of hooks to flag in screen files.
// Loaded from .pre-commit.json srpConfig.screenHooks; defaults to useState/useReducer/useContext.
var screenHooksConfig map[string]bool

func loadScreenHooksConfig() {
	defaults := map[string]bool{"useState": true, "useReducer": true, "useContext": true}
	allHooks := []string{"useState", "useReducer", "useContext", "useCallback", "useEffect", "useMemo"}

	screenHooksConfig = defaults

	var raw struct {
		SRPConfig struct {
			ScreenHooks []string `json:"screenHooks"`
		} `json:"srpConfig"`
	}
	if err := jsonc.Unmarshal(".pre-commit.json", &raw); err != nil {
		return
	}

	hooks := raw.SRPConfig.ScreenHooks
	if len(hooks) == 0 {
		return
	}

	result := make(map[string]bool)
	for _, h := range hooks {
		if h == "all" {
			for _, a := range allHooks {
				result[a] = true
			}
		} else {
			result[h] = true
		}
	}
	screenHooksConfig = result
}

// SRPViolation and ASTAnalysis alias the shared internal/srp types so this
// standalone/hook-mode validator and the pre-commit orchestrator run identical
// detection (internal/srp, tree-sitter AST). Detection used to be duplicated
// here as a parallel regex implementation.
type SRPViolation = srp.Violation
type ASTAnalysis = srp.Analysis

// ToolInput represents the input to a tool
type ToolInput struct {
	FilePath string                 `json:"file_path"`
	Content  string                 `json:"content"`
	Command  string                 `json:"command"`
	Extra    map[string]interface{} `json:"-"`
}

// ToolData represents the JSON data from stdin
type ToolData struct {
	ToolName  string                 `json:"tool_name"`
	ToolInput ToolInput              `json:"tool_input"`
	Extra     map[string]interface{} `json:"-"`
}

func init() {
	flag.StringVar(&pathFlag, "path", "", "Directory to check for SRP violations")
	flag.StringVar(&fileFlag, "file", "", "Single file to check for SRP violations")
	flag.BoolVar(&helpFlag, "help", false, "Show help message")
	flag.BoolVar(&helpFlag, "h", false, "Show help message")
	flag.BoolVar(&verboseFlag, "verbose", false, "Show verbose output including passed files")
	flag.BoolVar(&verboseFlag, "v", false, "Show verbose output")
}

func printUsage() {
	fmt.Println("validate-srp - Single Responsibility Principle validator for TypeScript/TSX")
	fmt.Println()
	fmt.Println("USAGE:")
	fmt.Println("  Standalone mode:")
	fmt.Println("    validate-srp --path <directory>    Check all TS/TSX files in directory")
	fmt.Println("    validate-srp --file <file>         Check a single file")
	fmt.Println()
	fmt.Println("  Claude hook mode (reads JSON from stdin):")
	fmt.Println("    echo '{...}' | validate-srp")
	fmt.Println()
	fmt.Println("FLAGS:")
	fmt.Println("  -path <dir>     Directory to recursively check")
	fmt.Println("  -file <file>    Single file to check")
	fmt.Println("  -v, -verbose    Show verbose output (including passed files)")
	fmt.Println("  -h, -help       Show this help message")
	fmt.Println()
	fmt.Println("CHECKS:")
	fmt.Println("  1. Direct Convex imports (must use data-layer)")
	fmt.Println("  2. State in screens (must be in content components)")
	fmt.Println("  3. Multiple exports in CRUD files (one per file)")
	fmt.Println("  4. File size limits (screens: 100, hooks: 150, components: 200)")
	fmt.Println("  5. Type exports location (must be in types/ folder)")
	fmt.Println("  6. Mixed concerns (data + UI + state in same file)")
	fmt.Println()
	fmt.Println("EXIT CODES:")
	fmt.Println("  0 - No violations")
	fmt.Println("  1 - Error running checks")
	fmt.Println("  2 - SRP violations found")
}

func main() {
	flag.Parse()
	loadScreenHooksConfig()

	if helpFlag {
		printUsage()
		os.Exit(0)
	}

	// Standalone mode: check path or file
	if pathFlag != "" || fileFlag != "" {
		os.Exit(runStandalone())
	}

	// Hook mode: read from stdin
	runHookMode()
}

func runStandalone() int {
	var files []string

	if fileFlag != "" {
		// Single file mode
		absPath, err := filepath.Abs(fileFlag)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error resolving path: %v\n", err)
			return 1
		}
		if !fileExists(absPath) {
			fmt.Fprintf(os.Stderr, "File not found: %s\n", absPath)
			return 1
		}
		if !isTypeScriptFile(absPath) {
			fmt.Fprintf(os.Stderr, "Not a TypeScript file: %s\n", absPath)
			return 1
		}
		files = append(files, absPath)
	} else if pathFlag != "" {
		// Directory mode
		absPath, err := filepath.Abs(pathFlag)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error resolving path: %v\n", err)
			return 1
		}

		err = filepath.Walk(absPath, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			// Skip hidden directories and node_modules
			if info.IsDir() {
				name := info.Name()
				if strings.HasPrefix(name, ".") || name == "node_modules" || name == "dist" || name == "build" {
					return filepath.SkipDir
				}
				return nil
			}

			// Only process TypeScript files (not test files)
			if isTypeScriptFile(path) && !strings.Contains(path, ".test.") && !strings.Contains(path, ".spec.") {
				files = append(files, path)
			}

			return nil
		})

		if err != nil {
			fmt.Fprintf(os.Stderr, "Error walking directory: %v\n", err)
			return 1
		}
	}

	if len(files) == 0 {
		fmt.Println("No TypeScript files found to check")
		return 0
	}

	fmt.Printf("Checking %d TypeScript file(s) for SRP compliance...\n\n", len(files))

	var allErrors, allWarnings []struct {
		file      string
		violation SRPViolation
	}
	filesChecked := 0

	for _, file := range files {
		content, err := os.ReadFile(file)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading %s: %v\n", file, err)
			continue
		}

		analysis := analyzeCode(string(content), file)
		violations := validateSRPCompliance(analysis, file)

		filesChecked++

		for _, v := range violations {
			if v.Severity == "error" {
				allErrors = append(allErrors, struct {
					file      string
					violation SRPViolation
				}{file, v})
			} else {
				allWarnings = append(allWarnings, struct {
					file      string
					violation SRPViolation
				}{file, v})
			}
		}

		if verboseFlag && len(violations) == 0 {
			fmt.Printf("✅ %s\n", file)
		}
	}

	// Print summary
	fmt.Println(strings.Repeat("=", 60))
	fmt.Println("  SRP CHECK RESULTS")
	fmt.Println(strings.Repeat("=", 60))

	if len(allWarnings) > 0 {
		fmt.Printf("\n⚠️  WARNINGS (%d):\n", len(allWarnings))
		for _, w := range allWarnings {
			fmt.Printf("\n  %s:\n", w.file)
			fmt.Printf("    %s\n", w.violation.Message)
			if w.violation.Suggestion != "" {
				fmt.Printf("    → %s\n", w.violation.Suggestion)
			}
		}
	}

	if len(allErrors) > 0 {
		fmt.Printf("\n❌ ERRORS (%d):\n", len(allErrors))
		for _, e := range allErrors {
			fmt.Printf("\n  %s:\n", e.file)
			fmt.Printf("    %s\n", e.violation.Message)
			if e.violation.Suggestion != "" {
				fmt.Printf("    FIX: %s\n", e.violation.Suggestion)
			}
		}
	}

	fmt.Println()
	fmt.Printf("Files checked: %d\n", filesChecked)
	fmt.Printf("Errors: %d, Warnings: %d\n", len(allErrors), len(allWarnings))

	if len(allErrors) > 0 {
		fmt.Println("\n❌ SRP check failed")
		return 2
	}

	fmt.Println("\n✅ SRP check passed")
	return 0
}

func runHookMode() {
	// Load project config
	loadProjectConfig()

	// OPT-IN ONLY: Validation only runs if project explicitly enables it
	if os.Getenv("CLAUDE_HOOKS_AST_VALIDATION") != "true" {
		os.Exit(0)
	}

	// Read JSON from stdin
	var data ToolData
	decoder := json.NewDecoder(os.Stdin)
	if err := decoder.Decode(&data); err != nil {
		// Not valid JSON or empty - skip validation
		os.Exit(0)
	}

	// Only validate on TypeScript file operations
	isTS, filePath, content := isComponentWriteOperation(data)
	if !isTS {
		os.Exit(0)
	}

	// Analyze the file
	var analysis *ASTAnalysis
	if content != "" {
		analysis = analyzeCode(content, filePath)
	} else if fileExists(filePath) {
		fileContent, err := os.ReadFile(filePath)
		if err != nil {
			os.Exit(0)
		}
		analysis = analyzeCode(string(fileContent), filePath)
	} else {
		os.Exit(0)
	}

	if analysis == nil {
		os.Exit(0)
	}

	// Run SRP validators
	violations := validateSRPCompliance(analysis, filePath)

	// Separate errors and warnings
	var errors, warnings []SRPViolation
	for _, v := range violations {
		if v.Severity == "error" {
			errors = append(errors, v)
		} else {
			warnings = append(warnings, v)
		}
	}

	if len(errors) > 0 {
		msg := fmt.Sprintf("\n❌ BLOCKED: SRP violation in %s\n", filepath.Base(filePath))
		msg += strings.Repeat("=", 60) + "\n"
		for _, v := range errors {
			msg += fmt.Sprintf("\n  ✗ %s\n", v.Message)
			if v.Suggestion != "" {
				msg += fmt.Sprintf("    FIX: %s\n", v.Suggestion)
			}
		}
		msg += "\n" + strings.Repeat("=", 60) + "\n"
		msg += "HOW TO FIX:\n"
		msg += "  1. Move direct Convex imports to data-layer hooks\n"
		msg += "  2. Move state (useState) from screens to content components\n"
		msg += "  3. Split files with multiple exports into separate files\n"
		msg += "  4. Move 'export type' definitions to types/ folder\n"
		msg += "\nSee: ~/.claude/skills/frontend-architecture/SKILL.md\n"
		fmt.Fprint(os.Stderr, msg)
		os.Exit(2)
	}

	if len(warnings) > 0 {
		msg := fmt.Sprintf("\n⚠️  SRP Warnings for %s:\n", filepath.Base(filePath))
		for _, v := range warnings {
			msg += fmt.Sprintf("\n  %s", v.Message)
			if v.Suggestion != "" {
				msg += fmt.Sprintf("\n  → %s", v.Suggestion)
			}
		}
		msg += "\n"
		fmt.Fprint(os.Stderr, msg)
	}

	os.Exit(0)
}

func loadProjectConfig() {
	configFile := filepath.Join(getCurrentDir(), ".claude-hooks-config.sh")
	if !fileExists(configFile) {
		return
	}

	cmd := exec.Command("bash", "-c", fmt.Sprintf("source %s && env", configFile))
	output, err := cmd.Output()
	if err != nil {
		return
	}

	// Parse environment variables
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		line := scanner.Text()
		if idx := strings.Index(line, "="); idx > 0 {
			key := line[:idx]
			value := line[idx+1:]
			_ = os.Setenv(key, value)
		}
	}
}

func getCurrentDir() string {
	dir, err := os.Getwd()
	if err != nil {
		return "."
	}
	return dir
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func isTypeScriptFile(filePath string) bool {
	return (strings.HasSuffix(filePath, ".tsx") || strings.HasSuffix(filePath, ".ts")) &&
		!strings.HasSuffix(filePath, ".d.ts")
}

func extractBashFileWrite(command string) (string, string) {
	// Pattern for heredoc: cat > file.tsx << 'EOF' ... EOF
	// Since Go doesn't support backreferences, we'll match and manually verify
	heredocRe := regexp.MustCompile(`(?s)cat\s*>\s*([^\s<]+\.tsx?)\s*<<\s*['"]?(\w+)['"]?\s*\n(.*?)\n(\w+)`)
	if matches := heredocRe.FindStringSubmatch(command); matches != nil {
		delimiter := matches[2]
		endDelimiter := matches[4]
		if delimiter == endDelimiter {
			return matches[1], matches[3]
		}
	}

	// Pattern for echo redirect: echo "..." > file.tsx
	echoRe := regexp.MustCompile(`echo\s+['"](.+?)['"]\s*>\s*([^\s]+\.tsx?)`)
	if matches := echoRe.FindStringSubmatch(command); matches != nil {
		return matches[2], matches[1]
	}

	// Pattern for tee: tee file.tsx << 'EOF'
	teeRe := regexp.MustCompile(`(?s)tee\s+([^\s<]+\.tsx?)\s*<<\s*['"]?(\w+)['"]?\s*\n(.*?)\n(\w+)`)
	if matches := teeRe.FindStringSubmatch(command); matches != nil {
		delimiter := matches[2]
		endDelimiter := matches[4]
		if delimiter == endDelimiter {
			return matches[1], matches[3]
		}
	}

	return "", ""
}

func isComponentWriteOperation(data ToolData) (bool, string, string) {
	toolName := data.ToolName
	toolInput := data.ToolInput

	// Handle Write and Edit tools
	if toolName == "Write" || toolName == "Edit" {
		filePath := toolInput.FilePath
		content := toolInput.Content
		if isTypeScriptFile(filePath) {
			return true, filePath, content
		}
		return false, "", ""
	}

	// Handle Bash tool - detect file writes
	if toolName == "Bash" {
		command := toolInput.Command
		filePath, content := extractBashFileWrite(command)
		if filePath != "" && isTypeScriptFile(filePath) {
			return true, filePath, content
		}
		return false, "", ""
	}

	return false, "", ""
}

// analyzeCode parses a TS/TSX file via the shared tree-sitter analyzer.
func analyzeCode(code, filePath string) *ASTAnalysis {
	return srp.Analyze(code, filePath)
}

// validateSRPCompliance runs the shared SRP detectors. The
// CLAUDE_HOOKS_AST_VALIDATION=false escape hatch disables all checks.
func validateSRPCompliance(analysis *ASTAnalysis, filePath string) []SRPViolation {
	if os.Getenv("CLAUDE_HOOKS_AST_VALIDATION") == "false" {
		return nil
	}
	return srp.RunDetectors(analysis, filePath, srp.Options{ScreenHooks: screenHooksConfig})
}
