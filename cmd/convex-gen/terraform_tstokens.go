package main

import (
	"fmt"
	"strings"
)

// generatedHeader is the DO-NOT-EDIT banner shared by every emitted TS file.
func generatedHeader() string {
	return "/**\n * Auto-generated public-API surface — DO NOT EDIT MANUALLY.\n" +
		" * Run 'convex-gen' to regenerate. Curated from convex-terraform-gen.json.\n */\n\n"
}

// optional wraps a validator token in v.optional() when the field is optional on
// the wire (input + patch).
func optional(f ResolvedField) string {
	if f.Optional {
		return "v.optional(" + f.TSType + ")"
	}
	return f.TSType
}

// patchValidatorToken is the validator token for a field on the PATCH surface,
// honoring a PatchTSType override (a Clearable id-ref widens to accept "").
func patchValidatorToken(f ResolvedField) string {
	if f.PatchTSType != "" {
		return f.PatchTSType
	}
	return f.TSType
}

// patchScalarType is the TS scalar type for a field on the PATCH surface,
// honoring a PatchTSScalar override (a Clearable id-ref → Id<"X"> | "").
func patchScalarType(f ResolvedField) string {
	if f.PatchTSScalar != "" {
		return f.PatchTSScalar
	}
	return fieldScalarType(f)
}

// outputType is the validator token for a field in the output object. Fields
// with a read-time default coalesce to that default, so they are always present
// and emitted REQUIRED — even when they are optional on input/patch.
func outputType(f ResolvedField) string {
	if f.OutputRequired {
		return f.TSType
	}
	return optional(f)
}

// computedOutType is the validator token for a computed field in the output
// object. Primitives have known shapes; the escape hatch uses its declared
// output token (guaranteed non-empty by ResolveResource).
func computedOutType(c ResolvedComputed) string {
	switch c.As {
	case "r2Urls":
		return "v.array(v.string())"
	case "r2Url":
		return "v.string()"
	case "iso8601":
		return "v.string()"
	case "sum":
		return "v.number()"
	default: // hatch
		return c.Out
	}
}

// computedExpr is the TypeScript expression that derives a computed field from
// the document inside toApi().
func computedExpr(c ResolvedComputed) string {
	switch c.As {
	case "iso8601":
		return "new Date(doc." + c.From + ").toISOString()"
	case "r2Urls":
		return "(doc." + c.From + " ?? [])\n      .map((key) => getR2Url(key))\n      .filter((url): url is string => url !== null)"
	case "r2Url":
		// Single R2 object key → its readable URL (empty string when unset),
		// mirroring the hand-written `getR2Url(doc.key) ?? ""` photo projection.
		return "doc." + c.From + " ? (getR2Url(doc." + c.From + ") ?? \"\") : \"\""
	case "sum":
		parts := make([]string, len(c.Of))
		for i, o := range c.Of {
			parts[i] = "(doc." + o + " ?? 0)"
		}
		return strings.Join(parts, " + ")
	default: // hatch
		return "projections." + c.Wire + "(doc)"
	}
}

// jsTypeOf maps a Convex validator token to the JS typeof string used in the
// runtime body-guards of the routes file.
func jsTypeOf(tsType string) string {
	switch {
	case strings.HasPrefix(tsType, "v.number()"):
		return "number"
	case strings.HasPrefix(tsType, "v.boolean()"):
		return "boolean"
	default:
		return "string"
	}
}

// fieldScalarType returns the TS scalar type for a resolved field, honoring an
// explicit TSScalar override (e.g. Id<"vehicles"> for an id-reference field) and
// otherwise deriving from the validator token.
func fieldScalarType(f ResolvedField) string {
	if f.TSScalar != "" {
		return f.TSScalar
	}
	return tsScalarType(f.TSType)
}

// inputHasIDRef reports whether any input field forces an Id import in the
// wire-types file: a top-level id-reference / id-array (scalar starts with Id<),
// OR a nested object / fixed-key map whose sub-fields include an id-ref/id-array.
// Enum/scalar nested fields set TSScalar too but carry no Id, so the top-level
// check stays prefix-precise and the nested check is shape-precise.
func inputHasIDRef(r ResolvedResource) bool {
	for _, f := range r.InputFields {
		if strings.HasPrefix(f.TSScalar, "Id<") {
			return true
		}
		if len(f.Nested) > 0 && nestedHasIDRef(f.Nested) {
			return true
		}
		if f.Map != nil && nestedHasIDRef(f.Map.Object) {
			return true
		}
	}
	return false
}

