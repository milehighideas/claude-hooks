package main

import (
	"fmt"
	"strings"
	"sync"
	"testing"
)

// withMockBundleRunner swaps bundleScriptRunner for the duration of the test
// and restores the original when the test exits.
func withMockBundleRunner(t *testing.T, mock func(appPath, packageManager, script string) (string, error)) {
	t.Helper()
	orig := bundleScriptRunner
	bundleScriptRunner = mock
	t.Cleanup(func() { bundleScriptRunner = orig })
}

func TestRunBundleCheck_NoAppsSkips(t *testing.T) {
	called := false
	withMockBundleRunner(t, func(_, _, _ string) (string, error) {
		called = true
		return "", nil
	})

	err := runBundleCheck(BundleCheckConfig{Apps: nil}, map[string]AppConfig{
		"web": {Path: "apps/web"},
	}, "bun")

	if err != nil {
		t.Errorf("expected no error with empty apps list, got %v", err)
	}
	if called {
		t.Error("expected runner not to be invoked when no apps configured")
	}
}

func TestRunBundleCheck_DefaultsScriptName(t *testing.T) {
	var seen string
	var mu sync.Mutex
	withMockBundleRunner(t, func(_, _, script string) (string, error) {
		mu.Lock()
		seen = script
		mu.Unlock()
		return "", nil
	})

	err := runBundleCheck(BundleCheckConfig{Apps: []string{"web"}}, map[string]AppConfig{
		"web": {Path: "apps/web"},
	}, "bun")

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if seen != "bundle:check" {
		t.Errorf("expected default script 'bundle:check', got %q", seen)
	}
}

func TestRunBundleCheck_RespectsCustomScript(t *testing.T) {
	var seen string
	var mu sync.Mutex
	withMockBundleRunner(t, func(_, _, script string) (string, error) {
		mu.Lock()
		seen = script
		mu.Unlock()
		return "", nil
	})

	err := runBundleCheck(
		BundleCheckConfig{Apps: []string{"web"}, Script: "expo:bundle"},
		map[string]AppConfig{"web": {Path: "apps/web"}},
		"bun",
	)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if seen != "expo:bundle" {
		t.Errorf("expected configured script 'expo:bundle', got %q", seen)
	}
}

func TestRunBundleCheck_DefaultsPackageManagerToPnpm(t *testing.T) {
	var seen string
	var mu sync.Mutex
	withMockBundleRunner(t, func(_, pm, _ string) (string, error) {
		mu.Lock()
		seen = pm
		mu.Unlock()
		return "", nil
	})

	if err := runBundleCheck(
		BundleCheckConfig{Apps: []string{"web"}},
		map[string]AppConfig{"web": {Path: "apps/web"}},
		"",
	); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if seen != "pnpm" {
		t.Errorf("expected default package manager 'pnpm', got %q", seen)
	}
}

func TestRunBundleCheck_AppNotFoundReturnsError(t *testing.T) {
	withMockBundleRunner(t, func(_, _, _ string) (string, error) {
		t.Error("runner should not be invoked when app is missing")
		return "", nil
	})

	err := runBundleCheck(
		BundleCheckConfig{Apps: []string{"missing"}},
		map[string]AppConfig{"web": {Path: "apps/web"}},
		"bun",
	)

	if err == nil {
		t.Fatal("expected error for missing app")
	}
	if !strings.Contains(err.Error(), "missing failed") {
		t.Errorf("expected aggregate to mention 'missing failed', got %v", err)
	}
}

func TestRunBundleCheck_RunsAllAppsEvenWhenOneFails(t *testing.T) {
	var (
		mu    sync.Mutex
		calls []string
	)
	withMockBundleRunner(t, func(appPath, _, _ string) (string, error) {
		mu.Lock()
		calls = append(calls, appPath)
		mu.Unlock()
		if appPath == "apps/web" {
			return "metro: cannot resolve foo", fmt.Errorf("exit 1")
		}
		return "", nil
	})

	err := runBundleCheck(
		BundleCheckConfig{Apps: []string{"web", "mobile", "shop"}},
		map[string]AppConfig{
			"web":    {Path: "apps/web"},
			"mobile": {Path: "apps/mobile"},
			"shop":   {Path: "apps/shop"},
		},
		"bun",
	)

	if err == nil {
		t.Fatal("expected error when one app fails")
	}
	if len(calls) != 3 {
		t.Errorf("expected all 3 apps to run; got %d (%v)", len(calls), calls)
	}
	if !strings.Contains(err.Error(), "web failed") {
		t.Errorf("expected 'web failed' in error, got %q", err)
	}
}

func TestRunBundleCheck_AggregatesMultipleFailures(t *testing.T) {
	withMockBundleRunner(t, func(_, _, _ string) (string, error) {
		return "boom", fmt.Errorf("exit 1")
	})

	err := runBundleCheck(
		BundleCheckConfig{Apps: []string{"web", "mobile"}},
		map[string]AppConfig{
			"web":    {Path: "apps/web"},
			"mobile": {Path: "apps/mobile"},
		},
		"bun",
	)

	if err == nil {
		t.Fatal("expected aggregated error")
	}
	if !strings.Contains(err.Error(), "web failed") || !strings.Contains(err.Error(), "mobile failed") {
		t.Errorf("expected both 'web failed' and 'mobile failed' in error, got %q", err)
	}
}

func TestRunBundleCheck_AllPassReturnsNil(t *testing.T) {
	withMockBundleRunner(t, func(_, _, _ string) (string, error) {
		return "", nil
	})

	err := runBundleCheck(
		BundleCheckConfig{Apps: []string{"web", "mobile"}},
		map[string]AppConfig{
			"web":    {Path: "apps/web"},
			"mobile": {Path: "apps/mobile"},
		},
		"bun",
	)

	if err != nil {
		t.Errorf("expected no error when all apps pass, got %v", err)
	}
}
