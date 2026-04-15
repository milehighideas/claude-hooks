package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/milehighideas/claude-hooks/internal/jsonc"
)

// Mode represents the operation mode of the hook
type Mode string

const (
	ModeEnforce Mode = "enforce"
	ModeTrack   Mode = "track"
)

// preCommitConfigFile is the shared project config. Its presence at a project
// root combined with features.docsTracker=true enables the hook. Shared with
// the pre-commit and validate-test-files binaries so there is one config file
// per project.
const preCommitConfigFile = ".pre-commit.json"

// defaultConvexBackendDir is the conventional Convex backend path used when
// `convex` is enabled and no explicit `backendDir` is provided.
const defaultConvexBackendDir = "packages/backend"

// DocMapping represents a directory pattern to required docs mapping.
// A mapping can require multiple documents; edits under Pattern are blocked
// until every doc in Docs has been read in the session.
type DocMapping struct {
	Pattern string
	Docs    []string
	Name    string
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

// Project represents a docs-tracker-enabled project with resolved mappings.
type Project struct {
	Root     string
	Mappings []DocMapping
	Config   *Config
}

// Config is the parsed `docsTrackerConfig` block from `.pre-commit.json`.
type Config struct {
	// AutoDiscover enables walking the project for files named in DocFileNames.
	// Default true.
	AutoDiscover *bool `json:"autoDiscover,omitempty"`

	// DocFileNames lists file names that auto-discovery treats as required docs
	// for their containing directory. Default ["CLAUDE.md"].
	DocFileNames []string `json:"docFileNames,omitempty"`

	// Convex enables the Convex preset: gating edits under the backend dir on
	// reading the generated guidelines plus every installed SKILL.md.
	Convex *ConvexConfig `json:"convex,omitempty"`

	// Mappings declares explicit directory-to-docs rules. Escape hatch for
	// cases where the required doc lives outside the gated directory, or
	// where you want a fixed path regardless of file name. Merges with
	// preset + auto-discovery on the same pattern (union of required docs).
	Mappings []CustomMapping `json:"mappings,omitempty"`

	// AppPaths restricts enforcement to files whose project-relative path
	// contains at least one of these substrings. Empty means "everything".
	// Mirrors the shape of srpConfig / testCoverageConfig / testFilesConfig.
	AppPaths []string `json:"appPaths,omitempty"`

	// ExcludePaths skips enforcement on files whose project-relative path
	// contains any of these substrings. Exclusions always win over AppPaths.
	ExcludePaths []string `json:"excludePaths,omitempty"`
}

// CustomMapping is an explicit directory-to-docs rule. Pattern is the
// project-relative directory prefix that triggers the gate; Docs are the
// project-relative doc paths that must be read before editing inside Pattern.
// Name is optional and used in the block message; derived from Pattern if
// unset.
type CustomMapping struct {
	Pattern string   `json:"pattern"`
	Docs    []string `json:"docs"`
	Name    string   `json:"name,omitempty"`
}

// rootConfig is the minimal view of .pre-commit.json we decode — just the
// feature gate and the nested docsTrackerConfig block. The full schema lives
// in cmd/pre-commit/config.go; we deliberately avoid coupling to it.
type rootConfig struct {
	Features struct {
		DocsTracker bool `json:"docsTracker"`
	} `json:"features"`
	DocsTrackerConfig Config `json:"docsTrackerConfig"`
}

// ConvexConfig configures the Convex preset. It accepts either a bare boolean
// (`"convex": true`) or an object (`"convex": {"backendDir": "..."}`).
type ConvexConfig struct {
	// Enabled is set by UnmarshalJSON when the key is present and truthy.
	Enabled bool `json:"-"`
	// BackendDir overrides the default `packages/backend` location.
	BackendDir string `json:"backendDir,omitempty"`
}

// UnmarshalJSON accepts both `true`/`false` literals and an object form.
func (c *ConvexConfig) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	switch {
	case bytes.Equal(trimmed, []byte("true")):
		c.Enabled = true
		return nil
	case bytes.Equal(trimmed, []byte("false")), bytes.Equal(trimmed, []byte("null")):
		c.Enabled = false
		return nil
	}
	type raw struct {
		BackendDir string `json:"backendDir"`
	}
	var r raw
	if err := json.Unmarshal(data, &r); err != nil {
		return fmt.Errorf("convex config: %w", err)
	}
	c.Enabled = true
	c.BackendDir = r.BackendDir
	return nil
}

