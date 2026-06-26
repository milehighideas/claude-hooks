package main

import (
	"fmt"
	"sort"
	"strings"
)

// patchFields returns the writable fields that participate in the PATCH surface:
// every input field except create-only immutable ones (FK references).
func patchFields(r ResolvedResource) []ResolvedField {
	var out []ResolvedField
	for _, f := range r.InputFields {
		if f.Immutable {
			continue
		}
		out = append(out, f)
	}
	return out
}

// immutableInputFields returns the create-only immutable input fields (FK refs).
func immutableInputFields(r ResolvedResource) []ResolvedField {
	var out []ResolvedField
	for _, f := range r.InputFields {
		if f.Immutable {
			out = append(out, f)
		}
	}
	return out
}

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
		if c.As == "r2Urls" || c.As == "r2Url" {
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
	singular := r.pascalSingular() // symbol base ("Vehicle", "Photo", "GloveboxDocument")

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

	// Create-only immutable FK refs are excluded from the PATCH type so a child
	// cannot be reparented through an update. Fields whose PATCH scalar differs
	// from the input scalar (clearable id-refs widened with "") are also omitted
	// from the Partial<Input> base and re-declared in an intersected object.
	imm := immutableInputFields(r)
	var overridden []ResolvedField
	for _, f := range patchFields(r) {
		if f.PatchTSScalar != "" {
			overridden = append(overridden, f)
		}
	}

	omits := make([]string, 0, len(imm)+len(overridden))
	for _, f := range imm {
		omits = append(omits, "\""+f.Wire+"\"")
	}
	for _, f := range overridden {
		omits = append(omits, "\""+f.Wire+"\"")
	}

	base := singular + "ApiInput"
	if len(omits) > 0 {
		base = "Omit<" + singular + "ApiInput, " + strings.Join(omits, " | ") + ">"
	}
	patchType := "Partial<" + base + ">"
	if len(overridden) > 0 {
		parts := make([]string, len(overridden))
		for i, f := range overridden {
			parts[i] = f.Wire + "?: " + patchScalarType(f)
		}
		patchType += " & { " + strings.Join(parts, "; ") + " }"
	}
	fmt.Fprintf(&b, "export type %sApiPatch = %s;\n", singular, patchType)
	return b.String()
}

