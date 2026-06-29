package main

import "testing"

func TestInSRPScope(t *testing.T) {
	srpAppPaths = []string{"apps/mobile", "apps/story"}
	srpExcludePaths = []string{"apps/story/test/"}

	cases := map[string]bool{
		"/repo/apps/mobile/components/Foo.tsx": true,
		"/repo/apps/story/components/Bar.tsx":  true,
		"/repo/apps/story/test/helper.ts":      false, // excluded
		"/repo/packages/backend/convex/x.ts":   false, // not in appPaths
	}
	for path, want := range cases {
		if got := inSRPScope(path); got != want {
			t.Errorf("inSRPScope(%q) = %v, want %v", path, got, want)
		}
	}

	// Empty appPaths => everything in scope (back-compat)
	srpAppPaths = nil
	srpExcludePaths = nil
	if !inSRPScope("/repo/packages/backend/convex/x.ts") {
		t.Error("empty appPaths should put all files in scope")
	}
}
