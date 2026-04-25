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
			name:    "weak: not.toBeNull only",
			content: `it("renders", () => { const { toJSON } = render(<X/>); expect(toJSON()).not.toBeNull(); });`,
			want:    true,
		},
		{
			name:    "weak: toBeDefined only",
			content: `it("x", () => { expect(result).toBeDefined(); });`,
			want:    true,
		},
		{
			name:    "weak: toBeTruthy only (getByTestId pattern)",
			content: `it("x", () => { const { getByTestId } = render(<X/>); expect(getByTestId('x')).toBeTruthy(); });`,
			want:    true,
		},
		{
			name:    "weak: toBeFalsy only",
			content: `it("x", () => { expect(result).toBeFalsy(); });`,
			want:    true,
		},
		{
			name:    "weak: not.toBeUndefined only",
			content: `it("x", () => { expect(result).not.toBeUndefined(); });`,
			want:    true,
		},
		{
			name:    "weak: toBeOnTheScreen only (getByText pattern)",
			content: `it("renders", () => { render(<X/>); expect(screen.getByText('foo')).toBeOnTheScreen(); });`,
			want:    true,
		},
		{
			name:    "weak: toBeInTheDocument only (getByText pattern)",
			content: `it("renders", () => { render(<X/>); expect(screen.getByText('foo')).toBeInTheDocument(); });`,
			want:    true,
		},
		{
			name:    "weak: toBeOnTheScreen with whitespace variation",
			content: `it("x", () => { expect( getByText('foo') ) . toBeOnTheScreen( ); });`,
			want:    true,
		},
		{
			name: "weak toBeOnTheScreen + real toHaveBeenCalled — NOT a stub",
			content: `it("renders and fires callback", () => {
  const onPress = jest.fn();
  render(<X onPress={onPress}/>);
  expect(queryByText('foo')).toBeOnTheScreen();
  expect(onPress).toHaveBeenCalled();
});`,
			want: false,
		},
		{
			name: "weak toBeInTheDocument + real toBe — NOT a stub",
			content: `it("x", () => {
  expect(getByText('foo')).toBeInTheDocument();
  expect(value).toBe(42);
});`,
			want: false,
		},
		{
			name: "mixed weak: different weak matchers all still count as weak",
			content: `it("a", () => { expect(x).toBeDefined(); });
it("b", () => { expect(y).not.toBeNull(); });`,
			want: true,
		},
		{
			name: "weak followed by real assertion — NOT weak",
			content: `it("renders", () => {
  const result = foo();
  expect(result).toBeDefined();
  expect(result.name).toBe('hello');
});`,
			want: false,
		},
		{
			name:    "toBe with non-placeholder value is a real assertion",
			content: `it("x", () => { expect(result).toBe(42); });`,
			want:    false,
		},
		{
			name:    "toEqual with value is a real assertion",
			content: `it("x", () => { expect(result).toEqual({ a: 1 }); });`,
			want:    false,
		},
		{
			name:    "toHaveBeenCalled is a real assertion",
			content: `it("x", () => { const spy = jest.fn(); foo(spy); expect(spy).toHaveBeenCalled(); });`,
			want:    false,
		},
		{
			name: "real test with non-weak matcher",
			content: `import { render } from "@testing-library/react";
it("renders", () => { expect(screen.getByText("hi").textContent).toBe("hi"); });`,
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

func TestIsStub_toBeVisible(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    bool
	}{
		{
			name:    "weak: toBeVisible only",
			content: `it("renders", () => { render(<X/>); expect(getByText('foo')).toBeVisible(); });`,
			want:    true,
		},
		{
			name:    "weak: toBeVisible with whitespace",
			content: `it("x", () => { expect( el ) . toBeVisible( ); });`,
			want:    true,
		},
		{
			name: "weak toBeVisible + real toBe — NOT a stub",
			content: `it("x", () => {
  expect(getByText('foo')).toBeVisible();
  expect(value).toBe(42);
});`,
			want: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsStub(tc.content); got != tc.want {
				t.Errorf("IsStub() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestIsStubMajority(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    bool
	}{
		{
			name:    "all weak (also a regular stub)",
			content: `it("a", () => { expect(x).toBeDefined(); expect(y).toBeTruthy(); });`,
			want:    true,
		},
		{
			name: "3 weak + 1 real — majority weak",
			content: `it("a", () => {
  expect(x).toBeDefined();
  expect(y).toBeTruthy();
  expect(z).not.toBeNull();
  expect(real).toBe(42);
});`,
			want: true,
		},
		{
			name: "2 weak + 2 real — split, not majority",
			content: `it("a", () => {
  expect(x).toBeDefined();
  expect(y).toBeTruthy();
  expect(real1).toBe(42);
  expect(real2).toEqual({ a: 1 });
});`,
			want: false,
		},
		{
			name: "1 weak + 2 real — minority weak",
			content: `it("a", () => {
  expect(x).toBeDefined();
  expect(real1).toBe(42);
  expect(real2).toEqual({ a: 1 });
});`,
			want: false,
		},
		{
			name:    "no weak matchers",
			content: `it("a", () => { expect(x).toBe(1); expect(y).toEqual(2); });`,
			want:    false,
		},
		{name: "empty file", content: ``, want: false},
		{name: "no expects", content: `it("a", () => {});`, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsStubMajority(tc.content); got != tc.want {
				t.Errorf("IsStubMajority() = %v, want %v", got, tc.want)
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
