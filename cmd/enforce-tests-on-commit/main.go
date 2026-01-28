package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const (
	exitAllow = 0
	exitBlock = 2
)

// escapeJestPattern escapes regex special characters in file paths for Jest
// Jest uses regex patterns for test file matching, so characters like [ ] ( ) need escaping
func escapeJestPattern(path string) string {
	// Characters that have special meaning in regex and need to be escaped
	specialChars := []string{"\\", "^", "$", ".", "|", "?", "*", "+", "(", ")", "[", "]", "{", "}"}
	result := path
	for _, char := range specialChars {
		result = strings.ReplaceAll(result, char, "\\"+char)
	}
	return result
}

// sessionData holds tracked files for a session
type sessionData struct {
	SourceFiles []string `json:"source_files"`
	TestFiles   []string `json:"test_files"`
}

// hookInput represents the JSON input from Claude
type hookInput struct {
	ToolInput struct {
		Command string `json:"command"`
	} `json:"tool_input"`
	SessionID string `json:"session_id"`
	Cwd       string `json:"cwd"`
}

// isGitCommit checks if this is a git commit command
func isGitCommit(command string) bool {
	// Match: git commit, git commit -m, etc.
	// Don't match: git commit --amend (allow amends for hook fixes)
	matched, _ := regexp.MatchString(`\bgit\s+commit\b`, command)
	if !matched {
		return false
	}
	// Allow amends (used for pre-commit hook fixes)
	if strings.Contains(command, "--amend") {
		return false
	}
	return true
}

// loadSessionData loads session tracking data
func loadSessionData(sessionID string) sessionData {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return sessionData{SourceFiles: []string{}, TestFiles: []string{}}
	}

	sessionFile := filepath.Join(homeDir, ".claude", "sessions", sessionID+".json")
	data, err := os.ReadFile(sessionFile)
	if err != nil {
		return sessionData{SourceFiles: []string{}, TestFiles: []string{}}
	}

	var sd sessionData
	if err := json.Unmarshal(data, &sd); err != nil {
		return sessionData{SourceFiles: []string{}, TestFiles: []string{}}
	}

	if sd.SourceFiles == nil {
		sd.SourceFiles = []string{}
	}
	if sd.TestFiles == nil {
		sd.TestFiles = []string{}
	}

	return sd
}

// saveSessionData persists session tracking data
func saveSessionData(sessionID string, sd sessionData) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	sessionFile := filepath.Join(homeDir, ".claude", "sessions", sessionID+".json")
	data, err := json.MarshalIndent(sd, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(sessionFile, data, 0644)
}

// cleanStaleEntries removes entries for files that no longer exist on disk
func cleanStaleEntries(sd sessionData) sessionData {
	var cleanedSources []string
	var cleanedTests []string

	for _, f := range sd.SourceFiles {
		if _, err := os.Stat(f); err == nil {
			cleanedSources = append(cleanedSources, f)
		}
	}

	for _, f := range sd.TestFiles {
		if _, err := os.Stat(f); err == nil {
			cleanedTests = append(cleanedTests, f)
		}
	}

	return sessionData{
		SourceFiles: cleanedSources,
		TestFiles:   cleanedTests,
	}
}

// getGitStagedFiles returns absolute paths of files staged for commit
func getGitStagedFiles(cwd string) []string {
	cmd := exec.Command("git", "diff", "--cached", "--name-only", "--diff-filter=ACMR")
	cmd.Dir = cwd
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil
	}

	// Get git root to convert relative paths to absolute
	rootCmd := exec.Command("git", "rev-parse", "--show-toplevel")
	rootCmd.Dir = cwd
	rootOutput, err := rootCmd.CombinedOutput()
	if err != nil {
		return nil
	}
	gitRoot := strings.TrimSpace(string(rootOutput))

	var stagedFiles []string
	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Convert to absolute path
		absPath := filepath.Join(gitRoot, line)
		stagedFiles = append(stagedFiles, absPath)
	}
	return stagedFiles
}

