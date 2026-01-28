package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestShouldTrackFile(t *testing.T) {
	tests := []struct {
		name     string
		filePath string
		want     bool
	}{
		// Valid files to track
		{
			name:     "backend convex typescript file",
			filePath: "/project/packages/backend/convex/users.ts",
			want:     true,
		},
		{
			name:     "mobile app component",
			filePath: "/project/apps/mobile/src/components/Button.tsx",
			want:     true,
		},
		{
			name:     "mobile app javascript file",
			filePath: "/project/apps/mobile/src/utils/helpers.js",
			want:     true,
		},

		// Files to skip - wrong extension
		{
			name:     "python file",
			filePath: "/project/packages/backend/convex/script.py",
			want:     false,
		},
		{
			name:     "markdown file",
			filePath: "/project/packages/backend/convex/README.md",
			want:     false,
		},
		{
			name:     "json file",
			filePath: "/project/apps/mobile/package.json",
			want:     false,
		},

		// Files to skip - generated
		{
			name:     "generated api file",
			filePath: "/project/packages/backend/_generated/api.d.ts",
			want:     false,
		},

		// Files to skip - test files
		{
			name:     "test file with .test.ts",
			filePath: "/project/packages/backend/convex/users.test.ts",
			want:     false,
		},
		{
			name:     "test file with .test.tsx",
			filePath: "/project/apps/mobile/src/components/Button.test.tsx",
			want:     false,
		},
		{
			name:     "test file in __tests__ directory",
			filePath: "/project/apps/mobile/src/__tests__/setup.ts",
			want:     false,
		},

		// Files to skip - config files
		{
			name:     "tsconfig.json",
			filePath: "/project/packages/backend/tsconfig.json",
			want:     false,
		},
		{
			name:     "metro.config in mobile",
			filePath: "/project/apps/mobile/metro.config.js",
			want:     false,
		},
		{
			name:     "babel.config in mobile",
			filePath: "/project/apps/mobile/babel.config.js",
			want:     false,
		},
		{
			name:     "app.config in mobile",
			filePath: "/project/apps/mobile/app.config.ts",
			want:     false,
		},

		// Files to skip - special patterns
		{
			name:     "schema.ts file",
			filePath: "/project/packages/backend/convex/schema.ts",
			want:     false,
		},
		{
			name:     "index.ts file",
			filePath: "/project/packages/backend/convex/index.ts",
			want:     false,
		},
		{
			name:     "Types.ts file",
			filePath: "/project/apps/mobile/src/Types.ts",
			want:     false,
		},
		{
			name:     "Constants.ts file",
			filePath: "/project/apps/mobile/src/Constants.ts",
			want:     false,
		},
		{
			name:     "declaration file",
			filePath: "/project/packages/backend/convex/types.d.ts",
			want:     false,
		},

		// Files to skip - wrong directory
		{
			name:     "web app file",
			filePath: "/project/apps/web/src/components/Button.tsx",
			want:     false,
		},
		{
			name:     "backend non-convex file",
			filePath: "/project/packages/backend/src/server.ts",
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldTrackFile(tt.filePath)
			if got != tt.want {
				t.Errorf("shouldTrackFile(%q) = %v, want %v", tt.filePath, got, tt.want)
			}
		})
	}
}

func TestIsTestFile(t *testing.T) {
	tests := []struct {
		name     string
		filePath string
		want     bool
	}{
		{
			name:     "test file with .test.ts",
			filePath: "/project/src/users.test.ts",
			want:     true,
		},
		{
			name:     "test file with .test.tsx",
			filePath: "/project/src/Button.test.tsx",
			want:     true,
		},
		{
			name:     "test file in __tests__ directory",
			filePath: "/project/src/__tests__/setup.ts",
			want:     true,
		},
		{
			name:     "regular source file",
			filePath: "/project/src/users.ts",
			want:     false,
		},
		{
			name:     "file with test in name but not pattern",
			filePath: "/project/src/testUtils.ts",
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isTestFile(tt.filePath)
			if got != tt.want {
				t.Errorf("isTestFile(%q) = %v, want %v", tt.filePath, got, tt.want)
			}
		})
	}
}

func TestContains(t *testing.T) {
	tests := []struct {
		name  string
		slice []string
		item  string
		want  bool
	}{
		{
			name:  "item exists",
			slice: []string{"a", "b", "c"},
			item:  "b",
			want:  true,
		},
		{
			name:  "item doesn't exist",
			slice: []string{"a", "b", "c"},
			item:  "d",
			want:  false,
		},
		{
			name:  "empty slice",
			slice: []string{},
			item:  "a",
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := contains(tt.slice, tt.item)
			if got != tt.want {
				t.Errorf("contains(%v, %q) = %v, want %v", tt.slice, tt.item, got, tt.want)
			}
		})
	}
}

