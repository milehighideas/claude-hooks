package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsConvexTarget(t *testing.T) {
	appPaths := []string{"packages/backend/convex"}
	exclude := []string{"_generated/"}
	cases := map[string]bool{
		"/r/packages/backend/convex/events/eventsQueries.ts": true,
		"/r/packages/backend/convex/_generated/api.d.ts":     false,
		"/r/packages/backend/convex/x.test.ts":               false,
		"/r/apps/story/foo.ts":                               false,
	}
	for path, want := range cases {
		if got := isConvexTarget(path, appPaths, exclude); got != want {
			t.Errorf("isConvexTarget(%q) = %v, want %v", path, got, want)
		}
	}
}

// TestProjectedContent covers the fix that makes the ratchet evaluate the
// PROPOSED edit result instead of the on-disk file — without it a file with an
// existing violation can never be fixed through Edit.
func TestProjectedContent(t *testing.T) {
	// Write returns its content verbatim.
	var in hookInput
	in.ToolName = "Write"
	in.ToolInput.Content = "hello"
	if got, ok := projectedContent(in); !ok || got != "hello" {
		t.Errorf("Write projection = %q,%v; want hello,true", got, ok)
	}

	// Write with empty content is not projectable (caller lints on-disk).
	in.ToolInput.Content = ""
	if _, ok := projectedContent(in); ok {
		t.Error("Write with empty content should not project")
	}

	// Edit applies old_string -> new_string against the on-disk file.
	tmp := filepath.Join(t.TempDir(), "f.ts")
	if err := os.WriteFile(tmp, []byte("export interface X {}\nkeep"), 0o600); err != nil {
		t.Fatal(err)
	}
	in = hookInput{}
	in.ToolName = "Edit"
	in.ToolInput.FilePath = tmp
	in.ToolInput.OldString = "export interface X {}"
	in.ToolInput.NewString = "export * from './x.types'"
	if got, ok := projectedContent(in); !ok || got != "export * from './x.types'\nkeep" {
		t.Errorf("Edit projection = %q,%v", got, ok)
	}

	// Edit whose old_string is absent is not projectable (fall back to on-disk;
	// the Edit tool itself surfaces the no-match error).
	in.ToolInput.OldString = "nonexistent"
	if _, ok := projectedContent(in); ok {
		t.Error("Edit with missing old_string should not project")
	}

	// replace_all replaces every occurrence.
	if err := os.WriteFile(tmp, []byte("a a a"), 0o600); err != nil {
		t.Fatal(err)
	}
	in.ToolInput.OldString = "a"
	in.ToolInput.NewString = "b"
	in.ToolInput.ReplaceAll = true
	if got, _ := projectedContent(in); got != "b b b" {
		t.Errorf("replace_all projection = %q; want %q", got, "b b b")
	}
}

