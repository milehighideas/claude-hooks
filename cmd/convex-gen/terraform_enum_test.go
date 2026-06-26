package main

import (
	"strings"
	"testing"
)

// --- Feature: enum/union expose ({field, wire, enum: ["a","b"]}) ---

// ownershipHistoryTable models the heritage ownership-chain child resource whose
// transfer_type column is a string-literal union, mirroring the live
// schema/heritage.ts transferType validator.
func ownershipHistoryTable() TableInfo {
	return TableInfo{Name: "vehicleOwnershipHistory", Fields: []FieldInfo{
		{Name: "vehicleId", Type: "id", IsID: true, TableRef: "vehicles"},
		{Name: "transferType", Type: "union", Literals: []string{
			"private_sale", "dealer_sale", "gift",
		}},
		{Name: "notes", Type: "string", Optional: true},
	}}
}

// ownershipHistorySpec exposes the FK + the enum transfer_type column.
func ownershipHistorySpec() ResourceSpec {
	return ResourceSpec{
		Table: "vehicleOwnershipHistory", Path: "ownership_history",
		Module:    "vehicles/ownershipHistoryApi.ts",
		Ownership: Ownership{Via: "vehicle_id", Table: "vehicles", Field: "ownerId"},
		Verbs:     []string{"create", "read", "update", "delete"},
		Expose: []ExposeField{
			{Field: "vehicleId", Wire: "vehicle_id", Ref: "vehicles"},
			{Field: "transferType", Wire: "transfer_type", Enum: []string{
				"private_sale", "dealer_sale", "gift",
			}},
			{Field: "notes", Wire: "notes"},
		},
	}
}

// TestExposeFieldUnmarshalEnum locks parsing of the enum object form.
func TestExposeFieldUnmarshalEnum(t *testing.T) {
	var e ExposeField
	if err := e.UnmarshalJSON([]byte(`{"field":"transferType","wire":"transfer_type","enum":["a","b","c"]}`)); err != nil {
		t.Fatalf("unmarshal enum expose: %v", err)
	}
	if e.Field != "transferType" || e.Wire != "transfer_type" {
		t.Errorf("enum expose parsed wrong: %+v", e)
	}
	if len(e.Enum) != 3 || e.Enum[0] != "a" || e.Enum[2] != "c" {
		t.Errorf("enum literals parsed wrong: %+v", e.Enum)
	}
}

// TestResolveEnumField locks the resolver: an enum field becomes a literal-union
// validator token and a TS string-union scalar type.
func TestResolveEnumField(t *testing.T) {
	r := mustResolve(t, ownershipHistoryTable(), ownershipHistorySpec())
	var ef *ResolvedField
	for i := range r.InputFields {
		if r.InputFields[i].Column == "transferType" {
			ef = &r.InputFields[i]
		}
	}
	if ef == nil {
		t.Fatal("transferType enum field not resolved as an input field")
	}
	want := `v.union(v.literal("private_sale"), v.literal("dealer_sale"), v.literal("gift"))`
	if ef.TSType != want {
		t.Errorf("enum TSType = %q, want %q", ef.TSType, want)
	}
	wantScalar := `"private_sale" | "dealer_sale" | "gift"`
	if ef.TSScalar != wantScalar {
		t.Errorf("enum TSScalar = %q, want %q", ef.TSScalar, wantScalar)
	}
}

// TestEmitApiTSEnumUnion is the core emitter lock: the enum field renders a
// v.union(v.literal(...)) in the input + patch + output validators (NOT widened
// to v.string()), and the wire type is the string union.
func TestEmitApiTSEnumUnion(t *testing.T) {
	r := mustResolve(t, ownershipHistoryTable(), ownershipHistorySpec())
	src := EmitApiTS(r)
	union := `v.union(v.literal("private_sale"), v.literal("dealer_sale"), v.literal("gift"))`

	inBody := between(src, "export const vehicleOwnershipHistoryApiInput = {", "};")
	if !strings.Contains(inBody, "transfer_type: "+union+",") {
		t.Errorf("enum input must render the literal union:\n%s", inBody)
	}
	if strings.Contains(inBody, "transfer_type: v.string()") {
		t.Errorf("enum input must NOT widen to v.string():\n%s", inBody)
	}

	patchBody := between(src, "export const vehicleOwnershipHistoryApiPatch = {", "};")
	if !strings.Contains(patchBody, "transfer_type: v.optional("+union+"),") {
		t.Errorf("enum patch must render the optional literal union:\n%s", patchBody)
	}

	outBody := between(src, "export const vehicleOwnershipHistoryApiOutput = v.object({", "});")
	if !strings.Contains(outBody, "transfer_type: "+union+",") {
		t.Errorf("enum output must render the literal union:\n%s", outBody)
	}
	if strings.Contains(outBody, "transfer_type: v.string()") {
		t.Errorf("enum output must NOT widen to v.string():\n%s", outBody)
	}

	types := EmitApiTypesTS(r)
	if !strings.Contains(types, `transfer_type: "private_sale" | "dealer_sale" | "gift";`) {
		t.Errorf("enum wire type must be the string union:\n%s", types)
	}
}

