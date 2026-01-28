package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseConventionalCommit(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantType    string
		wantScope   string
		wantDesc    string
		wantErr     bool
		errContains string
	}{
		{
			name:      "feat with scope",
			input:     "feat(native): add login functionality",
			wantType:  "feat",
			wantScope: "native",
			wantDesc:  "add login functionality",
		},
		{
			name:      "fix with scope",
			input:     "fix(web): resolve navigation bug",
			wantType:  "fix",
			wantScope: "web",
			wantDesc:  "resolve navigation bug",
		},
		{
			name:      "chore without scope",
			input:     "chore: update dependencies",
			wantType:  "chore",
			wantScope: "",
			wantDesc:  "update dependencies",
		},
		{
			name:      "uppercase type normalized",
			input:     "FEAT(native): add feature",
			wantType:  "feat",
			wantScope: "native",
			wantDesc:  "add feature",
		},
		{
			name:      "mixed case type",
			input:     "FiX(web): fix bug",
			wantType:  "fix",
			wantScope: "web",
			wantDesc:  "fix bug",
		},
		{
			name:      "all valid types - docs",
			input:     "docs: update readme",
			wantType:  "docs",
			wantScope: "",
			wantDesc:  "update readme",
		},
		{
			name:      "all valid types - test",
			input:     "test: add unit tests",
			wantType:  "test",
			wantScope: "",
			wantDesc:  "add unit tests",
		},
		{
			name:      "all valid types - style",
			input:     "style: format code",
			wantType:  "style",
			wantScope: "",
			wantDesc:  "format code",
		},
		{
			name:      "all valid types - refactor",
			input:     "refactor: restructure modules",
			wantType:  "refactor",
			wantScope: "",
			wantDesc:  "restructure modules",
		},
		{
			name:      "all valid types - perf",
			input:     "perf: optimize query",
			wantType:  "perf",
			wantScope: "",
			wantDesc:  "optimize query",
		},
		{
			name:      "all valid types - build",
			input:     "build: update webpack config",
			wantType:  "build",
			wantScope: "",
			wantDesc:  "update webpack config",
		},
		{
			name:      "all valid types - ci",
			input:     "ci: add github action",
			wantType:  "ci",
			wantScope: "",
			wantDesc:  "add github action",
		},
		{
			name:      "all valid types - revert",
			input:     "revert: undo last commit",
			wantType:  "revert",
			wantScope: "",
			wantDesc:  "undo last commit",
		},
		{
			name:      "description with extra spaces trimmed",
			input:     "feat(api):   add endpoint  ",
			wantType:  "feat",
			wantScope: "api",
			wantDesc:  "add endpoint",
		},
		{
			name:        "invalid format - no colon",
			input:       "feat add something",
			wantErr:     true,
			errContains: "invalid format",
		},
		{
			name:        "invalid format - empty",
			input:       "",
			wantErr:     true,
			errContains: "invalid format",
		},
		{
			name:        "invalid type",
			input:       "invalid(scope): description",
			wantErr:     true,
			errContains: "invalid type",
		},
		{
			name:        "invalid type - feature instead of feat",
			input:       "feature(scope): description",
			wantErr:     true,
			errContains: "invalid type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotType, gotScope, gotDesc, err := parseConventionalCommit(tt.input)

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.errContains)
					return
				}
				if !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("error = %q, want containing %q", err.Error(), tt.errContains)
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if gotType != tt.wantType {
				t.Errorf("type = %q, want %q", gotType, tt.wantType)
			}
			if gotScope != tt.wantScope {
				t.Errorf("scope = %q, want %q", gotScope, tt.wantScope)
			}
			if gotDesc != tt.wantDesc {
				t.Errorf("description = %q, want %q", gotDesc, tt.wantDesc)
			}
		})
	}
}

