// Command validate-convex is a PreToolUse hook (Write|Edit) that lints the edited
// Convex file and blocks (exit 2) on opted-in rules from .convex-lint.json:
//
//   • errorRules        — @milehighideas/oxlint-plugin-convex rules, run via
//     oxlint (fast, syntactic). Gated on rule-id membership (oxlint -D does not
//     override a JS-plugin rule's config severity).
//   • eslintErrorRules  — @convex-dev/eslint-plugin rules (the type-aware ones
//     oxlint can't do, e.g. explicit-table-ids), run via eslint_d (the daemon
//     keeps the TS program warm so a single-file lint is sub-second). Best
//     effort: if eslint_d isn't installed, this pass is silently skipped.
//
// Both default to empty = dormant.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/milehighideas/claude-hooks/internal/jsonc"
)

type hookInput struct {
	ToolName  string `json:"tool_name"`
	ToolInput struct {
		FilePath string `json:"file_path"`
	} `json:"tool_input"`
}

type convexCfg struct {
	AppPaths     []string `json:"appPaths"`
	ExcludePaths []string `json:"excludePaths"`
	// ErrorRules: oxlint-plugin-convex rule ids enforced (blocking) at edit
	// time. Empty = dormant. .oxlintrc.json stays at "warn"; this is the ratchet.
	ErrorRules []string `json:"errorRules"`
	// EslintErrorRules: @convex-dev/eslint-plugin rule ids (bare, e.g.
	// "explicit-table-ids") enforced at edit time via eslint_d.
	EslintErrorRules []string `json:"eslintErrorRules"`
}

func isConvexTarget(path string, appPaths, exclude []string) bool {
	f := strings.ReplaceAll(path, "\\", "/")
	if strings.Contains(f, "/_generated/") || strings.Contains(f, ".test.") || strings.Contains(f, ".spec.") {
		return false
	}
	if !strings.HasSuffix(f, ".ts") && !strings.HasSuffix(f, ".tsx") {
		return false
	}
	for _, ex := range exclude {
		if ex != "" && strings.Contains(f, ex) {
			return false
		}
	}
	if len(appPaths) == 0 {
		return false
	}
	for _, ap := range appPaths {
		if ap != "" && strings.Contains(f, ap) {
			return true
		}
	}
	return false
}

func toSet(xs []string) map[string]bool {
	s := map[string]bool{}
	for _, x := range xs {
		if x != "" {
			s[x] = true
		}
	}
	return s
}

// convexRuleID turns an oxlint diagnostic code like "convex(type-exports-location)"
// into the bare rule id "type-exports-location"; "" if not a convex rule.
func convexRuleID(code string) string {
	const prefix = "convex("
	if !strings.HasPrefix(code, prefix) || !strings.HasSuffix(code, ")") {
		return ""
	}
	return code[len(prefix) : len(code)-1]
}

type oxlintResult struct {
	Diagnostics []struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"diagnostics"`
}

// oxlintViolations runs oxlint on path and returns formatted messages for any
// convex(*) diagnostic whose rule id is in want.
func oxlintViolations(path string, want map[string]bool) []string {
	out, _ := exec.Command("oxlint", "--format=json", path).Output()
	var res oxlintResult
	if json.Unmarshal(out, &res) != nil {
		return nil
	}
	var msgs []string
	for _, d := range res.Diagnostics {
		if rule := convexRuleID(d.Code); rule != "" && want[rule] {
			msgs = append(msgs, fmt.Sprintf("  ✗ convex/%s — %s", rule, d.Message))
		}
	}
	return msgs
}

type eslintResult struct {
	Messages []struct {
		RuleID  string `json:"ruleId"`
		Message string `json:"message"`
		Line    int    `json:"line"`
	} `json:"messages"`
}

// eslintViolations runs eslint_d on path (daemon keeps the TS program warm) and
// returns messages for any @convex-dev/<rule> whose bare rule id is in want.
// Best effort: if eslint_d isn't installed, returns nil (no block).
func eslintViolations(projectRoot, path string, want map[string]bool) []string {
	bin := filepath.Join(projectRoot, "node_modules", ".bin", "eslint_d")
	if _, err := os.Stat(bin); err != nil {
		return nil // eslint_d not available — skip the type-aware pass
	}
	cmd := exec.Command(bin, "--format", "json", path)
	cmd.Dir = projectRoot
	out, _ := cmd.Output() // non-zero exit on lint errors; JSON still on stdout

	var results []eslintResult
	if json.Unmarshal(out, &results) != nil {
		return nil
	}
	var msgs []string
	for _, r := range results {
		for _, m := range r.Messages {
			const prefix = "@convex-dev/"
			if !strings.HasPrefix(m.RuleID, prefix) {
				continue
			}
			if want[strings.TrimPrefix(m.RuleID, prefix)] {
				msgs = append(msgs, fmt.Sprintf("  ✗ %s:%d — %s", m.RuleID, m.Line, m.Message))
			}
		}
	}
	return msgs
}

func main() {
	data, _ := io.ReadAll(os.Stdin)
	var in hookInput
	if json.Unmarshal(data, &in) != nil {
		os.Exit(0)
	}
	if in.ToolName != "Edit" && in.ToolName != "Write" {
		os.Exit(0)
	}
	path := in.ToolInput.FilePath
	if path == "" {
		os.Exit(0)
	}

	// .convex-lint.json is the single source of truth, shared with the oxlint
	// plugin and the convexCheck commit gate. Read from CWD (the hook runs in
	// the project root). If it can't be read, stay silent (allow).
	var cfg convexCfg
	if err := jsonc.Unmarshal(".convex-lint.json", &cfg); err != nil {
		os.Exit(0)
	}
	if !isConvexTarget(path, cfg.AppPaths, cfg.ExcludePaths) {
		os.Exit(0)
	}

	oxSet := toSet(cfg.ErrorRules)
	esSet := toSet(cfg.EslintErrorRules)
	if len(oxSet) == 0 && len(esSet) == 0 {
		os.Exit(0) // fully dormant
	}

	var msgs []string
	if len(oxSet) > 0 {
		msgs = append(msgs, oxlintViolations(path, oxSet)...)
	}
	if len(esSet) > 0 {
		cwd, _ := os.Getwd()
		msgs = append(msgs, eslintViolations(cwd, path, esSet)...)
	}

	if len(msgs) > 0 {
		fmt.Fprintf(os.Stderr, "\n❌ BLOCKED: Convex lint violation in %s\n%s\n\nFix the issue (split the file, add returns:, use db.get(\"table\", id), etc.) — see convex/REFACTORING.md.\n",
			filepath.Base(path), strings.Join(msgs, "\n"))
		os.Exit(2)
	}
	os.Exit(0)
}
