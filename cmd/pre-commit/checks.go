package main

import (
	"bytes"
	"fmt"
	"strings"
	"sync"
)

// AppCheckResult holds the result of checking a single app
type AppCheckResult struct {
	AppName string
	Output  string
	Err     error
}

// runLintTypecheck orchestrates lint and typecheck for all affected apps IN PARALLEL
func runLintTypecheck(apps map[string]AppConfig, appFiles map[string][]string, sharedChanged bool, typecheckFilter TypecheckFilter, lintFilter LintFilter, fullLintOnCommit bool) error {
	if !compactMode() {
		fmt.Println("================================")
		fmt.Println("  LINT & TYPE CHECKING (PARALLEL)")
		fmt.Println("================================")
	}

	// Collect apps that need checking
	type appJob struct {
		name     string
		config   AppConfig
		files    []string
		full     bool // true = full checks, false = incremental
		skipLint      bool // skip lint for this app (typecheck still runs)
		skipTypecheck bool // skip typecheck for this app (lint still runs)
	}
	var jobs []appJob

	for appName, appConfig := range apps {
		files := appFiles[appName]

		if fullLintOnCommit {
			jobs = append(jobs, appJob{name: appName, config: appConfig, files: files, full: true, skipLint: appConfig.SkipLint, skipTypecheck: appConfig.SkipTypecheck})
		} else if sharedChanged || len(files) > 0 {
			jobs = append(jobs, appJob{name: appName, config: appConfig, files: files, full: sharedChanged, skipLint: appConfig.SkipLint, skipTypecheck: appConfig.SkipTypecheck})
		}
	}

	if len(jobs) == 0 {
		if !compactMode() {
			fmt.Println("No apps to check")
		}
		return nil
	}

	if !compactMode() {
		fmt.Printf("Running checks on %d app(s) in parallel...\n\n", len(jobs))
	}

	// Run all jobs in parallel
	var wg sync.WaitGroup
	results := make([]AppCheckResult, len(jobs))

	for i, job := range jobs {
		wg.Add(1)
		go func(idx int, j appJob) {
			defer wg.Done()

			var output bytes.Buffer
			var err error

			// Merge global typecheck filter with per-app override
			effectiveTypecheckFilter := GetTypecheckFilter(typecheckFilter, j.config.TypecheckFilter)

			if j.full {
				err = runFullChecksBuffered(j.name, j.config.Path, j.config.Filter, effectiveTypecheckFilter, lintFilter, j.config.NodeMemoryMB, j.skipLint, j.skipTypecheck, &output)
			} else {
				err = runIncrementalChecksBuffered(j.name, j.config.Path, j.config.Filter, j.files, effectiveTypecheckFilter, lintFilter, fullLintOnCommit, j.config.NodeMemoryMB, j.skipLint, j.skipTypecheck, &output)
			}

			results[idx] = AppCheckResult{
				AppName: j.name,
				Output:  output.String(),
				Err:     err,
			}
		}(i, job)
	}

	wg.Wait()

	// Print results sequentially
	checksFailed := false
	var failedApps []string
	for _, result := range results {
		if compactMode() {
			// Compact: show per-app one-liner
			if result.Err != nil {
				checksFailed = true
				failedApps = append(failedApps, result.AppName)
			}
		} else {
			if result.Output != "" {
				fmt.Print(result.Output)
				fmt.Println()
			}
			if result.Err != nil {
				checksFailed = true
			}
		}
	}

	if compactMode() {
		if checksFailed {
			detail := fmt.Sprintf("%s failed", strings.Join(failedApps, ", "))
			printStatus("Lint & typecheck", false, detail)
			printReportHint("lint/ and typecheck/")
			return fmt.Errorf("lint/typecheck failed")
		}
		printStatus("Lint & typecheck", true, fmt.Sprintf("%d apps", len(jobs)))
		return nil
	}

	if checksFailed {
		fmt.Println("================================")
		fmt.Println("  PRE-COMMIT CHECKS FAILED")
		fmt.Println("================================")
		fmt.Println()
		fmt.Println("Fix the errors above and try again")
		fmt.Println()
		return fmt.Errorf("lint/typecheck failed")
	}
	return nil
}

// filterLintableFiles filters to only .ts/.tsx/.js/.jsx files
func filterLintableFiles(files []string) []string {
	var lintFiles []string
	for _, f := range files {
		ext := strings.ToLower(f)
		if strings.HasSuffix(ext, ".ts") || strings.HasSuffix(ext, ".tsx") ||
			strings.HasSuffix(ext, ".js") || strings.HasSuffix(ext, ".jsx") {
			lintFiles = append(lintFiles, f)
		}
	}
	return lintFiles
}

func runFullChecks(appName, appPath, filter string, typecheckFilter TypecheckFilter, lintFilter LintFilter, nodeMemoryMB int, skipLint bool, skipTypecheck bool) error {
	fmt.Printf("🔍 Running full lint and typecheck for %s...\n", appName)

	var hasError bool

	// Run typecheck with filtering
	if skipTypecheck {
		fmt.Printf("   ⏩ %s typecheck skipped (skipTypecheck: true)\n", appName)
	} else {
		fmt.Printf("   → Starting typecheck for %s...\n", appName)
		if err := runFilteredTypecheck(appName, filter, typecheckFilter, nodeMemoryMB); err != nil {
			fmt.Printf("   ❌ %s typecheck failed\n", appName)
			hasError = true
		} else {
			fmt.Printf("   ✓ %s passed typecheck\n", appName)
		}
	}

	// Run lint with filtering (continue even if typecheck failed)
	if skipLint {
		fmt.Printf("   ⏩ %s lint skipped (skipLint: true)\n", appName)
	} else {
		fmt.Printf("   → Starting lint for %s...\n", appName)
		if err := runFilteredLint(appName, appPath, lintFilter); err != nil {
			fmt.Printf("   ❌ %s lint failed\n", appName)
			hasError = true
		} else {
			fmt.Printf("   ✓ %s passed lint\n", appName)
		}
	}

	if hasError {
		fmt.Printf("❌ %s had failures\n", appName)
		return fmt.Errorf("%s checks failed", appName)
	}

	fmt.Printf("✅ %s passed all checks\n", appName)
	return nil
}

