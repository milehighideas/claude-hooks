package jsonc

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestStripComments(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"full-line comment is dropped", "// header\n{\"a\":1}", "{\"a\":1}"},
		{"indented full-line comment is dropped", "  // spaced\nx", "x"},
		{"inline comment after value is stripped", "{\"a\":1} // trailing", "{\"a\":1}"},
		{"tab before inline comment is trimmed", "x\t// c", "x"},
		{"slashes inside a string are preserved", "{\"url\":\"https://example.com\"}", "{\"url\":\"https://example.com\"}"},
		{"inline comment after a URL value is stripped but URL kept", "{\"url\":\"https://x.com\"} // note", "{\"url\":\"https://x.com\"}"},
		{"escaped quote inside string does not end the string", "{\"q\":\"a\\\"b\"} // c", "{\"q\":\"a\\\"b\"}"},
		{"no comment passes through unchanged", "{\"a\":1,\"b\":2}", "{\"a\":1,\"b\":2}"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := string(StripComments([]byte(tt.in)))
			if got != tt.want {
				t.Errorf("StripComments(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestReadFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.jsonc")
	content := "// config\n{\n  \"name\": \"x\", // inline\n  \"n\": 2\n}\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	got, err := ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(got, &m); err != nil {
		t.Fatalf("stripped output is not valid JSON: %v\n%s", err, got)
	}
	if m["name"] != "x" {
		t.Errorf("name = %v, want x", m["name"])
	}

	if _, err := ReadFile(filepath.Join(dir, "missing.jsonc")); err == nil {
		t.Error("ReadFile(missing) expected an error, got nil")
	}
}

func TestUnmarshal(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.jsonc")
	content := "{\n  // a comment\n  \"name\": \"acme\",\n  \"count\": 3\n}\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	var v struct {
		Name  string `json:"name"`
		Count int    `json:"count"`
	}
	if err := Unmarshal(path, &v); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if v.Name != "acme" || v.Count != 3 {
		t.Errorf("got %+v, want {Name:acme Count:3}", v)
	}

	if err := Unmarshal(filepath.Join(dir, "nope.jsonc"), &v); err == nil {
		t.Error("Unmarshal(missing) expected an error, got nil")
	}
}