// intersectFiles returns files that exist in both slices
func intersectFiles(a, b []string) []string {
	bSet := make(map[string]bool)
	for _, f := range b {
		bSet[f] = true
	}

	var result []string
	for _, f := range a {
		if bSet[f] {
			result = append(result, f)
		}
	}
	return result
}

// containsFile checks if a file path is in the slice
func containsFile(files []string, target string) bool {
	for _, f := range files {
		if f == target {
			return true
		}
	}
	return false
}

// getTestPathForSource maps a source file to its expected test file path
func getTestPathForSource(sourcePath string) string {
	dir := filepath.Dir(sourcePath)
	base := filepath.Base(sourcePath)
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)

	// Determine test extension
	testExt := ".test.ts"
	if ext == ".tsx" {
		testExt = ".test.tsx"
	}

	// All projects use co-located tests (test file next to source file)
	return filepath.Join(dir, stem+testExt)
}

// isInTestsFolder checks if a file path is inside a __tests__ folder
func isInTestsFolder(filePath string) bool {
	return strings.Contains(filePath, "/__tests__/") || strings.Contains(filePath, "\\__tests__\\")
}

// getStagedTestsInTestsFolders returns any staged test files that are in __tests__ folders
func getStagedTestsInTestsFolders() []string {
	cmd := exec.Command("git", "diff", "--cached", "--name-only", "--diff-filter=A")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil
	}

	var testsInFolders []string
	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Check if it's a test file in a __tests__ folder
		if isInTestsFolder(line) && (strings.HasSuffix(line, ".test.ts") || strings.HasSuffix(line, ".test.tsx")) {
			testsInFolders = append(testsInFolders, line)
		}
	}
	return testsInFolders
}

// findProjectRoot finds the project root (directory with package.json) for a file
func findProjectRoot(filePath string) string {
	path := filePath
	if !filepath.IsAbs(path) {
		abs, err := filepath.Abs(path)
		if err != nil {
			return ""
		}
		path = abs
	}

	// Walk up to find package.json
	current := filepath.Dir(path)
	for {
		packageJSON := filepath.Join(current, "package.json")
		if _, err := os.Stat(packageJSON); err == nil {
			// Check if this is the monorepo root or a package
			convexDir := filepath.Join(current, "convex")
			if _, err := os.Stat(convexDir); err == nil {
				// This is packages/backend
				return current
			}

			appJSON := filepath.Join(current, "app.json")
			if _, err := os.Stat(appJSON); err == nil {
				// This is apps/mobile
				return current
			}

			base := filepath.Base(current)
			parent := filepath.Dir(current)
			if (base == "web" || base == "portal") && strings.HasSuffix(parent, "/apps") {
				// This is apps/web or apps/portal
				return current
			}

			packagesDir := filepath.Join(current, "packages")
			if _, err := os.Stat(packagesDir); err == nil {
				// This is monorepo root, keep looking
				parent := filepath.Dir(current)
				if parent == current {
					break
				}
				current = parent
				continue
			}

			return current
		}

		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}

	return ""
}

// checkTestExists checks if a test file exists
func checkTestExists(testPath, cwd string) bool {
	// Try absolute path first
	if _, err := os.Stat(testPath); err == nil {
		return true
	}

	// Try relative to cwd
	relPath := filepath.Join(cwd, testPath)
	if _, err := os.Stat(relPath); err == nil {
		return true
	}

	return false
}

// checkVitestSetup checks if Vitest is properly configured
func checkVitestSetup(projectRoot string) bool {
	packageJSON := filepath.Join(projectRoot, "package.json")
	vitestConfig := filepath.Join(projectRoot, "vitest.config.ts")

	// Check if package.json exists
	data, err := os.ReadFile(packageJSON)
	if err != nil {
		return false
	}

	// Check if test:run script exists
	var pkgData map[string]interface{}
	if err := json.Unmarshal(data, &pkgData); err != nil {
		return false
	}

	scripts, ok := pkgData["scripts"].(map[string]interface{})
	if !ok {
		return false
	}

	if _, ok := scripts["test:run"]; !ok {
		return false
	}

	// Check if vitest.config.ts exists
	if _, err := os.Stat(vitestConfig); err != nil {
		return false
	}

	return true
}

