package main

import (
	"strings"
	"testing"
)

// ============================================================================
// Wave D — rich nested sub-fields, top-level id-array fields, fixed-key maps.
//
// Grounded in the page-level vehicle privacy shape:
//   privacy?: { type: <enum>, allowed_users: Id<"users">[],
//               allowed_communities: Id<"communities">[] }
// and tab_privacy?: { timeline?: <privacy>, build?: <privacy>, … } (fixed keys).
// ============================================================================

// privacyTypeValues is the curated public type enum (the 7 lowercase values only;
// deprecated UPPERCASE + selectedCommunities are dropped from the public surface).
var privacyTypeValues = []string{
	"public", "private", "friends", "inner_circle",
	"common_communities", "specific_communities", "specific_users",
}

// privacyNested is the rich single-object privacy shape: an enum `type`, two
// id-arrays, and two more id-arrays for exclusions — exactly the page-level
// extendedPrivacySettingsValidator minus the deprecated bits.
func privacyNested() []NestedField {
	return []NestedField{
		{Key: "type", Enum: privacyTypeValues},
		{Key: "allowed_users", RefArray: "users", Optional: true},
		{Key: "allowed_communities", RefArray: "communities", Optional: true},
		{Key: "blocked_users", RefArray: "users", Optional: true},
		{Key: "blocked_communities", RefArray: "communities", Optional: true},
	}
}

// privacyVehiclesTable adds the page-level privacy object column + a tabPrivacy
// fixed-key map column + a top-level id-array column to the vehicles fixture.
func privacyVehiclesTable() TableInfo {
	t := vehiclesTable()
	t.Fields = append(t.Fields,
		FieldInfo{Name: "privacy", Type: "object", Optional: true},
		FieldInfo{Name: "tabPrivacy", Type: "object", Optional: true},
		FieldInfo{Name: "favoriteCommunities", Type: "array", IsArray: true, ArrayType: "id", TableRef: "communities", Optional: true},
	)
	return t
}

// tabKeys is the fixed set of 11 known tab keys.
var tabKeys = []string{
	"timeline", "build", "media", "specs", "ownership", "safety",
	"analytics", "financials", "glovebox", "maintenance", "badges",
}

// privacyVehiclesSpec exposes the three Wave-D shapes on the vehicles resource.
func privacyVehiclesSpec() ResourceSpec {
	s := vehiclesSpec()
	s.Expose = append(s.Expose,
		// (1) Single nested object with enum + id-array sub-fields.
		ExposeField{Field: "privacy", Wire: "privacy", Nested: privacyNested(), NestedSingle: true},
		// (2) Fixed-key object map: each tab → the privacy object shape.
		ExposeField{Field: "tabPrivacy", Wire: "tab_privacy", ObjectMap: &ObjectMapShape{
			Keys: tabKeys, Object: privacyNested(),
		}},
		// (3) Top-level id-array field.
		ExposeField{Field: "favoriteCommunities", Wire: "favorite_communities", RefArray: "communities"},
	)
	return s
}

// ----------------------------------------------------------------------------
// Feature 1: nested sub-field grammar (enum / id / id[]) parsing
// ----------------------------------------------------------------------------

// TestParseNestedShapeRichTypes locks the extended per-key type grammar:
// "id:users", "id[]:communities", and "enum:a|b" (with optional "?").
func TestParseNestedShapeRichTypes(t *testing.T) {
	var e ExposeField
	js := `{"field":"privacy","wire":"privacy","object":{` +
		`"type":"enum:public|private|friends",` +
		`"allowed_users":"id[]:users?",` +
		`"owner":"id:users",` +
		`"note":"string?"}}`
	if err := e.UnmarshalJSON([]byte(js)); err != nil {
		t.Fatalf("unmarshal rich nested: %v", err)
	}
	if !e.NestedSingle {
		t.Error("object form must set NestedSingle")
	}
	if len(e.Nested) != 4 {
		t.Fatalf("nested fields = %d, want 4: %+v", len(e.Nested), e.Nested)
	}
	// type → enum
	if got := e.Nested[0]; got.Key != "type" || len(got.Enum) != 3 || got.Enum[0] != "public" {
		t.Errorf("type sub-field not parsed as enum: %+v", got)
	}
	// allowed_users → optional id-array
	if got := e.Nested[1]; got.Key != "allowed_users" || got.RefArray != "users" || !got.Optional {
		t.Errorf("allowed_users not parsed as optional id-array: %+v", got)
	}
	// owner → id-ref (required)
	if got := e.Nested[2]; got.Key != "owner" || got.Ref != "users" || got.Optional {
		t.Errorf("owner not parsed as required id-ref: %+v", got)
	}
	// note → plain optional string
	if got := e.Nested[3]; got.Key != "note" || got.Type != "string" || !got.Optional {
		t.Errorf("note not parsed as optional string: %+v", got)
	}
}

