package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// SRPViolation represents a Single Responsibility Principle violation
type SRPViolation struct {
	File       string
	Severity   string // "error" or "warning"
	Message    string
	Suggestion string
}

// SRPAnalysis represents the analysis of a TypeScript/TSX file
type SRPAnalysis struct {
	FilePath                 string
	Imports                  []SRPImportInfo
	Exports                  []SRPExportInfo
	StateManagement          []SRPStateInfo
	LineCount                int
	HasResponsibilityComment bool
}

// SRPImportInfo represents an import statement
type SRPImportInfo struct {
	Source string
	Names  []string
}

// SRPExportInfo represents an export statement
type SRPExportInfo struct {
	Name       string
	Type       string
	IsTypeOnly bool
	Source     string // For re-exports: the source module (e.g., "./types/foo")
}

// SRPStateInfo represents state management usage
type SRPStateInfo struct {
	Hook string
	Line int
}

// SRPChecker validates Single Responsibility Principle compliance
type SRPChecker struct {
	gitShowFunc   func(file string) ([]byte, error)
	readFileFunc  func(file string) ([]byte, error)
	useFilesystem bool
	config        SRPConfig
}

// NewSRPChecker creates a new SRP checker that reads from git staged content
func NewSRPChecker(config SRPConfig) *SRPChecker {
	return &SRPChecker{
		gitShowFunc: func(file string) ([]byte, error) {
			cmd := exec.Command("git", "show", ":"+file)
			return cmd.Output()
		},
		readFileFunc: func(file string) ([]byte, error) {
			return os.ReadFile(file)
		},
		useFilesystem: false,
		config:        config,
	}
}

// NewSRPCheckerFullMode creates a new SRP checker that reads from filesystem
func NewSRPCheckerFullMode(config SRPConfig) *SRPChecker {
	return &SRPChecker{
		gitShowFunc: func(file string) ([]byte, error) {
			cmd := exec.Command("git", "show", ":"+file)
			return cmd.Output()
		},
		readFileFunc: func(file string) ([]byte, error) {
			return os.ReadFile(file)
		},
		useFilesystem: true,
		config:        config,
	}
}

// CheckFiles validates SRP compliance for TypeScript files
func (c *SRPChecker) CheckFiles(files []string) ([]SRPViolation, error) {
	var allViolations []SRPViolation

	for _, file := range files {
		if !c.isTypeScriptFile(file) {
			continue
		}

		var content []byte
		var err error
		if c.useFilesystem {
			content, err = c.readFileFunc(file)
		} else {
			content, err = c.gitShowFunc(file)
		}
		if err != nil {
			continue
		}

		analysis := c.analyzeCode(string(content), file)
		violations := c.validateSRPCompliance(analysis, file)
		allViolations = append(allViolations, violations...)
	}

	return allViolations, nil
}

func (c *SRPChecker) isTypeScriptFile(filePath string) bool {
	return (strings.HasSuffix(filePath, ".tsx") || strings.HasSuffix(filePath, ".ts")) &&
		!strings.HasSuffix(filePath, ".d.ts") &&
		!strings.HasSuffix(filePath, ".test.ts") &&
		!strings.HasSuffix(filePath, ".test.tsx") &&
		!strings.HasSuffix(filePath, ".spec.ts") &&
		!strings.HasSuffix(filePath, ".spec.tsx")
}

