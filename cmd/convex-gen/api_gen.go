package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// APIGenerator generates API wrapper objects for Convex functions
type APIGenerator struct {
	config    *Config
	outputDir string
}

// NewAPIGenerator creates an API generator
func NewAPIGenerator(config *Config) *APIGenerator {
	return &APIGenerator{
		config:    config,
		outputDir: config.GetAPIOutputDir(),
	}
}

// Generate creates all API wrapper files
func (g *APIGenerator) Generate(functions []ConvexFunction) error {
	// Create output directory
	if err := os.MkdirAll(g.outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", g.outputDir, err)
	}

	// Clean existing files
	if err := cleanDirectory(g.outputDir); err != nil {
		return err
	}

	fileStructure := g.config.DataLayer.FileStructure
	var files []string

	if fileStructure == "grouped" {
		// Group by top-level namespace
		byTopLevel := make(map[string][]ConvexFunction)
		for _, fn := range functions {
			topLevel := getTopLevelNamespace(fn.Namespace)
			byTopLevel[topLevel] = append(byTopLevel[topLevel], fn)
		}

		for topNamespace, funcs := range byTopLevel {
			fileName := topNamespace
			filePath := filepath.Join(g.outputDir, fileName+".ts")

			content := g.generateGroupedAPIFileContent(topNamespace, funcs)

			if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
				return fmt.Errorf("failed to write %s: %w", filePath, err)
			}

			files = append(files, fileName)
		}
	} else {
		// Split or both - group by full namespace
		byNamespace := make(map[string][]ConvexFunction)
		for _, fn := range functions {
			byNamespace[fn.Namespace] = append(byNamespace[fn.Namespace], fn)
		}

		for namespace, funcs := range byNamespace {
			fileName := apiNamespaceToFileName(namespace)
			filePath := filepath.Join(g.outputDir, fileName+".ts")

			content := g.generateAPIFileContent(namespace, funcs)

			if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
				return fmt.Errorf("failed to write %s: %w", filePath, err)
			}

			files = append(files, fileName)
		}

		// For "both", also generate grouped files
		if fileStructure == "both" {
			byTopLevel := make(map[string][]ConvexFunction)
			for _, fn := range functions {
				topLevel := getTopLevelNamespace(fn.Namespace)
				byTopLevel[topLevel] = append(byTopLevel[topLevel], fn)
			}

			for topNamespace, funcs := range byTopLevel {
				fileName := topNamespace
				filePath := filepath.Join(g.outputDir, fileName+".ts")

				content := g.generateGroupedAPIFileContent(topNamespace, funcs)

				if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
					return fmt.Errorf("failed to write %s: %w", filePath, err)
				}

				files = append(files, fileName)
			}
		}
	}

	// Generate index file
	sort.Strings(files)
	files = uniqueStrings(files)

	if g.config.DataLayer.ExportAPI {
		return g.generateAPIIndexFile(files)
	}
	return generateIndexFile(g.outputDir, files)
}

// generateAPIIndexFile creates index.ts with an api re-export at the top
func (g *APIGenerator) generateAPIIndexFile(files []string) error {
	if len(files) == 0 {
		content := "// No files generated\nexport {};\n"
		return os.WriteFile(filepath.Join(g.outputDir, "index.ts"), []byte(content), 0644)
	}

	var sb strings.Builder
	sb.WriteString("/**\n")
	sb.WriteString(" * AUTO-GENERATED INDEX - DO NOT EDIT\n")
	sb.WriteString(" */\n\n")
	sb.WriteString(fmt.Sprintf("export { api } from '%s';\n", g.config.Imports.API))

	for _, file := range files {
		sb.WriteString(fmt.Sprintf("export * from './%s';\n", file))
	}

	return os.WriteFile(filepath.Join(g.outputDir, "index.ts"), []byte(sb.String()), 0644)
}

