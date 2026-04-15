package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/milehighideas/claude-hooks/internal/stubs"
)

// stubModeAll walks the configured scope and reports every stub in it. This
// is the "ratchet" semantics — once an app is clean, enabling it locks in
// stub-free state forever.
const stubModeAll = "all"

// stubModeStaged only checks test files that are currently staged for commit.
// Lighter-weight but redundant with validate-test-files' PreToolUse hook,
// which already prevents new stubs from being written.
const stubModeStaged = "staged"

// stubReport is the result of a stub scan. Separating data collection from
// output formatting keeps the check testable without reading stdout.
type stubReport struct {
	Stubs []string
}

// collectStubReport runs the stub scan against projectRoot using cfg. Paths
// in AppPaths inherit the global cfg.Mode; paths in AppModes override that
// with their own per-path mode. "all" paths are walked recursively; "staged"
// paths only flag currently-staged files. When neither AppPaths nor AppModes
// is set the global Mode is applied to the whole project. Returned paths are
// absolute.
func collectStubReport(cfg StubTestCheckConfig, projectRoot string, stagedFiles []string) (*stubReport, error) {
	globalMode := cfg.Mode
	if globalMode == "" {
		globalMode = stubModeAll
	}
	if err := validateStubMode(globalMode, "stubTestCheckConfig.mode"); err != nil {
		return nil, err
	}
	for path, m := range cfg.AppModes {
		ctx := fmt.Sprintf("stubTestCheckConfig.appModes[%q]", path)
		if err := validateStubMode(m, ctx); err != nil {
			return nil, err
		}
	}

	// No per-path scoping: fall back to global-mode behavior.
	if len(cfg.AppPaths) == 0 && len(cfg.AppModes) == 0 {
		return collectStubReportGlobal(cfg, projectRoot, stagedFiles, globalMode)
	}

	allPaths, stagedPaths := bucketStubPaths(cfg, globalMode)
	report := &stubReport{}
	seen := map[string]bool{}

	// "all" paths: walk each recursively and flag every stub under them.
	for _, p := range allPaths {
		root := filepath.Join(projectRoot, p)
		if _, err := os.Stat(root); err != nil {
			// Missing app path — skip instead of failing so partial
			// checkouts / renamed apps don't break pre-commit.
			continue
		}
		found, err := stubs.Find(root)
		if err != nil {
			return nil, fmt.Errorf("scanning %s: %w", root, err)
		}
		for _, s := range found {
			if stubPathExcluded(cfg, projectRoot, s) {
				continue
			}
			if !seen[s] {
				seen[s] = true
				report.Stubs = append(report.Stubs, s)
			}
		}
	}

	// "staged" paths: only flag staged test files that live under them.
	for _, f := range stagedFiles {
		if !stubs.IsTestFile(f) {
			continue
		}
		if stubPathExcluded(cfg, projectRoot, f) {
			continue
		}
		if !stubPathMatchesAny(projectRoot, f, stagedPaths) {
			continue
		}
		if seen[f] {
			continue
		}
		if stubs.CheckFile(f) {
			seen[f] = true
			report.Stubs = append(report.Stubs, f)
		}
	}

	return report, nil
}

// collectStubReportGlobal handles the legacy no-AppPaths / no-AppModes case.
// Preserves the pre-AppModes behavior so existing configs keep working.
func collectStubReportGlobal(cfg StubTestCheckConfig, projectRoot string, stagedFiles []string, mode string) (*stubReport, error) {
	report := &stubReport{}
	switch mode {
	case stubModeStaged:
		for _, f := range stagedFiles {
			if !stubs.IsTestFile(f) {
				continue
			}
			if stubPathExcluded(cfg, projectRoot, f) {
				continue
			}
			if stubs.CheckFile(f) {
				report.Stubs = append(report.Stubs, f)
			}
		}
	case stubModeAll:
		found, err := stubs.Find(projectRoot)
		if err != nil {
			return nil, fmt.Errorf("scanning %s: %w", projectRoot, err)
		}
		for _, s := range found {
			if stubPathExcluded(cfg, projectRoot, s) {
				continue
			}
			report.Stubs = append(report.Stubs, s)
		}
	}
	return report, nil
}

// bucketStubPaths splits every configured path into "all" vs "staged" buckets
// based on its effective mode. AppModes overrides the global Mode when a
// path appears in both. Returned slices are sorted for deterministic output.
func bucketStubPaths(cfg StubTestCheckConfig, globalMode string) (allPaths, stagedPaths []string) {
	effective := map[string]string{}
	for _, p := range cfg.AppPaths {
		effective[p] = globalMode
	}
	for p, m := range cfg.AppModes {
		effective[p] = m
	}
	for p, m := range effective {
		if m == stubModeAll {
			allPaths = append(allPaths, p)
		} else {
			stagedPaths = append(stagedPaths, p)
		}
	}
	sort.Strings(allPaths)
	sort.Strings(stagedPaths)
	return
}

// validateStubMode returns nil if m is a known stub mode, otherwise an error
// tagged with the given context string for call-site clarity.
func validateStubMode(m, context string) error {
	if m == stubModeAll || m == stubModeStaged {
		return nil
	}
	return fmt.Errorf("%s: unknown value %q (want %q or %q)",
		context, m, stubModeAll, stubModeStaged)
}

// stubPathExcluded reports whether path should be skipped based on
// ExcludePaths substring matches against the project-relative path.
func stubPathExcluded(cfg StubTestCheckConfig, projectRoot, path string) bool {
	rel, err := filepath.Rel(projectRoot, path)
	if err != nil {
		rel = path
	}
	rel = filepath.ToSlash(rel)
	for _, p := range cfg.ExcludePaths {
		if strings.Contains(rel, p) {
			return true
		}
	}
	return false
}

