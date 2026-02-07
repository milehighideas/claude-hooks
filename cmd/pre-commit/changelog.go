// Package main provides the pre-commit binary for running various checks.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// checkChangelog verifies that a changelog fragment is staged when required.
// It checks the SKIP_CHANGELOG_CHECK env var, applies exclude patterns,
// and looks for changelog files based on the configured mode.
func checkChangelog(stagedFiles []string, excludePatterns []string, config ChangelogConfig, apps map[string]AppConfig) error {
	if os.Getenv("SKIP_CHANGELOG_CHECK") == "1" {
		return nil
	}

	// Filter out excluded files
	var relevantFiles []string
	for _, file := range stagedFiles {
		if file == "" {
			continue
		}

		excluded := false
		for _, pattern := range excludePatterns {
			matched, _ := regexp.MatchString(pattern, file)
			if matched {
				excluded = true
				break
			}
		}

		if !excluded {
			relevantFiles = append(relevantFiles, file)
		}
	}

	if len(relevantFiles) == 0 {
		if compactMode() {
			printStatus("Changelog", true, "no relevant files")
		}
		return nil
	}

	// Filter apps to only those with changelog support
	changelogApps := getChangelogApps(config, apps)

	switch config.Mode {
	case "per-app":
		return checkPerAppChangelog(stagedFiles, relevantFiles, changelogApps, true)
	case "required":
		return checkPerAppChangelog(stagedFiles, relevantFiles, changelogApps, false)
	default:
		return checkGlobalChangelog(stagedFiles, config.GlobalDir)
	}
}

// getChangelogApps returns the apps that have changelog support based on config
func getChangelogApps(config ChangelogConfig, apps map[string]AppConfig) map[string]AppConfig {
	if len(config.Apps) == 0 {
		return apps
	}

	filtered := make(map[string]AppConfig)
	for _, name := range config.Apps {
		if app, ok := apps[name]; ok {
			filtered[name] = app
		}
	}
	return filtered
}

// checkGlobalChangelog checks for a single changelog fragment in the global directory
func checkGlobalChangelog(stagedFiles []string, changelogDir string) error {
	fragmentCount := 0
	for _, file := range stagedFiles {
		if strings.HasPrefix(file, changelogDir+"/") && strings.HasSuffix(file, ".txt") {
			fragmentCount++
		}
	}

	if fragmentCount == 0 {
		fmt.Println("================================")
		fmt.Println("  CHANGELOG FRAGMENT REQUIRED")
		fmt.Println("================================")
		fmt.Println()
		fmt.Println("You're making changes that require a changelog entry.")
		fmt.Println()
		fmt.Println("   Add an entry using Conventional Commits format:")
		fmt.Println("   changelog-add 'type: description'")
		fmt.Println()
		fmt.Println("To skip this check temporarily, use:")
		fmt.Println("   SKIP_CHANGELOG_CHECK=1 git commit")
		fmt.Println()
		return fmt.Errorf("changelog fragment required")
	}

	fmt.Printf("Found %d staged changelog fragment(s)\n", fragmentCount)
	fmt.Println()

	return nil
}

