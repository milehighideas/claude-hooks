package main

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// allConvexRules are the plugin rule ids that convexCheck can promote to error.
var allConvexRules = []string{
	"file-size", "max-functions", "require-returns", "no-any-returns",
	"no-api-imports", "type-exports-location", "no-filter-in-query",
	"no-collect-in-query",
}

const convexTestSuffixSkip = ".test."

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

// rulesToEnforce returns the convex rule ids to promote to error given severity.
func rulesToEnforce(cfg ConvexCheckConfig) []string {
	if cfg.Severity == "error" {
		// all + crud-structure (self-noops outside crudDomains)
		return append(append([]string{}, allConvexRules...), "crud-structure")
	}
	if len(cfg.CrudDomains) > 0 {
		return []string{"crud-structure"}
	}
	return nil
}

type oxlintResult struct {
	Diagnostics []struct {
		Code     string `json:"code"`
		Message  string `json:"message"`
		Filename string `json:"filename"`
	} `json:"diagnostics"`
}

// runConvexCheck runs oxlint with the convex rules forced to error on the
// staged convex files and returns an error (blocking) if any fire. No-op when
// dormant (severity != "error" and no crudDomains).
func runConvexCheck(cfg ConvexCheckConfig, projectRoot string, stagedAbs []string) error {
	rules := rulesToEnforce(cfg)
	if len(rules) == 0 {
		return nil // dormant
	}
	files := convexFilesInScope(cfg, projectRoot, stagedAbs)
	if len(files) == 0 {
		return nil
	}
	args := []string{"--format=json"}
	for _, r := range rules {
		args = append(args, "-D", "convex/"+r)
	}
	args = append(args, files...)

	cmd := exec.Command("oxlint", args...)
	cmd.Dir = projectRoot
	out, _ := cmd.Output() // non-zero exit when findings exist; parse stdout regardless

	var res oxlintResult
	if err := json.Unmarshal(out, &res); err != nil {
		return nil // oxlint not available / parse failure → don't wedge the commit
	}
	var convexDiags []string
	for _, d := range res.Diagnostics {
		if strings.HasPrefix(d.Code, "convex(") {
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
