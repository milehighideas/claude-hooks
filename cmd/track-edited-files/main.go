package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Input represents the JSON input from stdin
type Input struct {
	SessionID string                 `json:"session_id"`
	ToolInput map[string]interface{} `json:"tool_input"`
}

// SessionData tracks files edited during a Claude session
type SessionData struct {
	SourceFiles []string `json:"source_files"`
	TestFiles   []string `json:"test_files"`
}

// File patterns to skip (no test required)
var skipPatterns = []string{
	"_generated/",
	"schema.ts",
	"schema/",
	"index.ts",
	"Types.ts",
	"Constants.ts",
	".test.ts",
	".test.tsx",
	"__tests__/",
	".d.ts",
	".config.",
	"jest.setup",
	"vitest.config",
	".eslintrc",
	".prettierrc",
	"package.json",
	"tsconfig.json",
	".md",
	".json",
	".css",
	".scss",
}

// shouldTrackFile determines if this file should be tracked for test enforcement
func shouldTrackFile(filePath string) bool {
	// Only track TypeScript/JavaScript source files
	validExtensions := []string{".ts", ".tsx", ".js", ".jsx"}
	hasValidExtension := false
	for _, ext := range validExtensions {
		if strings.HasSuffix(filePath, ext) {
			hasValidExtension = true
			break
		}
	}
	if !hasValidExtension {
		return false
	}

	// Skip files matching skip patterns
	for _, pattern := range skipPatterns {
		if strings.Contains(filePath, pattern) {
			return false
		}
	}

	// Only track files in backend/convex or mobile app
	if strings.Contains(filePath, "packages/backend/convex/") {
		return true
	}

	if strings.Contains(filePath, "apps/mobile/") {
		// Skip mobile config/setup files
		mobileSkipPatterns := []string{"metro.config", "babel.config", "app.config"}
		for _, pattern := range mobileSkipPatterns {
			if strings.Contains(filePath, pattern) {
				return false
			}
		}
		return true
	}

	return false
}

// isTestFile checks if this is a test file
func isTestFile(filePath string) bool {
	return strings.Contains(filePath, ".test.ts") ||
		strings.Contains(filePath, ".test.tsx") ||
		strings.Contains(filePath, "__tests__/")
}

// loadSessionData loads existing session data or returns empty structure
func loadSessionData(sessionFile string) (*SessionData, error) {
	data := &SessionData{
		SourceFiles: []string{},
		TestFiles:   []string{},
	}

	if _, err := os.Stat(sessionFile); os.IsNotExist(err) {
		return data, nil
	}

	content, err := os.ReadFile(sessionFile)
	if err != nil {
		return data, nil
	}

	if err := json.Unmarshal(content, data); err != nil {
		return data, nil
	}

	return data, nil
}

// saveSessionData saves session data to file
func saveSessionData(sessionFile string, data *SessionData) error {
	// Create directory if it doesn't exist
	dir := filepath.Dir(sessionFile)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating sessions directory: %w", err)
	}

	// Marshal data to JSON
	content, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling session data: %w", err)
	}

	// Write to file
	if err := os.WriteFile(sessionFile, content, 0644); err != nil {
		return fmt.Errorf("writing session file: %w", err)
	}

	return nil
}

// contains checks if a string slice contains a string
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func main() {
	// Read input from stdin
	var input Input
	if err := json.NewDecoder(os.Stdin).Decode(&input); err != nil {
		// Invalid JSON - exit with success (non-blocking)
		os.Exit(0)
	}

	// Extract file path from tool_input
	filePath, ok := input.ToolInput["file_path"].(string)
	if !ok || filePath == "" {
		// No file path - exit with success
		os.Exit(0)
	}

	sessionID := input.SessionID
	if sessionID == "" {
		sessionID = "unknown"
	}

	// Determine session file path
	homeDir, err := os.UserHomeDir()
	if err != nil {
		// Can't determine home dir - exit with success (non-blocking)
		os.Exit(0)
	}

	sessionFile := filepath.Join(homeDir, ".claude", "sessions", sessionID+".json")

	// Load current session data
	data, err := loadSessionData(sessionFile)
	if err != nil {
		// Error loading - exit with success (non-blocking)
		os.Exit(0)
	}

	// Categorize and track the file
	if isTestFile(filePath) {
		if !contains(data.TestFiles, filePath) {
			data.TestFiles = append(data.TestFiles, filePath)
			if err := saveSessionData(sessionFile, data); err != nil {
				// Error saving - exit with success (non-blocking)
				os.Exit(0)
			}
		}
	} else if shouldTrackFile(filePath) {
		if !contains(data.SourceFiles, filePath) {
			data.SourceFiles = append(data.SourceFiles, filePath)
			if err := saveSessionData(sessionFile, data); err != nil {
				// Error saving - exit with success (non-blocking)
				os.Exit(0)
			}
		}
	}

	// Always exit 0 - tracking is non-blocking
	os.Exit(0)
}
