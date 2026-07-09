package main

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const (
	goMissingModeAll    = "all"
	goMissingModeStaged = "staged"
)

// goGeneratedRe matches Go's canonical generated-code marker line.
var goGeneratedRe = regexp.MustCompile(`^// Code generated .* DO NOT EDIT\.$`)

// goNeedsTest reports whether absPath is an in-scope, non-skipped Go source
// file whose package must therefore contain a *_test.go.
func goNeedsTest(cfg GoMissingTestsCheckConfig, projectRoot, absPath string) bool {
	if !strings.HasSuffix(absPath, ".go") {
		return false
	}
	base := filepath.Base(absPath)
	if strings.HasSuffix(base, "_test.go") || base == "doc.go" {
		return false
	}

	rel, err := filepath.Rel(projectRoot, absPath)
	if err != nil {
		rel = absPath
	}
	rel = filepath.ToSlash(rel)

	for _, p := range cfg.ExcludePaths {
		if strings.Contains(rel, p) {
			return false
		}
	}
	if len(cfg.AppPaths) > 0 {
		inScope := false
		for _, p := range cfg.AppPaths {
			if strings.Contains(rel, p) {
				inScope = true
				break
			}
		}
		if !inScope {
			return false
		}
	}
	return !isGoGenerated(absPath)
}

// isGoGenerated reports whether the file carries the standard generated marker
// in its first 15 lines.
func isGoGenerated(absPath string) bool {
	f, err := os.Open(absPath)
	if err != nil {
		return false
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	for i := 0; i < 15 && sc.Scan(); i++ {
		if goGeneratedRe.MatchString(strings.TrimSpace(sc.Text())) {
			return true
		}
	}
	return false
}

// dirHasGoTest reports whether dir contains at least one *_test.go file.
func dirHasGoTest(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), "_test.go") {
			return true
		}
	}
	return false
}

// collectGoMissingPackages returns absolute package dirs that hold in-scope Go
// source but no *_test.go, per mode.
func collectGoMissingPackages(cfg GoMissingTestsCheckConfig, projectRoot string, stagedFiles []string, mode string) ([]string, error) {
	switch mode {
	case goMissingModeStaged:
		return missingDirsFromSet(goStagedSourceDirs(cfg, projectRoot, stagedFiles)), nil
	case goMissingModeAll:
		return collectGoMissingAll(cfg, projectRoot)
	default:
		return nil, fmt.Errorf("goMissingTestsCheckConfig.mode: unknown value %q (want %q or %q)", mode, goMissingModeStaged, goMissingModeAll)
	}
}

// goStagedSourceDirs is the set of dirs containing a staged in-scope source file.
func goStagedSourceDirs(cfg GoMissingTestsCheckConfig, projectRoot string, stagedFiles []string) map[string]bool {
	dirs := map[string]bool{}
	for _, f := range stagedFiles {
		abs := f
		if !filepath.IsAbs(abs) {
			abs = filepath.Join(projectRoot, abs)
		}
		if goNeedsTest(cfg, projectRoot, abs) {
			dirs[filepath.Dir(abs)] = true
		}
	}
	return dirs
}

func missingDirsFromSet(dirs map[string]bool) []string {
	var missing []string
	for dir := range dirs {
		if !dirHasGoTest(dir) {
			missing = append(missing, dir)
		}
	}
	sort.Strings(missing)
	return missing
}

func collectGoMissingAll(cfg GoMissingTestsCheckConfig, projectRoot string) ([]string, error) {
	roots := cfg.AppPaths
	if len(roots) == 0 {
		roots = []string{"."}
	}
	sourceDirs := map[string]bool{}
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
				if d.Name() == "vendor" || d.Name() == ".git" || d.Name() == "node_modules" {
					return filepath.SkipDir
				}
				return nil
			}
			if goNeedsTest(cfg, projectRoot, path) {
				sourceDirs[filepath.Dir(path)] = true
			}
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("scanning %s: %w", root, err)
		}
	}
	return missingDirsFromSet(sourceDirs), nil
}

// runGoMissingTestsCheck is the pre-commit entry point.
func runGoMissingTestsCheck(cfg GoMissingTestsCheckConfig, projectRoot string, stagedFiles []string) error {
	if !compactMode() {
		fmt.Println("================================")
		fmt.Println("  GO MISSING TESTS CHECK")
		fmt.Println("================================")
	}

	mode := cfg.Mode
	if mode == "" {
		mode = goMissingModeStaged
	}

	missing, err := collectGoMissingPackages(cfg, projectRoot, stagedFiles, mode)
	if err != nil {
		if compactMode() {
			printStatus("Go missing tests", false, err.Error())
		} else {
			fmt.Printf("❌ Go missing tests error: %v\n\n", err)
		}
		return err
	}

	if len(missing) == 0 {
		_ = writeRunReport("go-missing-tests", "Go missing tests", "", false)
		if compactMode() {
			printStatus("Go missing tests", true, "")
		} else {
			fmt.Println("✅ Go missing tests check passed")
			fmt.Println()
		}
		return nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Found %d Go package(s) with source but no _test.go:\n\n", len(missing))
	for _, d := range missing {
		rel, rerr := filepath.Rel(projectRoot, d)
		if rerr != nil {
			rel = d
		}
		fmt.Fprintf(&sb, "  • %s\n", filepath.ToSlash(rel))
	}
	sb.WriteString("\nAdd at least one *_test.go to each package, or exclude the path via\n")
	sb.WriteString("goMissingTestsCheckConfig.excludePaths.\n")

	_ = writeRunReport("go-missing-tests", "Go missing tests", sb.String(), true)

	if compactMode() {
		printStatus("Go missing tests", false, fmt.Sprintf("%d package(s)", len(missing)))
		printReportHint("go-missing-tests/")
	} else {
		fmt.Print("❌ " + sb.String())
		fmt.Println()
	}
	return fmt.Errorf("found %d Go package(s) without tests", len(missing))
}
