package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnforce(t *testing.T) {
	tests := []struct {
		name           string
		input          HookInput
		docsRead       []string
		expectBlock    bool
		expectContains string
	}{
		{
			name: "allow non-edit tools",
			input: HookInput{
				ToolName:  "Read",
				ToolInput: map[string]interface{}{"file_path": "packages/backend/foo.ts"},
				SessionID: "test-session",
			},
			docsRead:    []string{},
			expectBlock: false,
		},
		{
			name: "allow edit when doc already read",
			input: HookInput{
				ToolName:  "Edit",
				ToolInput: map[string]interface{}{"file_path": "packages/backend/foo.ts"},
				SessionID: "test-session",
			},
			docsRead:    []string{"packages/backend/CLAUDE.md"},
			expectBlock: false,
		},
		{
			name: "block edit when doc not read",
			input: HookInput{
				ToolName:  "Edit",
				ToolInput: map[string]interface{}{"file_path": "packages/backend/foo.ts"},
				SessionID: "test-session",
			},
			docsRead:       []string{},
			expectBlock:    true,
			expectContains: "PLEASE READ DOCUMENTATION FIRST",
		},
		{
			name: "block write when doc not read",
			input: HookInput{
				ToolName:  "Write",
				ToolInput: map[string]interface{}{"file_path": "apps/mobile/components/Button.tsx"},
				SessionID: "test-session",
			},
			docsRead:       []string{},
			expectBlock:    true,
			expectContains: "Mobile components",
		},
		{
			name: "allow edit of test files without doc",
			input: HookInput{
				ToolName:  "Edit",
				ToolInput: map[string]interface{}{"file_path": "packages/backend/foo.test.ts"},
				SessionID: "test-session",
			},
			docsRead:    []string{},
			expectBlock: false,
		},
		{
			name: "allow edit of generated files without doc",
			input: HookInput{
				ToolName:  "Edit",
				ToolInput: map[string]interface{}{"file_path": "packages/backend/_generated/api.d.ts"},
				SessionID: "test-session",
			},
			docsRead:    []string{},
			expectBlock: false,
		},
		{
			name: "allow edit of CLAUDE.md itself",
			input: HookInput{
				ToolName:  "Edit",
				ToolInput: map[string]interface{}{"file_path": "packages/backend/CLAUDE.md"},
				SessionID: "test-session",
			},
			docsRead:    []string{},
			expectBlock: false,
		},
		{
			name: "allow edit of files without required docs",
			input: HookInput{
				ToolName:  "Edit",
				ToolInput: map[string]interface{}{"file_path": "src/utils/helper.ts"},
				SessionID: "test-session",
			},
			docsRead:    []string{},
			expectBlock: false,
		},
		{
			name: "handle mobile app routing directory",
			input: HookInput{
				ToolName:  "Edit",
				ToolInput: map[string]interface{}{"file_path": "apps/mobile/app/index.tsx"},
				SessionID: "test-session",
			},
			docsRead:       []string{},
			expectBlock:    true,
			expectContains: "Mobile app routing",
		},
		{
			name: "allow when different doc read",
			input: HookInput{
				ToolName:  "Edit",
				ToolInput: map[string]interface{}{"file_path": "packages/backend/foo.ts"},
				SessionID: "test-session",
			},
			docsRead:       []string{"apps/mobile/components/CLAUDE.md"},
			expectBlock:    true,
			expectContains: "Convex backend",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp session directory
			tempDir := t.TempDir()
			sessionFile := filepath.Join(tempDir, ".claude", "sessions", tt.input.SessionID+"-docs.json")

			// Create custom session file provider for testing
			provider := func(sessionID string) string {
				return filepath.Join(tempDir, ".claude", "sessions", sessionID+"-docs.json")
			}

			// Setup session data
			if len(tt.docsRead) > 0 {
				if err := os.MkdirAll(filepath.Dir(sessionFile), 0755); err != nil {
					t.Fatalf("failed to create session dir: %v", err)
				}
				sessionData := SessionData{DocsRead: tt.docsRead}
				data, _ := json.Marshal(sessionData)
				if err := os.WriteFile(sessionFile, data, 0644); err != nil {
					t.Fatalf("failed to write session file: %v", err)
				}
			}

			// Create input
			inputData, _ := json.Marshal(tt.input)
			inputReader := bytes.NewReader(inputData)
			var stderr bytes.Buffer

			// Run enforce
			err := enforceWithProvider(inputReader, &stderr, provider)

			if tt.expectBlock {
				// Should return exit code 2
				if err == nil {
					t.Fatal("expected error but got nil")
				}
				exitErr, ok := err.(*ExitError)
				if !ok {
					t.Fatalf("expected ExitError but got %T", err)
				}
				if exitErr.Code != 2 {
					t.Errorf("expected exit code 2 but got %d", exitErr.Code)
				}
				if tt.expectContains != "" && !strings.Contains(stderr.String(), tt.expectContains) {
					t.Errorf("expected stderr to contain %q but got: %s", tt.expectContains, stderr.String())
				}
			} else {
				// Should allow
				if err != nil {
					t.Errorf("expected no error but got: %v", err)
				}
				if stderr.Len() > 0 {
					t.Errorf("expected no stderr output but got: %s", stderr.String())
				}
			}
		})
	}
}

