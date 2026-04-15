package main

import (
	"os"
	"path/filepath"
	"strings"
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

// AppModes lets one specific path run in "all" (ratchet) mode while the
// rest of AppPaths stays in the global "staged" mode. The ratcheted path
// should flag every stub under it regardless of staging; the staged paths
// should still only flag currently-staged files.
func TestStubTestCheck_AppModes_RatchetOneAppStagedRest(t *testing.T) {
	root := writeStubTree(t)
	cfg := StubTestCheckConfig{
		Mode: "staged",
		AppPaths: []string{
			"apps/web",
			"apps/mobile",
			"packages/ui",
		},
		AppModes: map[string]string{
			"apps/web": "all", // ratchet: flag every stub under apps/web
		},
	}
	// Staged: a stub in apps/mobile and a stub in packages/ui. The apps/web
	// stub (a.test.tsx) is NOT staged — but the ratchet should still flag it.
	mobileStub := filepath.Join(root, "apps/mobile/c.test.tsx")
	uiStub := filepath.Join(root, "packages/ui/d.test.ts")

	report, err := collectStubReport(cfg, root, []string{mobileStub, uiStub})
	if err != nil {
		t.Fatalf("collectStubReport: %v", err)
	}
	// Expected: apps/web/a (ratchet), apps/mobile/c (staged), packages/ui/d (staged).
	// packages/legacy/e is not in scope.
	if len(report.Stubs) != 3 {
		t.Errorf("len = %d, want 3; got %v", len(report.Stubs), report.Stubs)
	}
	wantContains := []string{
		filepath.Join(root, "apps/web/a.test.tsx"),
		mobileStub,
		uiStub,
	}
	for _, want := range wantContains {
		found := false
		for _, s := range report.Stubs {
			if s == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing expected stub %q in %v", want, report.Stubs)
		}
	}
}

// A path listed only in AppModes (not in AppPaths) is still in scope for
// whichever mode it was assigned.
func TestStubTestCheck_AppModes_PathNotInAppPaths(t *testing.T) {
	root := writeStubTree(t)
	cfg := StubTestCheckConfig{
		Mode: "staged",
		// AppPaths is empty — but AppModes brings apps/web into scope.
		AppModes: map[string]string{
			"apps/web": "all",
		},
	}

	report, err := collectStubReport(cfg, root, nil)
	if err != nil {
		t.Fatalf("collectStubReport: %v", err)
	}
	// Only apps/web/a.test.tsx — the ratchet.
	if len(report.Stubs) != 1 {
		t.Errorf("len = %d, want 1; got %v", len(report.Stubs), report.Stubs)
	}
	want := filepath.Join(root, "apps/web/a.test.tsx")
	if len(report.Stubs) > 0 && report.Stubs[0] != want {
		t.Errorf("got %q, want %q", report.Stubs[0], want)
	}
}

// When a path appears in both AppPaths and AppModes, AppModes wins.
func TestStubTestCheck_AppModes_OverridesAppPaths(t *testing.T) {
	root := writeStubTree(t)
	cfg := StubTestCheckConfig{
		Mode:     "all",
		AppPaths: []string{"apps/web"}, // "all" by default
		AppModes: map[string]string{
			"apps/web": "staged", // override: only flag staged files
		},
	}
	// No staged files — "staged" override should yield zero findings despite
	// the walker being capable of finding apps/web/a.test.tsx under "all".
	report, err := collectStubReport(cfg, root, nil)
	if err != nil {
		t.Fatalf("collectStubReport: %v", err)
	}
	if len(report.Stubs) != 0 {
		t.Errorf("len = %d, want 0 (staged override with no staged files); got %v",
			len(report.Stubs), report.Stubs)
	}

	// Same config, but now stage a stub — it should be flagged.
	stagedStub := filepath.Join(root, "apps/web/a.test.tsx")
	report, err = collectStubReport(cfg, root, []string{stagedStub})
	if err != nil {
		t.Fatalf("collectStubReport (staged): %v", err)
	}
	if len(report.Stubs) != 1 || report.Stubs[0] != stagedStub {
		t.Errorf("len = %d stubs %v, want 1 staged stub %q",
			len(report.Stubs), report.Stubs, stagedStub)
	}
}

// A file matched by both an "all" path and a "staged" path must only be
// reported once.
func TestStubTestCheck_AppModes_DedupesAcrossBuckets(t *testing.T) {
	root := writeStubTree(t)
	cfg := StubTestCheckConfig{
		Mode: "staged",
		AppModes: map[string]string{
			"apps":     "all",    // matches apps/web/a.test.tsx
			"apps/web": "staged", // also matches apps/web/a.test.tsx
		},
	}
	// Stage the file that both buckets would match.
	stagedStub := filepath.Join(root, "apps/web/a.test.tsx")

	report, err := collectStubReport(cfg, root, []string{stagedStub})
	if err != nil {
		t.Fatalf("collectStubReport: %v", err)
	}
	// apps/web/a (matched once via "all") and apps/mobile/c (matched by "all"
	// under apps/). Without dedup, apps/web/a would report twice.
	if len(report.Stubs) != 2 {
		t.Errorf("len = %d, want 2 (deduped); got %v", len(report.Stubs), report.Stubs)
	}
	seen := map[string]int{}
	for _, s := range report.Stubs {
		seen[s]++
	}
	for path, count := range seen {
		if count > 1 {
			t.Errorf("duplicate report for %q: %d times", path, count)
		}
	}
}

// Invalid mode in AppModes returns a descriptive error.
func TestStubTestCheck_AppModes_InvalidMode(t *testing.T) {
	root := writeStubTree(t)
	cfg := StubTestCheckConfig{
		AppModes: map[string]string{
			"apps/web": "ratchet", // typo — valid values are "all" / "staged"
		},
	}

	_, err := collectStubReport(cfg, root, nil)
	if err == nil {
		t.Fatalf("expected error for invalid AppModes value, got nil")
	}
	if !strings.Contains(err.Error(), "apps/web") {
		t.Errorf("error should mention the offending path; got %q", err)
	}
	if !strings.Contains(err.Error(), "ratchet") {
		t.Errorf("error should mention the invalid value; got %q", err)
	}
}
