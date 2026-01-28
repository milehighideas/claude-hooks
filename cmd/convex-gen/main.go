package main

import (
	"fmt"
	"os"
	"strings"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	fmt.Println("convex-gen - Convex Data Layer Generator")
	fmt.Println()

	// Load configuration
	config, err := LoadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	fmt.Printf("Organization: %s\n", config.Org)
	fmt.Printf("Convex path: %s\n", config.Convex.Path)
	fmt.Printf("Data layer path: %s\n", config.DataLayer.Path)
	fmt.Println()

	// Create scanner
	scanner, err := NewScanner(config)
	if err != nil {
		return fmt.Errorf("failed to create scanner: %w", err)
	}

	// Create parser
	parser := NewParser(config)

	// Build validator cache for resolving referenced validators
	fmt.Println("Building validator cache...")
	if err := parser.BuildValidatorCache(config.Convex.Path); err != nil {
		fmt.Printf("Warning: failed to build validator cache: %v\n", err)
	}
	fmt.Printf("Cached %d validators\n", len(parser.validatorCache))
	fmt.Println()

	// Scan and parse Convex functions
	var allFunctions []ConvexFunction
	if config.Generators.Hooks || config.Generators.API {
		fmt.Println("Scanning Convex functions...")

		files, err := scanner.ScanConvexDirectory()
		if err != nil {
			return fmt.Errorf("failed to scan convex directory: %w", err)
		}

		fmt.Printf("Found %d Convex files\n", len(files))

		// DEBUG: Check if projects.ts is in the list
		for _, file := range files {
			if strings.Contains(file.Path, "projects.ts") && !strings.Contains(file.Path, "Internal") && !strings.Contains(file.Path, "test") {
			}
		}

		for _, file := range files {
			functions, err := parser.ParseConvexFile(file)
			if err != nil {
				fmt.Printf("Warning: failed to parse %s: %v\n", file.Path, err)
				continue
			}
			allFunctions = append(allFunctions, functions...)
		}

		fmt.Printf("Parsed %d functions\n", len(allFunctions))
		fmt.Println()
	}

	// Scan and parse schema
	var allTables []TableInfo
	if config.Generators.Types {
		fmt.Println("Scanning schema files...")

		schemaFiles, err := scanner.ScanSchemaFiles()
		if err != nil {
			return fmt.Errorf("failed to scan schema files: %w", err)
		}

		fmt.Printf("Found %d schema files\n", len(schemaFiles))

		// Check if we have a main schema file (with defineSchema)
		// If so, only use tables from that file
		mainSchemaFound := false
		for _, file := range schemaFiles {
			if file.Domain == "main" {
				tables, err := parser.ParseSchemaFile(file)
				if err != nil {
					fmt.Printf("Warning: failed to parse main schema %s: %v\n", file.Path, err)
					continue
				}
				if len(tables) > 0 {
					mainSchemaFound = true
					allTables = tables
					break
				}
			}
		}

		// If no main schema, fall back to individual schema files
		if !mainSchemaFound {
			for _, file := range schemaFiles {
				tables, err := parser.ParseSchemaFile(file)
				if err != nil {
					fmt.Printf("Warning: failed to parse schema %s: %v\n", file.Path, err)
					continue
				}
				allTables = append(allTables, tables...)
			}
		}

		// Deduplicate tables by name (keep first occurrence)
		seen := make(map[string]bool)
		uniqueTables := make([]TableInfo, 0, len(allTables))
		for _, table := range allTables {
			if !seen[table.Name] {
				seen[table.Name] = true
				uniqueTables = append(uniqueTables, table)
			}
		}
		allTables = uniqueTables

		fmt.Printf("Parsed %d tables\n", len(allTables))
		fmt.Println()
	}

	// Count by type
	var queryCount, mutationCount, actionCount int
	for _, fn := range allFunctions {
		switch fn.Type {
		case FunctionTypeQuery:
			queryCount++
		case FunctionTypeMutation:
			mutationCount++
		case FunctionTypeAction:
			actionCount++
		}
	}

	// Generate hooks
	if config.Generators.Hooks {
		fmt.Println("Generating hooks...")
		hooksGen := NewHooksGenerator(config)
		if err := hooksGen.Generate(allFunctions); err != nil {
			return fmt.Errorf("failed to generate hooks: %w", err)
		}
		fmt.Printf("  %d query hooks\n", queryCount)
		fmt.Printf("  %d mutation hooks\n", mutationCount)
		fmt.Printf("  %d action hooks\n", actionCount)
		fmt.Printf("  Output: %s\n", config.GetHooksOutputDir())
		fmt.Println()
	}

	// Generate API wrappers
	if config.Generators.API {
		fmt.Println("Generating API wrappers...")
		apiGen := NewAPIGenerator(config)
		if err := apiGen.Generate(allFunctions); err != nil {
			return fmt.Errorf("failed to generate API wrappers: %w", err)
		}
		fmt.Printf("  Output: %s\n", config.GetAPIOutputDir())
		fmt.Println()
	}

	// Generate types
	if config.Generators.Types {
		fmt.Println("Generating types...")
		typesGen := NewTypesGenerator(config)
		if err := typesGen.Generate(allTables); err != nil {
			return fmt.Errorf("failed to generate types: %w", err)
		}
		fmt.Printf("  %d table types\n", len(allTables))
		fmt.Printf("  %d ID types\n", len(allTables))
		fmt.Printf("  Output: %s\n", config.GetTypesOutputDir())
		fmt.Println()
	}

	fmt.Println("Generation complete!")

	return nil
}
