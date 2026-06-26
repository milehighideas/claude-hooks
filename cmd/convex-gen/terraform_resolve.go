package main

import (
	"fmt"
	"strings"
)

type ResolvedField struct {
	Column   string
	Wire     string
	TSType   string // Convex validator token, e.g. "v.string()"
	TSScalar string // explicit TS scalar type override (e.g. Id<"vehicles">); "" → derive from TSType
	// PatchTSType / PatchTSScalar override the validator/scalar on the PATCH
	// surface only (input + output keep TSType/TSScalar). Used by a Clearable
	// id-reference field, whose PATCH validator widens to v.union(v.id, v.literal(""))
	// so a curated update can clear the FK with "". Empty → fall back to TSType/TSScalar.
	PatchTSType   string
	PatchTSScalar string
	Optional      bool // wire-level optionality (input + patch)
	WriteAliases  []string
	Default       any
	// ReadOnly marks a server-set, output-only column: present in the output
	// validator and toApi, but excluded from input/patch and the write helpers.
	ReadOnly bool
	// WriteOnly marks an accept-but-never-return column (e.g. private customer
	// notes): present in the input/patch surface + write helpers, but excluded
	// from the output validator and toApi projection.
	WriteOnly bool
	// Immutable marks a create-only writable field (an id-reference FK): present
	// in the create input + toCreateArgs, but excluded from the PATCH surface
	// (patch validator, toUpdatePatch, readPatch, and the derived *ApiPatch type)
	// so a child cannot be reparented through an update.
	Immutable bool
	// OutputRequired is true when the field is always present in the response and
	// so emitted REQUIRED (not v.optional) in the output validator: the field is
	// required in the schema (always set), OR it has an effective create default
	// (so toCreateArgs/read-time coalesce always fill it). Fields optional in the
	// schema with no default stay optional in the output.
	OutputRequired bool
	// CoalesceDefault is true when toApi must emit `doc.col ?? OutputDefault`
	// because the column is optional in the schema but has an effective default
	// (an explicit overlay default, or a boolean's implicit false).
	CoalesceDefault bool
	// OutputDefault is the effective default literal used for the read-time
	// coalesce when CoalesceDefault is true.
	OutputDefault any
	// Nested, when non-nil, marks this field as a typed nested object (array form
	// unless NestedSingle): the validators emit v.array(v.object({…})) /
	// v.object({…}), and toApi/toCreateArgs/toUpdatePatch map between the public
	// wire keys and the DB camelCase columns. The whole nested value is one JSON
	// field on the wire (readPatch treats it as a single object/array).
	Nested       []NestedField
	NestedSingle bool
	// Map, when non-nil, marks this field as a fixed-key object map (tab_privacy):
	// the validator is v.object({ <key>: v.optional(<obj>), … }). The whole map is
	// one JSON object on the wire; readPatch guards it with typeof object and toApi/
	// write helpers pass it through (the closed key set is identical wire↔db, and
	// the inner object keys are already the public wire keys).
	Map *ObjectMapShape
}

type ResolvedComputed struct {
	Wire  string
	As    string
	From  string
	Of    []string
	Hatch bool
	// Out is the explicit output validator token for a hatch computed field
	// (e.g. "v.array(v.string())"). Required for hatch fields so the public
	// output validator never falls back to an untyped v.any().
	Out string
}

