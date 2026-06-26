package main

import (
	"strings"
	"testing"
)

// --- Feature 2: id-reference input field ({field, wire, ref}) ---

// TestExposeFieldUnmarshalRef locks parsing of the ref object form: it carries
// field/wire/ref through UnmarshalJSON.
func TestExposeFieldUnmarshalRef(t *testing.T) {
	var e ExposeField
	if err := e.UnmarshalJSON([]byte(`{"field":"vehicleId","wire":"vehicle_id","ref":"vehicles"}`)); err != nil {
		t.Fatalf("unmarshal ref expose: %v", err)
	}
	if e.Field != "vehicleId" || e.Wire != "vehicle_id" || e.Ref != "vehicles" {
		t.Errorf("ref expose parsed wrong: %+v", e)
	}
}

// TestResolveIDRefField locks resolution of an id-reference field: the validator
// token becomes v.id("<ref>") and the scalar type override becomes Id<"<ref>">.
func TestResolveIDRefField(t *testing.T) {
	r := mustResolve(t, albumsTable(), albumsViaParentSpec())
	var fk *ResolvedField
	for i := range r.InputFields {
		if r.InputFields[i].Column == "vehicleId" {
			fk = &r.InputFields[i]
		}
	}
	if fk == nil {
		t.Fatal("vehicleId id-ref field not resolved as an input field")
	}
	if fk.TSType != `v.id("vehicles")` {
		t.Errorf("id-ref TSType = %q, want v.id(\"vehicles\")", fk.TSType)
	}
	if fk.TSScalar != `Id<"vehicles">` {
		t.Errorf("id-ref TSScalar = %q, want Id<\"vehicles\">", fk.TSScalar)
	}
}

// TestEmitApiTSIDRefRendersVId is the core emitter lock for feature 2: the
// id-reference field renders v.id("vehicles") (NOT v.string()) in the input +
// patch validators and Id<"vehicles"> (NOT string) in the wire type.
func TestEmitApiTSIDRefRendersVId(t *testing.T) {
	r := mustResolve(t, albumsTable(), albumsViaParentSpec())
	src := EmitApiTS(r)

	inBody := between(src, "export const albumApiInput = {", "};")
	if !strings.Contains(inBody, `vehicle_id: v.id("vehicles"),`) {
		t.Errorf("id-ref input must render v.id(\"vehicles\"):\n%s", inBody)
	}
	if strings.Contains(inBody, "vehicle_id: v.string()") {
		t.Errorf("id-ref input must NOT render v.string():\n%s", inBody)
	}

	// An id-reference FK is create-only: it must NOT appear in the PATCH surface
	// (a child cannot be reparented through an update), while other writable
	// fields still do.
	patchBody := between(src, "export const albumApiPatch = {", "};")
	if strings.Contains(patchBody, "vehicle_id") {
		t.Errorf("id-ref FK must NOT appear in the patch validator:\n%s", patchBody)
	}
	if !strings.Contains(patchBody, "name: v.optional(v.string()),") {
		t.Errorf("non-FK writable field must still appear in the patch validator:\n%s", patchBody)
	}

	// The derived *ApiPatch type omits the immutable FK wire.
	patchType := EmitApiTypesTS(r)
	if !strings.Contains(patchType, `export type AlbumApiPatch = Partial<Omit<AlbumApiInput, "vehicle_id">>;`) {
		t.Errorf("id-ref FK must be Omit-ed from the *ApiPatch type:\n%s", patchType)
	}

	outBody := between(src, "export const albumApiOutput = v.object({", "});")
	if !strings.Contains(outBody, `vehicle_id: v.id("vehicles"),`) {
		t.Errorf("id-ref output must render v.id(\"vehicles\"):\n%s", outBody)
	}

	// wire type file uses the Id<"…"> scalar and imports Id
	types := EmitApiTypesTS(r)
	if !strings.Contains(types, `vehicle_id: Id<"vehicles">;`) {
		t.Errorf("id-ref wire type must be Id<\"vehicles\">:\n%s", types)
	}
	if !strings.Contains(types, `import type { Id } from "../../_generated/dataModel";`) {
		t.Errorf("id-ref wire-types file must import Id:\n%s", types)
	}
}

