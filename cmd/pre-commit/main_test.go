package main

import (
	"reflect"
	"testing"
)

func TestFilterFilesForSRP(t *testing.T) {
	tests := []struct {
		name     string
		files    []string
		config   SRPConfig
		expected []string
	}{
		{
			name: "empty config returns all files",
			files: []string{
				"apps/portal/src/component.tsx",
				"apps/mobile/src/screen.tsx",
				"apps/match/src/feature.tsx",
				"packages/utils/helper.ts",
			},
			config: SRPConfig{},
			expected: []string{
				"apps/portal/src/component.tsx",
				"apps/mobile/src/screen.tsx",
				"apps/match/src/feature.tsx",
				"packages/utils/helper.ts",
			},
		},
		{
			name: "appPaths filters to only matching apps",
			files: []string{
				"apps/portal/src/component.tsx",
				"apps/mobile/src/screen.tsx",
				"apps/match/src/feature.tsx",
				"packages/utils/helper.ts",
			},
			config: SRPConfig{
				AppPaths: []string{"apps/portal", "apps/mobile"},
			},
			expected: []string{
				"apps/portal/src/component.tsx",
				"apps/mobile/src/screen.tsx",
			},
		},
		{
			name: "excludePaths removes matching files",
			files: []string{
				"apps/portal/src/component.tsx",
				"apps/mobile/src/screen.tsx",
				"packages/react-email/template.tsx",
				"packages/utils/helper.ts",
			},
			config: SRPConfig{
				ExcludePaths: []string{"packages/react-email/"},
			},
			expected: []string{
				"apps/portal/src/component.tsx",
				"apps/mobile/src/screen.tsx",
				"packages/utils/helper.ts",
			},
		},
		{
			name: "appPaths and excludePaths work together",
			files: []string{
				"apps/portal/src/component.tsx",
				"apps/portal/src/legacy/old.tsx",
				"apps/mobile/src/screen.tsx",
				"apps/match/src/feature.tsx",
				"packages/utils/helper.ts",
			},
			config: SRPConfig{
				AppPaths:     []string{"apps/portal", "apps/mobile"},
				ExcludePaths: []string{"apps/portal/src/legacy/"},
			},
			expected: []string{
				"apps/portal/src/component.tsx",
				"apps/mobile/src/screen.tsx",
			},
		},
		{
			name: "excludePaths with contains matching",
			files: []string{
				"apps/portal/src/component.tsx",
				"apps/match/src/features/auth/login.tsx",
				"apps/match/src/features/chat/message.tsx",
				"apps/mobile/src/screen.tsx",
			},
			config: SRPConfig{
				ExcludePaths: []string{"apps/match/"},
			},
			expected: []string{
				"apps/portal/src/component.tsx",
				"apps/mobile/src/screen.tsx",
			},
		},
		{
			name: "multiple excludePaths",
			files: []string{
				"apps/portal/src/component.tsx",
				"apps/match/src/feature.tsx",
				"packages/react-email/template.tsx",
				"packages/utils/search/index.ts",
				"apps/mobile/src/screen.tsx",
			},
			config: SRPConfig{
				ExcludePaths: []string{
					"apps/match/",
					"packages/react-email/",
					"packages/utils/search/",
				},
			},
			expected: []string{
				"apps/portal/src/component.tsx",
				"apps/mobile/src/screen.tsx",
			},
		},
		{
			name: "empty files returns empty",
			files:    []string{},
			config:   SRPConfig{AppPaths: []string{"apps/portal"}},
			expected: nil,
		},
		{
			name: "no files match appPaths returns empty",
			files: []string{
				"apps/match/src/feature.tsx",
				"packages/utils/helper.ts",
			},
			config: SRPConfig{
				AppPaths: []string{"apps/portal", "apps/mobile"},
			},
			expected: nil,
		},
		{
			name: "all files excluded returns empty",
			files: []string{
				"apps/match/src/feature.tsx",
				"apps/match/src/other.tsx",
			},
			config: SRPConfig{
				ExcludePaths: []string{"apps/match/"},
			},
			expected: nil,
		},
		{
			name: "real-world dashtag config",
			files: []string{
				"apps/portal/src/components/Button.tsx",
				"apps/mobile/src/screens/Home.tsx",
				"apps/admin/src/routes/Dashboard.tsx",
				"apps/match/src/features/swipe/Card.tsx",
				"apps/match/src/api-client/auth.ts",
				"packages/react-email/templates/Welcome.tsx",
				"packages/utils/src/search/index.ts",
			},
			config: SRPConfig{
				AppPaths: []string{
					"apps/portal",
					"apps/mobile",
					"apps/crm",
					"apps/store",
					"apps/admin",
					"apps/web",
					"apps/marketing",
				},
				ExcludePaths: []string{
					"packages/react-email/",
					"packages/utils/src/search/",
					"apps/match/",
				},
			},
			expected: []string{
				"apps/portal/src/components/Button.tsx",
				"apps/mobile/src/screens/Home.tsx",
				"apps/admin/src/routes/Dashboard.tsx",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := filterFilesForSRP(tt.files, tt.config)

			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("filterFilesForSRP() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestFilterFilesForSRP_PreservesOrder(t *testing.T) {
	files := []string{
		"apps/portal/z.tsx",
		"apps/portal/a.tsx",
		"apps/portal/m.tsx",
	}
	config := SRPConfig{
		AppPaths: []string{"apps/portal"},
	}

	result := filterFilesForSRP(files, config)

	expected := []string{
		"apps/portal/z.tsx",
		"apps/portal/a.tsx",
		"apps/portal/m.tsx",
	}

	if !reflect.DeepEqual(result, expected) {
		t.Errorf("order not preserved: got %v, want %v", result, expected)
	}
}

func TestFilterFilesForSRPWithDetails(t *testing.T) {
	tests := []struct {
		name                   string
		files                  []string
		config                 SRPConfig
		expectedFiles          []string
		expectedSkippedAppPath int
		expectedSkippedExclude int
		expectedExcludeMatches map[string]int
	}{
		{
			name: "tracks skipped by appPaths",
			files: []string{
				"apps/portal/src/component.tsx",
				"apps/match/src/feature.tsx",
				"apps/match/src/other.tsx",
				"packages/utils/helper.ts",
			},
			config: SRPConfig{
				AppPaths: []string{"apps/portal"},
			},
			expectedFiles:          []string{"apps/portal/src/component.tsx"},
			expectedSkippedAppPath: 3,
			expectedSkippedExclude: 0,
			expectedExcludeMatches: map[string]int{},
		},
		{
			name: "tracks skipped by excludePaths with counts",
			files: []string{
				"apps/portal/src/component.tsx",
				"apps/match/src/feature.tsx",
				"apps/match/src/other.tsx",
				"packages/react-email/template.tsx",
			},
			config: SRPConfig{
				ExcludePaths: []string{"apps/match/", "packages/react-email/"},
			},
			expectedFiles:          []string{"apps/portal/src/component.tsx"},
			expectedSkippedAppPath: 0,
			expectedSkippedExclude: 3,
			expectedExcludeMatches: map[string]int{
				"apps/match/":          2,
				"packages/react-email/": 1,
			},
		},
		{
			name: "tracks both appPaths and excludePaths",
			files: []string{
				"apps/portal/src/component.tsx",
				"apps/portal/legacy/old.tsx",
				"apps/match/src/feature.tsx",
				"packages/utils/helper.ts",
			},
			config: SRPConfig{
				AppPaths:     []string{"apps/portal"},
				ExcludePaths: []string{"apps/portal/legacy/"},
			},
			expectedFiles:          []string{"apps/portal/src/component.tsx"},
			expectedSkippedAppPath: 2, // apps/match and packages/utils
			expectedSkippedExclude: 1, // apps/portal/legacy
			expectedExcludeMatches: map[string]int{
				"apps/portal/legacy/": 1,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := filterFilesForSRPWithDetails(tt.files, tt.config)

			if !reflect.DeepEqual(result.Files, tt.expectedFiles) {
				t.Errorf("Files = %v, want %v", result.Files, tt.expectedFiles)
			}

			if result.SkippedByAppPath != tt.expectedSkippedAppPath {
				t.Errorf("SkippedByAppPath = %d, want %d", result.SkippedByAppPath, tt.expectedSkippedAppPath)
			}

			if result.SkippedByExclude != tt.expectedSkippedExclude {
				t.Errorf("SkippedByExclude = %d, want %d", result.SkippedByExclude, tt.expectedSkippedExclude)
			}

			if !reflect.DeepEqual(result.ExcludeMatches, tt.expectedExcludeMatches) {
				t.Errorf("ExcludeMatches = %v, want %v", result.ExcludeMatches, tt.expectedExcludeMatches)
			}
		})
	}
}
