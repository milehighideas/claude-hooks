package main

import (
	"testing"
)

func TestResolvedScreenHooks(t *testing.T) {
	tests := []struct {
		name     string
		config   SRPConfig
		expected map[string]bool
	}{
		{
			name:   "empty defaults to useState/useReducer/useContext",
			config: SRPConfig{},
			expected: map[string]bool{
				"useState":   true,
				"useReducer": true,
				"useContext": true,
			},
		},
		{
			name:   "all expands to all 6 hooks",
			config: SRPConfig{ScreenHooks: []string{"all"}},
			expected: map[string]bool{
				"useState":    true,
				"useReducer":  true,
				"useContext":   true,
				"useCallback": true,
				"useEffect":   true,
				"useMemo":     true,
			},
		},
		{
			name:   "individual hooks",
			config: SRPConfig{ScreenHooks: []string{"useState", "useEffect"}},
			expected: map[string]bool{
				"useState":  true,
				"useEffect": true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.config.resolvedScreenHooks()
			if len(result) != len(tt.expected) {
				t.Errorf("got %d hooks, want %d", len(result), len(tt.expected))
			}
			for k := range tt.expected {
				if !result[k] {
					t.Errorf("missing hook %s", k)
				}
			}
		})
	}
}

func TestCheckStateInScreens(t *testing.T) {
	tests := []struct {
		name       string
		config     SRPConfig
		code       string
		filePath   string
		wantErrors int
	}{
		{
			name:       "default config flags useState in screen",
			config:     SRPConfig{},
			code:       `const [x, setX] = useState(false);`,
			filePath:   "apps/rsvp/src/screens/FooScreen.tsx",
			wantErrors: 1,
		},
		{
			name:       "default config ignores useEffect in screen",
			config:     SRPConfig{},
			code:       `useEffect(() => {}, []);`,
			filePath:   "apps/rsvp/src/screens/FooScreen.tsx",
			wantErrors: 0,
		},
		{
			name:       "all config flags useEffect in screen",
			config:     SRPConfig{ScreenHooks: []string{"all"}},
			code:       `useEffect(() => {}, []);`,
			filePath:   "apps/rsvp/src/screens/FooScreen.tsx",
			wantErrors: 1,
		},
		{
			name:       "all config flags useMemo in screen",
			config:     SRPConfig{ScreenHooks: []string{"all"}},
			code:       `const val = useMemo(() => 1, []);`,
			filePath:   "apps/rsvp/src/screens/FooScreen.tsx",
			wantErrors: 1,
		},
		{
			name:       "selective config only flags listed hooks",
			config:     SRPConfig{ScreenHooks: []string{"useEffect"}},
			code:       "const [x, setX] = useState(false);\nuseEffect(() => {}, []);",
			filePath:   "apps/rsvp/src/screens/FooScreen.tsx",
			wantErrors: 1, // only useEffect flagged, not useState
		},
		{
			name:       "non-screen file is never flagged",
			config:     SRPConfig{ScreenHooks: []string{"all"}},
			code:       `const [x, setX] = useState(false);`,
			filePath:   "apps/rsvp/src/components/read/Foo.tsx",
			wantErrors: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checker := &SRPChecker{config: tt.config}
			analysis := checker.analyzeCode(tt.code, tt.filePath)
			violations := checker.checkStateInScreens(analysis, tt.filePath)
			if len(violations) != tt.wantErrors {
				t.Errorf("got %d violations, want %d: %+v", len(violations), tt.wantErrors, violations)
			}
		})
	}
}
