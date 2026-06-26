package main

import (
	"reflect"
	"strings"
	"testing"
)

// --- Feature: typed array-of-nested-objects ({field, wire, nested:{…}}) ---

// maintenanceTable models the maintenanceRecords child resource carrying the
// partsUsed array-of-objects column, mirroring the hand-written
// maintenance/maintenanceApi.ts partApiValidator pattern.
func maintenanceTable() TableInfo {
	return TableInfo{Name: "maintenanceRecords", Fields: []FieldInfo{
		{Name: "vehicleId", Type: "id", IsID: true, TableRef: "vehicles"},
		{Name: "serviceName", Type: "string"},
		{Name: "partsUsed", Type: "array", IsArray: true, ArrayType: "object", Optional: true},
	}}
}

// maintenanceSpec exposes the FK, a scalar, and the typed-nested partsUsed array
// (wire parts_used) matching the spec's part shape.
func maintenanceSpec() ResourceSpec {
	return ResourceSpec{
		Table: "maintenanceRecords", Path: "maintenance",
		Module:    "maintenance/maintenanceApi.ts",
		Ownership: Ownership{Via: "vehicle_id", Table: "vehicles", Field: "ownerId"},
		Verbs:     []string{"create", "read", "update", "delete"},
		Expose: []ExposeField{
			{Field: "vehicleId", Wire: "vehicle_id", Ref: "vehicles"},
			{Field: "serviceName", Wire: "service_name"},
			{Field: "partsUsed", Wire: "parts_used", Nested: []NestedField{
				{Key: "name", Type: "string"},
				{Key: "quantity", Type: "number"},
				{Key: "brand", Type: "string", Optional: true},
				{Key: "part_number", Type: "string", Optional: true},
				{Key: "unit_price", Type: "number", Optional: true},
			}},
		},
	}
}

// TestExposeFieldUnmarshalNested locks parsing of the nested array form, with
// declaration order preserved and trailing "?" → Optional.
func TestExposeFieldUnmarshalNested(t *testing.T) {
	var e ExposeField
	js := `{"field":"partsUsed","wire":"parts_used","nested":{"name":"string","quantity":"number","brand":"string?","part_number":"string?","unit_price":"number?"}}`
	if err := e.UnmarshalJSON([]byte(js)); err != nil {
		t.Fatalf("unmarshal nested expose: %v", err)
	}
	if e.NestedSingle {
		t.Error("array nested form must NOT set NestedSingle")
	}
	want := []NestedField{
		{Key: "name", Type: "string"},
		{Key: "quantity", Type: "number"},
		{Key: "brand", Type: "string", Optional: true},
		{Key: "part_number", Type: "string", Optional: true},
		{Key: "unit_price", Type: "number", Optional: true},
	}
	if len(e.Nested) != len(want) {
		t.Fatalf("nested fields = %d, want %d: %+v", len(e.Nested), len(want), e.Nested)
	}
	for i, w := range want {
		// NestedField now carries an Enum []string (rich sub-fields), so it is no
		// longer comparable with ==; use reflect.DeepEqual.
		if !reflect.DeepEqual(e.Nested[i], w) {
			t.Errorf("nested[%d] = %+v, want %+v (order/optional must be preserved)", i, e.Nested[i], w)
		}
	}
}

// TestExposeFieldUnmarshalObjectSingle locks the single-object form.
func TestExposeFieldUnmarshalObjectSingle(t *testing.T) {
	var e ExposeField
	if err := e.UnmarshalJSON([]byte(`{"field":"geo","wire":"geo","object":{"lat":"number","lng":"number"}}`)); err != nil {
		t.Fatalf("unmarshal object expose: %v", err)
	}
	if !e.NestedSingle {
		t.Error("object form must set NestedSingle=true")
	}
	if len(e.Nested) != 2 {
		t.Fatalf("object nested fields = %d, want 2", len(e.Nested))
	}
}

// TestExposeFieldUnmarshalNestedBadType rejects an unsupported nested type.
func TestExposeFieldUnmarshalNestedBadType(t *testing.T) {
	var e ExposeField
	err := e.UnmarshalJSON([]byte(`{"field":"x","wire":"x","nested":{"k":"date"}}`))
	if err == nil {
		t.Fatal("expected error for unsupported nested type, got nil")
	}
}

// TestExposeFieldUnmarshalNestedAndObjectConflict rejects declaring both forms.
func TestExposeFieldUnmarshalNestedAndObjectConflict(t *testing.T) {
	var e ExposeField
	err := e.UnmarshalJSON([]byte(`{"field":"x","wire":"x","nested":{"k":"string"},"object":{"j":"string"}}`))
	if err == nil {
		t.Fatal("expected error when both nested and object are declared, got nil")
	}
}

