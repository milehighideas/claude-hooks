package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteRunReport_PassWritesOnlyFullreport(t *testing.T) {
	dir := t.TempDir()
	prev := reportDir
	reportDir = dir
	defer func() { reportDir = prev }()

	if err := writeRunReport("mock-check", "Mock check", "", false); err != nil {
		t.Fatalf("writeRunReport: %v", err)
	}

	full := readFile(t, filepath.Join(dir, "mock-check", "fullreport.txt"))
	if !strings.Contains(full, "Result: PASSED") {
		t.Errorf("fullreport missing PASSED banner; got:\n%s", full)
	}
	if !strings.Contains(full, "✅ Mock check passed") {
		t.Errorf("fullreport missing passed line; got:\n%s", full)
	}
	if _, err := os.Stat(filepath.Join(dir, "mock-check", "findings.txt")); !os.IsNotExist(err) {
		t.Errorf("expected no findings.txt on pass, stat err=%v", err)
	}
}

func TestWriteRunReport_FailWritesBoth(t *testing.T) {
	dir := t.TempDir()
	prev := reportDir
	reportDir = dir
	defer func() { reportDir = prev }()

	output := "compiling...\nApp/Foo.swift:12:3: error: cannot find 'Bar' in scope\nBuild failed\n"
	if err := writeRunReport("native-build", "Native build", output, true); err != nil {
		t.Fatalf("writeRunReport: %v", err)
	}

	full := readFile(t, filepath.Join(dir, "native-build", "fullreport.txt"))
	if !strings.Contains(full, "Result: FAILED") || !strings.Contains(full, "cannot find 'Bar'") {
		t.Errorf("fullreport should carry FAILED banner + raw output; got:\n%s", full)
	}

	findings := readFile(t, filepath.Join(dir, "native-build", "findings.txt"))
	// Only the error/failure lines survive into findings, not the "compiling..." noise.
	if !strings.Contains(findings, "error: cannot find 'Bar' in scope") {
		t.Errorf("findings missing the error line; got:\n%s", findings)
	}
	if !strings.Contains(findings, "Build failed") {
		t.Errorf("findings missing the failure line; got:\n%s", findings)
	}
	if strings.Contains(findings, "compiling...") {
		t.Errorf("findings should exclude non-error noise; got:\n%s", findings)
	}
}

func TestWriteRunReport_NoReportDirIsNoOp(t *testing.T) {
	prev := reportDir
	reportDir = ""
	defer func() { reportDir = prev }()
	if err := writeRunReport("x", "X", "boom error", true); err != nil {
		t.Errorf("expected no-op nil when reportDir unset, got %v", err)
	}
}

func TestExtractErrorLines(t *testing.T) {
	out := "line ok\n  3 passing\nFAIL src/x.test.ts\n  ✕ does a thing\nTypeError: boom\n"
	body, count := extractErrorLines(out)
	if count < 2 {
		t.Errorf("expected >=2 error lines, got %d (body:\n%s)", count, body)
	}
	for _, want := range []string{"FAIL src/x.test.ts", "✕ does a thing"} {
		if !strings.Contains(body, want) {
			t.Errorf("extractErrorLines dropped %q; got:\n%s", want, body)
		}
	}
	if strings.Contains(body, "3 passing") {
		t.Errorf("extractErrorLines should skip non-error lines; got:\n%s", body)
	}

	// No markers → falls back to trailing summary, count 0.
	body2, count2 := extractErrorLines("all good\nlooks fine\n")
	if count2 != 0 || !strings.Contains(body2, "looks fine") {
		t.Errorf("fallback path wrong: count=%d body=%q", count2, body2)
	}
}
