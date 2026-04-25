package stubs

import "testing"

func TestIsSelfMockStub(t *testing.T) {
	cases := []struct {
		name    string
		path    string
		content string
		want    bool
	}{
		{
			name: "self-mock with null return — flagged",
			path: "apps/events-mobile/components/events/check-ins/screens/EventCheckInsScreen.test.tsx",
			content: `
jest.mock('../read/CheckInContent', () => ({
  CheckInContent: () => null,
}));

jest.mock('./EventCheckInsScreen', () => ({
  EventCheckInsScreen: () => null,
}));

it('x', () => {
  const result = computeSomething();
  expect(result).toBe(42);
});
`,
			want: true,
		},
		{
			name: "self-mock with passthrough fragment — flagged",
			path: "apps/foo/ScannerView.test.tsx",
			content: `
jest.mock('./ScannerView', () => ({
  ScannerView: ({ children }) => <>{children}</>,
}));

it('x', () => { expect(something).toBe(true); });
`,
			want: true,
		},
		{
			name: "self-mock via relative path with extension",
			path: "apps/foo/MyComp.test.tsx",
			content: `
jest.mock('./MyComp.tsx', () => ({ MyComp: () => null }));
it('x', () => { expect(x).toBe(1); });
`,
			want: true,
		},
		{
			name: "self-mock via parent path",
			path: "apps/foo/hooks/useThing.test.ts",
			content: `
jest.mock('../useThing', () => ({ useThing: () => null }));
it('x', () => { expect(x).toBe(1); });
`,
			want: false, // '../useThing' doesn't match 'useThing' basename when parent is 'foo/hooks'; it references a sibling in the parent dir
		},
		{
			name: "partial mock via spread of requireActual — NOT flagged",
			path: "apps/foo/utils.test.ts",
			content: `
jest.mock('./utils', () => ({
  ...jest.requireActual('./utils'),
  foo: jest.fn(),
}));
it('x', () => { expect(x).toBe(1); });
`,
			want: false,
		},
		{
			name: "mock of OTHER module — NOT flagged (the mock is not the subject)",
			path: "apps/foo/Button.test.tsx",
			content: `
jest.mock('../lib/api', () => ({ fetchData: () => null }));
it('x', () => { expect(x).toBe(1); });
`,
			want: false,
		},
		{
			name: "no jest.mock calls — NOT flagged",
			path: "apps/foo/Button.test.tsx",
			content: `
it('x', () => { expect(x).toBe(1); });
`,
			want: false,
		},
		{
			name: "not a test file — NOT flagged",
			path: "apps/foo/Button.tsx",
			content: `
jest.mock('./Button', () => ({ Button: () => null }));
`,
			want: false,
		},
		{
			name: "self-mock where factory returns jest.fn() (real spy) — NOT flagged",
			path: "apps/foo/api.test.ts",
			content: `
jest.mock('./api', () => ({
  fetchData: jest.fn().mockResolvedValue({ id: 1 }),
}));
it('x', () => { expect(x).toBe(1); });
`,
			want: false,
		},
		{
			name: "malformed source — NOT flagged (fail open)",
			path: "apps/foo/Button.test.tsx",
			content: `
this is not valid typescript at all (((
`,
			want: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := IsSelfMockStub(tc.path, tc.content)
			if got != tc.want {
				t.Errorf("IsSelfMockStub(%q) = %v, want %v", tc.path, got, tc.want)
			}
		})
	}
}

func TestIsSelfMockStub_MultipleMocks(t *testing.T) {
	// File mocks several things including itself. Self-mock signal still fires.
	content := `
jest.mock('../lib/api', () => ({ fetchData: jest.fn() }));
jest.mock('./EventGiveawaysScreen', () => ({ EventGiveawaysScreen: () => null }));
jest.mock('expo-camera', () => ({ CameraView: ({ children }) => <>{children}</> }));

it('x', () => { expect(result).toEqual({ a: 1 }); });
`
	path := "apps/foo/EventGiveawaysScreen.test.tsx"
	if !IsSelfMockStub(path, content) {
		t.Errorf("expected self-mock flag for %q", path)
	}
}

func TestCountTautological(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    int
	}{
		{
			name:    "string literal equals itself",
			content: `it("x", () => { expect("hello").toBe("hello"); });`,
			want:    1,
		},
		{
			name:    "identifier equals itself",
			content: `it("x", () => { expect(planName).toBe(planName); });`,
			want:    1,
		},
		{
			name:    "member access equals itself",
			content: `it("x", () => { expect(obj.foo).toEqual(obj.foo); });`,
			want:    1,
		},
		{
			name:    "toStrictEqual same identifier",
			content: `it("x", () => { expect(arr).toStrictEqual(arr); });`,
			want:    1,
		},
		{
			name:    "toMatchObject same identifier",
			content: `it("x", () => { expect(user).toMatchObject(user); });`,
			want:    1,
		},
		{
			name:    "different identifiers — not tautological",
			content: `it("x", () => { expect(actual).toBe(expected); });`,
			want:    0,
		},
		{
			name:    "literal equals different literal — not tautological",
			content: `it("x", () => { expect("a").toBe("b"); });`,
			want:    0,
		},
		{
			name: "multiple tautologies in one file",
			content: `it("a", () => { expect("x").toBe("x"); });
it("b", () => { expect(y).toEqual(y); });`,
			want: 2,
		},
		{
			name:    "non-equality matcher with same args — not flagged",
			content: `it("x", () => { expect(x).toHaveBeenCalledWith(x); });`,
			want:    0,
		},
		{
			name:    "negated equality (.not.toBe(x)) — not flagged",
			content: `it("x", () => { expect(y).not.toBe(y); });`,
			want:    0,
		},
		{
			name:    "tautology mixed with real assertions",
			content: `it("x", () => { expect("k").toBe("k"); expect(real).toBe(42); });`,
			want:    1,
		},
		{name: "empty file", content: ``, want: 0},
		{
			name:    "syntactically malformed — fails open",
			content: `it("x", () => { expect("a").toBe(`, // unterminated
			want:    0,
		},
		{
			name: "real RTL pattern — not tautological",
			content: `it("renders", () => {
  render(<X />);
  expect(screen.getByText("Save").textContent).toBe("Save");
});`,
			// limitation: textContent === "Save" is a runtime tautology our
			// syntactic check can't see (different node shapes).
			want: 0,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := CountTautological(tc.content); got != tc.want {
				t.Errorf("CountTautological() = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestIsTautological(t *testing.T) {
	if !IsTautological(`expect(x).toBe(x);`) {
		t.Error("expected IsTautological(true) for identifier equals itself")
	}
	if IsTautological(`expect(x).toBe(y);`) {
		t.Error("expected IsTautological(false) for differing identifiers")
	}
	if IsTautological(``) {
		t.Error("expected IsTautological(false) for empty content")
	}
}
