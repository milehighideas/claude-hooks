package main

import (
	"fmt"
	"strings"
)

// EmitRoutesTS renders the full <res>Routes.ts source from a resolved resource.
func EmitRoutesTS(r ResolvedResource) string {
	var b strings.Builder
	singular := r.pascalSingular() // symbol base ("Vehicle", "Photo", "GloveboxDocument")
	lc := strings.ToLower(singular[:1]) + singular[1:]
	prefix := "/api/v1/" + r.Path + "/"
	collection := "/api/v1/" + r.Path
	apiBase := "internal." + internalModulePath(r.Module) // e.g. internal.vehicles.vehiclesApi
	mutationsBase := mutationsModulePath(r)               // e.g. internal.vehicles.vehicleMutations
	apiModulePath := apiModuleImportPath(r)

	readScope := r.Path + ":read"
	writeScope := r.Path + ":write"

	b.WriteString(generatedHeader())
	b.WriteString("import type { HttpRouter } from \"convex/server\";\n")
	b.WriteString("import { httpAction } from \"../_generated/server\";\n")
	// A media (presign) resource calls a public R2 action via `api`, so it imports
	// both `internal` (the CRUD fns + ownership check) and `api`.
	if hasMedia(r) {
		b.WriteString("import { internal, api } from \"../_generated/api\";\n")
	} else {
		b.WriteString("import { internal } from \"../_generated/api\";\n")
	}
	b.WriteString("import type { Id } from \"../_generated/dataModel\";\n")
	b.WriteString("import { authorize, jsonError, jsonOk } from \"./apiAuth\";\n")
	fmt.Fprintf(&b, "import { toCreateArgs } from \"%s\";\n", apiModulePath)
	fmt.Fprintf(&b, "import type {\n  %sApiInput,\n  %sApiPatch,\n} from \"%s\";\n\n", singular, singular, apiTypesRouteImport(r))

	fmt.Fprintf(&b, "const PREFIX = \"%s\";\n\n", prefix)

	// idFromPath
	b.WriteString("/** Last path segment as the resource id, or null for the collection root. */\n")
	b.WriteString("export function idFromPath(url: string): string | null {\n")
	b.WriteString("  const id = new URL(url).pathname.slice(PREFIX.length).split(\"/\")[0];\n")
	b.WriteString("  return id && id.length > 0 ? id : null;\n}\n\n")

	// readInput (depends on required fields)
	required := requiredInputFields(r)
	fmt.Fprintf(&b, "/** Read curated create input from a JSON body; null if required fields are missing. */\n")
	fmt.Fprintf(&b, "export function readInput(body: unknown): %sApiInput | null {\n", singular)
	b.WriteString("  if (typeof body !== \"object\" || body === null) return null;\n")
	b.WriteString("  const b = body as Record<string, unknown>;\n")
	if len(required) > 0 {
		var conds, fields []string
		for _, f := range required {
			cast := singular + "ApiInput[\"" + f.Wire + "\"]"
			// An ARRAY-valued field (nested array-of-objects OR a top-level id-array)
			// is guarded with Array.isArray and cast to the indexed wire type.
			if fieldIsArrayWire(f) {
				conds = append(conds, fmt.Sprintf("!Array.isArray(b.%s)", f.Wire))
				fields = append(fields, f.Wire+": b."+f.Wire+" as "+cast)
				continue
			}
			// An OBJECT-valued field (single nested object OR a fixed-key map) is
			// guarded with typeof object and cast to the indexed wire type.
			if fieldIsObjectWire(f) {
				conds = append(conds, fmt.Sprintf("typeof b.%s !== \"object\" || b.%s === null || Array.isArray(b.%s)", f.Wire, f.Wire, f.Wire))
				fields = append(fields, f.Wire+": b."+f.Wire+" as "+cast)
				continue
			}
			conds = append(conds, fmt.Sprintf("typeof b.%s !== \"%s\"", f.Wire, jsTypeOf(f.TSType)))
			// An id-reference field is read as a string from the body but typed
			// Id<"…"> on the input shape, so it needs an explicit cast.
			if f.TSScalar != "" {
				fields = append(fields, f.Wire+": b."+f.Wire+" as "+f.TSScalar)
			} else {
				fields = append(fields, f.Wire+": b."+f.Wire)
			}
		}
		fmt.Fprintf(&b, "  if (%s) return null;\n", strings.Join(conds, " || "))
		fmt.Fprintf(&b, "  return { %s, ...%s };\n", strings.Join(fields, ", "), readPatchSpread(r, singular))
	} else {
		fmt.Fprintf(&b, "  return { ...%s };\n", readPatchSpread(r, singular))
	}
	b.WriteString("}\n\n")

	// readPatch
	b.WriteString("/** Read the curated, all-optional patch fields, ignoring unknown keys. */\n")
	fmt.Fprintf(&b, "export function readPatch(body: unknown): %sApiPatch {\n", singular)
	b.WriteString("  const b = (body ?? {}) as Record<string, unknown>;\n")
	fmt.Fprintf(&b, "  const patch: %sApiPatch = {};\n", singular)
	for _, f := range patchFields(r) {
		// A typed nested / map / id-array field arrives as one JSON value (array or
		// object), not a scalar: guard with Array.isArray / typeof object and cast
		// to the indexed wire type rather than a typeof-string check.
		cast := singular + "ApiInput[\"" + f.Wire + "\"]"
		if fieldIsArrayWire(f) {
			fmt.Fprintf(&b, "  if (Array.isArray(b.%s)) patch.%s = b.%s as %s;\n", f.Wire, f.Wire, f.Wire, cast)
			continue
		}
		if fieldIsObjectWire(f) {
			fmt.Fprintf(&b, "  if (typeof b.%s === \"object\" && b.%s !== null && !Array.isArray(b.%s)) patch.%s = b.%s as %s;\n", f.Wire, f.Wire, f.Wire, f.Wire, f.Wire, cast)
			continue
		}
		// Enum and id-reference fields are read as a primitive from the body but
		// typed as a string-union / Id<"…"> on the patch shape, so they need an
		// explicit cast (mirrors readInput). A clearable id-ref widens its patch
		// scalar to Id<"…"> | "", so cast to the patch scalar when it differs.
		if scalar := patchScalarType(f); f.TSScalar != "" || f.PatchTSScalar != "" {
			fmt.Fprintf(&b, "  if (typeof b.%s === \"%s\") patch.%s = b.%s as %s;\n", f.Wire, jsTypeOf(f.TSType), f.Wire, f.Wire, scalar)
			continue
		}
		fmt.Fprintf(&b, "  if (typeof b.%s === \"%s\") patch.%s = b.%s;\n", f.Wire, jsTypeOf(f.TSType), f.Wire, f.Wire)
	}
	b.WriteString("  return patch;\n}\n\n")

	// parseJson
	b.WriteString("async function parseJson(req: Request): Promise<unknown | undefined> {\n")
	b.WriteString("  try {\n    return await req.json();\n  } catch {\n    return undefined;\n  }\n}\n\n")

	// extFromContentType — a media resource maps the optional content_type hint to
	// a file extension for the R2 presign action.
	if hasMedia(r) {
		b.WriteString(emitExtFromContentType())
	}

	// register fn
	notFoundMsg := capitalize(r.singular()) + " not found"
	notFoundLit, _ := jsonMarshalString(notFoundMsg)
	fmt.Fprintf(&b, "export function register%sRoutes(http: HttpRouter): void {\n", singular)

	// PRESIGN — the media escape-hatch route, emitted before CREATE so the more
	// specific `/presign` path reads first.
	if hasMedia(r) {
		b.WriteString(emitPresignRoute(r, collection, writeScope))
	}

	if hasVerb(r.Verbs, "create") {
		b.WriteString("  // CREATE\n")
		b.WriteString("  http.route({\n")
		fmt.Fprintf(&b, "    path: \"%s\",\n", collection)
		b.WriteString("    method: \"POST\",\n")
		b.WriteString("    handler: httpAction(async (ctx, req) => {\n")
		fmt.Fprintf(&b, "      const auth = await authorize(ctx, req, \"%s\");\n", writeScope)
		b.WriteString("      if (\"error\" in auth) return auth.error;\n\n")
		b.WriteString("      const body = await parseJson(req);\n")
		b.WriteString("      if (body === undefined) {\n")
		b.WriteString("        return jsonError(400, \"bad_json\", \"Request body must be valid JSON\");\n      }\n")
		b.WriteString("      const input = readInput(body);\n")
		b.WriteString("      if (!input) {\n")
		fmt.Fprintf(&b, "        return jsonError(422, \"validation\", %s);\n      }\n\n", validationMsg(required))
		// Wrap the create mutation + read-back so an ownership/not-found
		// ConvexError (e.g. the actor does not own the referenced parent) returns
		// a clean 404 instead of bubbling out as a 500.
		b.WriteString("      try {\n")
		fmt.Fprintf(&b, "        const id = await ctx.runMutation(\n          %s._create,\n          toCreateArgs(auth.actor.userId, input),\n        );\n", mutationsBase)
		fmt.Fprintf(&b, "        const %s = await ctx.runQuery(\n          %s.getForApi,\n          { actorId: auth.actor.userId, id },\n        );\n", lc, apiBase)
		fmt.Fprintf(&b, "        return jsonOk(%s, 201);\n", lc)
		fmt.Fprintf(&b, "      } catch {\n        return jsonError(404, \"not_found\", %s);\n      }\n", notFoundLit)
		b.WriteString("    }),\n  });\n\n")
	}

	if hasVerb(r.Verbs, "read") {
		b.WriteString("  // READ\n")
		b.WriteString("  http.route({\n")
		b.WriteString("    pathPrefix: PREFIX,\n")
		b.WriteString("    method: \"GET\",\n")
		b.WriteString("    handler: httpAction(async (ctx, req) => {\n")
		fmt.Fprintf(&b, "      const auth = await authorize(ctx, req, \"%s\");\n", readScope)
		b.WriteString("      if (\"error\" in auth) return auth.error;\n\n")
		b.WriteString("      const id = idFromPath(req.url);\n")
		fmt.Fprintf(&b, "      if (!id) return jsonError(404, \"not_found\", %s);\n\n", notFoundLit)
		b.WriteString("      try {\n")
		fmt.Fprintf(&b, "        const %s = await ctx.runQuery(\n          %s.getForApi,\n          { actorId: auth.actor.userId, id: id as Id<\"%s\"> },\n        );\n", lc, apiBase, r.Table)
		fmt.Fprintf(&b, "        return %s\n          ? jsonOk(%s)\n          : jsonError(404, \"not_found\", %s);\n", lc, lc, notFoundLit)
		fmt.Fprintf(&b, "      } catch {\n        return jsonError(404, \"not_found\", %s);\n      }\n", notFoundLit)
		b.WriteString("    }),\n  });\n\n")
	}

	if hasVerb(r.Verbs, "update") {
		b.WriteString("  // UPDATE\n")
		b.WriteString("  http.route({\n")
		b.WriteString("    pathPrefix: PREFIX,\n")
		b.WriteString("    method: \"PATCH\",\n")
		b.WriteString("    handler: httpAction(async (ctx, req) => {\n")
		fmt.Fprintf(&b, "      const auth = await authorize(ctx, req, \"%s\");\n", writeScope)
		b.WriteString("      if (\"error\" in auth) return auth.error;\n\n")
		b.WriteString("      const id = idFromPath(req.url);\n")
		fmt.Fprintf(&b, "      if (!id) return jsonError(404, \"not_found\", %s);\n\n", notFoundLit)
		b.WriteString("      const body = await parseJson(req);\n")
		b.WriteString("      if (body === undefined) {\n")
		b.WriteString("        return jsonError(400, \"bad_json\", \"Request body must be valid JSON\");\n      }\n\n")
		b.WriteString("      try {\n")
		fmt.Fprintf(&b, "        const %s = await ctx.runMutation(\n          %s.updateForApi,\n          {\n            actorId: auth.actor.userId,\n            id: id as Id<\"%s\">,\n            data: readPatch(body),\n          },\n        );\n", lc, apiBase, r.Table)
		fmt.Fprintf(&b, "        return %s\n          ? jsonOk(%s)\n          : jsonError(404, \"not_found\", %s);\n", lc, lc, notFoundLit)
		fmt.Fprintf(&b, "      } catch {\n        return jsonError(404, \"not_found\", %s);\n      }\n", notFoundLit)
		b.WriteString("    }),\n  });\n\n")
	}

	if hasVerb(r.Verbs, "delete") {
		b.WriteString("  // DELETE\n")
		b.WriteString("  http.route({\n")
		b.WriteString("    pathPrefix: PREFIX,\n")
		b.WriteString("    method: \"DELETE\",\n")
		b.WriteString("    handler: httpAction(async (ctx, req) => {\n")
		fmt.Fprintf(&b, "      const auth = await authorize(ctx, req, \"%s\");\n", writeScope)
		b.WriteString("      if (\"error\" in auth) return auth.error;\n\n")
		b.WriteString("      const id = idFromPath(req.url);\n")
		fmt.Fprintf(&b, "      if (!id) return jsonError(404, \"not_found\", %s);\n\n", notFoundLit)
		b.WriteString("      try {\n")
		fmt.Fprintf(&b, "        const { deleted } = await ctx.runMutation(\n          %s.removeForApi,\n          { actorId: auth.actor.userId, id: id as Id<\"%s\"> },\n        );\n", apiBase, r.Table)
		fmt.Fprintf(&b, "        return deleted\n          ? new Response(null, { status: 204 })\n          : jsonError(404, \"not_found\", %s);\n", notFoundLit)
		fmt.Fprintf(&b, "      } catch {\n        return jsonError(404, \"not_found\", %s);\n      }\n", notFoundLit)
		b.WriteString("    }),\n  });\n")
	}

	b.WriteString("}\n")
	return b.String()
}