type ResolvedResource struct {
	Name      string
	Table     string
	Path      string
	Module    string
	Ownership Ownership
	// OwnershipViaColumn is the resolved doc column for FK-via-parent ownership
	// (the Convex column behind Ownership.Via's wire name). Empty for the
	// row-local string form.
	OwnershipViaColumn string
	Verbs              []string
	InputFields        []ResolvedField // writable fields (input + patch)
	OutputFields       []ResolvedField // every projected base field (writable + read-only)
	Computed           []ResolvedComputed
	WritePath          map[string]string // verb ("update"/"delete") → model helper ref
	// MutationsModule overrides the internal module path (relative to the Convex
	// root) hosting the CREATE route's `_create` mutation. Empty → conventional
	// "<domain>/<singular>Mutations".
	MutationsModule string
	// SoftDelete is the boolean column that hides a row from the public API when
	// true (appended as `&& !doc.<col>` to the ownership/read guard). Empty → no
	// soft-delete filtering.
	SoftDelete string
	// CreateConstants are required, unexposed _create args emitted as literals in
	// toCreateArgs (column → constant value).
	CreateConstants map[string]any
	// Media, when non-nil, carries the presign media escape-hatch config forward
	// to the routes emitter. Nil → no media route.
	Media *MediaSpec
	// Singular is the camelCase singular symbol/file basename. It defaults to
	// singularize(Table) but is overridden by the overlay's Name when two
	// resources share one table (photos vs glovebox documents both on
	// vehiclePortfolioItems) so their emitted symbols, routes file, and wire-types
	// file do not collide. Every emitter derives its symbol base + file names from
	// this, NOT from Table.
	Singular string
	// Scope, when non-nil, narrows a shared table to one logical resource by an
	// indirect parent-album discriminator (see ScopeSpec). Read handlers append a
	// `&& inScope(...)` guard that resolves the row's Via FK parent and checks its
	// discriminator. Nil → no scope filtering.
	Scope *ScopeSpec
	// ScopeViaColumn is the resolved doc column behind Scope.Via (a wire name or
	// raw column). Empty when Scope is nil.
	ScopeViaColumn string
}

// singular returns the camelCase singular symbol/file base for the resource: the
// overlay Name override, else singularize(Table).
func (r ResolvedResource) singular() string {
	if r.Singular != "" {
		return r.Singular
	}
	return singularize(r.Table)
}

// pascalSingular returns the PascalCase symbol base (e.g. "Photo",
// "GloveboxDocument", "Vehicle").
func (r ResolvedResource) pascalSingular() string {
	return toPascalCase(r.singular())
}

// lcSingularName returns the lower-first-letter PascalCase singular used as the
// validator prefix + wire-types basename (e.g. "photo", "gloveboxDocument").
func (r ResolvedResource) lcSingularName() string {
	p := r.pascalSingular()
	return strings.ToLower(p[:1]) + p[1:]
}

// tsValidator maps a parsed FieldInfo to a Convex validator token.
func tsValidator(f FieldInfo) string {
	switch f.Type {
	case "string", "id":
		return "v.string()"
	case "number":
		return "v.number()"
	case "boolean":
		return "v.boolean()"
	case "array":
		switch f.ArrayType {
		case "number":
			return "v.array(v.number())"
		case "boolean":
			return "v.array(v.boolean())"
		default:
			return "v.array(v.string())"
		}
	default:
		return "v.string()"
	}
}

