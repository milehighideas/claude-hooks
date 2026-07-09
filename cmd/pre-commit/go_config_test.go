package main

import (
	"encoding/json"
	"testing"
)

func TestGoFeaturesDecode(t *testing.T) {
	const cfg = `{
		"features": {"goMissingTestsCheck": true, "goTests": true},
		"goMissingTestsCheckConfig": {"mode": "staged", "appPaths": ["apps/vendor-sync"], "excludePaths": ["/cmd/"]},
		"goTests": {"modules": ["apps/vendor-sync"], "affectedOnly": false}
	}`
	var c Config
	if err := json.Unmarshal([]byte(cfg), &c); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !c.Features.GoMissingTestsCheck || !c.Features.GoTests {
		t.Fatal("feature flags did not decode true")
	}
	if c.GoMissingTestsCheck.Mode != "staged" {
		t.Fatalf("mode = %q, want staged", c.GoMissingTestsCheck.Mode)
	}
	if len(c.GoMissingTestsCheck.AppPaths) != 1 || c.GoMissingTestsCheck.AppPaths[0] != "apps/vendor-sync" {
		t.Fatalf("appPaths = %v", c.GoMissingTestsCheck.AppPaths)
	}
	if len(c.GoTests.Modules) != 1 || c.GoTests.AffectedOnly {
		t.Fatalf("goTests = %+v", c.GoTests)
	}
}

func TestGoFeaturesDefaultOffWhenAbsent(t *testing.T) {
	var c Config
	if err := json.Unmarshal([]byte(`{"features": {}}`), &c); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if c.Features.GoMissingTestsCheck || c.Features.GoTests {
		t.Fatal("absent flags should decode false (backwards compat)")
	}
}
