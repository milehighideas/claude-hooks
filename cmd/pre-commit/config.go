package main

import (
	"encoding/json"
	"os"
	"strings"
)

// Config represents the .pre-commit.json configuration
type Config struct {
	PackageManager     string                `json:"packageManager"`     // Global package manager: "pnpm" (default), "bun", "npm", "yarn"
	Env                map[string]string     `json:"env"`                // Global environment variables for all commands
	Apps               map[string]AppConfig  `json:"apps"`
	SharedPaths        []string              `json:"sharedPaths"`
	ReportDir          string                `json:"reportDir"` // Directory to write analysis reports
	Features           Features              `json:"features"`
	ProtectedBranches  []string              `json:"protectedBranches"`
	ChangelogExclude   []string              `json:"changelogExclude"`
	ChangelogConfig    ChangelogConfig       `json:"changelog"`
	ConsoleAllowed     []string              `json:"consoleAllowed"`
	TypecheckFilter    TypecheckFilter       `json:"typecheckFilter"`
	LintFilter         LintFilter            `json:"lintFilter"`
	LintStagedConfig   LintStagedConfig      `json:"lintStagedConfig"`
	GoLint             GoLintConfig          `json:"goLint"`
	NativeBuild        NativeBuildConfig     `json:"nativeBuild"`
	Convex             ConvexConfig          `json:"convex"`
	Build              BuildConfig           `json:"build"`
	MockCheck          MockCheckConfig       `json:"mockCheck"`
	TestConfig         TestConfig            `json:"testConfig"`
	TestCoverageConfig TestCoverageConfig    `json:"testCoverageConfig"`
	SRPConfig          SRPConfig             `json:"srpConfig"`
	DataLayerAllowed   []string              `json:"dataLayerAllowed"`
	WarningChecks      []string              `json:"warningChecks"`    // Checks listed here run but don't block commits
}

// IsWarningCheck returns true if the named check should warn instead of block.
func (c *Config) IsWarningCheck(name string) bool {
	for _, w := range c.WarningChecks {
		if w == name {
			return true
		}
	}
	return false
}

// Features represents which pre-commit features are enabled
type Features struct {
	LintTypecheck      bool `json:"lintTypecheck"`
	LintStaged         bool `json:"lintStaged"`
	FullLintOnCommit   bool `json:"fullLintOnCommit"`
	Tests              bool `json:"tests"`
	Changelog          bool `json:"changelog"`
	ConsoleCheck       bool `json:"consoleCheck"`
	BranchProtection   bool `json:"branchProtection"`
	GoLint             bool `json:"goLint"`
	ConvexValidation   bool `json:"convexValidation"`
	BuildCheck         bool `json:"buildCheck"`
	FrontendStructure  bool `json:"frontendStructure"`
	SRP                bool `json:"srp"`
	FullSRPOnCommit    bool `json:"fullSRPOnCommit"`
	TestFiles          bool `json:"testFiles"`
	MockCheck          bool `json:"mockCheck"`
	VitestAssertions   bool `json:"vitestAssertions"`
	TestCoverage       bool `json:"testCoverage"`
	NativeBuild        bool `json:"nativeBuild"`
	DataLayerCheck     bool `json:"dataLayerCheck"`
}

// AppConfig represents configuration for a single app
type AppConfig struct {
	Path            string           `json:"path"`
	Filter          string           `json:"filter"`
	TestCommand     string           `json:"testCommand,omitempty"`
	TestArgs        []string         `json:"testArgs,omitempty"`        // Extra args passed to the test runner after "--" (e.g., ["--watchman=false"])
	NodeMemoryMB    int              `json:"nodeMemoryMB,omitempty"`    // Memory limit for Node.js (e.g., 8192 for 8GB)
	TypecheckFilter *TypecheckFilter `json:"typecheckFilter,omitempty"` // Per-app override for typecheck settings
	SkipLint        bool             `json:"skipLint,omitempty"`        // Skip lint for this app (typecheck still runs)
	SkipTypecheck   bool             `json:"skipTypecheck,omitempty"`   // Skip typecheck for this app (lint still runs)
}

