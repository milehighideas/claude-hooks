package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestIsGitCommit(t *testing.T) {
	tests := []struct {
		name     string
		command  string
		expected bool
	}{
		{"simple commit", "git commit", true},
		{"commit with message", "git commit -m 'test'", true},
		{"commit with flags", "git commit -am 'test'", true},
		{"commit amend", "git commit --amend", false},
		{"not a commit", "git status", false},
		{"git add", "git add .", false},
		{"empty string", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isGitCommit(tt.command)
			if result != tt.expected {
				t.Errorf("isGitCommit(%q) = %v, want %v", tt.command, result, tt.expected)
			}
		})
	}
}

func TestGetTestPathForSource(t *testing.T) {
	tests := []struct {
		name       string
		sourcePath string
		expected   string
	}{
		{
			"backend co-located ts",
			"packages/backend/convex/events/eventsMutations.ts",
			"packages/backend/convex/events/eventsMutations.test.ts",
		},
		{
			"mobile tsx co-located",
			"apps/mobile/components/Button.tsx",
			"apps/mobile/components/Button.test.tsx",
		},
		{
			"web tsx co-located",
			"apps/web/src/components/Header.tsx",
			"apps/web/src/components/Header.test.tsx",
		},
		{
			"portal ts co-located",
			"apps/portal/lib/utils.ts",
			"apps/portal/lib/utils.test.ts",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getTestPathForSource(tt.sourcePath)
			if result != tt.expected {
				t.Errorf("getTestPathForSource(%q) = %q, want %q", tt.sourcePath, result, tt.expected)
			}
		})
	}
}

func TestIsInTestsFolder(t *testing.T) {
	tests := []struct {
		name     string
		filePath string
		expected bool
	}{
		{"in __tests__ folder", "components/__tests__/Button.test.tsx", true},
		{"nested __tests__ folder", "src/components/__tests__/deep/Button.test.tsx", true},
		{"co-located test", "components/Button.test.tsx", false},
		{"source file", "components/Button.tsx", false},
		{"windows path in __tests__", "components\\__tests__\\Button.test.tsx", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isInTestsFolder(tt.filePath)
			if result != tt.expected {
				t.Errorf("isInTestsFolder(%q) = %v, want %v", tt.filePath, result, tt.expected)
			}
		})
	}
}

func TestGetProjectType(t *testing.T) {
	tests := []struct {
		name     string
		filePath string
		expected string
	}{
		{"backend file", "packages/backend/convex/events.ts", "backend"},
		{"mobile file", "apps/mobile/src/App.tsx", "mobile"},
		{"web file", "apps/web/src/pages/index.tsx", "web"},
		{"portal file", "apps/portal/src/Dashboard.tsx", "portal"},
		{"unknown file", "other/project/file.ts", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getProjectType(tt.filePath)
			if result != tt.expected {
				t.Errorf("getProjectType(%q) = %q, want %q", tt.filePath, result, tt.expected)
			}
		})
	}
}

