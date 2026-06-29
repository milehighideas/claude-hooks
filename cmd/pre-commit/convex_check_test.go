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
	// severity warn + no crudDomains => no-op (returns nil even with violations)
	cfg := ConvexCheckConfig{Severity: "warn"}
	if err := runConvexCheck(cfg, "/repo", nil); err != nil {
		t.Fatalf("expected nil when dormant, got %v", err)
	}
}

func TestRulesToEnforce(t *testing.T) {
	if r := rulesToEnforce(ConvexCheckConfig{Severity: "warn"}); r != nil {
		t.Errorf("warn + no crudDomains should enforce no rules, got %v", r)
	}
	if r := rulesToEnforce(ConvexCheckConfig{Severity: "warn", CrudDomains: []string{"convex/vehicles"}}); len(r) != 1 || r[0] != "crud-structure" {
		t.Errorf("warn + crudDomains should enforce only crud-structure, got %v", r)
	}
	if r := rulesToEnforce(ConvexCheckConfig{Severity: "error"}); len(r) != len(allConvexRules)+1 {
		t.Errorf("error should enforce all rules + crud-structure, got %v", r)
	}
}
