package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/milehighideas/claude-hooks/internal/schemachecks"
)

// Schema-file detection and defineTable parsing are thoroughly tested in
// internal/schemachecks/createdat_test.go. Tests here cover main-specific
// behavior: tool-payload handling, env-var gating, and the violation walker.

func TestResultingContent(t *testing.T) {
	tmpDir := t.TempDir()

	existing := filepath.Join(tmpDir, "schema.ts")
	if err := os.WriteFile(existing, []byte(`defineTable({ name: v.string() });`), 0644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	tests := []struct {
		name    string
		data    HookData
		want    string
		wantErr bool
	}{
		{
			name: "Write returns content verbatim",
			data: HookData{
				ToolName: "Write",
				ToolInput: ToolInput{
					FilePath: "/virtual/new-schema.ts",
					Content:  "defineTable({ created: true });",
				},
			},
			want: "defineTable({ created: true });",
		},
		{
			name: "Edit applies replacement to existing file",
			data: HookData{
				ToolName: "Edit",
				ToolInput: ToolInput{
					FilePath:  existing,
					OldString: "name: v.string()",
					NewString: "name: v.string(), createdAt: v.number()",
				},
			},
			want: "defineTable({ name: v.string(), createdAt: v.number() });",
		},
		{
			name: "Edit on missing file errors",
			data: HookData{
				ToolName: "Edit",
				ToolInput: ToolInput{
					FilePath: filepath.Join(tmpDir, "missing.ts"),
				},
			},
			wantErr: true,
		},
		{
			name:    "Unknown tool errors",
			data:    HookData{ToolName: "Read"},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resultingContent(tt.data)
			if (err != nil) != tt.wantErr {
				t.Errorf("err = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBlockingDecision(t *testing.T) {
	// End-to-end behavior: when after > before, the caller should block.
	// We exercise the calculation directly since main() calls os.Exit.
	tests := []struct {
		name    string
		before  string
		after   string
		blocked bool
	}{
		{
			name:    "adding a new createdAt blocks",
			before:  `defineTable({ name: v.string() });`,
			after:   `defineTable({ name: v.string(), createdAt: v.number() });`,
			blocked: true,
		},
		{
			name:    "keeping the same createdAt allows",
			before:  `defineTable({ name: v.string(), createdAt: v.number() });`,
			after:   `defineTable({ name: v.string(), createdAt: v.number(), extra: v.string() });`,
			blocked: false,
		},
		{
			name:    "removing createdAt allows",
			before:  `defineTable({ createdAt: v.number(), name: v.string() });`,
			after:   `defineTable({ name: v.string() });`,
			blocked: false,
		},
		{
			name:    "renaming createdAt to activatedAt allows",
			before:  `defineTable({ createdAt: v.number() });`,
			after:   `defineTable({ activatedAt: v.number() });`,
			blocked: false,
		},
		{
			name:    "new file introducing a fresh createdAt blocks",
			before:  ``,
			after:   `defineTable({ createdAt: v.number() });`,
			blocked: true,
		},
		{
			name:    "empty file, empty edit — no change allows",
			before:  ``,
			after:   ``,
			blocked: false,
		},
		{
			// Widen-to-optional with @deprecated JSDoc is exempt, so the count
			// doesn't increase and the edit is allowed.
			name:   "widening required createdAt to optional+@deprecated allows",
			before: `defineTable({ createdAt: v.string(), name: v.string() });`,
			after: `defineTable({
  /** @deprecated use _creationTime */
  createdAt: v.optional(v.string()),
  name: v.string(),
});`,
			blocked: false,
		},
		{
			// Inline hooks-allow marker is equally valid as an opt-out.
			name:    "adding optional createdAt with hooks-allow marker allows",
			before:  `defineTable({ name: v.string() });`,
			after:   `defineTable({ createdAt: v.optional(v.string()), /* hooks-allow: redundant-createdat */ name: v.string() });`,
			blocked: false,
		},
		{
			// Bare v.optional() without an intent marker is still flagged.
			name:    "bare optional createdAt without marker still blocks",
			before:  `defineTable({ name: v.string() });`,
			after:   `defineTable({ createdAt: v.optional(v.string()), name: v.string() });`,
			blocked: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := schemachecks.CountCreatedAt(tt.before)
			a := schemachecks.CountCreatedAt(tt.after)
			blocked := a > b
			if blocked != tt.blocked {
				t.Errorf("before=%d after=%d → blocked=%v, want %v", b, a, blocked, tt.blocked)
			}
		})
	}
}

func TestCheckDisabled(t *testing.T) {
	original := os.Getenv("CLAUDE_HOOKS_AST_VALIDATION")
	defer func() {
		if original != "" {
			_ = os.Setenv("CLAUDE_HOOKS_AST_VALIDATION", original)
		} else {
			_ = os.Unsetenv("CLAUDE_HOOKS_AST_VALIDATION")
		}
	}()

	_ = os.Setenv("CLAUDE_HOOKS_AST_VALIDATION", "false")
	if !checkDisabled() {
		t.Error("expected disabled when CLAUDE_HOOKS_AST_VALIDATION=false")
	}
	_ = os.Setenv("CLAUDE_HOOKS_AST_VALIDATION", "true")
	if checkDisabled() {
		t.Error("expected enabled when CLAUDE_HOOKS_AST_VALIDATION=true")
	}
	_ = os.Unsetenv("CLAUDE_HOOKS_AST_VALIDATION")
	if checkDisabled() {
		t.Error("expected enabled when env unset")
	}
}

func TestListViolations(t *testing.T) {
	tmpDir := t.TempDir()

	// Fake repo layout with three schema files — one clean, two violators —
	// plus a non-schema file to confirm it's ignored.
	schemaDir := filepath.Join(tmpDir, "packages", "backend", "convex", "schema")
	if err := os.MkdirAll(schemaDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	cleanFile := filepath.Join(schemaDir, "clean.ts")
	if err := os.WriteFile(cleanFile, []byte(`defineTable({ name: v.string() });`), 0644); err != nil {
		t.Fatalf("write clean: %v", err)
	}
	dirty1 := filepath.Join(schemaDir, "dirty1.ts")
	if err := os.WriteFile(dirty1, []byte(`defineTable({ createdAt: v.number(), name: v.string() });`), 0644); err != nil {
		t.Fatalf("write dirty1: %v", err)
	}
	dirty2 := filepath.Join(schemaDir, "dirty2.ts")
	if err := os.WriteFile(dirty2, []byte(`
defineTable({ createdAt: v.number() });
defineTable({ createdAt: v.string(), slug: v.string() });
`), 0644); err != nil {
		t.Fatalf("write dirty2: %v", err)
	}
	// File with exempt createdAt — should NOT be flagged.
	exempt := filepath.Join(schemaDir, "exempt.ts")
	if err := os.WriteFile(exempt, []byte(`defineTable({
  /** @deprecated use _creationTime */
  createdAt: v.optional(v.string()),
  name: v.string(),
});`), 0644); err != nil {
		t.Fatalf("write exempt: %v", err)
	}

	// Non-schema file at the same level — must be skipped.
	other := filepath.Join(tmpDir, "packages", "backend", "convex", "users.ts")
	if err := os.WriteFile(other, []byte(`const foo = { createdAt: v.number() };`), 0644); err != nil {
		t.Fatalf("write other: %v", err)
	}

	// node_modules should be skipped entirely.
	nmSchema := filepath.Join(tmpDir, "node_modules", "convex", "schema", "fake.ts")
	if err := os.MkdirAll(filepath.Dir(nmSchema), 0755); err != nil {
		t.Fatalf("mkdir nm: %v", err)
	}
	if err := os.WriteFile(nmSchema, []byte(`defineTable({ createdAt: v.number() });`), 0644); err != nil {
		t.Fatalf("write nm: %v", err)
	}

	var buf bytes.Buffer
	count, err := listViolations([]string{tmpDir}, &buf)
	if err != nil {
		t.Fatalf("listViolations: %v", err)
	}
	if count != 2 {
		t.Errorf("count = %d, want 2", count)
	}
	output := buf.String()
	if !strings.Contains(output, "dirty1.ts\t1") {
		t.Errorf("output missing dirty1 line with count 1:\n%s", output)
	}
	if !strings.Contains(output, "dirty2.ts\t2") {
		t.Errorf("output missing dirty2 line with count 2:\n%s", output)
	}
	if strings.Contains(output, "clean.ts") {
		t.Errorf("clean.ts should not appear in output:\n%s", output)
	}
	if strings.Contains(output, "exempt.ts") {
		t.Errorf("exempt.ts (with @deprecated marker) should not appear in output:\n%s", output)
	}
	if strings.Contains(output, "node_modules") {
		t.Errorf("node_modules should be skipped:\n%s", output)
	}
}

func TestListViolations_MissingRoot(t *testing.T) {
	// Missing roots are silently skipped so the walker can cross multiple
	// roots in one call.
	var buf bytes.Buffer
	count, err := listViolations([]string{"/definitely/does/not/exist"}, &buf)
	if err != nil {
		t.Errorf("want nil err for missing root, got %v", err)
	}
	if count != 0 {
		t.Errorf("count = %d, want 0", count)
	}
}
