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
	Severity     string   `json:"severity"`
	CrudDomains  []string `json:"crudDomains"`
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

func containsAny(path string, subs []string) bool {
	f := strings.ReplaceAll(path, "\\", "/")
	for _, s := range subs {
		if s != "" && strings.Contains(f, s) {
			return true
		}
	}
	return false
}

var allConvexRules = []string{
	"file-size", "max-functions", "require-returns", "no-any-returns",
	"no-api-imports", "type-exports-location", "no-filter-in-query",
	"no-collect-in-query",
}

type oxlintResult struct {
	Diagnostics []struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"diagnostics"`
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

	var raw struct {
		ConvexCheckConfig convexCfg `json:"convexCheckConfig"`
	}
	// jsonc.Unmarshal reads .pre-commit.json from CWD; the hook runs in the
	// project root. If it can't be read, stay silent (allow).
	if err := jsonc.Unmarshal(".pre-commit.json", &raw); err != nil {
		os.Exit(0)
	}
	cfg := raw.ConvexCheckConfig
	if !isConvexTarget(path, cfg.AppPaths, cfg.ExcludePaths) {
		os.Exit(0)
	}

	// Decide which rules to enforce at edit time.
	var rules []string
	if cfg.Severity == "error" {
		rules = append(append([]string{}, allConvexRules...), "crud-structure")
	} else if containsAny(path, cfg.CrudDomains) {
		rules = []string{"crud-structure"}
	} else {
		os.Exit(0) // dormant for this file
	}

	args := []string{"--format=json"}
	for _, r := range rules {
		args = append(args, "-D", "convex/"+r)
	}
	args = append(args, path)
	cmd := exec.Command("oxlint", args...)
	out, _ := cmd.Output()

	var res oxlintResult
	if json.Unmarshal(out, &res) != nil {
		os.Exit(0)
	}
	var msgs []string
	for _, d := range res.Diagnostics {
		if strings.HasPrefix(d.Code, "convex(") {
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
