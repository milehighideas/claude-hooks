package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// pipeline_test.go contains end-to-end tests that exercise the full
// convex-gen pipeline (config → scan → parse → generate) against fixture
// directory trees that mimic real project conventions.
//
// Two project styles are covered, matching the two real consumers of this
// tool — camcoapp (`fileStructure: "grouped"`, validators end with `;`) and
// upc-me (`fileStructure: "split"`, validators end without `;`). These tests
// guard against regressions where a parser change breaks one project's style
// while leaving the other working.

// fixture describes a virtual project tree to materialize on disk for testing.
type fixture struct {
	name           string
	convexPath     string                  // relative path under tmp dir, e.g. "packages/convex/convex"
	dataLayerPath  string                  // relative path under tmp dir, e.g. "packages/data-layer/src"
	fileStructure  string                  // "grouped" or "split"
	validatorFiles map[string]string       // map of relative path → file content
	functionFiles  map[string]string       // map of relative path → file content
}

// build materializes the fixture inside tmpDir and returns a fully populated
// Config that points at the fixture's convex/data-layer paths.
func (f fixture) build(t *testing.T, tmpDir string) *Config {
	t.Helper()

	convexAbs := filepath.Join(tmpDir, f.convexPath)
	if err := os.MkdirAll(convexAbs, 0o755); err != nil {
		t.Fatalf("mkdir convex: %v", err)
	}
	dataLayerAbs := filepath.Join(tmpDir, f.dataLayerPath)
	if err := os.MkdirAll(dataLayerAbs, 0o755); err != nil {
		t.Fatalf("mkdir data-layer: %v", err)
	}

	// Materialize validator files
	for relPath, content := range f.validatorFiles {
		fullPath := filepath.Join(convexAbs, relPath)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			t.Fatalf("mkdir validator parent: %v", err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
			t.Fatalf("write validator file %s: %v", relPath, err)
		}
	}

	// Materialize convex function files
	for relPath, content := range f.functionFiles {
		fullPath := filepath.Join(convexAbs, relPath)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			t.Fatalf("mkdir function parent: %v", err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
			t.Fatalf("write function file %s: %v", relPath, err)
		}
	}

	// Build a Config that satisfies validateConfig (the convex path must exist).
	cfg := &Config{
		Org: "@" + f.name,
		Convex: ConvexConfig{
			Path:      convexAbs,
			Structure: "flat",
		},
		DataLayer: DataLayerConfig{
			Path:          dataLayerAbs,
			HooksDir:      "generated-hooks",
			APIDir:        "generated-api",
			TypesDir:      "generated-types",
			FileStructure: f.fileStructure,
			HookNaming:    "flat",
		},
		Imports: ImportsConfig{
			Style:     "package",
			API:       "@" + f.name + "/backend/api",
			DataModel: "@" + f.name + "/backend/dataModel",
		},
		Generators: GeneratorsConfig{
			Hooks: true,
			API:   true,
			Types: true,
		},
	}
	applyConfigDefaults(cfg)
	return cfg
}

// runPipeline runs the parser-side of convex-gen against the fixture and
// returns the parsed functions. It does NOT actually write generated files
// to disk — that's tested separately via generateGroupedHookFileContent calls
// in the assertion helpers below.
func runPipeline(t *testing.T, cfg *Config) (*Parser, []ConvexFunction) {
	t.Helper()

	parser := NewParser(cfg)
	if err := parser.BuildValidatorCache(cfg.Convex.Path); err != nil {
		t.Fatalf("BuildValidatorCache: %v", err)
	}

	scanner, err := NewScanner(cfg)
	if err != nil {
		t.Fatalf("NewScanner: %v", err)
	}

	files, err := scanner.ScanConvexDirectory()
	if err != nil {
		t.Fatalf("ScanConvexDirectory: %v", err)
	}

	var allFunctions []ConvexFunction
	for _, file := range files {
		fns, err := parser.ParseConvexFile(file)
		if err != nil {
			t.Fatalf("ParseConvexFile %s: %v", file.Path, err)
		}
		allFunctions = append(allFunctions, fns...)
	}
	return parser, allFunctions
}

// findFunc returns the parsed ConvexFunction with the given name, or fails the test.
func findFunc(t *testing.T, fns []ConvexFunction, name string) ConvexFunction {
	t.Helper()
	for _, fn := range fns {
		if fn.Name == name {
			return fn
		}
	}
	t.Fatalf("function %q not found in parsed output (have: %v)", name, funcNames(fns))
	return ConvexFunction{}
}

func funcNames(fns []ConvexFunction) []string {
	names := make([]string, len(fns))
	for i, fn := range fns {
		names[i] = fn.Name
	}
	return names
}

// assertArgNames fails the test if fn.Args doesn't contain exactly the given
// arg names (in any order).
func assertArgNames(t *testing.T, fn ConvexFunction, want ...string) {
	t.Helper()
	got := make([]string, len(fn.Args))
	for i, a := range fn.Args {
		got[i] = a.Name
	}
	if len(got) != len(want) {
		t.Errorf("function %q: got args %v, want %v", fn.Name, got, want)
		return
	}
	wantSet := make(map[string]bool, len(want))
	for _, n := range want {
		wantSet[n] = true
	}
	for _, n := range got {
		if !wantSet[n] {
			t.Errorf("function %q: unexpected arg %q (got %v, want %v)", fn.Name, n, got, want)
		}
	}
}

