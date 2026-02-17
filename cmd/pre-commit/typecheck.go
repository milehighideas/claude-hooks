package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// tsError represents a parsed TypeScript error (may span multiple lines)
type tsError struct {
	filePath  string
	errorCode string
	fullText  string
}

// DefaultErrorCodes are the TypeScript error codes filtered by default
var DefaultErrorCodes = []string{"TS2589", "TS2742"}

// DefaultExcludePaths are the path patterns excluded by default (test files)
var DefaultExcludePaths = []string{"__tests__/", ".test.", ".spec."}

// buildTypecheckCmd builds the tsc command using the configured package manager
func buildTypecheckCmd(packageManager, filter string, tf TypecheckFilter) *exec.Cmd {
	if tf.UseBuildMode != nil && *tf.UseBuildMode {
		switch packageManager {
		case "bun":
			return exec.Command("bun", "--filter", filter, "tsc", "-b")
		case "yarn":
			return exec.Command("yarn", "workspace", filter, "exec", "tsc", "-b")
		default:
			return exec.Command("pnpm", "--filter", filter, "exec", "tsc", "-b")
		}
	}

	args := []string{"--noEmit"}
	if tf.SkipLibCheck != nil && *tf.SkipLibCheck {
		args = append(args, "--skipLibCheck")
	}

	switch packageManager {
	case "bun":
		return exec.Command("bun", append([]string{"--filter", filter, "tsc"}, args...)...)
	case "yarn":
		return exec.Command("yarn", append([]string{"workspace", filter, "exec", "tsc"}, args...)...)
	default:
		return exec.Command("pnpm", append([]string{"--filter", filter, "exec", "tsc"}, args...)...)
	}
}

