package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Config represents the .convex-gen.json configuration
type Config struct {
	Org       string          `json:"org"`       // e.g., "@dashtag"
	Convex    ConvexConfig    `json:"convex"`    // Convex backend configuration
	DataLayer DataLayerConfig `json:"dataLayer"` // Data layer output configuration
	Imports   ImportsConfig   `json:"imports"`   // Import path configuration
	Generators GeneratorsConfig `json:"generators"` // Which generators to run
	Skip      SkipConfig      `json:"skip"`      // Files/patterns to skip
}

// ConvexConfig configures where to find Convex functions
type ConvexConfig struct {
	Path       string `json:"path"`       // e.g., "packages/backend"
	SchemaPath string `json:"schemaPath"` // e.g., "packages/backend/schema" or "packages/backend/schema.ts"
	Structure  string `json:"structure"`  // "nested" or "flat"
}

// DataLayerConfig configures output locations
type DataLayerConfig struct {
	Path          string `json:"path"`          // e.g., "packages/data-layer/src"
	HooksDir      string `json:"hooksDir"`      // e.g., "generated-hooks"
	APIDir        string `json:"apiDir"`        // e.g., "generated-api"
	TypesDir      string `json:"typesDir"`      // e.g., "generated-types"
	FileStructure string `json:"fileStructure"` // "grouped", "split", or "both"
}

// ImportsConfig configures how generated code imports dependencies
type ImportsConfig struct {
	Style     string `json:"style"`     // "package" (recommended) or "relative"
	API       string `json:"api"`       // e.g., "@dashtag/backend/api" or relative path
	DataModel string `json:"dataModel"` // e.g., "@dashtag/backend/dataModel" or relative path
}

// GeneratorsConfig controls which generators run
type GeneratorsConfig struct {
	Hooks bool `json:"hooks"`
	API   bool `json:"api"`
	Types bool `json:"types"`
}

// SkipConfig configures files/directories to skip
type SkipConfig struct {
	Directories []string `json:"directories"` // Directory names to skip
	Patterns    []string `json:"patterns"`    // Regex patterns for files to skip
}

// LoadConfig loads configuration from .convex-gen.json
func LoadConfig() (*Config, error) {
	// Try multiple config file names
	configNames := []string{".convex-gen.json", "convex-gen.json"}

	var configPath string
	for _, name := range configNames {
		if _, err := os.Stat(name); err == nil {
			configPath = name
			break
		}
	}

	if configPath == "" {
		return nil, fmt.Errorf("config file not found (tried: %v)", configNames)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	// Apply defaults
	applyConfigDefaults(&config)

	// Validate
	if err := validateConfig(&config); err != nil {
		return nil, err
	}

	return &config, nil
}

// applyConfigDefaults sets sensible defaults for missing values
func applyConfigDefaults(config *Config) {
	// Convex defaults
	if config.Convex.Path == "" {
		config.Convex.Path = "packages/backend"
	}
	if config.Convex.Structure == "" {
		config.Convex.Structure = "nested"
	}
	if config.Convex.SchemaPath == "" {
		// Try to detect schema location
		schemaDir := filepath.Join(config.Convex.Path, "schema")
		schemaFile := filepath.Join(config.Convex.Path, "schema.ts")
		if _, err := os.Stat(schemaDir); err == nil {
			config.Convex.SchemaPath = schemaDir
		} else if _, err := os.Stat(schemaFile); err == nil {
			config.Convex.SchemaPath = schemaFile
		} else {
			config.Convex.SchemaPath = schemaDir // default to directory style
		}
	}

	// DataLayer defaults
	if config.DataLayer.Path == "" {
		config.DataLayer.Path = "packages/data-layer/src"
	}
	if config.DataLayer.HooksDir == "" {
		config.DataLayer.HooksDir = "generated-hooks"
	}
	if config.DataLayer.APIDir == "" {
		config.DataLayer.APIDir = "generated-api"
	}
	if config.DataLayer.TypesDir == "" {
		config.DataLayer.TypesDir = "generated-types"
	}
	if config.DataLayer.FileStructure == "" {
		config.DataLayer.FileStructure = "grouped" // default to grouped (single file per namespace)
	}

	// Imports defaults - prefer package aliases
	if config.Imports.Style == "" {
		config.Imports.Style = "package"
	}
	if config.Imports.API == "" {
		if config.Imports.Style == "package" && config.Org != "" {
			config.Imports.API = config.Org + "/backend/api"
		} else {
			config.Imports.API = "../../../backend/_generated/api"
		}
	}
	if config.Imports.DataModel == "" {
		if config.Imports.Style == "package" && config.Org != "" {
			config.Imports.DataModel = config.Org + "/backend/dataModel"
		} else {
			config.Imports.DataModel = "../../../backend/_generated/dataModel"
		}
	}

	// Generator defaults - all enabled
	if !config.Generators.Hooks && !config.Generators.API && !config.Generators.Types {
		config.Generators.Hooks = true
		config.Generators.API = true
		config.Generators.Types = true
	}

	// Skip defaults
	if len(config.Skip.Directories) == 0 {
		config.Skip.Directories = []string{
			"_generated",
			"node_modules",
			".turbo",
		}
	}
	if len(config.Skip.Patterns) == 0 {
		config.Skip.Patterns = []string{
			"^_",           // Files starting with underscore
			"\\.test\\.",   // Test files
			"\\.spec\\.",   // Spec files
			"^debug",       // Debug files
			"^migrate",     // Migration files
			"^seed",        // Seed files
		}
	}
}

// validateConfig checks config for errors
func validateConfig(config *Config) error {
	if config.Org == "" {
		return fmt.Errorf("org is required (e.g., \"@dashtag\")")
	}

	if _, err := os.Stat(config.Convex.Path); os.IsNotExist(err) {
		return fmt.Errorf("convex path does not exist: %s", config.Convex.Path)
	}

	if config.Convex.Structure != "nested" && config.Convex.Structure != "flat" {
		return fmt.Errorf("convex.structure must be 'nested' or 'flat', got: %s", config.Convex.Structure)
	}

	return nil
}

// GetHooksOutputDir returns the full path for generated hooks
func (c *Config) GetHooksOutputDir() string {
	return filepath.Join(c.DataLayer.Path, c.DataLayer.HooksDir)
}

// GetAPIOutputDir returns the full path for generated API wrappers
func (c *Config) GetAPIOutputDir() string {
	return filepath.Join(c.DataLayer.Path, c.DataLayer.APIDir)
}

// GetTypesOutputDir returns the full path for generated types
func (c *Config) GetTypesOutputDir() string {
	return filepath.Join(c.DataLayer.Path, c.DataLayer.TypesDir)
}

// IsSchemaDirectory returns true if schema is a directory (vs single file)
func (c *Config) IsSchemaDirectory() bool {
	info, err := os.Stat(c.Convex.SchemaPath)
	if err != nil {
		return true // default to directory style
	}
	return info.IsDir()
}
