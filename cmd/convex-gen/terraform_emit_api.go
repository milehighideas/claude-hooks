package main

import (
	"fmt"
	"sort"
	"strings"
)

// sortedComputed returns the resolved computed fields in a stable order (by wire
// name) so emitted output is deterministic regardless of map iteration order.
func sortedComputed(r ResolvedResource) []ResolvedComputed {
	c := make([]ResolvedComputed, len(r.Computed))
	copy(c, r.Computed)
	sort.Slice(c, func(i, j int) bool { return c[i].Wire < c[j].Wire })
	return c
}

// hasHatch reports whether any computed field uses the escape hatch (and so the
// projections module must be imported).
func hasHatch(r ResolvedResource) bool {
	for _, c := range r.Computed {
		if c.Hatch {
			return true
		}
	}
	return false
}

// hasR2 reports whether any computed field needs the R2 URL helper.
func hasR2(r ResolvedResource) bool {
	for _, c := range r.Computed {
		if c.As == "r2Urls" {
			return true
		}
	}
	return false
}

// EmitApiTypesTS renders the standalone wire-types file (<lcSingular>Api.types.ts)
// holding the plain-TS <Singular>ApiInput / <Singular>ApiPatch shapes. These are
// kept out of the Api module so the SRP gate's "type exports outside types/"
// check is satisfied. The shapes are primitive scalars only — no Id/Doc import.
func EmitApiTypesTS(r ResolvedResource) string {
	var b strings.Builder
	singular := toPascalCase(singularize(r.Table)) // "Vehicle"

	b.WriteString(generatedHeader())

	// Id-reference fields type as Id<"table"> and so need the dataModel Id import.
	// The types file lives at <domain>/types/, two levels below the Convex root.
	if inputHasIDRef(r) {
		b.WriteString("import type { Id } from \"../../_generated/dataModel\";\n\n")
	}

	fmt.Fprintf(&b, "export type %sApiInput = {\n", singular)
	for _, f := range r.InputFields {
		opt := ""
		if f.Optional {
			opt = "?"
		}
		b.WriteString("  " + f.Wire + opt + ": " + fieldScalarType(f) + ";\n")
	}
	b.WriteString("};\n\n")

	fmt.Fprintf(&b, "export type %sApiPatch = Partial<%sApiInput>;\n", singular, singular)
	return b.String()
}

// EmitApiTS renders the full <res>Api.ts source from a resolved resource.
func EmitApiTS(r ResolvedResource) string {
	var b strings.Builder
	singular := toPascalCase(singularize(r.Table)) // "Vehicle"
	lc := strings.ToLower(singular[:1]) + singular[1:]
	computed := sortedComputed(r)

	b.WriteString(generatedHeader())
	b.WriteString("import { v } from \"convex/values\";\n")
	b.WriteString("import { internalQuery } from \"../_generated/server\";\n")
	b.WriteString("import { internalMutation } from \"./" + lc + "Triggers\";\n")
	b.WriteString("import type { Doc, Id } from \"../_generated/dataModel\";\n")
	fmt.Fprintf(&b, "import type { %sApiInput, %sApiPatch } from \"%s\";\n", singular, singular, apiTypesSiblingImport(r.Table))
	if hasR2(r) {
		b.WriteString("import { getR2Url } from \"../r2/r2Utils\";\n")
	}
	if hasHatch(r) {
		b.WriteString("import * as projections from \"./" + lc + ".projections\";\n")
	}
	b.WriteString(emitWritePathImports(r))
	b.WriteString("\n")

	// <res>ApiInput
	fmt.Fprintf(&b, "export const %sApiInput = {\n", lc)
	for _, f := range r.InputFields {
		b.WriteString("  " + f.Wire + ": " + optional(f) + ",\n")
	}
	b.WriteString("};\n\n")

	// <res>ApiPatch (all optional)
	fmt.Fprintf(&b, "export const %sApiPatch = {\n", lc)
	for _, f := range r.InputFields {
		b.WriteString("  " + f.Wire + ": v.optional(" + f.TSType + "),\n")
	}
	b.WriteString("};\n\n")

	// <res>ApiOutput
	fmt.Fprintf(&b, "export const %sApiOutput = v.object({\n", lc)
	b.WriteString("  id: v.id(\"" + r.Table + "\"),\n")
	for _, f := range r.OutputFields {
		b.WriteString("  " + f.Wire + ": " + outputType(f) + ",\n")
	}
	for _, c := range computed {
		b.WriteString("  " + c.Wire + ": " + computedOutType(c) + ",\n")
	}
	b.WriteString("});\n\n")

	// toApi projection
	fmt.Fprintf(&b, "/** Project a %s row to the curated public shape. */\n", singularize(r.Table))
	fmt.Fprintf(&b, "export function toApi(doc: Doc<\"%s\">) {\n  return {\n", r.Table)
	b.WriteString("    id: doc._id,\n")
	for _, f := range r.OutputFields {
		expr := "doc." + f.Column
		if f.CoalesceDefault {
			expr += " ?? " + tsDefaultLiteral(f.OutputDefault)
		}
		b.WriteString("    " + f.Wire + ": " + expr + ",\n")
	}
	for _, c := range computed {
		b.WriteString("    " + c.Wire + ": " + computedExpr(c) + ",\n")
	}
	b.WriteString("  };\n}\n\n")

	b.WriteString(emitWriteHelpers(r, singular))
	b.WriteString(emitInternalFns(r, lc, singular))
	return b.String()
}

