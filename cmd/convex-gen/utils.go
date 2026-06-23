package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// jsonMarshalString returns a JSON-encoded (safely-quoted) string literal.
// JSON string encoding is valid TypeScript, so callers can use it directly.
func jsonMarshalString(s string) (string, error) {
	b, err := json.Marshal(s)
	return string(b), err
}

// cleanDirectory removes all .ts files from a directory
func cleanDirectory(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".ts") {
			if err := os.Remove(filepath.Join(dir, entry.Name())); err != nil {
				return err
			}
		}
	}

	return nil
}

// generateIndexFile creates index.ts barrel export
func generateIndexFile(dir string, files []string) error {
	if len(files) == 0 {
		// Create empty index
		content := "// No files generated\nexport {};\n"
		return os.WriteFile(filepath.Join(dir, "index.ts"), []byte(content), 0644)
	}

	var sb strings.Builder
	sb.WriteString("/**\n")
	sb.WriteString(" * AUTO-GENERATED INDEX - DO NOT EDIT\n")
	sb.WriteString(" */\n\n")

	for _, file := range files {
		fmt.Fprintf(&sb, "export * from './%s';\n", file)
	}

	return os.WriteFile(filepath.Join(dir, "index.ts"), []byte(sb.String()), 0644)
}

// toSnakeCase converts camelCase/PascalCase to snake_case (forSale → for_sale).
func toSnakeCase(s string) string {
	var b strings.Builder
	for i, r := range s {
		if r >= 'A' && r <= 'Z' {
			if i > 0 {
				b.WriteByte('_')
			}
			b.WriteRune(r - 'A' + 'a')
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// singularize converts a plural table name to its singular form (vehicles →
// vehicle). Naive: strips a trailing "s", with an "-ies" → "-y" special case
// (companies → company). Returns the input unchanged when it does not end in
// "s".
func singularize(s string) string {
	if strings.HasSuffix(s, "ies") && len(s) > 3 {
		return s[:len(s)-3] + "y"
	}
	if strings.HasSuffix(s, "s") && len(s) > 1 {
		return s[:len(s)-1]
	}
	return s
}

// capitalize capitalizes the first letter
func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// namespaceToFileName converts namespace to hook filename
func namespaceToFileName(namespace string) string {
	// Convert "events/voting" to "useEvents_Voting"
	parts := splitNamespace(namespace)
	for i, part := range parts {
		parts[i] = capitalize(part)
	}
	return "use" + strings.Join(parts, "_")
}

// toApiPath converts namespace to api path
func toApiPath(namespace, funcName string) string {
	// Convert "events/voting" to "api.events.voting.funcName"
	parts := splitNamespace(namespace)
	return "api." + strings.Join(parts, ".") + "." + funcName
}
