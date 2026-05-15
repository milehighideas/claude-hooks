package main

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// TestExtractMaestroTestIDs covers every classification branch of
// extractMaestroTestIDs and confirms that literals and patterns are
// separated correctly.
func TestExtractMaestroTestIDs(t *testing.T) {
	tests := []struct {
		name         string
		source       string
		wantLiterals []string // sorted, literal Value only
		wantPatterns []string // sorted, pattern Raw only
	}{
		{
			name:         "plain double-quoted string",
			source:       `<View testID="home-screen" />`,
			wantLiterals: []string{"home-screen"},
		},
		{
			name:         "plain single-quoted string",
			source:       `<View testID='home-screen' />`,
			wantLiterals: []string{"home-screen"},
		},
		{
			name:         "braced quoted string",
			source:       `<View testID={"home-screen"} />`,
			wantLiterals: []string{"home-screen"},
		},
		{
			name:         "simple template without interpolation",
			source:       "<View testID={`home-screen`} />",
			wantLiterals: []string{"home-screen"},
		},
		{
			name:         "template with single interpolation at end",
			source:       "<View testID={`community-card-${id}`} />",
			wantPatterns: []string{"community-card-${id}"},
		},
		{
			name:         "template with interpolation in middle",
			source:       "<View testID={`prefix-${x}-suffix`} />",
			wantPatterns: []string{"prefix-${x}-suffix"},
		},
		{
			name:         "template with interpolation at start",
			source:       "<View testID={`${prefix}-suffix`} />",
			wantPatterns: []string{"${prefix}-suffix"},
		},
		{
			name:         "ternary between two literals",
			source:       `<View testID={active ? "tab-active" : "tab-inactive"} />`,
			wantLiterals: []string{"tab-active", "tab-inactive"},
		},
		{
			name:         "template with logical fallback",
			source:       "<View testID={`card-${id}` || 'card-default'} />",
			wantLiterals: []string{"card-default"},
			wantPatterns: []string{"card-${id}"},
		},
		{
			name:         "bare identifier is unresolved and skipped",
			source:       `<View testID={someVar} />`,
			wantLiterals: nil,
			wantPatterns: nil,
		},
		{
			name: "multiple testIDs on different lines",
			source: `
				<View testID="one">
					<View testID="two" />
					<View testID={` + "`" + `three-${x}` + "`" + `} />
				</View>
			`,
			wantLiterals: []string{"one", "two"},
			wantPatterns: []string{"three-${x}"},
		},
		{
			name:         "nested braces inside template expression don't break matching",
			source:       "<View testID={`foo-${obj.nested ?? 'x'}`} />",
			wantPatterns: []string{"foo-${obj.nested ?? 'x'}"},
		},
		{
			name:         "testID as identifier suffix is not matched",
			source:       `<View mytestID="skip-me" testID="keep-me" />`,
			wantLiterals: []string{"keep-me"},
		},
		{
			name:         "testID with escaped quotes",
			source:       `<View testID="has\"quote" />`,
			wantLiterals: []string{`has\"quote`}, // raw capture is fine for matching
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			literals, patterns := extractMaestroTestIDs(tt.source, "test.tsx")

			var gotLits []string
			for _, l := range literals {
				gotLits = append(gotLits, l.Value)
			}
			sort.Strings(gotLits)
			sort.Strings(tt.wantLiterals)
			if !stringSlicesEqual(gotLits, tt.wantLiterals) {
				t.Errorf("literals mismatch\n  got:  %v\n  want: %v", gotLits, tt.wantLiterals)
			}

			var gotPats []string
			for _, p := range patterns {
				gotPats = append(gotPats, p.Raw)
			}
			sort.Strings(gotPats)
			sort.Strings(tt.wantPatterns)
			if !stringSlicesEqual(gotPats, tt.wantPatterns) {
				t.Errorf("patterns mismatch\n  got:  %v\n  want: %v", gotPats, tt.wantPatterns)
			}
		})
	}
}

