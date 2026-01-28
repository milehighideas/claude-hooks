package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Required folders for each feature
var requiredFolders = []string{
	"create",
	"read",
	"update",
	"delete",
	"hooks",
	"screens",
	"types",
	"utils",
}

// Required files for each folder
var requiredFiles = map[string]string{
	"index.ts": "Barrel export file",
	".gitkeep": "Git tracking file",
}

// ToolUseData represents the JSON input from Claude
type ToolUseData struct {
	ToolName  string                 `json:"tool_name"`
	ToolInput map[string]interface{} `json:"tool_input"`
}

// findProjectRoot finds the project root by looking for package.json
func findProjectRoot(startPath string) (string, error) {
	if startPath == "" {
		var err error
		startPath, err = os.Getwd()
		if err != nil {
			return "", err
		}
	}

	current, err := filepath.Abs(startPath)
	if err != nil {
		return "", err
	}

	for {
		packageJSON := filepath.Join(current, "package.json")
		if _, err := os.Stat(packageJSON); err == nil {
			return current, nil
		}

		parent := filepath.Dir(current)
		if parent == current {
			// Reached root without finding package.json
			return "", fmt.Errorf("package.json not found")
		}
		current = parent
	}
}

// checkFeatureStructure validates that a feature has the required folder structure
func checkFeatureStructure(featurePath string) []string {
	var issues []string
	featureName := filepath.Base(featurePath)

	// Check for required folders
	for _, folder := range requiredFolders {
		folderPath := filepath.Join(featurePath, folder)
		if info, err := os.Stat(folderPath); err != nil || !info.IsDir() {
			issues = append(issues, fmt.Sprintf("Missing required folder: %s/%s/", featureName, folder))
			continue
		}

		// Check for required files in each folder
		for filename, description := range requiredFiles {
			filePath := filepath.Join(folderPath, filename)
			if _, err := os.Stat(filePath); err != nil {
				issues = append(issues, fmt.Sprintf("Missing %s: %s/%s/%s", description, featureName, folder, filename))
			}
		}
	}

	// Check main barrel export
	mainIndex := filepath.Join(featurePath, "index.ts")
	if _, err := os.Stat(mainIndex); err != nil {
		issues = append(issues, fmt.Sprintf("Missing main barrel export: %s/index.ts", featureName))
	}

	// Check for loose component files directly in feature folder
	entries, err := os.ReadDir(featurePath)
	if err != nil {
		return issues
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			name := entry.Name()
			ext := filepath.Ext(name)
			if (ext == ".tsx" || ext == ".ts") && name != "index.ts" && name != "index.tsx" {
				issues = append(issues,
					fmt.Sprintf("Loose component file in %s/%s - move to appropriate CRUD folder (create/, read/, update/, delete/)",
						featureName, name))
			}
		}
	}

	return issues
}

// checkNoLooseComponents checks for loose .tsx files in components root
func checkNoLooseComponents(componentsDir string) []string {
	var issues []string

	if _, err := os.Stat(componentsDir); err != nil {
		return issues
	}

	entries, err := os.ReadDir(componentsDir)
	if err != nil {
		return issues
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			name := entry.Name()
			ext := filepath.Ext(name)
			if (ext == ".tsx" || ext == ".ts") && name != "index.ts" {
				issues = append(issues, fmt.Sprintf("Loose component file (must be in feature folder): components/%s", name))
			}
		}
	}

	return issues
}

// validateStructure validates the entire frontend structure
func validateStructure(projectRoot string) []string {
	var issues []string

	// Check for apps/web/components or components directory
	webComponents := filepath.Join(projectRoot, "apps", "web", "components")
	components := filepath.Join(projectRoot, "components")

	var componentsDir string
	if info, err := os.Stat(webComponents); err == nil && info.IsDir() {
		componentsDir = webComponents
	} else if info, err := os.Stat(components); err == nil && info.IsDir() {
		componentsDir = components
	} else {
		// No components directory found - this is fine, might be a different project
		return issues
	}

	// Check for loose components
	issues = append(issues, checkNoLooseComponents(componentsDir)...)

	// Check routes/ folder structure
	routesDir := filepath.Join(componentsDir, "routes")
	if info, err := os.Stat(routesDir); err == nil && info.IsDir() {
		entries, err := os.ReadDir(routesDir)
		if err == nil {
			for _, entry := range entries {
				if entry.IsDir() && !strings.HasPrefix(entry.Name(), ".") {
					featurePath := filepath.Join(routesDir, entry.Name())
					issues = append(issues, checkFeatureStructure(featurePath)...)
				}
			}
		}
	}

	// Check shared/ folder structure
	sharedDir := filepath.Join(componentsDir, "shared")
	if info, err := os.Stat(sharedDir); err == nil && info.IsDir() {
		issues = append(issues, checkFeatureStructure(sharedDir)...)
	}

	return issues
}

