package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeMissingTestsTree(t *testing.T) string {
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

	src := `export const x = 1;`

	// apps/web source files
	write("apps/web/components/Button.tsx", src)       // missing test — should flag
	write("apps/web/components/Button.test.tsx", src)  // pretend test exists
	write("apps/web/components/Header.tsx", src)       // missing test — should flag
	write("apps/web/components/index.ts", src)         // barrel, skip
	write("apps/web/types/User.ts", src)               // types/ folder, skip
	write("apps/web/lib/tailwind.config.ts", src)      // config, skip

	// packages/backend source files
	write("packages/backend/convex/users.ts", src)       // missing test — should flag
	write("packages/backend/convex/users.test.ts", src)  // pretend test exists
	write("packages/backend/convex/posts.ts", src)       // missing test — should flag
	write("packages/backend/convex/_generated/api.d.ts", src) // d.ts in generated, skip
	write("packages/backend/convex/schema.types.ts", src) // .types.ts, skip

	// apps/mobile — out of scope in later tests
	write("apps/mobile/components/Foo.tsx", src)         // missing test — should flag only when scoped

	// node_modules — always skipped
	write("node_modules/pkg/NMcomp.tsx", src)

	// test files themselves — never flagged
	write("apps/web/components/Alone.test.tsx", src)

	// spec file pretends to be a test
	write("apps/web/components/Other.tsx", src)
	write("apps/web/components/Other.spec.tsx", src)

	return root
}

func TestMissingTestsCheck_ModeAll_NoScope(t *testing.T) {
	root := writeMissingTestsTree(t)
	cfg := MissingTestsCheckConfig{} // default mode all, no scope

	report, err := collectMissingTestsReport(cfg, root, nil)
	if err != nil {
		t.Fatalf("collectMissingTestsReport: %v", err)
	}
	// Header.tsx ✗, posts.ts ✗, mobile/Foo.tsx ✗ — the other source files
	// in the fixture either have tests (Button/users/Other) or are skipped
	// (index, types, configs, __generated, node_modules, Alone.test).
	if got := len(report.Missing); got != 3 {
		t.Errorf("len = %d, want 3; got %v", got, report.Missing)
	}
}

func TestMissingTestsCheck_ModeAll_AppPaths(t *testing.T) {
	root := writeMissingTestsTree(t)
	cfg := MissingTestsCheckConfig{
		AppPaths: []string{"apps/web", "packages/backend"},
	}
	report, err := collectMissingTestsReport(cfg, root, nil)
	if err != nil {
		t.Fatalf("collectMissingTestsReport: %v", err)
	}
	// Same as no-scope but drops mobile/Foo.tsx (out of scope).
	if got := len(report.Missing); got != 2 {
		t.Errorf("len = %d, want 2; got %v", got, report.Missing)
	}
	for _, m := range report.Missing {
		if filepath.Base(m.Source) == "Foo.tsx" {
			t.Errorf("apps/mobile/Foo.tsx should be out of scope: %v", m)
		}
	}
}

func TestMissingTestsCheck_ModeAll_ExcludePaths(t *testing.T) {
	root := writeMissingTestsTree(t)
	cfg := MissingTestsCheckConfig{
		AppPaths:     []string{"apps/web", "packages/backend"},
		ExcludePaths: []string{"packages/backend/convex/posts"},
	}
	report, err := collectMissingTestsReport(cfg, root, nil)
	if err != nil {
		t.Fatalf("collectMissingTestsReport: %v", err)
	}
	// Web + backend in scope = 2, minus posts = 1 (just Header.tsx).
	if got := len(report.Missing); got != 1 {
		t.Errorf("len = %d, want 1; got %v", got, report.Missing)
	}
	for _, m := range report.Missing {
		if filepath.Base(m.Source) == "posts.ts" {
			t.Errorf("posts.ts should be excluded: %v", m)
		}
	}
}

