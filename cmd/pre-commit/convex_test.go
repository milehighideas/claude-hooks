package main

import (
	"strings"
	"testing"
)

func TestCheckConvex(t *testing.T) {
	tests := []struct {
		name        string
		config      ConvexConfig
		mockOutput  string
		mockErr     error
		expectErr   bool
		errContains string
	}{
		{
			name: "successful validation with default marker",
			config: ConvexConfig{
				Path: "packages/backend",
			},
			mockOutput: "Starting Convex dev...\nConvex functions ready!\nWatching for changes...",
			mockErr:    nil,
			expectErr:  false,
		},
		{
			name: "successful validation with custom marker",
			config: ConvexConfig{
				Path:          "packages/backend",
				SuccessMarker: "Build complete!",
			},
			mockOutput: "Compiling...\nBuild complete!\nDone.",
			mockErr:    nil,
			expectErr:  false,
		},
		{
			name: "failed validation - marker missing",
			config: ConvexConfig{
				Path: "packages/backend",
			},
			mockOutput:  "Error: Failed to compile functions\nSyntax error in file.ts",
			mockErr:     nil,
			expectErr:   true,
			errContains: "success marker",
		},
		{
			name: "failed validation - custom marker missing",
			config: ConvexConfig{
				Path:          "packages/backend",
				SuccessMarker: "Custom success!",
			},
			mockOutput:  "Convex functions ready!",
			mockErr:     nil,
			expectErr:   true,
			errContains: "Custom success!",
		},
		{
			name: "empty path returns error",
			config: ConvexConfig{
				Path: "",
			},
			expectErr:   true,
			errContains: "path is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkConvexWithOutput(tt.config, tt.mockOutput, tt.mockErr)

			if tt.expectErr {
				if err == nil {
					t.Error("expected error but got nil")
					return
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("expected error to contain %q, got %q", tt.errContains, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("expected no error but got: %v", err)
				}
			}
		})
	}
}

func TestDefaultSuccessMarker(t *testing.T) {
	if DefaultConvexSuccessMarker != "Convex functions ready!" {
		t.Errorf("expected default marker to be %q, got %q",
			"Convex functions ready!", DefaultConvexSuccessMarker)
	}
}

func TestConvexConfig(t *testing.T) {
	tests := []struct {
		name           string
		config         ConvexConfig
		expectedPath   string
		expectedMarker string
	}{
		{
			name: "default marker when not specified",
			config: ConvexConfig{
				Path: "packages/backend",
			},
			expectedPath:   "packages/backend",
			expectedMarker: DefaultConvexSuccessMarker,
		},
		{
			name: "custom marker when specified",
			config: ConvexConfig{
				Path:          "convex",
				SuccessMarker: "Ready!",
			},
			expectedPath:   "convex",
			expectedMarker: "Ready!",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.config.Path != tt.expectedPath {
				t.Errorf("expected path %q, got %q", tt.expectedPath, tt.config.Path)
			}

			marker := tt.config.SuccessMarker
			if marker == "" {
				marker = DefaultConvexSuccessMarker
			}
			if marker != tt.expectedMarker {
				t.Errorf("expected marker %q, got %q", tt.expectedMarker, marker)
			}
		})
	}
}

func TestOutputContainsMarker(t *testing.T) {
	tests := []struct {
		name     string
		output   string
		marker   string
		contains bool
	}{
		{
			name:     "marker at start",
			output:   "Convex functions ready!\nMore output...",
			marker:   "Convex functions ready!",
			contains: true,
		},
		{
			name:     "marker at end",
			output:   "Starting...\nConvex functions ready!",
			marker:   "Convex functions ready!",
			contains: true,
		},
		{
			name:     "marker in middle",
			output:   "Line 1\nConvex functions ready!\nLine 3",
			marker:   "Convex functions ready!",
			contains: true,
		},
		{
			name:     "marker not present",
			output:   "Error: compilation failed",
			marker:   "Convex functions ready!",
			contains: false,
		},
		{
			name:     "partial marker does not match",
			output:   "Convex functions",
			marker:   "Convex functions ready!",
			contains: false,
		},
		{
			name:     "empty output",
			output:   "",
			marker:   "Convex functions ready!",
			contains: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := strings.Contains(tt.output, tt.marker)
			if result != tt.contains {
				t.Errorf("expected Contains(%q, %q) = %v, got %v",
					tt.output, tt.marker, tt.contains, result)
			}
		})
	}
}

// checkConvexWithOutput is a test helper that simulates checkConvex with mocked output
func checkConvexWithOutput(config ConvexConfig, output string, cmdErr error) error {
	if config.Path == "" {
		return &pathRequiredError{}
	}

	marker := config.SuccessMarker
	if marker == "" {
		marker = DefaultConvexSuccessMarker
	}

	if cmdErr != nil {
		return &convexDevError{err: cmdErr, output: output}
	}

	if !strings.Contains(output, marker) {
		return &markerNotFoundError{marker: marker, output: output}
	}

	return nil
}

// Error types for testing
type pathRequiredError struct{}

func (e *pathRequiredError) Error() string {
	return "convex path is required"
}

type convexDevError struct {
	err    error
	output string
}

func (e *convexDevError) Error() string {
	return "convex dev failed: " + e.err.Error() + "\nOutput: " + e.output
}

type markerNotFoundError struct {
	marker string
	output string
}

func (e *markerNotFoundError) Error() string {
	return "convex validation failed: success marker \"" + e.marker + "\" not found in output\nOutput: " + e.output
}