// runTests runs tests for the given test files
func runTests(testFiles []string, projectType, projectRoot string) (bool, string) {
	if len(testFiles) == 0 {
		return true, ""
	}

	// Check if Vitest is configured for web/portal projects
	if projectType == "web" || projectType == "portal" {
		if !checkVitestSetup(projectRoot) {
			msg := fmt.Sprintf("\n⚠️  Vitest not configured for %s\n\n"+
				"Please ask Claude to set up Vitest for testing.\n"+
				"See hook documentation for setup instructions.\n",
				projectType)
			return false, msg
		}
	}

	// Make paths relative to project root
	var relativePaths []string
	for _, tf := range testFiles {
		absTest := tf
		if !filepath.IsAbs(tf) {
			absTest = filepath.Join(projectRoot, tf)
		}

		rel, err := filepath.Rel(projectRoot, absTest)
		if err != nil {
			relativePaths = append(relativePaths, tf)
		} else {
			relativePaths = append(relativePaths, rel)
		}
	}

	var cmd *exec.Cmd
	switch projectType {
	case "backend":
		args := []string{"run", "test:run", "--"}
		args = append(args, relativePaths...)
		cmd = exec.Command("npm", args...)
	case "mobile":
		args := []string{"run", "test", "--", "--watchAll=false"}
		// Escape regex special characters for Jest pattern matching
		// This handles dynamic route files like [id].test.tsx
		for _, p := range relativePaths {
			args = append(args, escapeJestPattern(p))
		}
		cmd = exec.Command("npm", args...)
	case "web", "portal":
		args := []string{"run", "test:run", "--"}
		args = append(args, relativePaths...)
		cmd = exec.Command("npm", args...)
	default:
		return false, fmt.Sprintf("Unknown project type: %s", projectType)
	}

	cmd.Dir = projectRoot

	// Set timeout
	done := make(chan struct{})
	var output []byte
	var cmdErr error

	go func() {
		output, cmdErr = cmd.CombinedOutput()
		close(done)
	}()

	select {
	case <-done:
		// Command completed
	case <-time.After(120 * time.Second):
		// Timeout
		_ = cmd.Process.Kill()
		return false, "Tests timed out after 120 seconds"
	}

	outputStr := string(output)
	if len(outputStr) > 3000 {
		outputStr = outputStr[len(outputStr)-3000:]
	}

	if cmdErr != nil {
		return false, outputStr
	}

	return true, outputStr
}

// getProjectType determines project type from file path
func getProjectType(filePath string) string {
	if strings.Contains(filePath, "packages/backend") {
		return "backend"
	}
	if strings.Contains(filePath, "apps/mobile") {
		return "mobile"
	}
	if strings.Contains(filePath, "apps/web") {
		return "web"
	}
	if strings.Contains(filePath, "apps/portal") {
		return "portal"
	}
	return ""
}