func TestTrack(t *testing.T) {
	tests := []struct {
		name           string
		input          HookInput
		existingDocs   []string
		expectTracked  []string
		shouldNotTrack bool
	}{
		{
			name: "track backend CLAUDE.md read",
			input: HookInput{
				ToolName:  "Read",
				ToolInput: map[string]interface{}{"file_path": "packages/backend/CLAUDE.md"},
				SessionID: "test-session",
			},
			existingDocs:  []string{},
			expectTracked: []string{"packages/backend/CLAUDE.md"},
		},
		{
			name: "track mobile components CLAUDE.md read",
			input: HookInput{
				ToolName:  "Read",
				ToolInput: map[string]interface{}{"file_path": "apps/mobile/components/CLAUDE.md"},
				SessionID: "test-session",
			},
			existingDocs:  []string{},
			expectTracked: []string{"apps/mobile/components/CLAUDE.md"},
		},
		{
			name: "append to existing tracked docs",
			input: HookInput{
				ToolName:  "Read",
				ToolInput: map[string]interface{}{"file_path": "apps/mobile/app/CLAUDE.md"},
				SessionID: "test-session",
			},
			existingDocs:  []string{"packages/backend/CLAUDE.md"},
			expectTracked: []string{"packages/backend/CLAUDE.md", "apps/mobile/app/CLAUDE.md"},
		},
		{
			name: "don't duplicate existing docs",
			input: HookInput{
				ToolName:  "Read",
				ToolInput: map[string]interface{}{"file_path": "packages/backend/CLAUDE.md"},
				SessionID: "test-session",
			},
			existingDocs:  []string{"packages/backend/CLAUDE.md"},
			expectTracked: []string{"packages/backend/CLAUDE.md"},
		},
		{
			name: "ignore non-Read tools",
			input: HookInput{
				ToolName:  "Edit",
				ToolInput: map[string]interface{}{"file_path": "packages/backend/CLAUDE.md"},
				SessionID: "test-session",
			},
			existingDocs:   []string{},
			shouldNotTrack: true,
		},
		{
			name: "ignore non-tracked files",
			input: HookInput{
				ToolName:  "Read",
				ToolInput: map[string]interface{}{"file_path": "src/utils/helper.ts"},
				SessionID: "test-session",
			},
			existingDocs:   []string{},
			shouldNotTrack: true,
		},
		{
			name: "track with absolute path",
			input: HookInput{
				ToolName:  "Read",
				ToolInput: map[string]interface{}{"file_path": "/Users/test/project/packages/backend/CLAUDE.md"},
				SessionID: "test-session",
			},
			existingDocs:  []string{},
			expectTracked: []string{"packages/backend/CLAUDE.md"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp session directory
			tempDir := t.TempDir()
			sessionFile := filepath.Join(tempDir, ".claude", "sessions", tt.input.SessionID+"-docs.json")

			// Create custom session file provider for testing
			provider := func(sessionID string) string {
				return filepath.Join(tempDir, ".claude", "sessions", sessionID+"-docs.json")
			}

			// Setup existing session data
			if len(tt.existingDocs) > 0 {
				if err := os.MkdirAll(filepath.Dir(sessionFile), 0755); err != nil {
					t.Fatalf("failed to create session dir: %v", err)
				}
				sessionData := SessionData{DocsRead: tt.existingDocs}
				data, _ := json.Marshal(sessionData)
				if err := os.WriteFile(sessionFile, data, 0644); err != nil {
					t.Fatalf("failed to write session file: %v", err)
				}
			}

			// Create input
			inputData, _ := json.Marshal(tt.input)
			inputReader := bytes.NewReader(inputData)

			// Run track
			err := trackWithProvider(inputReader, provider)
			if err != nil {
				t.Fatalf("track returned error: %v", err)
			}

			if tt.shouldNotTrack {
				// Verify file wasn't created or wasn't modified
				if _, err := os.Stat(sessionFile); err == nil && len(tt.existingDocs) == 0 {
					t.Error("expected no session file but it was created")
				}
			} else {
				// Verify session file was created/updated
				data, err := os.ReadFile(sessionFile)
				if err != nil {
					t.Fatalf("failed to read session file: %v", err)
				}

				var sessionData SessionData
				if err := json.Unmarshal(data, &sessionData); err != nil {
					t.Fatalf("failed to parse session file: %v", err)
				}

				if len(sessionData.DocsRead) != len(tt.expectTracked) {
					t.Errorf("expected %d docs but got %d: %v", len(tt.expectTracked), len(sessionData.DocsRead), sessionData.DocsRead)
				}

				for _, expected := range tt.expectTracked {
					if !contains(sessionData.DocsRead, expected) {
						t.Errorf("expected doc %q to be tracked but it wasn't: %v", expected, sessionData.DocsRead)
					}
				}
			}
		})
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
			result := shouldCheckFile(tt.path)
			if result != tt.expect {
				t.Errorf("shouldCheckFile(%q) = %v, want %v", tt.path, result, tt.expect)
			}
		})
	}
}

