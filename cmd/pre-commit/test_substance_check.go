package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/milehighideas/claude-hooks/internal/stubs"
	"github.com/milehighideas/claude-hooks/internal/substance"
)

// substanceFileResult is the per-file outcome of the substance check.
// Either Substance has at least one violation or Tautologies > 0 (or
// MajorityWeak is true). Files with no findings are not included in the
// report.
type substanceFileResult struct {
	Source       string                 // absolute path
	Test         string                 // absolute path
	Substance    []substance.Violation  // LOC ratio / interaction / branch
	Tautologies  int                    // count from stubs.CountTautological
	MajorityWeak bool                   // from stubs.IsStubMajority (item 6)
}

// substanceReport aggregates per-file results for output.
type substanceReport struct {
	Files []substanceFileResult
}

// runTestSubstanceCheck is the pre-commit / standalone entry point for the
// substance gates. Delegates the scan to collectSubstanceReport, prints a
// pass/fail status line (compact) or the full report (verbose), writes a
// per-app report to disk if reportDir is set, and returns a non-nil error if
// any file has at least one violation.
//
// The compact-mode printStatus calls mirror the sibling stub/missing-tests
// gates — without them this check ran silently, printing neither ✅ nor ❌ to
// the live runner output even though it could fail the commit.
func runTestSubstanceCheck(cfg TestSubstanceCheckConfig, projectRoot string, stagedFiles []string) error {
	if !compactMode() {
		fmt.Println("================================")
		fmt.Println("  TEST SUBSTANCE CHECK")
		fmt.Println("================================")
	}

	rep, err := collectSubstanceReport(cfg, projectRoot, stagedFiles)
	if err != nil {
		if compactMode() {
			printStatus("Test substance", false, err.Error())
		} else {
			fmt.Printf("❌ Test substance check error: %v\n\n", err)
		}
		return err
	}

	count := len(rep.Files)

	// Write per-app report files only when there are violations, matching the
	// stub-tests / srp / typecheck report conventions.
	if reportDir != "" && count > 0 {
		if werr := writeSubstanceReport(rep, projectRoot, reportDir); werr != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to write substance report: %v\n", werr)
		}
	} else if reportDir != "" {
		// Always-write: leave a passing fullreport.txt on a clean run.
		_ = writeRunReport("test-substance", "Test substance", "", false)
	}

	if compactMode() {
		if count > 0 {
			printStatus("Test substance", false, substanceAppBreakdownDetail(rep, projectRoot))
			printReportHint("test-substance/")
			return fmt.Errorf("%d test file(s) failed substance checks", count)
		}
		printStatus("Test substance", true, "")
		return nil
	}

	if count == 0 {
		fmt.Println("✅ Test substance check passed")
		fmt.Println()
		return nil
	}
	printSubstanceReportPlain(rep)
	return fmt.Errorf("%d test file(s) failed substance checks", count)
}