// isTypeOnlyChange checks if the changes to a file are purely type-related
func isTypeOnlyChange(filePath string) bool {
	cmd := exec.Command("git", "diff", "--cached", "--", filePath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return false
	}

	diffOutput := string(output)
	if strings.TrimSpace(diffOutput) == "" {
		return false
	}

	// Parse diff to get removed (-) and added (+) lines
	var removedLines, addedLines []string
	for _, line := range strings.Split(diffOutput, "\n") {
		// Skip diff headers and context lines
		if strings.HasPrefix(line, "+++") || strings.HasPrefix(line, "---") {
			continue
		}
		if strings.HasPrefix(line, "@@") {
			continue
		}
		if strings.HasPrefix(line, "-") {
			content := strings.TrimSpace(line[1:])
			if content != "" {
				removedLines = append(removedLines, content)
			}
		} else if strings.HasPrefix(line, "+") {
			content := strings.TrimSpace(line[1:])
			if content != "" {
				addedLines = append(addedLines, content)
			}
		}
	}

	if len(removedLines) == 0 && len(addedLines) == 0 {
		return false
	}

	// Type assertion patterns to strip when comparing lines
	typeAssertionPatterns := []*regexp.Regexp{
		regexp.MustCompile(`\s+as\s+Href\b`),
		regexp.MustCompile(`\s+as\s+const\b`),
		regexp.MustCompile(`\s+as\s+\w+<[^>]+>`),
		regexp.MustCompile(`\s+as\s+\w+`),
	}

	// Type import patterns
	typeImportAdditions := []*regexp.Regexp{
		regexp.MustCompile(`,\s*Href\b`),
		regexp.MustCompile(`\bHref\s*,`),
		regexp.MustCompile(`,\s*type\s+\w+`),
	}

	// Patterns for type-only changes (removed or added)
	typeOnlyPatterns := []*regexp.Regexp{
		// Type/interface definitions
		regexp.MustCompile(`^export\s+type\s+`),
		regexp.MustCompile(`^export\s+interface\s+`),
		regexp.MustCompile(`^type\s+\w+\s*=`),
		regexp.MustCompile(`^interface\s+\w+\s*\{`),
		// Type imports
		regexp.MustCompile(`^import\s+type\s+`),
		regexp.MustCompile(`^import\s+\{\s*type\s+`),
		// Type re-exports
		regexp.MustCompile(`^export\s+type\s+\{`),
		regexp.MustCompile(`^export\s+\{\s*type\s+`),
		// Interface/type closing braces
		regexp.MustCompile(`^\};\s*$`),
		regexp.MustCompile(`^\};$`),
		// Type property lines (inside interface/type)
		regexp.MustCompile(`^\w+\s*:\s*\w+`),
		regexp.MustCompile(`^\w+\s*\?\s*:\s*\w+`),
	}

	isTypeOnlyLine := func(line string) bool {
		for _, pattern := range typeOnlyPatterns {
			if pattern.MatchString(line) {
				return true
			}
		}
		return false
	}

	stripTypeAssertions := func(line string) string {
		result := line
		for _, pattern := range typeAssertionPatterns {
			result = pattern.ReplaceAllString(result, "")
		}
		return result
	}

	isTypeImportChange := func(removed, added string) bool {
		if !strings.HasPrefix(removed, "import ") || !strings.HasPrefix(added, "import ") {
			return false
		}
		for _, pattern := range typeImportAdditions {
			modifiedAdded := pattern.ReplaceAllString(added, "")
			if strings.TrimSpace(modifiedAdded) == strings.TrimSpace(removed) {
				return true
			}
		}
		return false
	}

	// Try to pair removed and added lines
	unmatchedRemoved := make([]string, len(removedLines))
	copy(unmatchedRemoved, removedLines)
	unmatchedAdded := make([]string, len(addedLines))
	copy(unmatchedAdded, addedLines)

	for _, removed := range removedLines {
		strippedRemoved := stripTypeAssertions(removed)
		for i, added := range unmatchedAdded {
			strippedAdded := stripTypeAssertions(added)

			// Lines match after stripping type assertions
			if strippedRemoved == strippedAdded {
				// Remove from unmatched lists
				for j, r := range unmatchedRemoved {
					if r == removed {
						unmatchedRemoved = append(unmatchedRemoved[:j], unmatchedRemoved[j+1:]...)
						break
					}
				}
				unmatchedAdded = append(unmatchedAdded[:i], unmatchedAdded[i+1:]...)
				break
			}

			// Check for type import changes
			if isTypeImportChange(removed, added) {
				for j, r := range unmatchedRemoved {
					if r == removed {
						unmatchedRemoved = append(unmatchedRemoved[:j], unmatchedRemoved[j+1:]...)
						break
					}
				}
				unmatchedAdded = append(unmatchedAdded[:i], unmatchedAdded[i+1:]...)
				break
			}
		}
	}

	// Check if any unmatched lines are purely type-related additions/removals
	for i := len(unmatchedAdded) - 1; i >= 0; i-- {
		added := unmatchedAdded[i]
		if regexp.MustCompile(`^import\s+type\s+`).MatchString(added) || isTypeOnlyLine(added) {
			unmatchedAdded = append(unmatchedAdded[:i], unmatchedAdded[i+1:]...)
		}
	}

	// Check if unmatched removed lines are type-only (e.g., removed type definitions)
	for i := len(unmatchedRemoved) - 1; i >= 0; i-- {
		removed := unmatchedRemoved[i]
		if isTypeOnlyLine(removed) {
			unmatchedRemoved = append(unmatchedRemoved[:i], unmatchedRemoved[i+1:]...)
		}
	}

	// If there are unmatched lines, it's not type-only
	return len(unmatchedRemoved) == 0 && len(unmatchedAdded) == 0
}