// TestPipeline_CamcoStyle exercises the full parser pipeline against a
// fixture that mimics camcoapp's conventions:
//   - Validators in model/X/validators.ts using `v.object({...});` (trailing ;)
//   - Convex functions in flat top-level files using `args: ImportedValidator`
//   - `fileStructure: "grouped"` data-layer output
//
// This is the regression case that motivated the parser fix — before the fix,
// the trailing semicolon caused validators to be silently dropped from the
// cache, and queries that referenced them parsed with empty fn.Args.
func TestPipeline_CamcoStyle(t *testing.T) {
	tmpDir := t.TempDir()
	fix := fixture{
		name:          "camconow",
		convexPath:    "packages/convex/convex",
		dataLayerPath: "packages/data-layer/src",
		fileStructure: "grouped",
		validatorFiles: map[string]string{
			"model/crew_member_exclusions/validators.ts": `import { v } from 'convex/values';

export const getExclusionValidator = v.object({
  id: v.id('crew_member_exclusions'),
});

export const getExclusionsByScheduleItemValidator = v.object({
  scheduleItemId: v.string(),
});

export const getExclusionsByUserValidator = v.object({
  userId: v.string(),
});
`,
		},
		functionFiles: map[string]string{
			"crewMemberExclusions.ts": `import { query, mutation } from './_generated/server';
import { v } from 'convex/values';
import * as CrewMemberExclusions from './model/crew_member_exclusions/validators';

export const getExclusion = query({
  args: CrewMemberExclusions.getExclusionValidator,
  handler: async (ctx, { id }) => {
    return null;
  },
});

export const getExclusionsByScheduleItem = query({
  args: CrewMemberExclusions.getExclusionsByScheduleItemValidator,
  handler: async (ctx, { scheduleItemId }) => {
    return null;
  },
});

export const getExclusionsByUser = query({
  args: CrewMemberExclusions.getExclusionsByUserValidator,
  handler: async (ctx, { userId }) => {
    return null;
  },
});

export const getActiveExclusions = query({
  args: { limit: v.optional(v.number()) },
  handler: async (ctx, { limit }) => {
    return null;
  },
});
`,
		},
	}

	cfg := fix.build(t, tmpDir)
	parser, fns := runPipeline(t, cfg)

	// Sanity: validator cache picked up all 3 trailing-semicolon validators.
	wantValidators := []string{
		"getExclusionValidator",
		"getExclusionsByScheduleItemValidator",
		"getExclusionsByUserValidator",
	}
	for _, name := range wantValidators {
		if _, ok := parser.validatorCache[name]; !ok {
			t.Errorf("validator %q missing from cache (camco-style with trailing semicolon)", name)
		}
	}

	// Validator-reference queries: args must be extracted, not empty.
	getExclusion := findFunc(t, fns, "getExclusion")
	assertArgNames(t, getExclusion, "id")

	getByScheduleItem := findFunc(t, fns, "getExclusionsByScheduleItem")
	assertArgNames(t, getByScheduleItem, "scheduleItemId")

	getByUser := findFunc(t, fns, "getExclusionsByUser")
	assertArgNames(t, getByUser, "userId")

	// Inline-args query: control case, should also work.
	getActive := findFunc(t, fns, "getActiveExclusions")
	assertArgNames(t, getActive, "limit")

	// End-to-end: generate the actual hook file content and verify the
	// signatures contain the expected typed parameters (not just shouldSkip).
	hooksGen := NewHooksGenerator(cfg)
	queryFns := filterByType(fns, FunctionTypeQuery)
	content := hooksGen.generateGroupedHookFileContent("crewMemberExclusions", queryFns, "query")

	wantSignatures := []string{
		// id: Id<"crew_member_exclusions"> | null | undefined
		`useCrewMemberExclusionsGetExclusion(id: Id<"crew_member_exclusions">`,
		// scheduleItemId: string
		`useCrewMemberExclusionsGetExclusionsByScheduleItem(scheduleItemId: string`,
		// userId: string
		`useCrewMemberExclusionsGetExclusionsByUser(userId: string`,
		// limit?: number | null
		`useCrewMemberExclusionsGetActiveExclusions(limit?: number`,
	}
	for _, want := range wantSignatures {
		if !strings.Contains(content, want) {
			t.Errorf("generated hook file missing expected signature substring %q\n--- content ---\n%s", want, content)
		}
	}

	// And the broken-state signature must NOT appear: a query that should
	// have a typed parameter must not be generated as `(shouldSkip?: boolean)` only.
	brokenSignatures := []string{
		`useCrewMemberExclusionsGetExclusionsByScheduleItem(shouldSkip?: boolean)`,
		`useCrewMemberExclusionsGetExclusionsByUser(shouldSkip?: boolean)`,
	}
	for _, broken := range brokenSignatures {
		if strings.Contains(content, broken) {
			t.Errorf("generated hook file contains broken signature %q (parser regressed?)", broken)
		}
	}
}

