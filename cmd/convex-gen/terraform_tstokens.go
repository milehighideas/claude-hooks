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

// inputHasIDRef reports whether any input field is an id-reference (carries an
// explicit Id<"…"> scalar type), so the wire-types file imports Id.
func inputHasIDRef(r ResolvedResource) bool {
	for _, f := range r.InputFields {
		if f.TSScalar != "" {
			return true
		}
	}
	return false
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
// ownerId) instead of a JSON literal; all other values render via
// tsDefaultLiteral.
func createConstantLiteral(d any) string {
	if s, ok := d.(string); ok && s == "$actor" {
		return "ownerId"
	}
	return tsDefaultLiteral(d)
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
// Api module and named "<singular>Mutations".
func mutationsModulePath(r ResolvedResource) string {
	singular := singularize(r.Table)
	lc := strings.ToLower(singular[:1]) + singular[1:]
	dir := moduleDir(r.Module)
	if dir == "" {
		dir = r.Table
	}
	return "internal." + strings.ReplaceAll(dir, "/", ".") + "." + lc + "Mutations"
}

// lcSingular returns the lower-first-letter PascalCase singular of the table
// (e.g. "vehicles" → "vehicle"), used as the basename of the wire-types file.
func lcSingular(table string) string {
	singular := toPascalCase(singularize(table))
	return strings.ToLower(singular[:1]) + singular[1:]
}

// apiTypesBasename returns the wire-types file basename (without extension),
// e.g. "vehicleApi.types".
func apiTypesBasename(table string) string {
	return lcSingular(table) + "Api.types"
}

// apiTypesSiblingImport returns the import specifier for the wire-types file
// relative to the Api module's own directory ("./types/vehicleApi.types").
func apiTypesSiblingImport(table string) string {
	return "./types/" + apiTypesBasename(table)
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
	return "../" + dir + "/types/" + apiTypesBasename(r.Table)
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
	return dir + "/types/" + apiTypesBasename(r.Table) + ".ts"
}

// apiModuleImportPath returns the import path (from the routes file in
// convex/api) to the resource's Api.ts module.
func apiModuleImportPath(r ResolvedResource) string {
	module := r.Module
	module = strings.TrimSuffix(module, ".ts")
	if module == "" {
		lc := strings.ToLower(singularize(r.Table)[:1]) + singularize(r.Table)[1:]
		module = r.Table + "/" + lc + "Api"
	}
	return "../" + module
}