// lowerCamel converts a snake_case nested wire key to its DB camelCase column
// (part_number → partNumber, name → name). The first segment stays lower-case.
func lowerCamel(s string) string {
	parts := strings.Split(s, "_")
	for i := 1; i < len(parts); i++ {
		parts[i] = capitalize(parts[i])
	}
	return strings.Join(parts, "")
}

// nestedDbColumn returns the DB column behind a nested wire key.
func nestedDbColumn(key string) string { return lowerCamel(key) }

// emitNestedObjectMap renders an object-literal `{ <a>: <srcVar>.<b>, … }` body
// where keyOf/valOf select the destination key and source accessor for each
// nested field. Used by toApi (wire←db) and the write helpers (db←wire).
func emitNestedObjectMap(fields []NestedField, srcVar string, wireToDb bool) string {
	var b strings.Builder
	for _, f := range fields {
		dst := f.Key
		src := nestedDbColumn(f.Key)
		if !wireToDb {
			// db ← wire: destination is the DB column, source is the wire key.
			dst, src = src, f.Key
		}
		fmt.Fprintf(&b, "      %s: %s%s,\n", dst, srcVar+"."+src, nestedReadNarrowing(f, wireToDb))
	}
	return b.String()
}

// nestedReadNarrowing returns a trailing TS cast for a nested sub-field's
// PROJECTION expression when reading a row column back out to the curated wire
// shape (the toApi direction, wireToDb=true). The public surface intentionally
// exposes a NARROWER enum than the underlying schema column (e.g. privacy.type
// drops the deprecated UPPERCASE variants), so the read value's type is wider
// than the curated output validator. A `as "a" | "b" | …` cast narrows it so the
// generated toApi type-checks against the curated output validator. It is empty
// for non-enum sub-fields and for the write direction (where the wire value is
// already the curated type and is being widened into the column, which is safe).
func nestedReadNarrowing(f NestedField, wireToDb bool) string {
	if !wireToDb || len(f.Enum) == 0 {
		return ""
	}
	return " as " + enumScalarType(f.Enum)
}

// enumValidatorToken renders a string-literal union validator token, e.g.
// v.union(v.literal("a"), v.literal("b")).
func enumValidatorToken(values []string) string {
	parts := make([]string, len(values))
	for i, val := range values {
		lit, _ := jsonMarshalString(val)
		parts[i] = "v.literal(" + lit + ")"
	}
	return "v.union(" + strings.Join(parts, ", ") + ")"
}

// enumScalarType renders the TS string-union scalar type, e.g. "a" | "b".
func enumScalarType(values []string) string {
	parts := make([]string, len(values))
	for i, val := range values {
		lit, _ := jsonMarshalString(val)
		parts[i] = lit
	}
	return strings.Join(parts, " | ")
}

// nestedFieldValidator maps one nested sub-field to its (un-optional'd) Convex
// validator, dispatching on its rich shape (enum / id / id-array / scalar).
func nestedFieldValidator(f NestedField) string {
	switch {
	case len(f.Enum) > 0:
		return enumValidatorToken(f.Enum)
	case f.RefArray != "":
		return "v.array(v.id(\"" + f.RefArray + "\"))"
	case f.Ref != "":
		return "v.id(\"" + f.Ref + "\")"
	case f.Type == "number":
		return "v.number()"
	case f.Type == "boolean":
		return "v.boolean()"
	default:
		return "v.string()"
	}
}

// nestedFieldScalar maps one nested sub-field to its TS scalar type, dispatching
// on its rich shape (enum / id / id-array / scalar).
func nestedFieldScalar(f NestedField) string {
	switch {
	case len(f.Enum) > 0:
		return enumScalarType(f.Enum)
	case f.RefArray != "":
		return "Id<\"" + f.RefArray + "\">[]"
	case f.Ref != "":
		return "Id<\"" + f.Ref + "\">"
	case f.Type == "number":
		return "number"
	case f.Type == "boolean":
		return "boolean"
	default:
		return "string"
	}
}

// nestedHasIDRef reports whether any nested sub-field is an id-ref or id-array
// (so a wire-types file carrying it must import Id).
func nestedHasIDRef(fields []NestedField) bool {
	for _, f := range fields {
		if f.Ref != "" || f.RefArray != "" {
			return true
		}
	}
	return false
}