func TestMissingTestsCheck_ModeStaged(t *testing.T) {
	root := writeMissingTestsTree(t)
	cfg := MissingTestsCheckConfig{Mode: "staged"}

	// Stage: one missing-test source, one that has a test, one non-source.
	missing := filepath.Join(root, "apps/web/components/Header.tsx")
	withTest := filepath.Join(root, "apps/web/components/Button.tsx")
	barrel := filepath.Join(root, "apps/web/components/index.ts")

	report, err := collectMissingTestsReport(cfg, root, []string{missing, withTest, barrel})
	if err != nil {
		t.Fatalf("collectMissingTestsReport: %v", err)
	}
	if got := len(report.Missing); got != 1 {
		t.Errorf("len = %d, want 1; got %v", got, report.Missing)
	}
	if report.Missing[0].Source != missing {
		t.Errorf("missing[0] = %q, want %q", report.Missing[0].Source, missing)
	}
}

func TestMissingTestsCheck_ModeStaged_AppPaths(t *testing.T) {
	root := writeMissingTestsTree(t)
	cfg := MissingTestsCheckConfig{
		Mode:     "staged",
		AppPaths: []string{"apps/web"},
	}
	webMissing := filepath.Join(root, "apps/web/components/Header.tsx")
	backendMissing := filepath.Join(root, "packages/backend/convex/users.ts")

	report, err := collectMissingTestsReport(cfg, root, []string{webMissing, backendMissing})
	if err != nil {
		t.Fatalf("collectMissingTestsReport: %v", err)
	}
	if got := len(report.Missing); got != 1 {
		t.Errorf("len = %d, want 1; got %v", got, report.Missing)
	}
	if report.Missing[0].Source != webMissing {
		t.Errorf("missing[0] = %q, want %q", report.Missing[0].Source, webMissing)
	}
}

// AppModes mixes a ratcheted path ("all") with globally-staged paths.
func TestMissingTestsCheck_AppModes_RatchetOneAppStagedRest(t *testing.T) {
	root := writeMissingTestsTree(t)
	cfg := MissingTestsCheckConfig{
		Mode: "staged",
		AppPaths: []string{
			"apps/web",
			"packages/backend",
		},
		AppModes: map[string]string{
			"apps/web": "all",
		},
	}
	// Stage a backend missing-test source. The apps/web sources are NOT
	// staged — but the ratchet should still flag every missing test under it.
	stagedBackend := filepath.Join(root, "packages/backend/convex/posts.ts")

	report, err := collectMissingTestsReport(cfg, root, []string{stagedBackend})
	if err != nil {
		t.Fatalf("collectMissingTestsReport: %v", err)
	}
	// apps/web/Header.tsx (ratchet) + packages/backend/posts.ts (staged).
	if got := len(report.Missing); got != 2 {
		t.Errorf("len = %d, want 2; got %v", got, report.Missing)
	}
	want := map[string]bool{
		filepath.Join(root, "apps/web/components/Header.tsx"): false,
		stagedBackend: false,
	}
	for _, m := range report.Missing {
		if _, ok := want[m.Source]; ok {
			want[m.Source] = true
		}
	}
	for src, found := range want {
		if !found {
			t.Errorf("missing expected source %q in %v", src, report.Missing)
		}
	}
}

// A path listed only in AppModes brings itself into scope.
func TestMissingTestsCheck_AppModes_PathNotInAppPaths(t *testing.T) {
	root := writeMissingTestsTree(t)
	cfg := MissingTestsCheckConfig{
		Mode: "staged",
		AppModes: map[string]string{
			"apps/web": "all",
		},
	}

	report, err := collectMissingTestsReport(cfg, root, nil)
	if err != nil {
		t.Fatalf("collectMissingTestsReport: %v", err)
	}
	// Only apps/web/Header.tsx.
	if got := len(report.Missing); got != 1 {
		t.Errorf("len = %d, want 1; got %v", got, report.Missing)
	}
	want := filepath.Join(root, "apps/web/components/Header.tsx")
	if len(report.Missing) > 0 && report.Missing[0].Source != want {
		t.Errorf("got %q, want %q", report.Missing[0].Source, want)
	}
}