// TestEmitApiTypesTSNoIdImportWithoutRef is the complementary branch: a resource
// with no id-reference input field must NOT import Id into the wire-types file.
func TestEmitApiTypesTSNoIdImportWithoutRef(t *testing.T) {
	types := EmitApiTypesTS(mustResolve(t, vehiclesTable(), vehiclesSpec()))
	if strings.Contains(types, "import type { Id }") {
		t.Errorf("resource without id-ref input must NOT import Id:\n%s", types)
	}
}

// --- Feature 3: readonly output-only column ({field, readonly: true}) ---

// TestExposeFieldUnmarshalReadOnly locks parsing of the readonly flag.
func TestExposeFieldUnmarshalReadOnly(t *testing.T) {
	var e ExposeField
	if err := e.UnmarshalJSON([]byte(`{"field":"albumType","wire":"album_type","readonly":true}`)); err != nil {
		t.Fatalf("unmarshal readonly expose: %v", err)
	}
	if !e.ReadOnly {
		t.Errorf("readonly expose must set ReadOnly=true: %+v", e)
	}
}

// readonlySpec adds a server-set album_type column exposed read-only.
func readonlySpec() (TableInfo, ResourceSpec) {
	tbl := albumsTable()
	tbl.Fields = append(tbl.Fields, FieldInfo{Name: "albumType", Type: "string"})
	spec := albumsViaParentSpec()
	spec.Expose = append(spec.Expose, ExposeField{Field: "albumType", Wire: "album_type", ReadOnly: true})
	return tbl, spec
}

// TestResolveReadOnlyExcludedFromInput locks feature 3 at the resolver level: a
// readonly field lands in OutputFields but NOT in InputFields.
func TestResolveReadOnlyExcludedFromInput(t *testing.T) {
	tbl, spec := readonlySpec()
	r := mustResolve(t, tbl, spec)

	inInput := false
	for _, f := range r.InputFields {
		if f.Column == "albumType" {
			inInput = true
		}
	}
	if inInput {
		t.Error("readonly field must be excluded from InputFields")
	}

	inOutput := false
	for _, f := range r.OutputFields {
		if f.Column == "albumType" {
			inOutput = true
			if !f.ReadOnly {
				t.Error("resolved readonly output field must carry ReadOnly=true")
			}
		}
	}
	if !inOutput {
		t.Error("readonly field must appear in OutputFields")
	}
}

// TestEmitApiTSReadOnlyOutputOnly is the core emitter lock for feature 3: the
// readonly column appears in the output validator and toApi (doc.<col>) but is
// absent from the input + patch validators, the wire types, and the write helpers.
func TestEmitApiTSReadOnlyOutputOnly(t *testing.T) {
	tbl, spec := readonlySpec()
	r := mustResolve(t, tbl, spec)
	src := EmitApiTS(r)

	// present in output validator
	outBody := between(src, "export const albumApiOutput = v.object({", "});")
	if !strings.Contains(outBody, "album_type: v.string(),") {
		t.Errorf("readonly field must appear in output validator:\n%s", outBody)
	}
	// present in toApi as doc.<col>
	if !strings.Contains(src, "album_type: doc.albumType,") {
		t.Errorf("readonly field must be projected in toApi as doc.albumType:\n%s", src)
	}

	// absent from input validator
	inBody := between(src, "export const albumApiInput = {", "};")
	if strings.Contains(inBody, "album_type") {
		t.Errorf("readonly field must NOT appear in input validator:\n%s", inBody)
	}
	// absent from patch validator
	patchBody := between(src, "export const albumApiPatch = {", "};")
	if strings.Contains(patchBody, "album_type") {
		t.Errorf("readonly field must NOT appear in patch validator:\n%s", patchBody)
	}
	// absent from write helpers (toCreateArgs / toUpdatePatch)
	createBody := between(src, "export function toCreateArgs(", "}")
	if strings.Contains(createBody, "albumType") {
		t.Errorf("readonly field must NOT be written in toCreateArgs:\n%s", createBody)
	}
	patchFn := between(src, "export function toUpdatePatch(", "return patch;")
	if strings.Contains(patchFn, "albumType") || strings.Contains(patchFn, "album_type") {
		t.Errorf("readonly field must NOT be written in toUpdatePatch:\n%s", patchFn)
	}

	// absent from the wire input type
	types := EmitApiTypesTS(r)
	if strings.Contains(types, "album_type") {
		t.Errorf("readonly field must NOT appear in the wire input type:\n%s", types)
	}
}

