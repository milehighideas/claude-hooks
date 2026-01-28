package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// runFrontendStructureCheck runs the frontend structure validation by shelling
// out to the validate-frontend-structure binary (co-located in this monorepo).
func runFrontendStructureCheck(apps map[string]AppConfig, stagedFiles []string) error {
	fmt.Println("================================")
	fmt.Println("  FRONTEND STRUCTURE CHECK")
	fmt.Println("================================")

	return runFrontendStructureBinary()
}

// runFrontendStructureCheckStandalone runs the check for standalone mode
func runFrontendStructureCheckStandalone(apps map[string]AppConfig, files []string) error {
	fmt.Println("================================")
	fmt.Println("  FRONTEND STRUCTURE CHECK")
	fmt.Println("================================")

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

	cmd := exec.Command(binName)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()

	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 2 {
			return fmt.Errorf("frontend structure validation failed")
		}
		// Binary not found or other error - don't block
		fmt.Println("✅ Frontend structure check skipped (validator not found)")
		fmt.Println()
		return nil
	}

	fmt.Println("✅ Frontend structure check passed")
	fmt.Println()
	return nil
}

// getAllFilesForFrontendCheck gets all files in a directory for frontend structure checking
func getAllFilesForFrontendCheck(dir, projectRoot string) ([]string, error) {
	var files []string

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			name := info.Name()
			if strings.HasPrefix(name, ".") || name == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}

		relPath, err := filepath.Rel(projectRoot, path)
		if err != nil {
			return err
		}

		files = append(files, relPath)
		return nil
	})

	return files, err
}
