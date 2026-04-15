package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIsSchemaFile(t *testing.T) {
	tests := []struct {
		name string
		path string
		want bool
	}{
		{"convex schema dir", "/repo/packages/backend/convex/schema/users.ts", true},
		{"convex schema dir nested", "/repo/packages/backend/convex/schema/events/core.ts", true},
		{"single-file schema.ts", "/repo/packages/backend/convex/schema.ts", true},
		{"single-file schema.tsx", "/repo/packages/backend/convex/schema.tsx", true},
		{"unrelated convex file", "/repo/packages/backend/convex/users/usersQueries.ts", false},
		{"components dir", "/repo/apps/story/components/Foo.tsx", false},
		{"empty", "", false},
		{"windows schema dir", `C:\repo\packages\backend\convex\schema\users.ts`, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isSchemaFile(tt.path); got != tt.want {
				t.Errorf("isSchemaFile(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestExtractDefineTableBlocks(t *testing.T) {
	src := `
export const users = defineTable({
  email: v.string(),
  name: v.string(),
});

const helper = { foo: "bar" };

export const posts = defineTable({
  userId: v.id("users"),
  title: v.string(),
});
`
	blocks := extractDefineTableBlocks(src)
	if len(blocks) != 2 {
		t.Fatalf("want 2 blocks, got %d", len(blocks))
	}
	if !contains(blocks[0], "email: v.string()") {
		t.Errorf("block 0 missing email field: %q", blocks[0])
	}
	if !contains(blocks[1], `userId: v.id("users")`) {
		t.Errorf("block 1 missing userId field: %q", blocks[1])
	}
}

func TestExtractDefineTableBlocks_NestedObjects(t *testing.T) {
	// defineTable blocks can contain nested objects (v.object, v.union, etc).
	// The brace-depth walker must not stop at the first inner `}`.
	src := `
defineTable({
  metadata: v.object({
    nested: v.string(),
    deeper: v.object({ x: v.number() }),
  }),
  status: v.string(),
});
`
	blocks := extractDefineTableBlocks(src)
	if len(blocks) != 1 {
		t.Fatalf("want 1 block, got %d", len(blocks))
	}
	// The block must contain both the outer fields and the nested objects.
	if !contains(blocks[0], "metadata:") ||
		!contains(blocks[0], "status:") ||
		!contains(blocks[0], "deeper:") {
		t.Errorf("block missing expected fields: %q", blocks[0])
	}
}

func TestCountCreatedAtInDefineTable(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want int
	}{
		{
			name: "no defineTable at all",
			src:  `const foo = { createdAt: v.number() };`,
			want: 0,
		},
		{
			name: "createdAt outside defineTable",
			src: `
const foo = { createdAt: v.number() };
defineTable({ name: v.string() });
`,
			want: 0,
		},
		{
			name: "one createdAt inside defineTable",
			src: `
defineTable({
  name: v.string(),
  createdAt: v.number(),
});
`,
			want: 1,
		},
		{
			name: "two tables each with createdAt",
			src: `
defineTable({ createdAt: v.number(), name: v.string() });
defineTable({ createdAt: v.string(), slug: v.string() });
`,
			want: 2,
		},
		{
			name: "createdAt in comment is ignored",
			src: `
defineTable({
  // createdAt: v.number(),  — removed in favor of _creationTime
  name: v.string(),
});
`,
			want: 0,
		},
		{
			name: "createdAt in block comment is ignored",
			src: `
defineTable({
  /*
   * createdAt: v.number()
   */
  name: v.string(),
});
`,
			want: 0,
		},
		{
			name: "whitespace variations still matched",
			src: `
defineTable({
    createdAt   :   v.number(),
});
`,
			want: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := countCreatedAtInDefineTable(tt.src); got != tt.want {
				t.Errorf("got %d, want %d", got, tt.want)
			}
		})
	}
}

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
			name: "Unknown tool errors",
			data: HookData{ToolName: "Read"},
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
		name  string
		before string
		after  string
		blocked bool
	}{
		{
			name: "adding a new createdAt blocks",
			before: `defineTable({ name: v.string() });`,
			after:  `defineTable({ name: v.string(), createdAt: v.number() });`,
			blocked: true,
		},
		{
			name: "keeping the same createdAt allows",
			before: `defineTable({ name: v.string(), createdAt: v.number() });`,
			after:  `defineTable({ name: v.string(), createdAt: v.number(), extra: v.string() });`,
			blocked: false,
		},
		{
			name: "removing createdAt allows",
			before: `defineTable({ createdAt: v.number(), name: v.string() });`,
			after:  `defineTable({ name: v.string() });`,
			blocked: false,
		},
		{
			name: "renaming createdAt to activatedAt allows",
			before: `defineTable({ createdAt: v.number() });`,
			after:  `defineTable({ activatedAt: v.number() });`,
			blocked: false,
		},
		{
			name: "new file introducing a fresh createdAt blocks",
			before: ``,
			after:  `defineTable({ createdAt: v.number() });`,
			blocked: true,
		},
		{
			name: "empty file, empty edit — no change allows",
			before: ``,
			after:  ``,
			blocked: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := countCreatedAtInDefineTable(tt.before)
			a := countCreatedAtInDefineTable(tt.after)
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

// contains is a tiny helper so tests don't need to import strings for a
// single call.
func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
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