// getUniqueExportName returns a unique export name for a function, prefixing with sub-namespace if needed
func getUniqueExportName(fn ConvexFunction, topNamespace string, seenNames map[string]bool) string {
	name := fn.Name
	if !seenNames[name] {
		seenNames[name] = true
		return name
	}
	// Name collision - prefix with sub-namespace
	// e.g., "shopAppointments/appointmentSettingsQueries" -> "settings" prefix
	subNs := strings.TrimPrefix(fn.Namespace, topNamespace+"/")
	if subNs != fn.Namespace && subNs != "" {
		// Extract meaningful prefix from sub-namespace
		parts := strings.Split(subNs, "/")
		prefix := parts[0]
		// Clean up common suffixes like "Queries", "Mutations"
		prefix = strings.TrimSuffix(prefix, "Queries")
		prefix = strings.TrimSuffix(prefix, "Mutations")
		prefix = strings.TrimSuffix(prefix, "Actions")
		if prefix != "" {
			// Convert to camelCase prefix
			uniqueName := prefix + capitalize(name)
			seenNames[uniqueName] = true
			return uniqueName
		}
	}
	// Fallback: append index
	i := 2
	for {
		uniqueName := fmt.Sprintf("%s%d", name, i)
		if !seenNames[uniqueName] {
			seenNames[uniqueName] = true
			return uniqueName
		}
		i++
	}
}

// generateGroupedAPIFileContent creates content for a grouped API file (all sub-namespaces combined)
func (g *APIGenerator) generateGroupedAPIFileContent(topNamespace string, funcs []ConvexFunction) string {
	var sb strings.Builder

	// Header
	sb.WriteString("/**\n")
	sb.WriteString(fmt.Sprintf(" * %s API References\n", capitalize(topNamespace)))
	sb.WriteString(" * Auto-generated from Convex backend functions\n")
	sb.WriteString(" *\n")
	sb.WriteString(" * DO NOT EDIT MANUALLY\n")
	sb.WriteString(" * Run 'convex-gen' to regenerate this file.\n")
	sb.WriteString(" */\n\n")

	// Imports
	sb.WriteString("import type { FunctionReference } from 'convex/server';\n")
	sb.WriteString(fmt.Sprintf("import { api } from '%s';\n\n", g.config.Imports.API))

	// Group by type
	var queries, mutations, actions []ConvexFunction

	for _, fn := range funcs {
		switch fn.Type {
		case FunctionTypeQuery:
			queries = append(queries, fn)
		case FunctionTypeMutation:
			mutations = append(mutations, fn)
		case FunctionTypeAction:
			actions = append(actions, fn)
		}
	}

	baseName := capitalize(topNamespace)

	// Generate queries export
	if len(queries) > 0 {
		seenNames := make(map[string]bool)
		sb.WriteString(fmt.Sprintf("export const %sQueries: Record<string, FunctionReference<\"query\">> = {\n", baseName))
		for _, fn := range queries {
			apiPath := toApiPath(fn.Namespace, fn.Name)
			exportName := getUniqueExportName(fn, topNamespace, seenNames)
			sb.WriteString(fmt.Sprintf("  %s: %s as unknown as FunctionReference<\"query\">,\n", exportName, apiPath))
		}
		sb.WriteString("};\n\n")
	}

	// Generate mutations export
	if len(mutations) > 0 {
		seenNames := make(map[string]bool)
		sb.WriteString(fmt.Sprintf("export const %sMutations: Record<string, FunctionReference<\"mutation\">> = {\n", baseName))
		for _, fn := range mutations {
			apiPath := toApiPath(fn.Namespace, fn.Name)
			exportName := getUniqueExportName(fn, topNamespace, seenNames)
			sb.WriteString(fmt.Sprintf("  %s: %s as unknown as FunctionReference<\"mutation\">,\n", exportName, apiPath))
		}
		sb.WriteString("};\n\n")
	}

	// Generate actions export
	if len(actions) > 0 {
		seenNames := make(map[string]bool)
		sb.WriteString(fmt.Sprintf("export const %sActions: Record<string, FunctionReference<\"action\">> = {\n", baseName))
		for _, fn := range actions {
			apiPath := toApiPath(fn.Namespace, fn.Name)
			exportName := getUniqueExportName(fn, topNamespace, seenNames)
			sb.WriteString(fmt.Sprintf("  %s: %s as unknown as FunctionReference<\"action\">,\n", exportName, apiPath))
		}
		sb.WriteString("};\n\n")
	}

	return sb.String()
}

