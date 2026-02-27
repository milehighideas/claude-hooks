package main

import (
	"os/exec"
	"strings"
)

// getStagedFiles returns a list of staged files from git
// It only returns files that are Added, Copied, Modified, or Renamed
// Paths are relative to the current working directory
func getStagedFiles() ([]string, error) {
	cmd := exec.Command("git", "diff", "--cached", "--name-only", "--diff-filter=ACMR", "--relative")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	return parseStagedFiles(string(output)), nil
}

// parseStagedFiles parses git diff output into a slice of file paths
func parseStagedFiles(output string) []string {
	var files []string
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			files = append(files, line)
		}
	}
	return files
}

// getNewlyAddedFiles returns a set of files that are newly added (not modified) in the staging area.
func getNewlyAddedFiles() (map[string]bool, error) {
	cmd := exec.Command("git", "diff", "--cached", "--name-only", "--diff-filter=A", "--relative")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	result := make(map[string]bool)
	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			result[line] = true
		}
	}
	return result, nil
}

// categorizeFiles separates files into app-specific groups and detects shared path changes
// Returns:
//   - appFiles: map of app name to files belonging to that app
//   - sharedChanged: true if any file matches a shared path
func categorizeFiles(files []string, apps map[string]AppConfig, sharedPaths []string) (appFiles map[string][]string, sharedChanged bool) {
	appFiles = make(map[string][]string)

	for _, file := range files {
		categorized := false

		// Check if file belongs to any app
		for appName, appConfig := range apps {
			if strings.HasPrefix(file, appConfig.Path+"/") {
				appFiles[appName] = append(appFiles[appName], file)
				categorized = true
				break
			}
		}

		// If not in any app, check if it's in a shared path
		if !categorized {
			for _, sharedPath := range sharedPaths {
				if strings.HasPrefix(file, sharedPath) {
					sharedChanged = true
					break
				}
			}
		}
	}

	return appFiles, sharedChanged
}
