package main

import "testing"

func TestParseTestFailureCount(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   int
	}{
		{
			name: "vitest output",
			output: ` Test Files  27 failed | 308 passed | 1 skipped (336)
      Tests  111 failed | 3878 passed | 39 skipped | 73 todo (4104)
     Errors  9 errors
   Start at  19:39:17
   Duration  27.44s`,
			want: 111,
		},
		{
			name: "jest output",
			output: `Test Suites: 3 failed, 98 passed, 101 total
Tests:       4 failed, 965 passed, 969 total
Snapshots:   0 total
Time:        2.611 s`,
			want: 4,
		},
		{
			name:   "all tests pass vitest",
			output: ` Test Files  42 passed (42)\n      Tests  350 passed (350)`,
			want:   0,
		},
		{
			name:   "all tests pass jest",
			output: `Test Suites: 5 passed, 5 total\nTests:       20 passed, 20 total`,
			want:   0,
		},
		{
			name:   "no test summary",
			output: `Error: Module not found`,
			want:   0,
		},
		{
			name:   "empty output",
			output: ``,
			want:   0,
		},
		{
			name: "vitest with prefix",
			output: `@dashtag/backend test:  Test Files  5 failed | 100 passed (105)
@dashtag/backend test:       Tests  23 failed | 500 passed (523)`,
			want: 23,
		},
		{
			name: "jest with prefix",
			output: `@dashtag/chat-mobile test: Test Suites: 3 failed, 98 passed, 101 total
@dashtag/chat-mobile test: Tests:       7 failed, 965 passed, 969 total`,
			want: 7,
		},
		{
			name:   "single failure vitest",
			output: `      Tests  1 failed | 99 passed (100)`,
			want:   1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseTestFailureCount(tt.output)
			if got != tt.want {
				t.Errorf("parseTestFailureCount() = %d, want %d", got, tt.want)
			}
		})
	}
}
