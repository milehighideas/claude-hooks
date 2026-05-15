package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// projectFixture describes a temp project layout for tests.
type projectFixture struct {
	// config is the literal contents of the docsTrackerConfig block inside
	// .pre-commit.json. If empty, no .pre-commit.json is created (project is
	// not opted in). Non-empty → fixture writes .pre-commit.json with
	// features.docsTracker=true and docsTrackerConfig set to this literal.
	config string
	// rawPreCommit, if non-empty, overrides config and is written verbatim as
	// .pre-commit.json. Use for tests that need to exercise the top-level
	// schema (feature flag off, missing features block, malformed JSON).
	rawPreCommit string
	// docs creates files (by relative path) with placeholder content.
	docs []string
	// extraFiles creates empty files (by relative path) for layout purposes.
	extraFiles []string
}

// setupProject creates a fixture on disk and returns the project root.
func setupProject(t *testing.T, fx projectFixture) string {
	t.Helper()
	root := t.TempDir()
	if fx.rawPreCommit != "" {
		if err := os.WriteFile(filepath.Join(root, ".pre-commit.json"), []byte(fx.rawPreCommit), 0644); err != nil {
			t.Fatalf("write config: %v", err)
		}
	} else if fx.config != "" {
		wrapped := fmt.Sprintf(`{"features":{"docsTracker":true},"docsTrackerConfig":%s}`, fx.config)
		if err := os.WriteFile(filepath.Join(root, ".pre-commit.json"), []byte(wrapped), 0644); err != nil {
			t.Fatalf("write config: %v", err)
		}
	}
	for _, doc := range fx.docs {
		full := filepath.Join(root, filepath.FromSlash(doc))
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			t.Fatalf("mkdir for doc %s: %v", doc, err)
		}
		if err := os.WriteFile(full, []byte("# docs\n"), 0644); err != nil {
			t.Fatalf("write doc %s: %v", doc, err)
		}
	}
	for _, f := range fx.extraFiles {
		full := filepath.Join(root, filepath.FromSlash(f))
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			t.Fatalf("mkdir for extra %s: %v", f, err)
		}
		if err := os.WriteFile(full, nil, 0644); err != nil {
			t.Fatalf("write extra %s: %v", f, err)
		}
	}
	return root
}

// sessionProvider returns a session file provider rooted at dir.
func sessionProvider(dir string) sessionFileProvider {
	return func(sessionID string) string {
		return filepath.Join(dir, ".claude", "sessions", sessionID+"-docs.json")
	}
}

// seedSession writes docsRead into the session file for sessionID.
func seedSession(t *testing.T, provider sessionFileProvider, sessionID string, docsRead []string) {
	t.Helper()
	sessionFile := provider(sessionID)
	if err := os.MkdirAll(filepath.Dir(sessionFile), 0755); err != nil {
		t.Fatalf("mkdir session dir: %v", err)
	}
	data, _ := json.Marshal(SessionData{DocsRead: docsRead})
	if err := os.WriteFile(sessionFile, data, 0644); err != nil {
		t.Fatalf("write session file: %v", err)
	}
}

// runEnforce runs enforceWithProvider for a given file_path/session.
func runEnforce(t *testing.T, provider sessionFileProvider, sessionID, filePath string) (string, error) {
	t.Helper()
	input := HookInput{
		ToolName:  "Edit",
		ToolInput: map[string]interface{}{"file_path": filePath},
		SessionID: sessionID,
	}
	data, _ := json.Marshal(input)
	var stderr bytes.Buffer
	err := enforceWithProvider(bytes.NewReader(data), &stderr, provider)
	return stderr.String(), err
}

// ---------------------------------------------------------------------------
// Opt-in gate
// ---------------------------------------------------------------------------

func TestEnforce_NoConfig_IsNoOp(t *testing.T) {
	root := setupProject(t, projectFixture{
		// no config = not opted in
		docs: []string{"packages/backend/CLAUDE.md"},
	})
	provider := sessionProvider(t.TempDir())

	stderr, err := runEnforce(t, provider, "s", filepath.Join(root, "packages", "backend", "foo.ts"))
	if err != nil {
		t.Fatalf("expected allow, got %v", err)
	}
	if stderr != "" {
		t.Errorf("expected no stderr, got %q", stderr)
	}
}

func TestEnforce_FeatureFlagOff_IsNoOp(t *testing.T) {
	// .pre-commit.json exists but features.docsTracker is false — must no-op.
	root := setupProject(t, projectFixture{
		rawPreCommit: `{"features":{"docsTracker":false}}`,
		docs:         []string{"packages/backend/CLAUDE.md"},
	})
	provider := sessionProvider(t.TempDir())

	stderr, err := runEnforce(t, provider, "s", filepath.Join(root, "packages", "backend", "foo.ts"))
	if err != nil {
		t.Fatalf("expected allow, got %v", err)
	}
	if stderr != "" {
		t.Errorf("expected no stderr, got %q", stderr)
	}
}

