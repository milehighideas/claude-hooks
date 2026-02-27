package main

import (
	"os"
	"path/filepath"
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
				TestRequired: &TestRequiredConfig{
					Scope:      "all",
					Extensions: []string{".tsx"},
				},
			},
			wantCount: 0,
		},
		{
			name:  "file without test produces violation",
			files: []string{srcWithoutTest},
			config: SRPConfig{
				EnabledRules: []string{"testRequired"},
				TestRequired: &TestRequiredConfig{
					Scope:      "all",
					Extensions: []string{".tsx"},
				},
			},
			wantCount: 1,
		},
		{
			name:  "file with __tests__ dir test passes",
			files: []string{srcWithTestsDir},
			config: SRPConfig{
				EnabledRules: []string{"testRequired"},
				TestRequired: &TestRequiredConfig{
					Scope:      "all",
					Extensions: []string{".tsx"},
				},
			},
			wantCount: 0,
		},
		{
			name:  "excluded filename is skipped",
			files: []string{filepath.Join(tmpDir, "index.tsx")},
			config: SRPConfig{
				EnabledRules: []string{"testRequired"},
				TestRequired: &TestRequiredConfig{
					Scope:        "all",
					Extensions:   []string{".tsx"},
					ExcludeFiles: []string{"index.tsx"},
				},
			},
			wantCount: 0,
		},
		{
			name:  "excluded path is skipped",
			files: []string{filepath.Join(tmpDir, "__mocks__", "Foo.tsx")},
			config: SRPConfig{
				EnabledRules: []string{"testRequired"},
				TestRequired: &TestRequiredConfig{
					Scope:        "all",
					Extensions:   []string{".tsx"},
					ExcludePaths: []string{"__mocks__/"},
				},
			},
			wantCount: 0,
		},
		{
			name:  "outside includePaths is skipped",
			files: []string{srcWithoutTest},
			config: SRPConfig{
				EnabledRules: []string{"testRequired"},
				TestRequired: &TestRequiredConfig{
					Scope:        "all",
					Extensions:   []string{".tsx"},
					IncludePaths: []string{"src/features/"},
				},
			},
			wantCount: 0,
		},
		{
			name:  "scope new skips non-new files",
			files: []string{srcWithoutTest},
			config: SRPConfig{
				EnabledRules: []string{"testRequired"},
				TestRequired: &TestRequiredConfig{
					Scope:      "new",
					Extensions: []string{".tsx"},
				},
			},
			newFiles:  map[string]bool{}, // srcWithoutTest not in newFiles
			wantCount: 0,
		},
		{
			name:  "scope new flags new files without test",
			files: []string{srcWithoutTest},
			config: SRPConfig{
				EnabledRules: []string{"testRequired"},
				TestRequired: &TestRequiredConfig{
					Scope:      "new",
					Extensions: []string{".tsx"},
				},
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
				TestRequired: &TestRequiredConfig{
					Scope:      "all",
					Extensions: []string{".tsx"},
				},
			},
			wantCount: 1,
		},
		{
			name:  "scope changed checks only staged files",
			files: []string{srcWithoutTest},
			config: SRPConfig{
				EnabledRules: []string{"testRequired"},
				TestRequired: &TestRequiredConfig{
					Scope:      "changed",
					Extensions: []string{".tsx"},
				},
			},
			changedFiles: map[string]bool{srcWithoutTest: true},
			wantCount:    1,
		},
		{
			name:  "scope changed skips non-staged files (fullSRPOnCommit scenario)",
			files: []string{srcWithoutTest, srcWithTest}, // full file list
			config: SRPConfig{
				EnabledRules: []string{"testRequired"},
				TestRequired: &TestRequiredConfig{
					Scope:      "changed",
					Extensions: []string{".tsx"},
				},
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
		name   string
		config SRPConfig
		want   TestRequiredConfig
	}{
		{
			name:   "nil config returns defaults",
			config: SRPConfig{},
			want: TestRequiredConfig{
				Scope:        "new",
				Extensions:   []string{".tsx"},
				TestPatterns: []string{".test.tsx", ".test.ts", ".spec.tsx", ".spec.ts"},
			},
		},
		{
			name: "partial config merges with defaults",
			config: SRPConfig{
				TestRequired: &TestRequiredConfig{
					Scope:      "changed",
					Extensions: []string{".tsx", ".ts"},
				},
			},
			want: TestRequiredConfig{
				Scope:        "changed",
				Extensions:   []string{".tsx", ".ts"},
				TestPatterns: []string{".test.tsx", ".test.ts", ".spec.tsx", ".spec.ts"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.config.resolvedTestRequired()
			if got.Scope != tt.want.Scope {
				t.Errorf("Scope = %q, want %q", got.Scope, tt.want.Scope)
			}
			if len(got.Extensions) != len(tt.want.Extensions) {
				t.Errorf("Extensions = %v, want %v", got.Extensions, tt.want.Extensions)
			}
			if len(got.TestPatterns) != len(tt.want.TestPatterns) {
				t.Errorf("TestPatterns = %v, want %v", got.TestPatterns, tt.want.TestPatterns)
			}
		})
	}
}