// generateAPIFileContent creates the content for an API file
func (g *APIGenerator) generateAPIFileContent(namespace string, funcs []ConvexFunction) string {
	var sb strings.Builder

	// Header
	sb.WriteString("/**\n")
	sb.WriteString(fmt.Sprintf(" * %s API References\n", capitalize(namespace)))
	sb.WriteString(" * Auto-generated from Convex backend functions\n")
	sb.WriteString(" *\n")
	sb.WriteString(" * DO NOT EDIT MANUALLY\n")
	sb.WriteString(" * Run 'convex-gen' to regenerate this file.\n")
	sb.WriteString(" */\n\n")

	// Imports
	sb.WriteString("import type { FunctionReference } from 'convex/server';\n")
	sb.WriteString(fmt.Sprintf("import { api } from '%s';\n\n", g.config.Imports.API))

	// Group by type
	var queries, mutations, actions []ConvexFunction

	for _, fn := range funcs {
		switch fn.Type {
		case FunctionTypeQuery:
			queries = append(queries, fn)
		case FunctionTypeMutation:
			mutations = append(mutations, fn)
		case FunctionTypeAction:
			actions = append(actions, fn)
		}
	}

	baseName := apiNamespaceToExportName(namespace)

	// Generate queries export
	if len(queries) > 0 {
		sb.WriteString(fmt.Sprintf("export const %sQueries: Record<string, FunctionReference<\"query\">> = {\n", baseName))
		for _, fn := range queries {
			apiPath := toApiPath(namespace, fn.Name)
			sb.WriteString(fmt.Sprintf("  %s: %s as unknown as FunctionReference<\"query\">,\n", fn.Name, apiPath))
		}
		sb.WriteString("};\n\n")
	}

	// Generate mutations export
	if len(mutations) > 0 {
		sb.WriteString(fmt.Sprintf("export const %sMutations: Record<string, FunctionReference<\"mutation\">> = {\n", baseName))
		for _, fn := range mutations {
			apiPath := toApiPath(namespace, fn.Name)
			sb.WriteString(fmt.Sprintf("  %s: %s as unknown as FunctionReference<\"mutation\">,\n", fn.Name, apiPath))
		}
		sb.WriteString("};\n\n")
	}

	// Generate actions export
	if len(actions) > 0 {
		sb.WriteString(fmt.Sprintf("export const %sActions: Record<string, FunctionReference<\"action\">> = {\n", baseName))
		for _, fn := range actions {
			apiPath := toApiPath(namespace, fn.Name)
			sb.WriteString(fmt.Sprintf("  %s: %s as unknown as FunctionReference<\"action\">,\n", fn.Name, apiPath))
		}
		sb.WriteString("};\n\n")
	}

	return sb.String()
}

// apiNamespaceToFileName converts namespace to filename
func apiNamespaceToFileName(namespace string) string {
	// Convert "events/voting" to "events-voting"
	return strings.ReplaceAll(namespace, string(filepath.Separator), "-")
}

// apiNamespaceToExportName converts namespace to export name
func apiNamespaceToExportName(namespace string) string {
	// Convert "events/voting" to "EventsVoting"
	parts := strings.Split(namespace, string(filepath.Separator))
	for i, part := range parts {
		parts[i] = capitalize(part)
	}
	return strings.Join(parts, "")
}