// TestResolveNestedField locks resolution: the validator becomes
// v.array(v.object({…})) and the TS scalar becomes the typed-array type.
func TestResolveNestedField(t *testing.T) {
	r := mustResolve(t, maintenanceTable(), maintenanceSpec())
	var nf *ResolvedField
	for i := range r.InputFields {
		if r.InputFields[i].Column == "partsUsed" {
			nf = &r.InputFields[i]
		}
	}
	if nf == nil {
		t.Fatal("partsUsed nested field not resolved as input")
	}
	wantTS := `v.array(v.object({ name: v.string(), quantity: v.number(), brand: v.optional(v.string()), part_number: v.optional(v.string()), unit_price: v.optional(v.number()) }))`
	if nf.TSType != wantTS {
		t.Errorf("nested TSType =\n  %q\nwant\n  %q", nf.TSType, wantTS)
	}
	wantScalar := `{ name: string; quantity: number; brand?: string; part_number?: string; unit_price?: number }[]`
	if nf.TSScalar != wantScalar {
		t.Errorf("nested TSScalar =\n  %q\nwant\n  %q", nf.TSScalar, wantScalar)
	}
	if len(nf.Nested) != 5 {
		t.Errorf("resolved field must carry the 5 nested keys, got %d", len(nf.Nested))
	}
}

// TestEmitApiTSNestedValidators locks the nested validator in all three surfaces.
func TestEmitApiTSNestedValidators(t *testing.T) {
	r := mustResolve(t, maintenanceTable(), maintenanceSpec())
	src := EmitApiTS(r)
	obj := `v.object({ name: v.string(), quantity: v.number(), brand: v.optional(v.string()), part_number: v.optional(v.string()), unit_price: v.optional(v.number()) })`
	arr := "v.array(" + obj + ")"

	inBody := between(src, "export const maintenanceRecordApiInput = {", "};")
	// partsUsed is schema-optional → optional on the wire.
	if !strings.Contains(inBody, "parts_used: v.optional("+arr+"),") {
		t.Errorf("nested input must render optional array-of-object validator:\n%s", inBody)
	}
	patchBody := between(src, "export const maintenanceRecordApiPatch = {", "};")
	if !strings.Contains(patchBody, "parts_used: v.optional("+arr+"),") {
		t.Errorf("nested patch must render array-of-object validator:\n%s", patchBody)
	}
	outBody := between(src, "export const maintenanceRecordApiOutput = v.object({", "});")
	if !strings.Contains(outBody, "parts_used: "+arr+",") {
		t.Errorf("nested output must render array-of-object validator:\n%s", outBody)
	}
}

// requiredNestedTable models the real maintenanceRecords case where the
// partsUsed array column is REQUIRED in the schema (not v.optional). The emitter
// must still treat a nested array as OPTIONAL on the input/patch wire surface
// (it coalesces `?? []`, so omission is a no-op) while keeping it REQUIRED in
// the output (always projected as []). This locks the Wave-B parts_used
// input-optionality regression fix.
func requiredNestedTable() TableInfo {
	return TableInfo{Name: "maintenanceRecords", Fields: []FieldInfo{
		{Name: "vehicleId", Type: "id", IsID: true, TableRef: "vehicles"},
		{Name: "serviceName", Type: "string"},
		// Optional:false → REQUIRED nested array column in the schema.
		{Name: "partsUsed", Type: "array", IsArray: true, ArrayType: "object", Optional: false},
	}}
}

// TestNestedArrayOptionalOnInputWhenSchemaRequired locks that a schema-REQUIRED
// nested array is optional on input/patch but required (→ []) on output.
func TestNestedArrayOptionalOnInputWhenSchemaRequired(t *testing.T) {
	r := mustResolve(t, requiredNestedTable(), maintenanceSpec())

	var in *ResolvedField
	for i := range r.InputFields {
		if r.InputFields[i].Column == "partsUsed" {
			in = &r.InputFields[i]
		}
	}
	if in == nil {
		t.Fatal("partsUsed must be present as an input field")
	}
	if !in.Optional {
		t.Error("nested array must be Optional on input even when schema-required")
	}

	var outF *ResolvedField
	for i := range r.OutputFields {
		if r.OutputFields[i].Column == "partsUsed" {
			outF = &r.OutputFields[i]
		}
	}
	if outF == nil {
		t.Fatal("partsUsed must be present as an output field")
	}
	if !outF.OutputRequired {
		t.Error("nested array must stay OutputRequired (projects to []) on output")
	}

	src := EmitApiTS(r)
	obj := `v.object({ name: v.string(), quantity: v.number(), brand: v.optional(v.string()), part_number: v.optional(v.string()), unit_price: v.optional(v.number()) })`
	arr := "v.array(" + obj + ")"

	inBody := between(src, "export const maintenanceRecordApiInput = {", "};")
	if !strings.Contains(inBody, "parts_used: v.optional("+arr+"),") {
		t.Errorf("schema-required nested array must still be optional on input:\n%s", inBody)
	}
	outBody := between(src, "export const maintenanceRecordApiOutput = v.object({", "});")
	if !strings.Contains(outBody, "parts_used: "+arr+",") {
		t.Errorf("schema-required nested array must be required (non-optional) on output:\n%s", outBody)
	}

	types := EmitApiTypesTS(r)
	if !strings.Contains(types, `parts_used?: { name: string;`) {
		t.Errorf("nested array input type must carry `?` (optional):\n%s", types)
	}
}