// nestedObjectValidator renders the inner v.object({…}) for a typed nested shape.
func nestedObjectValidator(fields []NestedField) string {
	var b strings.Builder
	b.WriteString("v.object({ ")
	parts := make([]string, len(fields))
	for i, f := range fields {
		inner := nestedFieldValidator(f)
		if f.Optional {
			inner = "v.optional(" + inner + ")"
		}
		parts[i] = f.Key + ": " + inner
	}
	b.WriteString(strings.Join(parts, ", "))
	b.WriteString(" })")
	return b.String()
}

// nestedValidatorToken renders the full validator token for a nested field:
// v.array(v.object({…})) for the array form, v.object({…}) for the single form.
func nestedValidatorToken(fields []NestedField, single bool) string {
	obj := nestedObjectValidator(fields)
	if single {
		return obj
	}
	return "v.array(" + obj + ")"
}

// nestedScalarType renders the derived TS type for a nested field: { … } for the
// single form, { … }[] for the array form.
func nestedScalarType(fields []NestedField, single bool) string {
	parts := make([]string, len(fields))
	for i, f := range fields {
		opt := ""
		if f.Optional {
			opt = "?"
		}
		parts[i] = f.Key + opt + ": " + nestedFieldScalar(f)
	}
	obj := "{ " + strings.Join(parts, "; ") + " }"
	if single {
		return obj
	}
	return obj + "[]"
}

// objectMapValidator renders the fixed-key object-map validator: a v.object whose
// keys are the map's Keys, each holding an OPTIONAL nested object value.
func objectMapValidator(m *ObjectMapShape) string {
	inner := nestedObjectValidator(m.Object)
	parts := make([]string, len(m.Keys))
	for i, k := range m.Keys {
		parts[i] = k + ": v.optional(" + inner + ")"
	}
	return "v.object({ " + strings.Join(parts, ", ") + " })"
}

// objectMapScalar renders the derived TS type for a fixed-key object map:
// { timeline?: <obj>; build?: <obj>; … }.
func objectMapScalar(m *ObjectMapShape) string {
	obj := nestedScalarType(m.Object, true)
	parts := make([]string, len(m.Keys))
	for i, k := range m.Keys {
		parts[i] = k + "?: " + obj
	}
	return "{ " + strings.Join(parts, "; ") + " }"
}

// tsScalarType maps a validator token to the TS scalar type used in the derived
// input type.
func tsScalarType(tsType string) string {
	switch {
	case strings.HasPrefix(tsType, "v.number()"):
		return "number"
	case strings.HasPrefix(tsType, "v.boolean()"):
		return "boolean"
	case strings.HasPrefix(tsType, "v.array(v.number())"):
		return "number[]"
	case strings.HasPrefix(tsType, "v.array(v.boolean())"):
		return "boolean[]"
	case strings.HasPrefix(tsType, "v.array"):
		return "string[]"
	default:
		return "string"
	}
}

// tsDefaultLiteral renders a Go default value (from the overlay JSON) as a TS
// literal. JSON numbers decode to float64; whole numbers are printed without a
// trailing ".0".
func tsDefaultLiteral(d any) string {
	switch v := d.(type) {
	case string:
		lit, _ := jsonMarshalString(v)
		return lit
	case bool:
		if v {
			return "true"
		}
		return "false"
	case float64:
		if v == float64(int64(v)) {
			return fmt.Sprintf("%d", int64(v))
		}
		return fmt.Sprintf("%g", v)
	case nil:
		return "undefined"
	case []any:
		parts := make([]string, len(v))
		for i, el := range v {
			parts[i] = tsDefaultLiteral(el)
		}
		return "[" + strings.Join(parts, ", ") + "]"
	default:
		return fmt.Sprintf("%v", v)
	}
}

// createConstantLiteral renders a createConstants value. The "$actor" sentinel
// injects the authenticated user id (the actor param of toCreateArgs, named
// ownerId) instead of a JSON literal. String and number constants are emitted
// `as const` so the inferred toCreateArgs return type carries the literal type
// (e.g. "owner" rather than string), which type-checks against a backing
// `_create` mutation that validates the column with v.literal(...). A literal is
// always assignable to its widened type, so `as const` is safe for columns typed
// with v.string()/v.number() too.
func createConstantLiteral(d any) string {
	switch v := d.(type) {
	case string:
		if v == "$actor" {
			return "ownerId"
		}
		return tsDefaultLiteral(d) + " as const"
	case float64:
		return tsDefaultLiteral(d) + " as const"
	default:
		return tsDefaultLiteral(d)
	}
}

