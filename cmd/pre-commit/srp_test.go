package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolvedScreenHooks(t *testing.T) {
	tests := []struct {
		name     string
		config   SRPConfig
		expected map[string]bool
	}{
		{
			name:   "empty defaults to useState/useReducer/useContext",
			config: SRPConfig{},
			expected: map[string]bool{
				"useState":   true,
				"useReducer": true,
				"useContext": true,
			},
		},
		{
			name:   "all expands to all 6 hooks",
			config: SRPConfig{ScreenHooks: []string{"all"}},
			expected: map[string]bool{
				"useState":    true,
				"useReducer":  true,
				"useContext":   true,
				"useCallback": true,
				"useEffect":   true,
				"useMemo":     true,
			},
		},
		{
			name:   "individual hooks",
			config: SRPConfig{ScreenHooks: []string{"useState", "useEffect"}},
			expected: map[string]bool{
				"useState":  true,
				"useEffect": true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.config.resolvedScreenHooks()
			if len(result) != len(tt.expected) {
				t.Errorf("got %d hooks, want %d", len(result), len(tt.expected))
			}
			for k := range tt.expected {
				if !result[k] {
					t.Errorf("missing hook %s", k)
				}
			}
		})
	}
}

func TestCheckStateInScreens(t *testing.T) {
	tests := []struct {
		name       string
		config     SRPConfig
		code       string
		filePath   string
		wantErrors int
	}{
		{
			name:       "default config flags useState in screen",
			config:     SRPConfig{},
			code:       `const [x, setX] = useState(false);`,
			filePath:   "apps/rsvp/src/screens/FooScreen.tsx",
			wantErrors: 1,
		},
		{
			name:       "default config ignores useEffect in screen",
			config:     SRPConfig{},
			code:       `useEffect(() => {}, []);`,
			filePath:   "apps/rsvp/src/screens/FooScreen.tsx",
			wantErrors: 0,
		},
		{
			name:       "all config flags useEffect in screen",
			config:     SRPConfig{ScreenHooks: []string{"all"}},
			code:       `useEffect(() => {}, []);`,
			filePath:   "apps/rsvp/src/screens/FooScreen.tsx",
			wantErrors: 1,
		},
		{
			name:       "all config flags useMemo in screen",
			config:     SRPConfig{ScreenHooks: []string{"all"}},
			code:       `const val = useMemo(() => 1, []);`,
			filePath:   "apps/rsvp/src/screens/FooScreen.tsx",
			wantErrors: 1,
		},
		{
			name:       "selective config only flags listed hooks",
			config:     SRPConfig{ScreenHooks: []string{"useEffect"}},
			code:       "const [x, setX] = useState(false);\nuseEffect(() => {}, []);",
			filePath:   "apps/rsvp/src/screens/FooScreen.tsx",
			wantErrors: 1, // only useEffect flagged, not useState
		},
		{
			name:       "non-screen file is never flagged",
			config:     SRPConfig{ScreenHooks: []string{"all"}},
			code:       `const [x, setX] = useState(false);`,
			filePath:   "apps/rsvp/src/components/read/Foo.tsx",
			wantErrors: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checker := &SRPChecker{config: tt.config}
			analysis := checker.analyzeCode(tt.code, tt.filePath)
			violations := checker.checkStateInScreens(analysis, tt.filePath)
			if len(violations) != tt.wantErrors {
				t.Errorf("got %d violations, want %d: %+v", len(violations), tt.wantErrors, violations)
			}
		})
	}
}

func TestIsRuleEnabled(t *testing.T) {
	tests := []struct {
		name         string
		enabledRules []string
		rule         string
		want         bool
	}{
		{
			name:         "empty enables all existing rules",
			enabledRules: nil,
			rule:         "directConvexImports",
			want:         true,
		},
		{
			name:         "empty enables stateInScreens",
			enabledRules: nil,
			rule:         "stateInScreens",
			want:         true,
		},
		{
			name:         "empty does NOT enable testRequired",
			enabledRules: nil,
			rule:         "testRequired",
			want:         false,
		},
		{
			name:         "explicit list enables only listed rule",
			enabledRules: []string{"testRequired"},
			rule:         "testRequired",
			want:         true,
		},
		{
			name:         "explicit list excludes unlisted rule",
			enabledRules: []string{"testRequired"},
			rule:         "directConvexImports",
			want:         false,
		},
		{
			name:         "multiple rules in list",
			enabledRules: []string{"fileSize", "testRequired"},
			rule:         "fileSize",
			want:         true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := SRPConfig{EnabledRules: tt.enabledRules}
			if got := cfg.isRuleEnabled(tt.rule); got != tt.want {
				t.Errorf("isRuleEnabled(%q) = %v, want %v", tt.rule, got, tt.want)
			}
		})
	}
}