func TestLoadSessionData(t *testing.T) {
	// Create temporary directory for test files
	tmpDir := t.TempDir()

	tests := []struct {
		name        string
		setupFile   func(string) string
		wantSources int
		wantTests   int
	}{
		{
			name: "non-existent file returns empty data",
			setupFile: func(dir string) string {
				return filepath.Join(dir, "nonexistent.json")
			},
			wantSources: 0,
			wantTests:   0,
		},
		{
			name: "valid file loads correctly",
			setupFile: func(dir string) string {
				path := filepath.Join(dir, "valid.json")
				data := SessionData{
					SourceFiles: []string{"/project/src/users.ts"},
					TestFiles:   []string{"/project/src/users.test.ts"},
				}
				content, _ := json.MarshalIndent(data, "", "  ")
				_ = os.WriteFile(path, content, 0644)
				return path
			},
			wantSources: 1,
			wantTests:   1,
		},
		{
			name: "invalid json returns empty data",
			setupFile: func(dir string) string {
				path := filepath.Join(dir, "invalid.json")
				_ = os.WriteFile(path, []byte("not valid json"), 0644)
				return path
			},
			wantSources: 0,
			wantTests:   0,
		},
		{
			name: "empty file returns empty data",
			setupFile: func(dir string) string {
				path := filepath.Join(dir, "empty.json")
				_ = os.WriteFile(path, []byte(""), 0644)
				return path
			},
			wantSources: 0,
			wantTests:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filePath := tt.setupFile(tmpDir)
			data, err := loadSessionData(filePath)
			if err != nil {
				t.Errorf("loadSessionData() error = %v", err)
				return
			}
			if len(data.SourceFiles) != tt.wantSources {
				t.Errorf("loadSessionData() got %d source files, want %d", len(data.SourceFiles), tt.wantSources)
			}
			if len(data.TestFiles) != tt.wantTests {
				t.Errorf("loadSessionData() got %d test files, want %d", len(data.TestFiles), tt.wantTests)
			}
		})
	}
}

func TestSaveSessionData(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name    string
		data    *SessionData
		wantErr bool
	}{
		{
			name: "saves empty data",
			data: &SessionData{
				SourceFiles: []string{},
				TestFiles:   []string{},
			},
			wantErr: false,
		},
		{
			name: "saves data with files",
			data: &SessionData{
				SourceFiles: []string{"/project/src/users.ts", "/project/src/posts.ts"},
				TestFiles:   []string{"/project/src/users.test.ts"},
			},
			wantErr: false,
		},
		{
			name: "creates directory if needed",
			data: &SessionData{
				SourceFiles: []string{"/project/src/users.ts"},
				TestFiles:   []string{},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filePath := filepath.Join(tmpDir, "sessions", tt.name+".json")
			err := saveSessionData(filePath, tt.data)
			if (err != nil) != tt.wantErr {
				t.Errorf("saveSessionData() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				// Verify file was created and can be loaded
				loaded, err := loadSessionData(filePath)
				if err != nil {
					t.Errorf("loadSessionData() error = %v", err)
					return
				}
				if len(loaded.SourceFiles) != len(tt.data.SourceFiles) {
					t.Errorf("saved/loaded source files mismatch: got %d, want %d",
						len(loaded.SourceFiles), len(tt.data.SourceFiles))
				}
				if len(loaded.TestFiles) != len(tt.data.TestFiles) {
					t.Errorf("saved/loaded test files mismatch: got %d, want %d",
						len(loaded.TestFiles), len(tt.data.TestFiles))
				}
			}
		})
	}
}

