package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// MockCheckConfig configures which modules should use __mocks__/ instead of inline jest.mock
type MockCheckConfig struct {
	// ForbiddenMocks lists modules that have __mocks__/ files and should NOT be mocked inline
	// e.g., ["expo-router", "@/lib/sentry", "@dashtag/mobile-ui"]
	ForbiddenMocks []string `json:"forbiddenMocks"`
	// AllowedFiles lists files that are allowed to have inline mocks (e.g., the __mocks__/ files themselves)
	AllowedFiles []string `json:"allowedFiles"`
}

// MockChecker checks for inline jest.mock() statements that should use __mocks__/ instead
type MockChecker struct {
	gitShowFunc func(file string) ([]byte, error)
}

// NewMockChecker creates a new MockChecker with default git show behavior
func NewMockChecker() *MockChecker {
	return &MockChecker{
		gitShowFunc: defaultGitShow,
	}
}

// Check checks for forbidden inline jest.mock() statements in the given files
func (c *MockChecker) Check(files []string, config MockCheckConfig) error {
	if !compactMode() {
		fmt.Println("================================")
		fmt.Println("  JEST MOCK CHECK")
		fmt.Println("================================")
		fmt.Println("🔍 Checking for inline jest.mock() that should use __mocks__/...")
	}

	if len(config.ForbiddenMocks) == 0 {
		if !compactMode() {
			fmt.Println("⚠️  No forbiddenMocks configured, skipping")
		}
		return nil
	}

	var violations []Violation

	for _, file := range files {
		if !c.isTestFile(file) {
			continue
		}
		if c.isAllowedFile(file, config.AllowedFiles) {
			continue
		}

		output, err := c.gitShowFunc(file)
		if err != nil {
			continue
		}

		fileViolations := c.findViolations(file, output, config.ForbiddenMocks)
		violations = append(violations, fileViolations...)
	}

	// Write report if reportDir is set
	if reportDir != "" && len(violations) > 0 {
		if err := writeMockCheckReport(violations, reportDir); err != nil {
			fmt.Printf("   Warning: failed to write mock check report: %v\n", err)
		}
	}

	if compactMode() {
		if len(violations) > 0 {
			printStatus("Mock check", false, fmt.Sprintf("%d violations", len(violations)))
			printReportHint("mock-check/")
			return fmt.Errorf("forbidden inline mocks found")
		}
		printStatus("Mock check", true, "")
		return nil
	}

	// Verbose output
	if len(violations) > 0 {
		fmt.Printf("\n❌ Found %d forbidden inline jest.mock() call(s):\n\n", len(violations))
		for _, v := range violations {
			fmt.Printf("  %s:\n", v.File)
			fmt.Printf("    Line %d: jest.mock('%s', ...)\n", v.Line, v.Module)
		}
		fmt.Println()
		fmt.Println("💡 These modules have __mocks__/ files and are auto-mocked via moduleNameMapper.")
		fmt.Println("   Remove the inline jest.mock() and import from @/test-utils/mocks if you need")
		fmt.Println("   to configure mock behavior.")
		fmt.Println()
		return fmt.Errorf("forbidden inline mocks found")
	}

	fmt.Println("✅ No forbidden inline jest.mock() calls found")
	fmt.Println()
	return nil
}

// Violation represents a forbidden mock found in a file
type Violation struct {
	File   string
	Line   int
	Module string
}

// isTestFile returns true for test files
func (c *MockChecker) isTestFile(file string) bool {
	return strings.HasSuffix(file, ".test.ts") ||
		strings.HasSuffix(file, ".test.tsx") ||
		strings.HasSuffix(file, ".spec.ts") ||
		strings.HasSuffix(file, ".spec.tsx")
}

// isAllowedFile checks if a file is in the allowed list
func (c *MockChecker) isAllowedFile(file string, allowedFiles []string) bool {
	for _, pattern := range allowedFiles {
		if strings.Contains(file, pattern) {
			return true
		}
	}
	return false
}

