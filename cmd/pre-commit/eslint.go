package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// lintError represents a parsed lint error (works for both ESLint and Oxlint)
type lintError struct {
	filePath string
	line     string
	column   string
	severity string // "error" or "warning"
	message  string
	rule     string
	fullText string
}

// DefaultLintExcludePaths are the path patterns excluded by default (empty - no filtering unless configured)
var DefaultLintExcludePaths = []string{}

// runFilteredLint runs the configured linter and filters out configured errors
func runFilteredLint(appName, appPath string, lf LintFilter) error {
	excludePaths := lf.ExcludePaths
	if excludePaths == nil {
		excludePaths = DefaultLintExcludePaths
	}

	// Determine which linter to use (default to eslint for backwards compatibility)
	linter := lf.Linter
	if linter == "" {
		linter = "eslint"
	}

	var output string
	var err error

	if linter == "oxlint" {
		output, err = runOxlint(appPath)
	} else {
		output, err = runEslint(appPath)
	}

	if err != nil {
		return err
	}

	// Parse and filter errors
	var errors []lintError
	if linter == "oxlint" {
		errors = parseOxlintErrors(output)
	} else {
		errors = parseEslintErrors(output)
	}

	var realErrors []lintError
	for _, e := range errors {
		if shouldFilterLintError(e, lf.Rules, excludePaths, lf.IgnoreWarnings) {
			continue
		}
		realErrors = append(realErrors, e)
	}

	// Print filtered count
	filteredCount := len(errors) - len(realErrors)
	if filteredCount > 0 {
		fmt.Printf("   (filtered %d lint errors)\n", filteredCount)
	}

	// Write report if reportDir is set
	if reportDir != "" {
		if err := writeLintReport(appName, output, errors, realErrors, reportDir); err != nil {
			fmt.Printf("   Warning: failed to write lint report: %v\n", err)
		}
	}

	// Print real errors grouped by file
	if len(realErrors) > 0 {
		fmt.Println()
		currentFile := ""
		for _, e := range realErrors {
			if e.filePath != currentFile {
				if currentFile != "" {
					fmt.Println()
				}
				fmt.Println(e.filePath)
				currentFile = e.filePath
			}
			fmt.Printf("  %s:%s  %s  %s  %s\n", e.line, e.column, e.severity, e.message, e.rule)
		}
		return fmt.Errorf("found %d lint error(s)", len(realErrors))
	}

	return nil
}

// writeLintReport writes lint findings to a report file
func writeLintReport(appName, rawOutput string, allErrors, realErrors []lintError, baseDir string) error {
	lintDir := filepath.Join(baseDir, "lint")
	if err := os.MkdirAll(lintDir, 0755); err != nil {
		return err
	}

	reportPath := filepath.Join(lintDir, fmt.Sprintf("%s.txt", appName))

	var sb strings.Builder
	sb.WriteString(strings.Repeat("=", 80) + "\n")
	sb.WriteString(fmt.Sprintf("LINT REPORT: %s\n", appName))
	sb.WriteString(fmt.Sprintf("Generated: %s\n", time.Now().Format("2006-01-02 15:04:05")))
	sb.WriteString(strings.Repeat("=", 80) + "\n\n")

	// Count by severity
	warningCount := 0
	errorCount := 0
	for _, e := range realErrors {
		if e.severity == "warning" {
			warningCount++
		} else {
			errorCount++
		}
	}

	sb.WriteString(fmt.Sprintf("Total findings: %d (%d errors, %d warnings)\n", len(realErrors), errorCount, warningCount))
	sb.WriteString(fmt.Sprintf("Total parsed: %d\n", len(allErrors)))
	sb.WriteString(fmt.Sprintf("Filtered out: %d\n\n", len(allErrors)-len(realErrors)))

	// Group errors by file
	errorsByFile := make(map[string][]lintError)
	for _, e := range realErrors {
		errorsByFile[e.filePath] = append(errorsByFile[e.filePath], e)
	}

	sb.WriteString(strings.Repeat("=", 80) + "\n")
	sb.WriteString("FINDINGS BY FILE\n")
	sb.WriteString(strings.Repeat("=", 80) + "\n\n")

	for file, errs := range errorsByFile {
		fileWarnings := 0
		fileErrors := 0
		for _, e := range errs {
			if e.severity == "warning" {
				fileWarnings++
			} else {
				fileErrors++
			}
		}
		var severityParts []string
		if fileErrors > 0 {
			severityParts = append(severityParts, fmt.Sprintf("%d errors", fileErrors))
		}
		if fileWarnings > 0 {
			severityParts = append(severityParts, fmt.Sprintf("%d warnings", fileWarnings))
		}
		sb.WriteString(fmt.Sprintf("\n%s (%s)\n", file, strings.Join(severityParts, ", ")))
		sb.WriteString(strings.Repeat("-", 40) + "\n")
		for _, e := range errs {
			sb.WriteString(fmt.Sprintf("  Line %s:%s [%s] %s\n", e.line, e.column, e.rule, e.message))
		}
	}

	// Also write raw output
	sb.WriteString("\n\n" + strings.Repeat("=", 80) + "\n")
	sb.WriteString("RAW OXLINT OUTPUT\n")
	sb.WriteString(strings.Repeat("=", 80) + "\n\n")
	sb.WriteString(rawOutput)

	return os.WriteFile(reportPath, []byte(sb.String()), 0644)
}