// TestParseNestedShapeRejectsEmptyEnum rejects an enum declaration with no values.
func TestParseNestedShapeRejectsEmptyEnum(t *testing.T) {
	var e ExposeField
	if err := e.UnmarshalJSON([]byte(`{"field":"x","wire":"x","object":{"k":"enum:"}}`)); err == nil {
		t.Fatal("expected error for empty enum, got nil")
	}
}

// TestParseNestedShapeRejectsEmptyIdTable rejects an id ref with no table name.
func TestParseNestedShapeRejectsEmptyIdTable(t *testing.T) {
	var e ExposeField
	if err := e.UnmarshalJSON([]byte(`{"field":"x","wire":"x","object":{"k":"id:"}}`)); err == nil {
		t.Fatal("expected error for empty id table, got nil")
	}
}

// ----------------------------------------------------------------------------
// Feature 1: single nested object validators + TS type with rich sub-fields
// ----------------------------------------------------------------------------

// wantPrivacyObjValidator is the inner v.object({...}) for the privacy shape.
const wantPrivacyObjValidator = `v.object({ ` +
	`type: v.union(v.literal("public"), v.literal("private"), v.literal("friends"), v.literal("inner_circle"), v.literal("common_communities"), v.literal("specific_communities"), v.literal("specific_users")), ` +
	`allowed_users: v.optional(v.array(v.id("users"))), ` +
	`allowed_communities: v.optional(v.array(v.id("communities"))), ` +
	`blocked_users: v.optional(v.array(v.id("users"))), ` +
	`blocked_communities: v.optional(v.array(v.id("communities"))) })`

// wantPrivacyObjScalar is the derived TS object type for the privacy shape.
const wantPrivacyObjScalar = `{ ` +
	`type: "public" | "private" | "friends" | "inner_circle" | "common_communities" | "specific_communities" | "specific_users"; ` +
	`allowed_users?: Id<"users">[]; ` +
	`allowed_communities?: Id<"communities">[]; ` +
	`blocked_users?: Id<"users">[]; ` +
	`blocked_communities?: Id<"communities">[] }`

func TestResolveNestedRichSubFields(t *testing.T) {
	r := mustResolve(t, privacyVehiclesTable(), privacyVehiclesSpec())
	var nf *ResolvedField
	for i := range r.InputFields {
		if r.InputFields[i].Column == "privacy" {
			nf = &r.InputFields[i]
		}
	}
	if nf == nil {
		t.Fatal("privacy nested object not resolved as input")
	}
	if nf.TSType != wantPrivacyObjValidator {
		t.Errorf("privacy TSType =\n  %q\nwant\n  %q", nf.TSType, wantPrivacyObjValidator)
	}
	if nf.TSScalar != wantPrivacyObjScalar {
		t.Errorf("privacy TSScalar =\n  %q\nwant\n  %q", nf.TSScalar, wantPrivacyObjScalar)
	}
}