// isAutoDiscover returns the effective AutoDiscover value, defaulting to true.
func (c *Config) isAutoDiscover() bool {
	if c == nil || c.AutoDiscover == nil {
		return true
	}
	return *c.AutoDiscover
}

// effectiveDocFileNames returns DocFileNames with a CLAUDE.md default.
func (c *Config) effectiveDocFileNames() []string {
	if c == nil || len(c.DocFileNames) == 0 {
		return []string{"CLAUDE.md"}
	}
	return c.DocFileNames
}

// skipPatterns are path fragments that bypass the docs-read requirement.
// Applies to tests, generated code, declaration files, etc. Docs themselves
// are skipped dynamically per-project in enforce.
var skipPatterns = []string{
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

// enforceWithProvider implements the PreToolUse hook logic.
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

	// Per-app scope filter: skip files outside docsTrackerConfig.appPaths or
	// inside excludePaths. Both matchers apply to the project-relative path.
	if !isFileInScope(project.Config, relPath) {
		return nil
	}

	// Check if this file requires docs to be read
	required := getRequiredDoc(relPath, project.Mappings)
	if required == nil {
		return nil
	}

	// Editing a file that is itself a required doc for this mapping is allowed
	// (you can't read something you're creating, and doc maintenance is fine).
	for _, doc := range required.Docs {
		if relPath == doc {
			return nil
		}
	}

	// Figure out which docs have been read this session.
	docsRead, err := loadDocsReadWithProvider(hookInput.SessionID, provider)
	if err != nil {
		// If we can't load session data, allow operation
		return nil
	}

	var missing []string
	for _, doc := range required.Docs {
		if !contains(docsRead, doc) {
			missing = append(missing, doc)
		}
	}
	if len(missing) == 0 {
		return nil
	}

	var list strings.Builder
	for _, d := range missing {
		fmt.Fprintf(&list, "  %s\n", d)
	}

	msg := fmt.Sprintf(`
⚠️  PLEASE READ DOCUMENTATION FIRST

Before editing files in %s, please read the following:
%s
This ensures you follow project conventions and patterns.

Retry your edit once the documentation has been read.
`, required.Name, list.String())

	_, _ = fmt.Fprint(stderr, msg)
	return &ExitError{Code: 2, Message: "Documentation not read"}
}

