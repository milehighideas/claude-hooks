// block-pre-commit-exceptions is a Claude Code PreToolUse hook that prevents
// agents from weakening the pre-commit gate by editing .pre-commit.json.
//
// Two escape hatches are blocked:
//
//  1. Adding any new entry to any "excludePaths" array (anywhere in the tree).
//     Agents hit a failing check like missingTestsCheck and are tempted to
//     paper over it by excluding the offending path. That defeats the gate.
//
//  2. Flipping any "features.*" boolean from true to false. Same logic at
//     the feature level — if a check is annoying, disable it.
//
// Additions to excludePaths that are legitimate (e.g. a genuinely generated
// directory) should be made by a human editing the file directly. There is
// no sentinel or bypass — agents learn sentinels.
//
// The hook is a no-op for any file that isn't .pre-commit.json, and for any
// tool other than Edit / Write.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/milehighideas/claude-hooks/internal/jsonc"
)

type hookInput struct {
	ToolName  string                 `json:"tool_name"`
	ToolInput map[string]interface{} `json:"tool_input"`
}

type hookOutput struct {
	Decision string `json:"decision"`
	Reason   string `json:"reason,omitempty"`
}

func main() {
	if err := run(os.Stdin, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "block-pre-commit-exceptions: %v\n", err)
		os.Exit(1)
	}
}

func run(stdin io.Reader, stdout io.Writer) error {
	in, err := readInput(stdin)
	if err != nil {
		return err
	}
	out := evaluate(in, os.ReadFile)
	return writeOutput(stdout, out)
}

func readInput(r io.Reader) (*hookInput, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("reading stdin: %w", err)
	}
	var in hookInput
	if err := json.Unmarshal(data, &in); err != nil {
		return nil, fmt.Errorf("unmarshaling JSON: %w", err)
	}
	return &in, nil
}

func writeOutput(w io.Writer, out *hookOutput) error {
	data, err := json.Marshal(out)
	if err != nil {
		return fmt.Errorf("marshaling output: %w", err)
	}
	_, err = w.Write(data)
	return err
}

// readFileFn is injected so tests don't need to touch disk.
type readFileFn func(string) ([]byte, error)

func evaluate(in *hookInput, readFile readFileFn) *hookOutput {
	if in.ToolName != "Edit" && in.ToolName != "Write" {
		return approve()
	}

	filePath, _ := in.ToolInput["file_path"].(string)
	if filepath.Base(filePath) != ".pre-commit.json" {
		return approve()
	}

	oldContent, _ := readFile(filePath)

	newContent, ok := resultingContent(in, oldContent)
	if !ok {
		// Couldn't compute the post-edit content (malformed input). Approve
		// and let the tool layer surface whatever error it surfaces.
		return approve()
	}

	oldClean := jsonc.StripComments(oldContent)
	newClean := jsonc.StripComments(newContent)

	var oldJSON, newJSON interface{}
	if len(oldClean) > 0 {
		_ = json.Unmarshal(oldClean, &oldJSON)
	}
	if err := json.Unmarshal(newClean, &newJSON); err != nil {
		// Don't block on an edit that produces invalid JSON — other tooling
		// will flag that. Our job is to catch well-formed escape hatches.
		return approve()
	}

	if added := newExcludePaths(oldJSON, newJSON); len(added) > 0 {
		return blockExcludePaths(added)
	}

	if disabled := disabledFeatures(oldJSON, newJSON); len(disabled) > 0 {
		return blockFeatureToggle(disabled)
	}

	return approve()
}

func approve() *hookOutput {
	return &hookOutput{Decision: "approve"}
}

// resultingContent returns the file content after the tool call would apply,
// and a flag indicating whether the computation succeeded.
func resultingContent(in *hookInput, oldContent []byte) ([]byte, bool) {
	switch in.ToolName {
	case "Write":
		content, ok := in.ToolInput["content"].(string)
		if !ok {
			return nil, false
		}
		return []byte(content), true
	case "Edit":
		oldStr, ok1 := in.ToolInput["old_string"].(string)
		newStr, ok2 := in.ToolInput["new_string"].(string)
		if !ok1 || !ok2 {
			return nil, false
		}
		replaceAll, _ := in.ToolInput["replace_all"].(bool)
		s := string(oldContent)
		if replaceAll {
			return []byte(strings.ReplaceAll(s, oldStr, newStr)), true
		}
		return []byte(strings.Replace(s, oldStr, newStr, 1)), true
	}
	return nil, false
}

