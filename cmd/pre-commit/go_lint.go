package main

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// checkGoLint runs Go linting on staged Go files
func checkGoLint(stagedFiles []string, config GoLintConfig) error {
	// Filter to only .go files matching configured paths
	goFiles := filterGoFiles(stagedFiles, config.Paths)
	if len(goFiles) == 0 {
		return nil
	}

	// Determine which directories have Go files
	dirs := collectGoDirectories(goFiles, config.Paths)
	if len(dirs) == 0 {
		return nil
	}

	// Determine which tool to use
	tool := config.Tool
	if tool == "" {
		tool = "golangci-lint"
	}

	// Check if preferred tool is available, fall back to go vet
	if tool == "golangci-lint" && !hasCommand("golangci-lint") {
		fmt.Println("   golangci-lint not found, falling back to go vet")
		tool = "go-vet"
	}

	// Run linter in each directory
	var lintErrors []string
	for _, dir := range dirs {
		if !compactMode() {
			fmt.Printf("   Linting Go code in %s...\n", dir)
		}

		var err error
		if compactMode() {
			if tool == "golangci-lint" {
				_, err = runCommandCapturedInDir(dir, "golangci-lint", "run", "./...")
			} else {
				_, err = runCommandCapturedInDir(dir, "go", "vet", "./...")
			}
		} else {
			if tool == "golangci-lint" {
				err = runCommandInDir(dir, "golangci-lint", "run", "./...")
			} else {
				err = runCommandInDir(dir, "go", "vet", "./...")
			}
		}

		if err != nil {
			lintErrors = append(lintErrors, fmt.Sprintf("%s: %v", dir, err))
		}
	}

	if len(lintErrors) > 0 {
		return fmt.Errorf("Go lint failed:\n  %s", strings.Join(lintErrors, "\n  "))
	}

	return nil
}

// filterGoFiles filters staged files to only .go files matching configured paths
func filterGoFiles(files []string, paths []string) []string {
	var result []string
	for _, file := range files {
		if !strings.HasSuffix(file, ".go") {
			continue
		}

		// If no paths configured, include all .go files
		if len(paths) == 0 {
			result = append(result, file)
			continue
		}

		// Check if file matches any configured path
		for _, path := range paths {
			// Normalize path to ensure consistent matching
			normalizedPath := strings.TrimSuffix(path, "/")
			if strings.HasPrefix(file, normalizedPath+"/") || file == normalizedPath {
				result = append(result, file)
				break
			}
		}
	}
	return result
}

// collectGoDirectories extracts unique directories from Go files that match configured paths
func collectGoDirectories(goFiles []string, paths []string) []string {
	dirSet := make(map[string]bool)

	for _, file := range goFiles {
		// Find which configured path this file belongs to
		for _, path := range paths {
			normalizedPath := strings.TrimSuffix(path, "/")
			if strings.HasPrefix(file, normalizedPath+"/") {
				dirSet[normalizedPath] = true
				break
			}
		}
	}

	// If no paths configured but we have Go files, use the file directories
	if len(paths) == 0 && len(goFiles) > 0 {
		for _, file := range goFiles {
			dir := filepath.Dir(file)
			dirSet[dir] = true
		}
	}

	var dirs []string
	for dir := range dirSet {
		dirs = append(dirs, dir)
	}
	return dirs
}

// hasCommand checks if a command is available in PATH
func hasCommand(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}
