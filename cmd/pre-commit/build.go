package main

import (
	"fmt"
	"strings"
)

// checkBuild runs build for configured apps
func checkBuild(config BuildConfig, apps map[string]AppConfig) error {
	if len(config.Apps) == 0 {
		return nil
	}

	var combined strings.Builder
	var buildErrors []string
	for _, appName := range config.Apps {
		appConfig, ok := apps[appName]
		if !ok {
			return fmt.Errorf("app %q not found in configuration", appName)
		}

		if !compactMode() {
			fmt.Printf("Building %s...\n", appName)
		}
		out, err := runCommandCapturedInDir(appConfig.Path, "pnpm", "build")
		if !compactMode() && out != "" {
			fmt.Print(out)
		}

		appReport := out
		if err != nil {
			appReport = fmt.Sprintf("%s\n\n%v", out, err)
		}
		_ = writeAppRunReport("build-check", appName, "Build: "+appName, appReport, err != nil)
		fmt.Fprintf(&combined, "===== %s =====\n%s\n", appName, out)

		if err != nil {
			buildErrors = append(buildErrors, fmt.Sprintf("%s: %v", appName, err))
		} else if !compactMode() {
			fmt.Printf("Build successful for %s\n", appName)
		}
	}

	failed := len(buildErrors) > 0
	_ = writeRunReport("build-check", "Build check", combined.String(), failed)
	if failed {
		return fmt.Errorf("build failed: %s", strings.Join(buildErrors, ", "))
	}

	return nil
}