func (c *SRPChecker) analyzeCode(code, filePath string) *SRPAnalysis {
	analysis := &SRPAnalysis{
		FilePath:  filePath,
		Imports:   []SRPImportInfo{},
		Exports:   []SRPExportInfo{},
		LineCount: strings.Count(code, "\n") + 1,
	}

	lines := strings.Split(code, "\n")

	importRe := regexp.MustCompile(`^import\s+(?:{([^}]+)}|(\w+))?\s*(?:,\s*{([^}]+)})?\s*from\s+['"]([^'"]+)['"]`)
	exportRe := regexp.MustCompile(`^export\s+(?:(type|interface)\s+)?(?:(const|let|var|function|class|default)\s+)?(\w+)`)
	exportTypeRe := regexp.MustCompile(`^export\s+type\s+`)
	exportFromRe := regexp.MustCompile(`from\s+['"]([^'"]+)['"]`)
	stateHookRe := regexp.MustCompile(`\b(useState|useReducer|useContext|useCallback|useEffect|useMemo)\s*\(`)

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Check for imports
		if matches := importRe.FindStringSubmatch(trimmed); matches != nil {
			source := matches[4]
			var names []string

			if matches[1] != "" {
				parts := strings.Split(matches[1], ",")
				for _, part := range parts {
					name := strings.TrimSpace(part)
					if name != "" {
						names = append(names, name)
					}
				}
			}

			if matches[2] != "" {
				names = append(names, strings.TrimSpace(matches[2]))
			}

			if matches[3] != "" {
				parts := strings.Split(matches[3], ",")
				for _, part := range parts {
					name := strings.TrimSpace(part)
					if name != "" {
						names = append(names, name)
					}
				}
			}

			analysis.Imports = append(analysis.Imports, SRPImportInfo{
				Source: source,
				Names:  names,
			})
		}

		// Check for exports
		if strings.HasPrefix(trimmed, "export ") {
			isTypeOnly := exportTypeRe.MatchString(trimmed)

			// Check if this is a re-export (export ... from '...')
			var source string
			if fromMatches := exportFromRe.FindStringSubmatch(trimmed); fromMatches != nil {
				source = fromMatches[1]
			}

			if matches := exportRe.FindStringSubmatch(trimmed); matches != nil {
				exportType := matches[1]
				if exportType == "" {
					exportType = matches[2]
				}
				name := matches[3]

				analysis.Exports = append(analysis.Exports, SRPExportInfo{
					Name:       name,
					Type:       exportType,
					IsTypeOnly: isTypeOnly,
					Source:     source,
				})
			}
		}

		// Check for state management hooks
		if stateHookRe.MatchString(trimmed) {
			matches := stateHookRe.FindStringSubmatch(trimmed)
			if len(matches) > 1 {
				analysis.StateManagement = append(analysis.StateManagement, SRPStateInfo{
					Hook: matches[1],
					Line: i + 1,
				})
			}
		}
	}

	return analysis
}

func (c *SRPChecker) validateSRPCompliance(analysis *SRPAnalysis, filePath string) []SRPViolation {
	var violations []SRPViolation

	violations = append(violations, c.checkDirectConvexImports(analysis, filePath)...)
	violations = append(violations, c.checkStateInScreens(analysis, filePath)...)
	violations = append(violations, c.checkMultipleExports(analysis, filePath)...)
	violations = append(violations, c.checkFileSize(analysis, filePath)...)
	violations = append(violations, c.checkTypeExportsLocation(analysis, filePath)...)
	violations = append(violations, c.checkMixedConcerns(analysis, filePath)...)

	return violations
}