// TestEmitApiTypesTSEnumNoIdImport guards against a regression where the enum
// TSScalar (a non-empty string) wrongly triggers the Id import. The resource has
// an id-ref FK so Id IS imported here — assert it's imported exactly because of
// the FK, not the enum: a resource with ONLY an enum (no FK) must not import Id.
func TestEmitApiTypesTSEnumNoIdImport(t *testing.T) {
	tbl := TableInfo{Name: "widgets", Fields: []FieldInfo{
		{Name: "kind", Type: "union", Literals: []string{"a", "b"}},
	}}
	spec := ResourceSpec{
		Table: "widgets", Path: "widgets", Module: "widgets/widgetsApi.ts",
		Verbs: []string{"create", "read"},
		Expose: []ExposeField{
			{Field: "kind", Wire: "kind", Enum: []string{"a", "b"}},
		},
	}
	types := EmitApiTypesTS(mustResolve(t, tbl, spec))
	if strings.Contains(types, "import type { Id }") {
		t.Errorf("enum-only resource must NOT import Id:\n%s", types)
	}
	if !strings.Contains(types, `kind: "a" | "b";`) {
		t.Errorf("enum-only wire type must be the string union:\n%s", types)
	}
}

// TestOpenAPIEnumSurfacesAsEnum confirms the generated output validator parses
// back into a FieldInfo union with literals, so the OpenAPI layer emits a proper
// `enum` rather than a bare string.
func TestOpenAPIEnumSurfacesAsEnum(t *testing.T) {
	r := mustResolve(t, ownershipHistoryTable(), ownershipHistorySpec())
	src := EmitApiTS(r)
	outBody := between(src, "export const vehicleOwnershipHistoryApiOutput = v.object({", "});")
	fields := parseTableFields(outBody)
	var got *FieldInfo
	for i := range fields {
		if fields[i].Name == "transfer_type" {
			got = &fields[i]
		}
	}
	if got == nil {
		t.Fatal("transfer_type missing from parsed output validator")
	}
	if got.Type != "union" {
		t.Errorf("parsed transfer_type type = %q, want union", got.Type)
	}
	if strings.Join(got.Literals, ",") != "private_sale,dealer_sale,gift" {
		t.Errorf("parsed transfer_type literals = %v, want the 3 transfer types", got.Literals)
	}
}

// TestImmutableOverrideKeepsEditableRefsInPatch locks the {"immutable": false}
// expose override: the parent FK (vehicle_id) stays create-only (excluded from
// PATCH), while explicitly-mutable id-references remain in the PATCH surface and
// toUpdatePatch — so a tagged owner/community can be reassigned or cleared.
func TestImmutableOverrideKeepsEditableRefsInPatch(t *testing.T) {
	f := false
	tbl := TableInfo{Name: "vehicleOwnershipHistory", Fields: []FieldInfo{
		{Name: "vehicleId", Type: "id", IsID: true, TableRef: "vehicles"},
		{Name: "userId", Type: "id", IsID: true, TableRef: "users", Optional: true},
		{Name: "communityId", Type: "id", IsID: true, TableRef: "communities", Optional: true},
		{Name: "notes", Type: "string", Optional: true},
	}}
	spec := ResourceSpec{
		Table: "vehicleOwnershipHistory", Path: "ownership_history",
		Module:    "heritage/ownershipHistoryApi.ts",
		Ownership: Ownership{Via: "vehicle_id", Table: "vehicles", Field: "ownerId"},
		Verbs:     []string{"create", "read", "update", "delete"},
		Expose: []ExposeField{
			{Field: "vehicleId", Wire: "vehicle_id", Ref: "vehicles"},
			{Field: "userId", Wire: "user_id", Ref: "users", Immutable: &f},
			{Field: "communityId", Wire: "community_id", Ref: "communities", Immutable: &f},
			{Field: "notes", Wire: "notes"},
		},
	}
	r := mustResolve(t, tbl, spec)

	imm := map[string]bool{}
	for _, fld := range r.InputFields {
		imm[fld.Wire] = fld.Immutable
	}
	if !imm["vehicle_id"] {
		t.Error("parent FK vehicle_id must stay immutable (create-only)")
	}
	if imm["user_id"] || imm["community_id"] {
		t.Errorf("user_id/community_id must be mutable with {\"immutable\": false}: %+v", imm)
	}

	src := EmitApiTS(r)
	patchBlock := between(src, "ApiPatch = {", "};")
	if !strings.Contains(patchBlock, "user_id:") || !strings.Contains(patchBlock, "community_id:") {
		t.Errorf("PATCH validator must include editable id-refs:\n%s", patchBlock)
	}
	if strings.Contains(patchBlock, "vehicle_id:") {
		t.Errorf("PATCH validator must exclude the parent FK vehicle_id:\n%s", patchBlock)
	}
	upd := between(src, "export function toUpdatePatch(", "return patch;")
	if !strings.Contains(upd, "patch.userId = input.user_id") {
		t.Errorf("toUpdatePatch must map user_id:\n%s", upd)
	}
	if strings.Contains(upd, "patch.vehicleId = input.vehicle_id") {
		t.Errorf("toUpdatePatch must NOT map the immutable parent FK:\n%s", upd)
	}
}

