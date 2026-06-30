package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
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

// errorLineRe matches output lines worth surfacing as findings: compiler/tool
// errors, failures, and the test-runner failure glyphs. Broad on purpose — for
// raw command output (xcodebuild, gradle, expo, tsc) the actionable lines are
// the ones mentioning error/fail/fatal/etc.
var errorLineRe = regexp.MustCompile(`(?i)(\berror\b|\berrors\b|\bfail(ed|ure|ing)?\b|\bfatal\b|\bpanic\b|\bexception\b|\bwarning\b|assertionerror|\bexpected\b|\breceived\b|\bunresolved\b|\bundefined\b|\bundeclared\b|\bcannot\b|\bmissing\b)|[✕×✗✖●❌]`)

// extractErrorLines pulls the error/failure-relevant lines out of raw command or
// tool output and counts them. When nothing matches it falls back to the
// trailing summary so a failed run's findings are never empty. Shared by every
// command-style check and by the test runner.
func extractErrorLines(output string) (string, int) {
	var b strings.Builder
	count := 0
	for _, line := range strings.Split(output, "\n") {
		if errorLineRe.MatchString(line) {
			b.WriteString(line)
			b.WriteByte('\n')
			count++
		}
	}
	if b.Len() == 0 {
		lines := strings.Split(strings.TrimRight(output, "\n"), "\n")
		if len(lines) > 40 {
			lines = lines[len(lines)-40:]
		}
		return strings.Join(lines, "\n") + "\n", 0
	}
	return b.String(), count
}

// runReportContent builds the (fullreport, findings) pair for a run-style check
// from its raw output. findings is empty unless failed. app is used only for the
// findings banner label (may be empty).
func runReportContent(title, app, output string, failed bool) (full, findings string) {
	var fb strings.Builder
	fb.WriteString(strings.Repeat("=", 80) + "\n")
	fmt.Fprintf(&fb, "%s\n", strings.ToUpper(title))
	fmt.Fprintf(&fb, "Generated: %s\n", time.Now().Format("2006-01-02 15:04:05"))
	if failed {
		fb.WriteString("Result: FAILED\n")
	} else {
		fb.WriteString("Result: PASSED\n")
	}
	fb.WriteString(strings.Repeat("=", 80) + "\n\n")
	if strings.TrimSpace(output) == "" {
		if failed {
			fb.WriteString("(no output captured)\n")
		} else {
			fmt.Fprintf(&fb, "✅ %s passed\n", title)
		}
	} else {
		fb.WriteString(output)
		if !strings.HasSuffix(output, "\n") {
			fb.WriteByte('\n')
		}
	}

	if failed {
		body, n := extractErrorLines(output)
		if n == 0 {
			n = 1
		}
		findings = findingsDoc(title, app, n, body)
	}
	return fb.String(), findings
}

// writeRunReport is the universal per-check report entry point. It always writes
// <reportDir>/<subdir>/fullreport.txt (the check's full output, or a passed
// banner), and writes findings.txt only when the check failed. It's a no-op when
// reportDir is unset. Checks with richer per-app breakdowns still write their own
// <subdir>/<app>/ detail on top of this.
func writeRunReport(subdir, title, output string, failed bool) error {
	if reportDir == "" {
		return nil
	}
	full, findings := runReportContent(title, "", output, failed)
	return writeDualReportFlat(reportDir, subdir, findings, full)
}

// writeAppRunReport is the per-app variant: it writes
// <reportDir>/<subdir>/<app>/{findings,fullreport}.txt for run-style checks that
// execute once per app (native build, bundle check).
func writeAppRunReport(subdir, app, title, output string, failed bool) error {
	if reportDir == "" {
		return nil
	}
	full, findings := runReportContent(title, app, output, failed)
	return writeDualReport(reportDir, subdir, app, findings, full)
}