// isReexportModule checks if a file is a re-export module
func isReexportModule(filePath string) bool {
	ext := filepath.Ext(filePath)
	if ext != ".ts" && ext != ".tsx" && ext != ".js" && ext != ".jsx" {
		return false
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return false
	}

	content := string(data)
	lines := strings.Split(content, "\n")

	// Filter out comments and empty lines
	var codeLines []string
	inMultilineComment := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		if strings.Contains(line, "/*") {
			inMultilineComment = true
		}
		if strings.Contains(line, "*/") {
			inMultilineComment = false
			continue
		}
		if inMultilineComment {
			continue
		}
		if strings.HasPrefix(trimmed, "//") {
			continue
		}

		codeLines = append(codeLines, trimmed)
	}

	// If file is very short and all lines are exports, it's a re-export module
	if len(codeLines) <= 10 && len(codeLines) > 0 {
		for _, line := range codeLines {
			if !strings.HasPrefix(line, "export ") &&
				!strings.HasPrefix(line, "export{") &&
				!strings.HasPrefix(line, "'use ") &&
				!strings.HasPrefix(line, "\"use ") {
				return false
			}
		}
		return true
	}

	return false
}

// shouldSkipTestRequirement checks if a file should be excluded from test requirements
func shouldSkipTestRequirement(filePath string) bool {
	pathLower := strings.ToLower(filePath)

	// Check if changes are type-only
	if isTypeOnlyChange(filePath) {
		return true
	}

	// Skip mock files
	if strings.Contains(filePath, "/__mocks__/") || strings.Contains(filePath, "\\__mocks__\\") {
		return true
	}

	// Skip testing utility files
	if strings.Contains(filePath, "/testing/") || strings.Contains(filePath, "\\testing\\") {
		return true
	}

	// Skip type definition files in types/ folders
	if strings.Contains(filePath, "/types/") || strings.Contains(filePath, "\\types\\") {
		return true
	}

	// Skip .types.ts files (type definition files)
	if strings.HasSuffix(filePath, ".types.ts") || strings.HasSuffix(filePath, ".types.tsx") {
		return true
	}

	// Skip root layout files
	if strings.HasSuffix(filePath, "_layout.tsx") || strings.HasSuffix(filePath, "_layout.ts") {
		return true
	}

	// Skip auth-related UI components
	authUIPatterns := []string{"social-connections", "sign-in-form", "oauth-callback"}
	for _, pattern := range authUIPatterns {
		if strings.Contains(pathLower, pattern) {
			return true
		}
	}

	// Skip config files
	configPatterns := []string{
		"jest.config", "vitest.config", "babel.config",
		"metro.config", "tailwind.config", "postcss.config",
		"eslint.config", "prettier.config", "tsconfig",
	}
	for _, pattern := range configPatterns {
		if strings.Contains(pathLower, pattern) {
			return true
		}
	}

	// Skip type declaration files
	if strings.HasSuffix(filePath, ".d.ts") {
		return true
	}

	// Skip setup files
	if strings.Contains(filePath, "jest.setup") || strings.Contains(filePath, "vitest.setup") {
		return true
	}

	// Skip validator/constant files
	if strings.Contains(filePath, "/lib/validators") || strings.Contains(filePath, "/lib/constants") {
		return true
	}

	// Skip re-export modules
	if isReexportModule(filePath) {
		return true
	}

	return false
}

