package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Config represents the .convex-gen.json configuration
type Config struct {
	Org        string           `json:"org"`        // e.g., "@dashtag"
	Convex     ConvexConfig     `json:"convex"`     // Convex backend configuration
	DataLayer  DataLayerConfig  `json:"dataLayer"`  // Data layer output configuration
	Imports    ImportsConfig    `json:"imports"`    // Import path configuration
	Generators GeneratorsConfig `json:"generators"` // Which generators to run
	Skip       SkipConfig       `json:"skip"`       // Files/patterns to skip
	AI         AIConfig         `json:"ai"`         // AI tool catalog generator policy (opt-in)
	OpenAPI    OpenAPIConfig    `json:"openapi"`    // OpenAPI spec generator policy (opt-in)
	Terraform  TerraformConfig  `json:"terraform"`  // Terraform/public-API emitter policy (opt-in)
}

// TerraformConfig controls the Terraform/public-API emitter (opt-in). It points
// at a separate convex-terraform-gen.json that holds the per-resource curation
// overlay. Off unless `generators.terraform` is true.
type TerraformConfig struct {
	ConfigPath string `json:"config"` // repo-root-relative path to convex-terraform-gen.json
}

// OpenAPIConfig controls the OpenAPI spec generator (opt-in). The generator
// scans the Convex tree for `*Api.ts` files and emits an OpenAPI 3.1 spec from
// the exported `<resource>ApiInput` / `<resource>ApiPatch` / `<resource>ApiOutput`
// validators in each. Off unless `generators.openapi` is true.
type OpenAPIConfig struct {
	OutputDir string `json:"outputDir"` // repo-root-relative dir for the spec; default "<dataLayer.path>/generated-openapi"
	FileName  string `json:"fileName"`  // spec filename; default "openapi.yaml"
	Title     string `json:"title"`     // info.title; default "<org> API"
	Version   string `json:"version"`   // info.version; default "1.0.0"
	ServerURL string `json:"serverUrl"` // servers[0].url
	BasePath  string `json:"basePath"`  // path prefix; default "/api/v1"
}

// AIConfig controls the AI tool catalog generator (opt-in).
type AIConfig struct {
	OutputDir         string            `json:"outputDir"`         // default "generated-ai"
	Deny              []string          `json:"deny"`              // glob patterns excluded from the catalog
	ForceRead         []string          `json:"forceRead"`         // query-like actions/mutations to classify as read
	DescriptionSource string            `json:"descriptionSource"` // "fallback" (default) | "jsdoc" (Phase 3)
	Descriptions      map[string]string `json:"descriptions"`      // fnPath -> override description
}

// ConvexConfig configures where to find Convex functions
type ConvexConfig struct {
	Path         string `json:"path"`         // e.g., "packages/backend"
	SchemaPath   string `json:"schemaPath"`   // e.g., "packages/backend/schema" or "packages/backend/schema.ts"
	Structure    string `json:"structure"`    // "nested" or "flat"
	FluentConvex bool   `json:"fluentConvex"` // Toggle fluent-convex builder chain parsing
}

// DataLayerConfig configures output locations
type DataLayerConfig struct {
	Path          string `json:"path"`          // e.g., "packages/data-layer/src"
	HooksDir      string `json:"hooksDir"`      // e.g., "generated-hooks"
	APIDir        string `json:"apiDir"`        // e.g., "generated-api"
	TypesDir      string `json:"typesDir"`      // e.g., "generated-types"
	MetadataDir   string `json:"metadataDir"`   // e.g., "generated-schema"
	FileStructure string `json:"fileStructure"` // "grouped", "split", or "both"
	HookNaming    string `json:"hookNaming"`    // "flat" (no sub-namespace), "qualified" (always sub-namespace), or "auto" (sub-namespace only on collision)
	ExportAPI     bool   `json:"exportApi"`     // Re-export { api } from the generated-api index
	TypedReturns  bool   `json:"typedReturns"`  // When true, emit typed `FunctionReturnType<typeof api.x.y> | undefined` on shouldSkip query hooks instead of `as any`
	TypedArgs     bool   `json:"typedArgs"`     // When true, emit typed `ReactMutation<typeof api.x.y>` / `ReactAction<...>` annotations on mutation/action hooks so caller args are type-checked. Defaults to false (untyped) for backwards compatibility.

	// RequireAuthGatedShouldSkip: when true, a query hook whose backend handler
	// calls one of AuthHelperNames gets a REQUIRED `shouldSkip: boolean` param
	// instead of the default `shouldSkip?: boolean`. Forces every caller to
	// reckon with the unauthenticated case at compile time instead of a runtime
	// ConvexError. Defaults to false for backwards compatibility — other
	// projects using this same convex-gen binary are unaffected unless they
	// opt in.
	RequireAuthGatedShouldSkip bool `json:"requireAuthGatedShouldSkip"`
	// AuthHelperNames lists the backend helper function calls (e.g.
	// `getAuthenticatedUser(ctx)`) that mark a query as auth-gated for the
	// RequireAuthGatedShouldSkip check. Defaults to
	// ["getAuthenticatedUser", "getAuthenticatedUserForActions"] when empty.
	AuthHelperNames []string `json:"authHelperNames"`
}

