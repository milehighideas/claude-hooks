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

// collectStubReport runs the stub scan against projectRoot using cfg. When
// cfg.Mode is "staged", stagedFiles scopes the search to those paths only;
// otherwise the walker descends from projectRoot (or each AppPath if set).
// Returned paths are absolute. The caller decides how to format / print.
func collectStubReport(cfg StubTestCheckConfig, projectRoot string, stagedFiles []string) (*stubReport, error) {
	report := &stubReport{}
	mode := cfg.Mode
	if mode == "" {
		mode = stubModeAll
	}

	inScope := func(path string) bool {
		return stubPathInScope(cfg, projectRoot, path)
	}

	switch mode {
	case stubModeStaged:
		for _, f := range stagedFiles {
			if !stubs.IsTestFile(f) {
				continue
			}
			if !inScope(f) {
				continue
			}
			if stubs.CheckFile(f) {
				report.Stubs = append(report.Stubs, f)
			}
		}
	case stubModeAll:
		roots := stubScanRoots(cfg, projectRoot)
		for _, r := range roots {
			found, err := stubs.Find(r)
			if err != nil {
				return nil, fmt.Errorf("scanning %s: %w", r, err)
			}
			for _, s := range found {
				if !inScope(s) {
					continue
				}
				report.Stubs = append(report.Stubs, s)
			}
		}
	default:
		return nil, fmt.Errorf("stubTestCheckConfig.mode: unknown value %q (want %q or %q)",
			mode, stubModeAll, stubModeStaged)
	}

	return report, nil
}

// stubScanRoots returns the absolute directories the walker should descend
// from. When AppPaths is set, each (valid) app path becomes a root so the
// walker doesn't traverse the whole repo; otherwise projectRoot itself.
func stubScanRoots(cfg StubTestCheckConfig, projectRoot string) []string {
	if len(cfg.AppPaths) == 0 {
		return []string{projectRoot}
	}
	var roots []string
	for _, p := range cfg.AppPaths {
		roots = append(roots, filepath.Join(projectRoot, p))
	}
	return roots
}

// stubPathInScope applies AppPaths / ExcludePaths as substring matches over
// the project-relative path — same semantics srpConfig / testCoverageConfig
// already use. Exclusions always win.
func stubPathInScope(cfg StubTestCheckConfig, projectRoot, path string) bool {
	rel, err := filepath.Rel(projectRoot, path)
	if err != nil {
		rel = path
	}
	rel = filepath.ToSlash(rel)

	for _, p := range cfg.ExcludePaths {
		if strings.Contains(rel, p) {
			return false
		}
	}
	if len(cfg.AppPaths) == 0 {
		return true
	}
	for _, p := range cfg.AppPaths {
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
			printStatus("Stub tests", false, fmt.Sprintf("%d stub(s)", count))
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