// trackWithProvider implements the PostToolUse hook logic.
func trackWithProvider(input io.Reader, provider sessionFileProvider) error {
	hookInput, err := parseInput(input)
	if err != nil {
		return nil
	}

	if hookInput.ToolName != "Read" {
		return nil
	}

	filePath, ok := hookInput.ToolInput["file_path"].(string)
	if !ok || filePath == "" {
		return nil
	}

	project := findProject(filePath)
	if project == nil || len(project.Mappings) == 0 {
		return nil
	}

	relPath, ok := relativeToProject(project.Root, filePath)
	if !ok {
		return nil
	}

	if !isRegisteredDoc(relPath, project.Mappings) {
		return nil
	}

	docsRead, err := loadDocsReadWithProvider(hookInput.SessionID, provider)
	if err != nil {
		docsRead = []string{}
	}
	if !contains(docsRead, relPath) {
		docsRead = append(docsRead, relPath)
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

// shouldCheckFile determines if this file should require doc reading.
func shouldCheckFile(filePath string) bool {
	for _, pattern := range skipPatterns {
		if strings.Contains(filePath, pattern) {
			return false
		}
	}
	return true
}

// getRequiredDoc returns the most-specific mapping (longest pattern) that covers relPath.
// Mappings are pre-sorted longest-first by buildMappings.
func getRequiredDoc(relPath string, mappings []DocMapping) *DocMapping {
	for i := range mappings {
		if strings.HasPrefix(relPath, mappings[i].Pattern) {
			return &mappings[i]
		}
	}
	return nil
}

// isRegisteredDoc reports whether relPath matches a required doc in any mapping.
func isRegisteredDoc(relPath string, mappings []DocMapping) bool {
	for _, m := range mappings {
		for _, doc := range m.Docs {
			if doc == relPath {
				return true
			}
		}
	}
	return false
}

// findProject walks up from filePath looking for the opt-in marker
// (`.pre-commit.json`) and resolves its mappings. Returns nil when no project
// is found, the config is unreadable, or features.docsTracker is off.
func findProject(filePath string) *Project {
	root := findProjectRoot(filePath)
	if root == "" {
		return nil
	}
	cfg, enabled, err := loadConfig(root)
	if err != nil || !enabled {
		// Fail closed on the enable flag: only opted-in projects do any work.
		return nil
	}
	return &Project{
		Root:     root,
		Mappings: buildMappings(root, cfg),
		Config:   cfg,
	}
}

// findProjectRoot walks up from filePath's directory until it finds a
// `.pre-commit.json` marker. Returns "" if none is found.
func findProjectRoot(filePath string) string {
	abs, err := filepath.Abs(filePath)
	if err != nil {
		return ""
	}
	dir := filepath.Dir(abs)
	for {
		if _, err := os.Stat(filepath.Join(dir, preCommitConfigFile)); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// loadConfig reads and parses the `.pre-commit.json` file at root, returning
// the docsTrackerConfig block and whether features.docsTracker is enabled.
// The returned *Config is always non-nil so callers can chain into
// buildMappings without a nil check; errors flow through the third return.
// JSONC comments are stripped before parsing so the file can be annotated.
func loadConfig(root string) (*Config, bool, error) {
	path := filepath.Join(root, preCommitConfigFile)
	cfg := &Config{}
	data, err := jsonc.ReadFile(path)
	if err != nil {
		return cfg, false, err
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return cfg, false, nil
	}
	var rc rootConfig
	if err := json.Unmarshal(data, &rc); err != nil {
		return cfg, false, fmt.Errorf("parsing %s: %w", path, err)
	}
	inner := rc.DocsTrackerConfig
	return &inner, rc.Features.DocsTracker, nil
}

// isFileInScope applies the per-app include/exclude filter. Paths are compared
// as substrings of the file's project-relative path. Exclusions always win.
// A nil config means "everything in scope."
func isFileInScope(cfg *Config, relPath string) bool {
	if cfg == nil {
		return true
	}
	for _, p := range cfg.ExcludePaths {
		if strings.Contains(relPath, p) {
			return false
		}
	}
	if len(cfg.AppPaths) == 0 {
		return true
	}
	for _, p := range cfg.AppPaths {
		if strings.Contains(relPath, p) {
			return true
		}
	}
	return false
}

// buildMappings combines preset, custom, and auto-discovered mappings. Preset
// and custom mappings on the same pattern are merged (union of docs);
// auto-discovered mappings are dropped when they collide with an explicit
// pattern so the preset stays authoritative for its subtree.
func buildMappings(root string, cfg *Config) []DocMapping {
	byPattern := map[string]*DocMapping{}
	var order []string

	merge := func(src DocMapping) {
		if src.Pattern == "" || len(src.Docs) == 0 {
			return
		}
		existing, ok := byPattern[src.Pattern]
		if !ok {
			dup := src
			dup.Docs = append([]string(nil), src.Docs...)
			byPattern[src.Pattern] = &dup
			order = append(order, src.Pattern)
			return
		}
		for _, d := range src.Docs {
			if !contains(existing.Docs, d) {
				existing.Docs = append(existing.Docs, d)
			}
		}
	}

	// 1. Convex preset
	if cfg.Convex != nil && cfg.Convex.Enabled {
		if m := convexPresetMapping(root, cfg.Convex); m != nil {
			merge(*m)
		}
	}

	// 2. Explicit custom mappings
	for _, cm := range cfg.Mappings {
		if m := normalizeCustomMapping(cm); m != nil {
			merge(*m)
		}
	}

	// 3. Auto-discovery fills gaps — skip patterns already claimed above.
	if cfg.isAutoDiscover() {
		for _, m := range discoverMappings(root, cfg.effectiveDocFileNames()) {
			if _, claimed := byPattern[m.Pattern]; claimed {
				continue
			}
			merge(m)
		}
	}

	result := make([]DocMapping, 0, len(order))
	for _, p := range order {
		result = append(result, *byPattern[p])
	}
	sort.SliceStable(result, func(i, j int) bool {
		return len(result[i].Pattern) > len(result[j].Pattern)
	})
	return result
}

// normalizeCustomMapping cleans up user input: strip whitespace, normalize to
// forward slashes, trim leading "/", require a trailing "/" on patterns, drop
// empty docs. Returns nil when the mapping has no pattern or no docs.
func normalizeCustomMapping(cm CustomMapping) *DocMapping {
	pattern := filepath.ToSlash(strings.TrimSpace(cm.Pattern))
	pattern = strings.TrimLeft(pattern, "/")
	if pattern == "" {
		return nil
	}
	if !strings.HasSuffix(pattern, "/") {
		pattern += "/"
	}
	docs := make([]string, 0, len(cm.Docs))
	for _, d := range cm.Docs {
		d = filepath.ToSlash(strings.TrimSpace(d))
		d = strings.TrimLeft(d, "/")
		if d == "" {
			continue
		}
		docs = append(docs, d)
	}
	if len(docs) == 0 {
		return nil
	}
	name := strings.TrimSpace(cm.Name)
	if name == "" {
		name = strings.TrimSuffix(pattern, "/")
	}
	return &DocMapping{Pattern: pattern, Docs: docs, Name: name}
}

// convexPresetMapping builds the mapping for the Convex preset, pointing at
// the generated guidelines and every installed SKILL.md under .agents/skills.
// Returns nil when the backend dir does not exist or has no resolvable docs.
func convexPresetMapping(root string, cfg *ConvexConfig) *DocMapping {
	backendDir := cfg.BackendDir
	if backendDir == "" {
		backendDir = defaultConvexBackendDir
	}
	backendDir = filepath.ToSlash(strings.TrimSuffix(backendDir, "/"))
	if backendDir == "" {
		return nil
	}
	backendAbs := filepath.Join(root, filepath.FromSlash(backendDir))
	if info, err := os.Stat(backendAbs); err != nil || !info.IsDir() {
		return nil
	}

	var docs []string

	guidelines := backendDir + "/convex/_generated/ai/guidelines.md"
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(guidelines))); err == nil {
		docs = append(docs, guidelines)
	}

	skillGlob := filepath.Join(backendAbs, ".agents", "skills", "*", "SKILL.md")
	matches, _ := filepath.Glob(skillGlob)
	sort.Strings(matches)
	for _, m := range matches {
		rel, err := filepath.Rel(root, m)
		if err != nil {
			continue
		}
		docs = append(docs, filepath.ToSlash(rel))
	}

	if len(docs) == 0 {
		return nil
	}

	return &DocMapping{
		Pattern: backendDir + "/",
		Docs:    docs,
		Name:    "Convex backend (" + backendDir + ")",
	}
}

// discoverMappings walks projectRoot for files whose base name matches one of
// docFileNames and builds one mapping per containing directory. The project
// root is skipped (Claude Code already loads a root-level doc as context).
// When a directory contains multiple matching files, all are required.
func discoverMappings(projectRoot string, docFileNames []string) []DocMapping {
	if len(docFileNames) == 0 {
		return nil
	}
	nameSet := make(map[string]bool, len(docFileNames))
	for _, n := range docFileNames {
		nameSet[n] = true
	}

	byDir := map[string][]string{}
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
		if !nameSet[d.Name()] {
			return nil
		}
		rel, err := filepath.Rel(projectRoot, path)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		subdir := filepath.ToSlash(filepath.Dir(rel))
		if subdir == "." {
			return nil // skip project-root-level doc
		}
		byDir[subdir] = append(byDir[subdir], rel)
		return nil
	})

	mappings := make([]DocMapping, 0, len(byDir))
	for subdir, docs := range byDir {
		sort.Strings(docs)
		mappings = append(mappings, DocMapping{
			Pattern: subdir + "/",
			Docs:    docs,
			Name:    subdir,
		})
	}
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
