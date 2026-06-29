package main

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/milehighideas/claude-hooks/internal/jsonc"
)

const convexTestSuffixSkip = ".test."

// loadConvexConfig reads .convex-lint.json from the project root — the single
// source of truth shared with the oxlint plugin. Missing/invalid file = empty
// config (feature dormant).
func loadConvexConfig(projectRoot string) ConvexCheckConfig {
	var cfg ConvexCheckConfig
	_ = jsonc.Unmarshal(filepath.Join(projectRoot, ".convex-lint.json"), &cfg)
	return cfg
}

// convexFilesInScope filters staged absolute paths to in-scope Convex files.
func convexFilesInScope(cfg ConvexCheckConfig, projectRoot string, staged []string) []string {
	var out []string
	for _, f := range staged {
		rel := strings.ReplaceAll(f, "\\", "/")
		if strings.Contains(rel, "/_generated/") || strings.Contains(rel, convexTestSuffixSkip) {
			continue
		}
		excluded := false
		for _, ex := range cfg.ExcludePaths {
			if ex != "" && strings.Contains(rel, ex) {
				excluded = true
				break
			}
		}
		if excluded {
			continue
		}
		inApp := len(cfg.AppPaths) == 0
		for _, ap := range cfg.AppPaths {
			if ap != "" && strings.Contains(rel, ap) {
				inApp = true
				break
			}
		}
		if inApp && (strings.HasSuffix(f, ".ts") || strings.HasSuffix(f, ".tsx")) {
			out = append(out, f)
		}
	}
	return out
}

type oxlintResult struct {
	Diagnostics []struct {
		Code     string `json:"code"`
		Message  string `json:"message"`
		Filename string `json:"filename"`
	} `json:"diagnostics"`
}

// convexRuleID turns "convex(type-exports-location)" into "type-exports-location";
// "" if not a convex rule.
func convexRuleID(code string) string {
	const prefix = "convex("
	if !strings.HasPrefix(code, prefix) || !strings.HasSuffix(code, ")") {
		return ""
	}
	return code[len(prefix) : len(code)-1]
}

// runConvexCheck loads .convex-lint.json and blocks when a staged convex file
// trips a rule listed in errorRules. oxlint emits every convex rule as a warning
// (from .oxlintrc); we gate on errorRules membership, not oxlint severity
// (oxlint -D does not override a JS-plugin rule's config severity). No-op when
// dormant (errorRules empty).
func runConvexCheck(projectRoot string, stagedAbs []string) error {
	cfg := loadConvexConfig(projectRoot)
	errSet := map[string]bool{}
	for _, r := range cfg.ErrorRules {
		errSet[r] = true
	}
	if len(errSet) == 0 {
		return nil // dormant
	}
	files := convexFilesInScope(cfg, projectRoot, stagedAbs)
	if len(files) == 0 {
		return nil
	}
	args := append([]string{"--format=json"}, files...)
	cmd := exec.Command("oxlint", args...)
	cmd.Dir = projectRoot
	out, _ := cmd.Output() // non-zero exit when findings exist; parse stdout regardless

	var res oxlintResult
	if err := json.Unmarshal(out, &res); err != nil {
		return nil // oxlint not available / parse failure → don't wedge the commit
	}
	var convexDiags []string
	for _, d := range res.Diagnostics {
		if rule := convexRuleID(d.Code); rule != "" && errSet[rule] {
			convexDiags = append(convexDiags,
				fmt.Sprintf("  %s  %s — %s", filepath.Base(d.Filename), d.Code, d.Message))
		}
	}
	if len(convexDiags) > 0 {
		return fmt.Errorf("convexCheck: %d violation(s)\n%s",
			len(convexDiags), strings.Join(convexDiags, "\n"))
	}
	return nil
}
