package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Mode represents the operation mode of the hook
type Mode string

const (
	ModeEnforce Mode = "enforce"
	ModeTrack   Mode = "track"
)

// configFileName is the opt-in marker file. Its presence at a project root
// enables the docs-tracker hook for that project.
const configFileName = "docs-tracker.json"

// DocMapping represents a directory pattern to required doc mapping
type DocMapping struct {
	Pattern string `json:"pattern"`
	Doc     string `json:"doc"`
	Name    string `json:"name"`
}

// HookInput represents the JSON input from Claude Code
type HookInput struct {
	ToolName  string                 `json:"tool_name"`
	ToolInput map[string]interface{} `json:"tool_input"`
	SessionID string                 `json:"session_id"`
}

// SessionData represents the stored session data
type SessionData struct {
	DocsRead []string `json:"docs_read"`
}

// Project represents a docs-tracker-enabled project with discovered mappings
type Project struct {
	Root     string
	Mappings []DocMapping
}

// skipPatterns are path fragments that bypass the docs-read requirement.
// Applies to tests, generated code, declaration files, etc.
var skipPatterns = []string{
	"CLAUDE.md",
	"__tests__/",
	".test.ts",
	".test.tsx",
	"_generated/",
	"node_modules/",
	".d.ts",
}

// skipDirs are directory names skipped during CLAUDE.md discovery.
var skipDirs = map[string]bool{
	"node_modules": true,
	".git":         true,
	"dist":         true,
	"build":        true,
	".next":        true,
	".turbo":       true,
	".vercel":      true,
	"_generated":   true,
}

// sessionFileProvider is a function that returns the session file path
type sessionFileProvider func(sessionID string) string

// defaultSessionFileProvider returns the default session file path
func defaultSessionFileProvider(sessionID string) string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		// Fallback to current directory
		homeDir = "."
	}
	return filepath.Join(homeDir, ".claude", "sessions", sessionID+"-docs.json")
}

// globalSessionFileProvider is the current session file provider
var globalSessionFileProvider = defaultSessionFileProvider

