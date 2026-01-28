package main

import (
	"reflect"
	"testing"
)

func TestParseStagedFiles(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   []string
	}{
		{
			name:   "empty output",
			output: "",
			want:   nil,
		},
		{
			name:   "single file",
			output: "apps/web/src/index.ts\n",
			want:   []string{"apps/web/src/index.ts"},
		},
		{
			name:   "multiple files",
			output: "apps/web/src/index.ts\napps/native/src/app.tsx\npackages/backend/schema.ts\n",
			want: []string{
				"apps/web/src/index.ts",
				"apps/native/src/app.tsx",
				"packages/backend/schema.ts",
			},
		},
		{
			name:   "files with extra whitespace",
			output: "  apps/web/src/index.ts  \n\napps/native/src/app.tsx\n\n",
			want: []string{
				"apps/web/src/index.ts",
				"apps/native/src/app.tsx",
			},
		},
		{
			name:   "only newlines",
			output: "\n\n\n",
			want:   nil,
		},
		{
			name:   "file with spaces in path",
			output: "apps/web/src/my file.ts\n",
			want:   []string{"apps/web/src/my file.ts"},
		},
		{
			name:   "deeply nested files",
			output: "apps/web/src/features/auth/components/login-form.tsx\npackages/data-layer/src/generated-hooks/queries/index.ts\n",
			want: []string{
				"apps/web/src/features/auth/components/login-form.tsx",
				"packages/data-layer/src/generated-hooks/queries/index.ts",
			},
		},
		{
			name:   "no trailing newline",
			output: "apps/web/index.ts",
			want:   []string{"apps/web/index.ts"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseStagedFiles(tt.output)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseStagedFiles() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCategorizeFiles(t *testing.T) {
	apps := map[string]AppConfig{
		"web": {
			Path:   "apps/web",
			Filter: "@repo/web",
		},
		"native": {
			Path:   "apps/native",
			Filter: "@repo/native",
		},
		"api": {
			Path:   "apps/api",
			Filter: "@repo/api",
		},
	}

	sharedPaths := []string{
		"packages/",
		"libs/",
	}

	tests := []struct {
		name          string
		files         []string
		apps          map[string]AppConfig
		sharedPaths   []string
		wantAppFiles  map[string][]string
		wantShared    bool
	}{
		{
			name:          "empty files",
			files:         []string{},
			apps:          apps,
			sharedPaths:   sharedPaths,
			wantAppFiles:  map[string][]string{},
			wantShared:    false,
		},
		{
			name:  "single app file",
			files: []string{"apps/web/src/index.ts"},
			apps:  apps,
			sharedPaths: sharedPaths,
			wantAppFiles: map[string][]string{
				"web": {"apps/web/src/index.ts"},
			},
			wantShared: false,
		},
		{
			name: "multiple files same app",
			files: []string{
				"apps/web/src/index.ts",
				"apps/web/src/components/button.tsx",
				"apps/web/package.json",
			},
			apps:        apps,
			sharedPaths: sharedPaths,
			wantAppFiles: map[string][]string{
				"web": {
					"apps/web/src/index.ts",
					"apps/web/src/components/button.tsx",
					"apps/web/package.json",
				},
			},
			wantShared: false,
		},
		{
			name: "files from multiple apps",
			files: []string{
				"apps/web/src/index.ts",
				"apps/native/src/app.tsx",
				"apps/api/src/server.ts",
			},
			apps:        apps,
			sharedPaths: sharedPaths,
			wantAppFiles: map[string][]string{
				"web":    {"apps/web/src/index.ts"},
				"native": {"apps/native/src/app.tsx"},
				"api":    {"apps/api/src/server.ts"},
			},
			wantShared: false,
		},
		{
			name:  "shared path only",
			files: []string{"packages/backend/schema.ts"},
			apps:  apps,
			sharedPaths: sharedPaths,
			wantAppFiles: map[string][]string{},
			wantShared:   true,
		},
		{
			name: "multiple shared paths",
			files: []string{
				"packages/backend/schema.ts",
				"libs/utils/index.ts",
			},
			apps:         apps,
			sharedPaths:  sharedPaths,
			wantAppFiles: map[string][]string{},
			wantShared:   true,
		},
		{
			name: "mixed app and shared files",
			files: []string{
				"apps/web/src/index.ts",
				"packages/backend/schema.ts",
				"apps/native/src/app.tsx",
			},
			apps:        apps,
			sharedPaths: sharedPaths,
			wantAppFiles: map[string][]string{
				"web":    {"apps/web/src/index.ts"},
				"native": {"apps/native/src/app.tsx"},
			},
			wantShared: true,
		},
		{
			name: "root files (not in app or shared)",
			files: []string{
				"package.json",
				"tsconfig.json",
				".eslintrc.js",
			},
			apps:         apps,
			sharedPaths:  sharedPaths,
			wantAppFiles: map[string][]string{},
			wantShared:   false,
		},
		{
			name: "mixed root and app files",
			files: []string{
				"apps/web/src/index.ts",
				"package.json",
			},
			apps:        apps,
			sharedPaths: sharedPaths,
			wantAppFiles: map[string][]string{
				"web": {"apps/web/src/index.ts"},
			},
			wantShared: false,
		},
		{
			name: "file path that starts with app path but not in app directory",
			files: []string{
				"apps/web-admin/src/index.ts",
			},
			apps:         apps,
			sharedPaths:  sharedPaths,
			wantAppFiles: map[string][]string{},
			wantShared:   false,
		},
		{
			name:          "nil apps map",
			files:         []string{"apps/web/src/index.ts"},
			apps:          nil,
			sharedPaths:   sharedPaths,
			wantAppFiles:  map[string][]string{},
			wantShared:    false,
		},
		{
			name:  "nil shared paths",
			files: []string{"packages/backend/schema.ts"},
			apps:  apps,
			sharedPaths: nil,
			wantAppFiles: map[string][]string{},
			wantShared:   false,
		},
		{
			name:          "empty apps and shared paths",
			files:         []string{"apps/web/src/index.ts", "packages/backend/schema.ts"},
			apps:          map[string]AppConfig{},
			sharedPaths:   []string{},
			wantAppFiles:  map[string][]string{},
			wantShared:    false,
		},
		{
			name: "shared path without trailing slash matches prefix",
			files: []string{
				"packages/backend/schema.ts",
				"packages-extra/something.ts",
			},
			apps:        apps,
			sharedPaths: []string{"packages/"},
			wantAppFiles: map[string][]string{},
			wantShared:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotAppFiles, gotShared := categorizeFiles(tt.files, tt.apps, tt.sharedPaths)

			if !reflect.DeepEqual(gotAppFiles, tt.wantAppFiles) {
				t.Errorf("categorizeFiles() appFiles = %v, want %v", gotAppFiles, tt.wantAppFiles)
			}
			if gotShared != tt.wantShared {
				t.Errorf("categorizeFiles() sharedChanged = %v, want %v", gotShared, tt.wantShared)
			}
		})
	}
}
