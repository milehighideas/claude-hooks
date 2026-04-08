package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestBuildValidatorCache_TrailingSemicolon is a regression test for the bug where
// validators ending with the conventional TypeScript trailing semicolon (`});`) were
// silently dropped from the cache, causing query hooks that referenced them to be
// generated with empty argument lists.
//
// Symptom before the fix: a query like
//
//	export const getX = query({ args: SomeModule.someValidator, handler: ... })
//
// would parse with empty fn.Args, and the generated hook would have signature
// `(shouldSkip?: boolean)` instead of `(someTypedArg: string, shouldSkip?: boolean)`.
//
// Root cause: validatorDefRe in BuildValidatorCache had `\s*$` as its trailing
// anchor, which only matched validators that did NOT end with a semicolon. The
// fix loosens this to `\s*;?\s*$`.
func TestBuildValidatorCache_TrailingSemicolon(t *testing.T) {
	tmpDir := t.TempDir()
	modelDir := filepath.Join(tmpDir, "model", "crew_member_exclusions")
	if err := os.MkdirAll(modelDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Real camcoapp-style validator file content with trailing semicolons.
	content := `import { v } from 'convex/values';

export const getExclusionValidator = v.object({
  id: v.id('crew_member_exclusions'),
});

export const getExclusionsByScheduleItemValidator = v.object({
  scheduleItemId: v.string(),
});

export const getExclusionsByUserValidator = v.object({
  userId: v.string(),
});
`
	if err := os.WriteFile(filepath.Join(modelDir, "validators.ts"), []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	parser := NewParser(&Config{})
	if err := parser.BuildValidatorCache(tmpDir); err != nil {
		t.Fatalf("BuildValidatorCache: %v", err)
	}

	// Each validator should be cacheable by both its short name and its
	// qualified `Namespace.name` form (namespace derived from the parent dir).
	wantShortNames := []string{
		"getExclusionValidator",
		"getExclusionsByScheduleItemValidator",
		"getExclusionsByUserValidator",
	}
	for _, name := range wantShortNames {
		if _, ok := parser.validatorCache[name]; !ok {
			t.Errorf("validator %q missing from cache (short name lookup)", name)
		}
	}

	wantQualifiedNames := []string{
		"Crew_member_exclusions.getExclusionValidator",
		"Crew_member_exclusions.getExclusionsByScheduleItemValidator",
		"Crew_member_exclusions.getExclusionsByUserValidator",
	}
	for _, name := range wantQualifiedNames {
		if _, ok := parser.validatorCache[name]; !ok {
			t.Errorf("validator %q missing from cache (qualified name lookup)", name)
		}
	}
}

// TestBuildValidatorCache_NoSemicolon ensures the fix doesn't regress projects
// that write validators without trailing semicolons (e.g., upc-me style).
func TestBuildValidatorCache_NoSemicolon(t *testing.T) {
	tmpDir := t.TempDir()
	modelDir := filepath.Join(tmpDir, "model", "issues")
	if err := os.MkdirAll(modelDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	content := `import { v } from 'convex/values'

export const getIssueValidator = v.object({
  id: v.id('issues')
})

export const listIssuesValidator = v.object({
  limit: v.optional(v.number())
})
`
	if err := os.WriteFile(filepath.Join(modelDir, "validators.ts"), []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	parser := NewParser(&Config{})
	if err := parser.BuildValidatorCache(tmpDir); err != nil {
		t.Fatalf("BuildValidatorCache: %v", err)
	}

	for _, name := range []string{"getIssueValidator", "listIssuesValidator"} {
		if _, ok := parser.validatorCache[name]; !ok {
			t.Errorf("validator %q missing from cache", name)
		}
	}
}

// TestBuildValidatorCache_PlainObjectLiteral ensures the second alternation
// branch (plain `export const X = { ... }`) still works for both styles.
func TestBuildValidatorCache_PlainObjectLiteral(t *testing.T) {
	tmpDir := t.TempDir()
	modelDir := filepath.Join(tmpDir, "model", "things")
	if err := os.MkdirAll(modelDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	content := `import { v } from 'convex/values';

export const withSemi = {
  name: v.string(),
};

export const withoutSemi = {
  count: v.number()
}
`
	if err := os.WriteFile(filepath.Join(modelDir, "validators.ts"), []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	parser := NewParser(&Config{})
	if err := parser.BuildValidatorCache(tmpDir); err != nil {
		t.Fatalf("BuildValidatorCache: %v", err)
	}

	for _, name := range []string{"withSemi", "withoutSemi"} {
		if _, ok := parser.validatorCache[name]; !ok {
			t.Errorf("validator %q missing from cache", name)
		}
	}
}