func TestShouldSkipTestRequirement(t *testing.T) {
	// Create temp files for testing
	tmpDir := t.TempDir()

	// Create a re-export module
	reexportFile := filepath.Join(tmpDir, "reexport.ts")
	reexportContent := `export { foo } from './foo'
export { bar } from './bar'
`
	if err := os.WriteFile(reexportFile, []byte(reexportContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Create a normal module
	normalFile := filepath.Join(tmpDir, "normal.ts")
	normalContent := `function doSomething() {
  return 42;
}

export { doSomething };
`
	if err := os.WriteFile(normalFile, []byte(normalContent), 0644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name     string
		filePath string
		expected bool
	}{
		{"mock file", "src/__mocks__/api.ts", true},
		{"testing utility", "src/testing/helpers.ts", true},
		{"layout file", "app/_layout.tsx", true},
		{"type declaration", "types/index.d.ts", true},
		{"jest config", "jest.config.js", true},
		{"vitest config", "vitest.config.ts", true},
		{"babel config", "babel.config.js", true},
		{"tsconfig", "tsconfig.json", true},
		{"jest setup", "jest.setup.ts", true},
		{"vitest setup", "vitest.setup.ts", true},
		{"lib validators", "src/lib/validators/schema.ts", true},
		{"lib constants", "src/lib/constants/config.ts", true},
		{"social connections", "components/social-connections.tsx", true},
		{"sign in form", "components/sign-in-form.tsx", true},
		{"oauth callback", "pages/oauth-callback.tsx", true},
		{"re-export module", reexportFile, true},
		{"normal module", normalFile, false},
		{"regular component", "components/Button.tsx", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := shouldSkipTestRequirement(tt.filePath)
			if result != tt.expected {
				t.Errorf("shouldSkipTestRequirement(%q) = %v, want %v", tt.filePath, result, tt.expected)
			}
		})
	}
}

func TestIsReexportModule(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name     string
		content  string
		expected bool
	}{
		{
			"simple reexport",
			`export { foo } from './foo'
export { bar } from './bar'`,
			true,
		},
		{
			"reexport with comments",
			`// This is a comment
export { foo } from './foo'
/* Multi-line
   comment */
export { bar } from './bar'`,
			true,
		},
		{
			"reexport with use directive",
			`'use client'
export { foo } from './foo'
export { bar } from './bar'`,
			true,
		},
		{
			"normal module",
			`function doSomething() {
  return 42;
}
export { doSomething };`,
			false,
		},
		{
			"too many lines",
			`export { a } from './a'
export { b } from './b'
export { c } from './c'
export { d } from './d'
export { e } from './e'
export { f } from './f'
export { g } from './g'
export { h } from './h'
export { i } from './i'
export { j } from './j'
export { k } from './k'`,
			false,
		},
		{
			"mixed content",
			`export { foo } from './foo'
const x = 42;
export { bar } from './bar'`,
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testFile := filepath.Join(tmpDir, "test.ts")
			if err := os.WriteFile(testFile, []byte(tt.content), 0644); err != nil {
				t.Fatal(err)
			}

			result := isReexportModule(testFile)
			if result != tt.expected {
				t.Errorf("isReexportModule() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestLoadSessionData(t *testing.T) {
	tmpDir := t.TempDir()
	sessionsDir := filepath.Join(tmpDir, ".claude", "sessions")
	if err := os.MkdirAll(sessionsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Override home directory for testing
	originalHome := os.Getenv("HOME")
	if err := os.Setenv("HOME", tmpDir); err != nil {
		t.Fatalf("failed to set HOME: %v", err)
	}
	defer func() {
		_ = os.Setenv("HOME", originalHome)
	}()

	// Test with valid session data
	sessionID := "test-session"
	sessionFile := filepath.Join(sessionsDir, sessionID+".json")
	sessionContent := sessionData{
		SourceFiles: []string{"src/foo.ts", "src/bar.ts"},
		TestFiles:   []string{"src/__tests__/foo.test.ts"},
	}
	data, err := json.Marshal(sessionContent)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sessionFile, data, 0644); err != nil {
		t.Fatal(err)
	}

	result := loadSessionData(sessionID)
	if len(result.SourceFiles) != 2 {
		t.Errorf("Expected 2 source files, got %d", len(result.SourceFiles))
	}
	if len(result.TestFiles) != 1 {
		t.Errorf("Expected 1 test file, got %d", len(result.TestFiles))
	}

	// Test with non-existent session
	result = loadSessionData("non-existent")
	if len(result.SourceFiles) != 0 || len(result.TestFiles) != 0 {
		t.Error("Expected empty session data for non-existent session")
	}
}

func TestCheckTestExists(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.ts")
	if err := os.WriteFile(testFile, []byte("// test"), 0644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name     string
		testPath string
		cwd      string
		expected bool
	}{
		{"absolute path exists", testFile, tmpDir, true},
		{"absolute path not exists", filepath.Join(tmpDir, "nonexistent.ts"), tmpDir, false},
		{"relative path exists", "test.ts", tmpDir, true},
		{"relative path not exists", "nonexistent.ts", tmpDir, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := checkTestExists(tt.testPath, tt.cwd)
			if result != tt.expected {
				t.Errorf("checkTestExists(%q, %q) = %v, want %v", tt.testPath, tt.cwd, result, tt.expected)
			}
		})
	}
}

func TestCheckVitestSetup(t *testing.T) {
	tmpDir := t.TempDir()

	// Create package.json without test:run script
	packageJSON := filepath.Join(tmpDir, "package.json")
	pkgContent := map[string]interface{}{
		"scripts": map[string]interface{}{
			"test": "vitest",
		},
	}
	data, _ := json.Marshal(pkgContent)
	_ = os.WriteFile(packageJSON, data, 0644)

	// Should fail - no test:run script
	if checkVitestSetup(tmpDir) {
		t.Error("Expected checkVitestSetup to fail without test:run script")
	}

	// Add test:run script
	pkgContent["scripts"].(map[string]interface{})["test:run"] = "vitest run"
	data, _ = json.Marshal(pkgContent)
	_ = os.WriteFile(packageJSON, data, 0644)

	// Should still fail - no vitest.config.ts
	if checkVitestSetup(tmpDir) {
		t.Error("Expected checkVitestSetup to fail without vitest.config.ts")
	}

	// Create vitest.config.ts
	vitestConfig := filepath.Join(tmpDir, "vitest.config.ts")
	_ = os.WriteFile(vitestConfig, []byte("export default {}"), 0644)

	// Should pass now
	if !checkVitestSetup(tmpDir) {
		t.Error("Expected checkVitestSetup to pass with both files")
	}
}

func TestFindProjectRoot(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a monorepo structure
	backendDir := filepath.Join(tmpDir, "packages", "backend")
	convexDir := filepath.Join(backendDir, "convex")
	_ = os.MkdirAll(convexDir, 0755)

	backendPkg := filepath.Join(backendDir, "package.json")
	_ = os.WriteFile(backendPkg, []byte(`{"name": "backend"}`), 0644)

	mobileDir := filepath.Join(tmpDir, "apps", "mobile")
	_ = os.MkdirAll(mobileDir, 0755)

	mobilePkg := filepath.Join(mobileDir, "package.json")
	_ = os.WriteFile(mobilePkg, []byte(`{"name": "mobile"}`), 0644)

	appJSON := filepath.Join(mobileDir, "app.json")
	_ = os.WriteFile(appJSON, []byte(`{}`), 0644)

	// Test backend
	testFile := filepath.Join(convexDir, "test.ts")
	result := findProjectRoot(testFile)
	if result != backendDir {
		t.Errorf("findProjectRoot(%q) = %q, want %q", testFile, result, backendDir)
	}

	// Test mobile
	testFile = filepath.Join(mobileDir, "src", "App.tsx")
	result = findProjectRoot(testFile)
	if result != mobileDir {
		t.Errorf("findProjectRoot(%q) = %q, want %q", testFile, result, mobileDir)
	}
}

func TestStripTypeAssertions(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			"as Href",
			"return '/path' as Href",
			"return '/path'",
		},
		{
			"as const",
			"const x = 42 as const",
			"const x = 42",
		},
		{
			"generic type assertion",
			"value as Type<string>",
			"value",
		},
		{
			"simple type assertion",
			"obj as MyType",
			"obj",
		},
	}

	// Test the type assertion stripping logic
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This tests the internal logic that would be in stripTypeAssertions
			// Since it's an internal function, we verify the behavior through isTypeOnlyChange
			// For now, this is a placeholder for the type assertion logic
		})
	}
}

