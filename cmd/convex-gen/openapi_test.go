package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const vehiclesApiModule = `
import { v } from "convex/values";

export const vehicleApiInput = {
  make: v.string(),
  model: v.string(),
  year: v.optional(v.number()),
  for_sale: v.optional(v.boolean()),
  status: v.union(v.literal("draft"), v.literal("active")),
};

export const vehicleApiPatch = {
  make: v.optional(v.string()),
  mileage: v.optional(v.number()),
};

export const vehicleApiOutput = v.object({
  id: v.id("vehicles"),
  make: v.string(),
  for_sale: v.boolean(),
  photo_urls: v.array(v.string()),
  created_at: v.string(),
});
`

func writeOpenAPIFixture(t *testing.T) (*Config, string) {
	t.Helper()
	tmp := t.TempDir()
	convexDir := filepath.Join(tmp, "convex")
	vehDir := filepath.Join(convexDir, "vehicles")
	if err := os.MkdirAll(vehDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(vehDir, "vehiclesApi.ts"), []byte(vehiclesApiModule), 0o644); err != nil {
		t.Fatalf("write module: %v", err)
	}
	// A non-API file and a test file must be ignored.
	if err := os.WriteFile(filepath.Join(vehDir, "vehicleQueries.ts"), []byte("export const x = 1;"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.WriteFile(filepath.Join(vehDir, "vehiclesApi.test.ts"), []byte("// test"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	outDir := filepath.Join(tmp, "out")
	cfg := &Config{
		Org:    "@acme",
		Convex: ConvexConfig{Path: convexDir, Structure: "flat"},
		Skip:   SkipConfig{Directories: []string{"_generated", "node_modules"}},
		OpenAPI: OpenAPIConfig{
			OutputDir: outDir,
			FileName:  "openapi.yaml",
			Title:     "Acme API",
			Version:   "1.0.0",
			ServerURL: "https://acme.example/api",
			BasePath:  "/api/v1",
		},
	}
	return cfg, filepath.Join(outDir, "openapi.yaml")
}

func TestOpenAPIGenerator_EmitsResource(t *testing.T) {
	cfg, specPath := writeOpenAPIFixture(t)

	n, err := NewOpenAPIGenerator(cfg).Generate()
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 resource, got %d", n)
	}

	data, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatalf("read spec: %v", err)
	}
	spec := string(data)

	wantContains := []string{
		"openapi: 3.1.0",
		"title: Acme API",
		"url: https://acme.example/api",
		"/api/v1/vehicles:",
		"/api/v1/vehicles/{id}:",
		"operationId: createVehicle",
		"operationId: getVehicle",
		"operationId: updateVehicle",
		"operationId: deleteVehicle",
		"#/components/schemas/VehicleInput",
		"#/components/schemas/VehiclePatch",
		"    Vehicle:",
		"    VehicleInput:",
		"    VehiclePatch:",
		"bearerAuth:",
		// status union → enum
		`enum: ["draft", "active"]`,
		// array field
		"items: { type: string }",
	}
	for _, w := range wantContains {
		if !strings.Contains(spec, w) {
			t.Errorf("spec missing %q\n---\n%s", w, spec)
		}
	}
}

func TestOpenAPIGenerator_RequiredAndReadOnly(t *testing.T) {
	cfg, specPath := writeOpenAPIFixture(t)
	if _, err := NewOpenAPIGenerator(cfg).Generate(); err != nil {
		t.Fatalf("generate: %v", err)
	}
	data, _ := os.ReadFile(specPath)
	spec := string(data)

	// VehicleInput requires make + model (the two non-optional input fields).
	inputBlock := section(spec, "    VehicleInput:")
	for _, req := range []string{"- make", "- model"} {
		if !strings.Contains(inputBlock, req) {
			t.Errorf("VehicleInput should require %s\n%s", req, inputBlock)
		}
	}

	// Output fields absent from input/patch are read-only: id, photo_urls, created_at.
	outBlock := section(spec, "    Vehicle:")
	for _, ro := range []string{"id", "photo_urls", "created_at"} {
		if !strings.Contains(outBlock, ro+":") {
			t.Errorf("Vehicle output missing field %s", ro)
		}
	}
	if !strings.Contains(outBlock, "readOnly: true") {
		t.Errorf("Vehicle output should mark read-only fields\n%s", outBlock)
	}
	// `make` is writable (present in input) → must NOT be read-only in output.
	if strings.Contains(propertyBlock(outBlock, "make"), "readOnly") {
		t.Errorf("make should not be read-only in output")
	}
}

func TestOpenAPIGenerator_IgnoresModulesWithoutValidators(t *testing.T) {
	tmp := t.TempDir()
	convexDir := filepath.Join(tmp, "convex")
	if err := os.MkdirAll(convexDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// A *Api.ts file with no Input/Output validators must be skipped.
	if err := os.WriteFile(filepath.Join(convexDir, "emptyApi.ts"), []byte("export const foo = 1;"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := &Config{
		Org:     "@acme",
		Convex:  ConvexConfig{Path: convexDir, Structure: "flat"},
		OpenAPI: OpenAPIConfig{OutputDir: filepath.Join(tmp, "out"), FileName: "openapi.yaml", BasePath: "/api/v1"},
	}
	n, err := NewOpenAPIGenerator(cfg).Generate()
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected 0 resources, got %d", n)
	}
}

// TestOpenAPI_StaysOptIn is the core opt-in guarantee: a config that does not
// set generators.openapi never has it enabled by defaulting — neither when other
// generators are set, nor via the "all off → enable all" fallback.
func TestOpenAPI_StaysOptIn(t *testing.T) {
	allOff := &Config{Org: "@acme", Convex: ConvexConfig{Path: ".", Structure: "flat"}}
	applyConfigDefaults(allOff)
	if !allOff.Generators.Hooks {
		t.Error("expected the all-off fallback to enable hooks")
	}
	if allOff.Generators.OpenAPI {
		t.Error("openapi must NOT be enabled by the all-off fallback")
	}

	someOn := &Config{Org: "@acme", Generators: GeneratorsConfig{Hooks: true}}
	applyConfigDefaults(someOn)
	if someOn.Generators.OpenAPI {
		t.Error("openapi must NOT auto-enable when other generators are set")
	}
}

// section returns the spec substring starting at header up to the next
// top-level-ish boundary, for scoped assertions.
func section(spec, header string) string {
	idx := strings.Index(spec, header)
	if idx < 0 {
		return ""
	}
	rest := spec[idx+len(header):]
	// stop at the next schema entry at the same indent ("    X:") or components boundary
	lines := strings.Split(rest, "\n")
	var out []string
	for i, ln := range lines {
		if i > 0 && strings.HasPrefix(ln, "    ") && !strings.HasPrefix(ln, "      ") && strings.HasSuffix(strings.TrimSpace(ln), ":") {
			break
		}
		out = append(out, ln)
	}
	return header + strings.Join(out, "\n")
}

// propertyBlock returns the lines for one property under a schema section.
func propertyBlock(schemaBlock, prop string) string {
	idx := strings.Index(schemaBlock, "        "+prop+":")
	if idx < 0 {
		return ""
	}
	rest := schemaBlock[idx:]
	lines := strings.Split(rest, "\n")
	var out []string
	for i, ln := range lines {
		if i > 0 && strings.HasPrefix(ln, "        ") && !strings.HasPrefix(ln, "          ") {
			break
		}
		out = append(out, ln)
	}
	return strings.Join(out, "\n")
}

// TestWriteProperty_NestedObjectArray locks the nested-object-array OpenAPI
// emission: a v.array(v.object({...})) field renders as `type: array` with an
// `items` object schema carrying the inner properties + required list, instead
// of a bare `items: { type: object }`.
func TestWriteProperty_NestedObjectArray(t *testing.T) {
	f := FieldInfo{
		Name: "parts_used", Type: "array", IsArray: true, ArrayType: "object",
		Nested: []FieldInfo{
			{Name: "name", Type: "string"},
			{Name: "quantity", Type: "number"},
			{Name: "brand", Type: "string", Optional: true},
			{Name: "part_number", Type: "string", Optional: true},
			{Name: "unit_price", Type: "number", Optional: true},
		},
	}
	var sb strings.Builder
	writeProperty(&sb, f, false)
	out := sb.String()

	for _, want := range []string{
		"type: array",
		"items:",
		"type: object",
		"name:",
		"quantity:",
		"part_number:",
		"unit_price:",
		"- name",
		"- quantity",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("nested array OpenAPI missing %q:\n%s", want, out)
		}
	}
	// Optional inner fields must NOT be in the nested required list.
	if strings.Contains(out, "- brand") {
		t.Errorf("optional inner field must not be required:\n%s", out)
	}
}

// TestWriteProperty_EnumString locks enum rendering (regression guard for the
// nested refactor): a union field stays `type: string` with an `enum` list.
func TestWriteProperty_EnumString(t *testing.T) {
	f := FieldInfo{Name: "transfer_type", Type: "union", Literals: []string{"gift", "private_sale"}}
	var sb strings.Builder
	writeProperty(&sb, f, false)
	out := sb.String()
	if !strings.Contains(out, "type: string") || !strings.Contains(out, `enum: ["gift", "private_sale"]`) {
		t.Errorf("enum field must render type:string + enum list:\n%s", out)
	}
}