// TestEmitApiTSNestedRichValidators locks the privacy object validator in all
// three surfaces (input/patch/output) and the snake↔camel round-trip in the
// write helpers + toApi for the rich sub-fields.
func TestEmitApiTSNestedRichValidators(t *testing.T) {
	r := mustResolve(t, privacyVehiclesTable(), privacyVehiclesSpec())
	src := EmitApiTS(r)

	inBody := between(src, "export const vehicleApiInput = {", "};")
	if !strings.Contains(inBody, "privacy: v.optional("+wantPrivacyObjValidator+"),") {
		t.Errorf("nested single input must render optional object validator:\n%s", inBody)
	}
	patchBody := between(src, "export const vehicleApiPatch = {", "};")
	if !strings.Contains(patchBody, "privacy: v.optional("+wantPrivacyObjValidator+"),") {
		t.Errorf("nested single patch must render object validator:\n%s", patchBody)
	}
	outBody := between(src, "export const vehicleApiOutput = v.object({", "});")
	// privacy is schema-optional with no default → optional on output too.
	if !strings.Contains(outBody, "privacy: v.optional("+wantPrivacyObjValidator+"),") {
		t.Errorf("nested single output must render optional object validator:\n%s", outBody)
	}
}

// TestEmitApiTSNestedRichRoundTrip locks the toApi / toCreateArgs / toUpdatePatch
// snake↔camel mapping for the rich sub-fields (id-arrays pass straight through;
// the wire keys are already snake → camel: allowed_users → allowedUsers).
func TestEmitApiTSNestedRichRoundTrip(t *testing.T) {
	r := mustResolve(t, privacyVehiclesTable(), privacyVehiclesSpec())
	src := EmitApiTS(r)

	toApi := between(src, "export function toApi(", "}\n\n")
	for _, frag := range []string{
		"privacy: doc.privacy",
		// The toApi (db→wire) read NARROWS the enum: the schema column may carry
		// deprecated values outside the curated 7, so the projection casts down to
		// the curated union to satisfy the narrower output validator.
		`type: doc.privacy.type as "public" | "private" | "friends" | "inner_circle" | "common_communities" | "specific_communities" | "specific_users",`,
		"allowed_users: doc.privacy.allowedUsers,",
		"allowed_communities: doc.privacy.allowedCommunities,",
		"blocked_users: doc.privacy.blockedUsers,",
	} {
		if !strings.Contains(toApi, frag) {
			t.Errorf("toApi rich nested projection missing %q:\n%s", frag, toApi)
		}
	}

	create := between(src, "export function toCreateArgs(", "}\n\n")
	for _, frag := range []string{
		"privacy: input.privacy",
		// The write (wire→db) direction does NOT cast: the curated input is already
		// the narrow type and widens into the column safely.
		"type: input.privacy.type,",
		"allowedUsers: input.privacy.allowed_users,",
		"allowedCommunities: input.privacy.allowed_communities,",
	} {
		if !strings.Contains(create, frag) {
			t.Errorf("toCreateArgs rich nested mapping missing %q:\n%s", frag, create)
		}
	}
	// Lock direction-sensitivity: the write path must NOT carry the read-narrowing cast.
	if strings.Contains(create, "input.privacy.type as ") {
		t.Errorf("toCreateArgs must NOT narrow-cast the write direction:\n%s", create)
	}

	patchFn := between(src, "export function toUpdatePatch(", "return patch;")
	for _, frag := range []string{
		"if (input.privacy !== undefined)",
		"allowedUsers: input.privacy.allowed_users,",
	} {
		if !strings.Contains(patchFn, frag) {
			t.Errorf("toUpdatePatch rich nested mapping missing %q:\n%s", frag, patchFn)
		}
	}
}

// TestEmitApiTypesTSNestedRich locks the derived wire type for the privacy object.
func TestEmitApiTypesTSNestedRich(t *testing.T) {
	types := EmitApiTypesTS(mustResolve(t, privacyVehiclesTable(), privacyVehiclesSpec()))
	if !strings.Contains(types, "privacy?: "+wantPrivacyObjScalar+";") {
		t.Errorf("nested rich wire type missing:\n  privacy?: %s;\nGOT:\n%s", wantPrivacyObjScalar, types)
	}
	// The id-array sub-fields require the Id import.
	if !strings.Contains(types, `import type { Id } from "../../_generated/dataModel";`) {
		t.Errorf("wire-types file must import Id for id-array sub-fields:\n%s", types)
	}
}

// ----------------------------------------------------------------------------
// Feature 3: top-level id-array field (refArray)
// ----------------------------------------------------------------------------

