package schemachecks

import (
	"bytes"
	"os"
	"path/filepath"
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
			if got := IsSchemaFile(tt.path); got != tt.want {
				t.Errorf("IsSchemaFile(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestDefineTableBlocks(t *testing.T) {
	src := `
export const users = defineTable({
  email: v.string(),
});
defineTable({
  metadata: v.object({
    nested: v.string(),
    deeper: v.object({ x: v.number() }),
  }),
  status: v.string(),
});
`
	blocks := DefineTableBlocks(src)
	if len(blocks) != 2 {
		t.Fatalf("want 2 blocks, got %d", len(blocks))
	}
	if !bytesContains(blocks[0], "email: v.string()") {
		t.Errorf("block 0 missing email: %q", blocks[0])
	}
	if !bytesContains(blocks[1], "metadata:") || !bytesContains(blocks[1], "status:") {
		t.Errorf("block 1 missing expected fields: %q", blocks[1])
	}
}

func TestCountCreatedAt(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want int
	}{
		{"empty", "", 0},
		{"no defineTable", `const foo = { createdAt: v.number() };`, 0},
		{"one createdAt in defineTable", `defineTable({ createdAt: v.number() });`, 1},
		{"two tables, each with createdAt", `
defineTable({ createdAt: v.number(), name: v.string() });
defineTable({ slug: v.string(), createdAt: v.string() });
`, 2},
		{"createdAt in line comment ignored", `
defineTable({
  // createdAt: v.number(),
  name: v.string(),
});
`, 0},
		{"createdAt in block comment ignored", `
defineTable({
  /* createdAt: v.number() */
  name: v.string(),
});
`, 0},
		{"whitespace variations matched", `
defineTable({
    createdAt   :   v.number(),
});
`, 1},
		{"notCreatedAt isn't matched", `defineTable({ notCreatedAt: v.number() });`, 0},
		{"createdAt outside defineTable isn't counted", `
const helper = { createdAt: v.number() };
defineTable({ name: v.string() });
`, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CountCreatedAt(tt.src); got != tt.want {
				t.Errorf("CountCreatedAt() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestHasRedundantCreatedAt(t *testing.T) {
	if HasRedundantCreatedAt(``) {
		t.Error("empty source shouldn't be flagged")
	}
	if !HasRedundantCreatedAt(`defineTable({ createdAt: v.number() });`) {
		t.Error("defineTable with createdAt should be flagged")
	}
	if HasRedundantCreatedAt(`defineTable({ name: v.string() });`) {
		t.Error("clean defineTable shouldn't be flagged")
	}
}

func TestListAndFind(t *testing.T) {
	// Scaffold: /tmp/.../packages/backend/convex/schema/{a,b,c}.ts plus one
	// unrelated file that should not be flagged.
	root := t.TempDir()
	schemaDir := filepath.Join(root, "packages", "backend", "convex", "schema")
	if err := os.MkdirAll(schemaDir, 0o755); err != nil {
		t.Fatal(err)
	}

	mustWrite := func(path, content string) {
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mustWrite(filepath.Join(schemaDir, "bad.ts"), `defineTable({ createdAt: v.number() });`)
	mustWrite(filepath.Join(schemaDir, "clean.ts"), `defineTable({ name: v.string() });`)
	mustWrite(filepath.Join(schemaDir, "worse.ts"), `
defineTable({ createdAt: v.number() });
defineTable({ createdAt: v.string() });
`)
	// Non-schema file with createdAt: should not be flagged by Find.
	unrelated := filepath.Join(root, "packages", "backend", "convex", "users", "usersQueries.ts")
	if err := os.MkdirAll(filepath.Dir(unrelated), 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(unrelated, `defineTable({ createdAt: v.number() });`)

	found, err := Find(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 2 {
		t.Fatalf("want 2 violating files, got %d: %v", len(found), found)
	}

	// List returns the same count and writes paths in deterministic (walk) order.
	var buf bytes.Buffer
	count, err := List(root, &buf)
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("List count = %d, want 2", count)
	}
	if !bytesContains(buf.String(), "bad.ts") || !bytesContains(buf.String(), "worse.ts") {
		t.Errorf("List output missing expected files: %q", buf.String())
	}
	if bytesContains(buf.String(), "clean.ts") || bytesContains(buf.String(), "usersQueries.ts") {
		t.Errorf("List output included files that should have been skipped: %q", buf.String())
	}
}

func TestCheckFile(t *testing.T) {
	tmpDir := t.TempDir()
	schemaDir := filepath.Join(tmpDir, "packages", "backend", "convex", "schema")
	if err := os.MkdirAll(schemaDir, 0o755); err != nil {
		t.Fatal(err)
	}
	dirty := filepath.Join(schemaDir, "dirty.ts")
	clean := filepath.Join(schemaDir, "clean.ts")
	if err := os.WriteFile(dirty, []byte(`defineTable({ createdAt: v.number() });`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(clean, []byte(`defineTable({ name: v.string() });`), 0o644); err != nil {
		t.Fatal(err)
	}

	if !CheckFile(dirty) {
		t.Error("CheckFile(dirty) should return true")
	}
	if CheckFile(clean) {
		t.Error("CheckFile(clean) should return false")
	}
	if CheckFile(filepath.Join(tmpDir, "missing.ts")) {
		t.Error("CheckFile(missing) should return false, not error")
	}
	// Non-schema file shouldn't be checked even if it contains createdAt.
	unrelated := filepath.Join(tmpDir, "packages", "backend", "convex", "users.ts")
	_ = os.MkdirAll(filepath.Dir(unrelated), 0o755)
	_ = os.WriteFile(unrelated, []byte(`defineTable({ createdAt: v.number() });`), 0o644)
	if CheckFile(unrelated) {
		t.Error("CheckFile on non-schema file should return false")
	}
}

func bytesContains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
