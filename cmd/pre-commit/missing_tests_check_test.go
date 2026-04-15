package main

import (
	"os"
	"path/filepath"
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
