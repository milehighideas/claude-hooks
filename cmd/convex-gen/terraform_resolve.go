package main

import "fmt"

type ResolvedField struct {
	Column       string
	Wire         string
	TSType       string // Convex validator token, e.g. "v.string()"
	TSScalar     string // explicit TS scalar type override (e.g. Id<"vehicles">); "" → derive from TSType
	Optional     bool   // wire-level optionality (input + patch)
	WriteAliases []string
	Default      any
	// ReadOnly marks a server-set, output-only column: present in the output
	// validator and toApi, but excluded from input/patch and the write helpers.
	ReadOnly bool
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
	Name         string
	Table        string
	Path         string
	Module       string
	Ownership    Ownership
	// OwnershipViaColumn is the resolved doc column for FK-via-parent ownership
	// (the Convex column behind Ownership.Via's wire name). Empty for the
	// row-local string form.
	OwnershipViaColumn string
	Verbs              []string
	InputFields  []ResolvedField // writable fields (input + patch)
	OutputFields []ResolvedField // every projected base field (writable + read-only)
	Computed     []ResolvedComputed
	WritePath    map[string]string // verb ("update"/"delete") → model helper ref
	// CreateConstants are required, unexposed _create args emitted as literals in
	// toCreateArgs (column → constant value).
	CreateConstants map[string]any
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
		CreateConstants: spec.CreateConstants,
	}

	for _, ex := range spec.Expose {
		fi, ok := byName[ex.Field]
		if !ok {
			return out, fmt.Errorf("resource %q exposes unknown column %q", spec.Table, ex.Field)
		}
		schemaOptional := fi.Optional
		rf := ResolvedField{
			Column:       ex.Field,
			Wire:         ex.Wire,
			TSType:       tsValidator(fi),
			Optional:     schemaOptional,
			WriteAliases: spec.WriteAliases[ex.Field],
			ReadOnly:     ex.ReadOnly,
		}
		// An id-reference field overrides the scalar validator/type: it is emitted
		// as v.id("<ref>") and typed Id<"<ref>"> instead of v.string()/string.
		if ex.Ref != "" {
			rf.TSType = "v.id(\"" + ex.Ref + "\")"
			rf.TSScalar = "Id<\"" + ex.Ref + "\">"
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
		// Every projected field is in the output; only writable (non-readonly)
		// fields are in the input/patch surface and the write helpers.
		out.OutputFields = append(out.OutputFields, rf)
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
