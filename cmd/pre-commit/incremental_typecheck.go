package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// IncrementalTypecheck handles incremental typechecking of specific files
type IncrementalTypecheck struct {
	projectPath     string
	files           []string
	typecheckFilter TypecheckFilter
}

// NewIncrementalTypecheck creates a new incremental typecheck instance
func NewIncrementalTypecheck(projectPath string, files []string, tf TypecheckFilter) *IncrementalTypecheck {
	return &IncrementalTypecheck{
		projectPath:     projectPath,
		files:           files,
		typecheckFilter: tf,
	}
}

// Run executes incremental typecheck on the specified files
// Returns nil if all checks pass, error otherwise
func (it *IncrementalTypecheck) Run() error {
	// Filter to only TypeScript files
	tsFiles := filterTypeScriptFiles(it.files)
	if len(tsFiles) == 0 {
		fmt.Println("   No TypeScript files to check")
		return nil
	}

	// Convert to relative paths from project directory
	relativePaths, err := it.toRelativePaths(tsFiles)
	if err != nil {
		return fmt.Errorf("failed to resolve file paths: %w", err)
	}

	if len(relativePaths) == 0 {
		fmt.Println("   No files in project directory to check")
		return nil
	}

	fmt.Printf("   Type checking %d file(s)...\n", len(relativePaths))

	// Find type definition files to include
	typeDefFiles := it.findTypeDefinitionFiles()

	// Build tsc-files command based on config
	args := []string{"tsc-files", "--noEmit"}
	if it.typecheckFilter.SkipLibCheck != nil && *it.typecheckFilter.SkipLibCheck {
		args = append(args, "--skipLibCheck")
	}
	args = append(args, typeDefFiles...)
	args = append(args, relativePaths...)

	// Run tsc-files
	cmd := exec.Command("npx", args...)
	cmd.Dir = it.projectPath

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Run() // Ignore exit code, we'll determine success from filtered errors

	// Combine output
	output := stdout.String() + stderr.String()

	// Filter to only show errors in our changed files
	errors := it.filterErrorsToChangedFiles(output, relativePaths)

	if len(errors) > 0 {
		fmt.Println()
		for _, errLine := range errors {
			fmt.Println(errLine)
		}
		return fmt.Errorf("found %d typecheck error(s) in changed files", len(errors))
	}

	fmt.Printf("   ✅ Type check passed for %d file(s)\n", len(relativePaths))
	return nil
}

// filterTypeScriptFiles returns only .ts and .tsx files
func filterTypeScriptFiles(files []string) []string {
	var tsFiles []string
	for _, f := range files {
		if strings.HasSuffix(f, ".ts") || strings.HasSuffix(f, ".tsx") {
			tsFiles = append(tsFiles, f)
		}
	}
	return tsFiles
}

// toRelativePaths converts absolute paths to paths relative to the project directory
func (it *IncrementalTypecheck) toRelativePaths(files []string) ([]string, error) {
	var relativePaths []string
	absProjectPath, err := filepath.Abs(it.projectPath)
	if err != nil {
		return nil, err
	}

	for _, file := range files {
		absFile, err := filepath.Abs(file)
		if err != nil {
			continue
		}

		// Check if file is within project directory
		if !strings.HasPrefix(absFile, absProjectPath+string(filepath.Separator)) {
			continue
		}

		// Get relative path
		relPath, err := filepath.Rel(absProjectPath, absFile)
		if err != nil {
			continue
		}

		relativePaths = append(relativePaths, relPath)
	}

	return relativePaths, nil
}

// findTypeDefinitionFiles looks for common type definition files that need to be included
func (it *IncrementalTypecheck) findTypeDefinitionFiles() []string {
	var typeDefFiles []string

	// Common type definition files
	candidates := []string{
		"uniwind-types.d.ts",
		"expo-env.d.ts",
		"env.d.ts",
		"global.d.ts",
	}

	for _, candidate := range candidates {
		fullPath := filepath.Join(it.projectPath, candidate)
		if _, err := os.Stat(fullPath); err == nil {
			typeDefFiles = append(typeDefFiles, candidate)
		}
	}

	return typeDefFiles
}

