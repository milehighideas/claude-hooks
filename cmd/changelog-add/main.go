package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

var validTypes = map[string]bool{
	"feat":     true,
	"fix":      true,
	"chore":    true,
	"docs":     true,
	"test":     true,
	"style":    true,
	"refactor": true,
	"perf":     true,
	"build":    true,
	"ci":       true,
	"revert":   true,
}

// AppConfig represents an app configuration from .pre-commit.json
type AppConfig struct {
	Path   string `json:"path"`
	Filter string `json:"filter"`
}

// ChangelogConfig represents the changelog configuration
type ChangelogConfig struct {
	// Mode determines how changelogs are organized:
	// - "global": all changelogs go to root .changelog/ (default)
	// - "per-app": changelogs routed to app .changelog/ based on scope
	// - "required": scope must match a configured app, error otherwise
	Mode string `json:"mode"`
	// Apps lists which apps have changelog support (optional, defaults to all apps)
	Apps []string `json:"apps"`
}

// PreCommitConfig represents the .pre-commit.json structure
type PreCommitConfig struct {
	Apps      map[string]AppConfig `json:"apps"`
	Changelog ChangelogConfig      `json:"changelog"`
}

// parseConventionalCommit parses a conventional commit message.
// Returns (type, scope, description, error)
func parseConventionalCommit(entry string) (string, string, string, error) {
	// Pattern: type(scope): description or type: description
	pattern := regexp.MustCompile(`(?i)^([a-z]+)(?:\(([^)]+)\))?: (.+)$`)
	match := pattern.FindStringSubmatch(strings.TrimSpace(entry))

	if match == nil {
		return "", "", "", fmt.Errorf("invalid format. Expected: 'type(scope): description' or 'type: description'")
	}

	commitType := strings.ToLower(match[1])
	scope := match[2] // May be empty
	description := strings.TrimSpace(match[3])

	if !validTypes[commitType] {
		types := make([]string, 0, len(validTypes))
		for t := range validTypes {
			types = append(types, t)
		}
		sort.Strings(types)
		return "", "", "", fmt.Errorf("invalid type '%s'. Valid types: %s", commitType, strings.Join(types, ", "))
	}

	return commitType, scope, description, nil
}

// sanitizeFilename converts text to a safe filename slug.
func sanitizeFilename(text string, maxLength int) string {
	slug := strings.ToLower(strings.TrimSpace(text))

	var result strings.Builder
	for _, c := range slug {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' {
			result.WriteRune(c)
		} else {
			result.WriteRune('-')
		}
	}
	slug = result.String()

	for strings.Contains(slug, "--") {
		slug = strings.ReplaceAll(slug, "--", "-")
	}

	slug = strings.Trim(slug, "-")

	if len(slug) > maxLength {
		slug = strings.TrimRight(slug[:maxLength], "-")
	}

	return slug
}

// findProjectRoot finds the monorepo root by looking for .pre-commit.json or pnpm-workspace.yaml
func findProjectRoot() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	current := cwd
	for {
		// Check for .pre-commit.json (preferred)
		if _, err := os.Stat(filepath.Join(current, ".pre-commit.json")); err == nil {
			return current, nil
		}
		// Check for pnpm-workspace.yaml (monorepo root)
		if _, err := os.Stat(filepath.Join(current, "pnpm-workspace.yaml")); err == nil {
			return current, nil
		}
		// Check for package.json with workspaces
		if _, err := os.Stat(filepath.Join(current, "package.json")); err == nil {
			return current, nil
		}

		parent := filepath.Dir(current)
		if parent == current {
			return cwd, nil
		}
		current = parent
	}
}