// stubPathMatchesAny reports whether path's project-relative form contains
// any entry in paths (substring match, same semantics as srp/testCoverage).
func stubPathMatchesAny(projectRoot, path string, paths []string) bool {
	if len(paths) == 0 {
		return false
	}
	rel, err := filepath.Rel(projectRoot, path)
	if err != nil {
		rel = path
	}
	rel = filepath.ToSlash(rel)
	for _, p := range paths {
		if strings.Contains(rel, p) {
			return true
		}
	}
	return false
}

// runStubTestCheck is the pre-commit entry point. Walks per cfg, prints
// offending paths, returns an error when stubs are found so the main flow
// collects it into the failure list.
func runStubTestCheck(cfg StubTestCheckConfig, projectRoot string, stagedFiles []string) error {
	if !compactMode() {
		fmt.Println("================================")
		fmt.Println("  STUB TEST CHECK")
		fmt.Println("================================")
	}

	report, err := collectStubReport(cfg, projectRoot, stagedFiles)
	if err != nil {
		if compactMode() {
			printStatus("Stub tests", false, err.Error())
		} else {
			fmt.Printf("❌ Stub test check error: %v\n\n", err)
		}
		return err
	}

	count := len(report.Stubs)

	// Write report file if reportDir is set and there are violations so
	// callers can inspect findings without rerunning the check.
	if reportDir != "" && count > 0 {
		if err := writeStubTestReport(report.Stubs, projectRoot, reportDir); err != nil {
			fmt.Printf("   Warning: failed to write stub tests report: %v\n", err)
		}
	}

	if compactMode() {
		if count > 0 {
			appCounts := make(map[string]int)
			for _, s := range report.Stubs {
				rel, err := filepath.Rel(projectRoot, s)
				if err != nil {
					rel = s
				}
				appCounts[appNameFromRel(filepath.ToSlash(rel))]++
			}
			apps := make([]string, 0, len(appCounts))
			for app := range appCounts {
				apps = append(apps, app)
			}
			sort.Strings(apps)
			parts := make([]string, len(apps))
			for i, app := range apps {
				parts[i] = fmt.Sprintf("%s %d stub(s)", app, appCounts[app])
			}
			printStatus("Stub tests", false, strings.Join(parts, ", "))
			printReportHint("stub-tests/")
			return fmt.Errorf("found %d stub test file(s)", count)
		}
		printStatus("Stub tests", true, "")
		return nil
	}

	if count == 0 {
		fmt.Println("✅ Stub test check passed")
		fmt.Println()
		return nil
	}

	fmt.Printf("❌ Found %d stub test file(s):\n\n", count)
	for _, s := range report.Stubs {
		rel, err := filepath.Rel(projectRoot, s)
		if err != nil {
			rel = s
		}
		fmt.Printf("  • %s\n", rel)
	}
	fmt.Println()
	fmt.Println("Stub tests contain only placeholder assertions (expect(true).toBe(true))")
	fmt.Println("that verify nothing. Write real assertions or delete the file.")
	fmt.Println()
	return fmt.Errorf("found %d stub test file(s)", count)
}

// writeStubTestReport writes one <baseDir>/stub-tests/<app>.txt file per app
// that has stub tests, matching the srp/<app>.txt and typecheck/<app>.txt
// conventions used by sibling checks. Paths are made project-relative.
func writeStubTestReport(stubs []string, projectRoot, baseDir string) error {
	outDir := filepath.Join(baseDir, "stub-tests")
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return err
	}

	byApp := make(map[string][]string)
	for _, s := range stubs {
		rel, err := filepath.Rel(projectRoot, s)
		if err != nil {
			rel = s
		}
		rel = filepath.ToSlash(rel)
		byApp[appNameFromRel(rel)] = append(byApp[appNameFromRel(rel)], rel)
	}

	generated := time.Now().Format("2006-01-02 15:04:05")
	for app, files := range byApp {
		sort.Strings(files)

		var sb strings.Builder
		sb.WriteString(strings.Repeat("=", 80) + "\n")
		sb.WriteString(fmt.Sprintf("STUB TESTS - %s\n", strings.ToUpper(app)))
		sb.WriteString(fmt.Sprintf("Generated: %s\n", generated))
		sb.WriteString(strings.Repeat("=", 80) + "\n\n")
		sb.WriteString(fmt.Sprintf("Total stub test files: %d\n\n", len(files)))
		sb.WriteString("A stub test is a file where every expect() call is a weak-only\n")
		sb.WriteString("assertion (expect(true).toBe(true), .toBeDefined(), .toBeTruthy(),\n")
		sb.WriteString(".toBeFalsy(), .not.toBeNull(), .not.toBeUndefined()) or a self-mock\n")
		sb.WriteString("that replaces the subject under test.\n\n")
		sb.WriteString(strings.Repeat("-", 40) + "\n")
		sb.WriteString("STUB FILES\n")
		sb.WriteString(strings.Repeat("-", 40) + "\n\n")
		for _, f := range files {
			sb.WriteString(fmt.Sprintf("  %s\n", f))
		}

		reportPath := filepath.Join(outDir, app+".txt")
		if err := os.WriteFile(reportPath, []byte(sb.String()), 0644); err != nil {
			return err
		}
	}

	return nil
}

// appNameFromRel extracts an app/package name from a project-relative path.
// apps/mobile/foo/bar.ts  -> mobile
// packages/backend/x.ts   -> backend
// something/else.ts       -> other
func appNameFromRel(rel string) string {
	parts := strings.Split(rel, "/")
	if len(parts) >= 2 && (parts[0] == "apps" || parts[0] == "packages") {
		return parts[1]
	}
	return "other"
}