func TestIsWarnOnly(t *testing.T) {
	tests := []struct {
		name     string
		warnOnly []string
		rule     string
		want     bool
	}{
		{
			name:     "empty warnOnly means nothing is warn-only",
			warnOnly: nil,
			rule:     "testRequired",
			want:     false,
		},
		{
			name:     "rule in warnOnly list",
			warnOnly: []string{"testRequired"},
			rule:     "testRequired",
			want:     true,
		},
		{
			name:     "rule not in warnOnly list",
			warnOnly: []string{"testRequired"},
			rule:     "fileSize",
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := SRPConfig{WarnOnly: tt.warnOnly}
			if got := cfg.isWarnOnly(tt.rule); got != tt.want {
				t.Errorf("isWarnOnly(%q) = %v, want %v", tt.rule, got, tt.want)
			}
		})
	}
}

func TestValidateSRPComplianceWithEnabledRules(t *testing.T) {
	// Code that would trigger both directConvexImports and stateInScreens
	code := "import { useQuery } from 'convex/react';\nconst [x, setX] = useState(false);"
	filePath := "apps/mobile/src/screens/FooScreen.tsx"

	tests := []struct {
		name         string
		enabledRules []string
		wantCount    int
	}{
		{
			name:         "all rules enabled by default",
			enabledRules: nil,
			wantCount:    2, // directConvexImports + stateInScreens
		},
		{
			name:         "only directConvexImports enabled",
			enabledRules: []string{"directConvexImports"},
			wantCount:    1,
		},
		{
			name:         "only stateInScreens enabled",
			enabledRules: []string{"stateInScreens"},
			wantCount:    1,
		},
		{
			name:         "only testRequired enabled skips both",
			enabledRules: []string{"testRequired"},
			wantCount:    0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checker := &SRPChecker{config: SRPConfig{EnabledRules: tt.enabledRules}}
			analysis := checker.analyzeCode(code, filePath)
			violations := checker.validateSRPCompliance(analysis, filePath)
			if len(violations) != tt.wantCount {
				t.Errorf("got %d violations, want %d: %+v", len(violations), tt.wantCount, violations)
			}
		})
	}
}

func TestWarnOnlyOverride(t *testing.T) {
	code := "import { useQuery } from 'convex/react';"
	filePath := "apps/mobile/src/components/Foo.tsx"

	tests := []struct {
		name         string
		warnOnly     []string
		wantSeverity string
	}{
		{
			name:         "without warnOnly violations are errors",
			warnOnly:     nil,
			wantSeverity: "error",
		},
		{
			name:         "with warnOnly violations become warnings",
			warnOnly:     []string{"directConvexImports"},
			wantSeverity: "warning",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checker := &SRPChecker{config: SRPConfig{WarnOnly: tt.warnOnly}}
			analysis := checker.analyzeCode(code, filePath)
			violations := checker.validateSRPCompliance(analysis, filePath)
			if len(violations) == 0 {
				t.Fatal("expected at least 1 violation")
			}
			if violations[0].Severity != tt.wantSeverity {
				t.Errorf("got severity %q, want %q", violations[0].Severity, tt.wantSeverity)
			}
		})
	}
}

