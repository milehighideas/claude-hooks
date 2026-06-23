package main

import (
	"strings"
	"testing"
)

func TestEmitApiTSStructure(t *testing.T) {
	r, _ := ResolveResource(vehiclesTable(), vehiclesSpec())
	src := EmitApiTS(r)

	must := []string{
		"DO NOT EDIT",                                // generated header
		"export const vehicleApiInput = {",           // input validator
		"make: v.string(),",                          // required field
		"for_sale: v.optional(v.boolean()),",         // wire rename + optional INPUT
		"export const vehicleApiPatch = {",           // patch (all optional)
		"export const vehicleApiOutput = v.object({", // output validator
		"created_at: v.string(),",                    // computed iso8601 → string out
		"photo_urls: v.array(v.string()),",           // computed r2Urls → string[] out
		"export const getForApi = internalQuery({",   // read fn
		"export const updateForApi = internalMutation({",
		"export const removeForApi = internalMutation({",
		"new Date(doc._creationTime).toISOString()", // iso8601 primitive
	}
	for _, frag := range must {
		if !strings.Contains(src, frag) {
			t.Errorf("EmitApiTS output missing fragment:\n  %s", frag)
		}
	}
}

// TestEmitApiTypesTS locks the separate wire-types file: it carries the
// generated header and exports the two plain-TS wire types (input + patch),
// with the patch derived as Partial of the input.
func TestEmitApiTypesTS(t *testing.T) {
	src := EmitApiTypesTS(mustResolve(t, vehiclesTable(), vehiclesSpec()))

	must := []string{
		"DO NOT EDIT",                     // generated header
		"export type VehicleApiInput = {", // input wire type
		"make: string;",                   // required scalar field
		"for_sale?: boolean;",             // wire rename + optional
		"export type VehicleApiPatch = Partial<VehicleApiInput>;", // patch derived
	}
	for _, frag := range must {
		if !strings.Contains(src, frag) {
			t.Errorf("EmitApiTypesTS output missing fragment:\n  %s\nGOT:\n%s", frag, src)
		}
	}

	// The wire-types file is plain TS — no validator/runtime imports.
	for _, banned := range []string{
		"import { v }",
		"import type { Doc",
		"v.string()",
		"export const",
	} {
		if strings.Contains(src, banned) {
			t.Errorf("EmitApiTypesTS must be a plain wire-types file, found %q:\n%s", banned, src)
		}
	}
}

// TestEmitApiTSImportsWireTypes locks the move of the wire types OUT of the Api
// module: it no longer declares them inline and instead imports them from its
// own ./types/<lcSingular>Api.types sibling.
func TestEmitApiTSImportsWireTypes(t *testing.T) {
	src := EmitApiTS(mustResolve(t, vehiclesTable(), vehiclesSpec()))

	if !strings.Contains(src, `import type { VehicleApiInput, VehicleApiPatch } from "./types/vehicleApi.types";`) {
		t.Errorf("EmitApiTS must import wire types from ./types/vehicleApi.types:\n%s", src)
	}
	// The inline TS type declarations must be gone (the runtime const
	// validators vehicleApiInput / vehicleApiPatch stay).
	if strings.Contains(src, "export type VehicleApiInput = {") {
		t.Errorf("EmitApiTS must NOT declare the wire input type inline:\n%s", src)
	}
	if strings.Contains(src, "export type VehicleApiPatch = Partial") {
		t.Errorf("EmitApiTS must NOT declare the wire patch type inline:\n%s", src)
	}
	// The type names are still used in the write-helper signatures.
	if !strings.Contains(src, "input: VehicleApiInput)") {
		t.Errorf("toCreateArgs must still use VehicleApiInput:\n%s", src)
	}
	if !strings.Contains(src, "input: VehicleApiPatch,") {
		t.Errorf("toUpdatePatch must still use VehicleApiPatch:\n%s", src)
	}
}

