package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Report writers in this package emit two files per subject instead of one:
//
//   - findings.txt   only the blocking, actionable lines (what failed the hook)
//   - fullreport.txt the complete verbose report (identical to the legacy
//                    single <subdir>/<app>.txt content)
//
// writeDualReport lays them out per app at <baseDir>/<subdir>/<app>/, and
// writeDualReportFlat lays them out at <baseDir>/<subdir>/ for the aggregate
// checks that have no per-app dimension (test-coverage, vitest-assertions,
// test-quality). Both files are always written so a clean run overwrites stale
// output from a previous one.

// writeDualReport writes <baseDir>/<subdir>/<app>/{findings,fullreport}.txt.
func writeDualReport(baseDir, subdir, app, findings, full string) error {
	dir := filepath.Join(baseDir, subdir, app)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "findings.txt"), []byte(findings), 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "fullreport.txt"), []byte(full), 0o644)
}

// writeDualReportFlat writes <baseDir>/<subdir>/{findings,fullreport}.txt for
// checks that aggregate every app into a single report.
func writeDualReportFlat(baseDir, subdir, findings, full string) error {
	dir := filepath.Join(baseDir, subdir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "findings.txt"), []byte(findings), 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "fullreport.txt"), []byte(full), 0o644)
}

// findingsHeader builds the standard findings.txt banner. app may be empty for
// aggregate (flat) reports.
func findingsHeader(title, app string, count int) string {
	var sb strings.Builder
	sb.WriteString(strings.Repeat("=", 80) + "\n")
	if app != "" {
		fmt.Fprintf(&sb, "%s — FINDINGS: %s\n", title, app)
	} else {
		fmt.Fprintf(&sb, "%s — FINDINGS\n", title)
	}
	fmt.Fprintf(&sb, "Generated: %s\n", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Fprintf(&sb, "Blocking findings: %d\n", count)
	sb.WriteString(strings.Repeat("=", 80) + "\n\n")
	return sb.String()
}

// findingsDoc builds a complete findings.txt: the standard header plus body.
// When count is 0 the body is replaced with an explicit "No blocking findings."
// line so the file is never ambiguous.
func findingsDoc(title, app string, count int, body string) string {
	h := findingsHeader(title, app, count)
	if count == 0 {
		return h + "No blocking findings.\n"
	}
	return h + body
}
