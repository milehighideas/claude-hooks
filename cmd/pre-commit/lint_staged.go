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

	pm := cfg.PackageManager
	if pm == "" {
		pm = "pnpm"
	}

	// Build command based on package manager
	var cmd string
	var args []string
	if pm == "bun" {
		cmd = "bunx"
		args = []string{"lint-staged", "--no-stash"}
	} else {
		cmd = pm
		args = []string{"exec", "lint-staged", "--no-stash"}
	}

	if compactMode() {
		// Capture output instead of piping to terminal
		if _, err := runCommandCapturedWithEnv(cfg.Env, cmd, args...); err != nil {
			printStatus("Formatting", false, "lint-staged failed")
			return fmt.Errorf("lint-staged failed: %w", err)
		}
		printStatus("Formatting", true, "")
		return nil
	}

	if err := runCommandWithEnv(cfg.Env, cmd, args...); err != nil {
		return fmt.Errorf("lint-staged failed: %w", err)
	}
	fmt.Println("Formatting complete")
	fmt.Println()
	return nil
}
