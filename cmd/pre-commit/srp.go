package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/milehighideas/claude-hooks/internal/srp"
)

// SRPViolation represents a Single Responsibility Principle violation
type SRPViolation struct {
	File       string
	Severity   string // "error" or "warning"
	Message    string
	Suggestion string
	RuleID     string // e.g. "directConvexImports", "testRequired"
}

// SRPChecker validates Single Responsibility Principle compliance
type SRPChecker struct {
	gitShowFunc   func(file string) ([]byte, error)
	readFileFunc  func(file string) ([]byte, error)
	statFunc      func(file string) (os.FileInfo, error) // for test file existence checks
	useFilesystem bool
	config        SRPConfig
	newFiles      map[string]bool // newly added files (git diff --diff-filter=A)
	changedFiles  map[string]bool // all staged files (git diff --cached --diff-filter=ACMR)
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
		statFunc:      os.Stat,
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
		statFunc:      os.Stat,
		useFilesystem: true,
		config:        config,
	}
}

// contentRules are the six structural detectors that operate on parsed file
// content. testRequired is handled separately (it works off the file list).
var contentRules = []string{
	"directConvexImports", "stateInScreens", "multipleExports",
	"fileSize", "typeExportsLocation", "mixedConcerns",
}

// enabledRuleSet resolves which content detectors run, per srpConfig.enabledRules.
func (c *SRPChecker) enabledRuleSet() map[string]bool {
	m := make(map[string]bool, len(contentRules))
	for _, r := range contentRules {
		m[r] = c.config.isRuleEnabled(r)
	}
	return m
}

// CheckFiles validates SRP compliance for TypeScript files. Structural
// detection is delegated to internal/srp (tree-sitter AST, shared with the
// standalone validate-srp binary); this method owns file selection, severity
// resolution, and the testRequired rule.
func (c *SRPChecker) CheckFiles(files []string) ([]SRPViolation, error) {
	var allViolations []SRPViolation

	opts := srp.Options{
		ScreenHooks:  c.config.resolvedScreenHooks(),
		EnabledRules: c.enabledRuleSet(),
	}

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

		analysis := srp.Analyze(string(content), file)
		for _, v := range srp.RunDetectors(analysis, file, opts) {
			allViolations = append(allViolations, c.resolveSeverity(SRPViolation(v)))
		}
	}

	// testRequired runs on the file list directly (doesn't need content analysis)
	if c.config.isRuleEnabled("testRequired") {
		allViolations = append(allViolations, c.checkTestRequired(files)...)
	}

	return allViolations, nil
}

// resolveSeverity applies the project's severity policy to a raw detector
// violation. Precedence (highest first):
//  1. WarningOnlyPaths — exemption, force "warning"
//  2. ErrorPaths       — keep original severity
//  3. ErrorScopes      — keep original severity for new/changed files
//  4. WarnOnly         — downgrade to "warning" when the rule is listed
func (c *SRPChecker) resolveSeverity(v SRPViolation) SRPViolation {
	switch {
	case c.config.isWarningOnlyPath(v.File):
		v.Severity = "warning"
	case c.config.isErrorPath(v.File):
		// keep
	case c.isInErrorScope(v.File):
		// keep
	case c.config.isWarnOnly(v.RuleID):
		v.Severity = "warning"
	}
	return v
}

func (c *SRPChecker) isTypeScriptFile(filePath string) bool {
	return (strings.HasSuffix(filePath, ".tsx") || strings.HasSuffix(filePath, ".ts")) &&
		!strings.HasSuffix(filePath, ".d.ts") &&
		!strings.HasSuffix(filePath, ".test.ts") &&
		!strings.HasSuffix(filePath, ".test.tsx") &&
		!strings.HasSuffix(filePath, ".spec.ts") &&
		!strings.HasSuffix(filePath, ".spec.tsx")
}

// isInErrorScope returns true when the file's git-change status matches one of
// the configured ErrorScopes. "new" requires the file to be in newFiles
// (diff-filter=A); "changed" requires it to be in changedFiles (diff-filter=ACMR).
// Returns false when no scope is configured or the relevant file set is nil.
func (c *SRPChecker) isInErrorScope(file string) bool {
	if c.config.hasErrorScope("new") && c.newFiles != nil && c.newFiles[file] {
		return true
	}
	if c.config.hasErrorScope("changed") && c.changedFiles != nil && c.changedFiles[file] {
		return true
	}
	return false
}

