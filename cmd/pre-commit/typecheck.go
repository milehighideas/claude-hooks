package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
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

// buildTypecheckCmd builds the tsc (or tsgo) command, preferring the project's
// installed compiler (node_modules/.bin) so the gate uses the same version as
// `bun run typecheck`/CI, never an npx-fetched one. ok is false when no compiler
// can be run without fetching (bun/npm with nothing installed yet), so the
// caller fails the commit instead of passing an unchecked project.
//
// When TypecheckFilter.UseTsgo is true, the binary name is swapped from `tsc`
// (TypeScript 6.x) to `tsgo` (TypeScript 7 native preview, from
// @typescript/native-preview). All flags pass through unchanged — tsgo accepts
// --noEmit, --skipLibCheck, and -b.
func buildTypecheckCmd(packageManager, filter, appPath string, tf TypecheckFilter) (*exec.Cmd, bool) {
	bin := "tsc"
	if tf.UseTsgo != nil && *tf.UseTsgo {
		bin = "tsgo"
	}

	var args []string
	if tf.UseBuildMode != nil && *tf.UseBuildMode {
		args = []string{"-b"}
	} else {
		args = []string{"--noEmit"}
		if tf.SkipLibCheck != nil && *tf.SkipLibCheck {
			args = append(args, "--skipLibCheck")
		}
	}

	// Prefer the project's installed compiler.
	if local, ok := resolveNodeBin(appPath, bin); ok {
		cmd := exec.Command(local, args...)
		cmd.Dir = appPath
		return cmd, true
	}

	// Non-network fallbacks for workspace layouts without a flat
	// node_modules/.bin (yarn PnP, pnpm's isolated store): these still run the
	// *installed* compiler via the package manager, not a fetched one.
	switch packageManager {
	case "yarn":
		return exec.Command("yarn", append([]string{"workspace", filter, "exec", bin}, args...)...), true
	case "pnpm":
		return exec.Command("pnpm", append([]string{"--filter", filter, "exec", bin}, args...)...), true
	default:
		// bun / npm with nothing installed locally — no non-network runner.
		return nil, false
	}
}

// writeTypecheckReport writes typecheck findings to a report file
func writeTypecheckReport(appName, rawOutput string, allErrors, realErrors []tsError, baseDir string) error {
	// Group errors by file and count by code.
	errorsByFile := make(map[string][]tsError)
	for _, e := range realErrors {
		errorsByFile[e.filePath] = append(errorsByFile[e.filePath], e)
	}
	errorsByCode := make(map[string]int)
	for _, e := range realErrors {
		errorsByCode[e.errorCode]++
	}

	// Build the actionable body (by-code + by-file) once; shared by both reports.
	var body strings.Builder
	body.WriteString(strings.Repeat("=", 80) + "\n")
	body.WriteString("ERRORS BY CODE\n")
	body.WriteString(strings.Repeat("=", 80) + "\n\n")
	for code, count := range errorsByCode {
		fmt.Fprintf(&body, "  %s: %d\n", code, count)
	}
	body.WriteString("\n" + strings.Repeat("=", 80) + "\n")
	body.WriteString("ERRORS BY FILE\n")
	body.WriteString(strings.Repeat("=", 80) + "\n\n")
	for file, errs := range errorsByFile {
		fmt.Fprintf(&body, "\n%s (%d errors)\n", file, len(errs))
		body.WriteString(strings.Repeat("-", 40) + "\n")
		for _, e := range errs {
			fmt.Fprintf(&body, "  [%s] %s\n", e.errorCode, e.fullText)
		}
	}

	// Full report (legacy content): counts + body + raw tsc output.
	var sb strings.Builder
	sb.WriteString(strings.Repeat("=", 80) + "\n")
	fmt.Fprintf(&sb, "TYPECHECK REPORT: %s\n", appName)
	fmt.Fprintf(&sb, "Generated: %s\n", time.Now().Format("2006-01-02 15:04:05"))
	sb.WriteString(strings.Repeat("=", 80) + "\n\n")

	fmt.Fprintf(&sb, "Total errors parsed: %d\n", len(allErrors))
	fmt.Fprintf(&sb, "Errors after filtering: %d\n", len(realErrors))
	fmt.Fprintf(&sb, "Filtered out: %d\n\n", len(allErrors)-len(realErrors))

	sb.WriteString(body.String())

	sb.WriteString("\n\n" + strings.Repeat("=", 80) + "\n")
	sb.WriteString("RAW TSC OUTPUT\n")
	sb.WriteString(strings.Repeat("=", 80) + "\n\n")
	sb.WriteString(rawOutput)

	// Findings-only report: just the surviving errors.
	findings := findingsDoc("TYPECHECK", appName, len(realErrors), body.String())

	return writeDualReport(baseDir, "typecheck", appName, findings, sb.String())
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
func runFilteredTypecheckBuffered(appName, appPath, filter, packageManager string, tf TypecheckFilter, nodeMemoryMB int) (string, error) {
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

	// Build tsc command, preferring the project's installed compiler. A missing
	// compiler fails the commit rather than passing an unchecked project.
	cmd, ok := buildTypecheckCmd(packageManager, filter, appPath, tf)
	if !ok {
		return output.String(), fmt.Errorf("typecheck compiler (tsc/tsgo) is not installed for %s — run your install and retry", appName)
	}

	// Set NODE_OPTIONS for memory limit if configured
	if nodeMemoryMB > 0 {
		cmd.Env = append(os.Environ(), fmt.Sprintf("NODE_OPTIONS=--max-old-space-size=%d", nodeMemoryMB))
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	_ = cmd.Run() // Ignore exit code, we'll determine success from filtered errors

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