// TestEmitApiTSCoalesceOutputDefault locks the read-time default behavior: an
// optional-in-schema field that has a create default (a boolean's default is
// false even when not listed) must coalesce in toApi (`doc.col ?? default`),
// be REQUIRED in the output validator, and stay OPTIONAL in input + patch.
func TestEmitApiTSCoalesceOutputDefault(t *testing.T) {
	spec := vehiclesSpec()
	// Expose a boolean optional column with no listed default ("sold"), plus an
	// optional number column that DOES have a listed default ("year" already in
	// fixture defaults). Both must coalesce + be required in the output.
	spec.Expose = append(spec.Expose, ExposeField{Field: "sold", Wire: "sold"})
	src := EmitApiTS(mustResolve(t, vehiclesTable(), spec))

	// toApi: read-time defaults
	for _, frag := range []string{
		"sold: doc.sold ?? false,", // boolean implicit-false default
		"year: doc.year ?? 0,",     // listed numeric default
	} {
		if !strings.Contains(src, frag) {
			t.Errorf("toApi missing coalesced projection: %s", frag)
		}
	}

	// Output validator: required (NOT v.optional)
	outBody := between(src, "export const vehicleApiOutput = v.object({", "});")
	for _, frag := range []string{"sold: v.boolean(),", "year: v.number(),"} {
		if !strings.Contains(outBody, frag) {
			t.Errorf("output validator must mark default field required: %s\nGOT:\n%s", frag, outBody)
		}
	}
	if strings.Contains(outBody, "sold: v.optional") || strings.Contains(outBody, "year: v.optional") {
		t.Errorf("default field must NOT be optional in output:\n%s", outBody)
	}

	// Input validator: still optional
	inBody := between(src, "export const vehicleApiInput = {", "};")
	for _, frag := range []string{"sold: v.optional(v.boolean()),", "year: v.optional(v.number()),"} {
		if !strings.Contains(inBody, frag) {
			t.Errorf("input validator must keep default field optional: %s\nGOT:\n%s", frag, inBody)
		}
	}

	// Patch validator: still optional
	patchBody := between(src, "export const vehicleApiPatch = {", "};")
	for _, frag := range []string{"sold: v.optional(v.boolean()),", "year: v.optional(v.number()),"} {
		if !strings.Contains(patchBody, frag) {
			t.Errorf("patch validator must keep default field optional: %s\nGOT:\n%s", frag, patchBody)
		}
	}
}

// TestEmitApiTSOmitsDeleteVerb locks verb gating in the Api emitter: a resource
// whose verbs omit "delete" must NOT emit removeForApi, while the read/update
// internal fns remain.
func TestEmitApiTSOmitsDeleteVerb(t *testing.T) {
	spec := vehiclesSpec()
	spec.Verbs = []string{"create", "read", "update"} // no delete
	src := EmitApiTS(mustResolve(t, vehiclesTable(), spec))

	if strings.Contains(src, "removeForApi") {
		t.Errorf("delete-omitted resource must NOT emit removeForApi:\n%s", src)
	}
	if strings.Contains(src, "ctx.db.delete(") {
		t.Errorf("delete-omitted resource must NOT emit ctx.db.delete:\n%s", src)
	}
	// read + update internal fns remain.
	for _, frag := range []string{
		"export const getForApi = internalQuery({",
		"export const updateForApi = internalMutation({",
	} {
		if !strings.Contains(src, frag) {
			t.Errorf("delete-omitted resource must keep other internal fns, missing %q:\n%s", frag, src)
		}
	}
}

// TestEmitApiTSOmitsUpdateVerb is the symmetric gate for "update": omitting it
// suppresses updateForApi (and its toUpdatePatch ctx.db.patch) while keeping the
// read + delete internal fns.
func TestEmitApiTSOmitsUpdateVerb(t *testing.T) {
	spec := vehiclesSpec()
	spec.Verbs = []string{"create", "read", "delete"} // no update
	src := EmitApiTS(mustResolve(t, vehiclesTable(), spec))

	if strings.Contains(src, "updateForApi") {
		t.Errorf("update-omitted resource must NOT emit updateForApi:\n%s", src)
	}
	if strings.Contains(src, "ctx.db.patch(") {
		t.Errorf("update-omitted resource must NOT emit ctx.db.patch:\n%s", src)
	}
	for _, frag := range []string{
		"export const getForApi = internalQuery({",
		"export const removeForApi = internalMutation({",
	} {
		if !strings.Contains(src, frag) {
			t.Errorf("update-omitted resource must keep other internal fns, missing %q:\n%s", frag, src)
		}
	}
}