// ResolveResource combines a parsed table with its curation overlay into the
// resolved field model the emitters consume. Exposed non-computed fields become
// Input/Output base fields; computed entries are output-only.
func ResolveResource(t TableInfo, spec ResourceSpec) (ResolvedResource, error) {
	byName := map[string]FieldInfo{}
	for _, f := range t.Fields {
		byName[f.Name] = f
	}

	out := ResolvedResource{
		Name: spec.Table, Table: spec.Table, Path: spec.Path, Module: spec.Module,
		Ownership: spec.Ownership, Verbs: spec.Verbs, WritePath: spec.WritePath,
		MutationsModule: spec.MutationsModule,
		SoftDelete:      spec.SoftDelete,
		CreateConstants: spec.CreateConstants,
		Media:           spec.Media,
		Singular:        spec.Name,
		Scope:           spec.Scope,
	}

	for _, ex := range spec.Expose {
		fi, ok := byName[ex.Field]
		if !ok {
			return out, fmt.Errorf("resource %q exposes unknown column %q", spec.Table, ex.Field)
		}
		schemaOptional := fi.Optional
		if ex.ReadOnly && ex.WriteOnly {
			return out, fmt.Errorf(
				"resource %q field %q cannot be both readonly and writeOnly",
				spec.Table, ex.Field,
			)
		}
		rf := ResolvedField{
			Column:       ex.Field,
			Wire:         ex.Wire,
			TSType:       tsValidator(fi),
			Optional:     schemaOptional,
			WriteAliases: spec.WriteAliases[ex.Field],
			ReadOnly:     ex.ReadOnly,
			WriteOnly:    ex.WriteOnly,
		}
		// An id-ARRAY field overrides the scalar validator/type with
		// v.array(v.id("<ref>")) / Id<"<ref>">[]. Unlike a single Ref it is a plain
		// editable collection (not a parent FK), so it is NOT create-only — it stays
		// on the PATCH surface.
		if ex.RefArray != "" {
			rf.TSType = "v.array(v.id(\"" + ex.RefArray + "\"))"
			rf.TSScalar = "Id<\"" + ex.RefArray + "\">[]"
		}
		// An id-reference field overrides the scalar validator/type: it is emitted
		// as v.id("<ref>") and typed Id<"<ref>"> instead of v.string()/string.
		if ex.Ref != "" {
			rf.TSType = "v.id(\"" + ex.Ref + "\")"
			rf.TSScalar = "Id<\"" + ex.Ref + "\">"
			// An FK reference defaults to create-only: it scopes the child to its
			// parent and must never be reassigned via PATCH. An explicit
			// {"immutable": false} keeps an editable id-reference (e.g. a tagged
			// user/community on an ownership record) in the PATCH surface.
			rf.Immutable = true
		}
		// An explicit immutable override (either direction) wins over the default.
		if ex.Immutable != nil {
			rf.Immutable = *ex.Immutable
		}
		// A clearable id-reference widens its PATCH validator to also accept "" so
		// a curated update can clear the optional FK (v.id rejects "" otherwise).
		// The hand-written write-path helper turns "" into an undefined column.
		if ex.Clearable && ex.Ref != "" {
			rf.PatchTSType = "v.union(v.id(\"" + ex.Ref + "\"), v.literal(\"\"))"
			rf.PatchTSScalar = "Id<\"" + ex.Ref + "\"> | \"\""
		}
		// An enum field overrides the scalar validator/type with a string-literal
		// union so the OpenAPI surface carries a proper `enum`.
		if len(ex.Enum) > 0 {
			rf.TSType = enumValidatorToken(ex.Enum)
			rf.TSScalar = enumScalarType(ex.Enum)
		}
		// A typed nested object/array overrides the scalar validator/type with
		// v.array(v.object({…})) / v.object({…}) and a derived TS type.
		if len(ex.Nested) > 0 {
			rf.Nested = ex.Nested
			rf.NestedSingle = ex.NestedSingle
			rf.TSType = nestedValidatorToken(ex.Nested, ex.NestedSingle)
			rf.TSScalar = nestedScalarType(ex.Nested, ex.NestedSingle)
		}
		// A fixed-key object map overrides the scalar validator/type with
		// v.object({ <key>: v.optional(<obj>), … }) and the derived map TS type.
		if ex.ObjectMap != nil {
			rf.Map = ex.ObjectMap
			rf.TSType = objectMapValidator(ex.ObjectMap)
			rf.TSScalar = objectMapScalar(ex.ObjectMap)
		}
		// Effective create default: an explicit overlay default, or a boolean's
		// implicit false (booleans always have a sane zero value).
		def, hasDefault := spec.Defaults[ex.Field]
		if !hasDefault && rf.TSType == "v.boolean()" {
			def, hasDefault = false, true
		}
		if hasDefault {
			rf.Default = def
			// A field with a curation default is optional on the wire even when
			// the underlying schema column is required: the default fills the gap
			// in toCreateArgs, so the public API never forces the client to send
			// it. This keeps the inferred Terraform attribute behavior at
			// "computed_optional" rather than "required" for these columns.
			rf.Optional = true
		}
		// A {"required": true} override forces a schema-optional column to be a
		// REQUIRED create input (the readInput guard 422s on omission), without
		// touching the schema. Used by a media KEY column: optional in the schema
		// but always carried from the presign step on the public commit contract.
		// It only tightens the INPUT surface; output optionality is unchanged.
		if ex.Required {
			rf.Optional = false
		}
		// Output optionality: a field is always present in the response (REQUIRED
		// in the output validator) when it is required in the schema OR has an
		// effective default. Only a schema-optional field with no default stays
		// optional in the output.
		rf.OutputRequired = !schemaOptional || hasDefault
		// A schema-optional field that has a default coalesces to it at read time
		// (`doc.col ?? default`), so it is always present despite being optional
		// on input/patch (computed_optional in Terraform).
		if schemaOptional && hasDefault {
			rf.CoalesceDefault = true
			rf.OutputDefault = def
		}
		// A typed nested ARRAY always projects to [] at read time (the toApi map
		// coalesces `doc.col ?? []`), so it is always present in the output even
		// when schema-optional. It is also always OPTIONAL on input/patch: the
		// create/update mappers coalesce `?? []`, so the client never has to send
		// it and an absent value is a no-op (readInput must not 422 on omission).
		// The single-object form keeps its schema optionality on both surfaces.
		if len(rf.Nested) > 0 && !rf.NestedSingle {
			rf.OutputRequired = true
			rf.Optional = true
		}
		// Field surface partition:
		//   - read-only  → output only (excluded from input/patch + write helpers)
		//   - write-only → input/patch only (excluded from the output projection)
		//   - default    → both
		// ReadOnly and WriteOnly are mutually exclusive (validated in ResolveResource).
		if !rf.WriteOnly {
			out.OutputFields = append(out.OutputFields, rf)
		}
		if !rf.ReadOnly {
			out.InputFields = append(out.InputFields, rf)
		}
	}

	// Resolve FK-via-parent ownership to the concrete doc column behind the wire
	// name in Ownership.Via (matched against an exposed field's wire, then column).
	if spec.Ownership.ViaParent() {
		via := spec.Ownership.Via
		col := ""
		for _, f := range out.OutputFields {
			if f.Wire == via || f.Column == via {
				col = f.Column
				break
			}
		}
		if col == "" {
			if _, ok := byName[via]; ok {
				col = via
			}
		}
		if col == "" {
			return out, fmt.Errorf(
				"resource %q ownership.via %q does not match any exposed field or column",
				spec.Table, via,
			)
		}
		out.OwnershipViaColumn = col
	}

	// Resolve the scope discriminator FK to its concrete doc column (matched
	// against an exposed field's wire/column, then a raw schema column). The
	// referenced parent column (Scope.Field) lives on Scope.Table, not this row,
	// so it is only validated for non-emptiness here.
	if spec.Scope != nil {
		s := spec.Scope
		if s.Via == "" || s.Table == "" || s.Field == "" || s.Equals == "" {
			return out, fmt.Errorf(
				"resource %q scope must set via, table, field, and equals",
				spec.Table,
			)
		}
		col := ""
		for _, f := range out.OutputFields {
			if f.Wire == s.Via || f.Column == s.Via {
				col = f.Column
				break
			}
		}
		if col == "" {
			if _, ok := byName[s.Via]; ok {
				col = s.Via
			}
		}
		if col == "" {
			return out, fmt.Errorf(
				"resource %q scope.via %q does not match any exposed field or column",
				spec.Table, s.Via,
			)
		}
		out.ScopeViaColumn = col
	}

	for wire, c := range spec.Computed {
		if c.Hatch && c.Out == "" {
			return out, fmt.Errorf(
				"resource %q hatch computed field %q must declare an output validator token (\"out\")",
				spec.Table, wire,
			)
		}
		out.Computed = append(out.Computed, ResolvedComputed{
			Wire: wire, As: c.As, From: c.From, Of: c.Of, Hatch: c.Hatch, Out: c.Out,
		})
	}
	return out, nil
}
