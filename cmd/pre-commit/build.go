package main

import (
	"fmt"
)

// checkBuild runs build for configured apps
func checkBuild(config BuildConfig, apps map[string]AppConfig) error {
	if len(config.Apps) == 0 {
		return nil
	}

	for _, appName := range config.Apps {
		appConfig, ok := apps[appName]
		if !ok {
			return fmt.Errorf("app %q not found in configuration", appName)
		}

		if compactMode() {
			if _, err := runCommandCapturedInDir(appConfig.Path, "pnpm", "build"); err != nil {
				return fmt.Errorf("build failed for %s: %w", appName, err)
			}
		} else {
			fmt.Printf("Building %s...\n", appName)
			if err := runBuildInDir(appConfig.Path); err != nil {
				return fmt.Errorf("build failed for %s: %w", appName, err)
			}
			fmt.Printf("Build successful for %s\n", appName)
		}
	}

	return nil
}

// runBuildInDir runs pnpm build in the specified directory
func runBuildInDir(dir string) error {
	return runCommandInDir(dir, "pnpm", "build")
}
