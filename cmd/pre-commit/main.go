package main

import (
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// CLI flags
var (
	standalone    bool
	targetPath    string
	checkName     string
	listChecks    bool
	configPath    string
	reportDir     string
	noLock        bool
)

func init() {
	flag.BoolVar(&standalone, "standalone", false, "Run without git context (check all files in path)")
	flag.StringVar(&targetPath, "path", "", "Directory path to check (used with --standalone)")
	flag.StringVar(&checkName, "check", "", "Run only a specific check (e.g., frontendStructure, srp, mockCheck)")
	flag.BoolVar(&listChecks, "list", false, "List available checks")
	flag.StringVar(&configPath, "config", "", "Path to .pre-commit.json config file (defaults to .pre-commit.json in target path)")
	flag.StringVar(&reportDir, "report-dir", "", "Directory to write detailed lint/typecheck reports (creates lint/ and typecheck/ subdirs)")
	flag.BoolVar(&noLock, "no-lock", false, "Skip exclusive lock (allow concurrent runs)")
}

// compactMode returns true when reports are being written to a directory,
// meaning detailed output should be suppressed in favor of pass/fail status lines.
func compactMode() bool {
	return reportDir != ""
}

// printStatus prints a compact pass/fail status line for a check.
// name is the check name, passed indicates success, detail is optional context (e.g., error count).
func printStatus(name string, passed bool, detail string) {
	if !compactMode() {
		return
	}
	icon := "✅"
	status := ""
	if !passed {
		icon = "❌"
	}
	if detail != "" {
		status = " (" + detail + ")"
	}
	fmt.Printf("  %s %s%s\n", icon, name, status)
}

// printWarningStatus prints a compact warning status line for a non-blocking check.
func printWarningStatus(name string, detail string) {
	if !compactMode() {
		return
	}
	status := ""
	if detail != "" {
		status = " (" + detail + ")"
	}
	fmt.Printf("  ⚠️  %s%s (warning)\n", name, status)
}

// printReportHint prints a pointer to the report directory for a failed check.
func printReportHint(subdir string) {
	if compactMode() {
		fmt.Printf("     → %s/%s\n", reportDir, subdir)
	}
}

func main() {
	flag.Parse()

	if listChecks {
		printAvailableChecks()
		return
	}

	// Acquire exclusive lock to prevent concurrent pre-commit runs
	var lockFile *os.File
	if !noLock {
		var err error
		lockFile, err = acquireLock()
		if err != nil {
			fmt.Fprintln(os.Stderr, "Error: pre-commit already running — commit rejected.")
			os.Exit(1)
		}
	}
	defer releaseLock(lockFile)

	// Set up report directory with branch name and timestamp
	if reportDir != "" {
		reportDir = setupReportDir(reportDir)
	}

	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// getLockPath returns a temp file path unique to the current git repo.
// In standalone mode with a target path, the lock is per-workspace so
// parallel turbo invocations across different workspaces don't block each other.
func getLockPath() string {
	// Use the git toplevel as the repo identifier
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	repoID := "unknown"
	if err == nil {
		repoID = strings.TrimSpace(string(out))
	}

	lockID := repoID
	if standalone && targetPath != "" {
		absPath, err := filepath.Abs(targetPath)
		if err == nil {
			lockID = absPath
		}
	}

	hash := sha256.Sum256([]byte(lockID))
	return filepath.Join(os.TempDir(), fmt.Sprintf("pre-commit-%x.lock", hash[:8]))
}

// setupReportDir creates a subdirectory with branch name and timestamp
func setupReportDir(baseDir string) string {
	branch := getGitBranch()
	timestamp := time.Now().Format("20060102_150405")

	// Sanitize branch name for filesystem (replace / with -)
	safeBranch := strings.ReplaceAll(branch, "/", "-")

	subDir := fmt.Sprintf("%s_%s", safeBranch, timestamp)
	fullPath := filepath.Join(baseDir, subDir)

	if err := os.MkdirAll(fullPath, 0755); err != nil {
		fmt.Printf("Warning: failed to create report directory: %v\n", err)
		return baseDir
	}

	fmt.Printf("📁 Reports will be written to: %s\n\n", fullPath)
	return fullPath
}

// getGitBranch returns the current git branch name
func getGitBranch() string {
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(output))
}

func printAvailableChecks() {
	fmt.Println("Available checks:")
	fmt.Println("  frontendStructure  - Validate CRUD folder structure in components/")
	fmt.Println("  srp                - Single Responsibility Principle check")
	fmt.Println("  mockCheck          - Ensure tests use __mocks__/ instead of inline mocks")
	fmt.Println("  consoleCheck       - Check for console.log statements")
	fmt.Println("  lint               - Run linting only (skip typecheck)")
	fmt.Println("  typecheck          - Run TypeScript type checking only (skip lint)")
	fmt.Println("  lintTypecheck      - Run both linting and TypeScript type checking")
	fmt.Println("  tests              - Run test suites")
	fmt.Println("  changelog          - Validate changelog entries")
	fmt.Println("  goLint             - Go linting (if enabled)")
	fmt.Println("  convexValidation   - Convex schema validation (if enabled)")
	fmt.Println("  buildCheck         - Build verification (if enabled)")
	fmt.Println("  vitestAssertions   - Ensure vitest configs have requireAssertions: true")
	fmt.Println("  testCoverage       - Ensure source files have corresponding test files")
	fmt.Println("  dataLayerCheck     - Check for direct Convex imports (should use data-layer)")
}

func run() error {
	// Handle standalone mode
	if standalone {
		return runStandalone()
	}

	fmt.Println("Running pre-commit checks...")
	fmt.Println()

	// Load configuration
	config, err := loadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Set up report directory from config if not provided via flag
	if reportDir == "" && config.ReportDir != "" {
		reportDir = setupReportDir(config.ReportDir)
	}

	// Get staged files
	stagedFiles, err := getStagedFiles()
	if err != nil {
		return fmt.Errorf("failed to get staged files: %w", err)
	}

	if len(stagedFiles) == 0 {
		fmt.Println("No staged files to check")
		return nil
	}

	// Categorize files by app
	appFiles, sharedChanged := categorizeFiles(stagedFiles, config.Apps, config.SharedPaths)

	// Print detection summary
	printDetectionSummary(appFiles, sharedChanged)

	// If a specific check is requested, run only that check
	if checkName != "" {
		return runSpecificCheck(checkName, config, stagedFiles)
	}

	// Branch protection
	if config.Features.BranchProtection {
		if err := checkBranchProtection(config.ProtectedBranches); err != nil {
			return err
		}
	}

	// Check changelog if enabled
	if config.Features.Changelog {
		if err := checkChangelog(stagedFiles, config.ChangelogExclude, config.ChangelogConfig, config.Apps); err != nil {
			if compactMode() {
				printStatus("Changelog", false, "missing fragments")
			}
			return err
		}
		printStatus("Changelog", true, "")
	}

	// Lint-staged formatting (auto-fix first, before validation)
	if config.Features.LintStaged {
		if err := runLintStaged(config.LintStagedConfig); err != nil {
			return err
		}
	}

	// Go linting
	if config.Features.GoLint {
		if !compactMode() {
			fmt.Println("================================")
			fmt.Println("  GO LINTING")
			fmt.Println("================================")
		}
		if err := checkGoLint(stagedFiles, config.GoLint); err != nil {
			printStatus("Go linting", false, "")
			return err
		}
		if !compactMode() {
			fmt.Println("Go linting passed")
			fmt.Println()
		} else {
			printStatus("Go linting", true, "")
		}
	}

	// Convex validation
	if config.Features.ConvexValidation {
		if !compactMode() {
			fmt.Println("================================")
			fmt.Println("  CONVEX VALIDATION")
			fmt.Println("================================")
		}
		if err := checkConvex(config.Convex); err != nil {
			printStatus("Convex validation", false, "")
			return err
		}
		if !compactMode() {
			fmt.Println("Convex validation passed")
			fmt.Println()
		} else {
			printStatus("Convex validation", true, "")
		}
	}

	// Collect all errors and warnings instead of failing fast
	var allErrors []string
	var allWarnings []string

	// collectResult routes a check failure to warnings or errors based on config
	collectResult := func(checkName string, err error) {
		if err == nil {
			return
		}
		msg := fmt.Sprintf("%s: %v", checkName, err)
		if config.IsWarningCheck(checkName) {
			allWarnings = append(allWarnings, msg)
		} else {
			allErrors = append(allErrors, msg)
		}
	}

	// Lint and typecheck
	if config.Features.LintTypecheck {
		if err := runLintTypecheck(config.Apps, appFiles, sharedChanged, config.TypecheckFilter, config.LintFilter, config.Features.FullLintOnCommit, config.PackageManager); err != nil {
			collectResult("lintTypecheck", err)
		}
	}

	// Console check
	if config.Features.ConsoleCheck {
		collectResult("consoleCheck", runConsoleCheck(appFiles, config.ConsoleAllowed))
	}

	// Data layer check
	if config.Features.DataLayerCheck {
		collectResult("dataLayerCheck", runDataLayerCheck(appFiles, config.DataLayerAllowed))
	}

	// Frontend structure check
	if config.Features.FrontendStructure {
		collectResult("frontendStructure", runFrontendStructureCheck(config.Apps, stagedFiles))
	}

	// SRP check
	if config.Features.SRP {
		var srpFiles []string
		fullMode := config.Features.FullSRPOnCommit

		if fullMode && len(config.SRPConfig.AppPaths) > 0 {
			// Get all files from configured app paths for full SRP check
			for _, appPath := range config.SRPConfig.AppPaths {
				files, err := getAllFilesInPath(appPath)
				if err != nil {
					fmt.Printf("Warning: failed to get files from %s: %v\n", appPath, err)
					continue
				}
				srpFiles = append(srpFiles, files...)
			}
		} else {
			srpFiles = stagedFiles
		}

		filterResult := filterFilesForSRPWithDetails(srpFiles, config.SRPConfig)
		collectResult("srp", runSRPCheckWithFilter(filterResult, config.SRPConfig, fullMode))
	}

	// Test files check
	if config.Features.TestFiles {
		collectResult("testFiles", runTestFilesCheck(stagedFiles))
	}

	// Mock check - ensures test files use __mocks__/ instead of inline jest.mock
	if config.Features.MockCheck {
		collectResult("mockCheck", runMockCheck(stagedFiles, config.MockCheck))
	}

	// Vitest assertions check - ensures vitest configs have requireAssertions: true
	if config.Features.VitestAssertions {
		collectResult("vitestAssertions", runVitestAssertionsCheck(config.Apps))
	}

	// Test coverage check - ensures source files have corresponding test files
	if config.Features.TestCoverage {
		collectResult("testCoverage", runTestCoverageCheck(config.TestCoverageConfig))
	}

	// Build check
	if config.Features.BuildCheck {
		if !compactMode() {
			fmt.Println("================================")
			fmt.Println("  BUILD CHECK")
			fmt.Println("================================")
		}
		if err := checkBuild(config.Build, config.Apps); err != nil {
			collectResult("buildCheck", err)
			printStatus("Build check", false, "")
		} else {
			printStatus("Build check", true, "")
		}
		if !compactMode() {
			fmt.Println()
		}
	}

	// Tests - run if global enabled OR any per-app override enables tests
	shouldRunTests := config.Features.Tests
	if !shouldRunTests {
		// Check if any app has tests explicitly enabled
		for _, override := range config.TestConfig.AppOverrides {
			if override.Enabled != nil && *override.Enabled {
				shouldRunTests = true
				break
			}
		}
	}
	if shouldRunTests {
		testCtx := TestRunContext{
			AllApps:        config.Apps,
			AffectedApps:   appFiles,
			SharedChanged:  sharedChanged,
			Config:         config.TestConfig,
			GlobalEnabled:  config.Features.Tests,
			PackageManager: config.PackageManager,
			Env:            config.Env,
		}
		collectResult("tests", runTests(testCtx))
	}

	// Report warnings
	if len(allWarnings) > 0 {
		fmt.Println()
		if compactMode() {
			fmt.Printf("\n%d check(s) produced warnings (non-blocking)\n", len(allWarnings))
		} else {
			fmt.Println("================================")
			fmt.Println("  WARNINGS (non-blocking)")
			fmt.Println("================================")
			for _, w := range allWarnings {
				fmt.Printf("  ⚠️  %s\n", w)
			}
			fmt.Println()
		}
	}

	// Report all errors at the end
	fmt.Println()
	if len(allErrors) > 0 {
		fmt.Println("================================")
		fmt.Println("  PRE-COMMIT CHECKS FAILED")
		fmt.Println("================================")
		fmt.Println()
		if compactMode() {
			fmt.Printf("%d check(s) failed — see reports: %s\n", len(allErrors), reportDir)
		} else {
			fmt.Println("Fix the errors above and try again")
		}
		return fmt.Errorf("%d check(s) failed", len(allErrors))
	}

	fmt.Println("================================")
	fmt.Println("  ALL PRE-COMMIT CHECKS PASSED!")
	fmt.Println("================================")

	return nil
}

// printDetectionSummary prints a summary of detected file changes
func printDetectionSummary(appFiles map[string][]string, sharedChanged bool) {
	if sharedChanged {
		fmt.Println("Detected changes in shared packages or root files")
		fmt.Println("   This requires checking all apps completely")
		fmt.Println()
	} else {
		for appName, files := range appFiles {
			fmt.Printf("Detected %d changed file(s) in %s app\n", len(files), appName)
		}
		if len(appFiles) > 0 {
			fmt.Println()
		}
	}
}

// runStandalone runs checks in standalone mode (without git context)
func runStandalone() error {
	if targetPath == "" {
		return fmt.Errorf("--path is required when using --standalone")
	}

	// Resolve to absolute path
	absPath, err := filepath.Abs(targetPath)
	if err != nil {
		return fmt.Errorf("failed to resolve path: %w", err)
	}

	// Verify path exists
	info, err := os.Stat(absPath)
	if err != nil {
		return fmt.Errorf("path does not exist: %s", absPath)
	}
	if !info.IsDir() {
		return fmt.Errorf("path is not a directory: %s", absPath)
	}

	fmt.Printf("Running standalone checks on: %s\n", absPath)
	fmt.Println()

	// Change to target directory to load config
	originalDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current directory: %w", err)
	}

	// Find project root (where .pre-commit.json is)
	projectRoot := findProjectRoot(absPath)
	if projectRoot == "" {
		return fmt.Errorf("could not find .pre-commit.json in %s or any parent directory", absPath)
	}

	if err := os.Chdir(projectRoot); err != nil {
		return fmt.Errorf("failed to change to project root: %w", err)
	}
	defer os.Chdir(originalDir)

	// Load configuration
	config, err := loadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Set up report directory from config if not provided via flag
	if reportDir == "" && config.ReportDir != "" {
		reportDir = setupReportDir(config.ReportDir)
	}

	// Get all files in the target path (simulate staged files)
	allFiles, err := getAllFiles(absPath, projectRoot)
	if err != nil {
		return fmt.Errorf("failed to list files: %w", err)
	}

	fmt.Printf("Found %d files to check\n", len(allFiles))
	fmt.Println()

	// Scope config.Apps to only apps that overlap with the target path.
	// This prevents --path packages/types from triggering checks on all apps
	// just because packages/ is a shared path.
	relTarget, err := filepath.Rel(projectRoot, absPath)
	if err == nil && relTarget != "." {
		scopedApps := make(map[string]AppConfig)
		for name, app := range config.Apps {
			// Include app if target is inside the app, or app is inside the target
			if strings.HasPrefix(relTarget, app.Path) || strings.HasPrefix(app.Path, relTarget) {
				scopedApps[name] = app
			}
		}
		if len(scopedApps) > 0 {
			config.Apps = scopedApps
		} else {
			// Target doesn't match any configured app (e.g., packages/types).
			// Synthesize an app entry from the target's package.json so the
			// per-app lint/typecheck machinery can handle it directly.
			syntheticApp, ok := synthAppFromPackageJSON(absPath, relTarget)
			if ok {
				config.Apps = map[string]AppConfig{syntheticApp.name: syntheticApp.config}
			}
			// If no package.json either, keep all apps as fallback
		}
	}

	// Run specific check or all applicable checks
	if checkName != "" {
		return runSpecificCheck(checkName, config, allFiles)
	}

	// Run all enabled checks that make sense in standalone mode
	return runAllStandaloneChecks(config, allFiles)
}