// TypecheckFilter configures which TypeScript errors to filter out
type TypecheckFilter struct {
	ErrorCodes     []string `json:"errorCodes"`
	ExcludePaths   []string `json:"excludePaths"`
	ErrorCodePaths []string `json:"errorCodePaths"`
	SkipLibCheck   *bool    `json:"skipLibCheck,omitempty"`   // If false, check .d.ts files (stricter). Default: true
	UseBuildMode   *bool    `json:"useBuildMode,omitempty"`   // If true, use `tsc -b` instead of `tsc --noEmit`. Default: false
}

// LintFilter configures which lint errors to filter out
type LintFilter struct {
	Rules          []string `json:"rules"`
	ExcludePaths   []string `json:"excludePaths"`
	Linter         string   `json:"linter"`         // "eslint" (default) or "oxlint"
	IgnoreWarnings bool     `json:"ignoreWarnings"` // If true, filter out warning-level lint errors
}

// LintStagedConfig configures lint-staged execution
type LintStagedConfig struct {
	PackageManager string            `json:"packageManager"` // "pnpm" (default), "bun", "npm", "yarn"
	Env            map[string]string `json:"env"`            // Environment variables to set (e.g., {"COREPACK_ENABLE_STRICT": "0"})
}

// GoLintConfig configures Go linting
type GoLintConfig struct {
	Paths []string `json:"paths"`
	Tool  string   `json:"tool"`
}

// ConvexConfig configures Convex validation
type ConvexConfig struct {
	Path           string `json:"path"`
	SuccessMarker  string `json:"successMarker"`
	PackageManager string `json:"-"` // Inherited from global config
}

// BuildConfig configures build checks
type BuildConfig struct {
	Apps []string `json:"apps"`
}

// TestConfig configures test execution behavior
type TestConfig struct {
	// AffectedOnly runs tests only for apps with staged changes (unless shared paths changed)
	AffectedOnly bool `json:"affectedOnly"`
	// RunOnSharedChanges forces all tests when shared paths change (default: true)
	RunOnSharedChanges *bool `json:"runOnSharedChanges,omitempty"`
	// AppOverrides allows per-app test configuration
	AppOverrides map[string]AppTestOverride `json:"appOverrides,omitempty"`
}

// AppTestOverride configures test behavior for a specific app
type AppTestOverride struct {
	// Enabled overrides the global tests feature flag (nil = use global, true = force on, false = force off)
	Enabled *bool `json:"enabled,omitempty"`
	// Skip disables tests for this app entirely (deprecated: use enabled: false instead)
	Skip bool `json:"skip"`
	// OnlyWhenAffected runs tests only when this app has staged changes (overrides global affectedOnly)
	OnlyWhenAffected *bool `json:"onlyWhenAffected,omitempty"`
}

// TestCoverageConfig configures test file coverage checking
type TestCoverageConfig struct {
	// AppPaths specifies which app paths to check (e.g., ["apps/admin", "apps/portal"])
	AppPaths []string `json:"appPaths"`
	// RequireTestFolders specifies which CRUD folders require test files
	// e.g., ["hooks", "read", "create", "update", "delete", "utils"]
	RequireTestFolders []string `json:"requireTestFolders"`
	// ExcludeFiles specifies file patterns to exclude (e.g., ["index.ts", "*.types.ts"])
	ExcludeFiles []string `json:"excludeFiles"`
	// ExcludePaths specifies path patterns to exclude entirely
	ExcludePaths []string `json:"excludePaths"`
}

// ChangelogConfig configures changelog fragment checking
type ChangelogConfig struct {
	// Mode: "global" (single .changelog/ at root), "per-app" (each app has its own .changelog/,
	// falls back to global for shared changes), or "required" (each affected app must have its own changelog)
	Mode string `json:"mode"`
	// GlobalDir: directory for global changelog (default: ".changelog")
	GlobalDir string `json:"globalDir,omitempty"`
	// Apps: list of app names that have changelog support (optional, defaults to all apps)
	Apps []string `json:"apps,omitempty"`
}

