package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	tests := []struct {
		name           string
		configContent  string
		configExists   bool
		wantErr        bool
		validate       func(t *testing.T, config *Config)
	}{
		{
			name:         "missing config file returns defaults",
			configExists: false,
			wantErr:      false,
			validate: func(t *testing.T, config *Config) {
				if config == nil {
					t.Fatal("expected config, got nil")
				}
				if !config.Features.LintTypecheck {
					t.Error("expected LintTypecheck to be true by default")
				}
				if !config.Features.LintStaged {
					t.Error("expected LintStaged to be true by default")
				}
				if config.Features.GoLint {
					t.Error("expected GoLint to be false by default")
				}
				if config.Features.ConvexValidation {
					t.Error("expected ConvexValidation to be false by default")
				}
				if config.Features.BuildCheck {
					t.Error("expected BuildCheck to be false by default")
				}
				if config.GoLint.Tool != "golangci-lint" {
					t.Errorf("expected GoLint.Tool to be 'golangci-lint', got %q", config.GoLint.Tool)
				}
				if config.Convex.SuccessMarker != "Convex functions ready!" {
					t.Errorf("expected Convex.SuccessMarker to be 'Convex functions ready!', got %q", config.Convex.SuccessMarker)
				}
			},
		},
		{
			name:         "invalid JSON returns error",
			configExists: true,
			configContent: `{
				"features": {
					"lintTypecheck": true,
				}
			}`,
			wantErr: true,
		},
		{
			name:         "valid config loads correctly",
			configExists: true,
			configContent: `{
				"apps": {
					"web": {
						"path": "apps/web",
						"filter": "@repo/web"
					}
				},
				"sharedPaths": ["packages/"],
				"features": {
					"lintTypecheck": true,
					"lintStaged": false,
					"tests": true,
					"goLint": true,
					"convexValidation": true,
					"buildCheck": true
				},
				"protectedBranches": ["main", "production"],
				"goLint": {
					"paths": ["apps/vendor-sync"],
					"tool": "go-vet"
				},
				"convex": {
					"path": "packages/backend",
					"successMarker": "Custom success!"
				},
				"build": {
					"apps": ["web", "native"]
				}
			}`,
			wantErr: false,
			validate: func(t *testing.T, config *Config) {
				// Check apps
				if len(config.Apps) != 1 {
					t.Errorf("expected 1 app, got %d", len(config.Apps))
				}
				if config.Apps["web"].Path != "apps/web" {
					t.Errorf("expected web path 'apps/web', got %q", config.Apps["web"].Path)
				}
				if config.Apps["web"].Filter != "@repo/web" {
					t.Errorf("expected web filter '@repo/web', got %q", config.Apps["web"].Filter)
				}

				// Check shared paths
				if len(config.SharedPaths) != 1 || config.SharedPaths[0] != "packages/" {
					t.Errorf("expected sharedPaths ['packages/'], got %v", config.SharedPaths)
				}

				// Check features
				if !config.Features.LintTypecheck {
					t.Error("expected LintTypecheck to be true")
				}
				if config.Features.LintStaged {
					t.Error("expected LintStaged to be false")
				}
				if !config.Features.Tests {
					t.Error("expected Tests to be true")
				}
				if !config.Features.GoLint {
					t.Error("expected GoLint to be true")
				}
				if !config.Features.ConvexValidation {
					t.Error("expected ConvexValidation to be true")
				}
				if !config.Features.BuildCheck {
					t.Error("expected BuildCheck to be true")
				}

				// Check protected branches
				if len(config.ProtectedBranches) != 2 {
					t.Errorf("expected 2 protected branches, got %d", len(config.ProtectedBranches))
				}

				// Check GoLint config
				if len(config.GoLint.Paths) != 1 || config.GoLint.Paths[0] != "apps/vendor-sync" {
					t.Errorf("expected GoLint.Paths ['apps/vendor-sync'], got %v", config.GoLint.Paths)
				}
				if config.GoLint.Tool != "go-vet" {
					t.Errorf("expected GoLint.Tool 'go-vet', got %q", config.GoLint.Tool)
				}

				// Check Convex config
				if config.Convex.Path != "packages/backend" {
					t.Errorf("expected Convex.Path 'packages/backend', got %q", config.Convex.Path)
				}
				if config.Convex.SuccessMarker != "Custom success!" {
					t.Errorf("expected Convex.SuccessMarker 'Custom success!', got %q", config.Convex.SuccessMarker)
				}

				// Check Build config
				if len(config.Build.Apps) != 2 {
					t.Errorf("expected 2 build apps, got %d", len(config.Build.Apps))
				}
				if config.Build.Apps[0] != "web" || config.Build.Apps[1] != "native" {
					t.Errorf("expected Build.Apps ['web', 'native'], got %v", config.Build.Apps)
				}
			},
		},
		{
			name:         "partial config with defaults applied",
			configExists: true,
			configContent: `{
				"features": {
					"goLint": true
				},
				"goLint": {
					"paths": ["apps/vendor-sync"]
				}
			}`,
			wantErr: false,
			validate: func(t *testing.T, config *Config) {
				// GoLint.Tool should default to golangci-lint
				if config.GoLint.Tool != "golangci-lint" {
					t.Errorf("expected GoLint.Tool default 'golangci-lint', got %q", config.GoLint.Tool)
				}
				// Convex.SuccessMarker should default
				if config.Convex.SuccessMarker != "Convex functions ready!" {
					t.Errorf("expected Convex.SuccessMarker default, got %q", config.Convex.SuccessMarker)
				}
				// Paths should be set from config
				if len(config.GoLint.Paths) != 1 || config.GoLint.Paths[0] != "apps/vendor-sync" {
					t.Errorf("expected GoLint.Paths ['apps/vendor-sync'], got %v", config.GoLint.Paths)
				}
			},
		},
		{
			name:         "empty config object",
			configExists: true,
			configContent: `{}`,
			wantErr:       false,
			validate: func(t *testing.T, config *Config) {
				if config == nil {
					t.Fatal("expected config, got nil")
				}
				// Defaults should be applied
				if config.GoLint.Tool != "golangci-lint" {
					t.Errorf("expected GoLint.Tool default, got %q", config.GoLint.Tool)
				}
				if config.Convex.SuccessMarker != "Convex functions ready!" {
					t.Errorf("expected Convex.SuccessMarker default, got %q", config.Convex.SuccessMarker)
				}
			},
		},
		{
			name:         "typecheck filter config",
			configExists: true,
			configContent: `{
				"typecheckFilter": {
					"errorCodes": ["TS2589", "TS2742"],
					"excludePaths": ["__tests__/", ".test."],
					"errorCodePaths": ["packages/backend/"]
				}
			}`,
			wantErr: false,
			validate: func(t *testing.T, config *Config) {
				if len(config.TypecheckFilter.ErrorCodes) != 2 {
					t.Errorf("expected 2 error codes, got %d", len(config.TypecheckFilter.ErrorCodes))
				}
				if config.TypecheckFilter.ErrorCodes[0] != "TS2589" {
					t.Errorf("expected first error code 'TS2589', got %q", config.TypecheckFilter.ErrorCodes[0])
				}
				if len(config.TypecheckFilter.ExcludePaths) != 2 {
					t.Errorf("expected 2 exclude paths, got %d", len(config.TypecheckFilter.ExcludePaths))
				}
			},
		},
		{
			name:         "app with custom test command",
			configExists: true,
			configContent: `{
				"apps": {
					"backend": {
						"path": "packages/backend",
						"filter": "@repo/backend",
						"testCommand": "test:unit"
					}
				}
			}`,
			wantErr: false,
			validate: func(t *testing.T, config *Config) {
				if config.Apps["backend"].TestCommand != "test:unit" {
					t.Errorf("expected testCommand 'test:unit', got %q", config.Apps["backend"].TestCommand)
				}
			},
		},
		{
			name:         "changelog exclude patterns",
			configExists: true,
			configContent: `{
				"changelogExclude": ["^docs/", "^.github/", "\\.md$"]
			}`,
			wantErr: false,
			validate: func(t *testing.T, config *Config) {
				if len(config.ChangelogExclude) != 3 {
					t.Errorf("expected 3 changelog exclude patterns, got %d", len(config.ChangelogExclude))
				}
			},
		},
		{
			name:         "console allowed files",
			configExists: true,
			configContent: `{
				"consoleAllowed": ["apps/web/lib/logger.ts", "packages/utils/debug.ts"]
			}`,
			wantErr: false,
			validate: func(t *testing.T, config *Config) {
				if len(config.ConsoleAllowed) != 2 {
					t.Errorf("expected 2 console allowed files, got %d", len(config.ConsoleAllowed))
				}
			},
		},
		{
			name:         "SRP config with appPaths and excludePaths",
			configExists: true,
			configContent: `{
				"features": {
					"srp": true
				},
				"srpConfig": {
					"appPaths": ["apps/portal", "apps/mobile", "apps/admin"],
					"excludePaths": ["packages/react-email/", "apps/match/"]
				}
			}`,
			wantErr: false,
			validate: func(t *testing.T, config *Config) {
				if !config.Features.SRP {
					t.Error("expected SRP feature to be true")
				}
				if len(config.SRPConfig.AppPaths) != 3 {
					t.Errorf("expected 3 app paths, got %d", len(config.SRPConfig.AppPaths))
				}
				if config.SRPConfig.AppPaths[0] != "apps/portal" {
					t.Errorf("expected first app path 'apps/portal', got %q", config.SRPConfig.AppPaths[0])
				}
				if len(config.SRPConfig.ExcludePaths) != 2 {
					t.Errorf("expected 2 exclude paths, got %d", len(config.SRPConfig.ExcludePaths))
				}
				if config.SRPConfig.ExcludePaths[0] != "packages/react-email/" {
					t.Errorf("expected first exclude path 'packages/react-email/', got %q", config.SRPConfig.ExcludePaths[0])
				}
			},
		},
		{
			name:         "SRP config with empty appPaths uses all files",
			configExists: true,
			configContent: `{
				"features": {
					"srp": true
				},
				"srpConfig": {
					"excludePaths": ["apps/legacy/"]
				}
			}`,
			wantErr: false,
			validate: func(t *testing.T, config *Config) {
				if len(config.SRPConfig.AppPaths) != 0 {
					t.Errorf("expected 0 app paths (empty), got %d", len(config.SRPConfig.AppPaths))
				}
				if len(config.SRPConfig.ExcludePaths) != 1 {
					t.Errorf("expected 1 exclude path, got %d", len(config.SRPConfig.ExcludePaths))
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp directory for test
			tempDir := t.TempDir()
			origDir, err := os.Getwd()
			if err != nil {
				t.Fatalf("failed to get working directory: %v", err)
			}
			defer os.Chdir(origDir)

			if err := os.Chdir(tempDir); err != nil {
				t.Fatalf("failed to change to temp directory: %v", err)
			}

			// Create config file if needed
			if tt.configExists {
				configPath := filepath.Join(tempDir, ".pre-commit.json")
				if err := os.WriteFile(configPath, []byte(tt.configContent), 0644); err != nil {
					t.Fatalf("failed to write config file: %v", err)
				}
			}

			// Run loadConfig
			config, err := loadConfig()

			// Check error expectation
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Run validation
			if tt.validate != nil {
				tt.validate(t, config)
			}
		})
	}
}

