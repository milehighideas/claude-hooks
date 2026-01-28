package main

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
)

// runCommand executes a command with stdout/stderr connected to os.Stdout/os.Stderr.
// It bypasses Go 1.19+ ErrDot security check for relative paths (e.g., node_modules/.bin).
func runCommand(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	// Bypass Go 1.19+ security check for relative paths (ErrDot)
	// This is safe because we trust node_modules/.bin executables
	if errors.Is(cmd.Err, exec.ErrDot) {
		cmd.Err = nil
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// runCommandWithEnv executes a command with custom environment variables.
// The env map is merged with the current environment (env vars take precedence).
func runCommandWithEnv(env map[string]string, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	if errors.Is(cmd.Err, exec.ErrDot) {
		cmd.Err = nil
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Start with current environment and add/override with custom vars
	if len(env) > 0 {
		cmd.Env = os.Environ()
		for k, v := range env {
			cmd.Env = append(cmd.Env, k+"="+v)
		}
	}

	return cmd.Run()
}

// runCommandInDir executes a command in a specific directory with stdout/stderr
// connected to os.Stdout/os.Stderr. It bypasses Go 1.19+ ErrDot security check.
func runCommandInDir(dir, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	// Bypass Go 1.19+ security check for relative paths (ErrDot)
	if errors.Is(cmd.Err, exec.ErrDot) {
		cmd.Err = nil
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// runCommandWithOutput executes a command and returns stdout as a string.
// On error, it returns stderr content along with the error.
func runCommandWithOutput(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		return stderr.String(), err
	}
	return stdout.String(), nil
}

// resolveCommand finds the absolute path for a command, bypassing Go 1.19+ ErrDot restriction.
// This is useful when you need to resolve a command path before execution.
func resolveCommand(name string) (string, error) {
	path, err := exec.LookPath(name)
	if err != nil {
		// If it's just the ErrDot security check, resolve to absolute path
		if errors.Is(err, exec.ErrDot) {
			// Get absolute path for the relative executable
			absPath, absErr := filepath.Abs(path)
			if absErr != nil {
				return "", err
			}
			return absPath, nil
		}
		return "", err
	}
	return path, nil
}