// emitWritePathImports imports any model write-path helpers referenced by the
// overlay's writePath map (update/delete delegation).
func emitWritePathImports(r ResolvedResource) string {
	if len(r.WritePath) == 0 {
		return ""
	}
	// Group functions by their module path so each module is imported once.
	byModule := map[string][]string{}
	order := []string{}
	for _, verb := range []string{"update", "delete"} {
		ref, ok := r.WritePath[verb]
		if !ok {
			continue
		}
		module, fn := splitWritePath(ref)
		if _, seen := byModule[module]; !seen {
			order = append(order, module)
		}
		byModule[module] = append(byModule[module], fn)
	}
	var b strings.Builder
	for _, module := range order {
		fmt.Fprintf(&b, "import { %s } from \"./%s\";\n", strings.Join(byModule[module], ", "), module)
	}
	return b.String()
}

// emitWriteHelpers renders toCreateArgs (filling defaults + ownership + write
// aliases) and toUpdatePatch (mapping each wire → its column, duplicating
// aliased columns).
func emitWriteHelpers(r ResolvedResource, singular string) string {
	var b strings.Builder

	// toCreateArgs
	fmt.Fprintf(&b, "/** Map curated API input to the full create argument set. */\n")
	fmt.Fprintf(&b, "export function toCreateArgs(ownerId: Id<\"users\">, input: %sApiInput) {\n", singular)
	b.WriteString("  return {\n")
	// Row-local ownership scopes the row by stamping ownerType + the owner column
	// with the actor. FK-via-parent ownership writes no owner column on the row
	// (authorization happens by loading the parent), so nothing is stamped here.
	if !r.Ownership.IsZero() && !r.Ownership.ViaParent() {
		b.WriteString("    ownerType: \"user\" as const,\n")
		b.WriteString("    " + r.Ownership.Field + ",\n")
	}
	for _, f := range r.InputFields {
		val := "input." + f.Wire
		if f.Default != nil {
			val = "input." + f.Wire + " ?? " + tsDefaultLiteral(f.Default)
		}
		// Primary column.
		b.WriteString("    " + f.Column + ": " + val + ",\n")
		// Extra write-alias columns (skip the primary if it appears first).
		for _, alias := range f.WriteAliases {
			if alias == f.Column {
				continue
			}
			b.WriteString("    " + alias + ": " + val + ",\n")
		}
	}
	// Constant, unexposed required _create args (sorted for stable output).
	if len(r.CreateConstants) > 0 {
		cols := make([]string, 0, len(r.CreateConstants))
		for col := range r.CreateConstants {
			cols = append(cols, col)
		}
		sort.Strings(cols)
		for _, col := range cols {
			b.WriteString("    " + col + ": " + createConstantLiteral(r.CreateConstants[col]) + ",\n")
		}
	}
	b.WriteString("    skipAuth: true,\n")
	b.WriteString("  };\n}\n\n")

	// toUpdatePatch
	fmt.Fprintf(&b, "/** Translate a curated PATCH into a table partial (only provided keys). */\n")
	fmt.Fprintf(&b, "export function toUpdatePatch(\n  input: %sApiPatch,\n): Partial<Doc<\"%s\">> {\n", singular, r.Table)
	fmt.Fprintf(&b, "  const patch: Partial<Doc<\"%s\">> = {};\n", r.Table)
	for _, f := range r.InputFields {
		cols := f.WriteAliases
		if len(cols) == 0 {
			cols = []string{f.Column}
		}
		if len(cols) == 1 {
			fmt.Fprintf(&b, "  if (input.%s !== undefined) patch.%s = input.%s;\n", f.Wire, cols[0], f.Wire)
			continue
		}
		fmt.Fprintf(&b, "  if (input.%s !== undefined) {\n", f.Wire)
		for _, col := range cols {
			fmt.Fprintf(&b, "    patch.%s = input.%s;\n", col, f.Wire)
		}
		b.WriteString("  }\n")
	}
	b.WriteString("  return patch;\n}\n\n")

	return b.String()
}

