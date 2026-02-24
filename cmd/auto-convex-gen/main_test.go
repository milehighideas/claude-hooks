package main

import (
	"testing"
)

func TestShouldSkip(t *testing.T) {
	config := &Config{}
	config.Skip.Directories = []string{"_generated", "node_modules", ".turbo"}
	config.Skip.Patterns = []string{
		"^_",
		"^debug",
		"^migrate",
		"^seed",
		"Internal\\.ts$",
		"^schema",
		"^convex\\.config",
		"^auth\\.config",
		"^crons\\.ts$",
		"^http\\.ts$",
		"^migrations\\.ts$",
		"\\.test\\.",
		"\\.spec\\.",
	}

	tests := []struct {
		name    string
		relPath string
		skip    bool
	}{
		// Should process
		{"regular function file", "projects.ts", false},
		{"nested function file", "model/issues/queries.ts", false},
		{"mutations file", "supportRequests.ts", false},
		{"deeply nested", "events/voting/queries.ts", false},

		// Should skip — directories
		{"_generated dir", "_generated/api.ts", true},
		{"node_modules", "node_modules/foo.ts", true},

		// Should skip — patterns
		{"underscore prefix", "_helpers.ts", true},
		{"debug file", "debugFoo.ts", true},
		{"migrate file", "migrateData.ts", true},
		{"seed file", "seedDatabase.ts", true},
		{"Internal suffix", "projectsInternal.ts", true},
		{"schema file", "schema.ts", true},
		{"schema enums", "schema.enums.ts", true},
		{"convex config", "convex.config.ts", true},
		{"auth config", "auth.config.ts", true},
		{"crons", "crons.ts", true},
		{"http", "http.ts", true},
		{"migrations", "migrations.ts", true},
		{"test file", "projects.test.ts", true},
		{"spec file", "projects.spec.ts", true},

		// Should skip — non-ts files
		{"json file", "package.json", true},
		{"js file", "helper.js", true},
		{"d.ts file", "types.d.ts", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := shouldSkip(tt.relPath, config)
			if result != tt.skip {
				t.Errorf("shouldSkip(%q) = %v, want %v", tt.relPath, result, tt.skip)
			}
		})
	}
}

func TestFindProjectRoot(t *testing.T) {
	// findProjectRoot with a non-existent path should return empty.
	result := findProjectRoot("/nonexistent/path/to/file.ts")
	if result != "" {
		t.Errorf("expected empty string for non-existent path, got %q", result)
	}
}