// collectExcludePaths walks the JSON tree and returns the set of string
// entries found in any array under a key named "excludePaths".
func collectExcludePaths(node interface{}) map[string]struct{} {
	found := map[string]struct{}{}
	walk(node, func(key string, value interface{}) {
		if key != "excludePaths" {
			return
		}
		arr, ok := value.([]interface{})
		if !ok {
			return
		}
		for _, v := range arr {
			if s, ok := v.(string); ok {
				found[s] = struct{}{}
			}
		}
	})
	return found
}

// newExcludePaths returns the entries present in new but missing from old,
// sorted for deterministic output.
func newExcludePaths(oldJSON, newJSON interface{}) []string {
	oldSet := collectExcludePaths(oldJSON)
	newSet := collectExcludePaths(newJSON)
	var added []string
	for s := range newSet {
		if _, ok := oldSet[s]; !ok {
			added = append(added, s)
		}
	}
	return sortedUnique(added)
}

// disabledFeatures returns the names of feature flags under a "features"
// object that changed from true (or unset) to false between old and new.
func disabledFeatures(oldJSON, newJSON interface{}) []string {
	oldFeatures := collectFeatures(oldJSON)
	newFeatures := collectFeatures(newJSON)
	var disabled []string
	for name, newVal := range newFeatures {
		if newVal {
			continue
		}
		// Newly false. Flag unless the old value was also explicitly false.
		if oldVal, had := oldFeatures[name]; had && !oldVal {
			continue
		}
		disabled = append(disabled, name)
	}
	return sortedUnique(disabled)
}

func collectFeatures(node interface{}) map[string]bool {
	out := map[string]bool{}
	walk(node, func(key string, value interface{}) {
		if key != "features" {
			return
		}
		obj, ok := value.(map[string]interface{})
		if !ok {
			return
		}
		for k, v := range obj {
			if b, ok := v.(bool); ok {
				out[k] = b
			}
		}
	})
	return out
}

// walk visits every (key, value) pair in the JSON tree, descending into
// objects and arrays. Root values are visited with key "".
func walk(node interface{}, visit func(key string, value interface{})) {
	switch n := node.(type) {
	case map[string]interface{}:
		for k, v := range n {
			visit(k, v)
			walk(v, visit)
		}
	case []interface{}:
		for _, v := range n {
			walk(v, visit)
		}
	}
}

func sortedUnique(in []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, s := range in {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	// Simple insertion sort — lists are tiny (one or two entries in practice).
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

func blockExcludePaths(added []string) *hookOutput {
	return &hookOutput{
		Decision: "block",
		Reason: fmt.Sprintf(`BLOCKED: adding entries to excludePaths in .pre-commit.json

New entries:
  - %s

Adding a path to excludePaths suppresses a check that's doing its job.
The pre-commit gate exists so that broken / untested code doesn't land —
excluding the offending path defeats the gate.

If the check is failing:
  1. Fix the underlying issue (write the test, remove the stub, etc.).
  2. If the exclusion is genuinely warranted (vendored code, generated
     files, a directory that can never be tested), a human must edit
     .pre-commit.json directly. Agents do not add exceptions.`,
			strings.Join(added, "\n  - ")),
	}
}

func blockFeatureToggle(disabled []string) *hookOutput {
	return &hookOutput{
		Decision: "block",
		Reason: fmt.Sprintf(`BLOCKED: disabling pre-commit feature(s) in .pre-commit.json

Disabled: %s

Flipping features.* from true to false turns off a check entirely, which
is a stronger escape hatch than excludePaths. If a check is failing, fix
the underlying issue. If a feature should genuinely be turned off, a
human must make that change directly.`, strings.Join(disabled, ", ")),
	}
}