// findProjectRoot finds the directory containing .pre-commit.json
func findProjectRoot(startPath string) string {
	current := startPath
	for {
		configFile := filepath.Join(current, ".pre-commit.json")
		if _, err := os.Stat(configFile); err == nil {
			return current
		}

		parent := filepath.Dir(current)
		if parent == current {
			return ""
		}
		current = parent
	}
}

// getAllFiles recursively gets all files in a directory
// synthResult holds a synthesized app name and config
type synthResult struct {
	name   string
	config AppConfig
}

// synthAppFromPackageJSON reads the package.json in dir and creates a synthetic
// AppConfig so that standalone --path works for non-app packages.
func synthAppFromPackageJSON(dir, relPath string) (synthResult, bool) {
	pkgPath := filepath.Join(dir, "package.json")
	data, err := os.ReadFile(pkgPath)
	if err != nil {
		return synthResult{}, false
	}

	var pkg struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil || pkg.Name == "" {
		return synthResult{}, false
	}

	// Derive a short name from the package name (e.g., "@dashtag/types" → "types")
	shortName := pkg.Name
	if idx := strings.LastIndex(shortName, "/"); idx >= 0 {
		shortName = shortName[idx+1:]
	}

	return synthResult{
		name: shortName,
		config: AppConfig{
			Path:   relPath,
			Filter: pkg.Name,
		},
	}, true
}

