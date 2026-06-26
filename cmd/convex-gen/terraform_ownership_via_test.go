package main

import (
	"strings"
	"testing"
)

// albumsTable models a child resource (vehicle portfolio albums) whose ownership
// is authorized through a parent vehicle FK, mirroring the hand-written
// vehicles/albumsApi.ts reference (ownsParentVehicle pattern).
func albumsTable() TableInfo {
	return TableInfo{Name: "albums", Fields: []FieldInfo{
		{Name: "vehicleId", Type: "id", IsID: true, TableRef: "vehicles"},
		{Name: "name", Type: "string"},
		{Name: "sortOrder", Type: "number", Optional: true},
	}}
}

// albumsViaParentSpec exposes the FK column (renamed wire vehicle_id) and scopes
// ownership through the parent vehicles row via the object form of Ownership.
func albumsViaParentSpec() ResourceSpec {
	return ResourceSpec{
		Table: "albums", Path: "albums", Module: "vehicles/albumsApi.ts",
		Ownership: Ownership{Via: "vehicle_id", Table: "vehicles", Field: "ownerId"},
		Verbs:     []string{"create", "read", "update", "delete"},
		Expose: []ExposeField{
			{Field: "vehicleId", Wire: "vehicle_id", Ref: "vehicles"},
			{Field: "name", Wire: "name"},
			{Field: "sortOrder", Wire: "sort_order"},
		},
	}
}

// TestOwnershipUnmarshalObjectForm locks parsing of the FK-via-parent object
// form: {via,table,field} populates Via/Table/Field, ViaParent() is true, and a
// missing field defaults to "ownerId".
func TestOwnershipUnmarshalObjectForm(t *testing.T) {
	var o Ownership
	if err := o.UnmarshalJSON([]byte(`{"via":"vehicle_id","table":"vehicles","field":"ownerId"}`)); err != nil {
		t.Fatalf("unmarshal object ownership: %v", err)
	}
	if o.Via != "vehicle_id" || o.Table != "vehicles" || o.Field != "ownerId" {
		t.Fatalf("object ownership parsed wrong: %+v", o)
	}
	if !o.ViaParent() {
		t.Error("object-form ownership must report ViaParent() == true")
	}
	if o.IsZero() {
		t.Error("object-form ownership must not be zero")
	}

	// field defaults to ownerId when omitted
	var d Ownership
	if err := d.UnmarshalJSON([]byte(`{"via":"vehicle_id","table":"vehicles"}`)); err != nil {
		t.Fatalf("unmarshal object ownership w/o field: %v", err)
	}
	if d.Field != "ownerId" {
		t.Errorf("ownership.field default = %q, want ownerId", d.Field)
	}

	// missing "via" is a hard error
	var bad Ownership
	if err := bad.UnmarshalJSON([]byte(`{"table":"vehicles"}`)); err == nil {
		t.Error("ownership object without 'via' must error")
	}
}

// TestOwnershipUnmarshalStringForm keeps the row-local string form working: a
// bare JSON string sets Field and is NOT a via-parent ownership.
func TestOwnershipUnmarshalStringForm(t *testing.T) {
	var o Ownership
	if err := o.UnmarshalJSON([]byte(`"ownerId"`)); err != nil {
		t.Fatalf("unmarshal string ownership: %v", err)
	}
	if o.Field != "ownerId" {
		t.Errorf("string ownership Field = %q, want ownerId", o.Field)
	}
	if o.ViaParent() {
		t.Error("string-form ownership must NOT report ViaParent()")
	}
	if o.Via != "" || o.Table != "" {
		t.Errorf("string-form ownership must leave Via/Table empty: %+v", o)
	}
}

// TestResolveOwnershipViaColumn locks the FK-via-parent resolution: the via wire
// name resolves to the underlying doc column (vehicle_id → vehicleId).
func TestResolveOwnershipViaColumn(t *testing.T) {
	r := mustResolve(t, albumsTable(), albumsViaParentSpec())
	if r.OwnershipViaColumn != "vehicleId" {
		t.Errorf("OwnershipViaColumn = %q, want vehicleId (resolved from wire vehicle_id)", r.OwnershipViaColumn)
	}
	if !r.Ownership.ViaParent() {
		t.Error("resolved resource must carry via-parent ownership")
	}
}

// TestResolveOwnershipViaColumnByColumnName locks the fallback path: when the via
// value names the doc column directly (not the wire alias), it still resolves.
func TestResolveOwnershipViaColumnByColumnName(t *testing.T) {
	spec := albumsViaParentSpec()
	spec.Ownership.Via = "vehicleId" // column name, not the wire alias
	r := mustResolve(t, albumsTable(), spec)
	if r.OwnershipViaColumn != "vehicleId" {
		t.Errorf("OwnershipViaColumn = %q, want vehicleId (resolved by column name)", r.OwnershipViaColumn)
	}
}

// TestResolveOwnershipViaColumnUnknown locks the error path: a via that matches
// neither an exposed field nor a real column is a resolve-time error.
func TestResolveOwnershipViaColumnUnknown(t *testing.T) {
	spec := albumsViaParentSpec()
	spec.Ownership.Via = "nope"
	_, err := ResolveResource(albumsTable(), spec)
	if err == nil {
		t.Fatal("expected error for unresolvable ownership.via, got nil")
	}
	if !strings.Contains(err.Error(), "ownership.via") {
		t.Errorf("error = %v, want it to mention ownership.via", err)
	}
}

