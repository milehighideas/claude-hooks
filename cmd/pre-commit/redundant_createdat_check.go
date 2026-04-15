package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

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

	// Write report file if reportDir is set and there are violations so
	// callers can inspect findings without rerunning the check.
	if reportDir != "" && count > 0 {
		if err := writeRedundantCreatedAtReport(report.Violations, projectRoot, reportDir); err != nil {
			fmt.Printf("   Warning: failed to write redundant createdAt report: %v\n", err)
		}
	}

	if compactMode() {
		if count > 0 {
			printStatus("Redundant createdAt", false, fmt.Sprintf("%d schema file(s)", count))
			printReportHint("redundant-createdat/")
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

// writeRedundantCreatedAtReport writes one
// <baseDir>/redundant-createdat/<app>.txt file per app/package with schema
// violations, matching the srp/<app>.txt and typecheck/<app>.txt
// conventions. Each entry includes the per-file createdAt count so heavy
// offenders stand out within an app.
func writeRedundantCreatedAtReport(violations []string, projectRoot, baseDir string) error {
	outDir := filepath.Join(baseDir, "redundant-createdat")
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return err
	}

	type entry struct {
		path  string
		count int
	}
	byApp := make(map[string][]entry)
	for _, p := range violations {
		rel, err := filepath.Rel(projectRoot, p)
		if err != nil {
			rel = p
		}
		rel = filepath.ToSlash(rel)
		// Count occurrences — best-effort; on read error we report the
		// violation with count 0 rather than skip it entirely.
		count := 0
		if src, err := os.ReadFile(p); err == nil {
			count = schemachecks.CountCreatedAt(string(src))
		}
		byApp[appNameFromRel(rel)] = append(byApp[appNameFromRel(rel)], entry{path: rel, count: count})
	}

	generated := time.Now().Format("2006-01-02 15:04:05")
	for app, entries := range byApp {
		// Sort by count desc so heavy offenders bubble to the top.
		sort.Slice(entries, func(i, j int) bool {
			if entries[i].count != entries[j].count {
				return entries[i].count > entries[j].count
			}
			return entries[i].path < entries[j].path
		})

		appTotal := 0
		for _, e := range entries {
			appTotal += e.count
		}

		var sb strings.Builder
		sb.WriteString(strings.Repeat("=", 80) + "\n")
		sb.WriteString(fmt.Sprintf("REDUNDANT createdAt - %s\n", strings.ToUpper(app)))
		sb.WriteString(fmt.Sprintf("Generated: %s\n", generated))
		sb.WriteString(strings.Repeat("=", 80) + "\n\n")
		sb.WriteString(fmt.Sprintf("Schema files with createdAt in defineTable: %d\n", len(entries)))
		sb.WriteString(fmt.Sprintf("Total createdAt occurrences: %d\n\n", appTotal))
		sb.WriteString("Convex automatically maintains `_creationTime: number` on every row.\n")
		sb.WriteString("A custom `createdAt` column inside defineTable({...}) duplicates that\n")
		sb.WriteString("data and risks drift when callers pass a different value.\n\n")
		sb.WriteString("Remediation: use `row._creationTime` in queries and sort via the\n")
		sb.WriteString("`by_creation_time` index. If you need a semantically different\n")
		sb.WriteString("timestamp, rename the field: activatedAt, publishedAt, verifiedAt.\n\n")
		sb.WriteString(strings.Repeat("-", 40) + "\n")
		sb.WriteString("VIOLATIONS (count = occurrences in that file)\n")
		sb.WriteString(strings.Repeat("-", 40) + "\n\n")
		for _, e := range entries {
			sb.WriteString(fmt.Sprintf("  [%3d]  %s\n", e.count, e.path))
		}

		reportPath := filepath.Join(outDir, app+".txt")
		if err := os.WriteFile(reportPath, []byte(sb.String()), 0644); err != nil {
			return err
		}
	}

	return nil
}
