package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// TestQualityViolation represents a test file that is just a stub
type TestQualityViolation struct {
	FilePath string
	AppName  string
	Reason   string
}

// TestQualityConfig configures test quality checking
type TestQualityConfig struct {
	// AppPaths specifies which app paths to check (e.g., ["apps/admin", "apps/story"])
	AppPaths []string `json:"appPaths"`
	// ExcludePaths specifies path patterns to exclude
	ExcludePaths []string `json:"excludePaths"`
}

// exportOnlyPatterns matches test files that only verify an export exists
// These are stub tests that provide no behavioral coverage
var exportOnlyPatterns = []*regexp.Regexp{
	// expect(X).toBeDefined() only
	regexp.MustCompile(`(?m)^\s*expect\(\w+\)\.toBeDefined\(\)`),
	// typeof X === "function" only
	regexp.MustCompile(`(?m)^\s*expect\(typeof \w+\)\.toBe\(["']function["']\)`),
	// typeof X === "object" only
	regexp.MustCompile(`(?m)^\s*expect\(typeof \w+\)\.toBe\(["']object["']\)`),
	// typeof X === "string" only
	regexp.MustCompile(`(?m)^\s*expect\(typeof \w+\)\.toBe\(["']string["']\)`),
}

// isExportOnlyTest checks if a test file only contains export verification assertions
func isExportOnlyTest(content string) bool {
	// Find all expect() calls in the file
	expectPattern := regexp.MustCompile(`(?m)^\s*expect\(`)
	allExpects := expectPattern.FindAllStringIndex(content, -1)

	if len(allExpects) == 0 {
		return false // No assertions at all — different problem
	}

	// Count how many are just export checks
	exportCheckCount := 0
	for _, pattern := range exportOnlyPatterns {
		matches := pattern.FindAllStringIndex(content, -1)
		exportCheckCount += len(matches)
	}

	// If ALL expect calls are export-only checks, it's a stub
	return exportCheckCount >= len(allExpects)
}

// checkTestQuality scans test files for export-only stubs
func checkTestQuality(config TestQualityConfig) ([]TestQualityViolation, error) {
	var violations []TestQualityViolation

	for _, appPath := range config.AppPaths {
		err := filepath.Walk(appPath, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil // Skip inaccessible files
			}

			// Skip directories
			if info.IsDir() {
				name := info.Name()
				if name == "node_modules" || name == ".git" || strings.HasPrefix(name, ".") {
					return filepath.SkipDir
				}
				return nil
			}

			// Only check test files
			if !strings.HasSuffix(path, ".test.ts") && !strings.HasSuffix(path, ".test.tsx") {
				return nil
			}

			// Check exclude paths
			for _, exclude := range config.ExcludePaths {
				if strings.Contains(path, exclude) {
					return nil
				}
			}

			// Read and check file content
			data, err := os.ReadFile(path)
			if err != nil {
				return nil
			}

			content := string(data)
			if isExportOnlyTest(content) {
				// Determine app name from path
				appName := "unknown"
				for _, ap := range config.AppPaths {
					if strings.HasPrefix(path, ap) {
						parts := strings.Split(ap, "/")
						if len(parts) > 0 {
							appName = parts[len(parts)-1]
						}
						break
					}
				}

				violations = append(violations, TestQualityViolation{
					FilePath: path,
					AppName:  appName,
					Reason:   "Test only verifies export exists (toBeDefined/typeof). Add behavioral assertions.",
				})
			}

			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("failed to walk %s: %w", appPath, err)
		}
	}

	return violations, nil
}

// runTestQualityCheck is the entry point for test quality validation
func runTestQualityCheck(config TestQualityConfig) error {
	if !compactMode() {
		fmt.Println("================================")
		fmt.Println("  TEST QUALITY CHECK")
		fmt.Println("================================")
	}

	violations, err := checkTestQuality(config)
	if err != nil {
		return fmt.Errorf("test quality check failed: %w", err)
	}

	// Write report if reportDir is set
	if reportDir != "" && len(violations) > 0 {
		if err := writeTestQualityReport(violations, reportDir); err != nil {
			fmt.Printf("   Warning: failed to write test quality report: %v\n", err)
		}
	}

	if compactMode() {
		if len(violations) > 0 {
			// Group by app
			appCounts := make(map[string]int)
			for _, v := range violations {
				appCounts[v.AppName]++
			}
			var parts []string
			for app, count := range appCounts {
				parts = append(parts, fmt.Sprintf("%s %d", app, count))
			}
			printStatus("Test quality", false, strings.Join(parts, ", ")+" export-only stubs")
			printReportHint("test-quality/")
			return fmt.Errorf("test quality check failed: %d export-only test stubs found", len(violations))
		}
		printStatus("Test quality", true, "")
		return nil
	}

	// Verbose output
	if len(violations) == 0 {
		fmt.Println("✅ No export-only test stubs found")
		fmt.Println()
		return nil
	}

	for _, v := range violations {
		fmt.Printf("❌ %s\n", v.FilePath)
		fmt.Printf("   %s\n\n", v.Reason)
	}

	fmt.Printf("\n❌ Found %d export-only test stub(s)\n", len(violations))
	fmt.Println()
	fmt.Println("Tests that only check toBeDefined() or typeof provide no behavioral coverage.")
	fmt.Println("Replace with tests that verify actual behavior, edge cases, and error handling.")
	fmt.Println()

	return fmt.Errorf("test quality check failed: %d export-only test stubs found", len(violations))
}

// writeTestQualityReport writes test quality findings to a report file
func writeTestQualityReport(violations []TestQualityViolation, baseDir string) error {
	qualityDir := filepath.Join(baseDir, "test-quality")
	if err := os.MkdirAll(qualityDir, 0755); err != nil {
		return err
	}

	reportPath := filepath.Join(qualityDir, "violations.txt")

	var sb strings.Builder
	sb.WriteString(strings.Repeat("=", 80) + "\n")
	sb.WriteString("TEST QUALITY VIOLATIONS REPORT\n")
	sb.WriteString(fmt.Sprintf("Generated: %s\n", time.Now().Format("2006-01-02 15:04:05")))
	sb.WriteString(strings.Repeat("=", 80) + "\n\n")

	sb.WriteString(fmt.Sprintf("Total export-only test stubs: %d\n\n", len(violations)))

	// Group by app
	appViolations := make(map[string][]TestQualityViolation)
	for _, v := range violations {
		appViolations[v.AppName] = append(appViolations[v.AppName], v)
	}

	sb.WriteString(strings.Repeat("=", 80) + "\n")
	sb.WriteString("VIOLATIONS BY APP\n")
	sb.WriteString(strings.Repeat("=", 80) + "\n\n")

	for app, vs := range appViolations {
		sb.WriteString(fmt.Sprintf("%s (%d files)\n", app, len(vs)))
		sb.WriteString(strings.Repeat("-", 40) + "\n")
		for _, v := range vs {
			sb.WriteString(fmt.Sprintf("  %s\n", v.FilePath))
		}
		sb.WriteString("\n")
	}

	return os.WriteFile(reportPath, []byte(sb.String()), 0644)
}
