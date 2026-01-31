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

	data, err := os.ReadFile(".pre-commit.json")
	if err != nil {
		return
	}

	var raw struct {
		SRPConfig struct {
			ScreenHooks []string `json:"screenHooks"`
		} `json:"srpConfig"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
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

// SRPViolation represents a Single Responsibility Principle violation
type SRPViolation struct {
	Severity   string // "error" or "warning"
	Message    string
	Line       int
	Suggestion string
}

// ASTAnalysis represents the analysis of a TypeScript/TSX file
type ASTAnalysis struct {
	FilePath                 string
	Imports                  []ImportInfo
	Exports                  []ExportInfo
	StateManagement          []StateInfo
	LineCount                int
	HasResponsibilityComment bool
}

// ImportInfo represents an import statement
type ImportInfo struct {
	Source string
	Names  []string
}

// ExportInfo represents an export statement
type ExportInfo struct {
	Name       string
	Type       string // "function", "const", "type", etc.
	IsTypeOnly bool
}

// StateInfo represents state management usage
type StateInfo struct {
	Hook string
	Line int
}

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

func analyzeCode(code, filePath string) *ASTAnalysis {
	analysis := &ASTAnalysis{
		FilePath:  filePath,
		Imports:   []ImportInfo{},
		Exports:   []ExportInfo{},
		LineCount: strings.Count(code, "\n") + 1,
	}

	lines := strings.Split(code, "\n")

	// Regex patterns
	importRe := regexp.MustCompile(`^import\s+(?:{([^}]+)}|(\w+))?\s*(?:,\s*{([^}]+)})?\s*from\s+['"]([^'"]+)['"]`)
	exportRe := regexp.MustCompile(`^export\s+(?:(type|interface)\s+)?(?:(const|let|var|function|class|default)\s+)?(\w+)`)
	exportTypeRe := regexp.MustCompile(`^export\s+type\s+`)
	stateHookRe := regexp.MustCompile(`\b(useState|useReducer|useContext|useCallback|useEffect|useMemo)\s*\(`)
	responsibilityRe := regexp.MustCompile(`/\*\*\s*Single Responsibility:`)

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Check for imports
		if matches := importRe.FindStringSubmatch(trimmed); matches != nil {
			source := matches[4]
			var names []string

			// Named imports
			if matches[1] != "" {
				parts := strings.Split(matches[1], ",")
				for _, part := range parts {
					name := strings.TrimSpace(part)
					if name != "" {
						names = append(names, name)
					}
				}
			}

			// Default import
			if matches[2] != "" {
				names = append(names, strings.TrimSpace(matches[2]))
			}

			// Additional named imports
			if matches[3] != "" {
				parts := strings.Split(matches[3], ",")
				for _, part := range parts {
					name := strings.TrimSpace(part)
					if name != "" {
						names = append(names, name)
					}
				}
			}

			analysis.Imports = append(analysis.Imports, ImportInfo{
				Source: source,
				Names:  names,
			})
		}

		// Check for exports
		if strings.HasPrefix(trimmed, "export ") {
			isTypeOnly := exportTypeRe.MatchString(trimmed)

			if matches := exportRe.FindStringSubmatch(trimmed); matches != nil {
				exportType := matches[1]
				if exportType == "" {
					exportType = matches[2]
				}
				name := matches[3]

				analysis.Exports = append(analysis.Exports, ExportInfo{
					Name:       name,
					Type:       exportType,
					IsTypeOnly: isTypeOnly,
				})
			}
		}

		// Check for state management hooks
		if stateHookRe.MatchString(trimmed) {
			matches := stateHookRe.FindStringSubmatch(trimmed)
			if len(matches) > 1 {
				analysis.StateManagement = append(analysis.StateManagement, StateInfo{
					Hook: matches[1],
					Line: i + 1,
				})
			}
		}

		// Check for responsibility comment
		if responsibilityRe.MatchString(trimmed) {
			analysis.HasResponsibilityComment = true
		}
	}

	return analysis
}

func validateSRPCompliance(analysis *ASTAnalysis, filePath string) []SRPViolation {
	if os.Getenv("CLAUDE_HOOKS_AST_VALIDATION") == "false" {
		return nil
	}

	var violations []SRPViolation

	violations = append(violations, checkDirectConvexImports(analysis, filePath)...)
	violations = append(violations, checkStateInScreens(analysis, filePath)...)
	violations = append(violations, checkMultipleExports(analysis, filePath)...)
	violations = append(violations, checkFileSize(analysis, filePath)...)
	violations = append(violations, checkTypeExportsLocation(analysis, filePath)...)
	violations = append(violations, checkMixedConcerns(analysis, filePath)...)

	return violations
}

func checkDirectConvexImports(analysis *ASTAnalysis, filePath string) []SRPViolation {
	var violations []SRPViolation

	// Skip if this IS the data-layer, backend, convex package, or scripts folder
	// Also skip _layout.tsx files - they're infrastructure that needs direct provider imports
	if strings.Contains(filePath, "/data-layer/") ||
		strings.Contains(filePath, "/backend/") ||
		strings.Contains(filePath, "/convex/") ||
		strings.Contains(filePath, "/scripts/") ||
		strings.Contains(filePath, "/providers/") ||
		strings.HasSuffix(filePath, "_layout.tsx") {
		return violations
	}

	// Check for direct Convex imports
	hasConvexReact := false
	hasGeneratedAPI := false
	hasGeneratedDataModel := false

	for _, imp := range analysis.Imports {
		if imp.Source == "convex/react" {
			hasConvexReact = true
		}
		if strings.Contains(imp.Source, "_generated/api") {
			hasGeneratedAPI = true
		}
		if strings.Contains(imp.Source, "_generated/dataModel") {
			hasGeneratedDataModel = true
		}
	}

	if hasConvexReact || hasGeneratedAPI {
		// Check if it's only using allowed imports
		allowedImports := map[string]bool{
			"Preloaded":         true,
			"usePreloadedQuery": true,
		}

		for _, imp := range analysis.Imports {
			if imp.Source == "convex/react" {
				hasNonAllowed := false
				for _, name := range imp.Names {
					if !allowedImports[name] {
						hasNonAllowed = true
						break
					}
				}

				if hasNonAllowed {
					violations = append(violations, SRPViolation{
						Severity:   "error",
						Message:    "🚨 ARCHITECTURAL VIOLATION: Direct Convex imports forbidden outside data-layer",
						Suggestion: "Use data-layer hooks instead (import from @dashtag/data-layer/generated-hooks)",
					})
					break
				}
			}
		}

		// Check for _generated/api imports (never allowed outside data-layer)
		if hasGeneratedAPI {
			violations = append(violations, SRPViolation{
				Severity:   "error",
				Message:    "🚨 ARCHITECTURAL VIOLATION: Direct Convex imports forbidden outside data-layer",
				Suggestion: "Use data-layer hooks instead (import from @dashtag/data-layer/generated-hooks)",
			})
		}
	}

	// Check for _generated/dataModel imports - only allow Id and Doc type imports
	if hasGeneratedDataModel {
		// Allowed types from dataModel (these have no data-layer alternatives)
		allowedDataModelTypes := map[string]bool{
			"Id":  true,
			"Doc": true,
		}

		for _, imp := range analysis.Imports {
			if strings.Contains(imp.Source, "_generated/dataModel") {
				hasNonAllowed := false
				for _, name := range imp.Names {
					// Strip "type " prefix if present (from "import type { Id }")
					cleanName := strings.TrimPrefix(name, "type ")
					if !allowedDataModelTypes[cleanName] {
						hasNonAllowed = true
						break
					}
				}

				if hasNonAllowed {
					violations = append(violations, SRPViolation{
						Severity:   "error",
						Message:    "🚨 ARCHITECTURAL VIOLATION: Only Id and Doc types allowed from _generated/dataModel",
						Suggestion: "Use data-layer types instead, or import only Id/Doc from _generated/dataModel",
					})
					break
				}
			}
		}
	}

	return violations
}

func checkStateInScreens(analysis *ASTAnalysis, filePath string) []SRPViolation {
	var violations []SRPViolation

	if !strings.Contains(filePath, "/screens/") {
		return violations
	}

	var flaggedHooks []string
	for _, s := range analysis.StateManagement {
		if screenHooksConfig[s.Hook] {
			flaggedHooks = append(flaggedHooks, s.Hook)
		}
	}

	if len(flaggedHooks) > 0 {
		violations = append(violations, SRPViolation{
			Severity:   "error",
			Message:    fmt.Sprintf("Screen has state management (%s)", strings.Join(flaggedHooks, ", ")),
			Suggestion: "Move state to content component or hook - screens are navigation-only",
		})
	}

	return violations
}

func checkMultipleExports(analysis *ASTAnalysis, filePath string) []SRPViolation {
	var violations []SRPViolation

	// Only check CRUD folders
	crudFolders := []string{"/create/", "/read/", "/update/", "/delete/"}
	hasCRUD := false
	for _, folder := range crudFolders {
		if strings.Contains(filePath, folder) {
			hasCRUD = true
			break
		}
	}

	if !hasCRUD {
		return violations
	}

	if len(analysis.Exports) > 1 {
		var names []string
		for _, exp := range analysis.Exports {
			names = append(names, exp.Name)
		}

		violations = append(violations, SRPViolation{
			Severity:   "error",
			Message:    fmt.Sprintf("Multiple exports (%d) in CRUD component: %s", len(analysis.Exports), strings.Join(names, ", ")),
			Suggestion: "Split into separate files (one component per file)",
		})
	}

	return violations
}

func checkFileSize(analysis *ASTAnalysis, filePath string) []SRPViolation {
	var violations []SRPViolation

	// Skip scripts folder
	if strings.Contains(filePath, "/scripts/") {
		return violations
	}

	limits := map[string]int{
		"screen":    100,
		"hook":      150,
		"component": 200,
	}

	lineCount := analysis.LineCount

	if strings.Contains(filePath, "/screens/") && lineCount > limits["screen"] {
		violations = append(violations, SRPViolation{
			Severity:   "warning",
			Message:    fmt.Sprintf("Screen file is %d lines (limit: %d)", lineCount, limits["screen"]),
			Suggestion: "Screens should only handle navigation - move logic to content component",
		})
	} else if strings.Contains(filePath, "/hooks/") && lineCount > limits["hook"] {
		violations = append(violations, SRPViolation{
			Severity:   "warning",
			Message:    fmt.Sprintf("Hook file is %d lines (limit: %d)", lineCount, limits["hook"]),
			Suggestion: "Consider splitting into smaller, focused hooks",
		})
	} else if lineCount > limits["component"] {
		violations = append(violations, SRPViolation{
			Severity:   "warning",
			Message:    fmt.Sprintf("File is %d lines (limit: %d)", lineCount, limits["component"]),
			Suggestion: "Large files likely violate SRP - consider splitting",
		})
	}

	return violations
}

func checkTypeExportsLocation(analysis *ASTAnalysis, filePath string) []SRPViolation {
	var violations []SRPViolation

	// Skip if file is already in types/ folder
	if strings.Contains(filePath, "/types/") {
		return violations
	}

	// Skip type definition files
	if strings.HasSuffix(filePath, ".d.ts") {
		return violations
	}

	// Get type exports
	var typeExports []ExportInfo
	for _, exp := range analysis.Exports {
		if exp.IsTypeOnly || exp.Type == "type" || exp.Type == "interface" {
			typeExports = append(typeExports, exp)
		}
	}

	if len(typeExports) > 0 {
		var names []string
		for _, exp := range typeExports {
			names = append(names, exp.Name)
		}

		violations = append(violations, SRPViolation{
			Severity:   "error",
			Message:    fmt.Sprintf("Type exports found outside types/ folder: %s", strings.Join(names, ", ")),
			Suggestion: "Move type definitions to ../types/ folder for better organization and reusability",
		})
	}

	return violations
}

func checkMixedConcerns(analysis *ASTAnalysis, filePath string) []SRPViolation {
	var violations []SRPViolation

	hasDataLayer := false
	hasUI := false
	hasState := len(analysis.StateManagement) > 0

	for _, imp := range analysis.Imports {
		if strings.Contains(imp.Source, "data-layer") {
			hasDataLayer = true
		}
		if strings.Contains(imp.Source, "@/components/ui") || strings.Contains(imp.Source, "../ui/") {
			hasUI = true
		}
	}

	// If file has all three: data fetching, UI, and state = mixed concerns
	var concerns []string
	if hasDataLayer {
		concerns = append(concerns, "data fetching")
	}
	if hasUI {
		concerns = append(concerns, "UI components")
	}
	if hasState {
		concerns = append(concerns, "state management")
	}

	if len(concerns) >= 3 {
		violations = append(violations, SRPViolation{
			Severity:   "warning",
			Message:    fmt.Sprintf("File mixes multiple concerns: %s", strings.Join(concerns, ", ")),
			Suggestion: "Separate data fetching (hooks), state (hooks/components), and UI (components)",
		})
	}

	return violations
}