func TestCheckTestRequired(t *testing.T) {
	// Create a temp dir for test files
	tmpDir := t.TempDir()

	// Create source file and a test file for one of them
	srcWithTest := filepath.Join(tmpDir, "Button.tsx")
	srcWithoutTest := filepath.Join(tmpDir, "Card.tsx")
	testFile := filepath.Join(tmpDir, "Button.test.tsx")

	os.WriteFile(srcWithTest, []byte("export function Button() {}"), 0644)
	os.WriteFile(srcWithoutTest, []byte("export function Card() {}"), 0644)
	os.WriteFile(testFile, []byte("test('Button', () => {})"), 0644)

	// Source in __tests__ dir
	srcWithTestsDir := filepath.Join(tmpDir, "Modal.tsx")
	testsDir := filepath.Join(tmpDir, "__tests__")
	os.MkdirAll(testsDir, 0755)
	os.WriteFile(srcWithTestsDir, []byte("export function Modal() {}"), 0644)
	os.WriteFile(filepath.Join(testsDir, "Modal.test.tsx"), []byte("test('Modal', () => {})"), 0644)

	tests := []struct {
		name         string
		files        []string
		config       SRPConfig
		newFiles     map[string]bool
		changedFiles map[string]bool
		wantCount    int
	}{
		{
			name:  "file with co-located test passes",
			files: []string{srcWithTest},
			config: SRPConfig{
				EnabledRules: []string{"testRequired"},
				TestRequired: TestRequiredProfiles{{
					Scope:      "all",
					Extensions: []string{".tsx"},
				}},
			},
			wantCount: 0,
		},
		{
			name:  "file without test produces violation",
			files: []string{srcWithoutTest},
			config: SRPConfig{
				EnabledRules: []string{"testRequired"},
				TestRequired: TestRequiredProfiles{{
					Scope:      "all",
					Extensions: []string{".tsx"},
				}},
			},
			wantCount: 1,
		},
		{
			name:  "file with __tests__ dir test passes",
			files: []string{srcWithTestsDir},
			config: SRPConfig{
				EnabledRules: []string{"testRequired"},
				TestRequired: TestRequiredProfiles{{
					Scope:      "all",
					Extensions: []string{".tsx"},
				}},
			},
			wantCount: 0,
		},
		{
			name:  "excluded filename is skipped",
			files: []string{filepath.Join(tmpDir, "index.tsx")},
			config: SRPConfig{
				EnabledRules: []string{"testRequired"},
				TestRequired: TestRequiredProfiles{{
					Scope:        "all",
					Extensions:   []string{".tsx"},
					ExcludeFiles: []string{"index.tsx"},
				}},
			},
			wantCount: 0,
		},
		{
			name:  "excluded path is skipped",
			files: []string{filepath.Join(tmpDir, "__mocks__", "Foo.tsx")},
			config: SRPConfig{
				EnabledRules: []string{"testRequired"},
				TestRequired: TestRequiredProfiles{{
					Scope:        "all",
					Extensions:   []string{".tsx"},
					ExcludePaths: []string{"__mocks__/"},
				}},
			},
			wantCount: 0,
		},
		{
			name:  "outside includePaths is skipped",
			files: []string{srcWithoutTest},
			config: SRPConfig{
				EnabledRules: []string{"testRequired"},
				TestRequired: TestRequiredProfiles{{
					Scope:        "all",
					Extensions:   []string{".tsx"},
					IncludePaths: []string{"src/features/"},
				}},
			},
			wantCount: 0,
		},
		{
			name:  "scope new skips non-new files",
			files: []string{srcWithoutTest},
			config: SRPConfig{
				EnabledRules: []string{"testRequired"},
				TestRequired: TestRequiredProfiles{{
					Scope:      "new",
					Extensions: []string{".tsx"},
				}},
			},
			newFiles:  map[string]bool{}, // srcWithoutTest not in newFiles
			wantCount: 0,
		},
		{
			name:  "scope new flags new files without test",
			files: []string{srcWithoutTest},
			config: SRPConfig{
				EnabledRules: []string{"testRequired"},
				TestRequired: TestRequiredProfiles{{
					Scope:      "new",
					Extensions: []string{".tsx"},
				}},
			},
			newFiles:  map[string]bool{srcWithoutTest: true},
			wantCount: 1,
		},
		{
			name:  "warnOnly makes violations warnings",
			files: []string{srcWithoutTest},
			config: SRPConfig{
				EnabledRules: []string{"testRequired"},
				WarnOnly:     []string{"testRequired"},
				TestRequired: TestRequiredProfiles{{
					Scope:      "all",
					Extensions: []string{".tsx"},
				}},
			},
			wantCount: 1,
		},
		{
			name:  "scope changed checks only staged files",
			files: []string{srcWithoutTest},
			config: SRPConfig{
				EnabledRules: []string{"testRequired"},
				TestRequired: TestRequiredProfiles{{
					Scope:      "changed",
					Extensions: []string{".tsx"},
				}},
			},
			changedFiles: map[string]bool{srcWithoutTest: true},
			wantCount:    1,
		},
		{
			name:  "scope changed skips non-staged files (fullSRPOnCommit scenario)",
			files: []string{srcWithoutTest, srcWithTest}, // full file list
			config: SRPConfig{
				EnabledRules: []string{"testRequired"},
				TestRequired: TestRequiredProfiles{{
					Scope:      "changed",
					Extensions: []string{".tsx"},
				}},
			},
			changedFiles: map[string]bool{srcWithTest: true}, // only srcWithTest is staged
			wantCount:    0,                                  // srcWithTest has a test, srcWithoutTest is not staged
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checker := &SRPChecker{
				config:       tt.config,
				statFunc:     os.Stat,
				newFiles:     tt.newFiles,
				changedFiles: tt.changedFiles,
			}
			violations := checker.checkTestRequired(tt.files)
			if len(violations) != tt.wantCount {
				t.Errorf("got %d violations, want %d: %+v", len(violations), tt.wantCount, violations)
			}
			// Verify warnOnly severity
			if tt.name == "warnOnly makes violations warnings" && len(violations) > 0 {
				if violations[0].Severity != "warning" {
					t.Errorf("got severity %q, want %q", violations[0].Severity, "warning")
				}
			}
		})
	}
}

