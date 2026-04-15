package main

import (
	"os"
	"path/filepath"
	"testing"
)

func writeStubTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()

	write := func(rel, content string) {
		full := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			t.Fatalf("mkdir %s: %v", rel, err)
		}
		if err := os.WriteFile(full, []byte(content), 0644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}

	stub := `it("x", () => { expect(true).toBe(true); });`
	real := `it("x", () => { expect(val).toBe(42); });`

	write("apps/web/a.test.tsx", stub)
	write("apps/web/b.test.tsx", real)
	write("apps/mobile/c.test.tsx", stub)
	write("packages/ui/d.test.ts", stub)
	write("packages/legacy/e.test.ts", stub)
	// Generated + node_modules stubs are always skipped by the walker.
	write("node_modules/pkg/nm.test.tsx", stub)
	write("packages/backend/_generated/gen.test.ts", stub)

	return root
}

func TestStubTestCheck_ModeAll_NoScope(t *testing.T) {
	root := writeStubTree(t)
	cfg := StubTestCheckConfig{} // mode default = "all", no scope limits

	report, err := collectStubReport(cfg, root, nil)
	if err != nil {
		t.Fatalf("collectStubReport: %v", err)
	}
	// Expect 4 stubs: apps/web/a, apps/mobile/c, packages/ui/d, packages/legacy/e
	if len(report.Stubs) != 4 {
		t.Errorf("len = %d, want 4; got %v", len(report.Stubs), report.Stubs)
	}
}

func TestStubTestCheck_ModeAll_AppPaths(t *testing.T) {
	root := writeStubTree(t)
	cfg := StubTestCheckConfig{
		AppPaths: []string{"apps/web"},
	}

	report, err := collectStubReport(cfg, root, nil)
	if err != nil {
		t.Fatalf("collectStubReport: %v", err)
	}
	if len(report.Stubs) != 1 {
		t.Errorf("len = %d, want 1; got %v", len(report.Stubs), report.Stubs)
	}
}

func TestStubTestCheck_ModeAll_ExcludePaths(t *testing.T) {
	root := writeStubTree(t)
	cfg := StubTestCheckConfig{
		ExcludePaths: []string{"packages/legacy"},
	}

	report, err := collectStubReport(cfg, root, nil)
	if err != nil {
		t.Fatalf("collectStubReport: %v", err)
	}
	// 4 stubs - 1 excluded = 3
	if len(report.Stubs) != 3 {
		t.Errorf("len = %d, want 3; got %v", len(report.Stubs), report.Stubs)
	}
	for _, s := range report.Stubs {
		if filepath.Base(s) == "e.test.ts" {
			t.Errorf("excluded file in output: %s", s)
		}
	}
}

func TestStubTestCheck_ModeStaged(t *testing.T) {
	root := writeStubTree(t)
	cfg := StubTestCheckConfig{
		Mode: "staged",
	}
	// Only one file staged, and it's a stub.
	stagedStub := filepath.Join(root, "apps/web/a.test.tsx")
	stagedReal := filepath.Join(root, "apps/web/b.test.tsx")
	stagedSrc := filepath.Join(root, "apps/web/foo.tsx") // non-test, ignored

	report, err := collectStubReport(cfg, root, []string{stagedStub, stagedReal, stagedSrc})
	if err != nil {
		t.Fatalf("collectStubReport: %v", err)
	}
	if len(report.Stubs) != 1 {
		t.Errorf("len = %d, want 1; got %v", len(report.Stubs), report.Stubs)
	}
	if report.Stubs[0] != stagedStub {
		t.Errorf("got %q, want %q", report.Stubs[0], stagedStub)
	}
}

func TestStubTestCheck_ModeStaged_AppPaths(t *testing.T) {
	root := writeStubTree(t)
	cfg := StubTestCheckConfig{
		Mode:     "staged",
		AppPaths: []string{"apps/web"},
	}
	// Staged stubs in two apps — only apps/web one should report.
	webStub := filepath.Join(root, "apps/web/a.test.tsx")
	mobileStub := filepath.Join(root, "apps/mobile/c.test.tsx")

	report, err := collectStubReport(cfg, root, []string{webStub, mobileStub})
	if err != nil {
		t.Fatalf("collectStubReport: %v", err)
	}
	if len(report.Stubs) != 1 {
		t.Errorf("len = %d, want 1; got %v", len(report.Stubs), report.Stubs)
	}
	if report.Stubs[0] != webStub {
		t.Errorf("got %q, want %q", report.Stubs[0], webStub)
	}
}