// runFullChecksBuffered runs full checks and writes output to a buffer (for parallel execution)
func runFullChecksBuffered(appName, appPath, filter string, typecheckFilter TypecheckFilter, lintFilter LintFilter, nodeMemoryMB int, skipLint bool, skipTypecheck bool, output *bytes.Buffer) error {
	fmt.Fprintf(output, "🔍 Running full lint and typecheck for %s...\n", appName)

	var hasError bool

	// Run typecheck with filtering
	if skipTypecheck {
		fmt.Fprintf(output, "   ⏩ %s typecheck skipped (skipTypecheck: true)\n", appName)
	} else {
		fmt.Fprintf(output, "   → Starting typecheck for %s...\n", appName)
		typecheckOutput, typecheckErr := runFilteredTypecheckBuffered(appName, filter, typecheckFilter, nodeMemoryMB)
		output.WriteString(typecheckOutput)
		if typecheckErr != nil {
			fmt.Fprintf(output, "   ❌ %s typecheck failed\n", appName)
			hasError = true
		} else {
			fmt.Fprintf(output, "   ✓ %s passed typecheck\n", appName)
		}
	}

	// Run lint with filtering (continue even if typecheck failed)
	if skipLint {
		fmt.Fprintf(output, "   ⏩ %s lint skipped (skipLint: true)\n", appName)
	} else {
		fmt.Fprintf(output, "   → Starting lint for %s...\n", appName)
		lintOutput, lintErr := runFilteredLintBuffered(appName, appPath, lintFilter)
		output.WriteString(lintOutput)
		if lintErr != nil {
			fmt.Fprintf(output, "   ❌ %s lint failed\n", appName)
			hasError = true
		} else {
			fmt.Fprintf(output, "   ✓ %s passed lint\n", appName)
		}
	}

	if hasError {
		fmt.Fprintf(output, "❌ %s had failures\n", appName)
		return fmt.Errorf("%s checks failed", appName)
	}

	fmt.Fprintf(output, "✅ %s passed all checks\n", appName)
	return nil
}

func runIncrementalChecks(appName string, appPath string, filter string, files []string, typecheckFilter TypecheckFilter, lintFilter LintFilter, fullLintOnCommit bool, nodeMemoryMB int, skipLint bool, skipTypecheck bool) error {
	lintFiles := filterLintableFiles(files)

	if len(lintFiles) == 0 {
		fmt.Printf("   No lintable files in %s\n", appName)
		return nil
	}

	// When fullLintOnCommit is enabled, run full checks instead of incremental
	if fullLintOnCommit {
		fmt.Printf("🔍 Running full lint and typecheck for %s (fullLintOnCommit enabled)...\n", appName)
		return runFullChecks(appName, appPath, filter, typecheckFilter, lintFilter, nodeMemoryMB, skipLint, skipTypecheck)
	}

	fmt.Printf("🔍 Running incremental checks for %s (%d files)...\n", appName, len(lintFiles))

	// lint-staged already ran eslint --fix on staged files, so skip redundant lint
	// But we still need to run typecheck on the changed files

	// Run incremental typecheck
	if skipTypecheck {
		fmt.Printf("   ⏩ %s typecheck skipped (skipTypecheck: true)\n", appName)
	} else if err := runIncrementalTypecheck(appPath, lintFiles, typecheckFilter); err != nil {
		fmt.Printf("❌ %s incremental typecheck failed\n", appName)
		return err
	}

	fmt.Printf("✅ %s passed incremental checks\n", appName)
	return nil
}

// runIncrementalChecksBuffered runs incremental checks and writes output to a buffer (for parallel execution)
func runIncrementalChecksBuffered(appName string, appPath string, filter string, files []string, typecheckFilter TypecheckFilter, lintFilter LintFilter, fullLintOnCommit bool, nodeMemoryMB int, skipLint bool, skipTypecheck bool, output *bytes.Buffer) error {
	lintFiles := filterLintableFiles(files)

	if len(lintFiles) == 0 {
		fmt.Fprintf(output, "   No lintable files in %s\n", appName)
		return nil
	}

	// When fullLintOnCommit is enabled, run full checks instead of incremental
	if fullLintOnCommit {
		fmt.Fprintf(output, "🔍 Running full lint and typecheck for %s (fullLintOnCommit enabled)...\n", appName)
		return runFullChecksBuffered(appName, appPath, filter, typecheckFilter, lintFilter, nodeMemoryMB, skipLint, skipTypecheck, output)
	}

	fmt.Fprintf(output, "🔍 Running incremental checks for %s (%d files)...\n", appName, len(lintFiles))

	// lint-staged already ran eslint --fix on staged files, so skip redundant lint
	// But we still need to run typecheck on the changed files

	// Run incremental typecheck
	if skipTypecheck {
		fmt.Fprintf(output, "   ⏩ %s typecheck skipped (skipTypecheck: true)\n", appName)
	} else {
		typecheckOutput, err := runIncrementalTypecheckBuffered(appPath, lintFiles, typecheckFilter)
		output.WriteString(typecheckOutput)
		if err != nil {
			fmt.Fprintf(output, "❌ %s incremental typecheck failed\n", appName)
			return err
		}
	}

	fmt.Fprintf(output, "✅ %s passed incremental checks\n", appName)
	return nil
}
