package main

import (
	"strings"
	"testing"
)

// authGatedShouldSkipFixture is a minimal flat-file project with one query
// that calls getAuthenticatedUser (auth-gated) and one that doesn't, used to
// exercise the `dataLayer.requireAuthGatedShouldSkip` flag.
func authGatedShouldSkipFixture() fixture {
	return fixture{
		name:          "thingco",
		convexPath:    "packages/convex/convex",
		dataLayerPath: "packages/data-layer/src",
		fileStructure: "grouped",
		functionFiles: map[string]string{
			"things.ts": `import { query } from './_generated/server';
import { v } from 'convex/values';
import { getAuthenticatedUser } from './usersAuth';

export const getMyThing = query({
  args: {},
  handler: async (ctx) => {
    const { user } = await getAuthenticatedUser(ctx);
    return user;
  },
});

export const getPublicThing = query({
  args: {},
  handler: async (ctx) => {
    return null;
  },
});
`,
		},
	}
}

// TestAuthGatedShouldSkip_DisabledIsBackwardsCompatible is the load-bearing
// guarantee: projects whose .convex-gen.json omits `requireAuthGatedShouldSkip`
// (the default, false) must get byte-for-byte the historical output — both
// the auth-gated query and the plain query emit the same optional
// `shouldSkip?: boolean`, regardless of RequiresAuth.
func TestAuthGatedShouldSkip_DisabledIsBackwardsCompatible(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := authGatedShouldSkipFixture().build(t, tmpDir)

	if cfg.DataLayer.RequireAuthGatedShouldSkip {
		t.Fatal("RequireAuthGatedShouldSkip must default to false when absent from config")
	}

	_, fns := runPipeline(t, cfg)
	hooksGen := NewHooksGenerator(cfg)
	content := hooksGen.generateGroupedHookFileContent("things", filterByType(fns, FunctionTypeQuery), "query")

	want := []string{
		"export function useThingsGetMyThing(shouldSkip?: boolean)",
		"export function useThingsGetPublicThing(shouldSkip?: boolean)",
	}
	for _, w := range want {
		if !strings.Contains(content, w) {
			t.Errorf("disabled output missing historical substring %q:\n%s", w, content)
		}
	}
	if strings.Contains(content, "shouldSkip: boolean") {
		t.Errorf("disabled requireAuthGatedShouldSkip must never emit a required shouldSkip param:\n%s", content)
	}
}

// TestAuthGatedShouldSkip_RequiresGuardOnAuthGatedQuery verifies that, with
// the flag on, only the query whose handler calls a configured auth helper
// gets a REQUIRED shouldSkip — the plain query is unaffected.
func TestAuthGatedShouldSkip_RequiresGuardOnAuthGatedQuery(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := authGatedShouldSkipFixture().build(t, tmpDir)
	cfg.DataLayer.RequireAuthGatedShouldSkip = true

	_, fns := runPipeline(t, cfg)
	hooksGen := NewHooksGenerator(cfg)
	content := hooksGen.generateGroupedHookFileContent("things", filterByType(fns, FunctionTypeQuery), "query")

	if !strings.Contains(content, "export function useThingsGetMyThing(shouldSkip: boolean)") {
		t.Errorf("auth-gated query must get a required shouldSkip param:\n%s", content)
	}
	if !strings.Contains(content, "export function useThingsGetPublicThing(shouldSkip?: boolean)") {
		t.Errorf("non-auth query must keep the optional shouldSkip param:\n%s", content)
	}
}

// TestAuthGatedShouldSkip_OptionalArgsStayOptional guards the TS1016 pitfall:
// TypeScript forbids a required param after an optional one, so an
// auth-gated query with an OPTIONAL arg ahead of shouldSkip must keep
// shouldSkip optional too (reordering the signature to make it required
// would be a separate, call-site-breaking change, out of scope here).
func TestAuthGatedShouldSkip_OptionalArgsStayOptional(t *testing.T) {
	tmpDir := t.TempDir()
	f := fixture{
		name:          "thingco",
		convexPath:    "packages/convex/convex",
		dataLayerPath: "packages/data-layer/src",
		fileStructure: "grouped",
		functionFiles: map[string]string{
			"things.ts": `import { query } from './_generated/server';
import { v } from 'convex/values';
import { getAuthenticatedUser } from './usersAuth';

export const getMyFilteredThing = query({
  args: { label: v.optional(v.string()) },
  handler: async (ctx, { label }) => {
    const { user } = await getAuthenticatedUser(ctx);
    return user;
  },
});
`,
		},
	}
	cfg := f.build(t, tmpDir)
	cfg.DataLayer.RequireAuthGatedShouldSkip = true

	_, fns := runPipeline(t, cfg)
	hooksGen := NewHooksGenerator(cfg)
	content := hooksGen.generateGroupedHookFileContent("things", filterByType(fns, FunctionTypeQuery), "query")

	want := "export function useThingsGetMyFilteredThing(label?: string | null, shouldSkip?: boolean)"
	if !strings.Contains(content, want) {
		t.Errorf("auth-gated query with a leading optional arg must keep shouldSkip optional (required-after-optional is invalid TS):\n%s", content)
	}
	if strings.Contains(content, "shouldSkip: boolean") {
		t.Errorf("must not emit a required shouldSkip when an optional arg precedes it:\n%s", content)
	}
}

// TestAuthGatedShouldSkip_CustomAuthHelperNames proves AuthHelperNames is
// actually consulted (not hardcoded) — a project using a differently-named
// auth helper (e.g. requireIdentity) must be able to opt in too.
func TestAuthGatedShouldSkip_CustomAuthHelperNames(t *testing.T) {
	tmpDir := t.TempDir()
	f := fixture{
		name:          "otherco",
		convexPath:    "packages/convex/convex",
		dataLayerPath: "packages/data-layer/src",
		fileStructure: "grouped",
		functionFiles: map[string]string{
			"things.ts": `import { query } from './_generated/server';

export const getMyThing = query({
  args: {},
  handler: async (ctx) => {
    const user = await requireIdentity(ctx);
    return user;
  },
});
`,
		},
	}
	cfg := f.build(t, tmpDir)
	cfg.DataLayer.RequireAuthGatedShouldSkip = true
	cfg.DataLayer.AuthHelperNames = []string{"requireIdentity"}

	_, fns := runPipeline(t, cfg)
	hooksGen := NewHooksGenerator(cfg)
	content := hooksGen.generateGroupedHookFileContent("things", filterByType(fns, FunctionTypeQuery), "query")

	if !strings.Contains(content, "export function useThingsGetMyThing(shouldSkip: boolean)") {
		t.Errorf("custom authHelperNames override must still be detected:\n%s", content)
	}
}