func TestSaveSessionData(t *testing.T) {
	tmpDir := t.TempDir()
	sessionsDir := filepath.Join(tmpDir, ".claude", "sessions")
	if err := os.MkdirAll(sessionsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Override home directory for testing
	originalHome := os.Getenv("HOME")
	if err := os.Setenv("HOME", tmpDir); err != nil {
		t.Fatalf("failed to set HOME: %v", err)
	}
	defer func() {
		_ = os.Setenv("HOME", originalHome)
	}()

	// Test saving session data
	sessionID := "save-test-session"
	sd := sessionData{
		SourceFiles: []string{"src/foo.ts", "src/bar.ts"},
		TestFiles:   []string{"src/foo.test.ts"},
	}

	err := saveSessionData(sessionID, sd)
	if err != nil {
		t.Fatalf("saveSessionData failed: %v", err)
	}

	// Verify file was created and has correct content
	sessionFile := filepath.Join(sessionsDir, sessionID+".json")
	data, err := os.ReadFile(sessionFile)
	if err != nil {
		t.Fatalf("failed to read session file: %v", err)
	}

	var loaded sessionData
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("failed to unmarshal session data: %v", err)
	}

	if len(loaded.SourceFiles) != 2 {
		t.Errorf("Expected 2 source files, got %d", len(loaded.SourceFiles))
	}
	if len(loaded.TestFiles) != 1 {
		t.Errorf("Expected 1 test file, got %d", len(loaded.TestFiles))
	}
}