// TestEmitApiTSOwnershipViaParent is the core emitter lock for feature 1: the
// FK-via-parent resource must emit an async owns<Singular> guard that loads the
// parent row and checks parent ownership — NOT a row-local doc.<owner> gate — and
// every handler must await that guard.
func TestEmitApiTSOwnershipViaParent(t *testing.T) {
	src := EmitApiTS(mustResolve(t, albumsTable(), albumsViaParentSpec()))

	// async guard signature + parent load + parent-owner check
	for _, frag := range []string{
		"async function ownsAlbum(",
		`const parent = await ctx.db.get("vehicles", doc.vehicleId);`,
		`!!parent && parent.ownerType === "user" && parent.ownerId === actorId`,
		"): Promise<boolean> {",
	} {
		if !strings.Contains(src, frag) {
			t.Errorf("via-parent owns guard missing fragment:\n  %s\nGOT:\n%s", frag, src)
		}
	}

	// It must NOT emit the row-local synchronous form (no doc.ownerId gate).
	if strings.Contains(src, "function ownsAlbum(doc: Doc<\"albums\">, actorId: Id<\"users\">): boolean {") {
		t.Errorf("via-parent resource must NOT emit a row-local synchronous owns guard:\n%s", src)
	}
	if strings.Contains(src, "doc.ownerType === \"user\" && doc.ownerId === actorId") {
		t.Errorf("via-parent resource must NOT check a row-local owner column:\n%s", src)
	}

	// Each handler awaits the async guard.
	for _, body := range []struct{ start, want string }{
		{"export const getForApi = internalQuery({", "if (!doc || !(await ownsAlbum(ctx, doc, args.actorId))) return null;"},
		{"export const updateForApi = internalMutation({", "if (!doc || !(await ownsAlbum(ctx, doc, args.actorId))) return null;"},
		{"export const removeForApi = internalMutation({", "if (!doc || !(await ownsAlbum(ctx, doc, args.actorId))) return { deleted: false };"},
	} {
		got := between(src, body.start, "});")
		if !strings.Contains(got, body.want) {
			t.Errorf("handler %q must await the via-parent guard:\n  want: %s\nGOT:\n%s", body.start, body.want, got)
		}
	}
}

// TestEmitApiTSViaParentNoOwnerStamp locks that an FK-via-parent create writes no
// owner column on the row: toCreateArgs must NOT stamp ownerType / the owner
// column (authorization happens by loading the parent, not by row denormalization).
func TestEmitApiTSViaParentNoOwnerStamp(t *testing.T) {
	src := EmitApiTS(mustResolve(t, albumsTable(), albumsViaParentSpec()))
	createBody := between(src, "export function toCreateArgs(", "}")
	if strings.Contains(createBody, "ownerType:") {
		t.Errorf("via-parent toCreateArgs must NOT stamp ownerType:\n%s", createBody)
	}
	// The FK column itself is a normal input field (vehicleId) and IS written; the
	// owner column ("ownerId") must not appear as a stamped scoping line.
	if strings.Contains(createBody, "    ownerId,\n") {
		t.Errorf("via-parent toCreateArgs must NOT stamp the owner column:\n%s", createBody)
	}
}

// TestEmitApiTSRowLocalOwnershipStillWorks guards against a regression in the
// row-local (string) form while feature 1 is added: the vehicles fixture must
// keep emitting the synchronous owns guard and the toCreateArgs owner stamp.
func TestEmitApiTSRowLocalOwnershipStillWorks(t *testing.T) {
	src := EmitApiTS(mustResolve(t, vehiclesTable(), vehiclesSpec()))

	// synchronous (non-async) row-local guard
	if !strings.Contains(src, "function ownsVehicle(doc: Doc<\"vehicles\">, actorId: Id<\"users\">): boolean {") {
		t.Errorf("row-local ownership must emit a synchronous owns guard:\n%s", src)
	}
	if !strings.Contains(src, "return doc.ownerType === \"user\" && doc.ownerId === actorId;") {
		t.Errorf("row-local guard must check the row's own owner column:\n%s", src)
	}
	if strings.Contains(src, "async function ownsVehicle(") {
		t.Errorf("row-local ownership must NOT emit an async (via-parent) guard:\n%s", src)
	}
	// no parent load for the row-local form
	if strings.Contains(src, "const parent = await ctx.db.get(") {
		t.Errorf("row-local ownership must NOT load a parent row:\n%s", src)
	}

	// toCreateArgs stamps owner scoping
	createBody := between(src, "export function toCreateArgs(", "}")
	if !strings.Contains(createBody, "ownerType: \"user\" as const,") {
		t.Errorf("row-local toCreateArgs must stamp ownerType:\n%s", createBody)
	}
	if !strings.Contains(createBody, "    ownerId,\n") {
		t.Errorf("row-local toCreateArgs must stamp the owner column:\n%s", createBody)
	}

	// handlers use the synchronous (non-awaited) guard tail
	getBody := between(src, "export const getForApi = internalQuery({", "});")
	if !strings.Contains(getBody, "if (!doc || !ownsVehicle(doc, args.actorId)) return null;") {
		t.Errorf("row-local handler must use the synchronous guard tail:\n%s", getBody)
	}
}
