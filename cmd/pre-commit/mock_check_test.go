package main

import (
	"testing"
)

func TestMockChecker_isTestFile(t *testing.T) {
	checker := NewMockChecker()

	tests := []struct {
		file     string
		expected bool
	}{
		{"component.test.ts", true},
		{"component.test.tsx", true},
		{"component.spec.ts", true},
		{"component.spec.tsx", true},
		{"component.ts", false},
		{"component.tsx", false},
		{"__mocks__/expo-router.js", false},
	}

	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			if got := checker.isTestFile(tt.file); got != tt.expected {
				t.Errorf("isTestFile(%q) = %v, want %v", tt.file, got, tt.expected)
			}
		})
	}
}

func TestMockChecker_isAllowedFile(t *testing.T) {
	checker := NewMockChecker()
	allowedFiles := []string{"__mocks__/", "test-utils/mocks/"}

	tests := []struct {
		file     string
		expected bool
	}{
		{"__mocks__/expo-router.js", true},
		{"test-utils/mocks/expo-router.ts", true},
		{"components/Button.test.tsx", false},
		{"app/Home.test.tsx", false},
	}

	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			if got := checker.isAllowedFile(tt.file, allowedFiles); got != tt.expected {
				t.Errorf("isAllowedFile(%q) = %v, want %v", tt.file, got, tt.expected)
			}
		})
	}
}

func TestMockChecker_findViolations(t *testing.T) {
	checker := NewMockChecker()
	forbiddenMocks := []string{"expo-router", "@/lib/sentry", "@dashtag/mobile-ui"}

	tests := []struct {
		name           string
		content        string
		expectedCount  int
		expectedModule string
	}{
		{
			name: "no violations",
			content: `import { render } from '@testing-library/react-native';
import { mockRouter } from '@/test-utils/mocks';

describe('Component', () => {
  it('works', () => {});
});`,
			expectedCount: 0,
		},
		{
			name: "single violation - expo-router",
			content: `jest.mock('expo-router', () => ({
  useRouter: () => ({ push: jest.fn() }),
}));`,
			expectedCount:  1,
			expectedModule: "expo-router",
		},
		{
			name: "single violation - sentry",
			content: `jest.mock('@/lib/sentry', () => ({
  logger: { error: jest.fn() },
}));`,
			expectedCount:  1,
			expectedModule: "@/lib/sentry",
		},
		{
			name: "multiple violations",
			content: `jest.mock('expo-router', () => ({}));
jest.mock('@/lib/sentry', () => ({}));
jest.mock('@dashtag/mobile-ui', () => ({}));`,
			expectedCount: 3,
		},
		{
			name: "double quotes",
			content: `jest.mock("expo-router", () => ({}));`,
			expectedCount:  1,
			expectedModule: "expo-router",
		},
		{
			name: "allowed mock - different module",
			content: `jest.mock('@/components/layout', () => ({}));`,
			expectedCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			violations := checker.findViolations("test.test.tsx", []byte(tt.content), forbiddenMocks)
			if len(violations) != tt.expectedCount {
				t.Errorf("findViolations() found %d violations, want %d", len(violations), tt.expectedCount)
			}
			if tt.expectedModule != "" && len(violations) > 0 {
				if violations[0].Module != tt.expectedModule {
					t.Errorf("findViolations() module = %q, want %q", violations[0].Module, tt.expectedModule)
				}
			}
		})
	}
}

func TestMockChecker_Check(t *testing.T) {
	mockContent := map[string][]byte{
		"apps/mobile/component.test.tsx": []byte(`jest.mock('expo-router', () => ({}));`),
		"apps/mobile/clean.test.tsx":     []byte(`import { mockRouter } from '@/test-utils/mocks';`),
		"__mocks__/expo-router.js":       []byte(`module.exports = { useRouter: jest.fn() };`),
	}

	checker := &MockChecker{
		gitShowFunc: func(file string) ([]byte, error) {
			return mockContent[file], nil
		},
	}

	config := MockCheckConfig{
		ForbiddenMocks: []string{"expo-router"},
		AllowedFiles:   []string{"__mocks__/"},
	}

	files := []string{
		"apps/mobile/component.test.tsx",
		"apps/mobile/clean.test.tsx",
		"__mocks__/expo-router.js",
	}

	err := checker.Check(files, config)
	if err == nil {
		t.Error("Check() should return error for violations")
	}
}

func TestMockChecker_Check_NoViolations(t *testing.T) {
	mockContent := map[string][]byte{
		"apps/mobile/clean.test.tsx": []byte(`import { mockRouter } from '@/test-utils/mocks';`),
	}

	checker := &MockChecker{
		gitShowFunc: func(file string) ([]byte, error) {
			return mockContent[file], nil
		},
	}

	config := MockCheckConfig{
		ForbiddenMocks: []string{"expo-router"},
		AllowedFiles:   []string{"__mocks__/"},
	}

	files := []string{"apps/mobile/clean.test.tsx"}

	err := checker.Check(files, config)
	if err != nil {
		t.Errorf("Check() returned error for clean files: %v", err)
	}
}
