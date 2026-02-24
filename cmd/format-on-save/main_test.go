package main

import (
	"testing"
)

func TestShouldFormat(t *testing.T) {
	tests := []struct {
		name     string
		filePath string
		expected bool
	}{
		// Should format
		{"TypeScript", "/project/src/app.ts", true},
		{"TSX", "/project/src/Component.tsx", true},
		{"JavaScript", "/project/src/index.js", true},
		{"JSX", "/project/src/App.jsx", true},
		{"JSON", "/project/package.json", true},
		{"Markdown", "/project/README.md", true},
		{"CSS", "/project/styles.css", true},

		// Should not format
		{"Go file", "/project/main.go", false},
		{"Python file", "/project/script.py", false},
		{"YAML file", "/project/config.yaml", false},
		{"Shell script", "/project/run.sh", false},
		{"Binary", "/project/bin/app", false},
		{"No extension", "/project/Makefile", false},
		{"HTML", "/project/index.html", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := shouldFormat(tt.filePath)
			if result != tt.expected {
				t.Errorf("shouldFormat(%q) = %v, want %v", tt.filePath, result, tt.expected)
			}
		})
	}
}