// runFilteredTypecheck runs tsc and filters out configured errors
func runFilteredTypecheck(appName, filter, packageManager string, tf TypecheckFilter, nodeMemoryMB int) error {
	// Default filter patterns if none configured (nil = not set, use defaults; empty = explicitly no filtering)
	errorCodes := tf.ErrorCodes
	if errorCodes == nil {
		errorCodes = DefaultErrorCodes
	}
	excludePaths := tf.ExcludePaths
	if excludePaths == nil {
		excludePaths = DefaultExcludePaths
	}

	// Build tsc command using configured package manager
	cmd := buildTypecheckCmd(packageManager, filter, tf)

	// Set NODE_OPTIONS for memory limit if configured
	if nodeMemoryMB > 0 {
		cmd.Env = append(os.Environ(), fmt.Sprintf("NODE_OPTIONS=--max-old-space-size=%d", nodeMemoryMB))
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Run() // Ignore exit code, we'll determine success from filtered errors

	// Combine stdout and stderr (tsc writes errors to stdout)
	output := stdout.String() + stderr.String()

	// Parse and filter errors
	errors := parseTypeScriptErrors(output)
	var realErrors []tsError

	for _, err := range errors {
		if shouldFilterError(err, errorCodes, excludePaths) {
			continue
		}
		realErrors = append(realErrors, err)
	}

	// Print filtered count
	filteredCount := len(errors) - len(realErrors)
	if filteredCount > 0 {
		fmt.Printf("   (filtered %d known errors)\n", filteredCount)
	}

	// Write report if reportDir is set
	if reportDir != "" {
		if err := writeTypecheckReport(appName, output, errors, realErrors, reportDir); err != nil {
			fmt.Printf("   Warning: failed to write typecheck report: %v\n", err)
		}
	}

	// Print real errors
	if len(realErrors) > 0 {
		fmt.Println()
		for _, err := range realErrors {
			fmt.Println(err.fullText)
		}
		return fmt.Errorf("found %d typecheck error(s)", len(realErrors))
	}

	return nil
}

// writeTypecheckReport writes typecheck findings to a report file
func writeTypecheckReport(appName, rawOutput string, allErrors, realErrors []tsError, baseDir string) error {
	typecheckDir := filepath.Join(baseDir, "typecheck")
	if err := os.MkdirAll(typecheckDir, 0755); err != nil {
		return err
	}

	reportPath := filepath.Join(typecheckDir, fmt.Sprintf("%s.txt", appName))

	var sb strings.Builder
	sb.WriteString(strings.Repeat("=", 80) + "\n")
	sb.WriteString(fmt.Sprintf("TYPECHECK REPORT: %s\n", appName))
	sb.WriteString(fmt.Sprintf("Generated: %s\n", time.Now().Format("2006-01-02 15:04:05")))
	sb.WriteString(strings.Repeat("=", 80) + "\n\n")

	sb.WriteString(fmt.Sprintf("Total errors parsed: %d\n", len(allErrors)))
	sb.WriteString(fmt.Sprintf("Errors after filtering: %d\n", len(realErrors)))
	sb.WriteString(fmt.Sprintf("Filtered out: %d\n\n", len(allErrors)-len(realErrors)))

	// Group errors by file
	errorsByFile := make(map[string][]tsError)
	for _, e := range realErrors {
		errorsByFile[e.filePath] = append(errorsByFile[e.filePath], e)
	}

	// Count by error code
	errorsByCode := make(map[string]int)
	for _, e := range realErrors {
		errorsByCode[e.errorCode]++
	}

	sb.WriteString(strings.Repeat("=", 80) + "\n")
	sb.WriteString("ERRORS BY CODE\n")
	sb.WriteString(strings.Repeat("=", 80) + "\n\n")
	for code, count := range errorsByCode {
		sb.WriteString(fmt.Sprintf("  %s: %d\n", code, count))
	}

	sb.WriteString("\n" + strings.Repeat("=", 80) + "\n")
	sb.WriteString("ERRORS BY FILE\n")
	sb.WriteString(strings.Repeat("=", 80) + "\n\n")

	for file, errs := range errorsByFile {
		sb.WriteString(fmt.Sprintf("\n%s (%d errors)\n", file, len(errs)))
		sb.WriteString(strings.Repeat("-", 40) + "\n")
		for _, e := range errs {
			sb.WriteString(fmt.Sprintf("  [%s] %s\n", e.errorCode, e.fullText))
		}
	}

	// Also write raw output
	sb.WriteString("\n\n" + strings.Repeat("=", 80) + "\n")
	sb.WriteString("RAW TSC OUTPUT\n")
	sb.WriteString(strings.Repeat("=", 80) + "\n\n")
	sb.WriteString(rawOutput)

	return os.WriteFile(reportPath, []byte(sb.String()), 0644)
}

// parseTypeScriptErrors parses tsc output into individual errors
// TypeScript errors look like:
// file.tsx(10,5): error TS2322: Type 'x' is not assignable to type 'y'.
//
//	Continuation line with more details
func parseTypeScriptErrors(output string) []tsError {
	var errors []tsError
	lines := strings.Split(output, "\n")

	// Regex to match the start of a TypeScript error
	errorLineRe := regexp.MustCompile(`^(.+?)\(\d+,\d+\): error (TS\d+):`)

	var currentError *tsError
	for _, line := range lines {
		if matches := errorLineRe.FindStringSubmatch(line); matches != nil {
			// Save previous error if exists
			if currentError != nil {
				errors = append(errors, *currentError)
			}
			// Start new error
			currentError = &tsError{
				filePath:  matches[1],
				errorCode: matches[2],
				fullText:  line,
			}
		} else if currentError != nil && (strings.HasPrefix(line, "  ") || strings.HasPrefix(line, "\t")) {
			// Continuation line - append to current error
			currentError.fullText += "\n" + line
		} else if currentError != nil {
			// Non-continuation line ends current error
			errors = append(errors, *currentError)
			currentError = nil
		}
	}
	// Don't forget last error
	if currentError != nil {
		errors = append(errors, *currentError)
	}

	return errors
}

// shouldFilterError checks if an error should be filtered out
// - errorCodes: filter these specific TS error codes from ANYWHERE
// - excludePaths: filter ALL errors from files matching these patterns (test files)
func shouldFilterError(err tsError, errorCodes, excludePaths []string) bool {
	// Filter specific error codes (TS2589, TS2742) from anywhere
	for _, code := range errorCodes {
		if err.errorCode == code {
			return true
		}
	}

	// Filter ALL errors from excluded paths (test files)
	for _, pattern := range excludePaths {
		if strings.Contains(err.filePath, pattern) {
			return true
		}
	}

	return false
}

// runFilteredTypecheckBuffered runs tsc and returns buffered output (for parallel execution)
func runFilteredTypecheckBuffered(appName, filter, packageManager string, tf TypecheckFilter, nodeMemoryMB int) (string, error) {
	var output strings.Builder

	// Default filter patterns if none configured (nil = not set, use defaults; empty = explicitly no filtering)
	errorCodes := tf.ErrorCodes
	if errorCodes == nil {
		errorCodes = DefaultErrorCodes
	}
	excludePaths := tf.ExcludePaths
	if excludePaths == nil {
		excludePaths = DefaultExcludePaths
	}

	// Build tsc command using configured package manager
	cmd := buildTypecheckCmd(packageManager, filter, tf)

	// Set NODE_OPTIONS for memory limit if configured
	if nodeMemoryMB > 0 {
		cmd.Env = append(os.Environ(), fmt.Sprintf("NODE_OPTIONS=--max-old-space-size=%d", nodeMemoryMB))
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Run() // Ignore exit code, we'll determine success from filtered errors

	// Combine stdout and stderr (tsc writes errors to stdout)
	tscOutput := stdout.String() + stderr.String()

	// Parse and filter errors
	errors := parseTypeScriptErrors(tscOutput)
	var realErrors []tsError

	for _, err := range errors {
		if shouldFilterError(err, errorCodes, excludePaths) {
			continue
		}
		realErrors = append(realErrors, err)
	}

	// Print filtered count
	filteredCount := len(errors) - len(realErrors)
	if filteredCount > 0 {
		fmt.Fprintf(&output, "   (filtered %d known errors)\n", filteredCount)
	}

	// Write report if reportDir is set
	if reportDir != "" {
		if err := writeTypecheckReport(appName, tscOutput, errors, realErrors, reportDir); err != nil {
			fmt.Fprintf(&output, "   Warning: failed to write typecheck report: %v\n", err)
		}
	}

	// Print real errors
	if len(realErrors) > 0 {
		output.WriteString("\n")
		for _, err := range realErrors {
			output.WriteString(err.fullText)
			output.WriteString("\n")
		}
		return output.String(), fmt.Errorf("found %d typecheck error(s)", len(realErrors))
	}

	return output.String(), nil
}