// TestParseExposeRefArray locks the {field, wire, refArray} parse.
func TestParseExposeRefArray(t *testing.T) {
	var e ExposeField
	if err := e.UnmarshalJSON([]byte(`{"field":"favoriteCommunities","wire":"favorite_communities","refArray":"communities"}`)); err != nil {
		t.Fatalf("unmarshal refArray: %v", err)
	}
	if e.RefArray != "communities" {
		t.Errorf("RefArray = %q, want communities", e.RefArray)
	}
}

// TestResolveRefArrayField locks the resolved validator/scalar for a top-level
// id-array field: v.array(v.id("X")) / Id<"X">[].
func TestResolveRefArrayField(t *testing.T) {
	r := mustResolve(t, privacyVehiclesTable(), privacyVehiclesSpec())
	var f *ResolvedField
	for i := range r.InputFields {
		if r.InputFields[i].Column == "favoriteCommunities" {
			f = &r.InputFields[i]
		}
	}
	if f == nil {
		t.Fatal("favoriteCommunities id-array not resolved as input")
	}
	if f.TSType != `v.array(v.id("communities"))` {
		t.Errorf("id-array TSType = %q, want v.array(v.id(\"communities\"))", f.TSType)
	}
	if f.TSScalar != `Id<"communities">[]` {
		t.Errorf("id-array TSScalar = %q, want Id<\"communities\">[]", f.TSScalar)
	}
}

// TestEmitRefArrayField locks the id-array field across the API + types + routes.
func TestEmitRefArrayField(t *testing.T) {
	r := mustResolve(t, privacyVehiclesTable(), privacyVehiclesSpec())
	src := EmitApiTS(r)
	inBody := between(src, "export const vehicleApiInput = {", "};")
	// favoriteCommunities is schema-optional → optional on the wire.
	if !strings.Contains(inBody, `favorite_communities: v.optional(v.array(v.id("communities"))),`) {
		t.Errorf("id-array input validator missing:\n%s", inBody)
	}

	types := EmitApiTypesTS(r)
	if !strings.Contains(types, `favorite_communities?: Id<"communities">[];`) {
		t.Errorf("id-array wire type missing:\n%s", types)
	}

	// readInput/readPatch must guard an id-array with Array.isArray, NOT typeof.
	routes := EmitRoutesTS(r)
	readPatch := between(routes, "export function readPatch(", "return patch;")
	if !strings.Contains(readPatch, "if (Array.isArray(b.favorite_communities))") {
		t.Errorf("readPatch must guard id-array with Array.isArray:\n%s", readPatch)
	}
	if strings.Contains(readPatch, `typeof b.favorite_communities === "string"`) {
		t.Errorf("readPatch must NOT typeof-string-check an id-array:\n%s", readPatch)
	}
}

// ----------------------------------------------------------------------------
// Feature 2: fixed-key object map (tabPrivacy)
// ----------------------------------------------------------------------------

// TestParseExposeObjectMap locks the {field, wire, objectMap:{keys, object}} parse.
func TestParseExposeObjectMap(t *testing.T) {
	var e ExposeField
	js := `{"field":"tabPrivacy","wire":"tab_privacy","objectMap":{` +
		`"keys":["timeline","build"],` +
		`"object":{"type":"enum:public|private","allowed_users":"id[]:users?"}}}`
	if err := e.UnmarshalJSON([]byte(js)); err != nil {
		t.Fatalf("unmarshal objectMap: %v", err)
	}
	if e.ObjectMap == nil {
		t.Fatal("ObjectMap must be set")
	}
	if len(e.ObjectMap.Keys) != 2 || e.ObjectMap.Keys[0] != "timeline" {
		t.Errorf("ObjectMap.Keys = %+v, want [timeline build]", e.ObjectMap.Keys)
	}
	if len(e.ObjectMap.Object) != 2 || e.ObjectMap.Object[0].Key != "type" {
		t.Errorf("ObjectMap.Object not parsed: %+v", e.ObjectMap.Object)
	}
}

