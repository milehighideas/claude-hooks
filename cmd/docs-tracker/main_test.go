package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// projectFixture describes a temp project layout for tests.
type projectFixture struct {
	// enabled creates the opt-in marker .claude/docs-tracker.json
	enabled bool
	// docs is the list of CLAUDE.md files to create (relative to project root),
	// e.g. []string{"packages/backend/CLAUDE.md", "apps/mobile/app/CLAUDE.md"}.
	docs []string
	// extraFiles lists additional empty files to create (relative paths), useful
	// for ensuring intermediate directories exist.
	extraFiles []string
}

// setupProject creates a fixture on disk and returns the project root.
func setupProject(t *testing.T, fx projectFixture) string {
	t.Helper()
	root := t.TempDir()
	if fx.enabled {
		claudeDir := filepath.Join(root, ".claude")
		if err := os.MkdirAll(claudeDir, 0755); err != nil {
			t.Fatalf("mkdir .claude: %v", err)
		}
		if err := os.WriteFile(filepath.Join(claudeDir, "docs-tracker.json"), []byte("{}"), 0644); err != nil {
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

func TestEnforce_NoConfig_IsNoOp(t *testing.T) {
	// No .claude/docs-tracker.json anywhere, even with a CLAUDE.md present.
	root := setupProject(t, projectFixture{
		enabled: false,
		docs:    []string{"packages/backend/CLAUDE.md"},
	})
	provider := sessionProvider(t.TempDir())

	input := HookInput{
		ToolName:  "Edit",
		ToolInput: map[string]interface{}{"file_path": filepath.Join(root, "packages", "backend", "foo.ts")},
		SessionID: "no-config",
	}
	data, _ := json.Marshal(input)
	var stderr bytes.Buffer
	if err := enforceWithProvider(bytes.NewReader(data), &stderr, provider); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if stderr.Len() != 0 {
		t.Errorf("expected no stderr, got: %s", stderr.String())
	}
}

func TestEnforce_ConfigButNoDocs_IsNoOp(t *testing.T) {
	root := setupProject(t, projectFixture{enabled: true})
	provider := sessionProvider(t.TempDir())

	input := HookInput{
		ToolName:  "Edit",
		ToolInput: map[string]interface{}{"file_path": filepath.Join(root, "src", "foo.ts")},
		SessionID: "config-no-docs",
	}
	data, _ := json.Marshal(input)
	var stderr bytes.Buffer
	if err := enforceWithProvider(bytes.NewReader(data), &stderr, provider); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestEnforce_BlocksWhenDocNotRead(t *testing.T) {
	root := setupProject(t, projectFixture{
		enabled: true,
		docs:    []string{"packages/backend/CLAUDE.md"},
	})
	provider := sessionProvider(t.TempDir())

	input := HookInput{
		ToolName:  "Edit",
		ToolInput: map[string]interface{}{"file_path": filepath.Join(root, "packages", "backend", "foo.ts")},
		SessionID: "block",
	}
	data, _ := json.Marshal(input)
	var stderr bytes.Buffer
	err := enforceWithProvider(bytes.NewReader(data), &stderr, provider)
	exitErr, ok := err.(*ExitError)
	if !ok {
		t.Fatalf("expected ExitError, got %T: %v", err, err)
	}
	if exitErr.Code != 2 {
		t.Errorf("expected exit code 2, got %d", exitErr.Code)
	}
	if !strings.Contains(stderr.String(), "PLEASE READ DOCUMENTATION FIRST") {
		t.Errorf("expected blocking message, got: %s", stderr.String())
	}
	if !strings.Contains(stderr.String(), "packages/backend/CLAUDE.md") {
		t.Errorf("expected doc path in message, got: %s", stderr.String())
	}
}

func TestEnforce_AllowsWhenDocRead(t *testing.T) {
	root := setupProject(t, projectFixture{
		enabled: true,
		docs:    []string{"packages/backend/CLAUDE.md"},
	})
	provider := sessionProvider(t.TempDir())
	seedSession(t, provider, "allow", []string{"packages/backend/CLAUDE.md"})

	input := HookInput{
		ToolName:  "Write",
		ToolInput: map[string]interface{}{"file_path": filepath.Join(root, "packages", "backend", "foo.ts")},
		SessionID: "allow",
	}
	data, _ := json.Marshal(input)
	var stderr bytes.Buffer
	if err := enforceWithProvider(bytes.NewReader(data), &stderr, provider); err != nil {
		t.Fatalf("expected allow, got: %v", err)
	}
}

func TestEnforce_NonEditToolsAllowed(t *testing.T) {
	root := setupProject(t, projectFixture{
		enabled: true,
		docs:    []string{"packages/backend/CLAUDE.md"},
	})
	provider := sessionProvider(t.TempDir())

	input := HookInput{
		ToolName:  "Read",
		ToolInput: map[string]interface{}{"file_path": filepath.Join(root, "packages", "backend", "foo.ts")},
		SessionID: "read-allow",
	}
	data, _ := json.Marshal(input)
	var stderr bytes.Buffer
	if err := enforceWithProvider(bytes.NewReader(data), &stderr, provider); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestEnforce_SkipPatterns(t *testing.T) {
	root := setupProject(t, projectFixture{
		enabled: true,
		docs:    []string{"packages/backend/CLAUDE.md"},
	})
	provider := sessionProvider(t.TempDir())

	cases := []string{
		filepath.Join(root, "packages", "backend", "foo.test.ts"),
		filepath.Join(root, "packages", "backend", "__tests__", "foo.ts"),
		filepath.Join(root, "packages", "backend", "_generated", "api.d.ts"),
		filepath.Join(root, "packages", "backend", "types.d.ts"),
		filepath.Join(root, "packages", "backend", "CLAUDE.md"),
	}
	for _, path := range cases {
		t.Run(path, func(t *testing.T) {
			input := HookInput{
				ToolName:  "Edit",
				ToolInput: map[string]interface{}{"file_path": path},
				SessionID: "skip",
			}
			data, _ := json.Marshal(input)
			var stderr bytes.Buffer
			if err := enforceWithProvider(bytes.NewReader(data), &stderr, provider); err != nil {
				t.Errorf("expected allow for %s, got %v", path, err)
			}
		})
	}
}

func TestEnforce_UnmappedFilesAllowed(t *testing.T) {
	root := setupProject(t, projectFixture{
		enabled: true,
		docs:    []string{"packages/backend/CLAUDE.md"},
	})
	provider := sessionProvider(t.TempDir())

	input := HookInput{
		ToolName:  "Edit",
		ToolInput: map[string]interface{}{"file_path": filepath.Join(root, "src", "utils", "helper.ts")},
		SessionID: "unmapped",
	}
	data, _ := json.Marshal(input)
	var stderr bytes.Buffer
	if err := enforceWithProvider(bytes.NewReader(data), &stderr, provider); err != nil {
		t.Errorf("expected allow for unmapped file, got %v", err)
	}
}

func TestEnforce_NestedDocTakesPrecedence(t *testing.T) {
	// apps/mobile/CLAUDE.md and apps/mobile/components/CLAUDE.md both exist.
	// Edits under components/ must require the more specific doc.
	root := setupProject(t, projectFixture{
		enabled: true,
		docs: []string{
			"apps/mobile/CLAUDE.md",
			"apps/mobile/components/CLAUDE.md",
		},
	})
	provider := sessionProvider(t.TempDir())
	// Session has parent doc read but not nested.
	seedSession(t, provider, "nested", []string{"apps/mobile/CLAUDE.md"})

	input := HookInput{
		ToolName:  "Edit",
		ToolInput: map[string]interface{}{"file_path": filepath.Join(root, "apps", "mobile", "components", "Button.tsx")},
		SessionID: "nested",
	}
	data, _ := json.Marshal(input)
	var stderr bytes.Buffer
	err := enforceWithProvider(bytes.NewReader(data), &stderr, provider)
	if _, ok := err.(*ExitError); !ok {
		t.Fatalf("expected block because nested doc not read, got %v", err)
	}
	if !strings.Contains(stderr.String(), "apps/mobile/components/CLAUDE.md") {
		t.Errorf("expected message to reference nested doc, got: %s", stderr.String())
	}
}

func TestEnforce_IgnoresRootClaudeMd(t *testing.T) {
	// A root-level CLAUDE.md should not itself cover every file in the project.
	root := setupProject(t, projectFixture{
		enabled: true,
		docs: []string{
			"CLAUDE.md",
			"packages/backend/CLAUDE.md",
		},
	})
	provider := sessionProvider(t.TempDir())
	// Only the scoped doc is needed; a file outside any mapping is allowed.
	input := HookInput{
		ToolName:  "Edit",
		ToolInput: map[string]interface{}{"file_path": filepath.Join(root, "scripts", "build.ts")},
		SessionID: "root-ignored",
	}
	data, _ := json.Marshal(input)
	var stderr bytes.Buffer
	if err := enforceWithProvider(bytes.NewReader(data), &stderr, provider); err != nil {
		t.Errorf("expected allow (root CLAUDE.md does not gate project-wide), got %v", err)
	}
}

func TestEnforce_SkipsDiscoveryInIgnoredDirs(t *testing.T) {
	// A CLAUDE.md under node_modules must not create a mapping.
	root := setupProject(t, projectFixture{
		enabled: true,
		docs:    []string{"node_modules/somepkg/CLAUDE.md"},
	})
	provider := sessionProvider(t.TempDir())
	input := HookInput{
		ToolName:  "Edit",
		ToolInput: map[string]interface{}{"file_path": filepath.Join(root, "src", "foo.ts")},
		SessionID: "ignored-dir",
	}
	data, _ := json.Marshal(input)
	var stderr bytes.Buffer
	if err := enforceWithProvider(bytes.NewReader(data), &stderr, provider); err != nil {
		t.Errorf("expected allow, got %v", err)
	}
}

func TestTrack_RecordsKnownDoc(t *testing.T) {
	root := setupProject(t, projectFixture{
		enabled: true,
		docs:    []string{"packages/backend/CLAUDE.md"},
	})
	sessionDir := t.TempDir()
	provider := sessionProvider(sessionDir)

	input := HookInput{
		ToolName:  "Read",
		ToolInput: map[string]interface{}{"file_path": filepath.Join(root, "packages", "backend", "CLAUDE.md")},
		SessionID: "track-basic",
	}
	data, _ := json.Marshal(input)
	if err := trackWithProvider(bytes.NewReader(data), provider); err != nil {
		t.Fatalf("track error: %v", err)
	}

	loaded, err := loadDocsReadWithProvider("track-basic", provider)
	if err != nil {
		t.Fatalf("load error: %v", err)
	}
	if !contains(loaded, "packages/backend/CLAUDE.md") {
		t.Errorf("expected tracked doc, got %v", loaded)
	}
}

func TestTrack_IgnoresNonReadTools(t *testing.T) {
	root := setupProject(t, projectFixture{
		enabled: true,
		docs:    []string{"packages/backend/CLAUDE.md"},
	})
	sessionDir := t.TempDir()
	provider := sessionProvider(sessionDir)

	input := HookInput{
		ToolName:  "Edit",
		ToolInput: map[string]interface{}{"file_path": filepath.Join(root, "packages", "backend", "CLAUDE.md")},
		SessionID: "track-edit",
	}
	data, _ := json.Marshal(input)
	if err := trackWithProvider(bytes.NewReader(data), provider); err != nil {
		t.Fatalf("track error: %v", err)
	}
	if _, err := os.Stat(provider("track-edit")); err == nil {
		t.Error("expected no session file because Edit is not a Read")
	}
}

func TestTrack_IgnoresUnregisteredDocs(t *testing.T) {
	root := setupProject(t, projectFixture{
		enabled: true,
		docs:    []string{"packages/backend/CLAUDE.md"},
	})
	sessionDir := t.TempDir()
	provider := sessionProvider(sessionDir)

	input := HookInput{
		ToolName:  "Read",
		ToolInput: map[string]interface{}{"file_path": filepath.Join(root, "src", "utils", "helper.ts")},
		SessionID: "track-unreg",
	}
	data, _ := json.Marshal(input)
	if err := trackWithProvider(bytes.NewReader(data), provider); err != nil {
		t.Fatalf("track error: %v", err)
	}
	if _, err := os.Stat(provider("track-unreg")); err == nil {
		t.Error("expected no session file for unregistered doc")
	}
}

func TestTrack_DoesNotDuplicate(t *testing.T) {
	root := setupProject(t, projectFixture{
		enabled: true,
		docs:    []string{"packages/backend/CLAUDE.md"},
	})
	sessionDir := t.TempDir()
	provider := sessionProvider(sessionDir)
	seedSession(t, provider, "track-dup", []string{"packages/backend/CLAUDE.md"})

	input := HookInput{
		ToolName:  "Read",
		ToolInput: map[string]interface{}{"file_path": filepath.Join(root, "packages", "backend", "CLAUDE.md")},
		SessionID: "track-dup",
	}
	data, _ := json.Marshal(input)
	if err := trackWithProvider(bytes.NewReader(data), provider); err != nil {
		t.Fatalf("track error: %v", err)
	}
	loaded, _ := loadDocsReadWithProvider("track-dup", provider)
	if len(loaded) != 1 {
		t.Errorf("expected 1 entry, got %d: %v", len(loaded), loaded)
	}
}

func TestTrack_NoConfigIsNoOp(t *testing.T) {
	root := setupProject(t, projectFixture{
		enabled: false,
		docs:    []string{"packages/backend/CLAUDE.md"},
	})
	sessionDir := t.TempDir()
	provider := sessionProvider(sessionDir)

	input := HookInput{
		ToolName:  "Read",
		ToolInput: map[string]interface{}{"file_path": filepath.Join(root, "packages", "backend", "CLAUDE.md")},
		SessionID: "track-no-config",
	}
	data, _ := json.Marshal(input)
	if err := trackWithProvider(bytes.NewReader(data), provider); err != nil {
		t.Fatalf("track error: %v", err)
	}
	if _, err := os.Stat(provider("track-no-config")); err == nil {
		t.Error("expected no session file when no config exists")
	}
}

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
		{"CLAUDE.md", "packages/backend/CLAUDE.md", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldCheckFile(tt.path); got != tt.expect {
				t.Errorf("shouldCheckFile(%q) = %v, want %v", tt.path, got, tt.expect)
			}
		})
	}
}

func TestGetRequiredDoc(t *testing.T) {
	mappings := []DocMapping{
		// Longest-first ordering (what discoverMappings produces).
		{Pattern: "apps/mobile/components/", Doc: "apps/mobile/components/CLAUDE.md", Name: "apps/mobile/components"},
		{Pattern: "apps/mobile/app/", Doc: "apps/mobile/app/CLAUDE.md", Name: "apps/mobile/app"},
		{Pattern: "packages/backend/", Doc: "packages/backend/CLAUDE.md", Name: "packages/backend"},
	}
	tests := []struct {
		name      string
		path      string
		expectDoc string
	}{
		{"backend file", "packages/backend/foo.ts", "packages/backend/CLAUDE.md"},
		{"components file", "apps/mobile/components/Button.tsx", "apps/mobile/components/CLAUDE.md"},
		{"app file", "apps/mobile/app/index.tsx", "apps/mobile/app/CLAUDE.md"},
		{"unmapped file", "src/utils/helper.ts", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getRequiredDoc(tt.path, mappings)
			if tt.expectDoc == "" {
				if got != nil {
					t.Errorf("expected nil, got %+v", got)
				}
				return
			}
			if got == nil {
				t.Fatalf("expected doc %q, got nil", tt.expectDoc)
			}
			if got.Doc != tt.expectDoc {
				t.Errorf("expected doc %q, got %q", tt.expectDoc, got.Doc)
			}
		})
	}
}

func TestDiscoverMappings_SortedByLengthDesc(t *testing.T) {
	root := setupProject(t, projectFixture{
		enabled: true,
		docs: []string{
			"apps/CLAUDE.md",
			"apps/mobile/CLAUDE.md",
			"apps/mobile/components/CLAUDE.md",
		},
	})
	got := discoverMappings(root)
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
		enabled: true,
		docs: []string{
			"node_modules/lib/CLAUDE.md",
			".git/CLAUDE.md",
			"packages/backend/CLAUDE.md",
		},
	})
	got := discoverMappings(root)
	if len(got) != 1 {
		t.Fatalf("expected only 1 mapping (packages/backend), got %d: %+v", len(got), got)
	}
	if got[0].Doc != "packages/backend/CLAUDE.md" {
		t.Errorf("unexpected doc: %q", got[0].Doc)
	}
}

func TestFindProjectRoot(t *testing.T) {
	root := setupProject(t, projectFixture{enabled: true})
	deep := filepath.Join(root, "a", "b", "c")
	if err := os.MkdirAll(deep, 0755); err != nil {
		t.Fatalf("mkdir deep: %v", err)
	}
	got := findProjectRoot(filepath.Join(deep, "file.ts"))
	if got != root {
		t.Errorf("expected root %q, got %q", root, got)
	}

	// Negative: no config in parents.
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