func getAllFiles(dir, projectRoot string) ([]string, error) {
	var files []string

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip hidden directories and node_modules
		if info.IsDir() {
			name := info.Name()
			if strings.HasPrefix(name, ".") || name == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}

		// Get relative path from project root
		relPath, err := filepath.Rel(projectRoot, path)
		if err != nil {
			return err
		}

		files = append(files, relPath)
		return nil
	})

	return files, err
}

// getAllFilesInPath recursively gets all files in a directory, returning paths relative to current dir
func getAllFilesInPath(dir string) ([]string, error) {
	var files []string

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip hidden directories and node_modules
		if info.IsDir() {
			name := info.Name()
			if strings.HasPrefix(name, ".") || name == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}

		files = append(files, path)
		return nil
	})

	return files, err
}

// runSpecificCheck runs a single named check
func runSpecificCheck(name string, config *Config, files []string) error {
	appFiles, sharedChanged := categorizeFiles(files, config.Apps, config.SharedPaths)

	switch name {
	case "frontendStructure":
		return runFrontendStructureCheck(config.Apps, files)
	case "srp":
		filterResult := filterFilesForSRPWithDetails(files, config.SRPConfig)
		return runSRPCheckWithFilter(filterResult, config.SRPConfig, true)
	case "mockCheck":
		return runMockCheck(files, config.MockCheck)
	case "consoleCheck":
		return runConsoleCheck(appFiles, config.ConsoleAllowed)
	case "lint":
		lintApps := make(map[string]AppConfig)
		for k, v := range config.Apps {
			v.SkipTypecheck = true
			lintApps[k] = v
		}
		return runLintTypecheck(lintApps, appFiles, sharedChanged, config.TypecheckFilter, config.LintFilter, config.Features.FullLintOnCommit, config.PackageManager)
	case "typecheck":
		tcApps := make(map[string]AppConfig)
		for k, v := range config.Apps {
			v.SkipLint = true
			tcApps[k] = v
		}
		return runLintTypecheck(tcApps, appFiles, sharedChanged, config.TypecheckFilter, config.LintFilter, config.Features.FullLintOnCommit, config.PackageManager)
	case "lintTypecheck":
		return runLintTypecheck(config.Apps, appFiles, sharedChanged, config.TypecheckFilter, config.LintFilter, config.Features.FullLintOnCommit, config.PackageManager)
	case "tests":
		testCtx := TestRunContext{
			AllApps:        config.Apps,
			AffectedApps:   appFiles,
			SharedChanged:  sharedChanged,
			Config:         config.TestConfig,
			GlobalEnabled:  true, // When running --check tests, assume enabled
			PackageManager: config.PackageManager,
			Env:            config.Env,
		}
		return runTests(testCtx)
	case "changelog":
		return checkChangelog(files, config.ChangelogExclude, config.ChangelogConfig, config.Apps)
	case "goLint":
		return checkGoLint(files, config.GoLint)
	case "convexValidation":
		return checkConvex(config.Convex)
	case "buildCheck":
		return checkBuild(config.Build, config.Apps)
	case "vitestAssertions":
		return runVitestAssertionsCheck(config.Apps)
	case "testCoverage":
		return runTestCoverageCheck(config.TestCoverageConfig)
	case "dataLayerCheck":
		return runDataLayerCheck(appFiles, config.DataLayerAllowed)
	default:
		return fmt.Errorf("unknown check: %s (use --list to see available checks)", name)
	}
}