func TestResolvedTestRequired(t *testing.T) {
	tests := []struct {
		name      string
		config    SRPConfig
		wantCount int
		want      TestRequiredConfig // checked against first element
	}{
		{
			name:      "nil config returns single default profile",
			config:    SRPConfig{},
			wantCount: 1,
			want: TestRequiredConfig{
				Scope:        "new",
				Extensions:   []string{".tsx"},
				TestPatterns: []string{".test.tsx", ".test.ts", ".spec.tsx", ".spec.ts"},
			},
		},
		{
			name: "single legacy profile merges with defaults",
			config: SRPConfig{
				TestRequired: TestRequiredProfiles{{
					Scope:      "changed",
					Extensions: []string{".tsx", ".ts"},
				}},
			},
			wantCount: 1,
			want: TestRequiredConfig{
				Scope:        "changed",
				Extensions:   []string{".tsx", ".ts"},
				TestPatterns: []string{".test.tsx", ".test.ts", ".spec.tsx", ".spec.ts"},
				Name:         "profile-0",
			},
		},
		{
			name: "multiple profiles each get defaults",
			config: SRPConfig{
				TestRequired: TestRequiredProfiles{
					{Name: "unit", Scope: "all", Extensions: []string{".tsx", ".ts"}},
					{Name: "e2e", Extensions: []string{".tsx"}, TestPatterns: []string{".yaml"}, TestDirs: []string{"flows/"}},
				},
			},
			wantCount: 2,
			want: TestRequiredConfig{
				Name:         "unit",
				Scope:        "all",
				Extensions:   []string{".tsx", ".ts"},
				TestPatterns: []string{".test.tsx", ".test.ts", ".spec.tsx", ".spec.ts"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.config.resolvedTestRequired()
			if len(got) != tt.wantCount {
				t.Fatalf("got %d profiles, want %d", len(got), tt.wantCount)
			}
			first := got[0]
			if first.Scope != tt.want.Scope {
				t.Errorf("Scope = %q, want %q", first.Scope, tt.want.Scope)
			}
			if len(first.Extensions) != len(tt.want.Extensions) {
				t.Errorf("Extensions = %v, want %v", first.Extensions, tt.want.Extensions)
			}
			if len(first.TestPatterns) != len(tt.want.TestPatterns) {
				t.Errorf("TestPatterns = %v, want %v", first.TestPatterns, tt.want.TestPatterns)
			}
			if tt.want.Name != "" && first.Name != tt.want.Name {
				t.Errorf("Name = %q, want %q", first.Name, tt.want.Name)
			}
		})
	}
}

func TestTestRequiredProfiles_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantCount int
		wantName  string // first profile name (if set)
	}{
		{
			name:      "null returns empty",
			input:     `{"testRequired": null}`,
			wantCount: 0,
		},
		{
			name:      "object unmarshals as single-element array",
			input:     `{"testRequired": {"name": "unit", "scope": "all"}}`,
			wantCount: 1,
			wantName:  "unit",
		},
		{
			name:      "array unmarshals as array",
			input:     `{"testRequired": [{"name": "unit"}, {"name": "e2e"}]}`,
			wantCount: 2,
			wantName:  "unit",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var cfg SRPConfig
			if err := json.Unmarshal([]byte(tt.input), &cfg); err != nil {
				t.Fatalf("unmarshal error: %v", err)
			}
			if len(cfg.TestRequired) != tt.wantCount {
				t.Errorf("got %d profiles, want %d", len(cfg.TestRequired), tt.wantCount)
			}
			if tt.wantName != "" && len(cfg.TestRequired) > 0 {
				if cfg.TestRequired[0].Name != tt.wantName {
					t.Errorf("first profile name = %q, want %q", cfg.TestRequired[0].Name, tt.wantName)
				}
			}
		})
	}
}

