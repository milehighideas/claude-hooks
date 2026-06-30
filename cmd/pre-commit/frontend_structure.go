package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// runFrontendStructureCheck runs the frontend structure validation by shelling
// out to the validate-frontend-structure binary (co-located in this monorepo).
func runFrontendStructureCheck(apps map[string]AppConfig, stagedFiles []string) error {
	if !compactMode() {
		fmt.Println("================================")
		fmt.Println("  FRONTEND STRUCTURE CHECK")
		fmt.Println("================================")
	}

	return runFrontendStructureBinary()
}

// runFrontendStructureBinary finds and executes the validate-frontend-structure binary.
// It looks next to the current executable first, then falls back to PATH.
func runFrontendStructureBinary() error {
	binName := "validate-frontend-structure"

	// Look for sibling binary in same directory as this executable
	self, err := os.Executable()
	if err == nil {
		sibling := filepath.Join(filepath.Dir(self), binName)
		if _, err := os.Stat(sibling); err == nil {
			binName = sibling
		}
	}

	// Always capture so the output can be persisted to a report; in verbose mode
	// it's also echoed to the terminal afterward.
	cmd := exec.Command(binName)
	cmd.Env = os.Environ()
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	runErr := cmd.Run()
	output := stripANSI(stdout.String() + stderr.String())

	if !compactMode() && output != "" {
		fmt.Print(output)
	}

	failed := false
	skipped := false
	if runErr != nil {
		if exitErr, ok := runErr.(*exec.ExitError); ok && exitErr.ExitCode() == 2 {
			failed = true
		} else {
			skipped = true // validator not found / non-2 exit
		}
	}

	_ = writeRunReport("frontend-structure", "Frontend structure", output, failed)

	if compactMode() {
		if failed {
			printStatus("Frontend structure", false, "")
			printReportHint("frontend-structure/")
			return fmt.Errorf("frontend structure validation failed")
		}
		if skipped {
			printStatus("Frontend structure", true, "skipped")
		} else {
			printStatus("Frontend structure", true, "")
		}
		return nil
	}

	if failed {
		return fmt.Errorf("frontend structure validation failed")
	}
	if skipped {
		fmt.Println("✅ Frontend structure check skipped (validator not found)")
		fmt.Println()
		return nil
	}
	fmt.Println("✅ Frontend structure check passed")
	fmt.Println()
	return nil
}

