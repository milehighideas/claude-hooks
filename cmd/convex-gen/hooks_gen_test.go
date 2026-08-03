package main

import "testing"

// A lone spread member makes the wrapping braces redundant, which trips
// unicorn/no-useless-spread in consumer repos. Any other member present means
// the spread is doing real merge work and the braces must stay.
func TestBuildArgsObject(t *testing.T) {
	cases := []struct {
		name  string
		parts []string
		want  string
	}{
		{
			name:  "lone spread drops the redundant braces",
			parts: []string{"...(limit !== null && limit !== undefined ? { limit } : {})"},
			want:  "(limit !== null && limit !== undefined ? { limit } : {})",
		},
		{
			name: "two spreads keep the braces (real merge)",
			parts: []string{
				"...(a !== null && a !== undefined ? { a } : {})",
				"...(b !== null && b !== undefined ? { b } : {})",
			},
			want: "{ ...(a !== null && a !== undefined ? { a } : {}), ...(b !== null && b !== undefined ? { b } : {}) }",
		},
		{
			name:  "required arg alongside a spread keeps the braces",
			parts: []string{"projectId", "...(limit !== null && limit !== undefined ? { limit } : {})"},
			want:  "{ projectId, ...(limit !== null && limit !== undefined ? { limit } : {}) }",
		},
		{
			name:  "single required arg is wrapped normally",
			parts: []string{"projectId"},
			want:  "{ projectId }",
		},
		{
			name:  "no members yields an empty object",
			parts: nil,
			want:  "{  }",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := buildArgsObject(tc.parts); got != tc.want {
				t.Errorf("buildArgsObject(%q)\n got: %s\nwant: %s", tc.parts, got, tc.want)
			}
		})
	}
}