// TestMaestroTemplateToRegex verifies that template bodies compile into regexes
// that match their own concrete instantiations.
func TestMaestroTemplateToRegex(t *testing.T) {
	tests := []struct {
		template string
		matches  []string
		rejects  []string
	}{
		{
			template: "community-card-${id}",
			matches:  []string{"community-card-abc", "community-card-0", "community-card-a-b"},
			rejects:  []string{"community-card", "other-abc", "community-card-"},
		},
		{
			template: "prefix-${x}-suffix",
			matches:  []string{"prefix-abc-suffix", "prefix-1-suffix"},
			rejects:  []string{"prefix-suffix", "prefix--suffix", "prefix-abc-other"},
		},
		{
			template: "${x}-tail",
			matches:  []string{"head-tail", "1-tail"},
			rejects:  []string{"-tail", "head-notail"},
		},
		{
			template: "literal-only",
			matches:  []string{"literal-only"},
			rejects:  []string{"literal", "literal-only-extra"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.template, func(t *testing.T) {
			rx := maestroTemplateToRegex(tt.template)
			if rx == nil {
				t.Fatalf("got nil regex for %q", tt.template)
			}
			for _, m := range tt.matches {
				if !rx.MatchString(m) {
					t.Errorf("%q should match %q (regex: %s)", tt.template, m, rx.String())
				}
			}
			for _, r := range tt.rejects {
				if rx.MatchString(r) {
					t.Errorf("%q should not match %q (regex: %s)", tt.template, r, rx.String())
				}
			}
		})
	}
}

// TestReadMaestroBalanced covers the brace-balanced walker, which is the most
// subtle part of the extractor. Focuses on cases where naive brace counting
// would fail.
func TestReadMaestroBalanced(t *testing.T) {
	tests := []struct {
		name    string
		content string
		startAt int
		wantOK  bool
		wantOut string
	}{
		{
			name:    "simple braces",
			content: "{foo}",
			startAt: 0,
			wantOK:  true,
			wantOut: "foo",
		},
		{
			name:    "template with interpolation",
			content: "{`foo-${x}`}",
			startAt: 0,
			wantOK:  true,
			wantOut: "`foo-${x}`",
		},
		{
			name:    "nested object literal",
			content: "{{ nested: 1 }}",
			startAt: 0,
			wantOK:  true,
			wantOut: "{ nested: 1 }",
		},
		{
			name:    "string literal with brace inside",
			content: `{"has}brace"}`,
			startAt: 0,
			wantOK:  true,
			wantOut: `"has}brace"`,
		},
		{
			name:    "ternary with template expressions",
			content: "{cond ? `a-${x}` : `b-${y}`}",
			startAt: 0,
			wantOK:  true,
			wantOut: "cond ? `a-${x}` : `b-${y}`",
		},
		{
			name:    "unbalanced returns false",
			content: "{foo",
			startAt: 0,
			wantOK:  false,
		},
		{
			name:    "start not at brace",
			content: "foo",
			startAt: 0,
			wantOK:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _, ok := readMaestroBalanced(tt.content, tt.startAt)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if ok && got != tt.wantOut {
				t.Errorf("got %q, want %q", got, tt.wantOut)
			}
		})
	}
}

// TestResolveMaestroRefs checks that literal and pattern matching both work
// and that unresolved refs are grouped and sorted.
func TestResolveMaestroRefs(t *testing.T) {
	literals := map[string]maestroTestIDLiteral{
		"home-screen":    {Value: "home-screen", File: "a.tsx", Line: 10},
		"profile-button": {Value: "profile-button", File: "b.tsx", Line: 20},
	}
	patterns := []maestroTestIDPattern{
		{Raw: "card-${id}", Regex: maestroTemplateToRegex("card-${id}")},
	}
	refs := []maestroFlowRef{
		{ID: "home-screen", File: "login.yaml", Line: 1},
		{ID: "card-abc", File: "feed.yaml", Line: 2},
		{ID: "card-xyz", File: "feed.yaml", Line: 3},
		{ID: "gone-button", File: "old.yaml", Line: 5},
		{ID: "gone-button", File: "old.yaml", Line: 8}, // duplicate
		{ID: "never-defined", File: "new.yaml", Line: 1},
	}

	unresolved := resolveMaestroRefs(refs, literals, patterns)

	if len(unresolved) != 2 {
		t.Fatalf("got %d unresolved, want 2: %+v", len(unresolved), unresolved)
	}

	// Sorted alphabetically by id
	if unresolved[0].ID != "gone-button" {
		t.Errorf("first unresolved = %q, want gone-button", unresolved[0].ID)
	}
	if unresolved[1].ID != "never-defined" {
		t.Errorf("second unresolved = %q, want never-defined", unresolved[1].ID)
	}
	if len(unresolved[0].References) != 2 {
		t.Errorf("gone-button should have 2 references, got %d", len(unresolved[0].References))
	}
}

