package main

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/milehighideas/claude-hooks/internal/schemachecks"
)

// Mode constants mirror the other scoped checks.
const (
	createdAtModeAll    = "all"
	createdAtModeStaged = "staged"
)

// redundantCreatedAtReport is the result of a scan: the paths of every
// Convex schema file that currently contains at least one `createdAt:`
// inside a `defineTable({...})` block.
type redundantCreatedAtReport struct {
	Violations []string
}

// collectRedundantCreatedAtReport walks the configured scope and returns the
// set of schema files that currently violate the rule. Paths are absolute.
func collectRedundantCreatedAtReport(
	cfg RedundantCreatedAtCheckConfig,
	projectRoot string,
	stagedFiles []string,
) (*redundantCreatedAtReport, error) {
	report := &redundantCreatedAtReport{}
	mode := cfg.Mode
	if mode == "" {
		mode = createdAtModeAll
	}

	inScope := func(path string) bool {
		return createdAtPathInScope(cfg, projectRoot, path)
	}

	switch mode {
	case createdAtModeStaged:
		for _, f := range stagedFiles {
			if !schemachecks.IsSchemaFile(f) {
				continue
			}
			if !inScope(f) {
				continue
			}
			if schemachecks.CheckFile(f) {
				report.Violations = append(report.Violations, f)
			}
		}
	case createdAtModeAll:
		roots := createdAtScanRoots(cfg, projectRoot)
		for _, r := range roots {
			found, err := schemachecks.Find(r)
			if err != nil {
				return nil, fmt.Errorf("scanning %s: %w", r, err)
			}
			for _, p := range found {
				if !inScope(p) {
					continue
				}
				report.Violations = append(report.Violations, p)
			}
		}
	default:
		return nil, fmt.Errorf(
			"redundantCreatedAtCheckConfig.mode: unknown value %q (want %q or %q)",
			mode, createdAtModeAll, createdAtModeStaged,
		)
	}

	return report, nil
}

// createdAtScanRoots returns the absolute directories the walker should
// descend from. When AppPaths is set, each (valid) app path becomes a root;
// otherwise projectRoot itself.
func createdAtScanRoots(cfg RedundantCreatedAtCheckConfig, projectRoot string) []string {
	if len(cfg.AppPaths) == 0 {
		return []string{projectRoot}
	}
	var roots []string
	for _, ap := range cfg.AppPaths {
		abs := ap
		if !filepath.IsAbs(abs) {
			abs = filepath.Join(projectRoot, abs)
		}
		roots = append(roots, abs)
	}
	return roots
}

// createdAtPathInScope returns true when path falls inside the configured
// AppPaths (or there are no AppPaths restricting scope) and isn't in
// ExcludePaths. Matching is substring-based on the project-relative path,
// mirroring the other scoped checks.
func createdAtPathInScope(cfg RedundantCreatedAtCheckConfig, projectRoot, path string) bool {
	rel := path
	if r, err := filepath.Rel(projectRoot, path); err == nil {
		rel = r
	}
	rel = filepath.ToSlash(rel)

	for _, ex := range cfg.ExcludePaths {
		if ex == "" {
			continue
		}
		if strings.Contains(rel, filepath.ToSlash(ex)) {
			return false
		}
	}

	if len(cfg.AppPaths) == 0 {
		return true
	}
	for _, ap := range cfg.AppPaths {
		if ap == "" {
			continue
		}
		if strings.Contains(rel, filepath.ToSlash(ap)) {
			return true
		}
	}
	return false
}

// runRedundantCreatedAtCheck is the entry point pre-commit calls when the
// feature flag is enabled. Mirrors runStubTestCheck's output shape:
// compact-mode status line in reportDir runs, verbose block on direct
// invocations.
func runRedundantCreatedAtCheck(
	cfg RedundantCreatedAtCheckConfig,
	projectRoot string,
	stagedFiles []string,
) error {
	if !compactMode() {
		fmt.Println("================================")
		fmt.Println("  REDUNDANT createdAt CHECK")
		fmt.Println("================================")
	}

	report, err := collectRedundantCreatedAtReport(cfg, projectRoot, stagedFiles)
	if err != nil {
		if compactMode() {
			printStatus("Redundant createdAt", false, err.Error())
		} else {
			fmt.Printf("❌ Redundant createdAt check error: %v\n\n", err)
		}
		return err
	}

	count := len(report.Violations)
	if compactMode() {
		if count > 0 {
			printStatus("Redundant createdAt", false, fmt.Sprintf("%d schema file(s)", count))
			return fmt.Errorf("found %d schema file(s) with redundant createdAt", count)
		}
		printStatus("Redundant createdAt", true, "")
		return nil
	}

	if count == 0 {
		fmt.Println("✅ No redundant createdAt fields in Convex schema")
		fmt.Println()
		return nil
	}

	fmt.Printf("❌ Found %d schema file(s) with redundant createdAt in defineTable:\n\n", count)
	for _, p := range report.Violations {
		rel, err := filepath.Rel(projectRoot, p)
		if err != nil {
			rel = p
		}
		fmt.Printf("  • %s\n", rel)
	}
	fmt.Println()
	fmt.Println("Convex automatically maintains `_creationTime: number` on every row and")
	fmt.Println("exposes a `by_creation_time` index for free. A custom `createdAt` column")
	fmt.Println("inside defineTable({...}) duplicates that data and risks drift when")
	fmt.Println("callers pass a different value.")
	fmt.Println()
	fmt.Println("Use `row._creationTime` in queries. If you need a semantically different")
	fmt.Println("timestamp, rename to reflect its meaning: activatedAt, publishedAt, etc.")
	fmt.Println()
	return fmt.Errorf("found %d schema file(s) with redundant createdAt", count)
}