// readPatchSpread renders the readPatch(b) call used to fill optional fields in
// readInput. When a patch field has a widened PATCH scalar (a clearable id-ref
// admits ""), the patch shape is not assignable to the create-input shape, so
// the spread is cast to Partial<<Singular>ApiInput> (the "" clear sentinel is
// meaningless on create and never produced by a real client there).
func readPatchSpread(r ResolvedResource, singular string) string {
	for _, f := range patchFields(r) {
		if f.PatchTSScalar != "" {
			return "(readPatch(b) as Partial<" + singular + "ApiInput>)"
		}
	}
	return "readPatch(b)"
}

// hasMedia reports whether the resource opts into the presign media route.
func hasMedia(r ResolvedResource) bool {
	return r.Media != nil && r.Media.Presign
}

// mediaFKFields returns the resource's id-reference (FK) input fields, in order.
// These are the foreign keys the presign route validates and forwards to the
// ownership check (e.g. vehicle_id + album_id for photos). The key column is a
// plain string, so it is never included here.
func mediaFKFields(r ResolvedResource) []ResolvedField {
	var out []ResolvedField
	for _, f := range r.InputFields {
		if strings.HasPrefix(f.TSScalar, "Id<") {
			out = append(out, f)
		}
	}
	return out
}

// mediaEntityIDField returns the FK field carrying the R2 entity id (the parent
// the upload is filed under) — the FK whose referenced table singular matches
// media.entityType (e.g. entityType "vehicle" → the vehicles FK). Falls back to
// the first FK field when none matches.
func mediaEntityIDField(r ResolvedResource) ResolvedField {
	fks := mediaFKFields(r)
	for _, f := range fks {
		// TSScalar is Id<"vehicles">; compare the referenced table singular.
		ref := strings.TrimSuffix(strings.TrimPrefix(f.TSScalar, "Id<\""), "\">")
		if ref != "" && singularize(ref) == singularize(r.Media.EntityType+"s") {
			return f
		}
		if singularize(ref) == r.Media.EntityType {
			return f
		}
	}
	if len(fks) > 0 {
		return fks[0]
	}
	return ResolvedField{}
}

