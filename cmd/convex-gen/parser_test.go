package main

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
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

// fieldNames returns the names of fields in declaration order — small helper for
// the extractAllTableFields tests below.
func fieldNames(fields []FieldInfo) []string {
	out := make([]string, len(fields))
	for i, f := range fields {
		out[i] = f.Name
	}
	return out
}

// TestExtractAllTableFields_InlineLiteral covers the original call shape:
//
//	const events = defineTable({ field: v.string(), ... }).index(...)
//
// The parser must still pick up the inline fields and must NOT mistake the
// searchIndex's `{ searchField, filterFields }` object for the table body.
func TestExtractAllTableFields_InlineLiteral(t *testing.T) {
	text := `
const events = defineTable({
  communityId: v.id("communities"),
  title: v.string(),
  isPublic: v.boolean(),
})
  .index("by_community", ["communityId"])
  .searchIndex("search_events_by_title", {
    searchField: "title",
    filterFields: ["status", "isPublic"],
  });
`
	p := NewParser(&Config{})
	got := p.extractAllTableFields(text)
	if _, ok := got["events"]; !ok {
		t.Fatalf("events table missing from result")
	}
	want := []string{"communityId", "title", "isPublic"}
	if !reflect.DeepEqual(fieldNames(got["events"]), want) {
		t.Errorf("inline fields: got %v, want %v", fieldNames(got["events"]), want)
	}
}

// TestExtractAllTableFields_ValidatorFieldsIndirection is a regression test for the
// dashtag bug where switching to the schema-derived returns-validator pattern broke
// data-layer codegen.
//
// Symptom before the fix: with
//
//	const eventsValidator = v.object({...});
//	const events = defineTable(eventsValidator.fields).searchIndex(...);
//
// the parser scanned for the first `{` after `defineTable(`, which lands inside the
// chained `.searchIndex({ searchField, filterFields })` call. The generated schema
// metadata then had only `searchField` + `filterFields` for the events table.
//
// After the fix the parser resolves `eventsValidator.fields` back to the named
// `v.object({...})` declaration in the same file and uses its fields.
func TestExtractAllTableFields_ValidatorFieldsIndirection(t *testing.T) {
	text := `
export const eventsValidator = v.object({
  communityId: v.id("communities"),
  title: v.string(),
  isPublic: v.boolean(),
});

const events = defineTable(eventsValidator.fields)
  .index("by_community", ["communityId"])
  .searchIndex("search_events_by_title", {
    searchField: "title",
    filterFields: ["status", "isPublic"],
  });
`
	p := NewParser(&Config{})
	got := p.extractAllTableFields(text)
	if _, ok := got["events"]; !ok {
		t.Fatalf("events table missing from result")
	}
	want := []string{"communityId", "title", "isPublic"}
	if !reflect.DeepEqual(fieldNames(got["events"]), want) {
		t.Errorf(".fields indirection: got %v, want %v", fieldNames(got["events"]), want)
	}
}

// TestExtractAllTableFields_ValidatorPassedDirectly covers the convex 1.13+ form
// where the v.object is passed straight to defineTable (no `.fields` access).
func TestExtractAllTableFields_ValidatorPassedDirectly(t *testing.T) {
	text := `
const usersValidator = v.object({
  username: v.string(),
  email: v.string(),
});

const users = defineTable(usersValidator).index("by_username", ["username"]);
`
	p := NewParser(&Config{})
	got := p.extractAllTableFields(text)
	if _, ok := got["users"]; !ok {
		t.Fatalf("users table missing from result")
	}
	want := []string{"username", "email"}
	if !reflect.DeepEqual(fieldNames(got["users"]), want) {
		t.Errorf("direct validator: got %v, want %v", fieldNames(got["users"]), want)
	}
}

// TestExtractAllTableFields_MultipleTables_MixedShapes ensures the parser handles a
// file that defines several tables in different shapes side-by-side, which is the
// real layout of dashtag's schema files.
func TestExtractAllTableFields_MultipleTables_MixedShapes(t *testing.T) {
	text := `
export const eventsValidator = v.object({
  communityId: v.id("communities"),
  title: v.string(),
});

const events = defineTable(eventsValidator.fields).index("by_community", ["communityId"]);

const eventAttendees = defineTable({
  eventId: v.id("events"),
  userId: v.id("users"),
}).index("by_event", ["eventId"]);
`
	p := NewParser(&Config{})
	got := p.extractAllTableFields(text)

	keys := make([]string, 0, len(got))
	for k := range got {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	wantKeys := []string{"eventAttendees", "events"}
	if !reflect.DeepEqual(keys, wantKeys) {
		t.Fatalf("table names: got %v, want %v", keys, wantKeys)
	}
	if got, want := fieldNames(got["events"]), []string{"communityId", "title"}; !reflect.DeepEqual(got, want) {
		t.Errorf("events fields: got %v, want %v", got, want)
	}
	if got, want := fieldNames(got["eventAttendees"]), []string{"eventId", "userId"}; !reflect.DeepEqual(got, want) {
		t.Errorf("eventAttendees fields: got %v, want %v", got, want)
	}
}
