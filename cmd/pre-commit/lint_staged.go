package main

import "fmt"

// runLintStaged runs lint-staged for file formatting
func runLintStaged(cfg LintStagedConfig) error {
	fmt.Println("================================")
	fmt.Println("  FORMATTING")
	fmt.Println("================================")
	fmt.Println("Running lint-staged...")

	pm := cfg.PackageManager
	if pm == "" {
		pm = "pnpm"
	}

	// Build command based on package manager
	var cmd string
	var args []string
	if pm == "bun" {
		// bun uses bunx to run local binaries
		cmd = "bunx"
		args = []string{"lint-staged", "--no-stash"}
	} else {
		// pnpm/npm/yarn use: <pm> exec lint-staged
		cmd = pm
		args = []string{"exec", "lint-staged", "--no-stash"}
	}

	if err := runCommandWithEnv(cfg.Env, cmd, args...); err != nil {
		return fmt.Errorf("lint-staged failed: %w", err)
	}
	fmt.Println("Formatting complete")
	fmt.Println()
	return nil
}