// runAllStandaloneChecks runs all checks that work in standalone mode
// Collects all errors and continues running all checks before reporting
func runAllStandaloneChecks(config *Config, files []string) error {
	appFiles, sharedChanged := categorizeFiles(files, config.Apps, config.SharedPaths)
	printDetectionSummary(appFiles, sharedChanged)

	var allErrors []string
	var allWarnings []string

	collectResult := func(checkName string, err error) {
		if err == nil {
			return
		}
		msg := fmt.Sprintf("%s: %v", checkName, err)
		if config.IsWarningCheck(checkName) {
			allWarnings = append(allWarnings, msg)
		} else {
			allErrors = append(allErrors, msg)
		}
	}

	// Frontend structure check
	if config.Features.FrontendStructure {
		collectResult("frontendStructure", runFrontendStructureCheck(config.Apps, files))
	}

	// SRP check
	if config.Features.SRP {
		filterResult := filterFilesForSRPWithDetails(files, config.SRPConfig)
		collectResult("srp", runSRPCheckWithFilter(filterResult, config.SRPConfig, true))
	}

	// Mock check
	if config.Features.MockCheck {
		collectResult("mockCheck", runMockCheck(files, config.MockCheck))
	}

	// Console check
	if config.Features.ConsoleCheck {
		collectResult("consoleCheck", runConsoleCheck(appFiles, config.ConsoleAllowed))
	}

	// Data layer check
	if config.Features.DataLayerCheck {
		collectResult("dataLayerCheck", runDataLayerCheck(appFiles, config.DataLayerAllowed))
	}

	// Vitest assertions check
	if config.Features.VitestAssertions {
		collectResult("vitestAssertions", runVitestAssertionsCheck(config.Apps))
	}

	// Test coverage check
	if config.Features.TestCoverage {
		collectResult("testCoverage", runTestCoverageCheck(config.TestCoverageConfig))
	}

	// Lint and typecheck
	if config.Features.LintTypecheck {
		collectResult("lintTypecheck", runLintTypecheck(config.Apps, appFiles, sharedChanged, config.TypecheckFilter, config.LintFilter, config.Features.FullLintOnCommit, config.PackageManager))
	}

	// Report warnings
	if len(allWarnings) > 0 {
		fmt.Println()
		fmt.Println("================================")
		fmt.Println("  WARNINGS (non-blocking)")
		fmt.Println("================================")
		for _, w := range allWarnings {
			fmt.Printf("  ⚠️  %s\n", w)
		}
	}

	fmt.Println()
	if len(allErrors) > 0 {
		fmt.Println("================================")
		fmt.Println("  STANDALONE CHECKS FAILED")
		fmt.Println("================================")
		fmt.Println()
		fmt.Println("Errors found:")
		for _, e := range allErrors {
			fmt.Printf("  • %s\n", e)
		}
		return fmt.Errorf("%d check(s) failed", len(allErrors))
	}

	fmt.Println("================================")
	fmt.Println("  ALL STANDALONE CHECKS PASSED!")
	fmt.Println("================================")

	return nil
}