// loadConfig loads the full configuration from .pre-commit.json
func loadConfig(projectRoot string) (*PreCommitConfig, error) {
	configPath := filepath.Join(projectRoot, ".pre-commit.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}

	var config PreCommitConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	// Default mode to global if not specified
	if config.Changelog.Mode == "" {
		config.Changelog.Mode = "global"
	}

	return &config, nil
}

// getChangelogApps returns the apps that have changelog support
func getChangelogApps(config *PreCommitConfig) map[string]AppConfig {
	if config == nil {
		return nil
	}

	// If changelog.apps is specified, filter to only those apps
	if len(config.Changelog.Apps) > 0 {
		filtered := make(map[string]AppConfig)
		for _, name := range config.Changelog.Apps {
			if app, ok := config.Apps[name]; ok {
				filtered[name] = app
			}
		}
		return filtered
	}

	// Otherwise, all apps have changelog support
	return config.Apps
}

// resolveApp determines which app to use based on scope or explicit app flag
func resolveApp(apps map[string]AppConfig, scope string, explicitApp string) (string, AppConfig, error) {
	// If explicit app provided, use it
	if explicitApp != "" {
		if app, ok := apps[explicitApp]; ok {
			return explicitApp, app, nil
		}
		return "", AppConfig{}, fmt.Errorf("unknown app '%s'", explicitApp)
	}

	// Try to match scope to app name
	if scope != "" {
		scopeLower := strings.ToLower(scope)
		for name, app := range apps {
			if strings.ToLower(name) == scopeLower {
				return name, app, nil
			}
		}
	}

	// No match found
	return "", AppConfig{}, nil
}

// createFragment creates a new changelog fragment file.
func createFragment(entryText string, appName string, appPath string, projectRoot string) (string, error) {
	commitType, scope, description, err := parseConventionalCommit(entryText)
	if err != nil {
		return "", fmt.Errorf("%w\n\nExamples:\n  feat(native): add login functionality\n  fix(web): resolve navigation bug\n  chore(backend): update dependencies", err)
	}

	// Determine changelog directory
	var changelogDir string
	if appPath != "" {
		changelogDir = filepath.Join(projectRoot, appPath, ".changelog")
	} else {
		changelogDir = filepath.Join(projectRoot, ".changelog")
	}

	// Create .changelog directory if it doesn't exist
	if err := os.MkdirAll(changelogDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create .changelog directory: %w", err)
	}

	// Create .gitkeep
	gitkeepPath := filepath.Join(changelogDir, ".gitkeep")
	if _, err := os.Stat(gitkeepPath); os.IsNotExist(err) {
		if err := os.WriteFile(gitkeepPath, []byte{}, 0644); err != nil {
			return "", fmt.Errorf("failed to create .gitkeep: %w", err)
		}
	}

	// Generate timestamp-based filename
	timestamp := time.Now().Format("20060102-150405")

	descSlug := sanitizeFilename(description, 50)
	if descSlug == "" {
		descSlug = "entry"
	}

	var filename string
	if scope != "" {
		scopeSlug := sanitizeFilename(scope, 20)
		filename = fmt.Sprintf("%s-%s-%s-%s.txt", timestamp, commitType, scopeSlug, descSlug)
	} else {
		filename = fmt.Sprintf("%s-%s-%s.txt", timestamp, commitType, descSlug)
	}

	fragmentPath := filepath.Join(changelogDir, filename)

	content := strings.TrimSpace(entryText) + "\n"
	if err := os.WriteFile(fragmentPath, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("failed to write fragment: %w", err)
	}

	if rel, err := filepath.Rel(projectRoot, fragmentPath); err == nil {
		return rel, nil
	}
	return fragmentPath, nil
}

func printUsage(apps map[string]AppConfig, mode string) {
	fmt.Fprintln(os.Stderr, "Usage: changelog-add [--app <app>] 'type(scope): description'")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Creates a changelog fragment in the appropriate .changelog/ directory.")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Changelog modes (configured in .pre-commit.json):")
	fmt.Fprintln(os.Stderr, "  global    - All changelogs go to root .changelog/")
	fmt.Fprintln(os.Stderr, "  per-app   - Changelogs routed by scope (e.g., feat(native): ... → apps/native/.changelog/)")
	fmt.Fprintln(os.Stderr, "  required  - Scope must match an app, error otherwise")
	fmt.Fprintf(os.Stderr, "\nCurrent mode: %s\n", mode)
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Options:")
	fmt.Fprintln(os.Stderr, "  --app <name>  Explicitly specify the app (overrides scope detection)")
	fmt.Fprintln(os.Stderr, "  --global      Create fragment in root .changelog/ (overrides config mode)")
	fmt.Fprintln(os.Stderr, "  --list        List available apps")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Valid types: feat, fix, chore, docs, test, style, refactor, perf, build, ci, revert")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Examples:")
	fmt.Fprintln(os.Stderr, "  changelog-add 'feat(native): add login functionality'")
	fmt.Fprintln(os.Stderr, "  changelog-add 'fix(web): resolve navigation bug'")
	fmt.Fprintln(os.Stderr, "  changelog-add --app backend 'chore: update dependencies'")
	fmt.Fprintln(os.Stderr, "  changelog-add --global 'chore: update CI workflows'")

	if len(apps) > 0 {
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Available apps:")
		names := make([]string, 0, len(apps))
		for name := range apps {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			fmt.Fprintf(os.Stderr, "  %s (%s)\n", name, apps[name].Path)
		}
	}
}