// checkTestRequired checks that component/feature files have corresponding test files.
// Each profile is evaluated independently against the file list.
func (c *SRPChecker) checkTestRequired(files []string) []SRPViolation {
	var violations []SRPViolation
	profiles := c.config.resolvedTestRequired()

	for _, cfg := range profiles {
		for _, file := range files {
			if !c.shouldCheckTestRequired(file, cfg) {
				continue
			}
			if !c.hasTestFile(file, cfg) {
				ext := filepath.Ext(file)
				base := strings.TrimSuffix(file, ext)

				// Suggest path based on first testDir if available, else co-located
				var suggestedTest string
				if len(cfg.TestDirs) > 0 {
					baseName := strings.TrimSuffix(filepath.Base(file), ext)
					suggestedTest = filepath.Join(cfg.TestDirs[0], baseName+cfg.TestPatterns[0])
				} else {
					suggestedTest = base + ".test" + ext
				}

				// Severity resolution for testRequired:
				//   1. WarningOnlyPaths — exemption, always wins
				//   2. cfg.Severity     — explicit per-profile setting
				//   3. WarnOnly + scope/path — fall back to global rules
				var severity string
				switch {
				case c.config.isWarningOnlyPath(file):
					severity = "warning"
				case cfg.Severity == "error" || cfg.Severity == "warning":
					severity = cfg.Severity
				case c.config.isWarnOnly("testRequired") && !c.config.isErrorPath(file) && !c.isInErrorScope(file):
					severity = "warning"
				default:
					severity = "error"
				}

				violations = append(violations, SRPViolation{
					File:       file,
					Severity:   severity,
					Message:    fmt.Sprintf("Missing test file (%s)", cfg.Name),
					Suggestion: fmt.Sprintf("Create %s", suggestedTest),
					RuleID:     "testRequired",
				})
			}
		}
	}
	return violations
}

// shouldCheckTestRequired returns true if the file should be checked for a test file.
func (c *SRPChecker) shouldCheckTestRequired(file string, cfg TestRequiredConfig) bool {
	// Must match one of the configured extensions
	hasExt := false
	for _, ext := range cfg.Extensions {
		if strings.HasSuffix(file, ext) {
			hasExt = true
			break
		}
	}
	if !hasExt {
		return false
	}

	// Must not be a test file itself
	for _, pattern := range cfg.TestPatterns {
		if strings.HasSuffix(file, pattern) {
			return false
		}
	}

	// Must not be an excluded filename (basename match)
	base := filepath.Base(file)
	for _, excl := range cfg.ExcludeFiles {
		if base == excl {
			return false
		}
	}

	// Must not match excluded paths (substring match)
	for _, excl := range cfg.ExcludePaths {
		if strings.Contains(file, excl) {
			return false
		}
	}

	// Must be in includePaths if specified (prefix match)
	if len(cfg.IncludePaths) > 0 {
		inIncluded := false
		for _, inc := range cfg.IncludePaths {
			if strings.Contains(file, inc) {
				inIncluded = true
				break
			}
		}
		if !inIncluded {
			return false
		}
	}

	// Scope filter: controls which files are eligible for the test requirement.
	// With fullSRPOnCommit the file list may contain ALL files in appPaths,
	// so "changed" must explicitly check against the staged file set.
	switch cfg.Scope {
	case "new":
		if !c.newFiles[file] {
			return false
		}
	case "changed":
		if c.changedFiles != nil && !c.changedFiles[file] {
			return false
		}
	case "all":
		// Everything passes
	default:
		if !c.newFiles[file] {
			return false
		}
	}

	return true
}