func TestEnforce_FeatureFlagMissing_IsNoOp(t *testing.T) {
	// .pre-commit.json exists but features block is absent — must no-op.
	root := setupProject(t, projectFixture{
		rawPreCommit: `{"packageManager":"pnpm"}`,
		docs:         []string{"packages/backend/CLAUDE.md"},
	})
	provider := sessionProvider(t.TempDir())

	stderr, err := runEnforce(t, provider, "s", filepath.Join(root, "packages", "backend", "foo.ts"))
	if err != nil {
		t.Fatalf("expected allow, got %v", err)
	}
	if stderr != "" {
		t.Errorf("expected no stderr, got %q", stderr)
	}
}

func TestEnforce_MalformedConfig_IsNoOp(t *testing.T) {
	// Broken JSON — fail open.
	root := setupProject(t, projectFixture{
		rawPreCommit: `{"features":{`,
		docs:         []string{"packages/backend/CLAUDE.md"},
	})
	provider := sessionProvider(t.TempDir())

	stderr, err := runEnforce(t, provider, "s", filepath.Join(root, "packages", "backend", "foo.ts"))
	if err != nil {
		t.Fatalf("expected allow, got %v", err)
	}
	if stderr != "" {
		t.Errorf("expected no stderr, got %q", stderr)
	}
}