// TestExposeFieldUnmarshalImmutableOverride locks JSON parsing of the explicit
// {"immutable": false} flag on an id-reference expose entry.
func TestExposeFieldUnmarshalImmutableOverride(t *testing.T) {
	var e ExposeField
	if err := e.UnmarshalJSON([]byte(`{"field":"userId","wire":"user_id","ref":"users","immutable":false}`)); err != nil {
		t.Fatalf("unmarshal immutable-override expose: %v", err)
	}
	if e.Immutable == nil {
		t.Fatal("immutable flag must be parsed (non-nil)")
	}
	if *e.Immutable {
		t.Errorf("immutable flag = true, want false")
	}
	// Omitted → nil (default behavior preserved).
	var d ExposeField
	if err := d.UnmarshalJSON([]byte(`{"field":"vehicleId","wire":"vehicle_id","ref":"vehicles"}`)); err != nil {
		t.Fatalf("unmarshal default expose: %v", err)
	}
	if d.Immutable != nil {
		t.Errorf("omitted immutable must stay nil, got %v", *d.Immutable)
	}
}

// TestClearableIdRefWidensPatch locks the clearable id-ref override: the PATCH
// validator and patch scalar widen to accept "" (so a curated update can clear
// the FK), while input/output keep the strict Id type.
func TestClearableIdRefWidensPatch(t *testing.T) {
	f := false
	tbl := TableInfo{Name: "vehicleOwnershipHistory", Fields: []FieldInfo{
		{Name: "vehicleId", Type: "id", IsID: true, TableRef: "vehicles"},
		{Name: "userId", Type: "id", IsID: true, TableRef: "users", Optional: true},
		{Name: "notes", Type: "string", Optional: true},
	}}
	spec := ResourceSpec{
		Table: "vehicleOwnershipHistory", Path: "ownership_history",
		Module:    "heritage/ownershipHistoryApi.ts",
		Ownership: Ownership{Via: "vehicle_id", Table: "vehicles", Field: "ownerId"},
		Verbs:     []string{"create", "read", "update", "delete"},
		Expose: []ExposeField{
			{Field: "vehicleId", Wire: "vehicle_id", Ref: "vehicles"},
			{Field: "userId", Wire: "user_id", Ref: "users", Immutable: &f, Clearable: true},
			{Field: "notes", Wire: "notes"},
		},
	}
	r := mustResolve(t, tbl, spec)
	src := EmitApiTS(r)

	// Input validator keeps the strict v.id.
	inputBlock := between(src, "ApiInput = {", "};")
	if !strings.Contains(inputBlock, "user_id: v.optional(v.id(\"users\"))") {
		t.Errorf("clearable input must stay strict v.id:\n%s", inputBlock)
	}
	// PATCH validator widens to accept "".
	patchBlock := between(src, "ApiPatch = {", "};")
	if !strings.Contains(patchBlock, "user_id: v.optional(v.union(v.id(\"users\"), v.literal(\"\")))") {
		t.Errorf("clearable PATCH validator must widen to accept \"\":\n%s", patchBlock)
	}
	// toUpdatePatch casts the widened value into the column type.
	upd := between(src, "export function toUpdatePatch(", "return patch;")
	if !strings.Contains(upd, "patch.userId = input.user_id as Doc<\"vehicleOwnershipHistory\">[\"userId\"]") {
		t.Errorf("toUpdatePatch must cast the clearable value:\n%s", upd)
	}

	// The patch TS type carries the widened scalar.
	types := EmitApiTypesTS(r)
	if !strings.Contains(types, "user_id?: Id<\"users\"> | \"\"") {
		t.Errorf("patch type must widen the clearable id-ref:\n%s", types)
	}
}
