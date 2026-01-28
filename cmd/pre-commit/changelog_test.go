package main

import (
	"os"
	"testing"
)

func TestCheckChangelog(t *testing.T) {
	// Default config for global mode tests
	globalConfig := ChangelogConfig{Mode: "global", GlobalDir: ".changelog"}
	emptyApps := map[string]AppConfig{}

	tests := []struct {
		name            string
		stagedFiles     []string
		excludePatterns []string
		config          ChangelogConfig
		apps            map[string]AppConfig
		skipEnvVar      string
		wantErr         bool
		errMsg          string
	}{
		{
			name:            "requires changelog for source files",
			stagedFiles:     []string{"src/app.ts", "src/utils.ts"},
			excludePatterns: []string{},
			config:          globalConfig,
			apps:            emptyApps,
			wantErr:         true,
			errMsg:          "changelog fragment required",
		},
		{
			name:            "excluded files do not require changelog",
			stagedFiles:     []string{"README.md", "docs/guide.md"},
			excludePatterns: []string{`\.md$`, `^docs/`},
			config:          globalConfig,
			apps:            emptyApps,
			wantErr:         false,
		},
		{
			name:            "changelog fragment found passes check",
			stagedFiles:     []string{"src/app.ts", ".changelog/123-add-feature.txt"},
			excludePatterns: []string{},
			config:          globalConfig,
			apps:            emptyApps,
			wantErr:         false,
		},
		{
			name:            "SKIP_CHANGELOG_CHECK bypasses check",
			stagedFiles:     []string{"src/app.ts", "src/utils.ts"},
			excludePatterns: []string{},
			config:          globalConfig,
			apps:            emptyApps,
			skipEnvVar:      "1",
			wantErr:         false,
		},
		{
			name:            "empty staged files passes",
			stagedFiles:     []string{},
			excludePatterns: []string{},
			config:          globalConfig,
			apps:            emptyApps,
			wantErr:         false,
		},
		{
			name:            "only empty strings in staged files passes",
			stagedFiles:     []string{"", ""},
			excludePatterns: []string{},
			config:          globalConfig,
			apps:            emptyApps,
			wantErr:         false,
		},
		{
			name:            "all files excluded passes",
			stagedFiles:     []string{"test/foo_test.go", "test/bar_test.go"},
			excludePatterns: []string{`^test/`, `_test\.go$`},
			config:          globalConfig,
			apps:            emptyApps,
			wantErr:         false,
		},
		{
			name:            "mixed excluded and non-excluded requires changelog",
			stagedFiles:     []string{"README.md", "src/app.ts"},
			excludePatterns: []string{`\.md$`},
			config:          globalConfig,
			apps:            emptyApps,
			wantErr:         true,
			errMsg:          "changelog fragment required",
		},
		{
			name:            "multiple changelog fragments pass",
			stagedFiles:     []string{"src/app.ts", ".changelog/001.txt", ".changelog/002.txt"},
			excludePatterns: []string{},
			config:          globalConfig,
			apps:            emptyApps,
			wantErr:         false,
		},
		{
			name:            "changelog file without .txt extension does not count",
			stagedFiles:     []string{"src/app.ts", ".changelog/readme.md"},
			excludePatterns: []string{},
			config:          globalConfig,
			apps:            emptyApps,
			wantErr:         true,
			errMsg:          "changelog fragment required",
		},
		{
			name:            "file in nested changelog directory does not count",
			stagedFiles:     []string{"src/app.ts", "docs/.changelog/note.txt"},
			excludePatterns: []string{},
			config:          globalConfig,
			apps:            emptyApps,
			wantErr:         true,
			errMsg:          "changelog fragment required",
		},
		{
			name:            "partial exclude pattern match",
			stagedFiles:     []string{"src/generated/types.ts"},
			excludePatterns: []string{`generated`},
			config:          globalConfig,
			apps:            emptyApps,
			wantErr:         false,
		},
		{
			name:            "complex regex exclude pattern",
			stagedFiles:     []string{"apps/web/components/Button.tsx"},
			excludePatterns: []string{`^apps/web/.*\.tsx$`},
			config:          globalConfig,
			apps:            emptyApps,
			wantErr:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set or clear the skip env var
			if tt.skipEnvVar != "" {
				os.Setenv("SKIP_CHANGELOG_CHECK", tt.skipEnvVar)
				defer os.Unsetenv("SKIP_CHANGELOG_CHECK")
			} else {
				os.Unsetenv("SKIP_CHANGELOG_CHECK")
			}

			err := checkChangelog(tt.stagedFiles, tt.excludePatterns, tt.config, tt.apps)

			if tt.wantErr {
				if err == nil {
					t.Errorf("checkChangelog() expected error, got nil")
					return
				}
				if tt.errMsg != "" && err.Error() != tt.errMsg {
					t.Errorf("checkChangelog() error = %q, want %q", err.Error(), tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("checkChangelog() unexpected error: %v", err)
				}
			}
		})
	}
}

