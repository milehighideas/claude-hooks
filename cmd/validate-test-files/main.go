package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/milehighideas/claude-hooks/internal/jsonc"
	"github.com/milehighideas/claude-hooks/internal/stubs"
)

// E2E test extensions by app type
var e2eExtensions = map[string]string{
	"mobile": ".maestro.yaml",
	"native": ".maestro.yaml",
	"web":    ".e2e.ts",
	"portal": ".e2e.ts",
}

// Violation represents a test requirement violation
type Violation struct {
	Severity     string `json:"severity"`
	Message      string `json:"message"`
	Reason       string `json:"reason"`
	ExpectedPath string `json:"expected_path"`
	AppType      string `json:"app_type,omitempty"`
}

// ToolInput represents the input to a tool call
type ToolInput struct {
	FilePath  string `json:"file_path"`
	Content   string `json:"content,omitempty"`    // Write
	OldString string `json:"old_string,omitempty"` // Edit
	NewString string `json:"new_string,omitempty"` // Edit
}

// HookData represents the JSON input from Claude
type HookData struct {
	ToolName  string    `json:"tool_name"`
	ToolInput ToolInput `json:"tool_input"`
}

// getAppType determines app type from file path
func getAppType(filePath string) string {
	apps := []string{"mobile", "native", "web", "portal"}
	for _, app := range apps {
		if strings.Contains(filePath, "/"+app+"/") {
			return app
		}
	}
	return ""
}

// isTestFile checks if file is already a test file
func isTestFile(filePath string) bool {
	testPatterns := []string{".test.", ".spec.", ".e2e.", ".maestro."}
	for _, pattern := range testPatterns {
		if strings.Contains(filePath, pattern) {
			return true
		}
	}
	return false
}

// isTypeOrBarrelFile checks if file is a type definition or barrel export
func isTypeOrBarrelFile(filePath string) bool {
	basename := filepath.Base(filePath)
	if basename == "index.ts" || basename == "index.tsx" {
		return true
	}
	return strings.Contains(filePath, "/types/")
}

