package main

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

// checkBranchProtection checks if the current branch is in the protected branches list.
// If SKIP_BRANCH_PROTECTION env var is set, the check is bypassed.
// Returns an error if the current branch matches any protected branch pattern.
func checkBranchProtection(protectedBranches []string) error {
	if os.Getenv("SKIP_BRANCH_PROTECTION") != "" {
		return nil
	}

	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		return nil // Ignore errors, might not be in a git repo
	}

	branch := strings.TrimSpace(string(output))
	pattern := "^(" + strings.Join(protectedBranches, "|") + ")$"
	matched, _ := regexp.MatchString(pattern, branch)
	if matched {
		return fmt.Errorf("direct commits to the %s branch are not allowed. Please choose a new branch name", branch)
	}

	return nil
}