// between returns the substring of s between the first occurrence of start and
// the next occurrence of end after it (exclusive). Returns "" if not found.
func between(s, start, end string) string {
	i := strings.Index(s, start)
	if i < 0 {
		return ""
	}
	i += len(start)
	j := strings.Index(s[i:], end)
	if j < 0 {
		return ""
	}
	return s[i : i+j]
}

// TestEmitApiTSRequiredWithDefaultNoCoalesce locks the schema-required + default
// case (e.g. "trim"): always present, so REQUIRED in the output validator, but
// NEVER coalesced in toApi (the column is always set) — while still optional on
// input/patch because the create default fills it.
func TestEmitApiTSRequiredWithDefaultNoCoalesce(t *testing.T) {
	spec := vehiclesSpec()
	spec.Expose = append(spec.Expose, ExposeField{Field: "trim", Wire: "trim"})
	spec.Defaults["trim"] = ""
	tbl := vehiclesTable()
	// trim is REQUIRED in the schema (Optional: false).
	tbl.Fields = append(tbl.Fields, FieldInfo{Name: "trim", Type: "string"})
	src := EmitApiTS(mustResolve(t, tbl, spec))

	// toApi: no coalesce (always present)
	if !strings.Contains(src, "trim: doc.trim,") {
		t.Errorf("schema-required field must not coalesce in toApi")
	}
	if strings.Contains(src, "trim: doc.trim ??") {
		t.Errorf("schema-required field must NOT be coalesced in toApi")
	}
	// output: required
	outBody := between(src, "export const vehicleApiOutput = v.object({", "});")
	if !strings.Contains(outBody, "trim: v.string(),") || strings.Contains(outBody, "trim: v.optional") {
		t.Errorf("schema-required+default field must be required in output:\n%s", outBody)
	}
	// input: optional (default fills it on create)
	inBody := between(src, "export const vehicleApiInput = {", "};")
	if !strings.Contains(inBody, "trim: v.optional(v.string()),") {
		t.Errorf("field with a create default must be optional on input:\n%s", inBody)
	}
}

func TestEmitApiTSWriteAliasAndDefault(t *testing.T) {
	spec := vehiclesSpec()
	spec.Expose = append(spec.Expose, ExposeField{Field: "color", Wire: "color"})
	spec.WriteAliases = map[string][]string{"color": {"color", "exteriorColor"}}
	spec.Defaults["color"] = ""
	tbl := vehiclesTable()
	tbl.Fields = append(tbl.Fields, FieldInfo{Name: "color", Type: "string", Optional: true})
	src := EmitApiTS(mustResolve(t, tbl, spec))
	for _, frag := range []string{"patch.color = ", "patch.exteriorColor = "} {
		if !strings.Contains(src, frag) {
			t.Errorf("missing write-alias fragment: %s", frag)
		}
	}
}

// TestEmitApiTSHatchOut asserts a hatch computed field emits its declared output
// validator token in vehicleApiOutput (never the untyped v.any() escape hatch).
func TestEmitApiTSHatchOut(t *testing.T) {
	spec := vehiclesSpec()
	spec.Computed["custom"] = ComputedField{Hatch: true, Out: "v.array(v.string())"}
	src := EmitApiTS(mustResolve(t, vehiclesTable(), spec))
	if strings.Contains(src, "v.any()") {
		t.Errorf("hatch with 'out' must not emit v.any():\n%s", src)
	}
	if !strings.Contains(src, "custom: v.array(v.string()),") {
		t.Errorf("hatch output validator must use the declared 'out' token")
	}
	// hatch projection expression still emitted in toApi
	if !strings.Contains(src, "custom: projections.custom(doc),") {
		t.Errorf("hatch toApi expression missing")
	}
	// the projections module must be imported when any hatch field is present
	if !strings.Contains(src, `import * as projections from "./vehicle.projections";`) {
		t.Errorf("hatch must emit the projections import line:\n%s", src)
	}
}