// EmitApiTS renders the full <res>Api.ts source from a resolved resource.
func EmitApiTS(r ResolvedResource) string {
	var b strings.Builder
	singular := r.pascalSingular() // symbol base ("Vehicle", "Photo", "GloveboxDocument")
	lc := strings.ToLower(singular[:1]) + singular[1:]
	computed := sortedComputed(r)

	b.WriteString(generatedHeader())
	b.WriteString("import { v } from \"convex/values\";\n")
	b.WriteString("import { internalQuery } from \"../_generated/server\";\n")
	b.WriteString("import { internalMutation } from \"./" + lc + "Triggers\";\n")
	b.WriteString("import type { Doc, Id } from \"../_generated/dataModel\";\n")
	fmt.Fprintf(&b, "import type { %sApiInput, %sApiPatch } from \"%s\";\n", singular, singular, apiTypesSiblingImport(r))
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

	// <res>ApiPatch (all optional; create-only immutable FK refs are excluded)
	fmt.Fprintf(&b, "export const %sApiPatch = {\n", lc)
	for _, f := range patchFields(r) {
		b.WriteString("  " + f.Wire + ": v.optional(" + patchValidatorToken(f) + "),\n")
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
		if len(f.Nested) > 0 {
			b.WriteString(nestedToApiExpr(f))
			continue
		}
		if f.Map != nil {
			b.WriteString(mapToApiExpr(f))
			continue
		}
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

// nestedToApiExpr renders the toApi projection line for a typed nested field,
// mapping each DB camelCase column back to its public wire key. The array form
// coalesces a missing column to [] so the output is always an array; the single
// form reads each sub-field straight off `doc.<col>`.
func nestedToApiExpr(f ResolvedField) string {
	if f.NestedSingle {
		// Single object: project key-by-key off doc.<col> when present, else undefined.
		body := emitNestedObjectMap(f.Nested, "doc."+f.Column, true)
		return fmt.Sprintf(
			"    %s: doc.%s\n      ? {\n%s        }\n      : undefined,\n",
			f.Wire, f.Column, indentMap(body, "  "),
		)
	}
	body := emitNestedObjectMap(f.Nested, "item", true)
	return fmt.Sprintf(
		"    %s: (doc.%s ?? []).map((item) => ({\n%s    })),\n",
		f.Wire, f.Column, body,
	)
}

// nestedCreateExpr renders the toCreateArgs value for a typed nested field,
// mapping each public wire key to its DB camelCase column. The array form
// coalesces a missing input to []; the single form reads off `input.<wire>`.
func nestedCreateExpr(f ResolvedField) string {
	if f.NestedSingle {
		body := emitNestedObjectMap(f.Nested, "input."+f.Wire, false)
		return fmt.Sprintf(
			"    %s: input.%s\n      ? {\n%s        }\n      : undefined,\n",
			f.Column, f.Wire, indentMap(body, "  "),
		)
	}
	body := emitNestedObjectMap(f.Nested, "item", false)
	return fmt.Sprintf(
		"    %s: (input.%s ?? []).map((item) => ({\n%s    })),\n",
		f.Column, f.Wire, body,
	)
}

// nestedPatchAssign renders the toUpdatePatch block for a typed nested field: a
// presence guard plus the wire→DB mapped assignment. The single form reads each
// sub-field off `input.<wire>`; the array form maps over the array.
func nestedPatchAssign(f ResolvedField) string {
	if f.NestedSingle {
		body := emitNestedObjectMap(f.Nested, "input."+f.Wire, false)
		return fmt.Sprintf(
			"  if (input.%s !== undefined) {\n    patch.%s = {\n%s    };\n  }\n",
			f.Wire, f.Column, indentMap(body, ""),
		)
	}
	body := emitNestedObjectMap(f.Nested, "item", false)
	return fmt.Sprintf(
		"  if (input.%s !== undefined) {\n    patch.%s = input.%s.map((item) => ({\n%s    }));\n  }\n",
		f.Wire, f.Column, f.Wire, body,
	)
}

// mapValueObject renders one tab value's snake↔camel object literal as a single
// inline `{ a: src.b, … }` (no trailing newlines), reading from accessor `src`.
// wireToDb selects the direction (toApi reads db→wire; write helpers wire→db).
func mapValueObject(fields []NestedField, src string, wireToDb bool) string {
	parts := make([]string, len(fields))
	for i, f := range fields {
		dst, sub := f.Key, nestedDbColumn(f.Key)
		if !wireToDb {
			dst, sub = sub, f.Key
		}
		parts[i] = dst + ": " + src + "." + sub + nestedReadNarrowing(f, wireToDb)
	}
	return "{ " + strings.Join(parts, ", ") + " }"
}

// mapObjectLiteral renders the full fixed-key map object literal at the given
// indent: each key is present-guarded and its inner object snake↔camel remapped.
//
//	{
//	  timeline: <root>.timeline ? <obj> : undefined,
//	  build:    <root>.build    ? <obj> : undefined,
//	}
//
// root is the accessor for the whole map (e.g. "doc.tabPrivacy"); the result is a
// `{ … }` expression with no leading indent on its first brace.
func mapObjectLiteral(m *ObjectMapShape, root, indent string, wireToDb bool) string {
	var b strings.Builder
	b.WriteString("{\n")
	for _, k := range m.Keys {
		acc := root + "." + k
		obj := mapValueObject(m.Object, acc, wireToDb)
		fmt.Fprintf(&b, "%s  %s: %s ? %s : undefined,\n", indent, k, acc, obj)
	}
	b.WriteString(indent + "}")
	return b.String()
}

// mapToApiExpr renders the toApi projection line for a fixed-key map field. The
// whole map is present-guarded (schema-optional → undefined when absent).
func mapToApiExpr(f ResolvedField) string {
	root := "doc." + f.Column
	body := mapObjectLiteral(f.Map, root, "      ", true)
	return fmt.Sprintf("    %s: %s\n      ? %s\n      : undefined,\n", f.Wire, root, body)
}

// mapCreateExpr renders the toCreateArgs value for a fixed-key map field.
func mapCreateExpr(f ResolvedField) string {
	root := "input." + f.Wire
	body := mapObjectLiteral(f.Map, root, "      ", false)
	return fmt.Sprintf("    %s: %s\n      ? %s\n      : undefined,\n", f.Column, root, body)
}

// mapPatchAssign renders the toUpdatePatch block for a fixed-key map field: a
// presence guard plus the wire→db remapped assignment.
func mapPatchAssign(f ResolvedField) string {
	root := "input." + f.Wire
	body := mapObjectLiteral(f.Map, root, "    ", false)
	return fmt.Sprintf("  if (%s !== undefined) {\n    patch.%s = %s;\n  }\n", root, f.Column, body)
}

// indentMap re-indents the lines produced by emitNestedObjectMap by an extra
// prefix (used to nest a single-object body deeper than the array form).
func indentMap(body, extra string) string {
	if extra == "" {
		return body
	}
	lines := strings.Split(strings.TrimRight(body, "\n"), "\n")
	for i, l := range lines {
		if l != "" {
			lines[i] = extra + l
		}
	}
	return strings.Join(lines, "\n") + "\n"
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

// createArgsUsesOwner reports whether toCreateArgs references its actor (ownerId)
// param: either row-local ownership stamps it, or a "$actor" create constant
// injects it. FK-via-parent resources with no $actor constant never read it, so
// the param is named "_ownerId" to satisfy noUnusedParameters.
func createArgsUsesOwner(r ResolvedResource) bool {
	if !r.Ownership.IsZero() {
		// Row-local stamps the owner column; FK-via-parent forwards the actor as
		// actorId so the create mutation can authorize against the parent row.
		return true
	}
	for _, v := range r.CreateConstants {
		if s, ok := v.(string); ok && s == "$actor" {
			return true
		}
	}
	return false
}

// emitWriteHelpers renders toCreateArgs (filling defaults + ownership + write
// aliases) and toUpdatePatch (mapping each wire → its column, duplicating
// aliased columns).
func emitWriteHelpers(r ResolvedResource, singular string) string {
	var b strings.Builder

	// toCreateArgs
	ownerParam := "ownerId"
	if !createArgsUsesOwner(r) {
		ownerParam = "_ownerId"
	}
	fmt.Fprintf(&b, "/** Map curated API input to the full create argument set. */\n")
	fmt.Fprintf(&b, "export function toCreateArgs(%s: Id<\"users\">, input: %sApiInput) {\n", ownerParam, singular)
	b.WriteString("  return {\n")
	// Row-local ownership scopes the row by stamping ownerType + the owner column
	// with the actor. FK-via-parent ownership writes no owner column on the row
	// (authorization happens by loading the parent), so nothing is stamped here.
	if !r.Ownership.IsZero() && !r.Ownership.ViaParent() {
		b.WriteString("    ownerType: \"user\" as const,\n")
		b.WriteString("    " + r.Ownership.Field + ",\n")
	}
	// FK-via-parent ownership forwards the actor as actorId so the create mutation
	// can verify the actor owns the referenced parent row before inserting.
	if r.Ownership.ViaParent() {
		b.WriteString("    actorId: ownerId,\n")
	}
	for _, f := range r.InputFields {
		// Typed nested fields map each wire key to its DB camelCase column.
		if len(f.Nested) > 0 {
			b.WriteString(nestedCreateExpr(f))
			continue
		}
		if f.Map != nil {
			b.WriteString(mapCreateExpr(f))
			continue
		}
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
	for _, f := range patchFields(r) {
		// Typed nested fields map each wire key to its DB camelCase column.
		if len(f.Nested) > 0 {
			b.WriteString(nestedPatchAssign(f))
			continue
		}
		if f.Map != nil {
			b.WriteString(mapPatchAssign(f))
			continue
		}
		cols := f.WriteAliases
		if len(cols) == 0 {
			cols = []string{f.Column}
		}
		if len(cols) == 1 {
			// A clearable id-ref carries an Id | "" value; the empty-string clear
			// sentinel is resolved to undefined by the hand-written write helper, so
			// cast it into the column type here to keep the patch shape.
			if f.PatchTSScalar != "" {
				fmt.Fprintf(&b, "  if (input.%s !== undefined) patch.%s = input.%s as Doc<\"%s\">[\"%s\"];\n", f.Wire, cols[0], f.Wire, r.Table, cols[0])
				continue
			}
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

	// A scope discriminator (two resources sharing one physical table) emits an
	// async inScope<Singular>(ctx, doc) helper that loads the row's Via FK parent
	// and checks its discriminator column, so a read never leaks a row belonging
	// to the sibling resource (a glovebox read must not return a gallery photo).
	if r.Scope != nil {
		s := r.Scope
		fmt.Fprintf(&b, "/** True when this %s row is in the %q scope (its %s is a %q %s). */\n",
			singularize(table), s.Equals, s.Via, s.Equals, singularize(s.Table))
		fmt.Fprintf(&b, "async function inScope%s(\n", singular)
		fmt.Fprintf(&b, "  ctx: {\n    db: {\n      get: (\n        t: \"%s\",\n        id: Id<\"%s\">,\n      ) => Promise<Doc<\"%s\"> | null>;\n    };\n  },\n", s.Table, s.Table, s.Table)
		fmt.Fprintf(&b, "  doc: Doc<\"%s\">,\n", table)
		b.WriteString("): Promise<boolean> {\n")
		fmt.Fprintf(&b, "  const scopeParent = await ctx.db.get(\"%s\", doc.%s);\n", s.Table, r.ScopeViaColumn)
		fmt.Fprintf(&b, "  return !!scopeParent && scopeParent.%s === %q;\n}\n\n", s.Field, s.Equals)
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
	// A scoped resource also hides any row outside its scope (the sibling
	// resource's rows on the shared table) — appended as an async guard tail.
	if r.Scope != nil {
		ownGuard += " || !(await inScope" + singular + "(ctx, doc))"
	}
	// A soft-deleted row is hidden from the public API: append `|| doc.<col>` so
	// read/update/delete handlers treat it as not-found (mirrors the hand-written
	// `!doc.deleted` ownership guard).
	if r.SoftDelete != "" {
		ownGuard += " || doc." + r.SoftDelete
	}

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
			fmt.Fprintf(&b, "    await %s(ctx, args.id, doc, patch, args.actorId);\n\n", fn)
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
			fmt.Fprintf(&b, "    await %s(ctx, args.id, doc, args.actorId);\n", fn)
		} else {
			b.WriteString("    await ctx.db.delete(\"" + table + "\", args.id);\n")
		}
		b.WriteString("    return { deleted: true };\n")
		b.WriteString("  },\n});\n")
	}

	return b.String()
}
