package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
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
// using cfg. When cfg.Mode is "staged", stagedFiles is the scan population;
// otherwise the walker descends from projectRoot (or each AppPath if set).
// Returned source/expected paths are absolute.
func collectMissingTestsReport(cfg MissingTestsCheckConfig, projectRoot string, stagedFiles []string) (*missingTestsReport, error) {
	report := &missingTestsReport{}
	mode := cfg.Mode
	if mode == "" {
		mode = missingModeAll
	}

	inScope := func(path string) bool {
		return missingPathInScope(cfg, projectRoot, path)
	}

	checkFile := func(path string) {
		if !needsTest(path) {
			return
		}
		expected := expectedTestPath(path)
		if _, err := os.Stat(expected); err == nil {
			return // test exists, fine
		}
		// Also accept .spec variant
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
			if !inScope(f) {
				continue
			}
			checkFile(f)
		}

	case missingModeAll:
		roots := missingScanRoots(cfg, projectRoot)
		for _, r := range roots {
			if _, err := os.Stat(r); err != nil {
				// Missing AppPath: skip rather than fail — lets partial repos
				// (e.g. packages/backend cloned without apps/web) work.
				continue
			}
			err := filepath.WalkDir(r, func(path string, d fs.DirEntry, werr error) error {
				if werr != nil {
					return nil
				}
				if d.IsDir() {
					if path != r && missingTestsSkipDirs[d.Name()] {
						return filepath.SkipDir
					}
					return nil
				}
				if !inScope(path) {
					return nil
				}
				checkFile(path)
				return nil
			})
			if err != nil {
				return nil, fmt.Errorf("scanning %s: %w", r, err)
			}
		}

	default:
		return nil, fmt.Errorf("missingTestsCheckConfig.mode: unknown value %q (want %q or %q)",
			mode, missingModeAll, missingModeStaged)
	}

	return report, nil
}

// missingScanRoots returns the absolute directories the walker should
// descend from. When AppPaths is set, each becomes a root; otherwise the
// whole project root.
func missingScanRoots(cfg MissingTestsCheckConfig, projectRoot string) []string {
	if len(cfg.AppPaths) == 0 {
		return []string{projectRoot}
	}
	roots := make([]string, 0, len(cfg.AppPaths))
	for _, p := range cfg.AppPaths {
		roots = append(roots, filepath.Join(projectRoot, p))
	}
	return roots
}

// missingPathInScope applies AppPaths / ExcludePaths as substring matches
// over the project-relative path, exclusions winning.
func missingPathInScope(cfg MissingTestsCheckConfig, projectRoot, path string) bool {
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
	if compactMode() {
		if count > 0 {
			printStatus("Missing tests", false, fmt.Sprintf("%d missing", count))
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