func main() {
	appFlag := flag.String("app", "", "Explicitly specify the app")
	globalFlag := flag.Bool("global", false, "Create fragment in root .changelog/")
	listFlag := flag.Bool("list", false, "List available apps")
	helpFlag := flag.Bool("help", false, "Show help")
	flag.BoolVar(helpFlag, "h", false, "Show help")

	flag.Parse()

	projectRoot, err := findProjectRoot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to find project root: %v\n", err)
		os.Exit(1)
	}

	config, _ := loadConfig(projectRoot)
	apps := getChangelogApps(config)
	mode := "global"
	if config != nil {
		mode = config.Changelog.Mode
	}

	if *helpFlag {
		printUsage(apps, mode)
		os.Exit(0)
	}

	if *listFlag {
		if len(apps) == 0 {
			fmt.Println("No apps configured in .pre-commit.json")
			os.Exit(0)
		}
		fmt.Printf("Changelog mode: %s\n", mode)
		fmt.Println("Available apps:")
		names := make([]string, 0, len(apps))
		for name := range apps {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			fmt.Printf("  %s (%s)\n", name, apps[name].Path)
		}
		os.Exit(0)
	}

	args := flag.Args()
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "Error: No changelog entry provided")
		fmt.Fprintln(os.Stderr, "")
		printUsage(apps, mode)
		os.Exit(1)
	}

	entryText := strings.TrimSpace(args[0])
	if entryText == "" {
		fmt.Fprintln(os.Stderr, "Error: Changelog entry cannot be empty")
		os.Exit(1)
	}

	// Parse the entry to extract scope
	_, scope, _, err := parseConventionalCommit(entryText)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	var appName string
	var appPath string

	// --global flag always overrides config mode
	if *globalFlag {
		appName = ""
		appPath = ""
	} else {
		switch mode {
		case "global":
			// All changelogs go to root
			appName = ""
			appPath = ""

		case "per-app":
			// Route by scope, fall back to root if no match
			if apps != nil {
				resolvedName, resolvedApp, err := resolveApp(apps, scope, *appFlag)
				if err != nil {
					printAppError(err, apps)
					os.Exit(1)
				}
				if resolvedName != "" {
					appName = resolvedName
					appPath = resolvedApp.Path
				} else if scope != "" {
					fmt.Fprintf(os.Stderr, "Warning: scope '%s' doesn't match any configured app, using root .changelog/\n", scope)
				}
			}

		case "required":
			// Scope must match an app
			if apps == nil || len(apps) == 0 {
				fmt.Fprintln(os.Stderr, "Error: mode 'required' requires apps to be configured")
				os.Exit(1)
			}
			resolvedName, resolvedApp, err := resolveApp(apps, scope, *appFlag)
			if err != nil {
				printAppError(err, apps)
				os.Exit(1)
			}
			if resolvedName == "" {
				fmt.Fprintf(os.Stderr, "Error: mode 'required' requires scope to match an app\n")
				fmt.Fprintf(os.Stderr, "Scope '%s' doesn't match any configured app\n", scope)
				fmt.Fprintln(os.Stderr, "")
				fmt.Fprintln(os.Stderr, "Available apps:")
				names := make([]string, 0, len(apps))
				for name := range apps {
					names = append(names, name)
				}
				sort.Strings(names)
				for _, name := range names {
					fmt.Fprintf(os.Stderr, "  %s\n", name)
				}
				os.Exit(1)
			}
			appName = resolvedName
			appPath = resolvedApp.Path

		default:
			fmt.Fprintf(os.Stderr, "Error: unknown changelog mode '%s' (valid: global, per-app, required)\n", mode)
			os.Exit(1)
		}
	}

	fragmentPath, err := createFragment(entryText, appName, appPath, projectRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Created changelog fragment: %s\n", fragmentPath)
	fmt.Printf("   Entry: %s\n", entryText)
	if appName != "" {
		fmt.Printf("   App: %s\n", appName)
	}
}

func printAppError(err error, apps map[string]AppConfig) {
	fmt.Fprintf(os.Stderr, "Error: %v\n", err)
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Available apps:")
	names := make([]string, 0, len(apps))
	for name := range apps {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		fmt.Fprintf(os.Stderr, "  %s\n", name)
	}
}
