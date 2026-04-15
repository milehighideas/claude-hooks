package stubs

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIsStub(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    bool
	}{
		{
			name: "canonical stub",
			content: `import { describe, it, expect } from "vitest";
describe("x", () => { it("stub", () => { expect(true).toBe(true); }); });`,
			want: true,
		},
		{
			name: "multiple stubs, all placeholder",
			content: `it("a", () => { expect(true).toBe(true); });
it("b", () => { expect(true).toBe(true); });`,
			want: true,
		},
		{
			name:    "whitespace variation",
			content: `it("stub", () => { expect( true ) . toBe( true ); });`,
			want:    true,
		},
		{
			name: "real test",
			content: `import { render } from "@testing-library/react";
it("renders", () => { expect(screen.getByText("hi")).toBeTruthy(); });`,
			want: false,
		},
		{
			name: "mixed: one stub + one real",
			content: `it("a", () => { expect(true).toBe(true); });
it("b", () => { expect(value).toBe(42); });`,
			want: false,
		},
		{name: "empty file", content: ``, want: false},
		{name: "no expect calls", content: `describe("x", () => { it("a", () => {}); });`, want: false},
		{name: "expect(true).toBe(false)", content: `it("a", () => { expect(true).toBe(false); });`, want: false},
		{
			name: "comment mentioning stub alongside real",
			content: `// avoid expect(true).toBe(true)
it("real", () => { expect(x).toBe(1); });`,
			want: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := IsStub(tc.content)
			if got != tc.want {
				t.Errorf("IsStub() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestList(t *testing.T) {
	root := t.TempDir()

	write := func(rel, content string) string {
		full := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(full, []byte(content), 0644); err != nil {
			t.Fatalf("write: %v", err)
		}
		return full
	}

	stub := write("a/Foo.test.tsx", `it("x", () => { expect(true).toBe(true); });`)
	write("a/Bar.test.tsx", `it("x", () => { expect(actual).toBe(42); });`)
	write("a/Mixed.test.ts", `it("a", () => { expect(true).toBe(true); }); it("b", () => { expect(x).toBe(1); });`)
	write("a/Foo.tsx", `export const Foo = () => null;`)
	write("node_modules/pkg/nm.test.tsx", `it("x", () => { expect(true).toBe(true); });`)
	write("packages/backend/convex/_generated/gen.test.ts", `it("x", () => { expect(true).toBe(true); });`)
	stubNested := write("packages/ui/Button.test.ts", `it("x", () => { expect(true).toBe(true); });`)

	var buf bytes.Buffer
	count, err := List(root, &buf)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if count != 2 {
		t.Errorf("count = %d, want 2 (output: %q)", count, buf.String())
	}
	out := buf.String()
	for _, want := range []string{stub, stubNested} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %s in output, got %q", want, out)
		}
	}
	for _, exclude := range []string{"Bar.test.tsx", "Mixed.test.ts", "node_modules", "_generated"} {
		if strings.Contains(out, exclude) {
			t.Errorf("did not expect %q in output, got %q", exclude, out)
		}
	}
}

func TestFind(t *testing.T) {
	root := t.TempDir()
	stub := filepath.Join(root, "Foo.test.tsx")
	if err := os.WriteFile(stub, []byte(`it("x", () => { expect(true).toBe(true); });`), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "Bar.test.tsx"),
		[]byte(`it("x", () => { expect(y).toBe(1); });`), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	found, err := Find(root)
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if len(found) != 1 {
		t.Fatalf("len = %d, want 1 (got %v)", len(found), found)
	}
	if found[0] != stub {
		t.Errorf("found[0] = %q, want %q", found[0], stub)
	}
}

func TestList_NonexistentPath(t *testing.T) {
	var buf bytes.Buffer
	_, err := List("/nope/does/not/exist", &buf)
	if err == nil {
		t.Error("expected error for nonexistent path, got nil")
	}
}