func (c *SRPChecker) checkDirectConvexImports(analysis *SRPAnalysis, filePath string) []SRPViolation {
	var violations []SRPViolation

	// Skip allowed paths
	if strings.Contains(filePath, "/data-layer/") ||
		strings.Contains(filePath, "/backend/") ||
		strings.Contains(filePath, "/convex/") ||
		strings.Contains(filePath, "/scripts/") ||
		strings.Contains(filePath, "/providers/") ||
		strings.HasSuffix(filePath, "_layout.tsx") {
		return violations
	}

	allowedImports := map[string]bool{
		"Preloaded":         true,
		"usePreloadedQuery": true,
	}

	// Allowed types from dataModel (these have no data-layer alternatives)
	allowedDataModelTypes := map[string]bool{
		"Id":  true,
		"Doc": true,
	}

	for _, imp := range analysis.Imports {
		if imp.Source == "convex/react" {
			for _, name := range imp.Names {
				if !allowedImports[name] {
					violations = append(violations, SRPViolation{
						File:       filePath,
						Severity:   "error",
						Message:    "Direct Convex imports forbidden outside data-layer",
						Suggestion: "Use data-layer hooks instead",
					})
					break
				}
			}
		}

		if strings.Contains(imp.Source, "_generated/api") {
			violations = append(violations, SRPViolation{
				File:       filePath,
				Severity:   "error",
				Message:    "Direct Convex API imports forbidden outside data-layer",
				Suggestion: "Use data-layer hooks instead",
			})
		}

		// Check _generated/dataModel imports - only allow Id and Doc
		if strings.Contains(imp.Source, "_generated/dataModel") {
			for _, name := range imp.Names {
				// Strip "type " prefix if present
				cleanName := strings.TrimPrefix(name, "type ")
				if !allowedDataModelTypes[cleanName] {
					violations = append(violations, SRPViolation{
						File:       filePath,
						Severity:   "error",
						Message:    fmt.Sprintf("Only Id and Doc types allowed from _generated/dataModel, found: %s", name),
						Suggestion: "Use data-layer types instead, or import only Id/Doc",
					})
					break
				}
			}
		}
	}

	return violations
}

// isScreenOrPage returns true if the file is a mobile screen or Next.js page
// These should be thin routing layers without state management
func (c *SRPChecker) isScreenOrPage(filePath string) bool {
	return strings.Contains(filePath, "/screens/") || strings.HasSuffix(filePath, "page.tsx")
}

func (c *SRPChecker) checkStateInScreens(analysis *SRPAnalysis, filePath string) []SRPViolation {
	var violations []SRPViolation

	if !c.isScreenOrPage(filePath) {
		return violations
	}

	allowedHooks := c.config.resolvedScreenHooks()

	var flaggedHooks []string
	for _, s := range analysis.StateManagement {
		if allowedHooks[s.Hook] {
			flaggedHooks = append(flaggedHooks, s.Hook)
		}
	}

	if len(flaggedHooks) > 0 {
		fileType := "Screen"
		if strings.HasSuffix(filePath, "page.tsx") {
			fileType = "Page"
		}

		violations = append(violations, SRPViolation{
			File:       filePath,
			Severity:   "error",
			Message:    fmt.Sprintf("%s has state management (%s)", fileType, strings.Join(flaggedHooks, ", ")),
			Suggestion: "Move state to content component or hook - screens are navigation-only",
		})
	}

	return violations
}

func (c *SRPChecker) checkMultipleExports(analysis *SRPAnalysis, filePath string) []SRPViolation {
	var violations []SRPViolation

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

	// Count non-type exports
	nonTypeExports := 0
	for _, exp := range analysis.Exports {
		if !exp.IsTypeOnly && exp.Type != "type" && exp.Type != "interface" {
			nonTypeExports++
		}
	}

	if nonTypeExports > 1 {
		violations = append(violations, SRPViolation{
			File:       filePath,
			Severity:   "error",
			Message:    fmt.Sprintf("Multiple exports (%d) in CRUD component", nonTypeExports),
			Suggestion: "Split into separate files (one component per file)",
		})
	}

	return violations
}

func (c *SRPChecker) checkFileSize(analysis *SRPAnalysis, filePath string) []SRPViolation {
	var violations []SRPViolation

	if strings.Contains(filePath, "/scripts/") {
		return violations
	}

	limits := map[string]int{
		"screen":    100,
		"hook":      150,
		"component": 200,
	}

	lineCount := analysis.LineCount

	if c.isScreenOrPage(filePath) && lineCount > limits["screen"] {
		fileType := "Screen"
		if strings.HasSuffix(filePath, "page.tsx") {
			fileType = "Page"
		}
		violations = append(violations, SRPViolation{
			File:       filePath,
			Severity:   "warning",
			Message:    fmt.Sprintf("%s file is %d lines (limit: %d)", fileType, lineCount, limits["screen"]),
			Suggestion: "Move logic to content component",
		})
	} else if strings.Contains(filePath, "/hooks/") && lineCount > limits["hook"] {
		violations = append(violations, SRPViolation{
			File:       filePath,
			Severity:   "warning",
			Message:    fmt.Sprintf("Hook file is %d lines (limit: %d)", lineCount, limits["hook"]),
			Suggestion: "Split into smaller hooks",
		})
	} else if lineCount > limits["component"] {
		violations = append(violations, SRPViolation{
			File:       filePath,
			Severity:   "warning",
			Message:    fmt.Sprintf("File is %d lines (limit: %d)", lineCount, limits["component"]),
			Suggestion: "Consider splitting",
		})
	}

	return violations
}