// getUnitTestPath returns expected unit test file path
func getUnitTestPath(filePath string) string {
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
func getE2ETestPath(filePath, appType string) string {
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
func isScreen(filePath string) bool {
	return strings.Contains(filePath, "/screens/")
}

// isCRUDFolder checks if file is in a CRUD folder
func isCRUDFolder(filePath string) bool {
	crudFolders := []string{"/create/", "/update/", "/delete/"}
	for _, folder := range crudFolders {
		if strings.Contains(filePath, folder) {
			return true
		}
	}
	return false
}

// isHookOrUtil checks if file is a hook or utility
func isHookOrUtil(filePath string) bool {
	folders := []string{"/hooks/", "/utils/"}
	for _, folder := range folders {
		if strings.Contains(filePath, folder) {
			return true
		}
	}
	return false
}

// isInteractiveComponent determines if component is interactive using code patterns
func isInteractiveComponent(filePath string) (bool, error) {
	// Forms are always interactive
	if isCRUDFolder(filePath) && !strings.Contains(filePath, "/read/") {
		return true, nil
	}

	// Read file content
	content, err := os.ReadFile(filePath)
	if err != nil {
		return false, fmt.Errorf("failed to read file: %w", err)
	}

	code := string(content)

	// Check for state management hooks
	stateHooks := []string{
		`useState`,
		`useReducer`,
		`useContext`,
		`useMutation`,
		`useQuery`,
	}

	for _, hook := range stateHooks {
		pattern := regexp.MustCompile(`\b` + hook + `\s*\(`)
		if pattern.MatchString(code) {
			return true, nil
		}
	}

	// Check for form hooks
	formHooks := []string{
		`useForm`,
		`useFormState`,
		`useFormContext`,
		`useController`,
	}

	for _, hook := range formHooks {
		pattern := regexp.MustCompile(`\b` + hook + `\b`)
		if pattern.MatchString(code) {
			return true, nil
		}
	}

	return false, nil
}

// checkTestRequirements checks if file meets test requirements
func checkTestRequirements(filePath string) ([]Violation, error) {
	violations := []Violation{}

	// Skip test files themselves
	if isTestFile(filePath) {
		return violations, nil
	}

	// Skip type files and barrels
	if isTypeOrBarrelFile(filePath) {
		return violations, nil
	}

	// Only check .tsx and .ts files
	if !strings.HasSuffix(filePath, ".tsx") && !strings.HasSuffix(filePath, ".ts") {
		return violations, nil
	}

	// Determine app type
	appType := getAppType(filePath)

	// Get expected test paths
	unitTestPath := getUnitTestPath(filePath)
	e2eTestPath := getE2ETestPath(filePath, appType)

	// Determine test requirements
	needsUnitTest := false
	needsE2ETest := false
	reason := ""

	if isScreen(filePath) {
		// Screens always need both unit and E2E tests
		needsUnitTest = true
		needsE2ETest = true
		reason = "Screen components"
	} else if isCRUDFolder(filePath) {
		// CRUD components need unit tests
		needsUnitTest = true
		// Create/Update (forms) need E2E tests
		if strings.Contains(filePath, "/create/") || strings.Contains(filePath, "/update/") {
			needsE2ETest = true
			reason = "Form components (create/update)"
		} else {
			reason = "CRUD components"
		}
	} else if isHookOrUtil(filePath) {
		// Hooks and utils need unit tests only
		needsUnitTest = true
		needsE2ETest = false
		reason = "Hooks and utilities"
	} else {
		// Other components - check if interactive
		interactive, err := isInteractiveComponent(filePath)
		if err != nil {
			// If we can't determine, skip validation
			return violations, nil
		}

		if interactive {
			needsUnitTest = true
			needsE2ETest = true
			reason = "Interactive components"
		} else {
			// Display-only components just need unit tests
			needsUnitTest = true
			needsE2ETest = false
			reason = "Display components"
		}
	}

	// Validate unit test exists
	if needsUnitTest && unitTestPath != "" {
		if _, err := os.Stat(unitTestPath); os.IsNotExist(err) {
			violations = append(violations, Violation{
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
			violations = append(violations, Violation{
				Severity:     "error",
				Message:      fmt.Sprintf("Missing E2E test: %s", filepath.Base(e2eTestPath)),
				Reason:       reason,
				ExpectedPath: e2eTestPath,
				AppType:      appType,
			})
		}
	}

	return violations, nil
}

// isComponentWriteOperation checks if operation creates/modifies a component file
func isComponentWriteOperation(data HookData) (bool, string) {
	// Only check Write and Edit operations
	if data.ToolName != "Write" && data.ToolName != "Edit" {
		return false, ""
	}

	filePath := data.ToolInput.FilePath

	// Only check TypeScript/TSX files in components/
	if strings.Contains(filePath, "/components/") {
		if strings.HasSuffix(filePath, ".tsx") || strings.HasSuffix(filePath, ".ts") {
			// Skip if it's already a test file
			if !isTestFile(filePath) {
				return true, filePath
			}
		}
	}

	return false, ""
}

// isTestFileWriteOperation checks if the tool call writes or edits a unit test file.
func isTestFileWriteOperation(data HookData) (bool, string) {
	if data.ToolName != "Write" && data.ToolName != "Edit" {
		return false, ""
	}
	fp := data.ToolInput.FilePath
	if strings.HasSuffix(fp, ".test.tsx") || strings.HasSuffix(fp, ".test.ts") {
		return true, fp
	}
	return false, ""
}

// isStubContent is an alias for the shared stubs.IsStub detector. Kept as a
// package-local name so the tests that exercise it don't need to know where
// the implementation lives.
func isStubContent(content string) bool {
	return stubs.IsStub(content)
}

// getResultingTestContent computes the file content that would exist after the
// tool call completes. Write supplies content directly; Edit applies a single
// replacement to the existing file.
func getResultingTestContent(data HookData) (string, error) {
	switch data.ToolName {
	case "Write":
		return data.ToolInput.Content, nil
	case "Edit":
		existing, err := os.ReadFile(data.ToolInput.FilePath)
		if err != nil {
			return "", fmt.Errorf("failed to read file for edit simulation: %w", err)
		}
		return strings.Replace(string(existing), data.ToolInput.OldString, data.ToolInput.NewString, 1), nil
	default:
		return "", fmt.Errorf("unsupported tool: %s", data.ToolName)
	}
}

// checkDisabled checks if the hook is disabled via environment variable
func checkDisabled() bool {
	return os.Getenv("CLAUDE_HOOKS_AST_VALIDATION") == "false"
}

// findProjectRoot walks up from filePath looking for a directory that contains
// .pre-commit.json. Returns the directory path if found, or "" if no marker is
// found on the way up to the filesystem root. Relative paths are resolved
// against the current working directory.
func findProjectRoot(filePath string) string {
	abs, err := filepath.Abs(filePath)
	if err != nil {
		return ""
	}
	dir := filepath.Dir(abs)
	for {
		if _, err := os.Stat(filepath.Join(dir, ".pre-commit.json")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// projectConfig is a minimal view of .pre-commit.json. The full schema lives
// in cmd/pre-commit/config.go; we decode only the fields this hook needs so
// the two binaries stay loosely coupled.
//
// The two feature flags control separate edit-time gates:
//   - features.testFiles gates the "every component needs a test" existence
//     check. Turning it on blocks Write/Edit on interactive components
//     without a sibling .test file.
//   - features.stubTestCheck gates the stub-test rejection (weak assertions,
//     self-mocks). Turning it on blocks Write/Edit on test files that are
//     stubs regardless of whether the component-existence gate is active.
//
// Either or both can be enabled. testFiles: true also enables stub
// rejection for backward compatibility with earlier behavior.
type projectConfig struct {
	Features struct {
		TestFiles     bool `json:"testFiles"`
		StubTestCheck bool `json:"stubTestCheck"`
	} `json:"features"`
	TestFilesConfig testFilesConfig `json:"testFilesConfig"`
}

// testFilesConfig controls which files inside an opted-in project are
// validated. Mirrors the shape of srpConfig / testCoverageConfig: an include
// list plus an exclude list, both matched as substrings of the file's
// project-relative path.
type testFilesConfig struct {
	// AppPaths restricts validation to files whose project-relative path
	// contains at least one of these substrings. Empty means "everything".
	AppPaths []string `json:"appPaths"`
	// ExcludePaths skips files whose project-relative path contains any of
	// these substrings. Exclusions always win over AppPaths.
	ExcludePaths []string `json:"excludePaths"`
}

// loadProjectConfig walks up from filePath for .pre-commit.json, parses it,
// and returns the project root and config. root is "" when no marker is
// found. Callers inspect cfg.Features.TestFiles / StubTestCheck to decide
// which gates apply.
func loadProjectConfig(filePath string) (root string, cfg projectConfig) {
	root = findProjectRoot(filePath)
	if root == "" {
		return "", projectConfig{}
	}
	if err := jsonc.Unmarshal(filepath.Join(root, ".pre-commit.json"), &cfg); err != nil {
		return root, projectConfig{}
	}
	return root, cfg
}

// isProjectOptedIn returns true when filePath lives inside a project whose
// .pre-commit.json enables features.testFiles. Preserved for external
// callers; internally run() inspects both flags.
func isProjectOptedIn(filePath string) bool {
	_, cfg := loadProjectConfig(filePath)
	return cfg.Features.TestFiles
}

// isFileInScope applies the per-app include/exclude filter from testFilesConfig.
// Paths are compared as substrings of the file's project-relative path. Files
// outside the project root degrade to "in scope" so the hook never silently
// drops work when rel-path computation fails.
func isFileInScope(projectRoot, filePath string, cfg projectConfig) bool {
	rel, err := filepath.Rel(projectRoot, filePath)
	if err != nil {
		return true
	}
	rel = filepath.ToSlash(rel)

	for _, p := range cfg.TestFilesConfig.ExcludePaths {
		if strings.Contains(rel, p) {
			return false
		}
	}

	if len(cfg.TestFilesConfig.AppPaths) == 0 {
		return true
	}

	for _, p := range cfg.TestFilesConfig.AppPaths {
		if strings.Contains(rel, p) {
			return true
		}
	}
	return false
}

// run applies all gates and runs validation, returning an exit code.
// stderr receives any block messages. It is extracted from main() so the
// full path is testable without stdin/exit plumbing.
func run(data HookData, stderr io.Writer) int {
	if checkDisabled() {
		return 0
	}

	// Need a file path to locate the project. Tool calls without one (or with
	// non-Write/Edit tools) can never match any of our validations anyway.
	filePath := data.ToolInput.FilePath
	if filePath == "" {
		return 0
	}

	// Load the project's .pre-commit.json. Two gates, each with its own
	// feature flag:
	//   features.stubTestCheck → edit-time stub rejection
	//   features.testFiles     → component-test-existence check (legacy flag
	//                            also enables stub rejection for back-compat)
	// Files outside any opted-in project get a silent no-op, which makes the
	// hook safe to register globally in ~/.claude/settings.json.
	projectRoot, cfg := loadProjectConfig(filePath)
	if projectRoot == "" {
		return 0
	}
	stubGateOn := cfg.Features.StubTestCheck || cfg.Features.TestFiles
	componentGateOn := cfg.Features.TestFiles
	if !stubGateOn && !componentGateOn {
		return 0
	}

	// Apply per-app scope (testFilesConfig.appPaths / excludePaths). Files
	// outside the configured scope are silently skipped.
	if !isFileInScope(projectRoot, filePath, cfg) {
		return 0
	}

	// Reject stub test files — both regex-level weak-assertion stubs and
	// AST-level self-mock anti-patterns where a test mocks out its own
	// subject. Runs before the component-write check because test files
	// are skipped there.
	//
	// Also rejects:
	//   - Tautological assertions: expect(X).toBe(X) where actual and
	//     expected are textually identical. These pass lint (the matcher
	//     is real) but assert nothing about behavior — a value equals
	//     itself by definition.
	//   - Majority-weak files: more than half of expect() calls use weak
	//     matchers, even if a couple of real assertions are mixed in.
	//     Closes the "wall of weak + one tautology" escape from IsStub.
	if stubGateOn {
		if isTestOp, testPath := isTestFileWriteOperation(data); isTestOp {
			content, err := getResultingTestContent(data)
			if err == nil {
				if stubs.IsStubFile(testPath, content) {
					_, _ = fmt.Fprintf(stderr, `BLOCKED: Stub test file rejected

File: %s

Test file contains only weak placeholder assertions
(expect(true).toBe(true), .toBeDefined(), .toBeTruthy(), .not.toBeNull(),
.toBeOnTheScreen(), .toBeInTheDocument(), .toBeVisible()),
OR mocks out its own subject with a null-returning factory.

Write real assertions that verify the component's behavior.

If the component is genuinely hard to test (complex Convex/Clerk context,
auth gating, etc.), ask the user how much mocking infrastructure to build
rather than falling back to stubs.
`+"\n", filepath.Base(testPath))
					return 2
				}
				if tautoCount := stubs.CountTautological(content); tautoCount > 0 {
					_, _ = fmt.Fprintf(stderr, `BLOCKED: Tautological assertion rejected

File: %s

Test file contains %d call(s) of the shape expect(X).toBe(X) where actual
and expected are textually identical (same literal, same identifier, same
member access). These assertions are guaranteed by the runtime regardless
of behavior — a value equals itself.

Examples that trip this check:
  expect("save").toBe("save")          // string equals itself
  expect(planName).toBe(planName)      // identifier equals itself
  expect(arr).toEqual(arr)             // same reference

Replace with an assertion that verifies what your code actually does. If
you're asserting a label rendered correctly, compare against the source
constant: expect(getByText(labels.SAVE).textContent).toBe(labels.SAVE).
`+"\n", filepath.Base(testPath), tautoCount)
					return 2
				}
				if stubs.IsStubMajority(content) {
					_, _ = fmt.Fprintf(stderr, `BLOCKED: Majority-weak test file rejected

File: %s

More than half of the expect() calls in this file use weak placeholder
matchers (toBeDefined / toBeTruthy / not.toBeNull / toBeOnTheScreen /
toBeInTheDocument / toBeVisible). Mixing a single non-weak matcher into
a wall of weak ones used to pass the all-weak detector — this gate
closes that loophole.

Replace the weak matchers with real assertions that compare against
expected values: toBe / toEqual / toMatchObject / toHaveBeenCalledWith.
`+"\n", filepath.Base(testPath))
					return 2
				}
			}
		}
	}

	// Only validate on component write operations when features.testFiles
	// is enabled. Many projects want stub rejection without the stricter
	// "every component needs a sibling test file" edit-time block.
	if !componentGateOn {
		return 0
	}
	isComponentOp, componentPath := isComponentWriteOperation(data)
	if !isComponentOp {
		return 0
	}

	violations, err := checkTestRequirements(componentPath)
	if err != nil {
		// Allow if we can't validate (don't block on errors)
		return 0
	}

	var errors []Violation
	for _, v := range violations {
		if v.Severity == "error" {
			errors = append(errors, v)
		}
	}
	if len(errors) == 0 {
		return 0
	}

	msg := fmt.Sprintf("BLOCKED: Test file requirements not met\n\nFile: %s\n\nMissing tests:\n",
		filepath.Base(componentPath))
	for _, v := range errors {
		msg += fmt.Sprintf("\n  ❌ %s", v.Message)
		msg += fmt.Sprintf("\n     Reason: %s require tests", v.Reason)
		msg += fmt.Sprintf("\n     Expected: %s", v.ExpectedPath)
		if v.AppType != "" {
			msg += fmt.Sprintf("\n     App type: %s", v.AppType)
		}
		msg += "\n"
	}
	msg += `
Test requirements:
  - Screens: Unit test (.test.tsx) + E2E test
  - Forms (create/update): Unit test + E2E test
  - Hooks/Utils: Unit test only
  - Interactive components: Unit test + E2E test
  - Display components: Unit test only

E2E test types:
  - mobile/native: .maestro.yaml
  - web/portal: .e2e.ts

To fix:
1. Create the missing test files
2. Disable per project: set features.testFiles=false in .pre-commit.json
`
	_, _ = fmt.Fprintln(stderr, msg)
	return 2
}

// listStubs is a thin wrapper around stubs.List so the CLI and tests keep
// the same entry point while the implementation lives in the shared package.
func listStubs(root string, out io.Writer) (int, error) {
	return stubs.List(root, out)
}

func main() {
	listStubsMode := flag.Bool("list-stubs", false,
		"scan positional paths (or cwd) for pure-stub test files and exit")
	flag.Parse()

	if *listStubsMode {
		roots := flag.Args()
		if len(roots) == 0 {
			roots = []string{"."}
		}
		total := 0
		for _, r := range roots {
			n, err := listStubs(r, os.Stdout)
			if err != nil {
				fmt.Fprintf(os.Stderr, "validate-test-files: %v\n", err)
				os.Exit(2)
			}
			total += n
		}
		if total == 0 {
			os.Exit(0)
		}
		os.Exit(1)
	}

	var data HookData
	if err := json.NewDecoder(os.Stdin).Decode(&data); err != nil {
		// Allow if we can't parse
		os.Exit(0)
	}
	os.Exit(run(data, os.Stderr))
}