// isStructureModifyingOperation checks if the operation modifies component structure
func isStructureModifyingOperation(data ToolUseData) bool {
	toolName := data.ToolName

	// Only check Write and Edit operations in components/
	if toolName != "Write" && toolName != "Edit" && toolName != "Bash" {
		return false
	}

	// Check file paths for Write and Edit
	if toolName == "Write" || toolName == "Edit" {
		if filePath, ok := data.ToolInput["file_path"].(string); ok {
			if strings.Contains(filePath, "/components/") &&
				(strings.HasSuffix(filePath, ".tsx") || strings.HasSuffix(filePath, ".ts")) {
				return true
			}
		}
	}

	// Check bash commands that might CREATE structure (not delete)
	if toolName == "Bash" {
		if command, ok := data.ToolInput["command"].(string); ok {
			if strings.Contains(command, "/components/") {
				// Only check for create operations, not rm (deletes are fine)
				for _, op := range []string{"mkdir", "touch", "mv", "cp"} {
					if strings.Contains(command, op) {
						return true
					}
				}
			}
		}
	}

	return false
}

// loadProjectConfig sources .claude-hooks-config.sh if it exists
func loadProjectConfig() error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	configFile := filepath.Join(cwd, ".claude-hooks-config.sh")
	if _, err := os.Stat(configFile); err != nil {
		// Config file doesn't exist, that's fine
		return nil
	}

	cmd := exec.Command("/bin/bash", "-c", fmt.Sprintf("source %s && env", configFile))
	output, err := cmd.Output()
	if err != nil {
		return err
	}

	// Parse environment variables from output
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if parts := strings.SplitN(line, "=", 2); len(parts) == 2 {
			_ = os.Setenv(parts[0], parts[1])
		}
	}

	return nil
}

func main() {
	// Load project-specific config
	_ = loadProjectConfig() // Ignore errors, continue if config can't be loaded

	// OPT-IN ONLY: Validation only runs if project explicitly enables it
	if os.Getenv("CLAUDE_HOOKS_AST_VALIDATION") != "true" {
		os.Exit(0)
	}

	// Parse JSON from stdin
	var data ToolUseData
	decoder := json.NewDecoder(os.Stdin)
	if err := decoder.Decode(&data); err != nil {
		// Allow if we can't parse
		os.Exit(0)
	}

	// Only validate on structure-modifying operations
	if !isStructureModifyingOperation(data) {
		os.Exit(0)
	}

	// Find project root
	projectRoot, err := findProjectRoot("")
	if err != nil {
		// Can't find project, allow operation
		os.Exit(0)
	}

	// Validate structure
	issues := validateStructure(projectRoot)

	if len(issues) > 0 {
		msg := "BLOCKED: Frontend structure validation failed\n\n" +
			"The following issues were found with your frontend architecture:\n\n"

		for _, issue := range issues {
			msg += fmt.Sprintf("  - %s\n", issue)
		}

		msg += "\nRequired structure for each feature in components/routes/ and components/shared/:\n" +
			"  - All CRUD folders: create/, read/, update/, delete/\n" +
			"  - Other folders: hooks/, screens/, types/, utils/\n" +
			"  - Each folder must have: index.ts and .gitkeep\n" +
			"  - Main feature folder must have: index.ts\n\n" +
			"No loose .tsx files allowed in components/ root - use feature folders!\n\n" +
			"To fix:\n" +
			"1. Create missing folders and files\n" +
			"2. Move loose components to appropriate feature folders\n" +
			"3. Follow the frontend-architecture skill guidelines\n\n" +
			"See: ~/.claude/skills/frontend-architecture/SKILL.md\n"

		fmt.Fprint(os.Stderr, msg)
		os.Exit(2)
	}

	os.Exit(0)
}