func main() {
	mode := flag.String("mode", "", "Operation mode: enforce or track")
	flag.Parse()

	if *mode == "" {
		fmt.Fprintln(os.Stderr, "Error: --mode flag is required (enforce or track)")
		os.Exit(1)
	}

	var err error
	switch Mode(*mode) {
	case ModeEnforce:
		err = enforceWithProvider(os.Stdin, os.Stderr, globalSessionFileProvider)
	case ModeTrack:
		err = trackWithProvider(os.Stdin, globalSessionFileProvider)
	default:
		fmt.Fprintf(os.Stderr, "Error: invalid mode %q (must be 'enforce' or 'track')\n", *mode)
		os.Exit(1)
	}

	if err != nil {
		if exitErr, ok := err.(*ExitError); ok {
			os.Exit(exitErr.Code)
		}
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// ExitError represents an error with a specific exit code
type ExitError struct {
	Code    int
	Message string
}

func (e *ExitError) Error() string {
	return e.Message
}

// enforceWithProvider implements the PreToolUse hook logic with a custom session file provider
func enforceWithProvider(input io.Reader, stderr io.Writer, provider sessionFileProvider) error {
	hookInput, err := parseInput(input)
	if err != nil {
		// Invalid JSON, allow operation
		return nil
	}

	// Only check Edit and Write tools
	if hookInput.ToolName != "Edit" && hookInput.ToolName != "Write" {
		return nil
	}

	filePath, ok := hookInput.ToolInput["file_path"].(string)
	if !ok || filePath == "" {
		return nil
	}

	// Skip certain files (tests, generated, etc.)
	if !shouldCheckFile(filePath) {
		return nil
	}

	// Find the opt-in project root; without one the hook is a no-op
	project := findProject(filePath)
	if project == nil || len(project.Mappings) == 0 {
		return nil
	}

	// Compute the file's path relative to the project root
	relPath, ok := relativeToProject(project.Root, filePath)
	if !ok {
		return nil
	}

	// Check if this file requires a doc to be read
	required := getRequiredDoc(relPath, project.Mappings)
	if required == nil {
		return nil
	}

	// Check if the doc has been read this session
	docsRead, err := loadDocsReadWithProvider(hookInput.SessionID, provider)
	if err != nil {
		// If we can't load session data, allow operation
		return nil
	}

	if contains(docsRead, required.Doc) {
		// Doc already read, allow edit
		return nil
	}

	// Doc not read - block with helpful message
	msg := fmt.Sprintf(`
⚠️  PLEASE READ DOCUMENTATION FIRST

Before editing files in %s, please read:
  %s

This ensures you follow project conventions and patterns.

Run: Read %s
Then retry your edit.
`, required.Name, required.Doc, required.Doc)

	_, _ = fmt.Fprint(stderr, msg)
	return &ExitError{Code: 2, Message: "Documentation not read"}
}

// trackWithProvider implements the PostToolUse hook logic with a custom session file provider
func trackWithProvider(input io.Reader, provider sessionFileProvider) error {
	hookInput, err := parseInput(input)
	if err != nil {
		// Invalid JSON, allow operation
		return nil
	}

	// Only track Read tool
	if hookInput.ToolName != "Read" {
		return nil
	}

	filePath, ok := hookInput.ToolInput["file_path"].(string)
	if !ok || filePath == "" {
		return nil
	}

	// Find the opt-in project; without one, nothing to track
	project := findProject(filePath)
	if project == nil || len(project.Mappings) == 0 {
		return nil
	}

	relPath, ok := relativeToProject(project.Root, filePath)
	if !ok {
		return nil
	}

	// Only track CLAUDE.md files this project has registered as mappings
	matchedDoc := ""
	for _, m := range project.Mappings {
		if relPath == m.Doc {
			matchedDoc = m.Doc
			break
		}
	}
	if matchedDoc == "" {
		return nil
	}

	// Track that this doc was read
	docsRead, err := loadDocsReadWithProvider(hookInput.SessionID, provider)
	if err != nil {
		docsRead = []string{}
	}

	// Add doc if not already present
	if !contains(docsRead, matchedDoc) {
		docsRead = append(docsRead, matchedDoc)
	}

	return saveDocsReadWithProvider(hookInput.SessionID, docsRead, provider)
}

// parseInput parses JSON input from stdin
func parseInput(input io.Reader) (*HookInput, error) {
	var hookInput HookInput
	decoder := json.NewDecoder(input)
	if err := decoder.Decode(&hookInput); err != nil {
		return nil, err
	}
	return &hookInput, nil
}

// shouldCheckFile determines if this file should require doc reading
func shouldCheckFile(filePath string) bool {
	for _, pattern := range skipPatterns {
		if strings.Contains(filePath, pattern) {
			return false
		}
	}
	return true
}

// getRequiredDoc returns the most-specific mapping (longest pattern) that covers relPath.
// Mappings are pre-sorted longest-first by discoverMappings.
func getRequiredDoc(relPath string, mappings []DocMapping) *DocMapping {
	for i := range mappings {
		if strings.HasPrefix(relPath, mappings[i].Pattern) {
			return &mappings[i]
		}
	}
	return nil
}

// findProject walks up from filePath looking for the opt-in marker
// (`.claude/docs-tracker.json`). Returns nil when no project is found.
func findProject(filePath string) *Project {
	root := findProjectRoot(filePath)
	if root == "" {
		return nil
	}
	return &Project{
		Root:     root,
		Mappings: discoverMappings(root),
	}
}

// findProjectRoot walks up from filePath's directory until it finds a
// `.claude/docs-tracker.json` marker. Returns "" if none is found.
func findProjectRoot(filePath string) string {
	abs, err := filepath.Abs(filePath)
	if err != nil {
		return ""
	}
	dir := filepath.Dir(abs)
	for {
		if _, err := os.Stat(filepath.Join(dir, ".claude", configFileName)); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// discoverMappings walks projectRoot for CLAUDE.md files and builds one mapping
// per discovered doc. The root-level CLAUDE.md is skipped (Claude Code already
// loads it as project context). Results are sorted longest-pattern-first so
// nested docs take precedence over parents.
func discoverMappings(projectRoot string) []DocMapping {
	var mappings []DocMapping
	_ = filepath.WalkDir(projectRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if path == projectRoot {
				return nil
			}
			if skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Name() != "CLAUDE.md" {
			return nil
		}
		rel, err := filepath.Rel(projectRoot, path)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if rel == "CLAUDE.md" {
			return nil // skip root doc
		}
		subdir := filepath.ToSlash(filepath.Dir(rel))
		mappings = append(mappings, DocMapping{
			Pattern: subdir + "/",
			Doc:     rel,
			Name:    subdir,
		})
		return nil
	})
	sort.Slice(mappings, func(i, j int) bool {
		return len(mappings[i].Pattern) > len(mappings[j].Pattern)
	})
	return mappings
}

// relativeToProject converts filePath to a forward-slash path relative to
// projectRoot. Returns false when the file lives outside the project.
func relativeToProject(projectRoot, filePath string) (string, bool) {
	abs, err := filepath.Abs(filePath)
	if err != nil {
		return "", false
	}
	rel, err := filepath.Rel(projectRoot, abs)
	if err != nil {
		return "", false
	}
	rel = filepath.ToSlash(rel)
	if rel == ".." || strings.HasPrefix(rel, "../") {
		return "", false
	}
	return rel, true
}

// loadDocsReadWithProvider loads the set of docs read this session using a custom provider
func loadDocsReadWithProvider(sessionID string, provider sessionFileProvider) ([]string, error) {
	sessionFile := provider(sessionID)

	data, err := os.ReadFile(sessionFile)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, fmt.Errorf("reading session file: %w", err)
	}

	var sessionData SessionData
	if err := json.Unmarshal(data, &sessionData); err != nil {
		return nil, fmt.Errorf("parsing session file: %w", err)
	}

	return sessionData.DocsRead, nil
}

// saveDocsReadWithProvider saves docs read to session file using a custom provider
func saveDocsReadWithProvider(sessionID string, docs []string, provider sessionFileProvider) error {
	sessionFile := provider(sessionID)

	// Ensure sessions directory exists
	sessionsDir := filepath.Dir(sessionFile)
	if err := os.MkdirAll(sessionsDir, 0755); err != nil {
		return fmt.Errorf("creating sessions directory: %w", err)
	}

	sessionData := SessionData{DocsRead: docs}
	data, err := json.Marshal(sessionData)
	if err != nil {
		return fmt.Errorf("marshaling session data: %w", err)
	}

	if err := os.WriteFile(sessionFile, data, 0644); err != nil {
		return fmt.Errorf("writing session file: %w", err)
	}

	return nil
}

// contains checks if a slice contains a string
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