// SRPConfig configures Single Responsibility Principle checking
type SRPConfig struct {
	// AppPaths specifies which app paths to check (e.g., ["apps/portal", "apps/mobile"])
	// If empty, all files are checked
	AppPaths []string `json:"appPaths"`
	// ExcludePaths specifies path patterns to exclude from checking
	ExcludePaths []string `json:"excludePaths"`
	// HideWarnings suppresses warning output, only showing errors
	HideWarnings bool `json:"hideWarnings"`
	// ScreenHooks specifies which React hooks are forbidden in screen/page files.
	// Accepts individual hook names: "useState", "useReducer", "useContext",
	// "useCallback", "useEffect", "useMemo", or "all" to flag all of them.
	// If empty/unset, defaults to ["useState", "useReducer", "useContext"] for
	// backwards compatibility.
	ScreenHooks []string `json:"screenHooks"`
	// EnabledRules specifies which SRP rules to run. If empty/unset, all 6
	// existing rules run (backwards compatible). The "testRequired" rule is
	// always opt-in — it only runs when explicitly listed here.
	EnabledRules []string `json:"enabledRules"`
	// WarnOnly specifies rules whose violations should be downgraded to warnings
	// instead of errors, making them non-blocking.
	WarnOnly []string `json:"warnOnly"`
	// TestRequired configures the testRequired rule (requires enabledRules to include "testRequired")
	TestRequired *TestRequiredConfig `json:"testRequired"`
}

// TestRequiredConfig configures the testRequired SRP rule
type TestRequiredConfig struct {
	// Scope controls which files are checked: "new" (only newly added), "changed" (all staged), "all" (everything).
	// Default: "new"
	Scope string `json:"scope"`
	// Extensions specifies which file extensions to check. Default: [".tsx"]
	Extensions []string `json:"extensions"`
	// TestPatterns specifies test file suffixes to look for. Default: [".test.tsx", ".test.ts", ".spec.tsx", ".spec.ts"]
	TestPatterns []string `json:"testPatterns"`
	// IncludePaths restricts checking to files within these paths (prefix match). If empty, all paths checked.
	IncludePaths []string `json:"includePaths"`
	// ExcludePaths skips files matching these patterns (substring match)
	ExcludePaths []string `json:"excludePaths"`
	// ExcludeFiles skips exact filenames (basename match, e.g., "index.tsx", "_layout.tsx")
	ExcludeFiles []string `json:"excludeFiles"`
}

// resolvedScreenHooks returns the set of hooks to check in screens,
// expanding "all" and falling back to the default set when unset.
func (c SRPConfig) resolvedScreenHooks() map[string]bool {
	defaults := []string{"useState", "useReducer", "useContext"}

	hooks := c.ScreenHooks
	if len(hooks) == 0 {
		hooks = defaults
	}

	result := make(map[string]bool)
	for _, h := range hooks {
		if h == "all" {
			for _, d := range []string{"useState", "useReducer", "useContext", "useCallback", "useEffect", "useMemo"} {
				result[d] = true
			}
		} else {
			result[h] = true
		}
	}
	return result
}

// existingRules lists the 6 original SRP rules (not including testRequired)
var existingRules = []string{
	"directConvexImports",
	"stateInScreens",
	"multipleExports",
	"fileSize",
	"typeExportsLocation",
	"mixedConcerns",
}

// isRuleEnabled returns true if the given rule should run.
// When EnabledRules is empty/nil, all 6 existing rules run and testRequired is off.
// When EnabledRules is set, only listed rules run.
func (c SRPConfig) isRuleEnabled(rule string) bool {
	if len(c.EnabledRules) == 0 {
		// Backwards compat: all existing rules on, testRequired off
		for _, r := range existingRules {
			if r == rule {
				return true
			}
		}
		return false
	}
	for _, r := range c.EnabledRules {
		if r == rule {
			return true
		}
	}
	return false
}

// isWarnOnly returns true if violations from this rule should be downgraded to warnings.
func (c SRPConfig) isWarnOnly(rule string) bool {
	for _, r := range c.WarnOnly {
		if r == rule {
			return true
		}
	}
	return false
}

// resolvedTestRequired returns the TestRequiredConfig with defaults applied.
func (c SRPConfig) resolvedTestRequired() TestRequiredConfig {
	defaults := TestRequiredConfig{
		Scope:        "new",
		Extensions:   []string{".tsx"},
		TestPatterns: []string{".test.tsx", ".test.ts", ".spec.tsx", ".spec.ts"},
	}
	if c.TestRequired == nil {
		return defaults
	}
	cfg := *c.TestRequired
	if cfg.Scope == "" {
		cfg.Scope = defaults.Scope
	}
	if len(cfg.Extensions) == 0 {
		cfg.Extensions = defaults.Extensions
	}
	if len(cfg.TestPatterns) == 0 {
		cfg.TestPatterns = defaults.TestPatterns
	}
	return cfg
}