// TestParseObjectMapRejectsNoKeys rejects a map with no fixed keys.
func TestParseObjectMapRejectsNoKeys(t *testing.T) {
	var e ExposeField
	js := `{"field":"x","wire":"x","objectMap":{"keys":[],"object":{"k":"string"}}}`
	if err := e.UnmarshalJSON([]byte(js)); err == nil {
		t.Fatal("expected error for objectMap with no keys, got nil")
	}
}

// TestResolveObjectMapField locks the resolved validator/scalar for the fixed-key
// map: v.object({ timeline: v.optional(<priv>), … }) / { timeline?: <priv>; … }.
func TestResolveObjectMapField(t *testing.T) {
	r := mustResolve(t, privacyVehiclesTable(), privacyVehiclesSpec())
	var f *ResolvedField
	for i := range r.InputFields {
		if r.InputFields[i].Column == "tabPrivacy" {
			f = &r.InputFields[i]
		}
	}
	if f == nil {
		t.Fatal("tabPrivacy map not resolved as input")
	}
	// Each tab value is the optional privacy object.
	wantStart := "v.object({ timeline: v.optional(" + wantPrivacyObjValidator + "),"
	if !strings.HasPrefix(f.TSType, wantStart) {
		t.Errorf("map TSType =\n  %q\nwant prefix\n  %q", f.TSType, wantStart)
	}
	// All 11 keys present.
	for _, k := range tabKeys {
		if !strings.Contains(f.TSType, k+": v.optional(") {
			t.Errorf("map TSType missing tab key %q:\n%s", k, f.TSType)
		}
	}
	wantScalarStart := "{ timeline?: " + wantPrivacyObjScalar + ";"
	if !strings.HasPrefix(f.TSScalar, wantScalarStart) {
		t.Errorf("map TSScalar =\n  %q\nwant prefix\n  %q", f.TSScalar, wantScalarStart)
	}
}

// TestEmitObjectMapField locks the map field in the API output + write helpers.
func TestEmitObjectMapField(t *testing.T) {
	r := mustResolve(t, privacyVehiclesTable(), privacyVehiclesSpec())
	src := EmitApiTS(r)

	outBody := between(src, "export const vehicleApiOutput = v.object({", "});")
	if !strings.Contains(outBody, "tab_privacy: v.optional(v.object({ timeline: v.optional(") {
		t.Errorf("map output validator missing:\n%s", outBody)
	}

	// The map value is one JSON object on the wire; the tab keys are identical
	// wire↔db, but each present tab's inner privacy object maps snake↔camel
	// (allowed_users ↔ allowedUsers). toApi reads from doc.tabPrivacy and remaps.
	toApi := between(src, "export function toApi(", "}\n\n")
	for _, frag := range []string{
		"tab_privacy: doc.tabPrivacy",
		"timeline: doc.tabPrivacy.timeline",
		"allowed_users: doc.tabPrivacy.timeline.allowedUsers,",
	} {
		if !strings.Contains(toApi, frag) {
			t.Errorf("toApi map projection missing %q:\n%s", frag, toApi)
		}
	}

	create := between(src, "export function toCreateArgs(", "}\n\n")
	for _, frag := range []string{
		"tabPrivacy: input.tab_privacy",
		"allowedUsers: input.tab_privacy.timeline.allowed_users,",
	} {
		if !strings.Contains(create, frag) {
			t.Errorf("toCreateArgs map mapping missing %q:\n%s", frag, create)
		}
	}

	patchFn := between(src, "export function toUpdatePatch(", "return patch;")
	for _, frag := range []string{
		"if (input.tab_privacy !== undefined)",
		"timeline: input.tab_privacy.timeline",
		"allowedUsers: input.tab_privacy.timeline.allowed_users,",
	} {
		if !strings.Contains(patchFn, frag) {
			t.Errorf("toUpdatePatch map mapping missing %q:\n%s", frag, patchFn)
		}
	}

	// readPatch guards the map with typeof object (not array, not string).
	routes := EmitRoutesTS(r)
	readPatch := between(routes, "export function readPatch(", "return patch;")
	if !strings.Contains(readPatch, `typeof b.tab_privacy === "object"`) {
		t.Errorf("readPatch must guard the map with typeof object:\n%s", readPatch)
	}
}