// emitInternalFns renders the ownership guard + getForApi/updateForApi/
// removeForApi, gated by the resource verbs, delegating writes to writePath
// helpers when set. The ownership guard is one of:
//   - row-local (string form): a synchronous owns<Singular>(doc, actorId).
//   - FK-via-parent (object form): an async owns<Singular>(ctx, doc, actorId)
//     that loads the parent row referenced by the FK column and checks its owner.
//   - none: handlers guard on !doc only.
func emitInternalFns(r ResolvedResource, lc, singular string) string {
	var b strings.Builder
	table := r.Table
	o := r.Ownership

	switch {
	case o.ViaParent():
		fmt.Fprintf(&b, "/** True when the API actor owns the parent %s of this %s. */\n", singularize(o.Table), singularize(table))
		fmt.Fprintf(&b, "async function owns%s(\n", singular)
		fmt.Fprintf(&b, "  ctx: {\n    db: {\n      get: (\n        t: \"%s\",\n        id: Id<\"%s\">,\n      ) => Promise<Doc<\"%s\"> | null>;\n    };\n  },\n", o.Table, o.Table, o.Table)
		fmt.Fprintf(&b, "  doc: Doc<\"%s\">,\n", table)
		b.WriteString("  actorId: Id<\"users\">,\n")
		b.WriteString("): Promise<boolean> {\n")
		fmt.Fprintf(&b, "  const parent = await ctx.db.get(\"%s\", doc.%s);\n", o.Table, r.OwnershipViaColumn)
		fmt.Fprintf(&b, "  return (\n    !!parent && parent.ownerType === \"user\" && parent.%s === actorId\n  );\n}\n\n", o.Field)
	case !o.IsZero():
		fmt.Fprintf(&b, "/** True when the API actor owns this user-owned %s. */\n", singularize(table))
		fmt.Fprintf(&b, "function owns%s(doc: Doc<\"%s\">, actorId: Id<\"users\">): boolean {\n", singular, table)
		fmt.Fprintf(&b, "  return doc.ownerType === \"user\" && doc.%s === actorId;\n}\n\n", o.Field)
	}

	// ownExpr builds the per-handler guard tail appended after `!doc`.
	ownExpr := func() string {
		switch {
		case o.ViaParent():
			return " || !(await owns" + singular + "(ctx, doc, args.actorId))"
		case !o.IsZero():
			return " || !owns" + singular + "(doc, args.actorId)"
		default:
			return ""
		}
	}
	ownGuard := ownExpr()

	if hasVerb(r.Verbs, "read") {
		b.WriteString("export const getForApi = internalQuery({\n")
		b.WriteString("  args: { actorId: v.id(\"users\"), id: v.id(\"" + table + "\") },\n")
		fmt.Fprintf(&b, "  returns: v.union(v.null(), %sApiOutput),\n", lc)
		b.WriteString("  handler: async (ctx, args) => {\n")
		b.WriteString("    const doc = await ctx.db.get(\"" + table + "\", args.id);\n")
		b.WriteString("    if (!doc" + ownGuard + ") return null;\n")
		b.WriteString("    return toApi(doc);\n")
		b.WriteString("  },\n});\n\n")
	}

	if hasVerb(r.Verbs, "update") {
		b.WriteString("export const updateForApi = internalMutation({\n")
		b.WriteString("  args: {\n")
		b.WriteString("    actorId: v.id(\"users\"),\n")
		b.WriteString("    id: v.id(\"" + table + "\"),\n")
		fmt.Fprintf(&b, "    data: v.object(%sApiPatch),\n", lc)
		b.WriteString("  },\n")
		fmt.Fprintf(&b, "  returns: v.union(v.null(), %sApiOutput),\n", lc)
		b.WriteString("  handler: async (ctx, args) => {\n")
		b.WriteString("    const doc = await ctx.db.get(\"" + table + "\", args.id);\n")
		b.WriteString("    if (!doc" + ownGuard + ") return null;\n\n")
		b.WriteString("    const patch = toUpdatePatch(args.data);\n")
		if ref, ok := r.WritePath["update"]; ok {
			_, fn := splitWritePath(ref)
			fmt.Fprintf(&b, "    await %s(ctx, args.id, doc, patch);\n\n", fn)
		} else {
			b.WriteString("    await ctx.db.patch(\"" + table + "\", args.id, patch);\n\n")
		}
		b.WriteString("    const updated = await ctx.db.get(\"" + table + "\", args.id);\n")
		b.WriteString("    return updated ? toApi(updated) : null;\n")
		b.WriteString("  },\n});\n\n")
	}

	if hasVerb(r.Verbs, "delete") {
		b.WriteString("export const removeForApi = internalMutation({\n")
		b.WriteString("  args: { actorId: v.id(\"users\"), id: v.id(\"" + table + "\") },\n")
		b.WriteString("  returns: v.object({ deleted: v.boolean() }),\n")
		b.WriteString("  handler: async (ctx, args) => {\n")
		b.WriteString("    const doc = await ctx.db.get(\"" + table + "\", args.id);\n")
		b.WriteString("    if (!doc" + ownGuard + ") return { deleted: false };\n")
		if ref, ok := r.WritePath["delete"]; ok {
			_, fn := splitWritePath(ref)
			fmt.Fprintf(&b, "    await %s(ctx, args.id, doc);\n", fn)
		} else {
			b.WriteString("    await ctx.db.delete(\"" + table + "\", args.id);\n")
		}
		b.WriteString("    return { deleted: true };\n")
		b.WriteString("  },\n});\n")
	}

	return b.String()
}
