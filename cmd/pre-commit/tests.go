package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
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
	failureCounts := make(map[string]int) // appName -> number of failed tests
	retriedApps := make(map[string]int)   // appName -> retry attempts that ultimately passed

	for appName, appConfig := range appsToTest {
		// Use custom test command if specified, otherwise default to "test"
		testCmd := appConfig.TestCommand
		if testCmd == "" {
			testCmd = "test"
		}

		// Build args based on package manager
		var args []string
		if pm == "bun" {
			args = []string{"--filter", appConfig.Filter, testCmd}
		} else {
			args = []string{"--filter", appConfig.Filter, testCmd}
		}

		// Append per-app test args (e.g., --watchman=false for Jest)
		if len(appConfig.TestArgs) > 0 {
			args = append(args, "--")
			args = append(args, appConfig.TestArgs...)
		}

		// Per-app start line so each app's run shows up live in compact mode
		// instead of disappearing into the long Tests phase.
		appCheck := "Tests " + appName
		printStart(appCheck)

		if compactMode() {
			// Capture output and write to report file
			output, err := runCommandCapturedWithEnv(ctx.Env, pm, args...)

			// Retry policy: TestConfig.Retries gives every failed run a
			// chance to recover from environmental flake. Files listed in
			// TestConfig.FlakyTestFiles get one extra retry beyond that,
			// but only when the failure output mentions one of them — a
			// real regression in non-quarantined code still fails fast.
			retries := ctx.Config.Retries
			if err != nil && hasFlakyFailure(output, ctx.Config.FlakyTestFiles) {
				retries++
			}
			retryAttempts := 0
			for err != nil && retryAttempts < retries {
				retryAttempts++
				output, err = runCommandCapturedWithEnv(ctx.Env, pm, args...)
			}

			writeTestReport(appName, output, err, reportDir)
			if err != nil {
				failedApps = append(failedApps, appName)
				failureCounts[appName] = parseTestFailureCount(output)
				printStatus(appCheck, false, fmt.Sprintf("%d failed", failureCounts[appName]))
			} else {
				if retryAttempts > 0 {
					retriedApps[appName] = retryAttempts
					printStatus(appCheck, true, fmt.Sprintf("after %d retry", retryAttempts))
				} else {
					printStatus(appCheck, true, "")
				}
				passedApps = append(passedApps, appName)
			}
		} else {
			fmt.Printf("\nRunning %s tests (command: %s)...\n", appName, testCmd)
			err := runCommandWithEnv(ctx.Env, pm, args...)
			retries := ctx.Config.Retries
			retryAttempts := 0
			for err != nil && retryAttempts < retries {
				retryAttempts++
				fmt.Printf("\nRetrying %s tests (attempt %d/%d)...\n", appName, retryAttempts, retries)
				err = runCommandWithEnv(ctx.Env, pm, args...)
			}
			if err != nil {
				fmt.Printf("%s tests failed\n", appName)
				return fmt.Errorf("%s tests failed", appName)
			}
			if retryAttempts > 0 {
				fmt.Printf("%s tests passed (after %d retry)\n", appName, retryAttempts)
			} else {
				fmt.Printf("%s tests passed\n", appName)
			}
		}
	}

	if compactMode() {
		if len(failedApps) > 0 {
			parts := make([]string, len(failedApps))
			for i, app := range failedApps {
				if count, ok := failureCounts[app]; ok && count > 0 {
					parts[i] = fmt.Sprintf("%s %d failed", app, count)
				} else {
					parts[i] = app + " failed"
				}
			}
			printStatus("Tests", false, strings.Join(parts, ", "))
			printReportHint("tests/")
			return fmt.Errorf("%s tests failed", strings.Join(failedApps, ", "))
		}
		summary := strings.Join(passedApps, ", ")
		if len(retriedApps) > 0 {
			retryParts := make([]string, 0, len(retriedApps))
			for app, n := range retriedApps {
				word := "retry"
				if n != 1 {
					word = "retries"
				}
				retryParts = append(retryParts, fmt.Sprintf("%s after %d %s", app, n, word))
			}
			summary += " — recovered: " + strings.Join(retryParts, ", ")
		}
		printStatus("Tests", true, summary)
	}

	return nil
}

// hasFlakyFailure reports whether any of the configured flaky-test files
// appears in the test runner's failure output. We match against the file
// path because both Vitest and Jest print failed test specs as
// "FAIL <relative-path-to-test-file>". This lets us widen the retry
// budget *only* when the failure is in a quarantined file; failures
// elsewhere fail fast under the normal Retries policy.
func hasFlakyFailure(output string, flakyTestFiles []string) bool {
	if len(flakyTestFiles) == 0 {
		return false
	}
	for _, p := range flakyTestFiles {
		if strings.Contains(output, p) {
			return true
		}
	}
	return false
}

// testFailurePatterns matches the "Tests" summary line from both Vitest and Jest output.
// Vitest: "Tests  111 failed | 3878 passed"
// Jest:   "Tests:       4 failed, 965 passed, 969 total"
var testFailurePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?m)Tests[:\s]+(\d+)\s+failed`),
}

// parseTestFailureCount extracts the number of failed tests from test runner output.
// Returns 0 if the count cannot be determined.
func parseTestFailureCount(output string) int {
	for _, re := range testFailurePatterns {
		if m := re.FindStringSubmatch(output); len(m) > 1 {
			if n, err := strconv.Atoi(m[1]); err == nil {
				return n
			}
		}
	}
	return 0
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