func TestCheckTestRequired_TestDirs(t *testing.T) {
	tmpDir := t.TempDir()

	// Source file without co-located test
	srcFile := filepath.Join(tmpDir, "src", "app", "search-results.tsx")
	os.MkdirAll(filepath.Join(tmpDir, "src", "app"), 0755)
	os.WriteFile(srcFile, []byte("export default function SearchResults() {}"), 0644)

	// Test file in a testDir (basename match)
	maestroDir := filepath.Join(tmpDir, ".maestro", "flows")
	os.MkdirAll(maestroDir, 0755)
	os.WriteFile(filepath.Join(maestroDir, "search-results.yaml"), []byte("appId: com.test"), 0644)

	// Second testDir (empty — should still be searched)
	altDir := filepath.Join(tmpDir, "e2e")
	os.MkdirAll(altDir, 0755)

	tests := []struct {
		name      string
		testDirs  []string
		wantCount int
	}{
		{
			name:      "found in testDir passes",
			testDirs:  []string{maestroDir},
			wantCount: 0,
		},
		{
			name:      "not found in testDir produces violation",
			testDirs:  []string{altDir},
			wantCount: 1,
		},
		{
			name:      "multiple testDirs searched — found in second",
			testDirs:  []string{altDir, maestroDir},
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checker := &SRPChecker{
				config: SRPConfig{
					EnabledRules: []string{"testRequired"},
					TestRequired: TestRequiredProfiles{{
						Name:         "e2e-mobile",
						Scope:        "all",
						Extensions:   []string{".tsx"},
						TestPatterns: []string{".yaml"},
						TestDirs:     tt.testDirs,
					}},
				},
				statFunc: os.Stat,
			}
			violations := checker.checkTestRequired([]string{srcFile})
			if len(violations) != tt.wantCount {
				t.Errorf("got %d violations, want %d: %+v", len(violations), tt.wantCount, violations)
			}
			// Verify violation includes profile name
			if tt.wantCount > 0 && len(violations) > 0 {
				if !strings.Contains(violations[0].Message, "e2e-mobile") {
					t.Errorf("violation message %q should contain profile name", violations[0].Message)
				}
			}
			// Verify suggestion uses testDir path
			if tt.wantCount > 0 && len(violations) > 0 {
				if !strings.Contains(violations[0].Suggestion, tt.testDirs[0]) {
					t.Errorf("suggestion %q should reference testDir %q", violations[0].Suggestion, tt.testDirs[0])
				}
			}
		})
	}
}

func TestCheckTestRequired_PerProfileSeverity(t *testing.T) {
	tmpDir := t.TempDir()
	srcFile := filepath.Join(tmpDir, "Foo.tsx")
	os.WriteFile(srcFile, []byte("export function Foo() {}"), 0644)

	tests := []struct {
		name         string
		severity     string
		warnOnly     []string
		wantSeverity string
	}{
		{
			name:         "default severity is error",
			severity:     "",
			wantSeverity: "error",
		},
		{
			name:         "profile severity warning",
			severity:     "warning",
			wantSeverity: "warning",
		},
		{
			name:         "profile severity error",
			severity:     "error",
			wantSeverity: "error",
		},
		{
			name:         "global warnOnly overrides profile error to warning",
			severity:     "error",
			warnOnly:     []string{"testRequired"},
			wantSeverity: "warning",
		},
		{
			name:         "global warnOnly overrides default to warning",
			severity:     "",
			warnOnly:     []string{"testRequired"},
			wantSeverity: "warning",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checker := &SRPChecker{
				config: SRPConfig{
					EnabledRules: []string{"testRequired"},
					WarnOnly:     tt.warnOnly,
					TestRequired: TestRequiredProfiles{{
						Scope:      "all",
						Extensions: []string{".tsx"},
						Severity:   tt.severity,
					}},
				},
				statFunc: os.Stat,
			}
			violations := checker.checkTestRequired([]string{srcFile})
			if len(violations) == 0 {
				t.Fatal("expected at least 1 violation")
			}
			if violations[0].Severity != tt.wantSeverity {
				t.Errorf("got severity %q, want %q", violations[0].Severity, tt.wantSeverity)
			}
		})
	}
}

