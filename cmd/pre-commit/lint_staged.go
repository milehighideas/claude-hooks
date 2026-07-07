package main

import "fmt"

// runLintStaged runs lint-staged for file formatting
func runLintStaged(cfg LintStagedConfig) error {
	if !compactMode() {
		fmt.Println("================================")
		fmt.Println("  FORMATTING")
		fmt.Println("================================")
		fmt.Println("Running lint-staged...")
	}

	// Run the project's installed lint-staged (never a bunx/npx-fetched one). A
	// missing tool fails the commit rather than silently skipping formatting.
	bin, ok := resolveNodeBin(".", "lint-staged")
	if !ok {
		if compactMode() {
			printStatus("Formatting", false, "lint-staged not installed")
		}
		return fmt.Errorf("lint-staged is not installed — run your install and retry")
	}
	args := []string{"--no-stash"}

	if compactMode() {
		// Capture output instead of piping to terminal
		if _, err := runCommandCapturedWithEnv(cfg.Env, bin, args...); err != nil {
			printStatus("Formatting", false, "lint-staged failed")
			return fmt.Errorf("lint-staged failed: %w", err)
		}
		printStatus("Formatting", true, "")
		return nil
	}

	if err := runCommandWithEnv(cfg.Env, bin, args...); err != nil {
		return fmt.Errorf("lint-staged failed: %w", err)
	}
	fmt.Println("Formatting complete")
	fmt.Println()
	return nil
}
