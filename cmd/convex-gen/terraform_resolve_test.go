package main

import (
	"strings"
	"testing"
)

func vehiclesTable() TableInfo {
	return TableInfo{Name: "vehicles", Fields: []FieldInfo{
		{Name: "make", Type: "string"},
		{Name: "model", Type: "string"},
		{Name: "year", Type: "number", Optional: true},
		{Name: "forSale", Type: "boolean", Optional: true},
		{Name: "sold", Type: "boolean", Optional: true},
		{Name: "ownerId", Type: "id", IsID: true, TableRef: "users"},
		{Name: "vehicleGalleryKeys", Type: "array", IsArray: true, ArrayType: "string", Optional: true},
	}}
}

func vehiclesSpec() ResourceSpec {
	return ResourceSpec{
		Table: "vehicles", Path: "vehicles", Module: "vehicles/vehiclesApi.ts",
		Ownership: Ownership{Field: "ownerId"}, Verbs: []string{"create", "read", "update", "delete"},
		Expose: []ExposeField{
			{Field: "make", Wire: "make"},
			{Field: "model", Wire: "model"},
			{Field: "year", Wire: "year"},
			{Field: "forSale", Wire: "for_sale"},
		},
		Defaults: map[string]any{"year": 0.0},
		Computed: map[string]ComputedField{
			"created_at": {As: "iso8601", From: "_creationTime"},
			"photo_urls": {As: "r2Urls", From: "vehicleGalleryKeys"},
		},
	}
}

func mustResolve(t *testing.T, tbl TableInfo, spec ResourceSpec) ResolvedResource {
	t.Helper()
	r, err := ResolveResource(tbl, spec)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestResolveResource(t *testing.T) {
	r, err := ResolveResource(vehiclesTable(), vehiclesSpec())
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if len(r.InputFields) != 4 {
		t.Fatalf("input fields = %d, want 4 (ownerId/gallery hidden)", len(r.InputFields))
	}
	// for_sale wire rename
	var forSale *ResolvedField
	for i := range r.InputFields {
		if r.InputFields[i].Column == "forSale" {
			forSale = &r.InputFields[i]
		}
	}
	if forSale == nil || forSale.Wire != "for_sale" || forSale.TSType != "v.boolean()" {
		t.Fatalf("forSale resolved wrong: %+v", forSale)
	}
	// computed are output-only
	if len(r.Computed) != 2 {
		t.Fatalf("computed = %d, want 2", len(r.Computed))
	}
	// output includes id + created_at + photo_urls + the 4 inputs
	if len(r.OutputFields) != len(r.InputFields) {
		t.Fatalf("output base fields should mirror inputs (computed tracked separately)")
	}
}

// TestResolveHatchRequiresOut asserts a hatch computed field with no output
// validator token is a resolve-time error (no untyped v.any() may leak into the
// public output validator).
func TestResolveHatchRequiresOut(t *testing.T) {
	spec := vehiclesSpec()
	spec.Computed["custom"] = ComputedField{Hatch: true}
	_, err := ResolveResource(vehiclesTable(), spec)
	if err == nil {
		t.Fatal("expected error for hatch computed field without 'out', got nil")
	}
	if !strings.Contains(err.Error(), "out") {
		t.Errorf("error = %v, want it to mention the missing 'out' token", err)
	}
}

// TestResolveHatchWithOut asserts a hatch computed field carries its output
// validator token through resolution.
func TestResolveHatchWithOut(t *testing.T) {
	spec := vehiclesSpec()
	spec.Computed["custom"] = ComputedField{Hatch: true, Out: "v.array(v.string())"}
	r := mustResolve(t, vehiclesTable(), spec)
	var found *ResolvedComputed
	for i := range r.Computed {
		if r.Computed[i].Wire == "custom" {
			found = &r.Computed[i]
		}
	}
	if found == nil {
		t.Fatal("hatch computed field 'custom' not resolved")
	}
	if found.Out != "v.array(v.string())" {
		t.Errorf("resolved Out = %q, want v.array(v.string())", found.Out)
	}
}