func TestDefaultConfig(t *testing.T) {
	config := defaultConfig()

	if config == nil {
		t.Fatal("defaultConfig returned nil")
	}

	// Check apps is initialized
	if config.Apps == nil {
		t.Error("Apps should be initialized, got nil")
	}

	// Check default features
	if !config.Features.LintTypecheck {
		t.Error("LintTypecheck should be true by default")
	}
	if !config.Features.LintStaged {
		t.Error("LintStaged should be true by default")
	}
	if config.Features.Tests {
		t.Error("Tests should be false by default")
	}
	if config.Features.Changelog {
		t.Error("Changelog should be false by default")
	}
	if config.Features.ConsoleCheck {
		t.Error("ConsoleCheck should be false by default")
	}
	if config.Features.BranchProtection {
		t.Error("BranchProtection should be false by default")
	}
	if config.Features.GoLint {
		t.Error("GoLint should be false by default")
	}
	if config.Features.ConvexValidation {
		t.Error("ConvexValidation should be false by default")
	}
	if config.Features.BuildCheck {
		t.Error("BuildCheck should be false by default")
	}

	// Check GoLint defaults
	if config.GoLint.Tool != "golangci-lint" {
		t.Errorf("GoLint.Tool should default to 'golangci-lint', got %q", config.GoLint.Tool)
	}

	// Check Convex defaults
	if config.Convex.SuccessMarker != "Convex functions ready!" {
		t.Errorf("Convex.SuccessMarker should default to 'Convex functions ready!', got %q", config.Convex.SuccessMarker)
	}
}