// hasVerb reports whether the resource exposes the given CRUD verb.
func hasVerb(verbs []string, want string) bool {
	for _, v := range verbs {
		if v == want {
			return true
		}
	}
	return false
}

// splitWritePath turns "model/vehicleWrites.applyVehicleUpdate" into the module
// path "model/vehicleWrites" and the function name "applyVehicleUpdate".
func splitWritePath(ref string) (module, fn string) {
	idx := strings.LastIndex(ref, ".")
	if idx < 0 {
		return ref, ref
	}
	return ref[:idx], ref[idx+1:]
}

// internalModulePath converts a module path like "vehicles/vehiclesApi.ts"
// into a Convex internal dotted reference body "vehicles.vehiclesApi".
func internalModulePath(module string) string {
	module = strings.TrimSuffix(module, ".ts")
	return strings.ReplaceAll(module, "/", ".")
}

// moduleDir returns the leading directory of the Api module path
// ("vehicles/vehiclesApi.ts" → "vehicles"), or "" when the module has no
// directory component.
func moduleDir(module string) string {
	module = strings.TrimSuffix(module, ".ts")
	if idx := strings.LastIndex(module, "/"); idx >= 0 {
		return module[:idx]
	}
	return ""
}

// mutationsModulePath returns the internal reference to the resource's create
// mutation module (e.g. internal.vehicles.vehicleMutations), placed beside the
// Api module and named "<singular>Mutations". An overlay MutationsModule
// overrides the conventional name (a module path relative to the Convex root,
// e.g. "maintenance/maintenanceApiMutations.ts") when the default would collide
// with a pre-existing, unrelated mutations file.
func mutationsModulePath(r ResolvedResource) string {
	if r.MutationsModule != "" {
		return "internal." + internalModulePath(r.MutationsModule)
	}
	lc := r.lcSingularName()
	dir := moduleDir(r.Module)
	if dir == "" {
		dir = r.Table
	}
	return "internal." + strings.ReplaceAll(dir, "/", ".") + "." + lc + "Mutations"
}

// apiTypesBasename returns the wire-types file basename (without extension),
// e.g. "vehicleApi.types" / "photoApi.types" / "gloveboxDocumentApi.types".
func apiTypesBasename(r ResolvedResource) string {
	return r.lcSingularName() + "Api.types"
}

// apiTypesSiblingImport returns the import specifier for the wire-types file
// relative to the Api module's own directory ("./types/vehicleApi.types").
func apiTypesSiblingImport(r ResolvedResource) string {
	return "./types/" + apiTypesBasename(r)
}

// apiTypesRouteImport returns the import specifier for the wire-types file from
// the routes file in convex/api ("../<domain>/types/vehicleApi.types"). The
// domain is the Api module's directory; it falls back to the table name when the
// module has no directory component.
func apiTypesRouteImport(r ResolvedResource) string {
	dir := moduleDir(r.Module)
	if dir == "" {
		dir = r.Table
	}
	return "../" + dir + "/types/" + apiTypesBasename(r)
}

// apiTypesFileRelPath returns the wire-types file path relative to the Convex
// root ("<domain>/types/vehicleApi.types.ts"), used by the orchestrator to write
// the file. The domain falls back to the table name when the module has no
// directory component.
func apiTypesFileRelPath(r ResolvedResource) string {
	dir := moduleDir(r.Module)
	if dir == "" {
		dir = r.Table
	}
	return dir + "/types/" + apiTypesBasename(r) + ".ts"
}

// routesFileBasename returns the routes file name written under convex/api. With
// no Name override it keeps the historical <table>Routes.ts convention (so the
// 9 existing resources are byte-stable); with an override (two resources sharing
// one table) it keys on the singular symbol base so the files do not collide.
func routesFileBasename(r ResolvedResource) string {
	if r.Singular != "" {
		return r.lcSingularName() + "Routes.ts"
	}
	return r.Table + "Routes.ts"
}

// apiModuleImportPath returns the import path (from the routes file in
// convex/api) to the resource's Api.ts module.
func apiModuleImportPath(r ResolvedResource) string {
	module := r.Module
	module = strings.TrimSuffix(module, ".ts")
	if module == "" {
		module = r.Table + "/" + r.lcSingularName() + "Api"
	}
	return "../" + module
}