func TestGetRequiredDoc(t *testing.T) {
	tests := []struct {
		name       string
		path       string
		expectDoc  string
		expectName string
		expectNil  bool
	}{
		{
			name:       "backend file",
			path:       "packages/backend/foo.ts",
			expectDoc:  "packages/backend/CLAUDE.md",
			expectName: "Convex backend",
		},
		{
			name:       "mobile components file",
			path:       "apps/mobile/components/Button.tsx",
			expectDoc:  "apps/mobile/components/CLAUDE.md",
			expectName: "Mobile components",
		},
		{
			name:       "mobile app file",
			path:       "apps/mobile/app/index.tsx",
			expectDoc:  "apps/mobile/app/CLAUDE.md",
			expectName: "Mobile app routing",
		},
		{
			name:      "unmapped file",
			path:      "src/utils/helper.ts",
			expectNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getRequiredDoc(tt.path)
			if tt.expectNil {
				if result != nil {
					t.Errorf("expected nil but got %v", result)
				}
			} else {
				if result == nil {
					t.Fatal("expected non-nil result but got nil")
				}
				if result.Doc != tt.expectDoc {
					t.Errorf("expected doc %q but got %q", tt.expectDoc, result.Doc)
				}
				if result.Name != tt.expectName {
					t.Errorf("expected name %q but got %q", tt.expectName, result.Name)
				}
			}
		})
	}
}

func TestParseInput(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		expectErr bool
	}{
		{
			name:      "valid input",
			input:     `{"tool_name":"Edit","tool_input":{"file_path":"foo.ts"},"session_id":"test"}`,
			expectErr: false,
		},
		{
			name:      "invalid JSON",
			input:     `{invalid`,
			expectErr: true,
		},
		{
			name:      "empty input",
			input:     ``,
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := strings.NewReader(tt.input)
			result, err := parseInput(reader)

			if tt.expectErr {
				if err == nil {
					t.Error("expected error but got nil")
				}
			} else {
				if err != nil {
					t.Errorf("expected no error but got: %v", err)
				}
				if result == nil {
					t.Error("expected non-nil result but got nil")
				}
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
			result := contains(slice, tt.item)
			if result != tt.expect {
				t.Errorf("contains(%v, %q) = %v, want %v", slice, tt.item, result, tt.expect)
			}
		})
	}
}

func TestSessionPersistence(t *testing.T) {
	tempDir := t.TempDir()

	// Create custom session file provider for testing
	provider := func(sessionID string) string {
		return filepath.Join(tempDir, ".claude", "sessions", sessionID+"-docs.json")
	}

	sessionID := "persist-test"
	docs := []string{"packages/backend/CLAUDE.md", "apps/mobile/components/CLAUDE.md"}

	// Save docs
	if err := saveDocsReadWithProvider(sessionID, docs, provider); err != nil {
		t.Fatalf("saveDocsReadWithProvider failed: %v", err)
	}

	// Load docs
	loaded, err := loadDocsReadWithProvider(sessionID, provider)
	if err != nil {
		t.Fatalf("loadDocsReadWithProvider failed: %v", err)
	}

	if len(loaded) != len(docs) {
		t.Errorf("expected %d docs but got %d", len(docs), len(loaded))
	}

	for _, doc := range docs {
		if !contains(loaded, doc) {
			t.Errorf("expected doc %q to be loaded but it wasn't: %v", doc, loaded)
		}
	}
}
