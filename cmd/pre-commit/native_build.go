package main

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"
)

// NativeBuildConfig configures native app compilation checks
type NativeBuildConfig struct {
	Apps []NativeBuildApp `json:"apps"`
}

// NativeBuildApp defines a native app and its build command
type NativeBuildApp struct {
	// Name is a human-readable label for the app (e.g., "iOS Maps")
	Name string `json:"name"`
	// Path is the app directory relative to repo root (used for file matching)
	Path string `json:"path"`
	// Dir is the working directory for the build command (relative to repo root)
	Dir string `json:"dir"`
	// Command is the build executable (e.g., "xcodebuild", "./gradlew")
	Command string `json:"command"`
	// Args are the arguments passed to the build command
	Args []string `json:"args"`
	// Extensions filters staged files — only trigger when these extensions change
	Extensions []string `json:"extensions"`
}

// checkNativeBuild runs compilation checks on native apps that have changed files.
// It follows the same pattern as checkGoLint: filter files by path/extension,
// then run the build command only for affected apps.
func checkNativeBuild(stagedFiles []string, config NativeBuildConfig) error {
	if len(config.Apps) == 0 {
		return nil
	}

	// Determine which apps have changed files
	var affectedApps []NativeBuildApp
	for _, app := range config.Apps {
		if hasMatchingFiles(stagedFiles, app.Path, app.Extensions) {
			affectedApps = append(affectedApps, app)
		}
	}

	if len(affectedApps) == 0 {
		return nil
	}

	// Run builds in parallel
	type buildResult struct {
		app    NativeBuildApp
		output string
		err    error
	}

	results := make(chan buildResult, len(affectedApps))
	var wg sync.WaitGroup

	for _, app := range affectedApps {
		wg.Add(1)
		go func(app NativeBuildApp) {
			defer wg.Done()

			if !compactMode() {
				fmt.Printf("   Compiling %s...\n", app.Name)
			}

			dir := app.Dir
			if dir == "" {
				dir = app.Path
			}

			// Always capture so we can persist the build output to a report,
			// even in verbose mode where it also streams to the terminal.
			output, err := runCommandCapturedInDir(dir, app.Command, app.Args...)
			if !compactMode() {
				fmt.Print(output)
			}

			results <- buildResult{app: app, output: output, err: err}
		}(app)
	}

	wg.Wait()
	close(results)

	var buildErrors []string
	var combined strings.Builder
	for result := range results {
		// Per-app report: <reportDir>/native-build/<app>/{findings,fullreport}.txt
		appFailed := result.err != nil
		appReport := result.output
		if appFailed && result.err != nil {
			appReport = fmt.Sprintf("%s\n\n%v", result.output, result.err)
		}
		_ = writeAppRunReport("native-build", appNameFromNative(result.app), "Native build: "+result.app.Name, appReport, appFailed)

		fmt.Fprintf(&combined, "===== %s =====\n%s\n", result.app.Name, result.output)
		if result.err != nil {
			buildErrors = append(buildErrors, fmt.Sprintf("%s: %v", result.app.Name, result.err))
		}
	}

	failed := len(buildErrors) > 0
	// Flat aggregate report across all affected native apps.
	_ = writeRunReport("native-build", "Native build", combined.String(), failed)

	if failed {
		return fmt.Errorf("native build failed:\n  %s", strings.Join(buildErrors, "\n  "))
	}

	return nil
}

// appNameFromNative derives a report-folder name from a native app, preferring
// the last path segment (e.g. "dashtag-maps-ios") for a stable, filesystem-safe
// label.
func appNameFromNative(app NativeBuildApp) string {
	if app.Path != "" {
		return filepath.Base(app.Path)
	}
	return strings.ReplaceAll(strings.ToLower(app.Name), " ", "-")
}

// hasMatchingFiles returns true if any staged file is under the given path
// and matches the configured extensions. If no extensions are configured,
// any file under the path counts.
func hasMatchingFiles(files []string, path string, extensions []string) bool {
	normalizedPath := strings.TrimSuffix(path, "/") + "/"

	for _, file := range files {
		if !strings.HasPrefix(file, normalizedPath) {
			continue
		}

		// If no extension filter, any file matches
		if len(extensions) == 0 {
			return true
		}

		// Check if file has a matching extension
		for _, ext := range extensions {
			if strings.HasSuffix(strings.ToLower(file), ext) {
				return true
			}
		}
	}

	return false
}