func TestEnforce_JSONCCommentsAllowed(t *testing.T) {
	// .pre-commit.json is JSONC — // comments must be stripped.
	root := setupProject(t, projectFixture{
		rawPreCommit: `{
  // opt into docs-tracker
  "features": { "docsTracker": true }
}`,
		docs: []string{"packages/backend/CLAUDE.md"},
	})
	provider := sessionProvider(t.TempDir())

	_, err := runEnforce(t, provider, "s", filepath.Join(root, "packages", "backend", "foo.ts"))
	exitErr, ok := err.(*ExitError)
	if !ok || exitErr.Code != 2 {
		t.Fatalf("expected block (exit 2), got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Per-app scope filter
// ---------------------------------------------------------------------------

func TestEnforce_AppPaths_LimitsScope(t *testing.T) {
	// Two apps, both with CLAUDE.md. Only apps/web is in appPaths — editing a
	// file in apps/mobile should no-op even though the doc exists.
	root := setupProject(t, projectFixture{
		config: `{"appPaths":["apps/web"]}`,
		docs: []string{
			"apps/web/CLAUDE.md",
			"apps/mobile/CLAUDE.md",
		},
	})
	provider := sessionProvider(t.TempDir())

	// In-scope: blocked.
	_, err := runEnforce(t, provider, "s", filepath.Join(root, "apps", "web", "foo.ts"))
	exitErr, ok := err.(*ExitError)
	if !ok || exitErr.Code != 2 {
		t.Fatalf("apps/web: expected block, got %v", err)
	}

	// Out-of-scope: allowed.
	stderr, err := runEnforce(t, provider, "s", filepath.Join(root, "apps", "mobile", "foo.ts"))
	if err != nil {
		t.Fatalf("apps/mobile: expected allow, got %v", err)
	}
	if stderr != "" {
		t.Errorf("expected no stderr, got %q", stderr)
	}
}

func TestEnforce_ExcludePaths_Skips(t *testing.T) {
	// apps/legacy is in excludePaths — should not be gated even though the
	// project otherwise enforces docs.
	root := setupProject(t, projectFixture{
		config: `{"excludePaths":["apps/legacy"]}`,
		docs: []string{
			"apps/web/CLAUDE.md",
			"apps/legacy/CLAUDE.md",
		},
	})
	provider := sessionProvider(t.TempDir())

	// Non-excluded: still blocked.
	_, err := runEnforce(t, provider, "s", filepath.Join(root, "apps", "web", "foo.ts"))
	exitErr, ok := err.(*ExitError)
	if !ok || exitErr.Code != 2 {
		t.Fatalf("apps/web: expected block, got %v", err)
	}

	// Excluded: allowed.
	stderr, err := runEnforce(t, provider, "s", filepath.Join(root, "apps", "legacy", "foo.ts"))
	if err != nil {
		t.Fatalf("apps/legacy: expected allow, got %v", err)
	}
	if stderr != "" {
		t.Errorf("expected no stderr, got %q", stderr)
	}
}

func TestEnforce_ExcludePaths_BeatsAppPaths(t *testing.T) {
	// Exclusions must win over inclusions.
	root := setupProject(t, projectFixture{
		config: `{"appPaths":["apps/web"],"excludePaths":["apps/web/legacy"]}`,
		docs:   []string{"apps/web/CLAUDE.md"},
	})
	provider := sessionProvider(t.TempDir())

	_, err := runEnforce(t, provider, "s", filepath.Join(root, "apps", "web", "legacy", "foo.ts"))
	if err != nil {
		t.Fatalf("expected allow (excluded), got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Custom mappings
// ---------------------------------------------------------------------------

func TestEnforce_CustomMapping_GatesOnExternalDoc(t *testing.T) {
	// A doc living outside the gated directory (at repo root) can still be
	// required via an explicit mapping.
	root := setupProject(t, projectFixture{
		config: `{
			"autoDiscover": false,
			"mappings": [
				{ "pattern": "apps/web/", "docs": ["docs/frontend.md"] }
			]
		}`,
		docs: []string{"docs/frontend.md"},
	})
	provider := sessionProvider(t.TempDir())

	stderr, err := runEnforce(t, provider, "s", filepath.Join(root, "apps", "web", "foo.tsx"))
	exitErr, ok := err.(*ExitError)
	if !ok || exitErr.Code != 2 {
		t.Fatalf("expected block, got %v", err)
	}
	if !strings.Contains(stderr, "docs/frontend.md") {
		t.Errorf("expected stderr to mention docs/frontend.md, got %q", stderr)
	}

	// After reading the doc, edits should be allowed.
	seedSession(t, provider, "s", []string{"docs/frontend.md"})
	_, err = runEnforce(t, provider, "s", filepath.Join(root, "apps", "web", "foo.tsx"))
	if err != nil {
		t.Fatalf("expected allow after reading, got %v", err)
	}
}

func TestEnforce_CustomMapping_MergesWithAutoDiscovery(t *testing.T) {
	// When a custom mapping and an auto-discovered CLAUDE.md share a pattern,
	// both docs must be read.
	root := setupProject(t, projectFixture{
		config: `{
			"mappings": [
				{ "pattern": "apps/web/", "docs": ["docs/frontend.md"] }
			]
		}`,
		docs: []string{
			"apps/web/CLAUDE.md",
			"docs/frontend.md",
		},
	})
	provider := sessionProvider(t.TempDir())

	// Reading only one → still blocked.
	seedSession(t, provider, "s", []string{"apps/web/CLAUDE.md"})
	stderr, err := runEnforce(t, provider, "s", filepath.Join(root, "apps", "web", "foo.tsx"))
	exitErr, ok := err.(*ExitError)
	if !ok || exitErr.Code != 2 {
		t.Fatalf("expected block until both read, got %v", err)
	}
	if !strings.Contains(stderr, "docs/frontend.md") {
		t.Errorf("expected unread doc in stderr, got %q", stderr)
	}

	// Reading both → allowed.
	seedSession(t, provider, "s", []string{"apps/web/CLAUDE.md", "docs/frontend.md"})
	_, err = runEnforce(t, provider, "s", filepath.Join(root, "apps", "web", "foo.tsx"))
	if err != nil {
		t.Fatalf("expected allow with both read, got %v", err)
	}
}

func TestEnforce_CustomMapping_MostSpecificWins(t *testing.T) {
	// Deeper custom mapping beats a broader one.
	root := setupProject(t, projectFixture{
		config: `{
			"autoDiscover": false,
			"mappings": [
				{ "pattern": "apps/",       "docs": ["docs/general.md"] },
				{ "pattern": "apps/web/",   "docs": ["docs/web.md"] }
			]
		}`,
		docs: []string{"docs/general.md", "docs/web.md"},
	})
	provider := sessionProvider(t.TempDir())

	stderr, err := runEnforce(t, provider, "s", filepath.Join(root, "apps", "web", "foo.tsx"))
	exitErr, ok := err.(*ExitError)
	if !ok || exitErr.Code != 2 {
		t.Fatalf("expected block, got %v", err)
	}
	if !strings.Contains(stderr, "docs/web.md") {
		t.Errorf("expected web.md mapping to win, got %q", stderr)
	}
	if strings.Contains(stderr, "docs/general.md") {
		t.Errorf("broader apps/ mapping should not apply to apps/web/, got %q", stderr)
	}
}

func TestEnforce_CustomMapping_NormalizesSlashes(t *testing.T) {
	// Accept patterns with or without trailing slash, and docs with leading slash.
	root := setupProject(t, projectFixture{
		config: `{
			"autoDiscover": false,
			"mappings": [
				{ "pattern": "/apps/web", "docs": ["/docs/frontend.md"] }
			]
		}`,
		docs: []string{"docs/frontend.md"},
	})
	provider := sessionProvider(t.TempDir())

	stderr, err := runEnforce(t, provider, "s", filepath.Join(root, "apps", "web", "foo.tsx"))
	exitErr, ok := err.(*ExitError)
	if !ok || exitErr.Code != 2 {
		t.Fatalf("expected block, got %v", err)
	}
	if !strings.Contains(stderr, "docs/frontend.md") {
		t.Errorf("expected normalized doc path in stderr, got %q", stderr)
	}
}

func TestEnforce_CustomMapping_SkipsEmptyEntries(t *testing.T) {
	// A mapping with no pattern or no docs is silently dropped.
	root := setupProject(t, projectFixture{
		config: `{
			"autoDiscover": false,
			"mappings": [
				{ "pattern": "",           "docs": ["docs/frontend.md"] },
				{ "pattern": "apps/web/",  "docs": [] },
				{ "pattern": "apps/mobile/", "docs": ["docs/mobile.md"] }
			]
		}`,
		docs: []string{"docs/frontend.md", "docs/mobile.md"},
	})
	provider := sessionProvider(t.TempDir())

	// apps/web/ had empty docs → not gated.
	_, err := runEnforce(t, provider, "s", filepath.Join(root, "apps", "web", "foo.tsx"))
	if err != nil {
		t.Fatalf("expected allow (dropped mapping), got %v", err)
	}

	// apps/mobile/ is valid → gated.
	_, err = runEnforce(t, provider, "s", filepath.Join(root, "apps", "mobile", "foo.tsx"))
	exitErr, ok := err.(*ExitError)
	if !ok || exitErr.Code != 2 {
		t.Fatalf("expected block on apps/mobile, got %v", err)
	}
}

func TestTrack_RecordsCustomDocRead(t *testing.T) {
	// Reading a doc declared only via a custom mapping must be tracked.
	root := setupProject(t, projectFixture{
		config: `{
			"autoDiscover": false,
			"mappings": [
				{ "pattern": "apps/web/", "docs": ["docs/frontend.md"] }
			]
		}`,
		docs: []string{"docs/frontend.md"},
	})
	provider := sessionProvider(t.TempDir())

	// Simulate Claude reading docs/frontend.md.
	input := HookInput{
		ToolName:  "Read",
		ToolInput: map[string]interface{}{"file_path": filepath.Join(root, "docs", "frontend.md")},
		SessionID: "s",
	}
	data, _ := json.Marshal(input)
	if err := trackWithProvider(bytes.NewReader(data), provider); err != nil {
		t.Fatalf("track: %v", err)
	}

	// Subsequent edit should be allowed.
	_, err := runEnforce(t, provider, "s", filepath.Join(root, "apps", "web", "foo.tsx"))
	if err != nil {
		t.Fatalf("expected allow after track, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// CLAUDE.md auto-discovery (default behavior with empty config)
// ---------------------------------------------------------------------------

func TestEnforce_AutoDiscover_BlocksWhenDocUnread(t *testing.T) {
	root := setupProject(t, projectFixture{
		config: `{}`,
		docs:   []string{"packages/backend/CLAUDE.md"},
	})
	provider := sessionProvider(t.TempDir())

	stderr, err := runEnforce(t, provider, "s", filepath.Join(root, "packages", "backend", "foo.ts"))
	exitErr, ok := err.(*ExitError)
	if !ok || exitErr.Code != 2 {
		t.Fatalf("expected block (exit 2), got %v", err)
	}
	if !strings.Contains(stderr, "packages/backend/CLAUDE.md") {
		t.Errorf("expected CLAUDE.md in message: %s", stderr)
	}
}

func TestEnforce_AutoDiscover_AllowsWhenDocRead(t *testing.T) {
	root := setupProject(t, projectFixture{
		config: `{}`,
		docs:   []string{"packages/backend/CLAUDE.md"},
	})
	provider := sessionProvider(t.TempDir())
	seedSession(t, provider, "s", []string{"packages/backend/CLAUDE.md"})

	_, err := runEnforce(t, provider, "s", filepath.Join(root, "packages", "backend", "foo.ts"))
	if err != nil {
		t.Errorf("expected allow, got %v", err)
	}
}

func TestEnforce_AutoDiscoverOff_IgnoresClaudeMd(t *testing.T) {
	root := setupProject(t, projectFixture{
		config: `{"autoDiscover": false}`,
		docs:   []string{"packages/backend/CLAUDE.md"},
	})
	provider := sessionProvider(t.TempDir())

	_, err := runEnforce(t, provider, "s", filepath.Join(root, "packages", "backend", "foo.ts"))
	if err != nil {
		t.Errorf("expected allow with autoDiscover false, got %v", err)
	}
}

func TestEnforce_CustomDocFileNames(t *testing.T) {
	root := setupProject(t, projectFixture{
		config: `{"docFileNames":["AGENTS.md"]}`,
		docs:   []string{"packages/backend/AGENTS.md"},
	})
	provider := sessionProvider(t.TempDir())

	stderr, err := runEnforce(t, provider, "s", filepath.Join(root, "packages", "backend", "foo.ts"))
	exitErr, ok := err.(*ExitError)
	if !ok || exitErr.Code != 2 {
		t.Fatalf("expected block, got %v", err)
	}
	if !strings.Contains(stderr, "packages/backend/AGENTS.md") {
		t.Errorf("expected AGENTS.md in message: %s", stderr)
	}
}

// ---------------------------------------------------------------------------
// Convex preset
// ---------------------------------------------------------------------------

const convexSkillContent = "# skill\n"

func TestEnforce_ConvexPreset_Blocks(t *testing.T) {
	root := setupProject(t, projectFixture{
		config: `{"convex": true}`,
		docs: []string{
			"packages/backend/convex/_generated/ai/guidelines.md",
			"packages/backend/.agents/skills/convex-quickstart/SKILL.md",
			"packages/backend/.agents/skills/convex-auth/SKILL.md",
		},
		extraFiles: []string{"packages/backend/foo.ts"},
	})
	provider := sessionProvider(t.TempDir())

	stderr, err := runEnforce(t, provider, "s", filepath.Join(root, "packages", "backend", "foo.ts"))
	if _, ok := err.(*ExitError); !ok {
		t.Fatalf("expected block, got %v", err)
	}
	// All three docs should be listed as missing.
	for _, must := range []string{
		"packages/backend/convex/_generated/ai/guidelines.md",
		"packages/backend/.agents/skills/convex-quickstart/SKILL.md",
		"packages/backend/.agents/skills/convex-auth/SKILL.md",
	} {
		if !strings.Contains(stderr, must) {
			t.Errorf("expected %q in message, got:\n%s", must, stderr)
		}
	}
	if !strings.Contains(stderr, "Convex backend") {
		t.Errorf("expected Convex backend label: %s", stderr)
	}
}

func TestEnforce_ConvexPreset_ListsOnlyMissing(t *testing.T) {
	root := setupProject(t, projectFixture{
		config: `{"convex": true}`,
		docs: []string{
			"packages/backend/convex/_generated/ai/guidelines.md",
			"packages/backend/.agents/skills/convex-quickstart/SKILL.md",
			"packages/backend/.agents/skills/convex-auth/SKILL.md",
		},
	})
	provider := sessionProvider(t.TempDir())
	// Guidelines and one skill have been read.
	seedSession(t, provider, "s", []string{
		"packages/backend/convex/_generated/ai/guidelines.md",
		"packages/backend/.agents/skills/convex-quickstart/SKILL.md",
	})

	stderr, err := runEnforce(t, provider, "s", filepath.Join(root, "packages", "backend", "foo.ts"))
	if _, ok := err.(*ExitError); !ok {
		t.Fatalf("expected block (one skill still missing), got %v", err)
	}
	// Should ONLY list the unread skill.
	if !strings.Contains(stderr, "convex-auth/SKILL.md") {
		t.Errorf("expected missing skill listed: %s", stderr)
	}
	if strings.Contains(stderr, "convex-quickstart/SKILL.md") {
		t.Errorf("should not list already-read skill: %s", stderr)
	}
	if strings.Contains(stderr, "guidelines.md") {
		t.Errorf("should not list already-read guidelines: %s", stderr)
	}
}

func TestEnforce_ConvexPreset_AllowsWhenAllDocsRead(t *testing.T) {
	root := setupProject(t, projectFixture{
		config: `{"convex": true}`,
		docs: []string{
			"packages/backend/convex/_generated/ai/guidelines.md",
			"packages/backend/.agents/skills/convex-quickstart/SKILL.md",
		},
	})
	provider := sessionProvider(t.TempDir())
	seedSession(t, provider, "s", []string{
		"packages/backend/convex/_generated/ai/guidelines.md",
		"packages/backend/.agents/skills/convex-quickstart/SKILL.md",
	})

	_, err := runEnforce(t, provider, "s", filepath.Join(root, "packages", "backend", "foo.ts"))
	if err != nil {
		t.Errorf("expected allow, got %v", err)
	}
}

func TestEnforce_ConvexPreset_CustomBackendDir(t *testing.T) {
	root := setupProject(t, projectFixture{
		config: `{"convex": {"backendDir": "apps/backend"}}`,
		docs: []string{
			"apps/backend/convex/_generated/ai/guidelines.md",
			"apps/backend/.agents/skills/convex-quickstart/SKILL.md",
		},
	})
	provider := sessionProvider(t.TempDir())

	stderr, err := runEnforce(t, provider, "s", filepath.Join(root, "apps", "backend", "foo.ts"))
	if _, ok := err.(*ExitError); !ok {
		t.Fatalf("expected block, got %v", err)
	}
	if !strings.Contains(stderr, "apps/backend/convex/_generated/ai/guidelines.md") {
		t.Errorf("expected apps/backend guidelines in message: %s", stderr)
	}
	if !strings.Contains(stderr, "apps/backend/.agents/skills/convex-quickstart/SKILL.md") {
		t.Errorf("expected apps/backend skill in message: %s", stderr)
	}
}

func TestEnforce_ConvexPreset_OnlyAffectsBackendDir(t *testing.T) {
	root := setupProject(t, projectFixture{
		config: `{"autoDiscover": false, "convex": true}`,
		docs: []string{
			"packages/backend/convex/_generated/ai/guidelines.md",
			"packages/backend/.agents/skills/convex-quickstart/SKILL.md",
		},
	})
	provider := sessionProvider(t.TempDir())

	// Editing a frontend file should NOT be blocked.
	_, err := runEnforce(t, provider, "s", filepath.Join(root, "apps", "web", "page.tsx"))
	if err != nil {
		t.Errorf("expected allow for apps/web/page.tsx, got %v", err)
	}
}

func TestEnforce_ConvexPreset_SkipsEditingOwnDocs(t *testing.T) {
	root := setupProject(t, projectFixture{
		config: `{"convex": true}`,
		docs: []string{
			"packages/backend/convex/_generated/ai/guidelines.md",
			"packages/backend/.agents/skills/convex-quickstart/SKILL.md",
		},
	})
	provider := sessionProvider(t.TempDir())

	// Editing the guidelines.md itself should be allowed.
	_, err := runEnforce(t, provider, "s", filepath.Join(root, "packages", "backend", "convex", "_generated", "ai", "guidelines.md"))
	if err != nil {
		t.Errorf("expected allow for editing own required doc, got %v", err)
	}
	// Editing a SKILL.md should be allowed.
	_, err = runEnforce(t, provider, "s", filepath.Join(root, "packages", "backend", ".agents", "skills", "convex-quickstart", "SKILL.md"))
	if err != nil {
		t.Errorf("expected allow for editing own SKILL.md, got %v", err)
	}
}

func TestEnforce_ConvexPreset_NoBackendDir_IsNoOp(t *testing.T) {
	root := setupProject(t, projectFixture{
		config: `{"convex": true}`,
		// No packages/backend exists — preset can't resolve docs.
		docs: []string{"packages/ui/CLAUDE.md"},
	})
	provider := sessionProvider(t.TempDir())

	// Random file edit with no backend dir: preset is silent, auto-discovery handles ui.
	_, err := runEnforce(t, provider, "s", filepath.Join(root, "apps", "web", "page.tsx"))
	if err != nil {
		t.Errorf("expected allow (no backend, no matching doc), got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Preset + auto-discovery coexistence
// ---------------------------------------------------------------------------

func TestEnforce_ConvexPreset_MergesWithClaudeMdAutoDiscovery(t *testing.T) {
	// When the convex preset and auto-discovered CLAUDE.md share the backend
	// pattern, required docs are the union of both sources.
	root := setupProject(t, projectFixture{
		config: `{"convex": true}`,
		docs: []string{
			"packages/backend/CLAUDE.md",                                 // auto-discovered
			"packages/backend/convex/_generated/ai/guidelines.md",        // preset
			"packages/backend/.agents/skills/convex-quickstart/SKILL.md", // preset
			"packages/ui/CLAUDE.md",                                      // auto-discover handles ui
		},
	})
	provider := sessionProvider(t.TempDir())

	// Reading only CLAUDE.md should NOT satisfy — preset docs still missing.
	seedSession(t, provider, "s", []string{"packages/backend/CLAUDE.md"})
	stderr, err := runEnforce(t, provider, "s", filepath.Join(root, "packages", "backend", "foo.ts"))
	if _, ok := err.(*ExitError); !ok {
		t.Fatalf("expected block (preset docs still unread), got %v", err)
	}
	if !strings.Contains(stderr, "guidelines.md") {
		t.Errorf("expected guidelines.md in message: %s", stderr)
	}

	// Reading only preset docs should NOT satisfy — CLAUDE.md still missing.
	seedSession(t, provider, "s2", []string{
		"packages/backend/convex/_generated/ai/guidelines.md",
		"packages/backend/.agents/skills/convex-quickstart/SKILL.md",
	})
	stderr, err = runEnforce(t, provider, "s2", filepath.Join(root, "packages", "backend", "foo.ts"))
	if _, ok := err.(*ExitError); !ok {
		t.Fatalf("expected block (CLAUDE.md still unread), got %v", err)
	}
	if !strings.Contains(stderr, "packages/backend/CLAUDE.md") {
		t.Errorf("expected CLAUDE.md in message: %s", stderr)
	}

	// Reading all of them → allow.
	seedSession(t, provider, "s3", []string{
		"packages/backend/CLAUDE.md",
		"packages/backend/convex/_generated/ai/guidelines.md",
		"packages/backend/.agents/skills/convex-quickstart/SKILL.md",
	})
	_, err = runEnforce(t, provider, "s3", filepath.Join(root, "packages", "backend", "foo.ts"))
	if err != nil {
		t.Fatalf("expected allow with full union read, got %v", err)
	}

	// Editing a ui file should still be gated by CLAUDE.md auto-discovery.
	stderr, err = runEnforce(t, provider, "s4", filepath.Join(root, "packages", "ui", "Button.tsx"))
	if _, ok := err.(*ExitError); !ok {
		t.Fatalf("expected ui block, got %v", err)
	}
	if !strings.Contains(stderr, "packages/ui/CLAUDE.md") {
		t.Errorf("expected ui CLAUDE.md: %s", stderr)
	}
}

// ---------------------------------------------------------------------------
// Skip patterns and edge cases
// ---------------------------------------------------------------------------

func TestEnforce_SkipPatterns(t *testing.T) {
	root := setupProject(t, projectFixture{
		config: `{}`,
		docs:   []string{"packages/backend/CLAUDE.md"},
	})
	provider := sessionProvider(t.TempDir())
	cases := []string{
		filepath.Join(root, "packages", "backend", "foo.test.ts"),
		filepath.Join(root, "packages", "backend", "__tests__", "foo.ts"),
		filepath.Join(root, "packages", "backend", "_generated", "api.d.ts"),
		filepath.Join(root, "packages", "backend", "types.d.ts"),
	}
	for _, path := range cases {
		t.Run(path, func(t *testing.T) {
			_, err := runEnforce(t, provider, "s", path)
			if err != nil {
				t.Errorf("expected allow, got %v", err)
			}
		})
	}
}

func TestEnforce_NonEditToolsAllowed(t *testing.T) {
	root := setupProject(t, projectFixture{
		config: `{}`,
		docs:   []string{"packages/backend/CLAUDE.md"},
	})
	provider := sessionProvider(t.TempDir())
	input := HookInput{
		ToolName:  "Read",
		ToolInput: map[string]interface{}{"file_path": filepath.Join(root, "packages", "backend", "foo.ts")},
		SessionID: "s",
	}
	data, _ := json.Marshal(input)
	var stderr bytes.Buffer
	if err := enforceWithProvider(bytes.NewReader(data), &stderr, provider); err != nil {
		t.Errorf("expected allow for Read tool, got %v", err)
	}
}

func TestEnforce_UnmappedFilesAllowed(t *testing.T) {
	root := setupProject(t, projectFixture{
		config: `{}`,
		docs:   []string{"packages/backend/CLAUDE.md"},
	})
	provider := sessionProvider(t.TempDir())
	_, err := runEnforce(t, provider, "s", filepath.Join(root, "src", "utils", "helper.ts"))
	if err != nil {
		t.Errorf("expected allow, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Track mode
// ---------------------------------------------------------------------------

func TestTrack_RegistersKnownDoc(t *testing.T) {
	root := setupProject(t, projectFixture{
		config: `{"convex": true}`,
		docs: []string{
			"packages/backend/convex/_generated/ai/guidelines.md",
			"packages/backend/.agents/skills/convex-quickstart/SKILL.md",
		},
	})
	provider := sessionProvider(t.TempDir())

	for _, read := range []string{
		filepath.Join(root, "packages", "backend", "convex", "_generated", "ai", "guidelines.md"),
		filepath.Join(root, "packages", "backend", ".agents", "skills", "convex-quickstart", "SKILL.md"),
	} {
		input := HookInput{
			ToolName:  "Read",
			ToolInput: map[string]interface{}{"file_path": read},
			SessionID: "s",
		}
		data, _ := json.Marshal(input)
		if err := trackWithProvider(bytes.NewReader(data), provider); err != nil {
			t.Fatalf("track %s: %v", read, err)
		}
	}

	// Edit should now be allowed.
	stderr, err := runEnforce(t, provider, "s", filepath.Join(root, "packages", "backend", "foo.ts"))
	if err != nil {
		t.Errorf("expected allow after reading both docs, got %v, stderr: %s", err, stderr)
	}
}

func TestTrack_IgnoresUnregisteredDocs(t *testing.T) {
	root := setupProject(t, projectFixture{
		config: `{"convex": true}`,
		docs: []string{
			"packages/backend/convex/_generated/ai/guidelines.md",
		},
	})
	provider := sessionProvider(t.TempDir())

	input := HookInput{
		ToolName:  "Read",
		ToolInput: map[string]interface{}{"file_path": filepath.Join(root, "README.md")},
		SessionID: "s",
	}
	data, _ := json.Marshal(input)
	if err := trackWithProvider(bytes.NewReader(data), provider); err != nil {
		t.Fatalf("track: %v", err)
	}
	if _, err := os.Stat(provider("s")); err == nil {
		t.Error("expected no session file for unregistered doc")
	}
}

func TestTrack_NoConfig_IsNoOp(t *testing.T) {
	root := setupProject(t, projectFixture{
		docs: []string{"packages/backend/CLAUDE.md"},
	})
	provider := sessionProvider(t.TempDir())

	input := HookInput{
		ToolName:  "Read",
		ToolInput: map[string]interface{}{"file_path": filepath.Join(root, "packages", "backend", "CLAUDE.md")},
		SessionID: "s",
	}
	data, _ := json.Marshal(input)
	if err := trackWithProvider(bytes.NewReader(data), provider); err != nil {
		t.Fatalf("track: %v", err)
	}
	if _, err := os.Stat(provider("s")); err == nil {
		t.Error("expected no session file without opt-in marker")
	}
}

func TestTrack_DoesNotDuplicate(t *testing.T) {
	root := setupProject(t, projectFixture{
		config: `{}`,
		docs:   []string{"packages/backend/CLAUDE.md"},
	})
	provider := sessionProvider(t.TempDir())
	seedSession(t, provider, "s", []string{"packages/backend/CLAUDE.md"})

	input := HookInput{
		ToolName:  "Read",
		ToolInput: map[string]interface{}{"file_path": filepath.Join(root, "packages", "backend", "CLAUDE.md")},
		SessionID: "s",
	}
	data, _ := json.Marshal(input)
	if err := trackWithProvider(bytes.NewReader(data), provider); err != nil {
		t.Fatalf("track: %v", err)
	}
	loaded, _ := loadDocsReadWithProvider("s", provider)
	if len(loaded) != 1 {
		t.Errorf("expected 1 entry, got %d: %v", len(loaded), loaded)
	}
}

// ---------------------------------------------------------------------------
// Unit tests for internal helpers
// ---------------------------------------------------------------------------

func TestShouldCheckFile(t *testing.T) {
	tests := []struct {
		name   string
		path   string
		expect bool
	}{
		{"regular file", "packages/backend/foo.ts", true},
		{"test file", "packages/backend/foo.test.ts", false},
		{"test directory", "packages/backend/__tests__/foo.ts", false},
		{"generated file", "packages/backend/_generated/api.d.ts", false},
		{"type definition", "packages/backend/types.d.ts", false},
		{"node modules", "node_modules/package/index.js", false},
		// CLAUDE.md is no longer in static skip; handled dynamically.
		{"CLAUDE.md", "packages/backend/CLAUDE.md", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldCheckFile(tt.path); got != tt.expect {
				t.Errorf("shouldCheckFile(%q) = %v, want %v", tt.path, got, tt.expect)
			}
		})
	}
}

func TestGetRequiredDoc_LongestPatternWins(t *testing.T) {
	// pre-sorted longest-first, mirroring buildMappings output
	mappings := []DocMapping{
		{Pattern: "apps/mobile/components/", Docs: []string{"apps/mobile/components/CLAUDE.md"}, Name: "apps/mobile/components"},
		{Pattern: "apps/mobile/", Docs: []string{"apps/mobile/CLAUDE.md"}, Name: "apps/mobile"},
	}
	got := getRequiredDoc("apps/mobile/components/Button.tsx", mappings)
	if got == nil || got.Docs[0] != "apps/mobile/components/CLAUDE.md" {
		t.Errorf("expected nested mapping to win, got %+v", got)
	}
	got = getRequiredDoc("apps/mobile/screen.tsx", mappings)
	if got == nil || got.Docs[0] != "apps/mobile/CLAUDE.md" {
		t.Errorf("expected parent mapping for non-nested path, got %+v", got)
	}
}

func TestConfig_ConvexUnmarshal(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		enabled bool
		backend string
	}{
		{"bare true", `{"convex": true}`, true, ""},
		{"bare false", `{"convex": false}`, false, ""},
		{"empty object", `{"convex": {}}`, true, ""},
		{"with backendDir", `{"convex": {"backendDir": "apps/backend"}}`, true, "apps/backend"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var c Config
			if err := json.Unmarshal([]byte(tt.raw), &c); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if c.Convex == nil {
				t.Fatalf("expected Convex to be populated, got nil")
			}
			if c.Convex.Enabled != tt.enabled {
				t.Errorf("Enabled = %v, want %v", c.Convex.Enabled, tt.enabled)
			}
			if c.Convex.BackendDir != tt.backend {
				t.Errorf("BackendDir = %q, want %q", c.Convex.BackendDir, tt.backend)
			}
		})
	}

	// `null` leaves Convex as nil (standard Go JSON pointer semantics);
	// the hook treats nil as disabled.
	var c Config
	if err := json.Unmarshal([]byte(`{"convex": null}`), &c); err != nil {
		t.Fatalf("unmarshal null: %v", err)
	}
	if c.Convex != nil {
		t.Errorf("expected nil Convex for null, got %+v", c.Convex)
	}
}

func TestDiscoverMappings_SortedByLengthDesc(t *testing.T) {
	root := setupProject(t, projectFixture{
		config: `{}`,
		docs: []string{
			"apps/CLAUDE.md",
			"apps/mobile/CLAUDE.md",
			"apps/mobile/components/CLAUDE.md",
		},
	})
	cfg, _, err := loadConfig(root)
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	got := buildMappings(root, cfg)
	if len(got) != 3 {
		t.Fatalf("expected 3 mappings, got %d: %+v", len(got), got)
	}
	if got[0].Pattern != "apps/mobile/components/" {
		t.Errorf("expected most-specific first, got %q", got[0].Pattern)
	}
	if got[2].Pattern != "apps/" {
		t.Errorf("expected least-specific last, got %q", got[2].Pattern)
	}
}

func TestDiscoverMappings_SkipsIgnoredDirs(t *testing.T) {
	root := setupProject(t, projectFixture{
		config: `{}`,
		docs: []string{
			"node_modules/lib/CLAUDE.md",
			".git/CLAUDE.md",
			"packages/backend/CLAUDE.md",
		},
	})
	cfg, _, _ := loadConfig(root)
	got := buildMappings(root, cfg)
	if len(got) != 1 || got[0].Docs[0] != "packages/backend/CLAUDE.md" {
		t.Errorf("expected only packages/backend mapping, got %+v", got)
	}
}

func TestFindProjectRoot(t *testing.T) {
	root := setupProject(t, projectFixture{config: `{}`})
	deep := filepath.Join(root, "a", "b", "c")
	if err := os.MkdirAll(deep, 0755); err != nil {
		t.Fatalf("mkdir deep: %v", err)
	}
	if got := findProjectRoot(filepath.Join(deep, "file.ts")); got != root {
		t.Errorf("expected root %q, got %q", root, got)
	}
	elsewhere := t.TempDir()
	if got := findProjectRoot(filepath.Join(elsewhere, "file.ts")); got != "" {
		t.Errorf("expected empty (no config), got %q", got)
	}
}

func TestParseInput(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		expectErr bool
	}{
		{"valid input", `{"tool_name":"Edit","tool_input":{"file_path":"foo.ts"},"session_id":"test"}`, false},
		{"invalid JSON", `{invalid`, true},
		{"empty input", ``, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseInput(strings.NewReader(tt.input))
			if tt.expectErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.expectErr && err != nil {
				t.Errorf("expected no error, got %v", err)
			}
		})
	}
}

func TestContains(t *testing.T) {
	slice := []string{"foo", "bar", "baz"}
	tests := []struct {
		item   string
		expect bool
	}{
		{"foo", true},
		{"bar", true},
		{"baz", true},
		{"qux", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.item, func(t *testing.T) {
			if got := contains(slice, tt.item); got != tt.expect {
				t.Errorf("contains(%v, %q) = %v, want %v", slice, tt.item, got, tt.expect)
			}
		})
	}
}

func TestSessionPersistence(t *testing.T) {
	provider := sessionProvider(t.TempDir())
	sessionID := "persist-test"
	docs := []string{"packages/backend/CLAUDE.md", "apps/mobile/components/CLAUDE.md"}

	if err := saveDocsReadWithProvider(sessionID, docs, provider); err != nil {
		t.Fatalf("save failed: %v", err)
	}
	loaded, err := loadDocsReadWithProvider(sessionID, provider)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if len(loaded) != len(docs) {
		t.Errorf("expected %d docs, got %d", len(docs), len(loaded))
	}
	for _, d := range docs {
		if !contains(loaded, d) {
			t.Errorf("missing doc %q in %v", d, loaded)
		}
	}
}

// suppress unused-const lint if fixture helpers ever drop the reference
var _ = convexSkillContent