func TestCheckTestRequired_MultipleProfiles(t *testing.T) {
	tmpDir := t.TempDir()

	// Source file with a unit test but no e2e test
	srcFile := filepath.Join(tmpDir, "src", "Button.tsx")
	os.MkdirAll(filepath.Join(tmpDir, "src"), 0755)
	os.WriteFile(srcFile, []byte("export function Button() {}"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "src", "Button.test.tsx"), []byte("test('Button')"), 0644)

	e2eDir := filepath.Join(tmpDir, "flows")
	os.MkdirAll(e2eDir, 0755)
	// No Button.yaml in flows/

	checker := &SRPChecker{
		config: SRPConfig{
			EnabledRules: []string{"testRequired"},
			TestRequired: TestRequiredProfiles{
				{
					Name:       "unit",
					Scope:      "all",
					Extensions: []string{".tsx"},
				},
				{
					Name:         "e2e",
					Scope:        "all",
					Extensions:   []string{".tsx"},
					TestPatterns: []string{".yaml"},
					TestDirs:     []string{e2eDir},
					Severity:     "warning",
				},
			},
		},
		statFunc: os.Stat,
	}

	violations := checker.checkTestRequired([]string{srcFile})
	// Unit profile should pass (has Button.test.tsx), e2e should fail (no Button.yaml in flows/)
	if len(violations) != 1 {
		t.Fatalf("got %d violations, want 1: %+v", len(violations), violations)
	}
	if !strings.Contains(violations[0].Message, "e2e") {
		t.Errorf("violation should be from e2e profile, got: %s", violations[0].Message)
	}
	if violations[0].Severity != "warning" {
		t.Errorf("e2e violation should be warning, got: %s", violations[0].Severity)
	}
}

func TestHasTestFile_TestDirs(t *testing.T) {
	tmpDir := t.TempDir()

	// Source file
	srcFile := filepath.Join(tmpDir, "src", "profile.tsx")
	os.MkdirAll(filepath.Join(tmpDir, "src"), 0755)
	os.WriteFile(srcFile, []byte("export function Profile() {}"), 0644)

	// Test in testDir
	testDir := filepath.Join(tmpDir, "e2e")
	os.MkdirAll(testDir, 0755)
	os.WriteFile(filepath.Join(testDir, "profile.spec.ts"), []byte("test('profile')"), 0644)

	// Co-located test (separate case)
	colocatedSrc := filepath.Join(tmpDir, "src", "settings.tsx")
	os.WriteFile(colocatedSrc, []byte("export function Settings() {}"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "src", "settings.test.tsx"), []byte("test('settings')"), 0644)

	checker := &SRPChecker{statFunc: os.Stat}

	cfg := TestRequiredConfig{
		Extensions:   []string{".tsx"},
		TestPatterns: []string{".spec.ts", ".test.tsx"},
		TestDirs:     []string{testDir},
	}

	// profile.tsx should be found via testDir (profile.spec.ts)
	if !checker.hasTestFile(srcFile, cfg) {
		t.Error("expected hasTestFile=true for profile.tsx (found in testDir)")
	}

	// settings.tsx should be found via co-located (settings.test.tsx)
	if !checker.hasTestFile(colocatedSrc, cfg) {
		t.Error("expected hasTestFile=true for settings.tsx (co-located)")
	}

	// nonexistent.tsx should not be found
	noFile := filepath.Join(tmpDir, "src", "nonexistent.tsx")
	os.WriteFile(noFile, []byte("export function X() {}"), 0644)
	if checker.hasTestFile(noFile, cfg) {
		t.Error("expected hasTestFile=false for nonexistent.tsx")
	}
}
