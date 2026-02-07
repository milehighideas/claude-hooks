package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// TestCoverageViolation represents a source file missing its test file
type TestCoverageViolation struct {
	SourceFile       string
	ExpectedTestFile string
	Folder           string
	AppPath          string
}

// TestCoverageChecker validates that source files have corresponding test files
type TestCoverageChecker struct {
	config TestCoverageConfig
}

// NewTestCoverageChecker creates a new test coverage checker
func NewTestCoverageChecker(config TestCoverageConfig) *TestCoverageChecker {
	return &TestCoverageChecker{config: config}
}

// Check validates all configured apps have test files for required folders
func (c *TestCoverageChecker) Check() ([]TestCoverageViolation, error) {
	var violations []TestCoverageViolation

	for _, appPath := range c.config.AppPaths {
		appViolations, err := c.checkApp(appPath)
		if err != nil {
			return nil, fmt.Errorf("failed to check %s: %w", appPath, err)
		}
		violations = append(violations, appViolations...)
	}

	return violations, nil
}

// checkApp checks a single app for test coverage
func (c *TestCoverageChecker) checkApp(appPath string) ([]TestCoverageViolation, error) {
	var violations []TestCoverageViolation

	// Walk the app directory looking for files in required folders
	err := filepath.Walk(appPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories
		if info.IsDir() {
			// Skip node_modules and hidden directories
			if info.Name() == "node_modules" || strings.HasPrefix(info.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}

		// Check if this file is in a folder that requires tests
		if c.shouldHaveTest(path) {
			testFile := c.getTestFilePath(path)
			if !fileExists(testFile) {
				violations = append(violations, TestCoverageViolation{
					SourceFile:       path,
					ExpectedTestFile: testFile,
					Folder:           filepath.Base(filepath.Dir(path)),
					AppPath:          appPath,
				})
			}
		}

		return nil
	})

	return violations, err
}

// shouldHaveTest determines if a source file should have a corresponding test file
func (c *TestCoverageChecker) shouldHaveTest(path string) bool {
	// Must be a .ts or .tsx file
	ext := filepath.Ext(path)
	if ext != ".ts" && ext != ".tsx" {
		return false
	}

	// Skip test files themselves
	if strings.Contains(path, ".test.") || strings.Contains(path, ".spec.") {
		return false
	}

	// Check if file matches any exclude pattern
	fileName := filepath.Base(path)
	for _, pattern := range c.config.ExcludeFiles {
		if matchPattern(fileName, pattern) {
			return false
		}
	}

	// Check if path matches any exclude path pattern
	for _, excludePath := range c.config.ExcludePaths {
		if strings.Contains(path, excludePath) {
			return false
		}
	}

	// Check if file is in a folder that requires tests
	dir := filepath.Base(filepath.Dir(path))
	for _, folder := range c.config.RequireTestFolders {
		if dir == folder {
			return true
		}
	}

	return false
}

// getTestFilePath returns the expected test file path for a source file
func (c *TestCoverageChecker) getTestFilePath(sourcePath string) string {
	ext := filepath.Ext(sourcePath)
	base := strings.TrimSuffix(sourcePath, ext)
	return base + ".test" + ext
}

// matchPattern checks if a filename matches a glob-like pattern
func matchPattern(filename, pattern string) bool {
	// Handle exact match
	if filename == pattern {
		return true
	}

	// Handle *.extension pattern
	if strings.HasPrefix(pattern, "*") {
		suffix := pattern[1:]
		return strings.HasSuffix(filename, suffix)
	}

	return false
}

