package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Report writers in this package emit up to two files per subject:
//
//   - findings.txt   only the blocking, actionable lines (what failed the hook).
//                    Written ONLY when there are findings — a clean subject gets
//                    no findings.txt at all.
//   - fullreport.txt the complete verbose report (identical to the legacy
//                    single <subdir>/<app>.txt content). Always written.
//
// writeDualReport lays them out per app at <baseDir>/<subdir>/<app>/, and
// writeDualReportFlat lays them out at <baseDir>/<subdir>/ for the aggregate
// checks that have no per-app dimension (test-coverage, vitest-assertions,
// test-quality).

// writeReportPair writes fullreport.txt to dir, and findings.txt only when
// findings is non-empty. Any stale findings.txt from a previous run is removed
// when there are no findings this time.
func writeReportPair(dir, findings, full string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	findingsPath := filepath.Join(dir, "findings.txt")
	if findings == "" {
		// No findings — make sure a stale file from a prior run is gone.
		_ = os.Remove(findingsPath)
	} else if err := os.WriteFile(findingsPath, []byte(findings), 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "fullreport.txt"), []byte(full), 0o644)
}

// writeDualReport writes <baseDir>/<subdir>/<app>/fullreport.txt and, when there
// are findings, findings.txt alongside it.
func writeDualReport(baseDir, subdir, app, findings, full string) error {
	return writeReportPair(filepath.Join(baseDir, subdir, app), findings, full)
}

// writeDualReportFlat writes <baseDir>/<subdir>/fullreport.txt and, when there
// are findings, findings.txt alongside it, for checks that aggregate every app
// into a single report.
func writeDualReportFlat(baseDir, subdir, findings, full string) error {
	return writeReportPair(filepath.Join(baseDir, subdir), findings, full)
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
// When count is 0 it returns the empty string, which signals the writers to skip
// findings.txt entirely (a clean subject gets no findings file).
func findingsDoc(title, app string, count int, body string) string {
	if count == 0 {
		return ""
	}
	return findingsHeader(title, app, count) + body
}