// hasTestFile checks whether a corresponding test file exists for the given source file.
// It searches in three locations: co-located, __tests__/ subdirectory, and configured testDirs.
func (c *SRPChecker) hasTestFile(file string, cfg TestRequiredConfig) bool {
	ext := filepath.Ext(file)
	base := strings.TrimSuffix(file, ext)
	dir := filepath.Dir(file)
	baseName := strings.TrimSuffix(filepath.Base(file), ext)

	stat := c.statFunc
	if stat == nil {
		stat = os.Stat
	}

	for _, pattern := range cfg.TestPatterns {
		// 1. Co-located: foo.test.tsx next to foo.tsx
		if _, err := stat(base + pattern); err == nil {
			return true
		}
		// 2. __tests__ directory: dir/__tests__/foo.test.tsx
		if _, err := stat(filepath.Join(dir, "__tests__", baseName+pattern)); err == nil {
			return true
		}
		// 3. testDirs: each configured directory (repo-root-relative, basename match)
		for _, testDir := range cfg.TestDirs {
			if _, err := stat(filepath.Join(testDir, baseName+pattern)); err == nil {
				return true
			}
		}
	}
	return false
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

		var sb strings.Builder
		sb.WriteString(strings.Repeat("=", 80) + "\n")
		fmt.Fprintf(&sb, "SRP ANALYSIS - %s\n", strings.ToUpper(app))
		fmt.Fprintf(&sb, "Generated: %s\n", time.Now().Format("2006-01-02 15:04:05"))
		sb.WriteString(strings.Repeat("=", 80) + "\n\n")

		fmt.Fprintf(&sb, "Total errors: %d\n", len(appErrors))
		fmt.Fprintf(&sb, "Total warnings: %d\n\n", len(appWarnings))

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
				fmt.Fprintf(&sb, "\n%s (%d occurrences)\n", msgType, len(errs))
				for _, e := range errs {
					fmt.Fprintf(&sb, "  %s\n", e.File)
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
				fmt.Fprintf(&sb, "\n%s (%d occurrences)\n", msgType, len(warns))
				for _, w := range warns {
					fmt.Fprintf(&sb, "  %s\n", w.File)
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
			fmt.Fprintf(&sb, "\n%s (%d issues)\n", file, len(violations))
			for _, v := range violations {
				prefix := "❌"
				if v.Severity == "warning" {
					prefix = "⚠️"
				}
				fmt.Fprintf(&sb, "  %s %s\n", prefix, v.Message)
				if v.Suggestion != "" {
					fmt.Fprintf(&sb, "     → %s\n", v.Suggestion)
				}
			}
		}

		// Findings-only report: the blocking errors (warnings are informational
		// and stay in the full report), grouped by file.
		var findingsBody strings.Builder
		if len(appErrors) > 0 {
			errByFile := make(map[string][]SRPViolation)
			for _, e := range appErrors {
				errByFile[e.File] = append(errByFile[e.File], e)
			}
			for file, errs := range errByFile {
				fmt.Fprintf(&findingsBody, "\n%s (%d errors)\n", file, len(errs))
				for _, v := range errs {
					fmt.Fprintf(&findingsBody, "  ❌ %s\n", v.Message)
					if v.Suggestion != "" {
						fmt.Fprintf(&findingsBody, "     → %s\n", v.Suggestion)
					}
				}
			}
		}
		findings := findingsDoc("SRP", app, len(appErrors), findingsBody.String())

		if err := writeDualReport(baseDir, "srp", app, findings, sb.String()); err != nil {
			return err
		}
	}

	return nil
}

// runSRPCheckWithFilter runs SRP check with filter information displayed.
// newFiles is a set of newly added files (for testRequired scope:"new").
// changedFiles is all staged files (for testRequired scope:"changed"); nil disables scope filtering.
func runSRPCheckWithFilter(filterResult SRPFilterResult, config SRPConfig, fullMode bool, newFiles, changedFiles map[string]bool) error {
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
	checker.newFiles = newFiles
	checker.changedFiles = changedFiles
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
		if len(errors) == 0 && len(warnings) == 0 {
			// Always-write: leave a passing fullreport.txt on a clean run
			// (writeSRPReport writes nothing when there are no violations).
			_ = writeRunReport("srp", "SRP compliance", "", false)
		}
	}

	if compactMode() {
		if len(errors) > 0 {
			appCounts := make(map[string]int)
			for _, e := range errors {
				appCounts[getSRPAppNameFromPath(e.File)]++
			}
			apps := make([]string, 0, len(appCounts))
			for app := range appCounts {
				apps = append(apps, app)
			}
			sort.Strings(apps)
			parts := make([]string, len(apps))
			for i, app := range apps {
				parts[i] = fmt.Sprintf("%s %d errors", app, appCounts[app])
			}
			printStatus("SRP compliance", false, strings.Join(parts, ", "))
			printReportHint("srp/")
			return fmt.Errorf("SRP violations found")
		}
		if len(warnings) > 0 && !config.HideWarnings {
			printWarningStatus("SRP compliance", fmt.Sprintf("%d warnings, %d files", len(warnings), len(filterResult.Files)))
			printReportHint("srp/")
		} else {
			printStatus("SRP compliance", true, fmt.Sprintf("%d files", len(filterResult.Files)))
		}
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
