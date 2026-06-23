package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTerraformGeneratorWritesFiles(t *testing.T) {
	dir := t.TempDir()

	// Minimal curation overlay covering all four verbs, a wire rename, and a
	// computed field.
	cfgPath := filepath.Join(dir, "convex-terraform-gen.json")
	if err := os.WriteFile(cfgPath, []byte(`{"resources":{"vehicles":{
	  "table":"vehicles","path":"vehicles","module":"vehicles/vehiclesApi.ts",
	  "ownership":"ownerId","verbs":["create","read","update","delete"],
	  "expose":["make","model",{"field":"forSale","wire":"for_sale"}],
	  "computed":{"created_at":{"as":"iso8601","from":"_creationTime"}}
	}}}`), 0o644); err != nil {
		t.Fatalf("write overlay: %v", err)
	}

	cfg := &Config{Org: "@dashtag"}
	cfg.Convex.Path = filepath.Join(dir, "convex")
	cfg.Terraform.ConfigPath = cfgPath
	cfg.OpenAPI.OutputDir = filepath.Join(dir, "convex", "api")
	cfg.OpenAPI.FileName = "openapi.v1.yaml"

	g := NewTerraformGenerator(cfg)
	if err := g.Generate([]TableInfo{vehiclesTable()}); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	// vehiclesApi.ts written at the overlay module path, with the curated input
	// validator.
	apiPath := filepath.Join(cfg.Convex.Path, "vehicles", "vehiclesApi.ts")
	apiData, err := os.ReadFile(apiPath)
	if err != nil {
		t.Fatalf("read vehiclesApi.ts: %v", err)
	}
	if !strings.Contains(string(apiData), "vehicleApiInput") {
		t.Fatalf("vehiclesApi.ts missing vehicleApiInput:\n%s", apiData)
	}
	if !strings.Contains(string(apiData), "for_sale") {
		t.Errorf("vehiclesApi.ts missing wire-renamed for_sale field")
	}

	// The wire types live in a separate <domain>/types/<lcSingular>Api.types.ts
	// file, and the Api module imports them from there.
	typesPath := filepath.Join(cfg.Convex.Path, "vehicles", "types", "vehicleApi.types.ts")
	typesData, err := os.ReadFile(typesPath)
	if err != nil {
		t.Fatalf("read vehicleApi.types.ts: %v", err)
	}
	for _, frag := range []string{"export type VehicleApiInput = {", "export type VehicleApiPatch = Partial<VehicleApiInput>;"} {
		if !strings.Contains(string(typesData), frag) {
			t.Errorf("vehicleApi.types.ts missing %q:\n%s", frag, typesData)
		}
	}
	if strings.Contains(string(apiData), "export type VehicleApiInput = {") {
		t.Errorf("vehiclesApi.ts must NOT declare wire types inline:\n%s", apiData)
	}
	if !strings.Contains(string(apiData), `from "./types/vehicleApi.types"`) {
		t.Errorf("vehiclesApi.ts must import wire types from ./types/vehicleApi.types:\n%s", apiData)
	}

	// vehiclesRoutes.ts written in convex/api with the pluralized name.
	routesPath := filepath.Join(cfg.Convex.Path, "api", "vehiclesRoutes.ts")
	routesData, err := os.ReadFile(routesPath)
	if err != nil {
		t.Fatalf("read vehiclesRoutes.ts: %v", err)
	}
	if !strings.Contains(string(routesData), "registerVehicleRoutes") {
		t.Errorf("vehiclesRoutes.ts missing registerVehicleRoutes")
	}

	// generator_config.yml written beside the OpenAPI output dir.
	cfgYML := filepath.Join(cfg.OpenAPI.OutputDir, "generator_config.yml")
	ymlData, err := os.ReadFile(cfgYML)
	if err != nil {
		t.Fatalf("read generator_config.yml: %v", err)
	}
	for _, frag := range []string{"provider:", "name: dashtag", "vehicle:", "method: POST"} {
		if !strings.Contains(string(ymlData), frag) {
			t.Errorf("generator_config.yml missing fragment %q", frag)
		}
	}
}

func TestTerraformGeneratorUnknownTable(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "convex-terraform-gen.json")
	if err := os.WriteFile(cfgPath, []byte(`{"resources":{"ghost":{
	  "table":"ghost","path":"ghost","module":"ghost/ghostApi.ts",
	  "verbs":["read"],"expose":["name"]
	}}}`), 0o644); err != nil {
		t.Fatalf("write overlay: %v", err)
	}

	cfg := &Config{Org: "@dashtag"}
	cfg.Convex.Path = filepath.Join(dir, "convex")
	cfg.Terraform.ConfigPath = cfgPath
	cfg.OpenAPI.OutputDir = filepath.Join(dir, "convex", "api")

	g := NewTerraformGenerator(cfg)
	err := g.Generate([]TableInfo{vehiclesTable()})
	if err == nil {
		t.Fatal("expected error for unknown table, got nil")
	}
	if !strings.Contains(err.Error(), "unknown table") {
		t.Errorf("error = %v, want it to mention unknown table", err)
	}
}