func TestSanitizeFilename(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		maxLength int
		want      string
	}{
		{
			name:      "simple text",
			input:     "add login functionality",
			maxLength: 50,
			want:      "add-login-functionality",
		},
		{
			name:      "with special characters",
			input:     "fix: bug in login!",
			maxLength: 50,
			want:      "fix-bug-in-login",
		},
		{
			name:      "uppercase converted",
			input:     "ADD LOGIN",
			maxLength: 50,
			want:      "add-login",
		},
		{
			name:      "multiple spaces",
			input:     "add   multiple   spaces",
			maxLength: 50,
			want:      "add-multiple-spaces",
		},
		{
			name:      "leading and trailing spaces",
			input:     "  trim me  ",
			maxLength: 50,
			want:      "trim-me",
		},
		{
			name:      "truncate to max length",
			input:     "this is a very long description that exceeds the maximum length",
			maxLength: 20,
			want:      "this-is-a-very-long",
		},
		{
			name:      "truncate removes trailing dash",
			input:     "this is a very long",
			maxLength: 15,
			want:      "this-is-a-very",
		},
		{
			name:      "empty string",
			input:     "",
			maxLength: 50,
			want:      "",
		},
		{
			name:      "only special characters",
			input:     "!@#$%^&*()",
			maxLength: 50,
			want:      "",
		},
		{
			name:      "numbers preserved",
			input:     "version 2.0.0",
			maxLength: 50,
			want:      "version-2-0-0",
		},
		{
			name:      "hyphens preserved",
			input:     "pre-existing-condition",
			maxLength: 50,
			want:      "pre-existing-condition",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeFilename(tt.input, tt.maxLength)
			if got != tt.want {
				t.Errorf("sanitizeFilename(%q, %d) = %q, want %q", tt.input, tt.maxLength, got, tt.want)
			}
		})
	}
}

