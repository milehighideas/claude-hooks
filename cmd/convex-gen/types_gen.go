package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// TypesGenerator generates TypeScript types from Convex schema
type TypesGenerator struct {
	config    *Config
	outputDir string
}

// NewTypesGenerator creates a types generator
func NewTypesGenerator(config *Config) *TypesGenerator {
	return &TypesGenerator{
		config:    config,
		outputDir: config.GetTypesOutputDir(),
	}
}

// Generate creates the types file
func (g *TypesGenerator) Generate(tables []TableInfo) error {
	// Create output directory
	if err := os.MkdirAll(g.outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", g.outputDir, err)
	}

	content := g.generateTypesContent(tables)

	filePath := filepath.Join(g.outputDir, "convex.ts")
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write %s: %w", filePath, err)
	}

	// Generate index file
	return g.generateTypesIndexFile()
}

// generateTypesContent creates the types file content
func (g *TypesGenerator) generateTypesContent(tables []TableInfo) string {
	var sb strings.Builder

	// Sort tables by name
	sort.Slice(tables, func(i, j int) bool {
		return tables[i].Name < tables[j].Name
	})

	// Header
	sb.WriteString("/**\n")
	sb.WriteString(" * Auto-generated Convex Types\n")
	sb.WriteString(" *\n")
	sb.WriteString(" * DO NOT EDIT MANUALLY - Run 'convex-gen' to regenerate\n")
	sb.WriteString(" *\n")
	sb.WriteString(" * This file exports TypeScript types derived from your Convex schema.\n")
	sb.WriteString(" * The Convex schema is the source of truth - these types are auto-generated\n")
	sb.WriteString(" * to prevent manual duplication and drift.\n")
	sb.WriteString(" *\n")
	sb.WriteString(" * Usage:\n")
	sb.WriteString(" * - Use Doc<\"tableName\"> for document types\n")
	sb.WriteString(" * - Use Id<\"tableName\"> for ID types\n")
	sb.WriteString(" * - Use derived types for specific fields\n")
	sb.WriteString(" */\n\n")

	// Imports
	sb.WriteString(fmt.Sprintf("import type { Doc, Id } from '%s';\n\n", g.config.Imports.DataModel))
	sb.WriteString("// Re-export Doc and Id types so they can be imported from this file\n")
	sb.WriteString("export type { Doc, Id };\n\n")

	// Table document types section
	sb.WriteString("// ============================================================================\n")
	sb.WriteString("// TABLE DOCUMENT TYPES\n")
	sb.WriteString("// ============================================================================\n\n")

	for _, table := range tables {
		sb.WriteString(fmt.Sprintf("/** %s table */\n", table.Name))
		sb.WriteString(fmt.Sprintf("export type %s = Doc<\"%s\">;\n\n", table.TypeName, table.Name))
	}

	// Table ID types section
	sb.WriteString("// ============================================================================\n")
	sb.WriteString("// TABLE ID TYPES\n")
	sb.WriteString("// ============================================================================\n\n")

	for _, table := range tables {
		sb.WriteString(fmt.Sprintf("export type %sId = Id<\"%s\">;\n", table.TypeName, table.Name))
	}
	sb.WriteString("\n")

	// Utility types section
	sb.WriteString("// ============================================================================\n")
	sb.WriteString("// UTILITY TYPES\n")
	sb.WriteString("// ============================================================================\n\n")

	// Table name union
	sb.WriteString("/** Union of all table names */\n")
	if len(tables) == 0 {
		sb.WriteString("export type TableName = never;\n\n")
	} else {
		tableNames := make([]string, len(tables))
		for i, table := range tables {
			tableNames[i] = fmt.Sprintf("\"%s\"", table.Name)
		}
		sb.WriteString(fmt.Sprintf("export type TableName = %s;\n\n", strings.Join(tableNames, " | ")))
	}

	// Entity type union (singular form)
	sb.WriteString("/** Union of all entity types (singular form) */\n")
	if len(tables) == 0 {
		sb.WriteString("export type EntityType = never;\n\n")
	} else {
		entityTypes := make([]string, len(tables))
		for i, table := range tables {
			entityTypes[i] = fmt.Sprintf("\"%s\"", toSingular(table.Name))
		}
		sb.WriteString(fmt.Sprintf("export type EntityType = %s;\n\n", strings.Join(entityTypes, " | ")))
	}

	// Summary comment
	sb.WriteString("/**\n")
	sb.WriteString(fmt.Sprintf(" * Generated %d table types from Convex schema\n", len(tables)))
	sb.WriteString(" *\n")
	sb.WriteString(" * Tables:\n")
	for _, table := range tables {
		sb.WriteString(fmt.Sprintf(" * - %s\n", table.Name))
	}
	sb.WriteString(" */\n")

	return sb.String()
}

// generateTypesIndexFile creates index.ts barrel export for types
func (g *TypesGenerator) generateTypesIndexFile() error {
	content := `/**
 * Generated Types Index
 * Auto-generated barrel export file
 *
 * DO NOT EDIT MANUALLY
 * Run 'convex-gen' to regenerate this file.
 */

export * from './convex';
`
	return os.WriteFile(filepath.Join(g.outputDir, "index.ts"), []byte(content), 0644)
}

// toSingular converts a plural table name to singular form
func toSingular(name string) string {
	if strings.HasSuffix(name, "ies") {
		return name[:len(name)-3] + "y" // categories -> category
	}
	if strings.HasSuffix(name, "ses") {
		return name[:len(name)-2] // addresses -> address
	}
	if strings.HasSuffix(name, "s") && !strings.HasSuffix(name, "ss") {
		return name[:len(name)-1] // projects -> project
	}
	return name
}