// stripJSONComments removes single-line // comments from JSONC content.
// It handles comments on their own line and inline after values.
// It does not strip comments inside string literals.
func stripJSONComments(data []byte) []byte {
	lines := strings.Split(string(data), "\n")
	var out []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		// Skip full-line comments
		if strings.HasPrefix(trimmed, "//") {
			continue
		}
		// Strip inline comments (after a value, outside strings)
		// Walk character by character to respect string boundaries
		inString := false
		escaped := false
		for i, ch := range line {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' && inString {
				escaped = true
				continue
			}
			if ch == '"' {
				inString = !inString
				continue
			}
			if !inString && ch == '/' && i+1 < len(line) && line[i+1] == '/' {
				line = strings.TrimRight(line[:i], " \t")
				break
			}
		}
		out = append(out, line)
	}
	return []byte(strings.Join(out, "\n"))
}

// loadConfig loads configuration from .pre-commit.json (supports JSONC comments)
func loadConfig() (*Config, error) {
	configPath := ".pre-commit.json"
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return defaultConfig(), nil
		}
		return nil, err
	}

	// Strip JSONC-style comments before parsing
	data = stripJSONComments(data)

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	applyDefaults(&config)

	return &config, nil
}

// defaultConfig returns the default configuration when no config file exists
func defaultConfig() *Config {
	return &Config{
		Apps: map[string]AppConfig{},
		Features: Features{
			LintTypecheck: true,
			LintStaged:    true,
		},
		GoLint: GoLintConfig{
			Tool: "golangci-lint",
		},
		Convex: ConvexConfig{
			SuccessMarker: "Convex functions ready!",
		},
	}
}

// GetTypecheckFilter returns the effective typecheck filter for an app,
// merging global settings with any per-app overrides
func GetTypecheckFilter(global TypecheckFilter, appOverride *TypecheckFilter) TypecheckFilter {
	if appOverride == nil {
		return global
	}

	result := global

	// Per-app settings override global when specified
	if len(appOverride.ErrorCodes) > 0 {
		result.ErrorCodes = appOverride.ErrorCodes
	}
	if len(appOverride.ExcludePaths) > 0 {
		result.ExcludePaths = appOverride.ExcludePaths
	}
	if len(appOverride.ErrorCodePaths) > 0 {
		result.ErrorCodePaths = appOverride.ErrorCodePaths
	}
	if appOverride.SkipLibCheck != nil {
		result.SkipLibCheck = appOverride.SkipLibCheck
	}
	if appOverride.UseBuildMode != nil {
		result.UseBuildMode = appOverride.UseBuildMode
	}

	return result
}

// applyDefaults sets default values for fields that weren't specified in JSON
func applyDefaults(config *Config) {
	if config.PackageManager == "" {
		config.PackageManager = "pnpm"
	}
	if config.GoLint.Tool == "" {
		config.GoLint.Tool = "golangci-lint"
	}
	if config.Convex.SuccessMarker == "" {
		config.Convex.SuccessMarker = "Convex functions ready!"
	}
	if config.Convex.PackageManager == "" {
		config.Convex.PackageManager = config.PackageManager
	}
	// LintStagedConfig inherits from global if not set
	if config.LintStagedConfig.PackageManager == "" {
		config.LintStagedConfig.PackageManager = config.PackageManager
	}
	// Merge global env into LintStagedConfig.Env (LintStagedConfig takes precedence)
	if len(config.Env) > 0 && config.LintStagedConfig.Env == nil {
		config.LintStagedConfig.Env = make(map[string]string)
	}
	for k, v := range config.Env {
		if _, exists := config.LintStagedConfig.Env[k]; !exists {
			config.LintStagedConfig.Env[k] = v
		}
	}
	// Default: run all tests when shared paths change
	if config.TestConfig.RunOnSharedChanges == nil {
		defaultTrue := true
		config.TestConfig.RunOnSharedChanges = &defaultTrue
	}
	// Default: skip lib check for backwards compatibility (lenient mode)
	if config.TypecheckFilter.SkipLibCheck == nil {
		defaultTrue := true
		config.TypecheckFilter.SkipLibCheck = &defaultTrue
	}
	// Default: global changelog mode
	if config.ChangelogConfig.Mode == "" {
		config.ChangelogConfig.Mode = "global"
	}
	if config.ChangelogConfig.GlobalDir == "" {
		config.ChangelogConfig.GlobalDir = ".changelog"
	}
}
