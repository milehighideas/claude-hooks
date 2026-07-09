package main

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

// goTestTarget is one `go test` invocation: a directory and a package pattern.
type goTestTarget struct {
	dir     string // absolute
	pattern string // "./..." (whole module) or "." (single package)
}

// goTestTargets computes the set of `go test` invocations for the staged Go
// files under the configured modules.
func goTestTargets(cfg GoTestsConfig, projectRoot string, goFiles []string) []goTestTarget {
	modules := cfg.Modules
	if len(modules) == 0 {
		modules = []string{"."}
	}
	var modRoots []string
	for _, m := range modules {
		modRoots = append(modRoots, filepath.Join(projectRoot, m))
	}

	abs := func(f string) string {
		if filepath.IsAbs(f) {
			return f
		}
		return filepath.Join(projectRoot, f)
	}

	if cfg.AffectedOnly {
		dirSet := map[string]bool{}
		for _, f := range goFiles {
			dir := filepath.Dir(abs(f))
			if underAnyModule(dir, modRoots) {
				dirSet[dir] = true
			}
		}
		var targets []goTestTarget
		for d := range dirSet {
			targets = append(targets, goTestTarget{dir: d, pattern: "."})
		}
		sort.Slice(targets, func(i, j int) bool { return targets[i].dir < targets[j].dir })
		return targets
	}

	var targets []goTestTarget
	for _, root := range modRoots {
		for _, f := range goFiles {
			a := abs(f)
			if a == root || strings.HasPrefix(a, root+string(filepath.Separator)) {
				targets = append(targets, goTestTarget{dir: root, pattern: "./..."})
				break
			}
		}
	}
	return targets
}

// underAnyModule reports whether dir is at or below any module root.
func underAnyModule(dir string, modRoots []string) bool {
	for _, root := range modRoots {
		if dir == root || strings.HasPrefix(dir, root+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

// runGoTests is the pre-commit entry point: runs `go test` on affected Go and
// blocks on failure. No-op when no Go files are staged.
func runGoTests(cfg GoTestsConfig, projectRoot string, stagedFiles []string) error {
	if !compactMode() {
		fmt.Println("================================")
		fmt.Println("  GO TESTS")
		fmt.Println("================================")
	}

	var goFiles []string
	for _, f := range stagedFiles {
		if strings.HasSuffix(f, ".go") {
			goFiles = append(goFiles, f)
		}
	}
	if len(goFiles) == 0 {
		if compactMode() {
			printStatus("Go tests", true, "no go files")
		} else {
			fmt.Println("No staged Go files — skipping")
			fmt.Println()
		}
		return nil
	}

	targets := goTestTargets(cfg, projectRoot, goFiles)
	if len(targets) == 0 {
		if compactMode() {
			printStatus("Go tests", true, "")
		}
		return nil
	}

	var failures []string
	var combined strings.Builder
	for _, tgt := range targets {
		if !compactMode() {
			fmt.Printf("   go test %s in %s ...\n", tgt.pattern, tgt.dir)
		}
		out, err := runCommandCapturedInDir(tgt.dir, "go", "test", tgt.pattern)
		if !compactMode() && out != "" {
			fmt.Print(out)
		}
		fmt.Fprintf(&combined, "===== %s %s =====\n%s\n", tgt.dir, tgt.pattern, out)
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", tgt.dir, err))
		}
	}

	failed := len(failures) > 0
	_ = writeRunReport("go-tests", "Go tests", combined.String(), failed)
	if failed {
		if compactMode() {
			printStatus("Go tests", false, fmt.Sprintf("%d target(s) failed", len(failures)))
			printReportHint("go-tests/")
		} else {
			fmt.Printf("❌ go test failed:\n  %s\n", strings.Join(failures, "\n  "))
		}
		return fmt.Errorf("go test failed:\n  %s", strings.Join(failures, "\n  "))
	}
	if compactMode() {
		printStatus("Go tests", true, "")
	} else {
		fmt.Println("✅ Go tests passed")
		fmt.Println()
	}
	return nil
}
