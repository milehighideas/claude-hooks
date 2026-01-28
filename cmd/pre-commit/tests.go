package main

import "fmt"

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
	fmt.Println()
	fmt.Println("================================")
	fmt.Println("  TESTS")
	fmt.Println("================================")

	appsToTest := determineAppsToTest(ctx)

	if len(appsToTest) == 0 {
		fmt.Println("No apps to test (all skipped or none affected)")
		return nil
	}

	// Print what we're testing and why
	printTestPlan(ctx, appsToTest)

	pm := ctx.PackageManager
	if pm == "" {
		pm = "pnpm"
	}

	for appName, appConfig := range appsToTest {
		// Use custom test command if specified, otherwise default to "test"
		testCmd := appConfig.TestCommand
		if testCmd == "" {
			testCmd = "test"
		}

		fmt.Printf("\nRunning %s tests (command: %s)...\n", appName, testCmd)

		// Build args based on package manager
		var args []string
		if pm == "bun" {
			// bun uses: bun run --filter <name> <script>
			args = []string{"run", "--filter", appConfig.Filter, testCmd}
		} else {
			// pnpm/npm/yarn use: <pm> --filter <name> <script>
			args = []string{"--filter", appConfig.Filter, testCmd}
		}

		if err := runCommandWithEnv(ctx.Env, pm, args...); err != nil {
			fmt.Printf("%s tests failed\n", appName)
			return fmt.Errorf("%s tests failed", appName)
		}
		fmt.Printf("%s tests passed\n", appName)
	}

	return nil
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