func (c *SRPChecker) checkTypeExportsLocation(analysis *SRPAnalysis, filePath string) []SRPViolation {
	var violations []SRPViolation

	// Skip type definition folders, declaration files, auto-generated data-layer files,
	// and shared UI packages (which commonly export props types alongside components)
	if strings.Contains(filePath, "/types/") ||
		strings.HasSuffix(filePath, ".d.ts") ||
		strings.Contains(filePath, "/generated-types/") ||
		strings.Contains(filePath, "/data-layer/") ||
		strings.Contains(filePath, "packages/ui/") ||
		strings.Contains(filePath, "packages/mobile-ui/") {
		return violations
	}

	for _, exp := range analysis.Exports {
		if exp.IsTypeOnly || exp.Type == "type" || exp.Type == "interface" {
			// Skip Props types - these are idiomatic to export alongside React components
			if strings.HasSuffix(exp.Name, "Props") {
				continue
			}
			violations = append(violations, SRPViolation{
				File:       filePath,
				Severity:   "error",
				Message:    fmt.Sprintf("Type export '%s' found outside types/ folder", exp.Name),
				Suggestion: "Move type definitions to types/ folder",
			})
		}
	}

	return violations
}

func (c *SRPChecker) checkMixedConcerns(analysis *SRPAnalysis, filePath string) []SRPViolation {
	var violations []SRPViolation

	hasDataLayer := false
	hasUI := false
	// Mixed concerns uses the original 3 state hooks (useState/useReducer/useContext)
	// regardless of screenHooks config — useCallback/useEffect/useMemo aren't "state"
	stateHooks := map[string]bool{"useState": true, "useReducer": true, "useContext": true}
	hasState := false
	for _, s := range analysis.StateManagement {
		if stateHooks[s.Hook] {
			hasState = true
			break
		}
	}

	for _, imp := range analysis.Imports {
		if strings.Contains(imp.Source, "data-layer") {
			hasDataLayer = true
		}
		if strings.Contains(imp.Source, "@/components/ui") ||
			strings.Contains(imp.Source, "../ui/") ||
			strings.Contains(imp.Source, "@dashtag/ui") ||
			strings.Contains(imp.Source, "@dashtag/mobile-ui") {
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
			File:       filePath,
			Severity:   "error",
			Message:    fmt.Sprintf("File mixes multiple concerns: %s", strings.Join(concerns, ", ")),
			Suggestion: "Separate data fetching (hooks), state (hooks/content), and UI (components)",
		})
	}

	return violations
}

// getSRPAppNameFromPath extracts the app name from a file path
// e.g., "apps/mobile/components/foo.tsx" -> "mobile"
// e.g., "packages/backend/convex/foo.ts" -> "backend"
func getSRPAppNameFromPath(filePath string) string {
	parts := strings.Split(filePath, "/")
	if len(parts) >= 2 {
		if parts[0] == "apps" || parts[0] == "packages" {
			return parts[1]
		}
	}
	return "other"
}