// findViolations looks for forbidden jest.mock() calls in file content
func (c *MockChecker) findViolations(file string, content []byte, forbiddenMocks []string) []Violation {
	var violations []Violation
	lines := strings.Split(string(content), "\n")

	for lineNum, line := range lines {
		for _, module := range forbiddenMocks {
			// Build patterns for this module
			// Match: jest.mock('module' or jest.mock("module"
			patterns := []string{
				fmt.Sprintf(`jest\.mock\(['"]%s['"]`, regexp.QuoteMeta(module)),
			}

			for _, patternStr := range patterns {
				pattern := regexp.MustCompile(patternStr)
				if pattern.MatchString(line) {
					violations = append(violations, Violation{
						File:   file,
						Line:   lineNum + 1,
						Module: module,
					})
					break // Only report once per line per module
				}
			}
		}
	}

	return violations
}

// getAppNameFromPath extracts the app name from a file path
// e.g., "apps/mobile/components/foo.tsx" -> "mobile"
// e.g., "packages/backend/convex/foo.ts" -> "backend"
func getAppNameFromPath(filePath string) string {
	parts := strings.Split(filePath, "/")
	if len(parts) >= 2 {
		if parts[0] == "apps" || parts[0] == "packages" {
			return parts[1]
		}
	}
	return "other"
}

// writeMockCheckReport writes mock check findings to per-app report files
func writeMockCheckReport(violations []Violation, baseDir string) error {
	mockDir := filepath.Join(baseDir, "mock-check")
	if err := os.MkdirAll(mockDir, 0755); err != nil {
		return err
	}

	// Group by app
	byApp := make(map[string][]Violation)
	for _, v := range violations {
		app := getAppNameFromPath(v.File)
		byApp[app] = append(byApp[app], v)
	}

	// Write a separate report file for each app
	for app, appViolations := range byApp {
		reportPath := filepath.Join(mockDir, app+".txt")

		var sb strings.Builder
		sb.WriteString(strings.Repeat("=", 80) + "\n")
		sb.WriteString(fmt.Sprintf("MOCK CHECK VIOLATIONS - %s\n", strings.ToUpper(app)))
		sb.WriteString(fmt.Sprintf("Generated: %s\n", time.Now().Format("2006-01-02 15:04:05")))
		sb.WriteString(strings.Repeat("=", 80) + "\n\n")

		sb.WriteString(fmt.Sprintf("Total violations: %d\n\n", len(appViolations)))

		// Group by module within this app
		byModule := make(map[string][]Violation)
		for _, v := range appViolations {
			byModule[v.Module] = append(byModule[v.Module], v)
		}

		sb.WriteString(strings.Repeat("-", 40) + "\n")
		sb.WriteString("VIOLATIONS BY MODULE\n")
		sb.WriteString(strings.Repeat("-", 40) + "\n\n")

		for module, modViolations := range byModule {
			sb.WriteString(fmt.Sprintf("\n%s (%d occurrences)\n", module, len(modViolations)))
			for _, v := range modViolations {
				sb.WriteString(fmt.Sprintf("  %s (line %d)\n", v.File, v.Line))
			}
		}

		sb.WriteString("\n" + strings.Repeat("-", 40) + "\n")
		sb.WriteString("VIOLATIONS BY FILE\n")
		sb.WriteString(strings.Repeat("-", 40) + "\n\n")

		// Group by file within this app
		byFile := make(map[string][]Violation)
		for _, v := range appViolations {
			byFile[v.File] = append(byFile[v.File], v)
		}

		for file, fileViolations := range byFile {
			sb.WriteString(fmt.Sprintf("\n%s (%d violations)\n", file, len(fileViolations)))
			for _, v := range fileViolations {
				sb.WriteString(fmt.Sprintf("  Line %d: jest.mock('%s', ...)\n", v.Line, v.Module))
			}
		}

		if err := os.WriteFile(reportPath, []byte(sb.String()), 0644); err != nil {
			return err
		}
	}

	return nil
}

// runMockCheck orchestrates mock checking for staged files
func runMockCheck(stagedFiles []string, config MockCheckConfig) error {
	checker := NewMockChecker()
	return checker.Check(stagedFiles, config)
}
