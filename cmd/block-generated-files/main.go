package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

type HookInput struct {
	ToolName  string                 `json:"tool_name"`
	ToolInput map[string]interface{} `json:"tool_input"`
}

type HookOutput struct {
	Decision string `json:"decision"`
	Reason   string `json:"reason,omitempty"`
}

// Protected paths that should not be modified directly.
// These are auto-generated directories managed by tooling (e.g. convex-gen).
var protectedPaths = []string{
	"packages/data-layer/src/generated-hooks",
	"packages/data-layer/src/generated-api",
	"packages/data-layer/src/generated-types",
}

func main() {
	if err := run(os.Stdin, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run(stdin io.Reader, stdout io.Writer) error {
	input, err := readHookInput(stdin)
	if err != nil {
		return fmt.Errorf("reading input: %w", err)
	}

	output := processHook(input)

	if err := writeHookOutput(stdout, output); err != nil {
		return fmt.Errorf("writing output: %w", err)
	}

	return nil
}

func readHookInput(r io.Reader) (*HookInput, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("reading stdin: %w", err)
	}

	var input HookInput
	if err := json.Unmarshal(data, &input); err != nil {
		return nil, fmt.Errorf("unmarshaling JSON: %w", err)
	}

	return &input, nil
}

func writeHookOutput(w io.Writer, output *HookOutput) error {
	data, err := json.Marshal(output)
	if err != nil {
		return fmt.Errorf("marshaling output: %w", err)
	}

	if _, err := w.Write(data); err != nil {
		return fmt.Errorf("writing output: %w", err)
	}

	return nil
}

func processHook(input *HookInput) *HookOutput {
	switch input.ToolName {
	case "Edit", "Write":
		return checkFilePath(getFilePath(input.ToolInput))
	case "Bash":
		return checkBashCommand(getCommand(input.ToolInput))
	default:
		return approve()
	}
}

func checkFilePath(filePath string) *HookOutput {
	if filePath == "" {
		return approve()
	}

	for _, protected := range protectedPaths {
		if strings.Contains(filePath, protected) {
			return block(protected)
		}
	}

	return approve()
}

func checkBashCommand(command string) *HookOutput {
	if command == "" {
		return approve()
	}

	for _, protected := range protectedPaths {
		if !strings.Contains(command, protected) {
			continue
		}

		// Only block destructive commands targeting protected paths
		destructive := []string{"rm ", "rm\t", "mv ", "mv\t", "> ", "truncate "}
		for _, prefix := range destructive {
			if strings.Contains(command, prefix) {
				return block(protected)
			}
		}
	}

	return approve()
}

func getFilePath(toolInput map[string]interface{}) string {
	if filePath, ok := toolInput["file_path"].(string); ok {
		return filePath
	}
	return ""
}

func getCommand(toolInput map[string]interface{}) string {
	if command, ok := toolInput["command"].(string); ok {
		return command
	}
	return ""
}

func approve() *HookOutput {
	return &HookOutput{Decision: "approve"}
}

func block(protectedPath string) *HookOutput {
	return &HookOutput{
		Decision: "block",
		Reason: fmt.Sprintf(`BLOCKED: Attempted modification of generated files

Path: %s

This directory contains auto-generated files managed by convex-gen.
Do not modify, create, or delete files here directly.
They are regenerated automatically during pre-commit.`, protectedPath),
	}
}