// TestPipeline_UpcMeStyle exercises the same pipeline against a fixture that
// mimics upc-me's conventions:
//   - Validators written without trailing semicolons
//   - `fileStructure: "split"` data-layer output
//
// This is the case that worked before the parser fix and must continue to
// work after — the fix loosens the trailing-semicolon anchor, but if anything
// in the refactor accidentally tightens the no-semicolon path, this test
// catches it.
func TestPipeline_UpcMeStyle(t *testing.T) {
	tmpDir := t.TempDir()
	fix := fixture{
		name:          "upcme",
		convexPath:    "packages/backend",
		dataLayerPath: "packages/data-layer/src",
		fileStructure: "split",
		validatorFiles: map[string]string{
			"model/issues/validators.ts": `import { v } from 'convex/values'

export const getIssueValidator = v.object({
  id: v.id('issues')
})

export const listIssuesValidator = v.object({
  limit: v.optional(v.number())
})
`,
		},
		functionFiles: map[string]string{
			"issues.ts": `import { query } from './_generated/server'
import { v } from 'convex/values'
import { getIssueValidator, listIssuesValidator } from './model/issues/validators'

export const getIssue = query({
  args: getIssueValidator,
  handler: async (ctx, { id }) => {
    return null
  }
})

export const listIssues = query({
  args: listIssuesValidator,
  handler: async (ctx, { limit }) => {
    return null
  }
})

export const getInline = query({
  args: { name: v.string() },
  handler: async (ctx, { name }) => {
    return null
  }
})
`,
		},
	}

	cfg := fix.build(t, tmpDir)
	parser, fns := runPipeline(t, cfg)

	// Sanity: cache populated for no-semicolon validators.
	for _, name := range []string{"getIssueValidator", "listIssuesValidator"} {
		if _, ok := parser.validatorCache[name]; !ok {
			t.Errorf("validator %q missing from cache (upc-me-style without trailing semicolon)", name)
		}
	}

	// Args extraction works for both validator-reference queries.
	getIssue := findFunc(t, fns, "getIssue")
	assertArgNames(t, getIssue, "id")

	listIssues := findFunc(t, fns, "listIssues")
	assertArgNames(t, listIssues, "limit")

	getInline := findFunc(t, fns, "getInline")
	assertArgNames(t, getInline, "name")

	// End-to-end: generate hook content and verify signatures.
	hooksGen := NewHooksGenerator(cfg)
	queryFns := filterByType(fns, FunctionTypeQuery)
	content := hooksGen.generateGroupedHookFileContent("issues", queryFns, "query")

	for _, want := range []string{
		`useIssuesGetIssue(id: Id<"issues">`,
		`useIssuesListIssues(limit?: number`,
		`useIssuesGetInline(name: string`,
	} {
		if !strings.Contains(content, want) {
			t.Errorf("generated hook file missing expected signature %q\n--- content ---\n%s", want, content)
		}
	}
}

// TestPipeline_PlainObjectLiteralValidator covers the second alternation
// branch of validatorDefRe — `export const X = { ... };` with no v.object
// wrapper. Convex accepts both forms for `args:`, and the parser must extract
// args correctly from both. This test would have caught the asymmetry where
// the fluent-convex code path had the v.object extractor but was missing the
// plain-literal fallback.
func TestPipeline_PlainObjectLiteralValidator(t *testing.T) {
	tmpDir := t.TempDir()
	fix := fixture{
		name:          "plainstyle",
		convexPath:    "packages/convex/convex",
		dataLayerPath: "packages/data-layer/src",
		fileStructure: "grouped",
		validatorFiles: map[string]string{
			"model/things/validators.ts": `import { v } from 'convex/values';

export const getThingArgs = {
  id: v.id('things'),
};

export const listThingsArgs = {
  limit: v.optional(v.number()),
};
`,
		},
		functionFiles: map[string]string{
			"things.ts": `import { query } from './_generated/server';
import * as ThingValidators from './model/things/validators';

export const getThing = query({
  args: ThingValidators.getThingArgs,
  handler: async (ctx, { id }) => {
    return null;
  },
});

export const listThings = query({
  args: ThingValidators.listThingsArgs,
  handler: async (ctx, { limit }) => {
    return null;
  },
});
`,
		},
	}

	cfg := fix.build(t, tmpDir)
	_, fns := runPipeline(t, cfg)

	getThing := findFunc(t, fns, "getThing")
	assertArgNames(t, getThing, "id")

	listThings := findFunc(t, fns, "listThings")
	assertArgNames(t, listThings, "limit")
}

// filterByType returns only functions of the given type. Helper used by the
// pipeline tests to feed query-only slices to generateGroupedHookFileContent.
func filterByType(fns []ConvexFunction, ft FunctionType) []ConvexFunction {
	var out []ConvexFunction
	for _, fn := range fns {
		if fn.Type == ft {
			out = append(out, fn)
		}
	}
	return out
}
