package main

import "testing"

func TestConvexFilesInScope(t *testing.T) {
	cfg := ConvexCheckConfig{
		AppPaths:     []string{"packages/backend/convex"},
		ExcludePaths: []string{"_generated/", "schema/"},
	}
	root := "/repo"
	staged := []string{
		"/repo/packages/backend/convex/events/eventsQueries.ts",
		"/repo/packages/backend/convex/_generated/api.d.ts", // excluded
		"/repo/packages/backend/convex/schema/events.ts",    // excluded
		"/repo/apps/story/foo.ts",                            // not in appPaths
		"/repo/packages/backend/convex/x.test.ts",           // test skip
	}
	got := convexFilesInScope(cfg, root, staged)
	if len(got) != 1 || got[0] != "/repo/packages/backend/convex/events/eventsQueries.ts" {
		t.Fatalf("unexpected scope result: %#v", got)
	}
}

func TestConvexCheckNoopWhenDormant(t *testing.T) {
	// no .convex-lint.json under projectRoot => empty config => dormant (nil)
	if err := runConvexCheck("/nonexistent-root", nil); err != nil {
		t.Fatalf("expected nil when dormant, got %v", err)
	}
}

func TestConvexRuleID(t *testing.T) {
	cases := map[string]string{
		"convex(type-exports-location)": "type-exports-location",
		"convex(file-size)":             "file-size",
		"eslint(no-unused-vars)":        "",
		"convex(":                       "",
	}
	for code, want := range cases {
		if got := convexRuleID(code); got != want {
			t.Errorf("convexRuleID(%q) = %q, want %q", code, got, want)
		}
	}
}
