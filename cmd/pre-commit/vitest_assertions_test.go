package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHasRequireAssertions(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected bool
	}{
		{
			name: "has requireAssertions true",
			content: `
export default defineConfig({
  test: {
    expect: {
      requireAssertions: true,
    },
  },
});`,
			expected: true,
		},
		{
			name: "missing requireAssertions",
			content: `
export default defineConfig({
  test: {
    globals: true,
    environment: 'jsdom',
  },
});`,
			expected: false,
		},
		{
			name: "requireAssertions false",
			content: `
export default defineConfig({
  test: {
    expect: {
      requireAssertions: false,
    },
  },
});`,
			expected: false,
		},
		{
			name: "requireAssertions in comment should not count",
			content: `
export default defineConfig({
  test: {
    // requireAssertions: true,
    globals: true,
  },
});`,
			expected: false,
		},
		{
			name: "requireAssertions with spaces",
			content: `
export default defineConfig({
  test: {
    expect: {
      requireAssertions :  true,
    },
  },
});`,
			expected: true,
		},
		{
			name: "requireAssertions without nested expect object",
			content: `
test: {
  requireAssertions: true,
}`,
			expected: true,
		},
		{
			name: "requireAssertions with glob pattern containing /*",
			content: `
export default defineConfig({
  test: {
    include: ['**/*.test.{ts,tsx}'],
    expect: {
      requireAssertions: true,
    },
  },
});`,
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hasRequireAssertions(tt.content)
			if result != tt.expected {
				t.Errorf("hasRequireAssertions() = %v, want %v", result, tt.expected)
			}
		})
	}
}


func TestVitestAssertionsChecker(t *testing.T) {
	// Create temp directory structure
	tempDir := t.TempDir()

	// Create app with valid config
	validApp := filepath.Join(tempDir, "valid-app")
	os.MkdirAll(validApp, 0755)
	os.WriteFile(filepath.Join(validApp, "vitest.config.ts"), []byte(`
export default defineConfig({
  test: {
    expect: {
      requireAssertions: true,
    },
  },
});
`), 0644)

	// Create app with invalid config
	invalidApp := filepath.Join(tempDir, "invalid-app")
	os.MkdirAll(invalidApp, 0755)
	os.WriteFile(filepath.Join(invalidApp, "vitest.config.ts"), []byte(`
export default defineConfig({
  test: {
    globals: true,
  },
});
`), 0644)

	// Create app without vitest config (should be skipped)
	noConfigApp := filepath.Join(tempDir, "no-config-app")
	os.MkdirAll(noConfigApp, 0755)

	apps := map[string]AppConfig{
		"valid":    {Path: validApp},
		"invalid":  {Path: invalidApp},
		"noConfig": {Path: noConfigApp},
	}

	checker := NewVitestAssertionsChecker(apps)
	violations, err := checker.Check()
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}

	// Should have exactly one violation (invalid-app)
	if len(violations) != 1 {
		t.Errorf("Expected 1 violation, got %d", len(violations))
	}

	if len(violations) > 0 && violations[0].AppName != "invalid" {
		t.Errorf("Expected violation for 'invalid' app, got '%s'", violations[0].AppName)
	}
}