// filterErrorsToChangedFiles filters TypeScript errors to only those in the changed files
func (it *IncrementalTypecheck) filterErrorsToChangedFiles(output string, changedFiles []string) []string {
	var filteredErrors []string

	// Build a set of changed file basenames for quick lookup
	changedSet := make(map[string]bool)
	for _, f := range changedFiles {
		changedSet[f] = true
		// Also add just the filename for matching
		changedSet[filepath.Base(f)] = true
	}

	// Regex to match TypeScript error lines: file.tsx(10,5): error TS2322: ...
	errorLineRe := regexp.MustCompile(`^(.+?)\(\d+,\d+\): error TS\d+:`)

	lines := strings.Split(output, "\n")
	var currentError []string
	var isRelevantError bool

	for _, line := range lines {
		if matches := errorLineRe.FindStringSubmatch(line); matches != nil {
			// Save previous error if it was relevant
			if isRelevantError && len(currentError) > 0 {
				filteredErrors = append(filteredErrors, strings.Join(currentError, "\n"))
			}

			// Check if this error is for a changed file
			errorFile := matches[1]
			isRelevantError = it.isChangedFile(errorFile, changedSet)

			if isRelevantError {
				currentError = []string{line}
			} else {
				currentError = nil
			}
		} else if isRelevantError && (strings.HasPrefix(line, "  ") || strings.HasPrefix(line, "\t")) {
			// Continuation line for a relevant error
			currentError = append(currentError, line)
		} else if isRelevantError && len(currentError) > 0 {
			// End of error block
			filteredErrors = append(filteredErrors, strings.Join(currentError, "\n"))
			currentError = nil
			isRelevantError = false
		}
	}

	// Don't forget the last error
	if isRelevantError && len(currentError) > 0 {
		filteredErrors = append(filteredErrors, strings.Join(currentError, "\n"))
	}

	return filteredErrors
}

// isChangedFile checks if an error file path matches any of our changed files
func (it *IncrementalTypecheck) isChangedFile(errorFile string, changedSet map[string]bool) bool {
	// Direct match
	if changedSet[errorFile] {
		return true
	}

	// Try basename match
	if changedSet[filepath.Base(errorFile)] {
		return true
	}

	// Try normalizing paths
	normalizedError := filepath.Clean(errorFile)
	if changedSet[normalizedError] {
		return true
	}

	return false
}

// runIncrementalTypecheck is a convenience function for running incremental typecheck
func runIncrementalTypecheck(projectPath string, files []string, tf TypecheckFilter) error {
	checker := NewIncrementalTypecheck(projectPath, files, tf)
	return checker.Run()
}

// RunBuffered executes incremental typecheck and returns buffered output (for parallel execution)
func (it *IncrementalTypecheck) RunBuffered() (string, error) {
	var output strings.Builder

	// Filter to only TypeScript files
	tsFiles := filterTypeScriptFiles(it.files)
	if len(tsFiles) == 0 {
		output.WriteString("   No TypeScript files to check\n")
		return output.String(), nil
	}

	// Convert to relative paths from project directory
	relativePaths, err := it.toRelativePaths(tsFiles)
	if err != nil {
		return "", fmt.Errorf("failed to resolve file paths: %w", err)
	}

	if len(relativePaths) == 0 {
		output.WriteString("   No files in project directory to check\n")
		return output.String(), nil
	}

	fmt.Fprintf(&output, "   Type checking %d file(s)...\n", len(relativePaths))

	// Find type definition files to include
	typeDefFiles := it.findTypeDefinitionFiles()

	// Build tsc-files command based on config
	args := []string{"tsc-files", "--noEmit"}
	if it.typecheckFilter.SkipLibCheck != nil && *it.typecheckFilter.SkipLibCheck {
		args = append(args, "--skipLibCheck")
	}
	args = append(args, typeDefFiles...)
	args = append(args, relativePaths...)

	// Run tsc-files
	cmd := exec.Command("npx", args...)
	cmd.Dir = it.projectPath

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Run() // Ignore exit code, we'll determine success from filtered errors

	// Combine output
	tscOutput := stdout.String() + stderr.String()

	// Filter to only show errors in our changed files
	errors := it.filterErrorsToChangedFiles(tscOutput, relativePaths)

	if len(errors) > 0 {
		output.WriteString("\n")
		for _, errLine := range errors {
			output.WriteString(errLine)
			output.WriteString("\n")
		}
		return output.String(), fmt.Errorf("found %d typecheck error(s) in changed files", len(errors))
	}

	fmt.Fprintf(&output, "   ✅ Type check passed for %d file(s)\n", len(relativePaths))
	return output.String(), nil
}

// runIncrementalTypecheckBuffered is a convenience function for running incremental typecheck with buffered output
func runIncrementalTypecheckBuffered(projectPath string, files []string, tf TypecheckFilter) (string, error) {
	checker := NewIncrementalTypecheck(projectPath, files, tf)
	return checker.RunBuffered()
}
