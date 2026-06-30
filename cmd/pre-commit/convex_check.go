package main

import (
	"encoding/json"
	"fmt"
	"os"
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

type eslintResult struct {
	FilePath string `json:"filePath"`
	Messages []struct {
		RuleID  string `json:"ruleId"`
		Message string `json:"message"`
		Line    int    `json:"line"`
	} `json:"messages"`
}

// oxlintCommitViolations runs oxlint on the staged files and returns formatted
// messages for convex(*) diagnostics whose rule id is in want. oxlint emits
// every convex rule as a warning (from .oxlintrc); we gate on errorRules
// membership, not oxlint severity (oxlint -D does not override a JS-plugin
// rule's config severity).
func oxlintCommitViolations(projectRoot string, files []string, want map[string]bool) []string {
	args := append([]string{"--format=json"}, files...)
	cmd := exec.Command("oxlint", args...)
	cmd.Dir = projectRoot
	out, _ := cmd.Output() // non-zero exit when findings exist; parse stdout regardless

	var res oxlintResult
	if json.Unmarshal(out, &res) != nil {
		return nil // oxlint not available / parse failure → don't wedge the commit
	}
	var msgs []string
	for _, d := range res.Diagnostics {
		if rule := convexRuleID(d.Code); rule != "" && want[rule] {
			msgs = append(msgs, fmt.Sprintf("  %s  convex/%s — %s", filepath.Base(d.Filename), rule, d.Message))
		}
	}
	return msgs
}

// eslintCommitViolations runs eslint_d on the staged files (warm daemon → fast)
// and returns messages for @convex-dev/<rule> whose bare id is in want. These
// are the type-aware rules oxlint can't do. Best-effort: skipped if eslint_d
// isn't installed.
func eslintCommitViolations(projectRoot string, files []string, want map[string]bool) []string {
	bin := filepath.Join(projectRoot, "node_modules", ".bin", "eslint_d")
	if _, err := os.Stat(bin); err != nil {
		return nil
	}
	args := append([]string{"--format", "json"}, files...)
	cmd := exec.Command(bin, args...)
	cmd.Dir = projectRoot
	out, _ := cmd.Output()

	var results []eslintResult
	if json.Unmarshal(out, &results) != nil {
		return nil
	}
	var msgs []string
	for _, r := range results {
		for _, m := range r.Messages {
			const prefix = "@convex-dev/"
			if strings.HasPrefix(m.RuleID, prefix) && want[strings.TrimPrefix(m.RuleID, prefix)] {
				msgs = append(msgs, fmt.Sprintf("  %s:%d  %s — %s", filepath.Base(r.FilePath), m.Line, m.RuleID, m.Message))
			}
		}
	}
	return msgs
}

// runConvexCheck loads .convex-lint.json and blocks when a staged convex file
// trips an opted-in rule: errorRules (oxlint, syntactic) or eslintErrorRules
// (eslint_d, type-aware). Mirrors the validate-convex edit-time hook so commit
// and edit enforce the same set. No-op when both are empty.
func runConvexCheck(projectRoot string, stagedAbs []string) error {
	cfg := loadConvexConfig(projectRoot)
	oxSet := map[string]bool{}
	for _, r := range cfg.ErrorRules {
		oxSet[r] = true
	}
	esSet := map[string]bool{}
	for _, r := range cfg.EslintErrorRules {
		esSet[r] = true
	}
	if len(oxSet) == 0 && len(esSet) == 0 {
		return nil // dormant
	}
	files := convexFilesInScope(cfg, projectRoot, stagedAbs)
	if len(files) == 0 {
		return nil
	}

	var diags []string
	if len(oxSet) > 0 {
		diags = append(diags, oxlintCommitViolations(projectRoot, files, oxSet)...)
	}
	if len(esSet) > 0 {
		diags = append(diags, eslintCommitViolations(projectRoot, files, esSet)...)
	}
	failed := len(diags) > 0
	_ = writeRunReport("convex-check", "Convex check", strings.Join(diags, "\n"), failed)
	if failed {
		printReportHint("convex-check/")
		return fmt.Errorf("convexCheck: %d violation(s)\n%s", len(diags), strings.Join(diags, "\n"))
	}
	return nil
}