// writeSRPReport writes SRP findings to per-app report files
func writeSRPReport(errors, warnings []SRPViolation, baseDir string) error {
	srpDir := filepath.Join(baseDir, "srp")
	if err := os.MkdirAll(srpDir, 0755); err != nil {
		return err
	}

	// Group errors and warnings by app
	errorsByApp := make(map[string][]SRPViolation)
	warningsByApp := make(map[string][]SRPViolation)

	for _, e := range errors {
		app := getSRPAppNameFromPath(e.File)
		errorsByApp[app] = append(errorsByApp[app], e)
	}
	for _, w := range warnings {
		app := getSRPAppNameFromPath(w.File)
		warningsByApp[app] = append(warningsByApp[app], w)
	}

	// Get all unique app names
	allApps := make(map[string]bool)
	for app := range errorsByApp {
		allApps[app] = true
	}
	for app := range warningsByApp {
		allApps[app] = true
	}

	// Write a separate report file for each app
	for app := range allApps {
		appErrors := errorsByApp[app]
		appWarnings := warningsByApp[app]

		reportPath := filepath.Join(srpDir, app+".txt")

		var sb strings.Builder
		sb.WriteString(strings.Repeat("=", 80) + "\n")
		sb.WriteString(fmt.Sprintf("SRP ANALYSIS - %s\n", strings.ToUpper(app)))
		sb.WriteString(fmt.Sprintf("Generated: %s\n", time.Now().Format("2006-01-02 15:04:05")))
		sb.WriteString(strings.Repeat("=", 80) + "\n\n")

		sb.WriteString(fmt.Sprintf("Total errors: %d\n", len(appErrors)))
		sb.WriteString(fmt.Sprintf("Total warnings: %d\n\n", len(appWarnings)))

		// Group errors by type within this app
		if len(appErrors) > 0 {
			errorsByType := make(map[string][]SRPViolation)
			for _, e := range appErrors {
				errorsByType[e.Message] = append(errorsByType[e.Message], e)
			}

			sb.WriteString(strings.Repeat("-", 40) + "\n")
			sb.WriteString("ERRORS BY TYPE\n")
			sb.WriteString(strings.Repeat("-", 40) + "\n\n")

			for msgType, errs := range errorsByType {
				sb.WriteString(fmt.Sprintf("\n%s (%d occurrences)\n", msgType, len(errs)))
				for _, e := range errs {
					sb.WriteString(fmt.Sprintf("  %s\n", e.File))
				}
			}
		}

		// Group warnings by type within this app
		if len(appWarnings) > 0 {
			warningsByType := make(map[string][]SRPViolation)
			for _, w := range appWarnings {
				warningsByType[w.Message] = append(warningsByType[w.Message], w)
			}

			sb.WriteString("\n" + strings.Repeat("-", 40) + "\n")
			sb.WriteString("WARNINGS BY TYPE\n")
			sb.WriteString(strings.Repeat("-", 40) + "\n\n")

			for msgType, warns := range warningsByType {
				sb.WriteString(fmt.Sprintf("\n%s (%d occurrences)\n", msgType, len(warns)))
				for _, w := range warns {
					sb.WriteString(fmt.Sprintf("  %s\n", w.File))
				}
			}
		}

		// All issues by file within this app
		sb.WriteString("\n" + strings.Repeat("-", 40) + "\n")
		sb.WriteString("ISSUES BY FILE\n")
		sb.WriteString(strings.Repeat("-", 40) + "\n\n")

		allByFile := make(map[string][]SRPViolation)
		for _, e := range appErrors {
			allByFile[e.File] = append(allByFile[e.File], e)
		}
		for _, w := range appWarnings {
			allByFile[w.File] = append(allByFile[w.File], w)
		}

		for file, violations := range allByFile {
			sb.WriteString(fmt.Sprintf("\n%s (%d issues)\n", file, len(violations)))
			for _, v := range violations {
				prefix := "❌"
				if v.Severity == "warning" {
					prefix = "⚠️"
				}
				sb.WriteString(fmt.Sprintf("  %s %s\n", prefix, v.Message))
				if v.Suggestion != "" {
					sb.WriteString(fmt.Sprintf("     → %s\n", v.Suggestion))
				}
			}
		}

		if err := os.WriteFile(reportPath, []byte(sb.String()), 0644); err != nil {
			return err
		}
	}

	return nil
}

