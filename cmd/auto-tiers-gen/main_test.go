package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFindProjectRoot_ReturnsEmptyForMissingConfig(t *testing.T) {
	result := findProjectRoot("/nonexistent/path/to/file.ts")
	if result != "" {
		t.Errorf("expected empty string for non-existent path, got %q", result)
	}
}

func TestFindProjectRoot_WalksUpToConfig(t *testing.T) {
	tmp := t.TempDir()
	nested := filepath.Join(tmp, "packages", "tiers", "src")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(tmp, configFileName)
	if err := os.WriteFile(cfgPath, []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}

	got := findProjectRoot(filepath.Join(nested, "config.ts"))
	if got != tmp {
		t.Errorf("findProjectRoot: expected %q, got %q", tmp, got)
	}
}

func TestGetFilePath(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]interface{}
		expected string
	}{
		{"returns file_path when present", map[string]interface{}{"file_path": "/a/b.ts"}, "/a/b.ts"},
		{"returns empty when missing", map[string]interface{}{}, ""},
		{"returns empty when wrong type", map[string]interface{}{"file_path": 42}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := getFilePath(tt.input); got != tt.expected {
				t.Errorf("getFilePath: got %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestRun_IgnoresNonEditTools(t *testing.T) {
	input := HookInput{
		ToolName:  "Read",
		ToolInput: map[string]interface{}{"file_path": "/tmp/x.ts"},
	}
	b, _ := json.Marshal(input)
	var stderr strings.Builder
	if err := run(strings.NewReader(string(b)), &stderr); err != nil {
		t.Errorf("expected no error for Read tool, got %v", err)
	}
}

func TestRun_IgnoresNonWatchedFile(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(tmp, configFileName),
		[]byte(`{"watchFile":"packages/tiers/src/config.ts","command":["echo","ok"]}`),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	unrelated := filepath.Join(tmp, "apps", "something", "else.ts")
	if err := os.MkdirAll(filepath.Dir(unrelated), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(unrelated, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}

	input := HookInput{
		ToolName:  "Edit",
		ToolInput: map[string]interface{}{"file_path": unrelated},
	}
	b, _ := json.Marshal(input)
	var stderr strings.Builder
	if err := run(strings.NewReader(string(b)), &stderr); err != nil {
		t.Errorf("expected no error for unwatched file, got %v", err)
	}
	if stderr.Len() != 0 {
		t.Errorf("expected no stderr output for unwatched file, got %q", stderr.String())
	}
}
