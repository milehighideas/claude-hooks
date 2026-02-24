package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// dataLayerPatterns matches forbidden direct Convex imports in frontend code.
// Pattern 1: any import from @/convex/_generated/api (the raw API object)
// Pattern 2: any import from convex/react (useQuery, useMutation, useAction, etc.)
var dataLayerPatterns = []*regexp.Regexp{
	regexp.MustCompile(`from\s+["']@/convex/_generated/api["']`),
	regexp.MustCompile(`from\s+["']convex/react["']`),
}

// DataLayerChecker checks for direct Convex imports that should use data-layer
type DataLayerChecker struct {
	gitShowFunc func(file string) ([]byte, error)
}

// NewDataLayerChecker creates a new DataLayerChecker with default git show behavior
func NewDataLayerChecker() *DataLayerChecker {
	return &DataLayerChecker{
		gitShowFunc: defaultGitShow,
	}
}

// Check checks for direct Convex imports in the given files.
// Returns an error if any violations are found.
func (c *DataLayerChecker) Check(appName string, files []string, allowedFiles []string) error {
	fmt.Printf("🔍 Checking for direct Convex imports in %s app...\n", appName)

	var violations []string

	for _, file := range files {
		if c.isAllowedFile(file, allowedFiles) {
			continue
		}
		if !c.isCheckableFile(file) {
			continue
		}

		output, err := c.gitShowFunc(file)
		if err != nil {
			continue
		}

		if c.hasDataLayerViolations(output) {
			violations = append(violations, file)
			fmt.Printf("  ❌ %s\n", file)
		}
	}

	if len(violations) > 0 {
		fmt.Printf("\n❌ Found direct Convex imports in %d file(s)\n", len(violations))
		fmt.Println("💡 Use hooks from packages/data-layer instead")
		fmt.Println()
		return fmt.Errorf("direct Convex imports found")
	}

	fmt.Println("✅ No direct Convex imports found")
	return nil
}

// isAllowedFile checks if a file matches any pattern in the allowed list
func (c *DataLayerChecker) isAllowedFile(file string, allowedFiles []string) bool {
	for _, pattern := range allowedFiles {
		if strings.Contains(file, pattern) {
			return true
		}
	}
	return false
}

// isCheckableFile returns true for TypeScript/JavaScript files
func (c *DataLayerChecker) isCheckableFile(file string) bool {
	return strings.HasSuffix(file, ".ts") ||
		strings.HasSuffix(file, ".tsx") ||
		strings.HasSuffix(file, ".js") ||
		strings.HasSuffix(file, ".jsx")
}

// hasDataLayerViolations checks if content contains direct Convex import patterns
func (c *DataLayerChecker) hasDataLayerViolations(content []byte) bool {
	for _, pattern := range dataLayerPatterns {
		if pattern.Match(content) {
			return true
		}
	}
	return false
}

// DataLayerViolation represents a direct Convex import violation
type DataLayerViolation struct {
	AppName  string
	File     string
	Patterns []string
}

// runDataLayerCheck orchestrates data layer checking for all affected apps
func runDataLayerCheck(appFiles map[string][]string, allowedFiles []string) error {
	var allViolations []DataLayerViolation

	for appName, files := range appFiles {
		if len(files) > 0 {
			if !compactMode() {
				fmt.Println("================================")
				fmt.Println("  DATA LAYER IMPORT CHECK")
				fmt.Println("================================")
			}
			violations := checkDataLayerWithViolations(appName, files, allowedFiles)
			allViolations = append(allViolations, violations...)
			if !compactMode() {
				fmt.Println()
			}
		}
	}

	// Write report if reportDir is set
	if reportDir != "" && len(allViolations) > 0 {
		if err := writeDataLayerCheckReport(allViolations, reportDir); err != nil {
			fmt.Printf("   Warning: failed to write data layer check report: %v\n", err)
		}
	}

	if compactMode() {
		if len(allViolations) > 0 {
			printStatus("Data layer check", false, fmt.Sprintf("%d files", len(allViolations)))
			printReportHint("data-layer-check/")
			return fmt.Errorf("direct Convex import violations found")
		}
		printStatus("Data layer check", true, "")
		return nil
	}

	if len(allViolations) > 0 {
		return fmt.Errorf("direct Convex import violations found")
	}
	return nil
}

// checkDataLayerWithViolations returns violations instead of just error
func checkDataLayerWithViolations(appName string, files []string, allowedFiles []string) []DataLayerViolation {
	checker := NewDataLayerChecker()

	if !compactMode() {
		fmt.Printf("🔍 Checking for direct Convex imports in %s app...\n", appName)
	}

	var violations []DataLayerViolation

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

		if checker.hasDataLayerViolations(output) {
			// Collect which patterns matched
			var matched []string
			for _, pattern := range dataLayerPatterns {
				if pattern.Match(output) {
					matched = append(matched, pattern.String())
				}
			}
			violations = append(violations, DataLayerViolation{AppName: appName, File: file, Patterns: matched})
			if !compactMode() {
				fmt.Printf("  ❌ %s\n", file)
			}
		}
	}

	if !compactMode() {
		if len(violations) > 0 {
			fmt.Printf("\n❌ Found direct Convex imports in %d file(s)\n", len(violations))
			fmt.Println("💡 Use hooks from packages/data-layer instead")
		} else {
			fmt.Println("✅ No direct Convex imports found")
		}
	}

	return violations
}

// writeDataLayerCheckReport writes data layer check findings to per-app report files
func writeDataLayerCheckReport(violations []DataLayerViolation, baseDir string) error {
	dataLayerDir := filepath.Join(baseDir, "data-layer-check")
	if err := os.MkdirAll(dataLayerDir, 0755); err != nil {
		return err
	}

	// Group by app
	byApp := make(map[string][]DataLayerViolation)
	for _, v := range violations {
		byApp[v.AppName] = append(byApp[v.AppName], v)
	}

	// Write a separate report file for each app
	for app, appViolations := range byApp {
		reportPath := filepath.Join(dataLayerDir, app+".txt")

		var sb strings.Builder
		sb.WriteString(strings.Repeat("=", 80) + "\n")
		sb.WriteString(fmt.Sprintf("DATA LAYER IMPORT VIOLATIONS - %s\n", strings.ToUpper(app)))
		sb.WriteString(fmt.Sprintf("Generated: %s\n", time.Now().Format("2006-01-02 15:04:05")))
		sb.WriteString(strings.Repeat("=", 80) + "\n\n")

		sb.WriteString(fmt.Sprintf("Total violations: %d\n\n", len(appViolations)))

		sb.WriteString(strings.Repeat("-", 40) + "\n")
		sb.WriteString("FILES WITH DIRECT CONVEX IMPORTS\n")
		sb.WriteString(strings.Repeat("-", 40) + "\n\n")

		for _, v := range appViolations {
			sb.WriteString(fmt.Sprintf("  %s\n", v.File))
			for _, p := range v.Patterns {
				sb.WriteString(fmt.Sprintf("    matched: %s\n", p))
			}
		}

		if err := os.WriteFile(reportPath, []byte(sb.String()), 0644); err != nil {
			return err
		}
	}

	return nil
}