// AppModes overrides AppPaths' inherited mode.
func TestMissingTestsCheck_AppModes_OverridesAppPaths(t *testing.T) {
	root := writeMissingTestsTree(t)
	cfg := MissingTestsCheckConfig{
		Mode:     "all",
		AppPaths: []string{"apps/web"},
		AppModes: map[string]string{
			"apps/web": "staged", // override: only staged gets flagged
		},
	}

	// No staged files — staged override yields zero.
	report, err := collectMissingTestsReport(cfg, root, nil)
	if err != nil {
		t.Fatalf("collectMissingTestsReport: %v", err)
	}
	if got := len(report.Missing); got != 0 {
		t.Errorf("len = %d, want 0; got %v", got, report.Missing)
	}

	// Stage a missing-test source — now flagged.
	stagedMissing := filepath.Join(root, "apps/web/components/Header.tsx")
	report, err = collectMissingTestsReport(cfg, root, []string{stagedMissing})
	if err != nil {
		t.Fatalf("collectMissingTestsReport (staged): %v", err)
	}
	if got := len(report.Missing); got != 1 || report.Missing[0].Source != stagedMissing {
		t.Errorf("got %v, want 1 missing %q", report.Missing, stagedMissing)
	}
}

// Files matched by both an "all" and a "staged" bucket are only reported once.
func TestMissingTestsCheck_AppModes_DedupesAcrossBuckets(t *testing.T) {
	root := writeMissingTestsTree(t)
	cfg := MissingTestsCheckConfig{
		Mode: "staged",
		AppModes: map[string]string{
			"apps":     "all",    // matches apps/web/Header.tsx + apps/mobile/Foo.tsx
			"apps/web": "staged", // also matches apps/web/Header.tsx
		},
	}
	stagedWeb := filepath.Join(root, "apps/web/components/Header.tsx")

	report, err := collectMissingTestsReport(cfg, root, []string{stagedWeb})
	if err != nil {
		t.Fatalf("collectMissingTestsReport: %v", err)
	}
	// apps/web/Header.tsx and apps/mobile/Foo.tsx (both via "all"). Without
	// dedup, Header.tsx would appear twice.
	if got := len(report.Missing); got != 2 {
		t.Errorf("len = %d, want 2; got %v", got, report.Missing)
	}
	seen := map[string]int{}
	for _, m := range report.Missing {
		seen[m.Source]++
	}
	for path, count := range seen {
		if count > 1 {
			t.Errorf("duplicate report for %q: %d times", path, count)
		}
	}
}

// Invalid mode in AppModes returns a descriptive error.
func TestMissingTestsCheck_AppModes_InvalidMode(t *testing.T) {
	root := writeMissingTestsTree(t)
	cfg := MissingTestsCheckConfig{
		AppModes: map[string]string{
			"apps/web": "ratchet",
		},
	}

	_, err := collectMissingTestsReport(cfg, root, nil)
	if err == nil {
		t.Fatalf("expected error for invalid AppModes value, got nil")
	}
	if !strings.Contains(err.Error(), "apps/web") {
		t.Errorf("error should mention path; got %q", err)
	}
	if !strings.Contains(err.Error(), "ratchet") {
		t.Errorf("error should mention the invalid value; got %q", err)
	}
}

func TestNeedsTest_SkipsCorrectly(t *testing.T) {
	cases := []struct {
		path     string
		expected bool
	}{
		// Valid source files that need tests
		{"apps/web/components/Button.tsx", true},
		{"apps/web/hooks/useAuth.ts", true},
		{"packages/backend/convex/users.ts", true},
		{"packages/backend/convex/nested/deep.ts", true},

		// Skipped paths and extensions
		{"apps/web/components/Button.test.tsx", false}, // test
		{"apps/web/components/Button.spec.tsx", false}, // spec
		{"apps/web/components/Button.e2e.ts", false},   // e2e
		{"apps/web/components/Home.maestro.yaml", false}, // maestro / wrong ext
		{"apps/web/components/index.ts", false},        // barrel
		{"apps/web/components/index.tsx", false},       // barrel
		{"apps/web/types/User.ts", false},              // /types/ folder
		{"apps/web/lib/User.types.ts", false},          // .types.ts
		{"apps/web/lib/api.d.ts", false},               // .d.ts
		{"apps/web/_layout.tsx", false},                // layout
		{"apps/web/tailwind.config.ts", false},         // config
		{"apps/web/vitest.config.ts", false},           // config
		{"packages/backend/convex/_generated/api.ts", false}, // _generated
		{"apps/web/components/__mocks__/api.ts", false},      // mocks
		{"apps/web/README.md", false},                  // wrong ext
		{"apps/web/package.json", false},               // wrong ext
	}

	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			got := needsTest(tc.path)
			if got != tc.expected {
				t.Errorf("needsTest(%q) = %v, want %v", tc.path, got, tc.expected)
			}
		})
	}
}