// runEslint runs ESLint with --fix
func runEslint(appPath string) (string, error) {
	// Use exec.LookPath to find pnpm in PATH (avoids Go 1.19+ relative path security check)
	pnpmPath, err := exec.LookPath("pnpm")
	if err != nil {
		return "", fmt.Errorf("pnpm not found in PATH: %w", err)
	}

	cmd := exec.Command(pnpmPath, "eslint", "--fix", ".")
	cmd.Dir = appPath
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Run() // Ignore exit code, we'll determine success from filtered errors

	return stdout.String(), nil
}

// runOxlint runs Oxlint with --fix
func runOxlint(appPath string) (string, error) {
	// Create temp file for output
	tmpFile, err := os.CreateTemp("", "oxlint-output-*.txt")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpPath)

	// Run oxlint via shell to avoid Go 1.19+ relative path issues
	// Redirect both stdout and stderr to the temp file
	shellCmd := fmt.Sprintf("oxlint --fix . > %s 2>&1", tmpPath)
	cmd := exec.Command("bash", "-c", shellCmd)
	cmd.Dir = appPath
	cmd.Run() // Ignore exit code, oxlint returns non-zero when errors found

	// Read the output file
	output, err := os.ReadFile(tmpPath)
	if err != nil {
		return "", fmt.Errorf("failed to read oxlint output: %w", err)
	}

	return string(output), nil
}

// parseEslintErrors parses ESLint output into individual errors
// ESLint output format:
//
//	/path/to/file.tsx
//	  10:5  error  'foo' is defined but never used  @typescript-eslint/no-unused-vars
//	  15:10  warning  Unexpected any  @typescript-eslint/no-explicit-any
func parseEslintErrors(output string) []lintError {
	var errors []lintError
	lines := strings.Split(output, "\n")

	// Regex to match error lines: "  10:5  error  message  rule"
	// Format: leading whitespace, line:col, severity, message, rule
	errorLineRe := regexp.MustCompile(`^\s+(\d+):(\d+)\s+(error|warning)\s+(.+?)\s{2,}([@\w/-]+)\s*$`)

	// Regex to match file paths (lines that start with / or drive letter and end with file extension)
	filePathRe := regexp.MustCompile(`^(/[^\s]+|[A-Za-z]:\\[^\s]+)\.(tsx?|jsx?|mjs|cjs)$`)

	currentFile := ""
	for _, line := range lines {
		// Check if this is a file path line
		if filePathRe.MatchString(strings.TrimSpace(line)) {
			currentFile = strings.TrimSpace(line)
			continue
		}

		// Check if this is an error line
		if matches := errorLineRe.FindStringSubmatch(line); matches != nil && currentFile != "" {
			errors = append(errors, lintError{
				filePath: currentFile,
				line:     matches[1],
				column:   matches[2],
				severity: matches[3],
				message:  strings.TrimSpace(matches[4]),
				rule:     matches[5],
				fullText: line,
			})
		}
	}

	return errors
}