// ImportsConfig configures how generated code imports dependencies
type ImportsConfig struct {
	Style     string `json:"style"`     // "package" (recommended) or "relative"
	API       string `json:"api"`       // e.g., "@dashtag/backend/api" or relative path
	DataModel string `json:"dataModel"` // e.g., "@dashtag/backend/dataModel" or relative path
}

// GeneratorsConfig controls which generators run
type GeneratorsConfig struct {
	Hooks     bool `json:"hooks"`
	API       bool `json:"api"`
	Types     bool `json:"types"`
	Metadata  bool `json:"metadata"`
	AICatalog bool `json:"aiCatalog"`
	OpenAPI   bool `json:"openapi"`
	Terraform bool `json:"terraform"`
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
	if config.DataLayer.MetadataDir == "" {
		config.DataLayer.MetadataDir = "generated-schema"
	}
	if config.DataLayer.FileStructure == "" {
		config.DataLayer.FileStructure = "grouped" // default to grouped (single file per namespace)
	}
	if config.DataLayer.HookNaming == "" {
		config.DataLayer.HookNaming = "flat" // default to flat for backward compatibility
	}
	if len(config.DataLayer.AuthHelperNames) == 0 {
		config.DataLayer.AuthHelperNames = []string{"getAuthenticatedUser", "getAuthenticatedUserForActions"}
	}

	// AI tool catalog defaults
	if config.AI.OutputDir == "" {
		config.AI.OutputDir = "generated-ai"
	}
	if config.AI.DescriptionSource == "" {
		config.AI.DescriptionSource = "fallback"
	}

	// OpenAPI spec defaults. Note: OpenAPI is deliberately NOT part of the
	// "all generators off → enable all" block below, so it stays opt-in and a
	// repo's output is unchanged unless it sets generators.openapi: true.
	if config.OpenAPI.OutputDir == "" {
		config.OpenAPI.OutputDir = filepath.Join(config.DataLayer.Path, "generated-openapi")
	}
	if config.OpenAPI.FileName == "" {
		config.OpenAPI.FileName = "openapi.yaml"
	}
	if config.OpenAPI.Version == "" {
		config.OpenAPI.Version = "1.0.0"
	}
	if config.OpenAPI.BasePath == "" {
		config.OpenAPI.BasePath = "/api/v1"
	}
	if config.OpenAPI.Title == "" {
		config.OpenAPI.Title = config.Org + " API"
	}

	// Terraform emitter default config path. Opt-in like OpenAPI, so not part of
	// the "all generators off → enable all" block.
	if config.Terraform.ConfigPath == "" {
		config.Terraform.ConfigPath = "convex-terraform-gen.json"
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
	if !config.Generators.Hooks && !config.Generators.API && !config.Generators.Types && !config.Generators.Metadata {
		config.Generators.Hooks = true
		config.Generators.API = true
		config.Generators.Types = true
		config.Generators.Metadata = true
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
			"^_",         // Files starting with underscore
			"\\.test\\.", // Test files
			"\\.spec\\.", // Spec files
			"^debug",     // Debug files
			"^migrate",   // Migration files
			"^seed",      // Seed files
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

// GetMetadataOutputDir returns the full path for generated schema metadata
func (c *Config) GetMetadataOutputDir() string {
	return filepath.Join(c.DataLayer.Path, c.DataLayer.MetadataDir)
}

// GetAICatalogOutputDir returns the full path for the generated AI tool catalog.
func (c *Config) GetAICatalogOutputDir() string {
	return filepath.Join(c.DataLayer.Path, c.AI.OutputDir)
}

// GetOpenAPISpecPath returns the full path for the generated OpenAPI spec file.
func (c *Config) GetOpenAPISpecPath() string {
	return filepath.Join(c.OpenAPI.OutputDir, c.OpenAPI.FileName)
}

// GetTerraformConfigPath returns the path to the Terraform curation overlay.
func (c *Config) GetTerraformConfigPath() string {
	return c.Terraform.ConfigPath
}

// IsSchemaDirectory returns true if schema is a directory (vs single file)
func (c *Config) IsSchemaDirectory() bool {
	info, err := os.Stat(c.Convex.SchemaPath)
	if err != nil {
		return true // default to directory style
	}
	return info.IsDir()
}
