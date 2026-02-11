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

// ConsoleChecker checks for console.* statements in staged files
type ConsoleChecker struct {
	// gitShowFunc allows injecting a mock for testing
	gitShowFunc func(file string) ([]byte, error)
}

// NewConsoleChecker creates a new ConsoleChecker with default git show behavior
func NewConsoleChecker() *ConsoleChecker {
	return &ConsoleChecker{
		gitShowFunc: defaultGitShow,
	}
}

// defaultGitShow retrieves file content from git staging area,
// or from disk when running in standalone mode.
func defaultGitShow(file string) ([]byte, error) {
	if standalone {
		return os.ReadFile(file)
	}
	cmd := exec.Command("git", "show", ":"+file)
	return cmd.Output()
}

// Check checks for console.* statements in the given files
// Returns an error if any violations are found
func (c *ConsoleChecker) Check(appName string, files []string, allowedFiles []string) error {
	fmt.Printf("🔍 Checking for console.* statements in %s app...\n", appName)

	var violations []string

	for _, file := range files {
		// Skip allowed files
		if c.isAllowedFile(file, allowedFiles) {
			continue
		}

		// Only check TypeScript/JavaScript files
		if !c.isCheckableFile(file) {
			continue
		}

		// Get file content from git staging area
		output, err := c.gitShowFunc(file)
		if err != nil {
			continue
		}

		// Check for console.* statements
		if c.hasConsoleStatements(output) {
			violations = append(violations, file)
			fmt.Printf("  ❌ %s\n", file)
		}
	}

	if len(violations) > 0 {
		fmt.Printf("\n❌ Found console.* statements in %d file(s)\n", len(violations))
		fmt.Println("💡 Use a proper logger instead")
		fmt.Println()
		return fmt.Errorf("console statements found")
	}

	fmt.Println("✅ No console.* statements found")
	return nil
}

// isAllowedFile checks if a file matches any pattern in the allowed list
// Patterns use substring matching (e.g., "scripts/" matches any file in scripts directory)
func (c *ConsoleChecker) isAllowedFile(file string, allowedFiles []string) bool {
	for _, pattern := range allowedFiles {
		if strings.Contains(file, pattern) {
			return true
		}
	}
	return false
}

// isCheckableFile returns true for TypeScript/JavaScript files
func (c *ConsoleChecker) isCheckableFile(file string) bool {
	return strings.HasSuffix(file, ".ts") ||
		strings.HasSuffix(file, ".tsx") ||
		strings.HasSuffix(file, ".js") ||
		strings.HasSuffix(file, ".jsx")
}

// hasConsoleStatements checks if content contains console.* statements
func (c *ConsoleChecker) hasConsoleStatements(content []byte) bool {
	pattern := regexp.MustCompile(`console\.(log|warn|error|info|debug)\(`)
	return pattern.Match(content)
}

// checkConsoleStatements is the original function signature for backward compatibility
func checkConsoleStatements(appName string, files []string, allowedFiles []string) error {
	checker := NewConsoleChecker()
	return checker.Check(appName, files, allowedFiles)
}

// ConsoleViolation represents a console statement violation
type ConsoleViolation struct {
	AppName string
	File    string
}

// runConsoleCheck orchestrates console statement checking for all affected apps
func runConsoleCheck(appFiles map[string][]string, allowedFiles []string) error {
	var allViolations []ConsoleViolation

	for appName, files := range appFiles {
		if len(files) > 0 {
			if !compactMode() {
				fmt.Println("================================")
				fmt.Println("  CONSOLE STATEMENT CHECK")
				fmt.Println("================================")
			}
			violations := checkConsoleStatementsWithViolations(appName, files, allowedFiles)
			allViolations = append(allViolations, violations...)
			if !compactMode() {
				fmt.Println()
			}
		}
	}

	// Write report if reportDir is set
	if reportDir != "" && len(allViolations) > 0 {
		if err := writeConsoleCheckReport(allViolations, reportDir); err != nil {
			fmt.Printf("   Warning: failed to write console check report: %v\n", err)
		}
	}

	if compactMode() {
		if len(allViolations) > 0 {
			printStatus("Console check", false, fmt.Sprintf("%d files", len(allViolations)))
			printReportHint("console-check/")
			return fmt.Errorf("console statement violations found")
		}
		printStatus("Console check", true, "")
		return nil
	}

	if len(allViolations) > 0 {
		return fmt.Errorf("console statement violations found")
	}
	return nil
}

// checkConsoleStatementsWithViolations returns violations instead of just error
func checkConsoleStatementsWithViolations(appName string, files []string, allowedFiles []string) []ConsoleViolation {
	checker := NewConsoleChecker()

	if !compactMode() {
		fmt.Printf("🔍 Checking for console.* statements in %s app...\n", appName)
	}

	var violations []ConsoleViolation

	for _, file := range files {
		if checker.isAllowedFile(file, allowedFiles) {
			continue
		}
		if !checker.isCheckableFile(file) {
			continue
		}

		output, err := checker.gitShowFunc(file)
		if err != nil {
			continue
		}

		if checker.hasConsoleStatements(output) {
			violations = append(violations, ConsoleViolation{AppName: appName, File: file})
			if !compactMode() {
				fmt.Printf("  ❌ %s\n", file)
			}
		}
	}

	if !compactMode() {
		if len(violations) > 0 {
			fmt.Printf("\n❌ Found console.* statements in %d file(s)\n", len(violations))
			fmt.Println("💡 Use a proper logger instead")
		} else {
			fmt.Println("✅ No console.* statements found")
		}
	}

	return violations
}

// writeConsoleCheckReport writes console check findings to per-app report files
func writeConsoleCheckReport(violations []ConsoleViolation, baseDir string) error {
	consoleDir := filepath.Join(baseDir, "console-check")
	if err := os.MkdirAll(consoleDir, 0755); err != nil {
		return err
	}

	// Group by app
	byApp := make(map[string][]ConsoleViolation)
	for _, v := range violations {
		byApp[v.AppName] = append(byApp[v.AppName], v)
	}

	// Write a separate report file for each app
	for app, appViolations := range byApp {
		reportPath := filepath.Join(consoleDir, app+".txt")

		var sb strings.Builder
		sb.WriteString(strings.Repeat("=", 80) + "\n")
		sb.WriteString(fmt.Sprintf("CONSOLE STATEMENT VIOLATIONS - %s\n", strings.ToUpper(app)))
		sb.WriteString(fmt.Sprintf("Generated: %s\n", time.Now().Format("2006-01-02 15:04:05")))
		sb.WriteString(strings.Repeat("=", 80) + "\n\n")

		sb.WriteString(fmt.Sprintf("Total violations: %d\n\n", len(appViolations)))

		sb.WriteString(strings.Repeat("-", 40) + "\n")
		sb.WriteString("FILES WITH CONSOLE STATEMENTS\n")
		sb.WriteString(strings.Repeat("-", 40) + "\n\n")

		for _, v := range appViolations {
			sb.WriteString(fmt.Sprintf("  %s\n", v.File))
		}

		if err := os.WriteFile(reportPath, []byte(sb.String()), 0644); err != nil {
			return err
		}
	}

	return nil
}
