package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGetFilePath(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]interface{}
		expected string
	}{
		{"returns file_path when present", map[string]interface{}{"file_path": "/a/b.ts"}, "/a/b.ts"},
		{"returns empty when missing", map[string]interface{}{}, ""},
		{"returns empty when wrong type", map[string]interface{}{"file_path": 42}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := getFilePath(tt.input); got != tt.expected {
				t.Errorf("getFilePath: got %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestFindProjectRoot(t *testing.T) {
	if got := findProjectRoot("/nonexistent/path/file.ts"); got != "" {
		t.Errorf("expected empty for missing config, got %q", got)
	}

	tmp := t.TempDir()
	nested := filepath.Join(tmp, "apps", "mobile", "components")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmp, configFileName), []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := findProjectRoot(filepath.Join(nested, "Foo.tsx")); got != tmp {
		t.Errorf("findProjectRoot: got %q, want %q", got, tmp)
	}
}

func TestHasExtension(t *testing.T) {
	exts := []string{".ts", ".tsx"}
	tests := []struct {
		path string
		want bool
	}{
		{"apps/mobile/components/Foo.tsx", true},
		{"apps/mobile/src/util.ts", true},
		{"apps/mobile/src/styles.css", false},
		{"apps/mobile/src/locales/en.po", false},
	}
	for _, tt := range tests {
		if got := hasExtension(tt.path, exts); got != tt.want {
			t.Errorf("hasExtension(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

func TestIsSkipped(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"components/Foo.tsx", false},
		{"components/Foo.test.tsx", true},
		{"hooks/useFoo.test.ts", true},
		{"hooks/useFoo.spec.ts", true},
		{"types/foo.d.ts", true},
	}
	for _, tt := range tests {
		if got := isSkipped(tt.path); got != tt.want {
			t.Errorf("isSkipped(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

func TestMatchTarget(t *testing.T) {
	targets := []Target{
		{
			Include: []string{"apps/mobile/src/", "apps/mobile/components/", "apps/mobile/app/"},
			Exclude: []string{"apps/mobile/src/locales/"},
			Command: []string{"bun", "--filter", "@dashtag/mobile", "lingui:extract"},
		},
		{
			Include: []string{"packages/i18n-strings/src/"},
			Exclude: []string{"packages/i18n-strings/src/locales/"},
			Command: []string{"bun", "--filter", "@dashtag/i18n-strings", "lingui:extract"},
		},
	}

	tests := []struct {
		name    string
		relPath string
		wantPkg string // "" means no match
	}{
		{"mobile component matches", "apps/mobile/components/more/MoreMenu.tsx", "@dashtag/mobile"},
		{"mobile src matches", "apps/mobile/src/i18n/index.ts", "@dashtag/mobile"},
		{"mobile locales excluded", "apps/mobile/src/locales/en/messages.po", ""},
		{"i18n-strings src matches", "packages/i18n-strings/src/common.ts", "@dashtag/i18n-strings"},
		{"i18n-strings locales excluded", "packages/i18n-strings/src/locales/en.ts", ""},
		{"unrelated path no match", "apps/story/components/Foo.tsx", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchTarget(tt.relPath, targets)
			if tt.wantPkg == "" {
				if got != nil {
					t.Errorf("matchTarget(%q) = %v, want nil", tt.relPath, got.Command)
				}
				return
			}
			if got == nil {
				t.Fatalf("matchTarget(%q) = nil, want %q", tt.relPath, tt.wantPkg)
			}
			if got.Command[2] != tt.wantPkg {
				t.Errorf("matchTarget(%q) ran %q, want %q", tt.relPath, got.Command[2], tt.wantPkg)
			}
		})
	}
}

func TestFileContainsMarker(t *testing.T) {
	tmp := t.TempDir()
	markers := defaultMacroMarkers

	withMacro := filepath.Join(tmp, "with.tsx")
	if err := os.WriteFile(withMacro, []byte("import { msg } from '@lingui/core/macro';\nconst x = msg`Save`;"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !fileContainsMarker(withMacro, markers) {
		t.Error("expected marker detected in file importing @lingui/core/macro")
	}

	withoutMacro := filepath.Join(tmp, "without.tsx")
	if err := os.WriteFile(withoutMacro, []byte("import { View } from 'react-native';\nexport const Foo = () => null;"), 0o644); err != nil {
		t.Fatal(err)
	}
	if fileContainsMarker(withoutMacro, markers) {
		t.Error("did not expect marker in non-i18n file")
	}

	if fileContainsMarker(filepath.Join(tmp, "missing.tsx"), markers) {
		t.Error("missing file should not report a marker")
	}
}

func TestApplyDefaults(t *testing.T) {
	cfg := &Config{}
	applyDefaults(cfg)
	if len(cfg.MacroMarkers) == 0 || len(cfg.Extensions) == 0 {
		t.Fatal("applyDefaults should populate empty marker/extension lists")
	}

	custom := &Config{MacroMarkers: []string{"x"}, Extensions: []string{".y"}}
	applyDefaults(custom)
	if len(custom.MacroMarkers) != 1 || custom.MacroMarkers[0] != "x" {
		t.Error("applyDefaults should not overwrite provided markers")
	}
}
