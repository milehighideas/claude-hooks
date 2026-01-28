package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// ConvexFile represents a TypeScript file containing Convex functions
type ConvexFile struct {
	Path      string // Full path to file
	Namespace string // Namespace derived from directory structure
	FileName  string // File name without extension
}

// Scanner finds TypeScript files in Convex directory
type Scanner struct {
	config       *Config
	skipDirSet   map[string]bool
	skipPatterns []*regexp.Regexp
}

// NewScanner creates a new scanner with compiled patterns
func NewScanner(config *Config) (*Scanner, error) {
	// Build skip directory set
	skipDirSet := make(map[string]bool)
	for _, dir := range config.Skip.Directories {
		skipDirSet[dir] = true
	}

	// Compile skip patterns
	var patterns []*regexp.Regexp
	for _, pattern := range config.Skip.Patterns {
		re, err := regexp.Compile(pattern)
		if err != nil {
			return nil, err
		}
		patterns = append(patterns, re)
	}

	return &Scanner{
		config:       config,
		skipDirSet:   skipDirSet,
		skipPatterns: patterns,
	}, nil
}

// ScanConvexDirectory finds all TypeScript files with Convex functions
func (s *Scanner) ScanConvexDirectory() ([]ConvexFile, error) {
	var files []ConvexFile

	convexDir := s.config.Convex.Path

	err := filepath.Walk(convexDir, func(path string, info os.FileInfo, err error) error {
		// DEBUG: Log each file we encounter
		if strings.HasSuffix(path, "projects.ts") {
		}
		if err != nil {
			return err
		}

		// Skip directories
		if info.IsDir() {
			dirName := info.Name()

			// Skip configured directories
			if s.skipDirSet[dirName] {
				return filepath.SkipDir
			}

			// Skip schema directory (handled separately)
			if dirName == "schema" || dirName == "schemas" {
				return filepath.SkipDir
			}

			return nil
		}

		// Only process TypeScript files
		if !strings.HasSuffix(path, ".ts") {
			return nil
		}

		// Skip .d.ts files
		if strings.HasSuffix(path, ".d.ts") {
			return nil
		}

		fileName := info.Name()

		// DEBUG: Check projects.ts specifically
		if fileName == "projects.ts" {
		}

		// Check skip patterns
		for _, pattern := range s.skipPatterns {
			if pattern.MatchString(fileName) {
				if fileName == "projects.ts" {
				}
				return nil
			}
		}

		// Skip special files
		if s.isSpecialFile(fileName) {
			if fileName == "projects.ts" {
			}
			return nil
		}

		if fileName == "projects.ts" {
		}

		// Calculate namespace from relative path
		relPath, err := filepath.Rel(convexDir, path)
		if err != nil {
			return err
		}

		// Get directory part as namespace
		dir := filepath.Dir(relPath)
		namespace := ""
		if dir != "." {
			namespace = dir
		}

		// Calculate namespace based on structure
		baseName := strings.TrimSuffix(fileName, ".ts")
		if namespace == "" {
			// File is at root level - use filename as namespace
			namespace = baseName
		} else if s.config.Convex.Structure == "flat" {
			// For flat structure with files in subdirectories,
			// use directory/filename to avoid namespace collisions
			namespace = filepath.Join(namespace, baseName)
		} else {
			// For nested structure, include filename in namespace path
			namespace = filepath.Join(namespace, baseName)
		}

		files = append(files, ConvexFile{
			Path:      path,
			Namespace: namespace,
			FileName:  baseName,
		})

		return nil
	})

	return files, err
}

// isSpecialFile checks if a file should be skipped (Convex config files, etc.)
func (s *Scanner) isSpecialFile(fileName string) bool {
	specialFiles := map[string]bool{
		"convex.config.ts": true,
		"auth.config.ts":   true,
		"crons.ts":         true,
		"http.ts":          true,
		"schema.ts":        true,
		"migrations.ts":    true,
		"index.ts":         true, // Usually just re-exports
	}
	return specialFiles[fileName]
}

// SchemaFile represents a schema definition file
type SchemaFile struct {
	Path   string
	Domain string // Domain name from directory or filename
}

// ScanSchemaFiles finds schema definition files
func (s *Scanner) ScanSchemaFiles() ([]SchemaFile, error) {
	var files []SchemaFile

	schemaPath := s.config.Convex.SchemaPath

	// Check for main schema files in order of preference:
	// 1. schemaPath.ts (e.g., packages/backend/schema.ts)
	// 2. schemaPath/index.ts (e.g., packages/backend/schema/index.ts)
	mainSchemaPath := schemaPath + ".ts"
	indexSchemaPath := filepath.Join(schemaPath, "index.ts")

	// Check which main schema file exists and contains defineSchema
	var mainFile string
	for _, path := range []string{mainSchemaPath, indexSchemaPath} {
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			// Check if this file contains defineSchema
			content, err := os.ReadFile(path)
			if err == nil && strings.Contains(string(content), "defineSchema(") {
				mainFile = path
				break
			}
		}
	}

	if mainFile != "" {
		files = append(files, SchemaFile{
			Path:   mainFile,
			Domain: "main",
		})
	}

	info, err := os.Stat(schemaPath)
	if os.IsNotExist(err) {
		// No schema directory found, but we may have found the main schema.ts
		return files, nil
	}
	if err != nil {
		return nil, err
	}

	if !info.IsDir() {
		// Single schema file (same as schemaPath)
		if len(files) == 0 { // Don't add twice if already added as main
			files = append(files, SchemaFile{
				Path:   schemaPath,
				Domain: "root",
			})
		}
		return files, nil
	}

	// Directory of schema files
	err = filepath.Walk(schemaPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		// Only process .ts files
		if !strings.HasSuffix(path, ".ts") {
			return nil
		}

		// Skip index files
		if info.Name() == "index.ts" {
			return nil
		}

		// Get domain from directory or filename
		relPath, _ := filepath.Rel(schemaPath, path)
		dir := filepath.Dir(relPath)
		domain := dir
		if domain == "." {
			domain = strings.TrimSuffix(info.Name(), ".ts")
			domain = strings.TrimSuffix(domain, ".schema")
		}

		files = append(files, SchemaFile{
			Path:   path,
			Domain: domain,
		})

		return nil
	})

	return files, err
}
