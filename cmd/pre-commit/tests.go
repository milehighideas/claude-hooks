package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// TestRunContext provides context for determining which apps to test
type TestRunContext struct {
	AllApps        map[string]AppConfig
	AffectedApps   map[string][]string // appName -> list of changed files
	SharedChanged  bool
	Config         TestConfig
	GlobalEnabled  bool              // Global tests feature flag (can be overridden per-app)
	PackageManager string            // Package manager to use (pnpm, bun, npm, yarn)
	Env            map[string]string // Environment variables for commands
}

// runTests runs tests based on configuration and affected files
func runTests(ctx TestRunContext) error {
	if !compactMode() {
		fmt.Println()
		fmt.Println("================================")
		fmt.Println("  TESTS")
		fmt.Println("================================")
	}

	appsToTest := determineAppsToTest(ctx)

	if len(appsToTest) == 0 {
		if !compactMode() {
			fmt.Println("No apps to test (all skipped or none affected)")
		}
		return nil
	}

	// Print what we're testing and why (verbose only)
	if !compactMode() {
		printTestPlan(ctx, appsToTest)
	}

	pm := ctx.PackageManager
	if pm == "" {
		pm = "pnpm"
	}

	var failedApps []string
	var passedApps []string

	for appName, appConfig := range appsToTest {
		// Use custom test command if specified, otherwise default to "test"
		testCmd := appConfig.TestCommand
		if testCmd == "" {
			testCmd = "test"
		}

		// Build args based on package manager
		var args []string
		if pm == "bun" {
			args = []string{"run", "--filter", appConfig.Filter, testCmd}
		} else {
			args = []string{"--filter", appConfig.Filter, testCmd}
		}

		// Append per-app test args (e.g., --watchman=false for Jest)
		if len(appConfig.TestArgs) > 0 {
			args = append(args, "--")
			args = append(args, appConfig.TestArgs...)
		}

		if compactMode() {
			// Capture output and write to report file
			output, err := runCommandCapturedWithEnv(ctx.Env, pm, args...)
			writeTestReport(appName, output, err, reportDir)
			if err != nil {
				failedApps = append(failedApps, appName)
			} else {
				passedApps = append(passedApps, appName)
			}
		} else {
			fmt.Printf("\nRunning %s tests (command: %s)...\n", appName, testCmd)
			if err := runCommandWithEnv(ctx.Env, pm, args...); err != nil {
				fmt.Printf("%s tests failed\n", appName)
				return fmt.Errorf("%s tests failed", appName)
			}
			fmt.Printf("%s tests passed\n", appName)
		}
	}

	if compactMode() {
		if len(failedApps) > 0 {
			printStatus("Tests", false, strings.Join(failedApps, ", ")+" failed")
			printReportHint("tests/")
			return fmt.Errorf("%s tests failed", strings.Join(failedApps, ", "))
		}
		printStatus("Tests", true, strings.Join(passedApps, ", "))
	}

	return nil
}

// writeTestReport writes test output to a report file
func writeTestReport(appName, output string, testErr error, baseDir string) {
	if baseDir == "" {
		return
	}

	testsDir := filepath.Join(baseDir, "tests")
	if err := os.MkdirAll(testsDir, 0755); err != nil {
		return
	}

	reportPath := filepath.Join(testsDir, appName+".txt")

	var sb strings.Builder
	sb.WriteString(strings.Repeat("=", 80) + "\n")
	sb.WriteString(fmt.Sprintf("TEST REPORT: %s\n", appName))
	sb.WriteString(fmt.Sprintf("Generated: %s\n", time.Now().Format("2006-01-02 15:04:05")))
	if testErr != nil {
		sb.WriteString("Result: FAILED\n")
	} else {
		sb.WriteString("Result: PASSED\n")
	}
	sb.WriteString(strings.Repeat("=", 80) + "\n\n")
	sb.WriteString(output)

	os.WriteFile(reportPath, []byte(sb.String()), 0644)
}

// determineAppsToTest returns the apps that should have tests run based on config and context
func determineAppsToTest(ctx TestRunContext) map[string]AppConfig {
	result := make(map[string]AppConfig)

	// Check if shared paths changed and we should run all tests
	runAllDueToShared := ctx.SharedChanged && *ctx.Config.RunOnSharedChanges

	for appName, appConfig := range ctx.AllApps {
		// Determine if tests are enabled for this app
		testsEnabled := ctx.GlobalEnabled // Start with global setting

		// Check per-app override
		if override, exists := ctx.Config.AppOverrides[appName]; exists {
			// Check enabled override first (takes precedence)
			if override.Enabled != nil {
				testsEnabled = *override.Enabled
			} else if override.Skip {
				// Legacy skip field (deprecated but still supported)
				testsEnabled = false
			}
		}

		// Skip if tests not enabled for this app
		if !testsEnabled {
			continue
		}

		// Check per-app onlyWhenAffected override
		if override, exists := ctx.Config.AppOverrides[appName]; exists {
			if override.OnlyWhenAffected != nil {
				if *override.OnlyWhenAffected {
					// Only run if this app is affected (or shared changed and runOnSharedChanges is true)
					_, isAffected := ctx.AffectedApps[appName]
					if !isAffected && !runAllDueToShared {
						continue
					}
				}
				// If onlyWhenAffected is explicitly false, always run this app's tests
				result[appName] = appConfig
				continue
			}
		}

		// Apply global affectedOnly setting
		if ctx.Config.AffectedOnly {
			_, isAffected := ctx.AffectedApps[appName]
			if !isAffected && !runAllDueToShared {
				continue
			}
		}

		result[appName] = appConfig
	}

	return result
}

// printTestPlan prints information about what tests will run and why
func printTestPlan(ctx TestRunContext, appsToTest map[string]AppConfig) {
	if ctx.SharedChanged && *ctx.Config.RunOnSharedChanges {
		fmt.Println("Shared paths changed - running tests for all non-skipped apps")
	} else if ctx.Config.AffectedOnly {
		fmt.Println("Running tests only for affected apps:")
		for appName := range appsToTest {
			if _, affected := ctx.AffectedApps[appName]; affected {
				fmt.Printf("  - %s (has staged changes)\n", appName)
			} else {
				fmt.Printf("  - %s (explicitly configured to always run)\n", appName)
			}
		}
	} else {
		fmt.Println("Running tests for all configured apps...")
	}

	// Print skipped apps
	for appName := range ctx.AllApps {
		if _, willTest := appsToTest[appName]; !willTest {
			if override, exists := ctx.Config.AppOverrides[appName]; exists && override.Skip {
				fmt.Printf("  - %s (skipped via config)\n", appName)
			} else if ctx.Config.AffectedOnly {
				fmt.Printf("  - %s (skipped - not affected)\n", appName)
			}
		}
	}
}
