package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	missingModeAll    = "all"
	missingModeStaged = "staged"
)

// missingTest is a single violation: source file without a co-located test.
type missingTest struct {
	Source   string // absolute path
	Expected string // absolute path of expected .test.ts(x)
}

// missingTestsReport is the result of a missing-tests scan.
type missingTestsReport struct {
	Missing []missingTest
}

// testFileSuffixes are extensions that indicate a file IS a test (not a
// candidate for needing one). Matches the convention the edit-time hook
// enforces, plus spec/e2e/maestro for belt-and-suspenders.
var testFileSuffixes = []string{".test.ts", ".test.tsx", ".spec.ts", ".spec.tsx", ".e2e.ts", ".e2e.tsx", ".maestro.yaml"}

// missingTestsSkipDirs lists directory basenames the walker never descends
// into. Mirrors stubs.walkSkipDirs with an added __mocks__ since mocks
// don't need their own test files.
var missingTestsSkipDirs = map[string]bool{
	"node_modules": true,
	".git":         true,
	"_generated":   true,
	"dist":         true,
	"build":        true,
	".next":        true,
	".turbo":       true,
	".vercel":      true,
	"__mocks__":    true,
	"__tests__":    true,
}

// needsTest reports whether filePath is a source file that ought to have a
// co-located test. False for test files, type files, configs, barrels,
// layouts, and everything the walker would skip anyway.
func needsTest(filePath string) bool {
	// Must be .ts or .tsx (not .d.ts)
	if strings.HasSuffix(filePath, ".d.ts") {
		return false
	}
	if !strings.HasSuffix(filePath, ".ts") && !strings.HasSuffix(filePath, ".tsx") {
		return false
	}

	base := filepath.Base(filePath)

	// Already a test file
	for _, s := range testFileSuffixes {
		if strings.HasSuffix(base, s) {
			return false
		}
	}

	// Barrel exports
	if base == "index.ts" || base == "index.tsx" {
		return false
	}

	// Type definition files
	if strings.HasSuffix(base, ".types.ts") || strings.HasSuffix(base, ".types.tsx") {
		return false
	}

	// Routing layout files (Expo Router, Next.js App Router)
	if strings.HasSuffix(base, "_layout.ts") || strings.HasSuffix(base, "_layout.tsx") {
		return false
	}

	// Config files by convention
	configKeywords := []string{
		".config.ts", ".config.tsx", // *.config.ts
	}
	for _, kw := range configKeywords {
		if strings.HasSuffix(base, kw) {
			return false
		}
	}

	// Normalized path segments for directory-based skips
	norm := filepath.ToSlash(filePath)

	// Anywhere under /types/
	if strings.Contains(norm, "/types/") {
		return false
	}

	// Under any of the skip dirs (belt-and-suspenders for callers that
	// pass paths directly instead of relying on the walker).
	for dir := range missingTestsSkipDirs {
		if strings.Contains(norm, "/"+dir+"/") {
			return false
		}
	}

	return true
}

// expectedTestPath returns the co-located test path for a source file.
// .tsx -> .test.tsx ; .ts -> .test.ts.
func expectedTestPath(sourcePath string) string {
	ext := filepath.Ext(sourcePath)
	stem := strings.TrimSuffix(sourcePath, ext)
	return stem + ".test" + ext
}

// collectMissingTestsReport runs the missing-tests scan against projectRoot
// using cfg. Paths in AppPaths inherit the global cfg.Mode; paths in
// AppModes override that with their own per-path mode. "all" paths are
// walked recursively; "staged" paths only consider currently-staged files.
// When neither AppPaths nor AppModes is set the global Mode is applied to
// the whole project. Returned source/expected paths are absolute.
func collectMissingTestsReport(cfg MissingTestsCheckConfig, projectRoot string, stagedFiles []string) (*missingTestsReport, error) {
	globalMode := cfg.Mode
	if globalMode == "" {
		globalMode = missingModeAll
	}
	if err := validateMissingMode(globalMode, "missingTestsCheckConfig.mode"); err != nil {
		return nil, err
	}
	for path, m := range cfg.AppModes {
		ctx := fmt.Sprintf("missingTestsCheckConfig.appModes[%q]", path)
		if err := validateMissingMode(m, ctx); err != nil {
			return nil, err
		}
	}

	// No per-path scoping: fall back to global-mode behavior.
	if len(cfg.AppPaths) == 0 && len(cfg.AppModes) == 0 {
		return collectMissingTestsReportGlobal(cfg, projectRoot, stagedFiles, globalMode)
	}

	allPaths, stagedPaths := bucketMissingPaths(cfg, globalMode)
	report := &missingTestsReport{}
	seen := map[string]bool{}

	recordIfMissing := func(path string) {
		if seen[path] {
			return
		}
		if !needsTest(path) {
			return
		}
		expected := expectedTestPath(path)
		if _, err := os.Stat(expected); err == nil {
			return
		}
		specAlt := strings.Replace(expected, ".test.", ".spec.", 1)
		if specAlt != expected {
			if _, err := os.Stat(specAlt); err == nil {
				return
			}
		}
		seen[path] = true
		report.Missing = append(report.Missing, missingTest{Source: path, Expected: expected})
	}

	// "all" paths: walk each recursively.
	for _, p := range allPaths {
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
			if missingPathExcluded(cfg, projectRoot, path) {
				return nil
			}
			recordIfMissing(path)
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("scanning %s: %w", root, err)
		}
	}

	// "staged" paths: only consider staged source files living under them.
	for _, f := range stagedFiles {
		if missingPathExcluded(cfg, projectRoot, f) {
			continue
		}
		if !missingPathMatchesAny(projectRoot, f, stagedPaths) {
			continue
		}
		recordIfMissing(f)
	}

	return report, nil
}