// checkPerAppChangelog checks for changelog fragments in each affected app's directory
// If allowGlobalFallback is true, shared path changes can use global .changelog/ as fallback
func checkPerAppChangelog(stagedFiles, relevantFiles []string, apps map[string]AppConfig, allowGlobalFallback bool) error {
	// Determine which apps are affected by the changes
	affectedApps := make(map[string]bool)

	for _, file := range relevantFiles {
		for appName, appConfig := range apps {
			if strings.HasPrefix(file, appConfig.Path+"/") {
				affectedApps[appName] = true
				break
			}
		}
	}

	if len(affectedApps) == 0 {
		// Changes are in shared paths or outside apps
		if allowGlobalFallback {
			// per-app mode: accept any app's changelog or global
			return checkAnyAppChangelog(stagedFiles, apps)
		}
		// required mode: must have at least one app changelog (no global fallback)
		return checkAnyAppChangelogStrict(stagedFiles, apps)
	}

	// For each affected app, check if there's a changelog fragment
	var missingApps []string
	var foundApps []string

	for appName := range affectedApps {
		appConfig := apps[appName]
		changelogDir := filepath.Join(appConfig.Path, ".changelog")

		found := false
		for _, file := range stagedFiles {
			if strings.HasPrefix(file, changelogDir+"/") && strings.HasSuffix(file, ".txt") {
				found = true
				break
			}
		}

		if found {
			foundApps = append(foundApps, appName)
		} else {
			missingApps = append(missingApps, appName)
		}
	}

	if len(missingApps) > 0 {
		fmt.Println("================================")
		fmt.Println("  CHANGELOG FRAGMENT REQUIRED")
		fmt.Println("================================")
		fmt.Println()
		fmt.Println("You're making changes that require changelog entries for:")
		for _, app := range missingApps {
			appConfig := apps[app]
			fmt.Printf("   • %s (%s/.changelog/)\n", app, appConfig.Path)
		}
		fmt.Println()
		fmt.Println("Add entries using:")
		for _, app := range missingApps {
			fmt.Printf("   changelog-add --app %s 'type: description'\n", app)
		}
		fmt.Println()
		fmt.Println("To skip this check temporarily, use:")
		fmt.Println("   SKIP_CHANGELOG_CHECK=1 git commit")
		fmt.Println()
		return fmt.Errorf("changelog fragments required for %d app(s)", len(missingApps))
	}

	if len(foundApps) > 0 {
		fmt.Printf("Found changelog fragments for: %s\n", strings.Join(foundApps, ", "))
		fmt.Println()
	}

	return nil
}

// checkAnyAppChangelog checks if at least one app has a changelog fragment
// Used when changes are in shared paths (per-app mode with global fallback)
func checkAnyAppChangelog(stagedFiles []string, apps map[string]AppConfig) error {
	for _, appConfig := range apps {
		changelogDir := filepath.Join(appConfig.Path, ".changelog")
		for _, file := range stagedFiles {
			if strings.HasPrefix(file, changelogDir+"/") && strings.HasSuffix(file, ".txt") {
				return nil // Found at least one
			}
		}
	}

	// Also check global .changelog/ as fallback
	for _, file := range stagedFiles {
		if strings.HasPrefix(file, ".changelog/") && strings.HasSuffix(file, ".txt") {
			return nil
		}
	}

	fmt.Println("================================")
	fmt.Println("  CHANGELOG FRAGMENT REQUIRED")
	fmt.Println("================================")
	fmt.Println()
	fmt.Println("You're making changes to shared code that require a changelog entry.")
	fmt.Println()
	fmt.Println("Add an entry to any affected app:")
	fmt.Println("   changelog-add --app <app> 'type: description'")
	fmt.Println()
	fmt.Println("To skip this check temporarily, use:")
	fmt.Println("   SKIP_CHANGELOG_CHECK=1 git commit")
	fmt.Println()
	return fmt.Errorf("changelog fragment required for shared changes")
}

// checkAnyAppChangelogStrict checks if at least one app has a changelog fragment
// Used when changes are in shared paths (required mode - no global fallback)
func checkAnyAppChangelogStrict(stagedFiles []string, apps map[string]AppConfig) error {
	for _, appConfig := range apps {
		changelogDir := filepath.Join(appConfig.Path, ".changelog")
		for _, file := range stagedFiles {
			if strings.HasPrefix(file, changelogDir+"/") && strings.HasSuffix(file, ".txt") {
				return nil // Found at least one
			}
		}
	}

	fmt.Println("================================")
	fmt.Println("  CHANGELOG FRAGMENT REQUIRED")
	fmt.Println("================================")
	fmt.Println()
	fmt.Println("You're making changes to shared code that require a changelog entry.")
	fmt.Println()
	fmt.Println("Add an entry to one of the affected apps:")
	for appName := range apps {
		fmt.Printf("   changelog-add --app %s 'type: description'\n", appName)
	}
	fmt.Println()
	fmt.Println("To skip this check temporarily, use:")
	fmt.Println("   SKIP_CHANGELOG_CHECK=1 git commit")
	fmt.Println()
	return fmt.Errorf("changelog fragment required for shared changes")
}
