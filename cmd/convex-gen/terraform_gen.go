package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// TerraformGenerator orchestrates the Terraform/public-API emitter: it loads the
// curation overlay, resolves each opt-in resource against the parsed schema, and
// writes the curated <res>Api.ts, <res>Routes.ts, and the tfplugingen-openapi
// generator_config.yml.
type TerraformGenerator struct{ config *Config }

// NewTerraformGenerator constructs a TerraformGenerator bound to the config.
func NewTerraformGenerator(config *Config) *TerraformGenerator {
	return &TerraformGenerator{config: config}
}

// Generate resolves every resource in the overlay and writes its emitted files.
func (g *TerraformGenerator) Generate(tables []TableInfo) error {
	spec, err := LoadTerraformSpec(g.config.GetTerraformConfigPath())
	if err != nil {
		return err
	}

	byName := map[string]TableInfo{}
	for _, t := range tables {
		byName[t.Name] = t
	}

	// Resolve resources in a stable order (by overlay key) so emitted output is
	// deterministic regardless of map iteration order.
	names := make([]string, 0, len(spec.Resources))
	for name := range spec.Resources {
		names = append(names, name)
	}
	sort.Strings(names)

	var resolved []ResolvedResource
	for _, name := range names {
		rs := spec.Resources[name]
		tbl, ok := byName[rs.Table]
		if !ok {
			return fmt.Errorf("terraform resource %q references unknown table %q", name, rs.Table)
		}
		r, err := ResolveResource(tbl, rs)
		if err != nil {
			return err
		}
		resolved = append(resolved, r)

		// Write the curated Api.ts at the overlay's module path.
		apiPath := filepath.Join(g.config.Convex.Path, rs.Module)
		if err := writeGeneratedFile(apiPath, EmitApiTS(r)); err != nil {
			return err
		}

		// Write the wire types to <domain>/types/<lcSingular>Api.types.ts so the
		// type exports live under types/ (SRP gate).
		typesPath := filepath.Join(g.config.Convex.Path, apiTypesFileRelPath(r))
		if err := writeGeneratedFile(typesPath, EmitApiTypesTS(r)); err != nil {
			return err
		}

		// Write the routes file in convex/api. By default this keeps the
		// <table>Routes.ts convention the existing backend uses; when a Name
		// override disambiguates two resources sharing one table, the file is keyed
		// on the singular symbol base (<lcSingular>Routes.ts) so the two routes
		// files do not collide on the shared table name.
		routesPath := filepath.Join(g.config.Convex.Path, "api", routesFileBasename(r))
		if err := writeGeneratedFile(routesPath, EmitRoutesTS(r)); err != nil {
			return err
		}
	}

	// generator_config.yml for the HashiCorp codegen tools, written beside the
	// OpenAPI spec output so the provider repo can consume both together.
	cfgYML := filepath.Join(g.config.OpenAPI.OutputDir, "generator_config.yml")
	if err := writeGeneratedFile(cfgYML, EmitGeneratorConfig(resolved)); err != nil {
		return err
	}
	return nil
}

// writeGeneratedFile creates the parent directory and writes the file content.
func writeGeneratedFile(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating directory for %s: %w", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}