func TestResolveApp(t *testing.T) {
	apps := map[string]AppConfig{
		"native":  {Path: "apps/native", Filter: "native"},
		"web":     {Path: "apps/web", Filter: "@upc-me/web"},
		"backend": {Path: "packages/backend", Filter: "@upc-me/backend"},
	}

	tests := []struct {
		name        string
		apps        map[string]AppConfig
		scope       string
		explicitApp string
		wantName    string
		wantPath    string
		wantErr     bool
		errContains string
	}{
		{
			name:     "match scope to app - native",
			apps:     apps,
			scope:    "native",
			wantName: "native",
			wantPath: "apps/native",
		},
		{
			name:     "match scope to app - web",
			apps:     apps,
			scope:    "web",
			wantName: "web",
			wantPath: "apps/web",
		},
		{
			name:     "match scope case insensitive",
			apps:     apps,
			scope:    "NATIVE",
			wantName: "native",
			wantPath: "apps/native",
		},
		{
			name:        "explicit app",
			apps:        apps,
			scope:       "native",
			explicitApp: "backend",
			wantName:    "backend",
			wantPath:    "packages/backend",
		},
		{
			name:        "explicit app overrides scope",
			apps:        apps,
			scope:       "web",
			explicitApp: "native",
			wantName:    "native",
			wantPath:    "apps/native",
		},
		{
			name:        "unknown explicit app",
			apps:        apps,
			explicitApp: "unknown",
			wantErr:     true,
			errContains: "unknown app",
		},
		{
			name:     "no match - returns empty",
			apps:     apps,
			scope:    "unknown",
			wantName: "",
			wantPath: "",
		},
		{
			name:     "empty scope - returns empty",
			apps:     apps,
			scope:    "",
			wantName: "",
			wantPath: "",
		},
		{
			name:     "nil apps - returns empty",
			apps:     nil,
			scope:    "native",
			wantName: "",
			wantPath: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotName, gotApp, err := resolveApp(tt.apps, tt.scope, tt.explicitApp)

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.errContains)
					return
				}
				if !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("error = %q, want containing %q", err.Error(), tt.errContains)
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if gotName != tt.wantName {
				t.Errorf("name = %q, want %q", gotName, tt.wantName)
			}
			if gotApp.Path != tt.wantPath {
				t.Errorf("path = %q, want %q", gotApp.Path, tt.wantPath)
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
		config   *PreCommitConfig
		wantApps []string
	}{
		{
			name:     "nil config",
			config:   nil,
			wantApps: nil,
		},
		{
			name: "no changelog.apps - returns all apps",
			config: &PreCommitConfig{
				Apps:      allApps,
				Changelog: ChangelogConfig{Mode: "per-app"},
			},
			wantApps: []string{"native", "web", "backend"},
		},
		{
			name: "changelog.apps filters to specified apps",
			config: &PreCommitConfig{
				Apps:      allApps,
				Changelog: ChangelogConfig{Mode: "per-app", Apps: []string{"native", "web"}},
			},
			wantApps: []string{"native", "web"},
		},
		{
			name: "changelog.apps with unknown app - ignores unknown",
			config: &PreCommitConfig{
				Apps:      allApps,
				Changelog: ChangelogConfig{Mode: "per-app", Apps: []string{"native", "unknown"}},
			},
			wantApps: []string{"native"},
		},
		{
			name: "changelog.apps all unknown - returns empty",
			config: &PreCommitConfig{
				Apps:      allApps,
				Changelog: ChangelogConfig{Mode: "per-app", Apps: []string{"unknown1", "unknown2"}},
			},
			wantApps: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getChangelogApps(tt.config)

			if tt.wantApps == nil {
				if got != nil {
					t.Errorf("expected nil, got %v", got)
				}
				return
			}

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

func TestLoadConfig(t *testing.T) {
	tests := []struct {
		name        string
		configJSON  string
		wantMode    string
		wantApps    int
		wantErr     bool
		errContains string
	}{
		{
			name: "full config with changelog",
			configJSON: `{
				"apps": {
					"native": {"path": "apps/native"},
					"web": {"path": "apps/web"}
				},
				"changelog": {
					"mode": "per-app",
					"apps": ["native"]
				}
			}`,
			wantMode: "per-app",
			wantApps: 2,
		},
		{
			name: "config without changelog - defaults to global",
			configJSON: `{
				"apps": {
					"native": {"path": "apps/native"}
				}
			}`,
			wantMode: "global",
			wantApps: 1,
		},
		{
			name: "config with empty changelog mode - defaults to global",
			configJSON: `{
				"apps": {
					"native": {"path": "apps/native"}
				},
				"changelog": {}
			}`,
			wantMode: "global",
			wantApps: 1,
		},
		{
			name: "config with required mode",
			configJSON: `{
				"apps": {
					"native": {"path": "apps/native"}
				},
				"changelog": {
					"mode": "required"
				}
			}`,
			wantMode: "required",
			wantApps: 1,
		},
		{
			name:        "invalid JSON",
			configJSON:  `{invalid}`,
			wantErr:     true,
			errContains: "invalid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp directory with config
			tmpDir := t.TempDir()
			configPath := filepath.Join(tmpDir, ".pre-commit.json")
			if err := os.WriteFile(configPath, []byte(tt.configJSON), 0644); err != nil {
				t.Fatalf("failed to write config: %v", err)
			}

			config, err := loadConfig(tmpDir)

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil")
					return
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if config.Changelog.Mode != tt.wantMode {
				t.Errorf("mode = %q, want %q", config.Changelog.Mode, tt.wantMode)
			}

			if len(config.Apps) != tt.wantApps {
				t.Errorf("got %d apps, want %d", len(config.Apps), tt.wantApps)
			}
		})
	}
}

func TestLoadConfigMissingFile(t *testing.T) {
	tmpDir := t.TempDir()
	_, err := loadConfig(tmpDir)
	if err == nil {
		t.Error("expected error for missing config file")
	}
}

func TestCreateFragment(t *testing.T) {
	tests := []struct {
		name           string
		entry          string
		appName        string
		appPath        string
		wantDirSuffix  string
		wantFilePrefix string
		wantContent    string
		wantErr        bool
	}{
		{
			name:           "global changelog",
			entry:          "feat: add feature",
			appName:        "",
			appPath:        "",
			wantDirSuffix:  ".changelog",
			wantFilePrefix: "-feat-add-feature.txt",
			wantContent:    "feat: add feature\n",
		},
		{
			name:           "app-specific changelog",
			entry:          "feat(native): add login",
			appName:        "native",
			appPath:        "apps/native",
			wantDirSuffix:  "apps/native/.changelog",
			wantFilePrefix: "-feat-native-add-login.txt",
			wantContent:    "feat(native): add login\n",
		},
		{
			name:           "entry with scope in filename",
			entry:          "fix(web): resolve bug",
			appName:        "web",
			appPath:        "apps/web",
			wantDirSuffix:  "apps/web/.changelog",
			wantFilePrefix: "-fix-web-resolve-bug.txt",
			wantContent:    "fix(web): resolve bug\n",
		},
		{
			name:    "invalid entry format",
			entry:   "invalid entry",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()

			// Create app directory if needed
			if tt.appPath != "" {
				if err := os.MkdirAll(filepath.Join(tmpDir, tt.appPath), 0755); err != nil {
					t.Fatalf("failed to create app dir: %v", err)
				}
			}

			fragmentPath, err := createFragment(tt.entry, tt.appName, tt.appPath, tmpDir)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			// Check directory
			fullPath := filepath.Join(tmpDir, fragmentPath)
			dir := filepath.Dir(fullPath)
			if !strings.HasSuffix(dir, tt.wantDirSuffix) {
				t.Errorf("directory = %q, want suffix %q", dir, tt.wantDirSuffix)
			}

			// Check filename
			filename := filepath.Base(fragmentPath)
			if !strings.HasSuffix(filename, tt.wantFilePrefix) {
				t.Errorf("filename = %q, want suffix %q", filename, tt.wantFilePrefix)
			}

			// Check content
			content, err := os.ReadFile(fullPath)
			if err != nil {
				t.Errorf("failed to read fragment: %v", err)
				return
			}
			if string(content) != tt.wantContent {
				t.Errorf("content = %q, want %q", string(content), tt.wantContent)
			}

			// Check .gitkeep exists
			gitkeepPath := filepath.Join(dir, ".gitkeep")
			if _, err := os.Stat(gitkeepPath); os.IsNotExist(err) {
				t.Error(".gitkeep not created")
			}
		})
	}
}

func TestFindProjectRoot(t *testing.T) {
	// Create temp directory structure
	tmpDir := t.TempDir()

	// Helper to resolve symlinks for path comparison (needed on macOS)
	resolvePath := func(p string) string {
		resolved, err := filepath.EvalSymlinks(p)
		if err != nil {
			return p
		}
		return resolved
	}

	// Test with .pre-commit.json
	t.Run("finds .pre-commit.json", func(t *testing.T) {
		projectDir := filepath.Join(tmpDir, "project1")
		subDir := filepath.Join(projectDir, "apps", "native")
		if err := os.MkdirAll(subDir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(projectDir, ".pre-commit.json"), []byte("{}"), 0644); err != nil {
			t.Fatal(err)
		}

		// Change to subdirectory
		oldWd, _ := os.Getwd()
		defer os.Chdir(oldWd)
		os.Chdir(subDir)

		root, err := findProjectRoot()
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if resolvePath(root) != resolvePath(projectDir) {
			t.Errorf("root = %q, want %q", root, projectDir)
		}
	})

	// Test with pnpm-workspace.yaml
	t.Run("finds pnpm-workspace.yaml", func(t *testing.T) {
		projectDir := filepath.Join(tmpDir, "project2")
		subDir := filepath.Join(projectDir, "packages", "backend")
		if err := os.MkdirAll(subDir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(projectDir, "pnpm-workspace.yaml"), []byte("packages:\n  - packages/*"), 0644); err != nil {
			t.Fatal(err)
		}

		oldWd, _ := os.Getwd()
		defer os.Chdir(oldWd)
		os.Chdir(subDir)

		root, err := findProjectRoot()
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if resolvePath(root) != resolvePath(projectDir) {
			t.Errorf("root = %q, want %q", root, projectDir)
		}
	})

	// Test with package.json
	t.Run("finds package.json", func(t *testing.T) {
		projectDir := filepath.Join(tmpDir, "project3")
		subDir := filepath.Join(projectDir, "src")
		if err := os.MkdirAll(subDir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(projectDir, "package.json"), []byte("{}"), 0644); err != nil {
			t.Fatal(err)
		}

		oldWd, _ := os.Getwd()
		defer os.Chdir(oldWd)
		os.Chdir(subDir)

		root, err := findProjectRoot()
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if resolvePath(root) != resolvePath(projectDir) {
			t.Errorf("root = %q, want %q", root, projectDir)
		}
	})
}

func TestChangelogModes(t *testing.T) {
	apps := map[string]AppConfig{
		"native": {Path: "apps/native"},
		"web":    {Path: "apps/web"},
	}

	tests := []struct {
		name        string
		mode        string
		scope       string
		explicitApp string
		globalFlag  bool
		wantAppName string
		wantAppPath string
		wantErr     bool
	}{
		// Global mode tests
		{
			name:        "global mode - no scope",
			mode:        "global",
			scope:       "",
			wantAppName: "",
			wantAppPath: "",
		},
		{
			name:        "global mode - with scope (ignored)",
			mode:        "global",
			scope:       "native",
			wantAppName: "",
			wantAppPath: "",
		},

		// Per-app mode tests
		{
			name:        "per-app mode - matching scope",
			mode:        "per-app",
			scope:       "native",
			wantAppName: "native",
			wantAppPath: "apps/native",
		},
		{
			name:        "per-app mode - no scope falls back to root",
			mode:        "per-app",
			scope:       "",
			wantAppName: "",
			wantAppPath: "",
		},
		{
			name:        "per-app mode - unknown scope falls back to root",
			mode:        "per-app",
			scope:       "unknown",
			wantAppName: "",
			wantAppPath: "",
		},
		{
			name:        "per-app mode - explicit app",
			mode:        "per-app",
			scope:       "",
			explicitApp: "web",
			wantAppName: "web",
			wantAppPath: "apps/web",
		},

		// Required mode tests
		{
			name:        "required mode - matching scope",
			mode:        "required",
			scope:       "native",
			wantAppName: "native",
			wantAppPath: "apps/native",
		},
		{
			name:    "required mode - no scope errors",
			mode:    "required",
			scope:   "",
			wantErr: true,
		},
		{
			name:    "required mode - unknown scope errors",
			mode:    "required",
			scope:   "unknown",
			wantErr: true,
		},
		{
			name:        "required mode - explicit app",
			mode:        "required",
			scope:       "",
			explicitApp: "web",
			wantAppName: "web",
			wantAppPath: "apps/web",
		},

		// Global flag override tests
		{
			name:        "global flag overrides per-app mode",
			mode:        "per-app",
			scope:       "native",
			globalFlag:  true,
			wantAppName: "",
			wantAppPath: "",
		},
		{
			name:        "global flag overrides required mode",
			mode:        "required",
			scope:       "native",
			globalFlag:  true,
			wantAppName: "",
			wantAppPath: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var appName, appPath string
			var err error

			if tt.globalFlag {
				appName = ""
				appPath = ""
			} else {
				switch tt.mode {
				case "global":
					appName = ""
					appPath = ""

				case "per-app":
					resolvedName, resolvedApp, resolveErr := resolveApp(apps, tt.scope, tt.explicitApp)
					if resolveErr != nil {
						err = resolveErr
					} else {
						appName = resolvedName
						appPath = resolvedApp.Path
					}

				case "required":
					resolvedName, resolvedApp, resolveErr := resolveApp(apps, tt.scope, tt.explicitApp)
					if resolveErr != nil {
						err = resolveErr
					} else if resolvedName == "" {
						err = &requiredModeError{scope: tt.scope}
					} else {
						appName = resolvedName
						appPath = resolvedApp.Path
					}
				}
			}

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if appName != tt.wantAppName {
				t.Errorf("appName = %q, want %q", appName, tt.wantAppName)
			}
			if appPath != tt.wantAppPath {
				t.Errorf("appPath = %q, want %q", appPath, tt.wantAppPath)
			}
		})
	}
}

// requiredModeError is a helper for testing required mode
type requiredModeError struct {
	scope string
}

func (e *requiredModeError) Error() string {
	return "scope required"
}

func TestValidTypes(t *testing.T) {
	expectedTypes := []string{
		"feat", "fix", "chore", "docs", "test",
		"style", "refactor", "perf", "build", "ci", "revert",
	}

	for _, typ := range expectedTypes {
		if !validTypes[typ] {
			t.Errorf("expected %q to be a valid type", typ)
		}
	}

	if len(validTypes) != len(expectedTypes) {
		t.Errorf("got %d valid types, want %d", len(validTypes), len(expectedTypes))
	}
}
