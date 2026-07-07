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

	output, ok, err := runConvexDev(config.Path)
	if !ok {
		return fmt.Errorf("convex CLI is not installed at %s — run your install and retry", config.Path)
	}
	failed := err != nil || !strings.Contains(output, marker)
	_ = writeRunReport("convex-validation", "Convex validation", output, failed)

	if err != nil {
		// Include output in error for debugging
		return fmt.Errorf("convex dev failed: %w\nOutput: %s", err, output)
	}

	if !strings.Contains(output, marker) {
		return fmt.Errorf("convex validation failed: success marker %q not found in output\nOutput: %s", marker, output)
	}

	return nil
}

// runConvexDev runs `convex dev --once` using the project's installed Convex CLI
// (see resolveNodeBin), never a bunx/npx-fetched one. ok is false when convex
// isn't installed, so the caller fails the commit loudly rather than passing an
// unvalidated backend.
func runConvexDev(path string) (string, bool, error) {
	bin, ok := resolveNodeBin(path, "convex")
	if !ok {
		return "", false, nil
	}

	cmd := exec.Command(bin, "dev", "--once")
	cmd.Dir = path

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	// Combine stdout and stderr
	output := stdout.String() + stderr.String()

	return output, true, err
}
