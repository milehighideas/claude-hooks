package main

import (
	"fmt"
	"strings"
)

// EmitRoutesTS renders the full <res>Routes.ts source from a resolved resource.
func EmitRoutesTS(r ResolvedResource) string {
	var b strings.Builder
	singular := toPascalCase(singularize(r.Table)) // "Vehicle"
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
	b.WriteString("import { internal } from \"../_generated/api\";\n")
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
			conds = append(conds, fmt.Sprintf("typeof b.%s !== \"%s\"", f.Wire, jsTypeOf(f.TSType)))
			fields = append(fields, f.Wire+": b."+f.Wire)
		}
		fmt.Fprintf(&b, "  if (%s) return null;\n", strings.Join(conds, " || "))
		fmt.Fprintf(&b, "  return { %s, ...readPatch(b) };\n", strings.Join(fields, ", "))
	} else {
		b.WriteString("  return { ...readPatch(b) };\n")
	}
	b.WriteString("}\n\n")

	// readPatch
	b.WriteString("/** Read the curated, all-optional patch fields, ignoring unknown keys. */\n")
	fmt.Fprintf(&b, "export function readPatch(body: unknown): %sApiPatch {\n", singular)
	b.WriteString("  const b = (body ?? {}) as Record<string, unknown>;\n")
	fmt.Fprintf(&b, "  const patch: %sApiPatch = {};\n", singular)
	for _, f := range r.InputFields {
		fmt.Fprintf(&b, "  if (typeof b.%s === \"%s\") patch.%s = b.%s;\n", f.Wire, jsTypeOf(f.TSType), f.Wire, f.Wire)
	}
	b.WriteString("  return patch;\n}\n\n")

	// parseJson
	b.WriteString("async function parseJson(req: Request): Promise<unknown | undefined> {\n")
	b.WriteString("  try {\n    return await req.json();\n  } catch {\n    return undefined;\n  }\n}\n\n")

	// register fn
	notFoundMsg := capitalize(singularize(r.Table)) + " not found"
	notFoundLit, _ := jsonMarshalString(notFoundMsg)
	fmt.Fprintf(&b, "export function register%sRoutes(http: HttpRouter): void {\n", singular)

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
		fmt.Fprintf(&b, "      const id = await ctx.runMutation(\n        %s._create,\n        toCreateArgs(auth.actor.userId, input),\n      );\n", mutationsBase)
		fmt.Fprintf(&b, "      const %s = await ctx.runQuery(\n        %s.getForApi,\n        { actorId: auth.actor.userId, id },\n      );\n", lc, apiBase)
		fmt.Fprintf(&b, "      return jsonOk(%s, 201);\n", lc)
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

// requiredInputFields returns the non-optional exposed input fields, in order.
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
