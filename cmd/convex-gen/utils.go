package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

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
		sb.WriteString(fmt.Sprintf("export * from './%s';\n", file))
	}

	return os.WriteFile(filepath.Join(dir, "index.ts"), []byte(sb.String()), 0644)
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
	// Convert "events/voting" to "useEvents_voting"
	parts := strings.Split(namespace, string(filepath.Separator))
	for i, part := range parts {
		parts[i] = capitalize(part)
	}
	return "use" + strings.Join(parts, "_")
}

// toHookName converts namespace + function to hook name
func toHookName(namespace, funcName string) string {
	// Convert namespace "events/voting" and function "getById" to "useEvents_votingGetById"
	parts := strings.Split(namespace, string(filepath.Separator))
	for i, part := range parts {
		parts[i] = capitalize(part)
	}
	return "use" + strings.Join(parts, "_") + capitalize(funcName)
}

// toApiPath converts namespace to api path
func toApiPath(namespace, funcName string) string {
	// Convert "events/voting" to "api.events.voting.funcName"
	parts := strings.Split(namespace, string(filepath.Separator))
	return "api." + strings.Join(parts, ".") + "." + funcName
}

// generateArgsObject creates the args object with skip logic (multiline)
func generateArgsObject(args []ArgInfo) string {
	if len(args) == 0 {
		return "    {},\n"
	}

	// Find skippable args (IDs and required strings)
	var skippableArgs []ArgInfo
	for _, arg := range args {
		if arg.IsID || (arg.Type == "string" && !arg.Optional) {
			skippableArgs = append(skippableArgs, arg)
		}
	}

	if len(skippableArgs) == 0 {
		// No skip logic needed
		argNames := make([]string, len(args))
		for i, arg := range args {
			argNames[i] = arg.Name
		}
		return fmt.Sprintf("    { %s },\n", strings.Join(argNames, ", "))
	}

	// Build skip condition
	var conditions []string
	for _, arg := range skippableArgs {
		if arg.IsArrayID {
			conditions = append(conditions, arg.Name)
		} else {
			conditions = append(conditions, arg.Name+" !== null")
		}
	}

	argNames := make([]string, len(args))
	for i, arg := range args {
		argNames[i] = arg.Name
	}

	condition := strings.Join(conditions, " && ")
	return fmt.Sprintf("    %s ? { %s } : \"skip\",\n", condition, strings.Join(argNames, ", "))
}

// generateArgsObjectInline creates inline args object
func generateArgsObjectInline(args []ArgInfo) string {
	if len(args) == 0 {
		return "{}"
	}

	// Find skippable args
	var skippableArgs []ArgInfo
	for _, arg := range args {
		if arg.IsID || (arg.Type == "string" && !arg.Optional) {
			skippableArgs = append(skippableArgs, arg)
		}
	}

	argNames := make([]string, len(args))
	for i, arg := range args {
		argNames[i] = arg.Name
	}

	if len(skippableArgs) == 0 {
		return "{ " + strings.Join(argNames, ", ") + " }"
	}

	var conditions []string
	for _, arg := range skippableArgs {
		if arg.IsArrayID {
			conditions = append(conditions, arg.Name)
		} else {
			conditions = append(conditions, arg.Name+" !== null")
		}
	}

	condition := strings.Join(conditions, " && ")
	return fmt.Sprintf("%s ? { %s } : \"skip\"", condition, strings.Join(argNames, ", "))
}
