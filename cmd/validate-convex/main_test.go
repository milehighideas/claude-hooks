package main

import "testing"

func TestIsConvexTarget(t *testing.T) {
	appPaths := []string{"packages/backend/convex"}
	exclude := []string{"_generated/"}
	cases := map[string]bool{
		"/r/packages/backend/convex/events/eventsQueries.ts": true,
		"/r/packages/backend/convex/_generated/api.d.ts":     false,
		"/r/packages/backend/convex/x.test.ts":               false,
		"/r/apps/story/foo.ts":                               false,
	}
	for path, want := range cases {
		if got := isConvexTarget(path, appPaths, exclude); got != want {
			t.Errorf("isConvexTarget(%q) = %v, want %v", path, got, want)
		}
	}
}

func TestContainsAny(t *testing.T) {
	if !containsAny("/r/packages/backend/convex/vehicles/read/getVehicle.ts", []string{"convex/vehicles"}) {
		t.Error("expected match for convex/vehicles")
	}
	if containsAny("/r/packages/backend/convex/events/x.ts", []string{"convex/vehicles"}) {
		t.Error("did not expect match")
	}
}