func main() {
	var input hookInput
	if err := json.NewDecoder(os.Stdin).Decode(&input); err != nil {
		os.Exit(exitAllow)
	}

	command := input.ToolInput.Command

	// Only intercept git commit
	if !isGitCommit(command) {
		os.Exit(exitAllow)
	}

	// Block commits that add test files in __tests__ folders
	testsInFolders := getStagedTestsInTestsFolders()
	if len(testsInFolders) > 0 {
		msg := "\n❌ COMMIT BLOCKED - Tests in __tests__/ folders\n\n"
		msg += "Test files should be co-located with source files, not in __tests__/ folders.\n\n"
		msg += "Move these test files next to their source files:\n\n"
		for _, tf := range testsInFolders {
			msg += fmt.Sprintf("  • %s\n", tf)
			// Suggest the correct location
			dir := filepath.Dir(tf)
			parentDir := filepath.Dir(dir)
			base := filepath.Base(tf)
			suggested := filepath.Join(parentDir, base)
			msg += fmt.Sprintf("    → Move to: %s\n\n", suggested)
		}
		fmt.Fprint(os.Stderr, msg)
		os.Exit(exitBlock)
	}

	sessionID := input.SessionID
	if sessionID == "" {
		sessionID = "unknown"
	}

	cwd := input.Cwd
	if cwd == "" {
		cwd, _ = os.Getwd()
	}

	// HYBRID APPROACH: Git as source of truth + Session as scope filter
	// 1. Get files staged for commit (git is always accurate)
	stagedFiles := getGitStagedFiles(cwd)
	if len(stagedFiles) == 0 {
		// No staged files, allow commit
		os.Exit(exitAllow)
	}

	// 2. Load session data
	session := loadSessionData(sessionID)

	// 3. Clean stale entries (self-healing: removes renamed/deleted files)
	cleanedSession := cleanStaleEntries(session)

	// 4. Save cleaned session data if anything changed
	if len(cleanedSession.SourceFiles) != len(session.SourceFiles) ||
		len(cleanedSession.TestFiles) != len(session.TestFiles) {
		_ = saveSessionData(sessionID, cleanedSession)
	}

	// 5. Intersect: only enforce on files that are BOTH staged AND in session
	// This means we only check files Claude touched that you're committing
	sourceFiles := intersectFiles(stagedFiles, cleanedSession.SourceFiles)

	// Build test files map from session
	testFilesEdited := make(map[string]bool)
	for _, tf := range cleanedSession.TestFiles {
		testFilesEdited[tf] = true
	}

	// Also track which test files are being staged
	stagedFilesSet := make(map[string]bool)
	for _, sf := range stagedFiles {
		stagedFilesSet[sf] = true
	}

	if len(sourceFiles) == 0 {
		// No Claude-touched source files being committed, allow commit
		os.Exit(exitAllow)
	}

	// Check for missing tests
	var missingTests []struct {
		source   string
		expected string
	}
	testsToRun := map[string][]string{
		"backend": {},
		"mobile":  {},
		"web":     {},
		"portal":  {},
	}

	for _, sourceFile := range sourceFiles {
		// Skip files that don't need tests
		if shouldSkipTestRequirement(sourceFile) {
			continue
		}

		expectedTest := getTestPathForSource(sourceFile)
		if expectedTest == "" {
			continue
		}

		projectType := getProjectType(sourceFile)
		if projectType == "" {
			continue
		}

		// Check if test exists (multiple ways to satisfy):
		// 1. Test file is in session's test_files list
		// 2. Test file is being staged in this commit
		// 3. Test file exists on disk
		testExists := testFilesEdited[expectedTest]
		if !testExists {
			// Check if any edited test file contains the expected test path
			for tf := range testFilesEdited {
				if strings.Contains(tf, expectedTest) {
					testExists = true
					break
				}
			}
		}
		if !testExists {
			// Check if test file is being staged in this commit
			testExists = stagedFilesSet[expectedTest]
		}
		if !testExists {
			// Check if test file exists on disk
			testExists = checkTestExists(expectedTest, cwd)
		}

		if !testExists {
			missingTests = append(missingTests, struct {
				source   string
				expected string
			}{sourceFile, expectedTest})
		} else {
			// Find the actual test path
			actualTestPath := expectedTest
			for tf := range testFilesEdited {
				if strings.Contains(tf, expectedTest) {
					actualTestPath = tf
					break
				}
			}
			testsToRun[projectType] = append(testsToRun[projectType], actualTestPath)
		}
	}

	// Block if tests are missing
	if len(missingTests) > 0 {
		msg := "\n❌ COMMIT BLOCKED - Missing test files\n\n"
		msg += "Source files edited without corresponding tests:\n\n"
		for _, mt := range missingTests {
			msg += fmt.Sprintf("  • %s\n", mt.source)
			msg += fmt.Sprintf("    Expected: %s\n\n", mt.expected)
		}
		msg += "Create the test files before committing.\n"
		fmt.Fprint(os.Stderr, msg)
		os.Exit(exitBlock)
	}

	// Run tests for each project type
	allPassed := true
	var testOutput strings.Builder

	for projectType, testList := range testsToRun {
		if len(testList) == 0 {
			continue
		}

		// Find project root from first test file
		sampleFile := testList[0]
		projectRoot := findProjectRoot(sampleFile)

		if projectRoot == "" {
			// Try to infer from cwd
			if strings.Contains(cwd, "packages/backend") {
				projectRoot = cwd
				for filepath.Base(projectRoot) != "backend" {
					projectRoot = filepath.Dir(projectRoot)
					if projectRoot == "/" {
						projectRoot = ""
						break
					}
				}
			} else if strings.Contains(cwd, "apps/mobile") {
				projectRoot = cwd
				for filepath.Base(projectRoot) != "mobile" {
					projectRoot = filepath.Dir(projectRoot)
					if projectRoot == "/" {
						projectRoot = ""
						break
					}
				}
			} else if strings.Contains(cwd, "apps/web") {
				projectRoot = cwd
				for filepath.Base(projectRoot) != "web" {
					projectRoot = filepath.Dir(projectRoot)
					if projectRoot == "/" {
						projectRoot = ""
						break
					}
				}
			} else if strings.Contains(cwd, "apps/portal") {
				projectRoot = cwd
				for filepath.Base(projectRoot) != "portal" {
					projectRoot = filepath.Dir(projectRoot)
					if projectRoot == "/" {
						projectRoot = ""
						break
					}
				}
			}

			if projectRoot == "" {
				continue
			}
		}

		passed, output := runTests(testList, projectType, projectRoot)
		testOutput.WriteString(output)

		if !passed {
			allPassed = false
		}
	}

	if !allPassed {
		msg := "\n❌ COMMIT BLOCKED - Tests failing\n\n"
		msg += "Fix the failing tests before committing:\n\n"
		msg += testOutput.String()
		fmt.Fprint(os.Stderr, msg)
		os.Exit(exitBlock)
	}

	// All tests pass
	totalTests := 0
	for _, tests := range testsToRun {
		totalTests += len(tests)
	}
	if totalTests > 0 {
		fmt.Fprintf(os.Stderr, "✅ %d test file(s) passed\n", totalTests)
	}

	os.Exit(exitAllow)
}