// TestLoadMaestroFlowRefs creates a temp directory with sample flow files
// and verifies parsing.
func TestLoadMaestroFlowRefs(t *testing.T) {
	tmp := t.TempDir()

	writeFile(t, filepath.Join(tmp, "login.yaml"), `appId: test
---
- tapOn:
    id: "sign-in-button"
- assertVisible:
    id: home-screen
- assertVisible:
    id: "${ENV_VAR}"
`)
	writeFile(t, filepath.Join(tmp, "nested", "profile.yaml"), `appId: test
---
- tapOn:
    id: 'profile-button'
`)
	// Non-yaml file should be ignored.
	writeFile(t, filepath.Join(tmp, "README.md"), `id: "should-not-match"`)

	refs, err := loadMaestroFlowRefs(tmp)
	if err != nil {
		t.Fatalf("loadMaestroFlowRefs: %v", err)
	}

	wantIDs := []string{"sign-in-button", "home-screen", "profile-button"}
	var gotIDs []string
	for _, r := range refs {
		gotIDs = append(gotIDs, r.ID)
	}
	sort.Strings(gotIDs)
	sort.Strings(wantIDs)
	if !stringSlicesEqual(gotIDs, wantIDs) {
		t.Errorf("flow refs mismatch\n  got:  %v\n  want: %v", gotIDs, wantIDs)
	}
}

// TestRunMaestroValidationEndToEnd sets up a minimal repo structure that
// simulates the real dashtag layout (flows + source with drift) and exercises
// the full runMaestroValidation pipeline.
func TestRunMaestroValidationEndToEnd(t *testing.T) {
	tmp := t.TempDir()

	// Flows directory
	flowsDir := filepath.Join(tmp, "flows")
	writeFile(t, filepath.Join(flowsDir, "login.yaml"), `appId: test
---
- tapOn:
    id: "sign-in-button"
- assertVisible:
    id: "home-screen"
- tapOn:
    id: "stale-button"
`)

	// Source directory with testIDs defined
	srcDir := filepath.Join(tmp, "src")
	writeFile(t, filepath.Join(srcDir, "home.tsx"), `
import { View } from 'react-native';
export default function Home() {
  return <View testID="home-screen" />;
}
`)
	writeFile(t, filepath.Join(srcDir, "signin.tsx"), `
import { Pressable } from 'react-native';
export default function SignIn() {
  return <Pressable testID="sign-in-button" />;
}
`)
	// "stale-button" is intentionally NOT defined in source → drift.

	// Test file should be ignored
	writeFile(t, filepath.Join(srcDir, "home.test.tsx"), `
<View testID="stale-button" />
`)

	// Change to tmp so relative paths resolve cleanly
	origDir, _ := os.Getwd()
	defer func() { _ = os.Chdir(origDir) }()
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	cfg := MaestroValidationConfig{
		FlowsDir:   "flows",
		SourceDirs: []string{"src"},
	}

	err := runMaestroValidation(cfg)
	if err == nil {
		t.Fatal("expected error for stale-button drift, got nil")
	}
	if !strings.Contains(err.Error(), "1 missing") {
		t.Errorf("error should mention 1 missing testID, got: %v", err)
	}
}

// TestRunMaestroValidationNoDrift confirms the happy path.
func TestRunMaestroValidationNoDrift(t *testing.T) {
	tmp := t.TempDir()
	flowsDir := filepath.Join(tmp, "flows")
	writeFile(t, filepath.Join(flowsDir, "smoke.yaml"), `appId: test
---
- assertVisible:
    id: "home-screen"
- assertVisible:
    id: "card-abc"
`)

	srcDir := filepath.Join(tmp, "src")
	writeFile(t, filepath.Join(srcDir, "home.tsx"), `
<View testID="home-screen" />
`)
	writeFile(t, filepath.Join(srcDir, "card.tsx"), "<View testID={`card-${id}`} />")

	origDir, _ := os.Getwd()
	defer func() { _ = os.Chdir(origDir) }()
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	cfg := MaestroValidationConfig{
		FlowsDir:   "flows",
		SourceDirs: []string{"src"},
	}
	if err := runMaestroValidation(cfg); err != nil {
		t.Errorf("expected no drift, got %v", err)
	}
}

// TestRunMaestroValidationSkipsWhenUnconfigured confirms that a missing
// config is a no-op rather than an error — the feature is opt-in.
func TestRunMaestroValidationSkipsWhenUnconfigured(t *testing.T) {
	cases := []MaestroValidationConfig{
		{}, // empty
		{FlowsDir: "x"}, // no source dirs
		{SourceDirs: []string{"y"}}, // no flows dir
	}
	for _, cfg := range cases {
		if err := runMaestroValidation(cfg); err != nil {
			t.Errorf("unconfigured should be no-op, got %v (cfg=%+v)", err, cfg)
		}
	}
}

// writeFile writes content to path, creating parent directories as needed.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// stringSlicesEqual returns true if both slices contain the same elements in
// the same order. Empty and nil are equal.
func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
