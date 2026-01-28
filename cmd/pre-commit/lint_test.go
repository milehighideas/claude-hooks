package main

import (
	"testing"
)

func TestFilterLintableFiles(t *testing.T) {
	tests := []struct {
		name     string
		files    []string
		expected []string
	}{
		{
			name:     "empty file list",
			files:    []string{},
			expected: nil,
		},
		{
			name:     "nil file list",
			files:    nil,
			expected: nil,
		},
		{
			name:     "only TypeScript files",
			files:    []string{"app.ts", "component.tsx"},
			expected: []string{"app.ts", "component.tsx"},
		},
		{
			name:     "only JavaScript files",
			files:    []string{"utils.js", "config.jsx"},
			expected: []string{"utils.js", "config.jsx"},
		},
		{
			name:     "mixed lintable and non-lintable files",
			files:    []string{"app.ts", "readme.md", "style.css", "component.tsx", "data.json"},
			expected: []string{"app.ts", "component.tsx"},
		},
		{
			name:     "no lintable files",
			files:    []string{"readme.md", "style.css", "data.json", "image.png"},
			expected: nil,
		},
		{
			name:     "all supported extensions",
			files:    []string{"a.ts", "b.tsx", "c.js", "d.jsx"},
			expected: []string{"a.ts", "b.tsx", "c.js", "d.jsx"},
		},
		{
			name:     "files with paths",
			files:    []string{"src/app.ts", "components/Button.tsx", "lib/utils.js", "config/settings.jsx"},
			expected: []string{"src/app.ts", "components/Button.tsx", "lib/utils.js", "config/settings.jsx"},
		},
		{
			name:     "case sensitivity check",
			files:    []string{"App.TS", "Component.TSX", "util.JS", "config.JSX"},
			expected: []string{"App.TS", "Component.TSX", "util.JS", "config.JSX"},
		},
		{
			name:     "similar but invalid extensions",
			files:    []string{"file.ts.bak", "file.tsx.old", "file.typescript", "file.tss"},
			expected: nil,
		},
		{
			name:     "dotfiles and hidden files",
			files:    []string{".eslintrc.js", ".prettierrc.ts"},
			expected: []string{".eslintrc.js", ".prettierrc.ts"},
		},
		{
			name:     "deeply nested paths",
			files:    []string{"apps/web/src/components/ui/Button.tsx", "packages/shared/lib/utils.ts"},
			expected: []string{"apps/web/src/components/ui/Button.tsx", "packages/shared/lib/utils.ts"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := filterLintableFiles(tt.files)

			if len(result) != len(tt.expected) {
				t.Errorf("filterLintableFiles() returned %d files, expected %d", len(result), len(tt.expected))
				t.Errorf("  got:      %v", result)
				t.Errorf("  expected: %v", tt.expected)
				return
			}

			for i := range result {
				if result[i] != tt.expected[i] {
					t.Errorf("filterLintableFiles()[%d] = %q, expected %q", i, result[i], tt.expected[i])
				}
			}
		})
	}
}