// fileExists checks if a file exists
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// runTestCoverageCheck is the entry point for test coverage validation
func runTestCoverageCheck(config TestCoverageConfig) error {
	if !compactMode() {
		fmt.Println("================================")
		fmt.Println("  TEST COVERAGE CHECK")
		fmt.Println("================================")
	}

	// Skip if no app paths configured
	if len(config.AppPaths) == 0 {
		if !compactMode() {
			fmt.Println("⚠️  No app paths configured for test coverage check")
			fmt.Println()
		}
		return nil
	}

	checker := NewTestCoverageChecker(config)
	violations, err := checker.Check()
	if err != nil {
		return fmt.Errorf("test coverage check failed: %w", err)
	}

	// Write report if reportDir is set
	if reportDir != "" && len(violations) > 0 {
		if err := writeTestCoverageReport(violations, reportDir); err != nil {
			fmt.Printf("   Warning: failed to write test coverage report: %v\n", err)
		}
	}

	if compactMode() {
		if len(violations) > 0 {
			printStatus("Test coverage", false, fmt.Sprintf("%d missing", len(violations)))
			printReportHint("test-coverage/")
			return fmt.Errorf("test coverage check failed")
		}
		printStatus("Test coverage", true, "")
		return nil
	}

	// Verbose output
	if len(violations) == 0 {
		fmt.Println("✅ All source files have corresponding test files")
		fmt.Println()
		return nil
	}

	byApp := make(map[string][]TestCoverageViolation)
	for _, v := range violations {
		byApp[v.AppPath] = append(byApp[v.AppPath], v)
	}

	for appPath, appViolations := range byApp {
		fmt.Printf("\n❌ %s - %d file(s) missing tests:\n", appPath, len(appViolations))
		for _, v := range appViolations {
			relSource, _ := filepath.Rel(".", v.SourceFile)
			relTest, _ := filepath.Rel(".", v.ExpectedTestFile)
			fmt.Printf("   %s\n", relSource)
			fmt.Printf("      → expected: %s\n", relTest)
		}
	}

	fmt.Printf("\n❌ Found %d source file(s) missing test files\n", len(violations))
	fmt.Println()
	fmt.Println("Every component, hook, and utility in the configured folders")
	fmt.Println("should have a corresponding .test.ts(x) file.")
	fmt.Println()

	return fmt.Errorf("test coverage check failed")
}

// writeTestCoverageReport writes test coverage findings to a report file
func writeTestCoverageReport(violations []TestCoverageViolation, baseDir string) error {
	coverageDir := filepath.Join(baseDir, "test-coverage")
	if err := os.MkdirAll(coverageDir, 0755); err != nil {
		return err
	}

	reportPath := filepath.Join(coverageDir, "violations.txt")

	var sb strings.Builder
	sb.WriteString(strings.Repeat("=", 80) + "\n")
	sb.WriteString("TEST COVERAGE VIOLATIONS REPORT\n")
	sb.WriteString(fmt.Sprintf("Generated: %s\n", time.Now().Format("2006-01-02 15:04:05")))
	sb.WriteString(strings.Repeat("=", 80) + "\n\n")

	sb.WriteString(fmt.Sprintf("Total files missing tests: %d\n\n", len(violations)))

	// Group by app
	byApp := make(map[string][]TestCoverageViolation)
	for _, v := range violations {
		byApp[v.AppPath] = append(byApp[v.AppPath], v)
	}

	sb.WriteString(strings.Repeat("=", 80) + "\n")
	sb.WriteString("MISSING TESTS BY APP\n")
	sb.WriteString(strings.Repeat("=", 80) + "\n\n")

	for appPath, appViolations := range byApp {
		sb.WriteString(fmt.Sprintf("\n%s (%d files)\n", appPath, len(appViolations)))
		sb.WriteString(strings.Repeat("-", 40) + "\n")

		// Group by folder within app
		byFolder := make(map[string][]TestCoverageViolation)
		for _, v := range appViolations {
			byFolder[v.Folder] = append(byFolder[v.Folder], v)
		}

		for folder, folderViolations := range byFolder {
			sb.WriteString(fmt.Sprintf("\n  %s/ (%d files)\n", folder, len(folderViolations)))
			for _, v := range folderViolations {
				relSource, _ := filepath.Rel(".", v.SourceFile)
				relTest, _ := filepath.Rel(".", v.ExpectedTestFile)
				sb.WriteString(fmt.Sprintf("    %s\n", relSource))
				sb.WriteString(fmt.Sprintf("      → expected: %s\n", relTest))
			}
		}
	}

	return os.WriteFile(reportPath, []byte(sb.String()), 0644)
}
