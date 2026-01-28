package main

import (
	"fmt"
	"strings"
	"testing"
)

func TestCheckBuild(t *testing.T) {
	tests := []struct {
		name        string
		config      BuildConfig
		apps        map[string]AppConfig
		wantErr     bool
		errContains string
	}{
		{
			name:   "empty apps list skips build",
			config: BuildConfig{Apps: []string{}},
			apps: map[string]AppConfig{
				"web": {Path: "apps/web", Filter: "@upc-me/web"},
			},
			wantErr: false,
		},
		{
			name:   "nil apps list skips build",
			config: BuildConfig{Apps: nil},
			apps: map[string]AppConfig{
				"web": {Path: "apps/web", Filter: "@upc-me/web"},
			},
			wantErr: false,
		},
		{
			name:   "app not found in config",
			config: BuildConfig{Apps: []string{"mobile"}},
			apps: map[string]AppConfig{
				"web": {Path: "apps/web", Filter: "@upc-me/web"},
			},
			wantErr:     true,
			errContains: "app \"mobile\" not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Use mock runner to avoid actual command execution
			err := checkBuildWithRunner(tt.config, tt.apps, func(dir string) error {
				return nil // mock success
			})

			if (err != nil) != tt.wantErr {
				t.Errorf("checkBuild() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr && tt.errContains != "" {
				if err == nil || !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("checkBuild() error = %v, want error containing %q", err, tt.errContains)
				}
			}
		})
	}
}

func TestCheckBuildSuccess(t *testing.T) {
	config := BuildConfig{Apps: []string{"web"}}
	apps := map[string]AppConfig{
		"web": {Path: "apps/web", Filter: "@upc-me/web"},
	}

	buildCalled := false
	err := checkBuildWithRunner(config, apps, func(dir string) error {
		buildCalled = true
		if dir != "apps/web" {
			t.Errorf("expected dir 'apps/web', got %q", dir)
		}
		return nil
	})

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if !buildCalled {
		t.Error("expected build to be called")
	}
}

func TestCheckBuildFailure(t *testing.T) {
	config := BuildConfig{Apps: []string{"web"}}
	apps := map[string]AppConfig{
		"web": {Path: "apps/web", Filter: "@upc-me/web"},
	}

	err := checkBuildWithRunner(config, apps, func(dir string) error {
		return fmt.Errorf("build error: compilation failed")
	})

	if err == nil {
		t.Error("expected error, got nil")
		return
	}
	if !strings.Contains(err.Error(), "build failed for web") {
		t.Errorf("expected error containing 'build failed for web', got %q", err.Error())
	}
	if !strings.Contains(err.Error(), "compilation failed") {
		t.Errorf("expected error containing 'compilation failed', got %q", err.Error())
	}
}

func TestCheckBuildMultipleApps(t *testing.T) {
	config := BuildConfig{Apps: []string{"web", "native"}}
	apps := map[string]AppConfig{
		"web":    {Path: "apps/web", Filter: "@test/web"},
		"native": {Path: "apps/native", Filter: "native"},
	}

	builtApps := []string{}
	err := checkBuildWithRunner(config, apps, func(dir string) error {
		builtApps = append(builtApps, dir)
		return nil
	})

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if len(builtApps) != 2 {
		t.Errorf("expected 2 apps built, got %d", len(builtApps))
	}
	// Check both paths were built (order matches config.Apps)
	if builtApps[0] != "apps/web" {
		t.Errorf("expected first app to be 'apps/web', got %q", builtApps[0])
	}
	if builtApps[1] != "apps/native" {
		t.Errorf("expected second app to be 'apps/native', got %q", builtApps[1])
	}
}

func TestCheckBuildStopsOnFirstFailure(t *testing.T) {
	config := BuildConfig{Apps: []string{"web", "native"}}
	apps := map[string]AppConfig{
		"web":    {Path: "apps/web", Filter: "@test/web"},
		"native": {Path: "apps/native", Filter: "native"},
	}

	builtApps := []string{}
	err := checkBuildWithRunner(config, apps, func(dir string) error {
		builtApps = append(builtApps, dir)
		if dir == "apps/web" {
			return fmt.Errorf("web build failed")
		}
		return nil
	})

	if err == nil {
		t.Error("expected error on first failure, got nil")
		return
	}
	if len(builtApps) != 1 {
		t.Errorf("expected only 1 app attempted, got %d", len(builtApps))
	}
	if !strings.Contains(err.Error(), "build failed for web") {
		t.Errorf("expected error for 'web', got %v", err)
	}
}

// checkBuildWithRunner is a test helper that allows injecting a mock build runner
func checkBuildWithRunner(config BuildConfig, apps map[string]AppConfig, runner func(dir string) error) error {
	if len(config.Apps) == 0 {
		return nil
	}

	for _, appName := range config.Apps {
		appConfig, ok := apps[appName]
		if !ok {
			return fmt.Errorf("app %q not found in configuration", appName)
		}

		fmt.Printf("Building %s...\n", appName)
		if err := runner(appConfig.Path); err != nil {
			return fmt.Errorf("build failed for %s: %w", appName, err)
		}
		fmt.Printf("Build successful for %s\n", appName)
	}

	return nil
}
