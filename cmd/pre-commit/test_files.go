package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// E2E test extensions by app type
var e2eExtensions = map[string]string{
	"mobile":      ".maestro.yaml",
	"match-mobile": ".maestro.yaml",
	"native":      ".maestro.yaml",
	"web":         ".e2e.ts",
	"portal":      ".e2e.ts",
	"shop":        ".e2e.ts",
	"store":       ".e2e.ts",
}

// TestFileViolation represents a missing test file
type TestFileViolation struct {
	File         string
	Severity     string
	Message      string
	Reason       string
	ExpectedPath string
	AppType      string
}

// TestFilesChecker validates test file requirements
type TestFilesChecker struct {
	gitShowFunc func(file string) ([]byte, error)
}

// NewTestFilesChecker creates a new test files checker
func NewTestFilesChecker() *TestFilesChecker {
	return &TestFilesChecker{
		gitShowFunc: defaultGitShow,
	}
}

// CheckFiles validates test requirements for staged files
func (c *TestFilesChecker) CheckFiles(files []string) ([]TestFileViolation, error) {
	var violations []TestFileViolation

	for _, file := range files {
		fileViolations := c.checkTestRequirements(file)
		violations = append(violations, fileViolations...)
	}

	return violations, nil
}

// getAppType determines app type from file path
func (c *TestFilesChecker) getAppType(filePath string) string {
	apps := []string{"mobile", "match-mobile", "native", "web", "portal", "shop", "store"}
	for _, app := range apps {
		if strings.Contains(filePath, "/"+app+"/") || strings.HasPrefix(filePath, "apps/"+app+"/") {
			return app
		}
	}
	return ""
}

// isTestFile checks if file is already a test file
func (c *TestFilesChecker) isTestFile(filePath string) bool {
	testPatterns := []string{".test.", ".spec.", ".e2e.", ".maestro."}
	for _, pattern := range testPatterns {
		if strings.Contains(filePath, pattern) {
			return true
		}
	}
	return false
}

// isMockFile checks if file is in a __mocks__ directory
func (c *TestFilesChecker) isMockFile(filePath string) bool {
	return strings.Contains(filePath, "/__mocks__/")
}

// isTestFileInMocks checks if a test file is incorrectly placed in __mocks__
func (c *TestFilesChecker) isTestFileInMocks(filePath string) bool {
	return c.isMockFile(filePath) && c.isTestFile(filePath)
}

// isTypeOrBarrelFile checks if file is a type definition or barrel export
func (c *TestFilesChecker) isTypeOrBarrelFile(filePath string) bool {
	basename := filepath.Base(filePath)
	if basename == "index.ts" || basename == "index.tsx" {
		return true
	}
	return strings.Contains(filePath, "/types/")
}

// isComponentFile checks if file is in components directory
func (c *TestFilesChecker) isComponentFile(filePath string) bool {
	return strings.Contains(filePath, "/components/")
}

// getUnitTestPath returns expected unit test file path
func (c *TestFilesChecker) getUnitTestPath(filePath string) string {
	ext := filepath.Ext(filePath)
	switch ext {
	case ".tsx":
		return strings.TrimSuffix(filePath, ".tsx") + ".test.tsx"
	case ".ts":
		return strings.TrimSuffix(filePath, ".ts") + ".test.ts"
	default:
		return ""
	}
}

// getE2ETestPath returns expected E2E test file path
func (c *TestFilesChecker) getE2ETestPath(filePath, appType string) string {
	if appType == "" {
		return ""
	}

	extension, exists := e2eExtensions[appType]
	if !exists {
		return ""
	}

	ext := filepath.Ext(filePath)
	return strings.TrimSuffix(filePath, ext) + extension
}

// isScreen checks if file is a screen component
func (c *TestFilesChecker) isScreen(filePath string) bool {
	return strings.Contains(filePath, "/screens/")
}

// isCRUDFolder checks if file is in a CRUD folder
func (c *TestFilesChecker) isCRUDFolder(filePath string) bool {
	crudFolders := []string{"/create/", "/update/", "/delete/"}
	for _, folder := range crudFolders {
		if strings.Contains(filePath, folder) {
			return true
		}
	}
	return false
}

// isHookOrUtil checks if file is a hook or utility
func (c *TestFilesChecker) isHookOrUtil(filePath string) bool {
	folders := []string{"/hooks/", "/utils/"}
	for _, folder := range folders {
		if strings.Contains(filePath, folder) {
			return true
		}
	}
	return false
}

// isInteractiveComponent determines if component is interactive
func (c *TestFilesChecker) isInteractiveComponent(filePath string) bool {
	// Forms are always interactive
	if c.isCRUDFolder(filePath) && !strings.Contains(filePath, "/read/") {
		return true
	}

	// Read file content from staging area
	content, err := c.gitShowFunc(filePath)
	if err != nil {
		return false
	}

	code := string(content)

	// Check for state management hooks
	stateHooks := []string{"useState", "useReducer", "useContext", "useMutation"}
	for _, hook := range stateHooks {
		pattern := regexp.MustCompile(`\b` + hook + `\s*\(`)
		if pattern.MatchString(code) {
			return true
		}
	}

	// Check for form hooks
	formHooks := []string{"useForm", "useFormState", "useFormContext", "useController"}
	for _, hook := range formHooks {
		pattern := regexp.MustCompile(`\b` + hook + `\b`)
		if pattern.MatchString(code) {
			return true
		}
	}

	return false
}