// TestEmitApiTSNestedToApi locks the toApi projection: each row is mapped key by
// key with snake→camel column renames (part_number → partNumber).
func TestEmitApiTSNestedToApi(t *testing.T) {
	r := mustResolve(t, maintenanceTable(), maintenanceSpec())
	src := EmitApiTS(r)
	toApi := between(src, "export function toApi(", "}\n\n")
	for _, frag := range []string{
		"parts_used: (doc.partsUsed ?? []).map((item) => ({",
		"name: item.name,",
		"quantity: item.quantity,",
		"brand: item.brand,",
		"part_number: item.partNumber,",
		"unit_price: item.unitPrice,",
	} {
		if !strings.Contains(toApi, frag) {
			t.Errorf("toApi nested projection missing %q:\n%s", frag, toApi)
		}
	}
}

// TestEmitApiTSNestedWriteHelpers locks the create + patch write path: wire keys
// map to DB camelCase columns in both directions.
func TestEmitApiTSNestedWriteHelpers(t *testing.T) {
	r := mustResolve(t, maintenanceTable(), maintenanceSpec())
	src := EmitApiTS(r)

	createBody := between(src, "export function toCreateArgs(", "return {")
	create := between(src, "export function toCreateArgs(", "}\n\n")
	_ = createBody
	for _, frag := range []string{
		"partsUsed: (input.parts_used ?? []).map((item) => ({",
		"name: item.name,",
		"partNumber: item.part_number,",
		"unitPrice: item.unit_price,",
	} {
		if !strings.Contains(create, frag) {
			t.Errorf("toCreateArgs nested mapping missing %q:\n%s", frag, create)
		}
	}

	patchFn := between(src, "export function toUpdatePatch(", "return patch;")
	for _, frag := range []string{
		"if (input.parts_used !== undefined)",
		"patch.partsUsed = input.parts_used.map((item) => ({",
		"partNumber: item.part_number,",
	} {
		if !strings.Contains(patchFn, frag) {
			t.Errorf("toUpdatePatch nested mapping missing %q:\n%s", frag, patchFn)
		}
	}
}

// TestEmitRoutesTSNestedReadPatch locks that readPatch treats the nested array as
// a single field (Array.isArray guard), not a typeof-string check.
func TestEmitRoutesTSNestedReadPatch(t *testing.T) {
	src := EmitRoutesTS(mustResolve(t, maintenanceTable(), maintenanceSpec()))
	readPatch := between(src, "export function readPatch(", "return patch;")
	if !strings.Contains(readPatch, "if (Array.isArray(b.parts_used))") {
		t.Errorf("readPatch must guard the nested array with Array.isArray:\n%s", readPatch)
	}
	if !strings.Contains(readPatch, "patch.parts_used = b.parts_used as MaintenanceRecordApiInput[\"parts_used\"];") {
		t.Errorf("readPatch must assign the nested array with a typed cast:\n%s", readPatch)
	}
	if strings.Contains(readPatch, `typeof b.parts_used === "string"`) {
		t.Errorf("readPatch must NOT typeof-string-check the nested array:\n%s", readPatch)
	}
}

// TestEmitApiTypesTSNested locks the derived wire type for the nested array.
func TestEmitApiTypesTSNested(t *testing.T) {
	types := EmitApiTypesTS(mustResolve(t, maintenanceTable(), maintenanceSpec()))
	want := `parts_used?: { name: string; quantity: number; brand?: string; part_number?: string; unit_price?: number }[];`
	if !strings.Contains(types, want) {
		t.Errorf("nested wire type missing:\n  %s\nGOT:\n%s", want, types)
	}
}