func TestCheckChangelogPerApp(t *testing.T) {
	apps := map[string]AppConfig{
		"native": {Path: "apps/native"},
		"web":    {Path: "apps/web"},
	}
	perAppConfig := ChangelogConfig{Mode: "per-app"}
	requiredConfig := ChangelogConfig{Mode: "required"}

	tests := []struct {
		name            string
		stagedFiles     []string
		excludePatterns []string
		config          ChangelogConfig
		apps            map[string]AppConfig
		wantErr         bool
	}{
		{
			name:            "per-app: app change with app changelog passes",
			stagedFiles:     []string{"apps/native/src/app.ts", "apps/native/.changelog/001.txt"},
			excludePatterns: []string{},
			config:          perAppConfig,
			apps:            apps,
			wantErr:         false,
		},
		{
			name:            "per-app: app change without changelog fails",
			stagedFiles:     []string{"apps/native/src/app.ts"},
			excludePatterns: []string{},
			config:          perAppConfig,
			apps:            apps,
			wantErr:         true,
		},
		{
			name:            "per-app: shared change with global changelog passes",
			stagedFiles:     []string{"packages/shared/utils.ts", ".changelog/001.txt"},
			excludePatterns: []string{},
			config:          perAppConfig,
			apps:            apps,
			wantErr:         false,
		},
		{
			name:            "per-app: shared change with any app changelog passes",
			stagedFiles:     []string{"packages/shared/utils.ts", "apps/web/.changelog/001.txt"},
			excludePatterns: []string{},
			config:          perAppConfig,
			apps:            apps,
			wantErr:         false,
		},
		{
			name:            "required: app change with app changelog passes",
			stagedFiles:     []string{"apps/native/src/app.ts", "apps/native/.changelog/001.txt"},
			excludePatterns: []string{},
			config:          requiredConfig,
			apps:            apps,
			wantErr:         false,
		},
		{
			name:            "required: app change without changelog fails",
			stagedFiles:     []string{"apps/native/src/app.ts"},
			excludePatterns: []string{},
			config:          requiredConfig,
			apps:            apps,
			wantErr:         true,
		},
		{
			name:            "required: shared change with global changelog fails (no fallback)",
			stagedFiles:     []string{"packages/shared/utils.ts", ".changelog/001.txt"},
			excludePatterns: []string{},
			config:          requiredConfig,
			apps:            apps,
			wantErr:         true,
		},
		{
			name:            "required: shared change with app changelog passes",
			stagedFiles:     []string{"packages/shared/utils.ts", "apps/web/.changelog/001.txt"},
			excludePatterns: []string{},
			config:          requiredConfig,
			apps:            apps,
			wantErr:         false,
		},
		{
			name:            "multiple apps changed - all need changelogs",
			stagedFiles:     []string{"apps/native/src/app.ts", "apps/web/src/page.ts", "apps/native/.changelog/001.txt"},
			excludePatterns: []string{},
			config:          requiredConfig,
			apps:            apps,
			wantErr:         true, // web missing changelog
		},
		{
			name:            "multiple apps changed - all have changelogs",
			stagedFiles:     []string{"apps/native/src/app.ts", "apps/web/src/page.ts", "apps/native/.changelog/001.txt", "apps/web/.changelog/002.txt"},
			excludePatterns: []string{},
			config:          requiredConfig,
			apps:            apps,
			wantErr:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Unsetenv("SKIP_CHANGELOG_CHECK")

			err := checkChangelog(tt.stagedFiles, tt.excludePatterns, tt.config, tt.apps)

			if tt.wantErr && err == nil {
				t.Errorf("checkChangelog() expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("checkChangelog() unexpected error: %v", err)
			}
		})
	}
}

func TestGetChangelogApps(t *testing.T) {
	allApps := map[string]AppConfig{
		"native":  {Path: "apps/native"},
		"web":     {Path: "apps/web"},
		"backend": {Path: "packages/backend"},
	}

	tests := []struct {
		name     string
		config   ChangelogConfig
		apps     map[string]AppConfig
		wantApps []string
	}{
		{
			name:     "empty config.Apps returns all apps",
			config:   ChangelogConfig{Mode: "per-app"},
			apps:     allApps,
			wantApps: []string{"native", "web", "backend"},
		},
		{
			name:     "config.Apps filters to specified apps",
			config:   ChangelogConfig{Mode: "per-app", Apps: []string{"native", "web"}},
			apps:     allApps,
			wantApps: []string{"native", "web"},
		},
		{
			name:     "config.Apps ignores unknown apps",
			config:   ChangelogConfig{Mode: "per-app", Apps: []string{"native", "unknown"}},
			apps:     allApps,
			wantApps: []string{"native"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getChangelogApps(tt.config, tt.apps)

			if len(got) != len(tt.wantApps) {
				t.Errorf("got %d apps, want %d", len(got), len(tt.wantApps))
				return
			}

			for _, wantApp := range tt.wantApps {
				if _, ok := got[wantApp]; !ok {
					t.Errorf("missing expected app %q", wantApp)
				}
			}
		})
	}
}