// TestWriteOnlyField locks the writeOnly expose flag: the column is present in
// the input/patch validators and write helpers but excluded from the output
// validator and toApi projection (accept-but-never-return, e.g. private notes).
func TestWriteOnlyField(t *testing.T) {
	tbl := TableInfo{Name: "maintenanceRecords", Fields: []FieldInfo{
		{Name: "vehicleId", Type: "id", IsID: true, TableRef: "vehicles"},
		{Name: "serviceName", Type: "string"},
		{Name: "customerNotes", Type: "string", Optional: true},
	}}
	spec := ResourceSpec{
		Table: "maintenanceRecords", Path: "maintenance",
		Module:    "maintenance/maintenanceApi.ts",
		Ownership: Ownership{Via: "vehicle_id", Table: "vehicles", Field: "ownerId"},
		Verbs:     []string{"create", "read", "update", "delete"},
		Expose: []ExposeField{
			{Field: "vehicleId", Wire: "vehicle_id", Ref: "vehicles"},
			{Field: "serviceName", Wire: "service_name"},
			{Field: "customerNotes", Wire: "customer_notes", WriteOnly: true},
		},
	}
	src := EmitApiTS(mustResolve(t, tbl, spec))

	inputBlock := between(src, "ApiInput = {", "};")
	if !strings.Contains(inputBlock, "customer_notes:") {
		t.Errorf("write-only field must be in the input validator:\n%s", inputBlock)
	}
	outputBlock := between(src, "ApiOutput = v.object({", "});")
	if strings.Contains(outputBlock, "customer_notes:") {
		t.Errorf("write-only field must NOT be in the output validator:\n%s", outputBlock)
	}
	toApiBlock := between(src, "export function toApi(", "}")
	if strings.Contains(toApiBlock, "customer_notes:") {
		t.Errorf("write-only field must NOT be in the toApi projection:\n%s", toApiBlock)
	}
	createBlock := between(src, "export function toCreateArgs(", "}")
	if !strings.Contains(createBlock, "customerNotes: input.customer_notes") {
		t.Errorf("write-only field must still be in toCreateArgs:\n%s", createBlock)
	}
}

// TestReadOnlyAndWriteOnlyConflict locks the mutual-exclusivity guard.
func TestReadOnlyAndWriteOnlyConflict(t *testing.T) {
	tbl := TableInfo{Name: "maintenanceRecords", Fields: []FieldInfo{
		{Name: "serviceName", Type: "string"},
	}}
	spec := ResourceSpec{
		Table: "maintenanceRecords", Path: "maintenance",
		Module: "maintenance/maintenanceApi.ts",
		Verbs:  []string{"read"},
		Expose: []ExposeField{
			{Field: "serviceName", Wire: "service_name", ReadOnly: true, WriteOnly: true},
		},
	}
	if _, err := ResolveResource(tbl, spec); err == nil {
		t.Error("a field marked both readonly and writeOnly must error")
	}
}