// SRPFilterResult contains the result of filtering files for SRP
type SRPFilterResult struct {
	Files            []string
	SkippedByAppPath int
	SkippedByExclude int
	ExcludeMatches   map[string]int // excludePath -> count of files matched
	SkippedPaths     map[string]int // top-level directory -> count of files skipped
}

// filterFilesForSRP filters files based on SRP config (appPaths and excludePaths)
func filterFilesForSRP(files []string, config SRPConfig) []string {
	result := filterFilesForSRPWithDetails(files, config)
	return result.Files
}

// filterFilesForSRPWithDetails filters files and returns detailed information about what was filtered
func filterFilesForSRPWithDetails(files []string, config SRPConfig) SRPFilterResult {
	result := SRPFilterResult{
		ExcludeMatches: make(map[string]int),
		SkippedPaths:   make(map[string]int),
	}

	if len(config.AppPaths) == 0 && len(config.ExcludePaths) == 0 {
		result.Files = files
		return result
	}

	for _, file := range files {
		// Check if file is in an allowed app path
		if len(config.AppPaths) > 0 {
			inAllowedPath := false
			for _, appPath := range config.AppPaths {
				if strings.HasPrefix(file, appPath) {
					inAllowedPath = true
					break
				}
			}
			if !inAllowedPath {
				result.SkippedByAppPath++
				// Track which top-level directory was skipped
				topLevel := getTopLevelDir(file)
				result.SkippedPaths[topLevel]++
				continue
			}
		}

		// Check if file is in an excluded path
		excluded := false
		for _, excludePath := range config.ExcludePaths {
			if strings.HasPrefix(file, excludePath) || strings.Contains(file, excludePath) {
				excluded = true
				result.ExcludeMatches[excludePath]++
				break
			}
		}
		if excluded {
			result.SkippedByExclude++
			continue
		}

		result.Files = append(result.Files, file)
	}

	return result
}

// getTopLevelDir extracts the top-level directory from a file path
// e.g., "packages/ui/src/Button.tsx" -> "packages/"
// e.g., "apps/mobile/App.tsx" -> "apps/"
// e.g., "tsconfig.json" -> "(root)"
func getTopLevelDir(filePath string) string {
	parts := strings.Split(filePath, "/")
	if len(parts) <= 1 {
		return "(root)"
	}
	return parts[0] + "/"
}