// ----------------------------------------------------------------------------
// Non-regression: existing resources are byte-stable
// ----------------------------------------------------------------------------

// ----------------------------------------------------------------------------
// OpenAPI round-trip: the emitted rich-nested validators parse back correctly.
//
// The OpenAPI generator reparses the emitted *Api.ts validators (classifyValidator
// → parseObjectBody), so this locks that a single nested object carrying an enum
// + id-array sub-field surfaces with the right OpenAPI shapes: enum sub-field →
// `type: string` + enum list; id-array sub-field → `type: array` + `items: string`
// (refs are opaque id strings on the public surface).
// ----------------------------------------------------------------------------

func TestWaveDOpenAPIRoundTripRichNested(t *testing.T) {
	r := mustResolve(t, privacyVehiclesTable(), privacyVehiclesSpec())
	// The emitted output validator carries the privacy object; reparse the privacy
	// field's validator through the OpenAPI parser and render it as a property.
	field := classifyValidator("privacy", "v.optional("+wantPrivacyObjValidator+")")
	if field.Type != "object" {
		t.Fatalf("privacy validator must classify as object, got %q", field.Type)
	}
	if !field.Optional {
		t.Error("privacy validator must classify as optional")
	}
	// Sub-fields: type → union(enum), allowed_users → array-of-id.
	byName := map[string]FieldInfo{}
	for _, f := range field.Nested {
		byName[f.Name] = f
	}
	if tf := byName["type"]; tf.Type != "union" || len(tf.Literals) != 7 || tf.Literals[0] != "public" {
		t.Errorf("type sub-field must reparse as a 7-value enum union: %+v", tf)
	}
	if af := byName["allowed_users"]; af.Type != "array" || af.ArrayType != "id" || af.TableRef != "users" {
		t.Errorf("allowed_users sub-field must reparse as array-of-id(users): %+v", af)
	}

	// Render the property and assert the OpenAPI surface is well-typed.
	var sb strings.Builder
	writeProperty(&sb, field, false)
	out := sb.String()
	for _, want := range []string{
		"type: object",
		"type:",
		`enum: ["public", "private", "friends", "inner_circle", "common_communities", "specific_communities", "specific_users"]`,
		"type: array",
		"items: { type: string }",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("rich-nested OpenAPI round-trip missing %q:\n%s", want, out)
		}
	}
	_ = EmitApiTS(r) // ensure the emitter still produces this resource without panicking
}

// TestWaveDNonRegressionMaintenance locks that the existing maintenance nested
// ARRAY resource (scalar sub-fields only) emits identically after the Wave-D
// changes — the rich sub-field support must be purely additive.
func TestWaveDNonRegressionMaintenance(t *testing.T) {
	r := mustResolve(t, maintenanceTable(), maintenanceSpec())
	src := EmitApiTS(r)
	obj := `v.object({ name: v.string(), quantity: v.number(), brand: v.optional(v.string()), part_number: v.optional(v.string()), unit_price: v.optional(v.number()) })`
	arr := "v.array(" + obj + ")"
	inBody := between(src, "export const maintenanceRecordApiInput = {", "};")
	if !strings.Contains(inBody, "parts_used: v.optional("+arr+"),") {
		t.Errorf("maintenance nested array regressed:\n%s", inBody)
	}
}

// TestWaveDNonRegressionVehicles locks that a vanilla vehicles resource (no
// Wave-D fields) emits its scalar input fields unchanged.
func TestWaveDNonRegressionVehicles(t *testing.T) {
	src := EmitApiTS(mustResolve(t, vehiclesTable(), vehiclesSpec()))
	for _, frag := range []string{
		"make: v.string(),",
		"for_sale: v.optional(v.boolean()),",
		"created_at: v.string(),",
	} {
		if !strings.Contains(src, frag) {
			t.Errorf("vanilla vehicles regressed, missing %q", frag)
		}
	}
	// No Wave-D artifacts leak into the vanilla resource.
	for _, banned := range []string{"tab_privacy", "v.array(v.id(", "privacy:"} {
		if strings.Contains(src, banned) {
			t.Errorf("vanilla vehicles must not contain Wave-D artifact %q", banned)
		}
	}
}