// TestEmitApiTSWritePathDelegation locks write-path delegation: when the overlay
// declares writePath.update/delete model helpers, updateForApi/removeForApi must
// (1) import those helpers grouped per module and (2) call the helper instead of
// raw ctx.db.patch / ctx.db.delete.
func TestEmitApiTSWritePathDelegation(t *testing.T) {
	spec := vehiclesSpec()
	spec.WritePath = map[string]string{
		"update": "model/vehicleWrites.applyVehicleUpdate",
		"delete": "model/vehicleWrites.removeVehicle",
	}
	src := EmitApiTS(mustResolve(t, vehiclesTable(), spec))

	// (1) grouped import: both helpers from the single shared module, one line.
	if !strings.Contains(src, `import { applyVehicleUpdate, removeVehicle } from "./model/vehicleWrites";`) {
		t.Errorf("write-path helpers must be imported grouped per module:\n%s", src)
	}

	// (2) updateForApi delegates to the helper, not ctx.db.patch.
	updateBody := between(src, "export const updateForApi = internalMutation({", "});")
	if !strings.Contains(updateBody, "await applyVehicleUpdate(ctx, args.id, doc, patch);") {
		t.Errorf("updateForApi must call the write-path helper:\n%s", updateBody)
	}
	if strings.Contains(updateBody, "ctx.db.patch(") {
		t.Errorf("updateForApi must NOT emit raw ctx.db.patch when delegating:\n%s", updateBody)
	}

	// (2) removeForApi delegates to the helper, not ctx.db.delete.
	removeBody := between(src, "export const removeForApi = internalMutation({", "});")
	if !strings.Contains(removeBody, "await removeVehicle(ctx, args.id, doc);") {
		t.Errorf("removeForApi must call the write-path helper:\n%s", removeBody)
	}
	if strings.Contains(removeBody, "ctx.db.delete(") {
		t.Errorf("removeForApi must NOT emit raw ctx.db.delete when delegating:\n%s", removeBody)
	}
}

// TestEmitApiTSWritePathAbsent is the complementary branch: with no writePath
// overlay, updateForApi/removeForApi emit raw ctx.db.patch / ctx.db.delete and
// no model-helper import is produced.
func TestEmitApiTSWritePathAbsent(t *testing.T) {
	src := EmitApiTS(mustResolve(t, vehiclesTable(), vehiclesSpec()))

	updateBody := between(src, "export const updateForApi = internalMutation({", "});")
	if !strings.Contains(updateBody, `await ctx.db.patch("vehicles", args.id, patch);`) {
		t.Errorf("updateForApi must emit raw ctx.db.patch when no writePath:\n%s", updateBody)
	}
	removeBody := between(src, "export const removeForApi = internalMutation({", "});")
	if !strings.Contains(removeBody, `await ctx.db.delete("vehicles", args.id);`) {
		t.Errorf("removeForApi must emit raw ctx.db.delete when no writePath:\n%s", removeBody)
	}
	// no spurious model import
	if strings.Contains(src, `from "./model/`) {
		t.Errorf("no write-path means no model helper import:\n%s", src)
	}
}

// TestEmitApiTSComputedIso8601Expr locks the exact emitted expression for an
// iso8601 computed primitive in toApi.
func TestEmitApiTSComputedIso8601Expr(t *testing.T) {
	src := EmitApiTS(mustResolve(t, vehiclesTable(), vehiclesSpec()))
	if !strings.Contains(src, "created_at: new Date(doc._creationTime).toISOString(),") {
		t.Errorf("iso8601 computed must emit new Date(...).toISOString():\n%s", src)
	}
}

// TestEmitApiTSComputedR2UrlsExpr locks the r2Urls map/filter body AND the
// getR2Url import line.
func TestEmitApiTSComputedR2UrlsExpr(t *testing.T) {
	src := EmitApiTS(mustResolve(t, vehiclesTable(), vehiclesSpec()))
	if !strings.Contains(src, `import { getR2Url } from "../r2/r2Utils";`) {
		t.Errorf("r2Urls computed must emit the getR2Url import:\n%s", src)
	}
	for _, frag := range []string{
		"photo_urls: (doc.vehicleGalleryKeys ?? [])",
		".map((key) => getR2Url(key))",
		".filter((url): url is string => url !== null)",
	} {
		if !strings.Contains(src, frag) {
			t.Errorf("r2Urls computed body missing fragment %q:\n%s", frag, src)
		}
	}
}