// checkTestRequirements checks if file meets test requirements
func (c *TestFilesChecker) checkTestRequirements(filePath string) []TestFileViolation {
	var violations []TestFileViolation

	// Error if test file is inside __mocks__ directory
	if c.isTestFileInMocks(filePath) {
		violations = append(violations, TestFileViolation{
			File:     filePath,
			Severity: "error",
			Message:  "Test file should not be in __mocks__ directory",
			Reason:   "Mocks are simple stubs and don't need tests",
		})
		return violations
	}

	// Skip mock files - they don't need tests
	if c.isMockFile(filePath) {
		return violations
	}

	// Skip test files themselves
	if c.isTestFile(filePath) {
		return violations
	}

	// Skip type files and barrels
	if c.isTypeOrBarrelFile(filePath) {
		return violations
	}

	// Only check .tsx and .ts files
	if !strings.HasSuffix(filePath, ".tsx") && !strings.HasSuffix(filePath, ".ts") {
		return violations
	}

	// Only check component files
	if !c.isComponentFile(filePath) {
		return violations
	}

	// Determine app type
	appType := c.getAppType(filePath)

	// Get expected test paths
	unitTestPath := c.getUnitTestPath(filePath)
	e2eTestPath := c.getE2ETestPath(filePath, appType)

	// Determine test requirements
	needsUnitTest := false
	needsE2ETest := false
	reason := ""

	if c.isScreen(filePath) {
		needsUnitTest = true
		needsE2ETest = true
		reason = "Screen components"
	} else if c.isCRUDFolder(filePath) {
		needsUnitTest = true
		if strings.Contains(filePath, "/create/") || strings.Contains(filePath, "/update/") {
			needsE2ETest = true
			reason = "Form components (create/update)"
		} else {
			reason = "CRUD components"
		}
	} else if c.isHookOrUtil(filePath) {
		needsUnitTest = true
		needsE2ETest = false
		reason = "Hooks and utilities"
	} else {
		if c.isInteractiveComponent(filePath) {
			needsUnitTest = true
			needsE2ETest = true
			reason = "Interactive components"
		} else {
			needsUnitTest = true
			needsE2ETest = false
			reason = "Display components"
		}
	}

	// Validate unit test exists
	if needsUnitTest && unitTestPath != "" {
		if _, err := os.Stat(unitTestPath); os.IsNotExist(err) {
			violations = append(violations, TestFileViolation{
				File:         filePath,
				Severity:     "error",
				Message:      fmt.Sprintf("Missing unit test: %s", filepath.Base(unitTestPath)),
				Reason:       reason,
				ExpectedPath: unitTestPath,
			})
		}
	}

	// Validate E2E test exists
	if needsE2ETest && e2eTestPath != "" {
		if _, err := os.Stat(e2eTestPath); os.IsNotExist(err) {
			violations = append(violations, TestFileViolation{
				File:         filePath,
				Severity:     "warning", // E2E tests as warnings, not blockers
				Message:      fmt.Sprintf("Missing E2E test: %s", filepath.Base(e2eTestPath)),
				Reason:       reason,
				ExpectedPath: e2eTestPath,
				AppType:      appType,
			})
		}
	}

	return violations
}

// runTestFilesCheck is the entry point for test file validation
func runTestFilesCheck(stagedFiles []string) error {
	if !compactMode() {
		fmt.Println("================================")
		fmt.Println("  TEST FILES CHECK")
		fmt.Println("================================")
	}

	checker := NewTestFilesChecker()
	violations, err := checker.CheckFiles(stagedFiles)
	if err != nil {
		return fmt.Errorf("test files check failed: %w", err)
	}

	var errors, warnings []TestFileViolation
	for _, v := range violations {
		if v.Severity == "error" {
			errors = append(errors, v)
		} else {
			warnings = append(warnings, v)
		}
	}

	if compactMode() {
		if len(errors) > 0 {
			printStatus("Test files", false, fmt.Sprintf("%d missing", len(errors)))
			return fmt.Errorf("missing test files")
		}
		printStatus("Test files", true, "")
		return nil
	}

	// Verbose output
	for _, v := range warnings {
		fmt.Printf("⚠️  %s: %s\n", filepath.Base(v.File), v.Message)
		fmt.Printf("   Reason: %s\n", v.Reason)
		fmt.Printf("   Expected: %s\n", v.ExpectedPath)
	}

	for _, v := range errors {
		fmt.Printf("❌ %s: %s\n", filepath.Base(v.File), v.Message)
		fmt.Printf("   Reason: %s require tests\n", v.Reason)
		fmt.Printf("   Expected: %s\n", v.ExpectedPath)
	}

	if len(errors) > 0 {
		fmt.Printf("\n❌ Found %d missing test file(s)\n", len(errors))
		fmt.Println("\nTest requirements:")
		fmt.Println("  - Screens: Unit test (.test.tsx)")
		fmt.Println("  - Forms (create/update): Unit test")
		fmt.Println("  - Hooks/Utils: Unit test")
		fmt.Println("  - Interactive components: Unit test")
		fmt.Println("  - Display components: Unit test")
		fmt.Println()
		return fmt.Errorf("missing test files")
	}

	if len(warnings) > 0 {
		fmt.Printf("\n⚠️  %d E2E test warning(s) - consider adding\n", len(warnings))
	}

	fmt.Println("✅ Test files check passed")
	fmt.Println()
	return nil
}