// collectMissingTestsReportGlobal preserves the pre-AppModes fallback path.
func collectMissingTestsReportGlobal(cfg MissingTestsCheckConfig, projectRoot string, stagedFiles []string, mode string) (*missingTestsReport, error) {
	report := &missingTestsReport{}

	recordIfMissing := func(path string) {
		if !needsTest(path) {
			return
		}
		expected := expectedTestPath(path)
		if _, err := os.Stat(expected); err == nil {
			return
		}
		specAlt := strings.Replace(expected, ".test.", ".spec.", 1)
		if specAlt != expected {
			if _, err := os.Stat(specAlt); err == nil {
				return
			}
		}
		report.Missing = append(report.Missing, missingTest{Source: path, Expected: expected})
	}

	switch mode {
	case missingModeStaged:
		for _, f := range stagedFiles {
			if missingPathExcluded(cfg, projectRoot, f) {
				continue
			}
			recordIfMissing(f)
		}
	case missingModeAll:
		err := filepath.WalkDir(projectRoot, func(path string, d fs.DirEntry, werr error) error {
			if werr != nil {
				return nil
			}
			if d.IsDir() {
				if path != projectRoot && missingTestsSkipDirs[d.Name()] {
					return filepath.SkipDir
				}
				return nil
			}
			if missingPathExcluded(cfg, projectRoot, path) {
				return nil
			}
			recordIfMissing(path)
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("scanning %s: %w", projectRoot, err)
		}
	}
	return report, nil
}

// bucketMissingPaths splits every configured path into "all" vs "staged"
// buckets based on its effective mode. AppModes overrides the global Mode
// when a path appears in both. Returned slices are sorted.
func bucketMissingPaths(cfg MissingTestsCheckConfig, globalMode string) (allPaths, stagedPaths []string) {
	effective := map[string]string{}
	for _, p := range cfg.AppPaths {
		effective[p] = globalMode
	}
	for p, m := range cfg.AppModes {
		effective[p] = m
	}
	for p, m := range effective {
		if m == missingModeAll {
			allPaths = append(allPaths, p)
		} else {
			stagedPaths = append(stagedPaths, p)
		}
	}
	sort.Strings(allPaths)
	sort.Strings(stagedPaths)
	return
}

// validateMissingMode returns nil if m is a known missing-tests mode.
func validateMissingMode(m, context string) error {
	if m == missingModeAll || m == missingModeStaged {
		return nil
	}
	return fmt.Errorf("%s: unknown value %q (want %q or %q)",
		context, m, missingModeAll, missingModeStaged)
}

// missingPathExcluded reports whether path should be skipped based on
// ExcludePaths substring matches against the project-relative path.
func missingPathExcluded(cfg MissingTestsCheckConfig, projectRoot, path string) bool {
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

// missingPathMatchesAny reports whether path's project-relative form
// contains any entry in paths (substring match).
func missingPathMatchesAny(projectRoot, path string, paths []string) bool {
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

// runMissingTestsCheck is the pre-commit entry point.
func runMissingTestsCheck(cfg MissingTestsCheckConfig, projectRoot string, stagedFiles []string) error {
	if !compactMode() {
		fmt.Println("================================")
		fmt.Println("  MISSING TESTS CHECK")
		fmt.Println("================================")
	}

	report, err := collectMissingTestsReport(cfg, projectRoot, stagedFiles)
	if err != nil {
		if compactMode() {
			printStatus("Missing tests", false, err.Error())
		} else {
			fmt.Printf("❌ Missing tests check error: %v\n\n", err)
		}
		return err
	}

	count := len(report.Missing)

	// Write report file if reportDir is set and there are violations so
	// callers can inspect findings without rerunning the check.
	if reportDir != "" && count > 0 {
		if err := writeMissingTestsReport(report.Missing, projectRoot, reportDir); err != nil {
			fmt.Printf("   Warning: failed to write missing tests report: %v\n", err)
		}
	}

	if compactMode() {
		if count > 0 {
			appCounts := make(map[string]int)
			for _, m := range report.Missing {
				relSrc, err := filepath.Rel(projectRoot, m.Source)
				if err != nil {
					relSrc = m.Source
				}
				appCounts[appNameFromRel(filepath.ToSlash(relSrc))]++
			}
			apps := make([]string, 0, len(appCounts))
			for app := range appCounts {
				apps = append(apps, app)
			}
			sort.Strings(apps)
			parts := make([]string, len(apps))
			for i, app := range apps {
				parts[i] = fmt.Sprintf("%s %d missing", app, appCounts[app])
			}
			printStatus("Missing tests", false, strings.Join(parts, ", "))
			printReportHint("missing-tests/")
			return fmt.Errorf("found %d source file(s) without tests", count)
		}
		printStatus("Missing tests", true, "")
		return nil
	}

	if count == 0 {
		fmt.Println("✅ Missing tests check passed")
		fmt.Println()
		return nil
	}

	fmt.Printf("❌ Found %d source file(s) without tests:\n\n", count)
	for _, m := range report.Missing {
		relSrc, err := filepath.Rel(projectRoot, m.Source)
		if err != nil {
			relSrc = m.Source
		}
		relExp, err := filepath.Rel(projectRoot, m.Expected)
		if err != nil {
			relExp = m.Expected
		}
		fmt.Printf("  • %s\n    Expected: %s\n\n", relSrc, relExp)
	}
	fmt.Println("Create the test files or exclude these paths via")
	fmt.Println("missingTestsCheckConfig.excludePaths.")
	fmt.Println()
	return fmt.Errorf("found %d source file(s) without tests", count)
}

// writeMissingTestsReport writes one <baseDir>/missing-tests/<app>.txt file
// per app that has source files without co-located tests, matching the
// srp/<app>.txt and typecheck/<app>.txt conventions. Paths are made
// project-relative for readability.
func writeMissingTestsReport(missing []missingTest, projectRoot, baseDir string) error {
	outDir := filepath.Join(baseDir, "missing-tests")
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return err
	}

	type entry struct {
		source, expected string
	}
	byApp := make(map[string][]entry)
	for _, m := range missing {
		relSrc, err := filepath.Rel(projectRoot, m.Source)
		if err != nil {
			relSrc = m.Source
		}
		relSrc = filepath.ToSlash(relSrc)
		relExp, err := filepath.Rel(projectRoot, m.Expected)
		if err != nil {
			relExp = m.Expected
		}
		relExp = filepath.ToSlash(relExp)
		byApp[appNameFromRel(relSrc)] = append(byApp[appNameFromRel(relSrc)], entry{source: relSrc, expected: relExp})
	}

	generated := time.Now().Format("2006-01-02 15:04:05")
	for app, entries := range byApp {
		sort.Slice(entries, func(i, j int) bool { return entries[i].source < entries[j].source })

		var sb strings.Builder
		sb.WriteString(strings.Repeat("=", 80) + "\n")
		sb.WriteString(fmt.Sprintf("MISSING TESTS - %s\n", strings.ToUpper(app)))
		sb.WriteString(fmt.Sprintf("Generated: %s\n", generated))
		sb.WriteString(strings.Repeat("=", 80) + "\n\n")
		sb.WriteString(fmt.Sprintf("Total source files without co-located tests: %d\n\n", len(entries)))
		sb.WriteString("Each source file below is expected to have a .test.ts(x) or .spec.ts(x)\n")
		sb.WriteString("sibling. Create the test file or add the path to\n")
		sb.WriteString("missingTestsCheckConfig.excludePaths.\n\n")
		sb.WriteString(strings.Repeat("-", 40) + "\n")
		sb.WriteString("MISSING TESTS\n")
		sb.WriteString(strings.Repeat("-", 40) + "\n\n")
		for _, e := range entries {
			sb.WriteString(fmt.Sprintf("  %s\n", e.source))
			sb.WriteString(fmt.Sprintf("    → expected: %s\n", e.expected))
		}

		reportPath := filepath.Join(outDir, app+".txt")
		if err := os.WriteFile(reportPath, []byte(sb.String()), 0644); err != nil {
			return err
		}
	}

	return nil
}
