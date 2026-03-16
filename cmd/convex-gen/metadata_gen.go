package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// MetadataGenerator generates TypeScript schema metadata from Convex schema
type MetadataGenerator struct {
	config    *Config
	outputDir string
}

// NewMetadataGenerator creates a metadata generator
func NewMetadataGenerator(config *Config) *MetadataGenerator {
	return &MetadataGenerator{
		config:    config,
		outputDir: config.GetMetadataOutputDir(),
	}
}

// Generate creates the schema metadata file
func (g *MetadataGenerator) Generate(tables []TableInfo) error {
	// Create output directory
	if err := os.MkdirAll(g.outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", g.outputDir, err)
	}

	content := g.generateMetadataContent(tables)

	filePath := filepath.Join(g.outputDir, "schemaMetadata.ts")
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write %s: %w", filePath, err)
	}

	// Generate index file
	return g.generateMetadataIndexFile()
}

// generateMetadataContent creates the schema metadata TypeScript file
func (g *MetadataGenerator) generateMetadataContent(tables []TableInfo) string {
	var sb strings.Builder

	// Sort tables by name
	sort.Slice(tables, func(i, j int) bool {
		return tables[i].Name < tables[j].Name
	})

	// Header
	sb.WriteString("/**\n")
	sb.WriteString(" * Auto-generated Convex Schema Metadata\n")
	sb.WriteString(" *\n")
	sb.WriteString(" * DO NOT EDIT MANUALLY - Run 'convex-gen' to regenerate\n")
	sb.WriteString(" *\n")
	sb.WriteString(" * This file exports structured metadata about all Convex schema tables\n")
	sb.WriteString(" * and their fields. Use this for building dynamic, schema-aware UIs.\n")
	sb.WriteString(" */\n\n")

	// SchemaFieldInfo interface
	sb.WriteString("export interface SchemaFieldInfo {\n")
	sb.WriteString("  name: string;\n")
	sb.WriteString("  type: \"string\" | \"number\" | \"boolean\" | \"id\" | \"object\" | \"array\" | \"union\" | \"any\";\n")
	sb.WriteString("  optional: boolean;\n")
	sb.WriteString("  isId: boolean;\n")
	sb.WriteString("  tableRef?: string;\n")
	sb.WriteString("  isArray: boolean;\n")
	sb.WriteString("  arrayType?: string;\n")
	sb.WriteString("  literals?: string[];\n")
	sb.WriteString("}\n\n")

	// TableMetadata interface
	sb.WriteString("export interface TableMetadata {\n")
	sb.WriteString("  name: string;\n")
	sb.WriteString("  fields: SchemaFieldInfo[];\n")
	sb.WriteString("}\n\n")

	// SCHEMA_METADATA record
	sb.WriteString("export const SCHEMA_METADATA: Record<string, TableMetadata> = {\n")
	for _, table := range tables {
		sb.WriteString(fmt.Sprintf("  %s: {\n", table.Name))
		sb.WriteString(fmt.Sprintf("    name: \"%s\",\n", table.Name))
		sb.WriteString("    fields: [\n")
		for _, field := range table.Fields {
			sb.WriteString("      ")
			sb.WriteString(g.fieldToTS(field))
			sb.WriteString(",\n")
		}
		sb.WriteString("    ],\n")
		sb.WriteString("  },\n")
	}
	sb.WriteString("};\n\n")

	// TABLE_NAMES const array
	sb.WriteString("export const TABLE_NAMES = [\n")
	for _, table := range tables {
		sb.WriteString(fmt.Sprintf("  \"%s\",\n", table.Name))
	}
	sb.WriteString("] as const;\n\n")

	// TableName type
	sb.WriteString("export type TableName = (typeof TABLE_NAMES)[number];\n")

	return sb.String()
}

// fieldToTS converts a FieldInfo to a TypeScript object literal string
func (g *MetadataGenerator) fieldToTS(f FieldInfo) string {
	var parts []string

	parts = append(parts, fmt.Sprintf("name: \"%s\"", f.Name))
	parts = append(parts, fmt.Sprintf("type: \"%s\"", f.Type))
	parts = append(parts, fmt.Sprintf("optional: %t", f.Optional))
	parts = append(parts, fmt.Sprintf("isId: %t", f.IsID))

	if f.TableRef != "" {
		parts = append(parts, fmt.Sprintf("tableRef: \"%s\"", f.TableRef))
	}

	parts = append(parts, fmt.Sprintf("isArray: %t", f.IsArray))

	if f.ArrayType != "" {
		parts = append(parts, fmt.Sprintf("arrayType: \"%s\"", f.ArrayType))
	}

	if len(f.Literals) > 0 {
		quoted := make([]string, len(f.Literals))
		for i, lit := range f.Literals {
			quoted[i] = fmt.Sprintf("\"%s\"", lit)
		}
		parts = append(parts, fmt.Sprintf("literals: [%s]", strings.Join(quoted, ", ")))
	}

	return "{ " + strings.Join(parts, ", ") + " }"
}

// generateMetadataIndexFile creates index.ts barrel export for metadata
func (g *MetadataGenerator) generateMetadataIndexFile() error {
	content := `/**
 * Generated Schema Metadata Index
 * Auto-generated barrel export file
 *
 * DO NOT EDIT MANUALLY
 * Run 'convex-gen' to regenerate this file.
 */

export * from './schemaMetadata';
`
	return os.WriteFile(filepath.Join(g.outputDir, "index.ts"), []byte(content), 0644)
}