// parseOxlintErrors parses Oxlint output into individual errors
// Oxlint output format:
//
//	  x plugin(rule): message
//	   ,-[filepath:line:col]
//
// or for warnings:
//
//	  ! plugin(rule): message
//	   ,-[filepath:line:col]
func parseOxlintErrors(output string) []lintError {
	var errors []lintError
	lines := strings.Split(output, "\n")

	// Regex to match error/warning lines: "  x plugin(rule): message" or "  ! plugin(rule): message"
	errorLineRe := regexp.MustCompile(`^\s+([x!])\s+(\S+)\(([^)]+)\):\s+(.+)$`)

	// Regex to match file location: ",-[filepath:line:col]"
	fileLocRe := regexp.MustCompile(`,-\[([^:]+):(\d+):(\d+)\]`)

	var currentRule, currentMessage, currentSeverity string
	for _, line := range lines {
		// Check if this is an error/warning line
		if match := errorLineRe.FindStringSubmatch(line); match != nil {
			severity := "error"
			if match[1] == "!" {
				severity = "warning"
			}
			currentSeverity = severity
			currentRule = match[2] + "/" + match[3] // e.g., "eslint/no-unused-vars"
			currentMessage = match[4]
			continue
		}

		// Check if this is a file location line
		if currentRule != "" {
			if match := fileLocRe.FindStringSubmatch(line); match != nil {
				errors = append(errors, lintError{
					filePath: match[1],
					line:     match[2],
					column:   match[3],
					severity: currentSeverity,
					message:  currentMessage,
					rule:     currentRule,
					fullText: line,
				})
				currentRule = ""
			}
		}
	}

	return errors
}

// shouldFilterLintError checks if a lint error should be filtered out
func shouldFilterLintError(err lintError, rules, excludePaths []string, ignoreWarnings bool) bool {
	// Filter warnings when configured to only fail on errors
	if ignoreWarnings && err.severity == "warning" {
		return true
	}

	// Filter specific rules
	for _, rule := range rules {
		if err.rule == rule {
			return true
		}
	}

	// Filter ALL errors from excluded paths (test files)
	for _, pattern := range excludePaths {
		if strings.Contains(err.filePath, pattern) {
			return true
		}
	}

	return false
}

// runFilteredLintBuffered runs the configured linter and returns buffered output (for parallel execution)
func runFilteredLintBuffered(appName, appPath string, lf LintFilter) (string, error) {
	var output strings.Builder

	excludePaths := lf.ExcludePaths
	if excludePaths == nil {
		excludePaths = DefaultLintExcludePaths
	}

	// Determine which linter to use (default to eslint for backwards compatibility)
	linter := lf.Linter
	if linter == "" {
		linter = "eslint"
	}

	var lintOutput string
	var err error

	if linter == "oxlint" {
		lintOutput, err = runOxlint(appPath)
	} else {
		lintOutput, err = runEslint(appPath)
	}

	if err != nil {
		return "", err
	}

	// Parse and filter errors
	var errors []lintError
	if linter == "oxlint" {
		errors = parseOxlintErrors(lintOutput)
	} else {
		errors = parseEslintErrors(lintOutput)
	}

	var realErrors []lintError
	for _, e := range errors {
		if shouldFilterLintError(e, lf.Rules, excludePaths, lf.IgnoreWarnings) {
			continue
		}
		realErrors = append(realErrors, e)
	}

	// Print filtered count
	filteredCount := len(errors) - len(realErrors)
	if filteredCount > 0 {
		fmt.Fprintf(&output, "   (filtered %d lint errors)\n", filteredCount)
	}

	// Write report if reportDir is set
	if reportDir != "" {
		if err := writeLintReport(appName, lintOutput, errors, realErrors, reportDir); err != nil {
			fmt.Fprintf(&output, "   Warning: failed to write lint report: %v\n", err)
		}
	}

	// Print real errors grouped by file
	if len(realErrors) > 0 {
		output.WriteString("\n")
		currentFile := ""
		for _, e := range realErrors {
			if e.filePath != currentFile {
				if currentFile != "" {
					output.WriteString("\n")
				}
				output.WriteString(e.filePath)
				output.WriteString("\n")
				currentFile = e.filePath
			}
			fmt.Fprintf(&output, "  %s:%s  %s  %s  %s\n", e.line, e.column, e.severity, e.message, e.rule)
		}
		return output.String(), fmt.Errorf("found %d lint error(s)", len(realErrors))
	}

	return output.String(), nil
}
