// Package jsonc provides utilities for reading JSONC (JSON with comments) files.
package jsonc

import (
	"encoding/json"
	"os"
	"strings"
)

// StripComments removes single-line // comments from JSONC content.
// It handles comments on their own line and inline after values.
// It does not strip comments inside string literals.
func StripComments(data []byte) []byte {
	lines := strings.Split(string(data), "\n")
	var out []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		// Skip full-line comments
		if strings.HasPrefix(trimmed, "//") {
			continue
		}
		// Strip inline comments (after a value, outside strings)
		// Walk character by character to respect string boundaries
		inString := false
		escaped := false
		for i, ch := range line {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' && inString {
				escaped = true
				continue
			}
			if ch == '"' {
				inString = !inString
				continue
			}
			if !inString && ch == '/' && i+1 < len(line) && line[i+1] == '/' {
				line = strings.TrimRight(line[:i], " \t")
				break
			}
		}
		out = append(out, line)
	}
	return []byte(strings.Join(out, "\n"))
}

// ReadFile reads a JSONC file, strips comments, and returns clean JSON bytes.
func ReadFile(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return StripComments(data), nil
}

// Unmarshal reads a JSONC file, strips comments, and unmarshals into v.
func Unmarshal(path string, v any) error {
	data, err := ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}
