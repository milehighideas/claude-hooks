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