// TestEmitApiTSComputedSumExpr locks the additive join produced from a sum
// computed field's "of" columns: (doc.a ?? 0) + (doc.b ?? 0).
func TestEmitApiTSComputedSumExpr(t *testing.T) {
	spec := vehiclesSpec()
	spec.Computed["total"] = ComputedField{As: "sum", Of: []string{"likes", "shares"}}
	src := EmitApiTS(mustResolve(t, vehiclesTable(), spec))
	if !strings.Contains(src, "total: (doc.likes ?? 0) + (doc.shares ?? 0),") {
		t.Errorf("sum computed must emit additive coalesced join:\n%s", src)
	}
	// sum is a number on the wire.
	outBody := between(src, "export const vehicleApiOutput = v.object({", "});")
	if !strings.Contains(outBody, "total: v.number(),") {
		t.Errorf("sum computed output validator must be v.number():\n%s", outBody)
	}
}

// TestEmitApiTSCreateConstants locks the sorted, literal-rendered _create
// constants emitted into toCreateArgs: empty array, number, and string-constant
// via tsDefaultLiteral.
func TestEmitApiTSCreateConstants(t *testing.T) {
	spec := vehiclesSpec()
	spec.CreateConstants = map[string]any{
		"status":   "draft",
		"viewArr":  []any{},
		"revision": 1.0,
	}
	src := EmitApiTS(mustResolve(t, vehiclesTable(), spec))
	createBody := between(src, "export function toCreateArgs(", "}")

	// Each constant rendered through tsDefaultLiteral.
	for _, frag := range []string{
		"revision: 1,",       // number
		"status: \"draft\",", // string constant (JSON-quoted)
		"viewArr: [],",       // empty array literal
	} {
		if !strings.Contains(createBody, frag) {
			t.Errorf("toCreateArgs missing create constant %q:\n%s", frag, createBody)
		}
	}

	// Sorted order: revision < status < viewArr (lexicographic by column).
	ri := strings.Index(createBody, "revision:")
	si := strings.Index(createBody, "status:")
	vi := strings.Index(createBody, "viewArr:")
	if ri >= si || si >= vi {
		t.Errorf("create constants must be emitted in sorted column order, got positions revision=%d status=%d viewArr=%d:\n%s", ri, si, vi, createBody)
	}
}

// TestEmitApiTSOwnershipAbsent locks the non-owned branch: with Ownership="",
// no owns<Singular> guard is generated, toCreateArgs omits ownerType/owner
// scoping, and the handlers guard only on `!doc` (no ownsVehicle actor scoping).
func TestEmitApiTSOwnershipAbsent(t *testing.T) {
	spec := vehiclesSpec()
	spec.Ownership = Ownership{}
	src := EmitApiTS(mustResolve(t, vehiclesTable(), spec))

	if strings.Contains(src, "ownsVehicle") {
		t.Errorf("ownership-absent resource must NOT emit ownsVehicle guard:\n%s", src)
	}
	if strings.Contains(src, "function ownsVehicle(") {
		t.Errorf("ownership-absent resource must NOT define an owns helper:\n%s", src)
	}

	// toCreateArgs: no ownerType / owner column scoping.
	createBody := between(src, "export function toCreateArgs(", "}")
	if strings.Contains(createBody, "ownerType:") || strings.Contains(createBody, "ownerId,") {
		t.Errorf("ownership-absent toCreateArgs must not scope by owner:\n%s", createBody)
	}

	// Handlers guard only on !doc (no owns guard tail).
	getBody := between(src, "export const getForApi = internalQuery({", "});")
	if !strings.Contains(getBody, "if (!doc) return null;") {
		t.Errorf("getForApi must guard on !doc only when unowned:\n%s", getBody)
	}
	updateBody := between(src, "export const updateForApi = internalMutation({", "});")
	if !strings.Contains(updateBody, "if (!doc) return null;") {
		t.Errorf("updateForApi must guard on !doc only when unowned:\n%s", updateBody)
	}
	removeBody := between(src, "export const removeForApi = internalMutation({", "});")
	if !strings.Contains(removeBody, "if (!doc) return { deleted: false };") {
		t.Errorf("removeForApi must guard on !doc only when unowned:\n%s", removeBody)
	}
}
