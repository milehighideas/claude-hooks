// Command validate-convex is a PreToolUse hook (Write|Edit) that runs the
// @milehighideas/oxlint-plugin-convex rules on the edited Convex file and blocks
// (exit 2) when a convex(*) diagnostic fires — but only when enforcement is on
// (convexCheckConfig.severity == "error", or the file is under crudDomains).
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
	// ErrorRules: convex rule ids enforced (blocking) at edit time. Empty =
	// dormant. .oxlintrc.json stays at "warn"; this is the per-rule ratchet.
	ErrorRules []string `json:"errorRules"`
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

type oxlintResult struct {
	Diagnostics []struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"diagnostics"`
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

	// Enforce only the rules opted into via .convex-lint.json errorRules (the
	// ratchet). Empty = dormant. oxlint emits every convex rule as a warning
	// (from .oxlintrc); we block on the subset whose rule id is in errorRules.
	// (oxlint -D does NOT override a JS-plugin rule's config severity, so we
	// gate on errorRules membership rather than oxlint severity.)
	errSet := map[string]bool{}
	for _, r := range cfg.ErrorRules {
		errSet[r] = true
	}
	if len(errSet) == 0 {
		os.Exit(0)
	}

	cmd := exec.Command("oxlint", "--format=json", path)
	out, _ := cmd.Output()

	var res oxlintResult
	if json.Unmarshal(out, &res) != nil {
		os.Exit(0)
	}
	var msgs []string
	for _, d := range res.Diagnostics {
		if rule := convexRuleID(d.Code); rule != "" && errSet[rule] {
			msgs = append(msgs, fmt.Sprintf("  ✗ %s — %s", d.Code, d.Message))
		}
	}
	if len(msgs) > 0 {
		fmt.Fprintf(os.Stderr, "\n❌ BLOCKED: Convex lint violation in %s\n%s\n\nFix the issue (split the file, add returns:, etc.) — see convex/REFACTORING.md.\n",
			filepath.Base(path), strings.Join(msgs, "\n"))
		os.Exit(2)
	}
	os.Exit(0)
}