// emitExtFromContentType renders the content_type → file-extension mapper the
// presign route uses to name the R2 object.
func emitExtFromContentType() string {
	return "/** Map an optional content_type hint to a file extension for R2. */\n" +
		"function extFromContentType(ct: string | undefined): string {\n" +
		"  switch ((ct ?? \"\").toLowerCase()) {\n" +
		"    case \"image/png\":\n      return \"png\";\n" +
		"    case \"image/webp\":\n      return \"webp\";\n" +
		"    case \"image/gif\":\n      return \"gif\";\n" +
		"    case \"image/heic\":\n      return \"heic\";\n" +
		"    case \"application/pdf\":\n      return \"pdf\";\n" +
		"    case \"video/mp4\":\n      return \"mp4\";\n" +
		"    case \"image/jpeg\":\n    case \"image/jpg\":\n    default:\n      return \"jpg\";\n" +
		"  }\n}\n\n"
}

// emitPresignRoute renders the POST /api/v1/<path>/presign companion route: it
// authorizes write, validates the FK fields, calls the hand-written ownership
// check, runs the shared R2 presign action with the overlay's concrete
// entity/media constants, and returns { upload_url, key } (+ bucket when set).
func emitPresignRoute(r ResolvedResource, collection, writeScope string) string {
	var b strings.Builder
	m := r.Media
	fks := mediaFKFields(r)
	entity := mediaEntityIDField(r)
	// internalModulePath turns "r2/genericStructuredUpload.generateEntityUploadUrl"
	// into "r2.genericStructuredUpload.generateEntityUploadUrl" (slashes → dots).
	checkRef := "internal." + internalModulePath(m.OwnershipCheck)
	actionRef := "api." + internalModulePath(m.R2Action)

	b.WriteString("  // PRESIGN — returns a signed R2 upload URL + key for a direct client PUT.\n")
	b.WriteString("  http.route({\n")
	fmt.Fprintf(&b, "    path: \"%s/presign\",\n", collection)
	b.WriteString("    method: \"POST\",\n")
	b.WriteString("    handler: httpAction(async (ctx, req) => {\n")
	fmt.Fprintf(&b, "      const auth = await authorize(ctx, req, \"%s\");\n", writeScope)
	b.WriteString("      if (\"error\" in auth) return auth.error;\n\n")
	b.WriteString("      const body = await parseJson(req);\n")
	b.WriteString("      if (typeof body !== \"object\" || body === null) {\n")
	b.WriteString("        return jsonError(400, \"bad_json\", \"Request body must be valid JSON\");\n      }\n")
	b.WriteString("      const b = body as Record<string, unknown>;\n")

	// FK presence validation.
	conds := make([]string, len(fks))
	names := make([]string, len(fks))
	for i, f := range fks {
		conds[i] = "typeof b." + f.Wire + " !== \"string\""
		names[i] = "`" + f.Wire + "`"
	}
	if len(fks) > 0 {
		fmt.Fprintf(&b, "      if (%s) {\n", strings.Join(conds, " || "))
		fmt.Fprintf(&b, "        return jsonError(422, \"validation\", %s);\n      }\n\n", validationListLit(names))
	} else {
		b.WriteString("\n")
	}

	// Ownership check.
	fmt.Fprintf(&b, "      const ok = await ctx.runQuery(%s, {\n", checkRef)
	b.WriteString("        actorId: auth.actor.userId,\n")
	for _, f := range fks {
		fmt.Fprintf(&b, "        %s: b.%s as %s,\n", f.Column, f.Wire, f.TSScalar)
	}
	b.WriteString("      });\n")
	b.WriteString("      if (!ok) return jsonError(404, \"not_found\", \"Not found\");\n\n")

	// R2 presign action.
	fmt.Fprintf(&b, "      const result = await ctx.runAction(%s, {\n", actionRef)
	fmt.Fprintf(&b, "        entityType: \"%s\",\n", m.EntityType)
	if entity.Wire != "" {
		fmt.Fprintf(&b, "        entityId: b.%s,\n", entity.Wire)
	}
	fmt.Fprintf(&b, "        mediaType: \"%s\",\n", m.MediaType)
	b.WriteString("        fileExtension: extFromContentType(\n")
	b.WriteString("          typeof b.content_type === \"string\" ? b.content_type : undefined,\n")
	b.WriteString("        ),\n")
	fmt.Fprintf(&b, "        isPublic: %t,\n", m.IsPublic)
	b.WriteString("      });\n")

	// Response.
	if m.BucketField != "" {
		b.WriteString("      return jsonOk({\n        upload_url: result.uploadUrl,\n        key: result.key,\n        bucket: result.bucket,\n      });\n")
	} else {
		b.WriteString("      return jsonOk({ upload_url: result.uploadUrl, key: result.key });\n")
	}
	b.WriteString("    }),\n  });\n\n")
	return b.String()
}

