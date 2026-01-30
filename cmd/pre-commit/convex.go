package main

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// DefaultConvexSuccessMarker is the default success message from Convex dev
const DefaultConvexSuccessMarker = "Convex functions ready!"

// checkConvex validates Convex functions compile
func checkConvex(config ConvexConfig) error {
	if config.Path == "" {
		return fmt.Errorf("convex path is required")
	}

	marker := config.SuccessMarker
	if marker == "" {
		marker = DefaultConvexSuccessMarker
	}

	output, err := runConvexDev(config.Path, config.PackageManager)
	if err != nil {
		// Include output in error for debugging
		return fmt.Errorf("convex dev failed: %w\nOutput: %s", err, output)
	}

	if !strings.Contains(output, marker) {
		return fmt.Errorf("convex validation failed: success marker %q not found in output\nOutput: %s", marker, output)
	}

	return nil
}

// runConvexDev runs convex dev --once using the configured package manager
func runConvexDev(path string, packageManager string) (string, error) {
	var runner string
	var args []string
	switch packageManager {
	case "bun":
		runner = "bunx"
		args = []string{"convex", "dev", "--once"}
	case "yarn":
		runner = "yarn"
		args = []string{"dlx", "convex", "dev", "--once"}
	default:
		runner = "npx"
		args = []string{"convex", "dev", "--once"}
	}

	cmd := exec.Command(runner, args...)
	cmd.Dir = path

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	// Combine stdout and stderr
	output := stdout.String() + stderr.String()

	return output, err
}