func TestApplyDefaults(t *testing.T) {
	tests := []struct {
		name     string
		config   Config
		validate func(t *testing.T, config *Config)
	}{
		{
			name: "empty GoLint.Tool gets default",
			config: Config{
				GoLint: GoLintConfig{
					Paths: []string{"apps/vendor-sync"},
					Tool:  "",
				},
			},
			validate: func(t *testing.T, config *Config) {
				if config.GoLint.Tool != "golangci-lint" {
					t.Errorf("expected Tool 'golangci-lint', got %q", config.GoLint.Tool)
				}
				// Paths should remain unchanged
				if len(config.GoLint.Paths) != 1 {
					t.Errorf("expected 1 path, got %d", len(config.GoLint.Paths))
				}
			},
		},
		{
			name: "non-empty GoLint.Tool preserved",
			config: Config{
				GoLint: GoLintConfig{
					Tool: "go-vet",
				},
			},
			validate: func(t *testing.T, config *Config) {
				if config.GoLint.Tool != "go-vet" {
					t.Errorf("expected Tool 'go-vet', got %q", config.GoLint.Tool)
				}
			},
		},
		{
			name: "empty Convex.SuccessMarker gets default",
			config: Config{
				Convex: ConvexConfig{
					Path:          "packages/backend",
					SuccessMarker: "",
				},
			},
			validate: func(t *testing.T, config *Config) {
				if config.Convex.SuccessMarker != "Convex functions ready!" {
					t.Errorf("expected SuccessMarker 'Convex functions ready!', got %q", config.Convex.SuccessMarker)
				}
				// Path should remain unchanged
				if config.Convex.Path != "packages/backend" {
					t.Errorf("expected Path 'packages/backend', got %q", config.Convex.Path)
				}
			},
		},
		{
			name: "non-empty Convex.SuccessMarker preserved",
			config: Config{
				Convex: ConvexConfig{
					SuccessMarker: "Custom marker!",
				},
			},
			validate: func(t *testing.T, config *Config) {
				if config.Convex.SuccessMarker != "Custom marker!" {
					t.Errorf("expected SuccessMarker 'Custom marker!', got %q", config.Convex.SuccessMarker)
				}
			},
		},
		{
			name: "both defaults applied",
			config: Config{
				GoLint: GoLintConfig{},
				Convex: ConvexConfig{},
			},
			validate: func(t *testing.T, config *Config) {
				if config.GoLint.Tool != "golangci-lint" {
					t.Errorf("expected GoLint.Tool 'golangci-lint', got %q", config.GoLint.Tool)
				}
				if config.Convex.SuccessMarker != "Convex functions ready!" {
					t.Errorf("expected Convex.SuccessMarker 'Convex functions ready!', got %q", config.Convex.SuccessMarker)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := tt.config
			applyDefaults(&config)
			tt.validate(t, &config)
		})
	}
}

func TestStripJSONComments(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "no comments unchanged",
			input: `{"key": "value"}`,
			want:  `{"key": "value"}`,
		},
		{
			name:  "full-line comment removed",
			input: "// this is a comment\n{\"key\": \"value\"}",
			want:  "{\"key\": \"value\"}",
		},
		{
			name:  "inline comment removed",
			input: "{\"key\": \"value\" // inline comment\n}",
			want:  "{\"key\": \"value\"\n}",
		},
		{
			name:  "comment-like string preserved",
			input: `{"url": "https://example.com"}`,
			want:  `{"url": "https://example.com"}`,
		},
		{
			name:  "double slash in string value preserved",
			input: `{"pattern": "src//lib"}`,
			want:  `{"pattern": "src//lib"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := string(stripJSONComments([]byte(tt.input)))
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}