// validationListLit renders a 422 message listing the required wire names with
// the same `a` and `b` / `a, b, and c` joining as validationMsg.
func validationListLit(names []string) string {
	var joined, verb string
	switch len(names) {
	case 0:
		joined = "Invalid request body"
		lit, _ := jsonMarshalString(joined)
		return lit
	case 1:
		joined, verb = names[0], " is required"
	case 2:
		joined, verb = names[0]+" and "+names[1], " are required"
	default:
		joined = strings.Join(names[:len(names)-1], ", ") + ", and " + names[len(names)-1]
		verb = " are required"
	}
	lit, _ := jsonMarshalString(joined + verb)
	return lit
}

// requiredInputFields returns the non-optional exposed input fields, in order.
// fieldIsArrayWire reports whether a field arrives as a JSON ARRAY (guard with
// Array.isArray): a nested array-of-objects OR a top-level id-array (Id<…>[]).
func fieldIsArrayWire(f ResolvedField) bool {
	if len(f.Nested) > 0 {
		return !f.NestedSingle
	}
	return strings.HasPrefix(f.TSType, "v.array(")
}

// fieldIsObjectWire reports whether a field arrives as a JSON OBJECT (guard with
// typeof object): a single nested object OR a fixed-key map.
func fieldIsObjectWire(f ResolvedField) bool {
	return (len(f.Nested) > 0 && f.NestedSingle) || f.Map != nil
}

func requiredInputFields(r ResolvedResource) []ResolvedField {
	var out []ResolvedField
	for _, f := range r.InputFields {
		if !f.Optional {
			out = append(out, f)
		}
	}
	return out
}

// validationMsg builds the 422 validation message listing the required wire
// fields ("`make` and `model` are required").
func validationMsg(required []ResolvedField) string {
	if len(required) == 0 {
		lit, _ := jsonMarshalString("Invalid request body")
		return lit
	}
	names := make([]string, len(required))
	for i, f := range required {
		names[i] = "`" + f.Wire + "`"
	}
	var joined string
	switch len(names) {
	case 1:
		joined = names[0]
	case 2:
		joined = names[0] + " and " + names[1]
	default:
		joined = strings.Join(names[:len(names)-1], ", ") + ", and " + names[len(names)-1]
	}
	lit, _ := jsonMarshalString(joined + " are required")
	return lit
}
