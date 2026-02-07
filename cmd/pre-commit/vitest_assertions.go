package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// VitestAssertionViolation represents a vitest config missing requireAssertions
type VitestAssertionViolation struct {
	ConfigPath string
	AppName    string
	Message    string
}

// VitestAssertionsChecker validates vitest configs have requireAssertions enabled
type VitestAssertionsChecker struct {
	apps map[string]AppConfig
}

// NewVitestAssertionsChecker creates a new vitest assertions checker
func NewVitestAssertionsChecker(apps map[string]AppConfig) *VitestAssertionsChecker {
	return &VitestAssertionsChecker{apps: apps}
}

// Check validates all app vitest configs have requireAssertions: true
func (c *VitestAssertionsChecker) Check() ([]VitestAssertionViolation, error) {
	var violations []VitestAssertionViolation

	// Vitest config file patterns to check
	configPatterns := []string{
		"vitest.config.ts",
		"vitest.config.mts",
		"vitest.config.js",
		"vitest.config.mjs",
	}

	for appName, appConfig := range c.apps {
		appPath := appConfig.Path

		// Find vitest config in app
		var configPath string
		for _, pattern := range configPatterns {
			candidate := filepath.Join(appPath, pattern)
			if _, err := os.Stat(candidate); err == nil {
				configPath = candidate
				break
			}
		}

		// Skip apps without vitest config (they might use jest or no unit tests)
		if configPath == "" {
			continue
		}

		// Read and check the config
		content, err := os.ReadFile(configPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read %s: %w", configPath, err)
		}

		if !hasRequireAssertions(string(content)) {
			violations = append(violations, VitestAssertionViolation{
				ConfigPath: configPath,
				AppName:    appName,
				Message:    "vitest config missing requireAssertions: true",
			})
		}
	}

	return violations, nil
}

// hasRequireAssertions checks if vitest config has requireAssertions: true
func hasRequireAssertions(content string) bool {
	// Look for requireAssertions: true not preceded by // on the same line
	// This avoids false positives from commented-out config
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		// Skip lines that are pure comments
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "//") {
			continue
		}

		// Check if line has requireAssertions: true before any //
		commentIdx := strings.Index(line, "//")
		checkPart := line
		if commentIdx != -1 {
			checkPart = line[:commentIdx]
		}

		pattern := regexp.MustCompile(`requireAssertions\s*:\s*true`)
		if pattern.MatchString(checkPart) {
			return true
		}
	}

	return false
}

// runVitestAssertionsCheck is the entry point for vitest assertions validation
func runVitestAssertionsCheck(apps map[string]AppConfig) error {
	if !compactMode() {
		fmt.Println("================================")
		fmt.Println("  VITEST ASSERTIONS CHECK")
		fmt.Println("================================")
	}

	checker := NewVitestAssertionsChecker(apps)
	violations, err := checker.Check()
	if err != nil {
		return fmt.Errorf("vitest assertions check failed: %w", err)
	}

	// Write report if reportDir is set
	if reportDir != "" && len(violations) > 0 {
		if err := writeVitestAssertionsReport(violations, reportDir); err != nil {
			fmt.Printf("   Warning: failed to write vitest assertions report: %v\n", err)
		}
	}

	if compactMode() {
		if len(violations) > 0 {
			printStatus("Vitest assertions", false, fmt.Sprintf("%d configs", len(violations)))
			printReportHint("vitest-assertions/")
			return fmt.Errorf("vitest assertions check failed")
		}
		printStatus("Vitest assertions", true, "")
		return nil
	}

	// Verbose output
	if len(violations) == 0 {
		fmt.Println("✅ All vitest configs have requireAssertions enabled")
		fmt.Println()
		return nil
	}

	for _, v := range violations {
		fmt.Printf("❌ %s: %s\n", v.AppName, v.Message)
		fmt.Printf("   Config: %s\n", v.ConfigPath)
		fmt.Println("   FIX: Add to vitest config:")
		fmt.Println("     test: {")
		fmt.Println("       expect: {")
		fmt.Println("         requireAssertions: true,")
		fmt.Println("       },")
		fmt.Println("     }")
		fmt.Println()
	}

	fmt.Printf("\n❌ Found %d vitest config(s) missing requireAssertions\n", len(violations))
	fmt.Println()
	fmt.Println("Tests without assertions provide false confidence.")
	fmt.Println("requireAssertions: true ensures every test has at least one expect() call.")
	fmt.Println()

	return fmt.Errorf("vitest assertions check failed")
}

// writeVitestAssertionsReport writes vitest assertions findings to a report file
func writeVitestAssertionsReport(violations []VitestAssertionViolation, baseDir string) error {
	vitestDir := filepath.Join(baseDir, "vitest-assertions")
	if err := os.MkdirAll(vitestDir, 0755); err != nil {
		return err
	}

	reportPath := filepath.Join(vitestDir, "violations.txt")

	var sb strings.Builder
	sb.WriteString(strings.Repeat("=", 80) + "\n")
	sb.WriteString("VITEST ASSERTIONS VIOLATIONS REPORT\n")
	sb.WriteString(fmt.Sprintf("Generated: %s\n", time.Now().Format("2006-01-02 15:04:05")))
	sb.WriteString(strings.Repeat("=", 80) + "\n\n")

	sb.WriteString(fmt.Sprintf("Total violations: %d\n\n", len(violations)))

	sb.WriteString(strings.Repeat("=", 80) + "\n")
	sb.WriteString("APPS MISSING requireAssertions: true\n")
	sb.WriteString(strings.Repeat("=", 80) + "\n\n")

	for _, v := range violations {
		sb.WriteString(fmt.Sprintf("%s\n", v.AppName))
		sb.WriteString(fmt.Sprintf("  Config: %s\n", v.ConfigPath))
		sb.WriteString(fmt.Sprintf("  Issue: %s\n\n", v.Message))
	}

	sb.WriteString(strings.Repeat("=", 80) + "\n")
	sb.WriteString("HOW TO FIX\n")
	sb.WriteString(strings.Repeat("=", 80) + "\n\n")
	sb.WriteString("Add to your vitest config:\n\n")
	sb.WriteString("  test: {\n")
	sb.WriteString("    expect: {\n")
	sb.WriteString("      requireAssertions: true,\n")
	sb.WriteString("    },\n")
	sb.WriteString("  }\n\n")
	sb.WriteString("This ensures every test has at least one expect() call.\n")

	return os.WriteFile(reportPath, []byte(sb.String()), 0644)
}