// substanceAppBreakdownDetail builds the compact status detail string,
// grouping the failing files by app/package: "mobile 2 file(s), story 11
// file(s)". Mirrors the stub-tests breakdown so the live status line tells you
// which apps to look at before opening the report.
func substanceAppBreakdownDetail(rep *substanceReport, projectRoot string) string {
	appCounts := make(map[string]int)
	for _, f := range rep.Files {
		rel, err := filepath.Rel(projectRoot, f.Source)
		if err != nil {
			rel = f.Source
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
		parts[i] = fmt.Sprintf("%s %d file(s)", app, appCounts[app])
	}
	return strings.Join(parts, ", ")
}

// substanceConfigToInternal converts the JSON-bound TestSubstanceCheckConfig
// into the internal/substance package's Config, applying defaults for
// any zero-valued field. Pulled out so unit tests can exercise it.
func substanceConfigToInternal(cfg TestSubstanceCheckConfig) substance.Config {
	out := substance.DefaultConfig
	if cfg.MinTestSourceRatio != 0 {
		out.MinTestSourceRatio = cfg.MinTestSourceRatio
	}
	if cfg.BranchToItRatio != 0 {
		out.BranchToItRatio = cfg.BranchToItRatio
	}
	if cfg.RequireInteraction != nil {
		out.RequireInteraction = *cfg.RequireInteraction
	}
	if cfg.MinSourceLOCForCheck != 0 {
		out.MinSourceLOCForCheck = cfg.MinSourceLOCForCheck
	}
	return out
}

// collectSubstanceReport runs the substance gates against the configured
// scope. In "staged" mode (default) it inspects each staged source file
// that has a co-located test; in "all" mode it walks every appPath
// recursively. Returns nil report (Files==nil) when nothing is scanned.
func collectSubstanceReport(cfg TestSubstanceCheckConfig, projectRoot string, stagedFiles []string) (*substanceReport, error) {
	mode := cfg.Mode
	if mode == "" {
		mode = missingModeStaged
	}
	if err := validateMissingMode(mode, "testSubstanceCheckConfig.mode"); err != nil {
		return nil, err
	}

	subCfg := substanceConfigToInternal(cfg)
	report := &substanceReport{}
	seen := map[string]bool{}

	check := func(sourceAbs string) {
		if seen[sourceAbs] {
			return
		}
		if !needsTest(sourceAbs) {
			return
		}
		if testSubstanceExcluded(cfg, projectRoot, sourceAbs) {
			return
		}
		// Resolve test path. If neither .test nor .spec exists, skip —
		// the missingTestsCheck gate handles "no test exists at all".
		testAbs, ok := resolveSiblingTest(sourceAbs)
		if !ok {
			return
		}
		seen[sourceAbs] = true

		sourceContent, err := os.ReadFile(sourceAbs)
		if err != nil {
			return
		}
		testContent, err := os.ReadFile(testAbs)
		if err != nil {
			return
		}

		violations := substance.Check(string(sourceContent), string(testContent), subCfg)

		var tautologies int
		if !cfg.AllowTautological {
			tautologies = stubs.CountTautological(string(testContent))
		}

		majorityWeak := stubs.IsStubMajority(string(testContent))

		if len(violations) == 0 && tautologies == 0 && !majorityWeak {
			return
		}
		report.Files = append(report.Files, substanceFileResult{
			Source:       sourceAbs,
			Test:         testAbs,
			Substance:    violations,
			Tautologies:  tautologies,
			MajorityWeak: majorityWeak,
		})
	}

	if mode == missingModeStaged {
		for _, f := range stagedFiles {
			abs := f
			if !filepath.IsAbs(abs) {
				abs = filepath.Join(projectRoot, f)
			}
			check(abs)
		}
	} else {
		// "all" — walk each appPath recursively. If no appPaths set,
		// walk the project root.
		roots := cfg.AppPaths
		if len(roots) == 0 {
			roots = []string{"."}
		}
		for _, p := range roots {
			root := filepath.Join(projectRoot, p)
			if _, err := os.Stat(root); err != nil {
				continue
			}
			err := filepath.WalkDir(root, func(path string, d fs.DirEntry, werr error) error {
				if werr != nil {
					return nil
				}
				if d.IsDir() {
					if path != root && missingTestsSkipDirs[d.Name()] {
						return filepath.SkipDir
					}
					return nil
				}
				check(path)
				return nil
			})
			if err != nil {
				return nil, err
			}
		}
	}

	// Stable order for reproducible reports.
	sort.Slice(report.Files, func(i, j int) bool {
		return report.Files[i].Source < report.Files[j].Source
	})
	return report, nil
}

// resolveSiblingTest returns the absolute path of the .test or .spec
// sibling for sourcePath, if one exists. Mirrors the lookup
// missingTestsCheck does at line 178-186.
func resolveSiblingTest(sourcePath string) (string, bool) {
	expected := expectedTestPath(sourcePath)
	if _, err := os.Stat(expected); err == nil {
		return expected, true
	}
	specAlt := strings.Replace(expected, ".test.", ".spec.", 1)
	if specAlt != expected {
		if _, err := os.Stat(specAlt); err == nil {
			return specAlt, true
		}
	}
	return "", false
}

// testSubstanceExcluded reports whether sourcePath should be skipped by
// the substance gates because its project-relative path matches one of
// cfg.ExcludePaths, or because cfg.AppPaths is set and the file is not
// under any of them.
func testSubstanceExcluded(cfg TestSubstanceCheckConfig, projectRoot, sourcePath string) bool {
	rel, err := filepath.Rel(projectRoot, sourcePath)
	if err != nil {
		rel = sourcePath
	}
	rel = filepath.ToSlash(rel)
	for _, ex := range cfg.ExcludePaths {
		if strings.Contains(rel, ex) {
			return true
		}
	}
	if len(cfg.AppPaths) == 0 {
		return false
	}
	for _, ap := range cfg.AppPaths {
		if strings.HasPrefix(rel, ap) || strings.Contains(rel, "/"+ap) {
			return false
		}
	}
	return true
}

// printSubstanceReportPlain prints a verbose summary to stdout. Only
// fires in non-compact mode (i.e. when --report-dir is not set OR
// --verbose is). Compact mode just prints the status line and a pointer
// to the report dir.
func printSubstanceReportPlain(rep *substanceReport) {
	fmt.Println()
	fmt.Println("================================")
	fmt.Println("  TEST SUBSTANCE FAILURES")
	fmt.Println("================================")
	for _, f := range rep.Files {
		fmt.Printf("\n%s\n  test: %s\n", f.Source, f.Test)
		for _, v := range f.Substance {
			fmt.Printf("  - %s: %s\n", v.Kind, v.Message)
		}
		if f.Tautologies > 0 {
			fmt.Printf("  - tautological_assertions: %d expect(X).toBe(X)-style call(s) where actual==expected (text-identical)\n", f.Tautologies)
		}
		if f.MajorityWeak {
			fmt.Println("  - majority_weak: more than half of expect() calls are weak/tautological matchers")
		}
	}
	fmt.Println()
}

// writeSubstanceReport writes one <baseDir>/test-substance/<app>.txt file per
// app/package that has substance violations, matching the srp/<app>.txt,
// typecheck/<app>.txt, and stub-tests/<app>.txt conventions used by sibling
// checks. Previously this emitted a single report.txt for the whole repo,
// which buried per-app findings and diverged from the other report dirs.
func writeSubstanceReport(rep *substanceReport, projectRoot, baseDir string) error {
	if baseDir == "" || len(rep.Files) == 0 {
		return nil
	}
	outDir := filepath.Join(baseDir, "test-substance")
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return err
	}

	byApp := make(map[string][]substanceFileResult)
	for _, f := range rep.Files {
		rel, err := filepath.Rel(projectRoot, f.Source)
		if err != nil {
			rel = f.Source
		}
		byApp[appNameFromRel(filepath.ToSlash(rel))] = append(byApp[appNameFromRel(filepath.ToSlash(rel))], f)
	}

	generated := time.Now().Format("2006-01-02 15:04:05")
	for app, files := range byApp {
		sort.Slice(files, func(i, j int) bool { return files[i].Source < files[j].Source })

		// Build the per-file violation detail once; it's the actual findings,
		// shared by both findings.txt and the full report.
		var detail strings.Builder
		for _, f := range files {
			fmt.Fprintf(&detail, "%s\n", f.Source)
			fmt.Fprintf(&detail, "  test: %s\n", f.Test)
			for _, v := range f.Substance {
				fmt.Fprintf(&detail, "  - [%s] %s\n", v.Kind, v.Message)
			}
			if f.Tautologies > 0 {
				fmt.Fprintf(&detail, "  - [tautological_assertions] %d expect(X).toBe(X)-style call(s)\n", f.Tautologies)
			}
			if f.MajorityWeak {
				detail.WriteString("  - [majority_weak] more than half of expect() calls are weak/tautological matchers\n")
			}
			detail.WriteString("\n")
		}

		var sb strings.Builder
		sb.WriteString(strings.Repeat("=", 80) + "\n")
		fmt.Fprintf(&sb, "TEST SUBSTANCE - %s\n", strings.ToUpper(app))
		fmt.Fprintf(&sb, "Generated: %s\n", generated)
		sb.WriteString(strings.Repeat("=", 80) + "\n\n")
		fmt.Fprintf(&sb, "Files with violations: %d\n\n", len(files))
		sb.WriteString(detail.String())

		findings := findingsDoc("TEST SUBSTANCE", app, len(files), detail.String())

		if err := writeDualReport(baseDir, "test-substance", app, findings, sb.String()); err != nil {
			return err
		}
	}
	return nil
}