// runSRPCheck is the entry point for SRP checking (uses default config)
func runSRPCheck(stagedFiles []string) error {
	return runSRPCheckWithFilter(SRPFilterResult{Files: stagedFiles}, SRPConfig{}, false)
}

// runSRPCheckWithFilter runs SRP check with filter information displayed
func runSRPCheckWithFilter(filterResult SRPFilterResult, config SRPConfig, fullMode bool) error {
	if !compactMode() {
		fmt.Println("================================")
		fmt.Println("  SRP COMPLIANCE CHECK")
		fmt.Println("================================")

		// Print filter information if files were skipped
		totalSkipped := filterResult.SkippedByAppPath + filterResult.SkippedByExclude
		if totalSkipped > 0 || len(config.AppPaths) > 0 {
			fmt.Printf("ℹ️  Checking SRP in: %v\n", config.AppPaths)

			if filterResult.SkippedByAppPath > 0 {
				fmt.Printf("   • %d file(s) outside these paths were skipped:\n", filterResult.SkippedByAppPath)
				for skippedPath, count := range filterResult.SkippedPaths {
					fmt.Printf("     - %s (%d files)\n", skippedPath, count)
				}
			}

			if filterResult.SkippedByExclude > 0 {
				fmt.Printf("   • %d file(s) excluded by excludePaths:\n", filterResult.SkippedByExclude)
				for excludePath, count := range filterResult.ExcludeMatches {
					fmt.Printf("     - %q matched %d file(s)\n", excludePath, count)
				}
			}

			fmt.Printf("   • %d file(s) will be checked\n", len(filterResult.Files))
			fmt.Println()
		}
	}

	if len(filterResult.Files) == 0 {
		if compactMode() {
			printStatus("SRP compliance", true, "no files")
		} else {
			fmt.Println("✅ SRP check passed (no files to check after filtering)")
			fmt.Println()
		}
		return nil
	}

	var checker *SRPChecker
	if fullMode {
		if !compactMode() {
			fmt.Println("🔍 Running FULL SRP check (all files in configured paths)")
		}
		checker = NewSRPCheckerFullMode(config)
	} else {
		checker = NewSRPChecker(config)
	}
	violations, err := checker.CheckFiles(filterResult.Files)
	if err != nil {
		return fmt.Errorf("SRP check failed: %w", err)
	}

	var errors, warnings []SRPViolation
	for _, v := range violations {
		if v.Severity == "error" {
			errors = append(errors, v)
		} else {
			warnings = append(warnings, v)
		}
	}

	// Write report if reportDir is set
	if reportDir != "" {
		if err := writeSRPReport(errors, warnings, reportDir); err != nil {
			fmt.Printf("   Warning: failed to write SRP report: %v\n", err)
		}
	}

	if compactMode() {
		if len(errors) > 0 {
			printStatus("SRP compliance", false, fmt.Sprintf("%d errors", len(errors)))
			printReportHint("srp/")
			return fmt.Errorf("SRP violations found")
		}
		printStatus("SRP compliance", true, fmt.Sprintf("%d files", len(filterResult.Files)))
		return nil
	}

	// Verbose output: print individual violations
	if !config.HideWarnings {
		for _, v := range warnings {
			fmt.Printf("⚠️  %s: %s\n", filepath.Base(v.File), v.Message)
			if v.Suggestion != "" {
				fmt.Printf("   → %s\n", v.Suggestion)
			}
		}
	}

	for _, v := range errors {
		fmt.Printf("❌ %s: %s\n", v.File, v.Message)
		if v.Suggestion != "" {
			fmt.Printf("   FIX: %s\n", v.Suggestion)
		}
	}

	if len(errors) > 0 {
		fmt.Printf("\n❌ Found %d SRP violation(s)\n", len(errors))
		fmt.Println()
		return fmt.Errorf("SRP violations found")
	}

	if len(warnings) > 0 && !config.HideWarnings {
		fmt.Printf("\n⚠️  %d warning(s) - consider fixing\n", len(warnings))
	}

	fmt.Println("✅ SRP check passed")
	fmt.Println()
	return nil
}
