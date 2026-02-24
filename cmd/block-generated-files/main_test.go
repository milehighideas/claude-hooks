package main

import (
	"testing"
)

func TestProcessHook(t *testing.T) {
	tests := []struct {
		name     string
		input    HookInput
		decision string
	}{
		// === Allowed operations ===
		{
			name: "Edit regular file",
			input: HookInput{
				ToolName: "Edit",
				ToolInput: map[string]interface{}{
					"file_path": "/project/packages/data-layer/src/hooks/useMyHook.ts",
				},
			},
			decision: "approve",
		},
		{
			name: "Write regular file",
			input: HookInput{
				ToolName: "Write",
				ToolInput: map[string]interface{}{
					"file_path": "/project/packages/data-layer/src/types/foo.ts",
				},
			},
			decision: "approve",
		},
		{
			name: "Bash ls generated-hooks",
			input: HookInput{
				ToolName: "Bash",
				ToolInput: map[string]interface{}{
					"command": "ls packages/data-layer/src/generated-hooks/",
				},
			},
			decision: "approve",
		},
		{
			name: "Bash cat generated-hooks file",
			input: HookInput{
				ToolName: "Bash",
				ToolInput: map[string]interface{}{
					"command": "cat packages/data-layer/src/generated-hooks/index.ts",
				},
			},
			decision: "approve",
		},
		{
			name: "Read tool is ignored",
			input: HookInput{
				ToolName: "Read",
				ToolInput: map[string]interface{}{
					"file_path": "/project/packages/data-layer/src/generated-hooks/index.ts",
				},
			},
			decision: "approve",
		},
		{
			name: "empty file path",
			input: HookInput{
				ToolName: "Edit",
				ToolInput: map[string]interface{}{},
			},
			decision: "approve",
		},
		{
			name: "empty command",
			input: HookInput{
				ToolName: "Bash",
				ToolInput: map[string]interface{}{},
			},
			decision: "approve",
		},

		// === Blocked operations ===
		{
			name: "Edit generated-hooks file",
			input: HookInput{
				ToolName: "Edit",
				ToolInput: map[string]interface{}{
					"file_path": "/project/packages/data-layer/src/generated-hooks/index.ts",
				},
			},
			decision: "block",
		},
		{
			name: "Write generated-hooks file",
			input: HookInput{
				ToolName: "Write",
				ToolInput: map[string]interface{}{
					"file_path": "/project/packages/data-layer/src/generated-hooks/useProjects.ts",
				},
			},
			decision: "block",
		},
		{
			name: "Edit nested generated-hooks file",
			input: HookInput{
				ToolName: "Edit",
				ToolInput: map[string]interface{}{
					"file_path": "/project/packages/data-layer/src/generated-hooks/deep/nested/file.ts",
				},
			},
			decision: "block",
		},
		{
			name: "Bash rm generated-hooks file",
			input: HookInput{
				ToolName: "Bash",
				ToolInput: map[string]interface{}{
					"command": "rm packages/data-layer/src/generated-hooks/index.ts",
				},
			},
			decision: "block",
		},
		{
			name: "Bash rm -rf generated-hooks directory",
			input: HookInput{
				ToolName: "Bash",
				ToolInput: map[string]interface{}{
					"command": "rm -rf packages/data-layer/src/generated-hooks/",
				},
			},
			decision: "block",
		},
		{
			name: "Bash mv generated-hooks file",
			input: HookInput{
				ToolName: "Bash",
				ToolInput: map[string]interface{}{
					"command": "mv packages/data-layer/src/generated-hooks/old.ts packages/data-layer/src/generated-hooks/new.ts",
				},
			},
			decision: "block",
		},
		{
			name: "Bash redirect into generated-hooks",
			input: HookInput{
				ToolName: "Bash",
				ToolInput: map[string]interface{}{
					"command": "echo 'export {}' > packages/data-layer/src/generated-hooks/index.ts",
				},
			},
			decision: "block",
		},
		{
			name: "Bash truncate generated-hooks file",
			input: HookInput{
				ToolName: "Bash",
				ToolInput: map[string]interface{}{
					"command": "truncate -s 0 packages/data-layer/src/generated-hooks/index.ts",
				},
			},
			decision: "block",
		},

		// === generated-api blocked ===
		{
			name: "Edit generated-api file",
			input: HookInput{
				ToolName: "Edit",
				ToolInput: map[string]interface{}{
					"file_path": "/project/packages/data-layer/src/generated-api/projects.ts",
				},
			},
			decision: "block",
		},
		{
			name: "Write generated-api file",
			input: HookInput{
				ToolName: "Write",
				ToolInput: map[string]interface{}{
					"file_path": "/project/packages/data-layer/src/generated-api/index.ts",
				},
			},
			decision: "block",
		},
		{
			name: "Bash rm generated-api file",
			input: HookInput{
				ToolName: "Bash",
				ToolInput: map[string]interface{}{
					"command": "rm packages/data-layer/src/generated-api/projects.ts",
				},
			},
			decision: "block",
		},

		// === generated-types blocked ===
		{
			name: "Edit generated-types file",
			input: HookInput{
				ToolName: "Edit",
				ToolInput: map[string]interface{}{
					"file_path": "/project/packages/data-layer/src/generated-types/convex.ts",
				},
			},
			decision: "block",
		},
		{
			name: "Write generated-types file",
			input: HookInput{
				ToolName: "Write",
				ToolInput: map[string]interface{}{
					"file_path": "/project/packages/data-layer/src/generated-types/index.ts",
				},
			},
			decision: "block",
		},
		{
			name: "Bash rm -rf generated-types directory",
			input: HookInput{
				ToolName: "Bash",
				ToolInput: map[string]interface{}{
					"command": "rm -rf packages/data-layer/src/generated-types/",
				},
			},
			decision: "block",
		},

		// === generated-api and generated-types allowed (read-only) ===
		{
			name: "Bash ls generated-api",
			input: HookInput{
				ToolName: "Bash",
				ToolInput: map[string]interface{}{
					"command": "ls packages/data-layer/src/generated-api/",
				},
			},
			decision: "approve",
		},
		{
			name: "Bash cat generated-types file",
			input: HookInput{
				ToolName: "Bash",
				ToolInput: map[string]interface{}{
					"command": "cat packages/data-layer/src/generated-types/convex.ts",
				},
			},
			decision: "approve",
		},
		{
			name: "Read generated-api is ignored",
			input: HookInput{
				ToolName: "Read",
				ToolInput: map[string]interface{}{
					"file_path": "/project/packages/data-layer/src/generated-api/index.ts",
				},
			},
			decision: "approve",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := processHook(&tt.input)
			if output.Decision != tt.decision {
				t.Errorf("expected decision %q, got %q (reason: %s)", tt.decision, output.Decision, output.Reason)
			}
		})
	}
}