func TestCleanStaleEntries(t *testing.T) {
	tmpDir := t.TempDir()

	// Create some files
	existingFile1 := filepath.Join(tmpDir, "existing1.ts")
	existingFile2 := filepath.Join(tmpDir, "existing2.ts")
	existingTest := filepath.Join(tmpDir, "existing1.test.ts")
	_ = os.WriteFile(existingFile1, []byte("// file 1"), 0644)
	_ = os.WriteFile(existingFile2, []byte("// file 2"), 0644)
	_ = os.WriteFile(existingTest, []byte("// test"), 0644)

	// Create session with mix of existing and non-existing files
	sd := sessionData{
		SourceFiles: []string{
			existingFile1,
			existingFile2,
			filepath.Join(tmpDir, "deleted.ts"),       // doesn't exist
			filepath.Join(tmpDir, "also-deleted.ts"), // doesn't exist
		},
		TestFiles: []string{
			existingTest,
			filepath.Join(tmpDir, "deleted.test.ts"), // doesn't exist
		},
	}

	cleaned := cleanStaleEntries(sd)

	// Should only have existing files
	if len(cleaned.SourceFiles) != 2 {
		t.Errorf("Expected 2 source files after cleaning, got %d", len(cleaned.SourceFiles))
	}
	if len(cleaned.TestFiles) != 1 {
		t.Errorf("Expected 1 test file after cleaning, got %d", len(cleaned.TestFiles))
	}

	// Verify the correct files were kept
	sourceSet := make(map[string]bool)
	for _, f := range cleaned.SourceFiles {
		sourceSet[f] = true
	}
	if !sourceSet[existingFile1] || !sourceSet[existingFile2] {
		t.Error("Expected existing files to be kept")
	}
}

func TestIntersectFiles(t *testing.T) {
	tests := []struct {
		name     string
		a        []string
		b        []string
		expected int
	}{
		{
			"full overlap",
			[]string{"file1.ts", "file2.ts"},
			[]string{"file1.ts", "file2.ts"},
			2,
		},
		{
			"partial overlap",
			[]string{"file1.ts", "file2.ts", "file3.ts"},
			[]string{"file1.ts", "file3.ts"},
			2,
		},
		{
			"no overlap",
			[]string{"file1.ts", "file2.ts"},
			[]string{"file3.ts", "file4.ts"},
			0,
		},
		{
			"empty first slice",
			[]string{},
			[]string{"file1.ts", "file2.ts"},
			0,
		},
		{
			"empty second slice",
			[]string{"file1.ts", "file2.ts"},
			[]string{},
			0,
		},
		{
			"both empty",
			[]string{},
			[]string{},
			0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := intersectFiles(tt.a, tt.b)
			if len(result) != tt.expected {
				t.Errorf("intersectFiles() returned %d files, want %d", len(result), tt.expected)
			}
		})
	}
}

func TestContainsFile(t *testing.T) {
	files := []string{"/path/to/file1.ts", "/path/to/file2.ts", "/path/to/file3.ts"}

	tests := []struct {
		name     string
		target   string
		expected bool
	}{
		{"file exists", "/path/to/file1.ts", true},
		{"file not exists", "/path/to/file4.ts", false},
		{"empty target", "", false},
		{"similar but different", "/path/to/file1.tsx", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := containsFile(files, tt.target)
			if result != tt.expected {
				t.Errorf("containsFile() = %v, want %v", result, tt.expected)
			}
		})
	}

	// Test with empty slice
	if containsFile([]string{}, "file.ts") {
		t.Error("containsFile with empty slice should return false")
	}
}