func TestEndToEnd(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name         string
		input        Input
		wantSources  []string
		wantTests    []string
		initialData  *SessionData
		shouldModify bool
		description  string
	}{
		{
			name: "tracks new source file",
			input: Input{
				SessionID: "test-session",
				ToolInput: map[string]interface{}{
					"file_path": "/project/packages/backend/convex/users.ts",
				},
			},
			initialData: &SessionData{
				SourceFiles: []string{},
				TestFiles:   []string{},
			},
			wantSources:  []string{"/project/packages/backend/convex/users.ts"},
			wantTests:    []string{},
			shouldModify: true,
			description:  "should add source file to tracking",
		},
		{
			name: "tracks new test file",
			input: Input{
				SessionID: "test-session",
				ToolInput: map[string]interface{}{
					"file_path": "/project/packages/backend/convex/users.test.ts",
				},
			},
			initialData: &SessionData{
				SourceFiles: []string{},
				TestFiles:   []string{},
			},
			wantSources:  []string{},
			wantTests:    []string{"/project/packages/backend/convex/users.test.ts"},
			shouldModify: true,
			description:  "should add test file to tracking",
		},
		{
			name: "doesn't track duplicate source file",
			input: Input{
				SessionID: "test-session",
				ToolInput: map[string]interface{}{
					"file_path": "/project/packages/backend/convex/users.ts",
				},
			},
			initialData: &SessionData{
				SourceFiles: []string{"/project/packages/backend/convex/users.ts"},
				TestFiles:   []string{},
			},
			wantSources:  []string{"/project/packages/backend/convex/users.ts"},
			wantTests:    []string{},
			shouldModify: false,
			description:  "should not duplicate existing source file",
		},
		{
			name: "ignores non-trackable file",
			input: Input{
				SessionID: "test-session",
				ToolInput: map[string]interface{}{
					"file_path": "/project/README.md",
				},
			},
			initialData: &SessionData{
				SourceFiles: []string{},
				TestFiles:   []string{},
			},
			wantSources:  []string{},
			wantTests:    []string{},
			shouldModify: false,
			description:  "should ignore markdown files",
		},
		{
			name: "tracks mobile app file",
			input: Input{
				SessionID: "test-session",
				ToolInput: map[string]interface{}{
					"file_path": "/project/apps/mobile/src/components/Button.tsx",
				},
			},
			initialData: &SessionData{
				SourceFiles: []string{},
				TestFiles:   []string{},
			},
			wantSources:  []string{"/project/apps/mobile/src/components/Button.tsx"},
			wantTests:    []string{},
			shouldModify: true,
			description:  "should track mobile app source files",
		},
		{
			name: "ignores mobile config file",
			input: Input{
				SessionID: "test-session",
				ToolInput: map[string]interface{}{
					"file_path": "/project/apps/mobile/metro.config.js",
				},
			},
			initialData: &SessionData{
				SourceFiles: []string{},
				TestFiles:   []string{},
			},
			wantSources:  []string{},
			wantTests:    []string{},
			shouldModify: false,
			description:  "should ignore mobile config files",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up temp session file
			sessionFile := filepath.Join(tmpDir, tt.input.SessionID+".json")

			// Save initial data if provided
			if tt.initialData != nil {
				if err := saveSessionData(sessionFile, tt.initialData); err != nil {
					t.Fatalf("setup failed: %v", err)
				}
			}

			// Simulate the main logic (without os.Exit)
			filePath, ok := tt.input.ToolInput["file_path"].(string)
			if ok && filePath != "" {
				data, err := loadSessionData(sessionFile)
				if err != nil {
					t.Fatalf("loadSessionData failed: %v", err)
				}

				if isTestFile(filePath) {
					if !contains(data.TestFiles, filePath) {
						data.TestFiles = append(data.TestFiles, filePath)
						if err := saveSessionData(sessionFile, data); err != nil {
							t.Fatalf("saveSessionData failed: %v", err)
						}
					}
				} else if shouldTrackFile(filePath) {
					if !contains(data.SourceFiles, filePath) {
						data.SourceFiles = append(data.SourceFiles, filePath)
						if err := saveSessionData(sessionFile, data); err != nil {
							t.Fatalf("saveSessionData failed: %v", err)
						}
					}
				}
			}

			// Verify results
			result, err := loadSessionData(sessionFile)
			if err != nil {
				t.Fatalf("loadSessionData failed: %v", err)
			}

			// Check source files
			if len(result.SourceFiles) != len(tt.wantSources) {
				t.Errorf("%s: got %d source files, want %d",
					tt.description, len(result.SourceFiles), len(tt.wantSources))
			}
			for _, want := range tt.wantSources {
				if !contains(result.SourceFiles, want) {
					t.Errorf("%s: missing source file %q", tt.description, want)
				}
			}

			// Check test files
			if len(result.TestFiles) != len(tt.wantTests) {
				t.Errorf("%s: got %d test files, want %d",
					tt.description, len(result.TestFiles), len(tt.wantTests))
			}
			for _, want := range tt.wantTests {
				if !contains(result.TestFiles, want) {
					t.Errorf("%s: missing test file %q", tt.description, want)
				}
			}
		})
	}
}
