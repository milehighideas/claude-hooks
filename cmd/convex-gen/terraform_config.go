package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// ExposeField is one curated public field. In JSON it is either a bare string
// (the column name, wire = snake_case of it) or an object {field, wire, ref?,
// readonly?}.
type ExposeField struct {
	Field string // Convex column name
	Wire  string // public snake_case wire name
	// Ref, when non-empty, marks the field as an id-reference: it is emitted as
	// v.id("<Ref>") in the input/patch validators and typed Id<"<Ref>"> in the
	// derived wire type (instead of v.string()/string).
	Ref string
	// RefArray, when non-empty, marks the field as an id-ARRAY: it is emitted as
	// v.array(v.id("<RefArray>")) in every validator and typed Id<"<RefArray>">[]
	// in the derived wire type. Unlike Ref it is NOT create-only by default (an
	// id-array is a plain editable collection, not a parent FK), so it stays on the
	// PATCH surface. Mutually exclusive with Ref/Enum/Nested/ObjectMap.
	RefArray string
	// ObjectMap, when non-nil, marks the field as a fixed-key object map: a closed
	// set of keys each holding an OPTIONAL value of one nested object shape (see
	// ObjectMapShape). Models tab_privacy. Mutually exclusive with Ref/RefArray/
	// Enum/Nested.
	ObjectMap *ObjectMapShape
	// ReadOnly marks a server-set, output-only column: it appears in the output
	// validator (and toApi as doc.<col>) but NOT in the input/patch validators,
	// so Terraform infers it as a computed attribute.
	ReadOnly bool
	// WriteOnly marks an accept-but-never-return column: it appears in the
	// input/patch validators and the write helpers but NOT in the output
	// validator / toApi projection (e.g. private notes the API accepts but never
	// echoes back). Mutually exclusive with ReadOnly.
	WriteOnly bool
	// Clearable widens an id-reference field's PATCH validator to also accept the
	// empty string (v.union(v.id("X"), v.literal("")) / Id<"X"> | ""), so a curated
	// update can CLEAR an optional foreign key — Convex's v.id validator otherwise
	// rejects "" before any write helper runs. The hand-written write-path helper
	// is responsible for turning "" into an undefined column. No effect on plain
	// string fields (v.string already accepts "").
	Clearable bool
	// Enum, when non-empty, marks the field as a string-literal union: it is
	// emitted as v.union(v.literal("a"), …) in the input/patch/output validators
	// and typed as the string union ("a" | "b") in the derived wire type (instead
	// of v.string()/string), so the OpenAPI surface carries a proper `enum`.
	Enum []string
	// Nested, when non-nil, marks the field as a typed array-of-objects (or a
	// single nested object when NestedSingle is set): emitted as
	// v.array(v.object({…})) / v.object({…}) in every validator, with a derived
	// TS type and snake→camel column renames in the projection/write helpers.
	Nested []NestedField
	// NestedSingle is true when the nested shape is a single object ({field, wire,
	// object:{…}}) rather than an array ({field, wire, nested:{…}}).
	NestedSingle bool
	// Immutable, when non-nil, explicitly overrides whether the field is excluded
	// from the PATCH surface. By default an id-reference (Ref) field is immutable
	// (the parent FK must never be reparented via update); set {"immutable": false}
	// in the overlay to keep an editable id-reference (e.g. a tagged user/community
	// on an ownership record) in the PATCH surface.
	Immutable *bool
	// Required forces a schema-optional column to be a REQUIRED create input field
	// (the readInput guard 422s when it is absent), without changing the schema.
	// Needed for a media KEY column: the R2 object key is optional in the schema
	// (a row may predate the upload), but the public commit contract always
	// carries it from the presign step — so the curated create must demand it,
	// matching the hand-written `key: v.string()` required input. No effect on a
	// field that is already required in the schema.
	Required bool
}

// NestedField is one key inside a typed nested object. Key is the wire (public)
// key; Optional marks a trailing "?". Exactly one of Type / Ref / RefArray / Enum
// is set, selecting the sub-field's shape:
//   - Type  ("string"|"number"|"boolean") → v.string()/v.number()/v.boolean()
//   - Ref   ("users")                      → v.id("users")          / Id<"users">
//   - RefArray ("communities")             → v.array(v.id("communities")) / Id<…>[]
//   - Enum  (["public","private"])         → v.union(v.literal(…))  / "a" | "b"
//
// The compact JSON grammar (a per-key type string) selects these via prefixes:
//
//	"string" / "number" / "boolean"   → scalar Type
//	"id:users"                        → Ref
//	"id[]:communities"                → RefArray
//	"enum:public|private|friends"     → Enum
//
// Any form may carry a trailing "?" to mark the sub-field optional.
type NestedField struct {
	Key      string
	Type     string
	Optional bool
	// Ref, when non-empty, makes this sub-field an id-reference: v.id("<Ref>") /
	// Id<"<Ref>">. Mutually exclusive with Type/RefArray/Enum.
	Ref string
	// RefArray, when non-empty, makes this sub-field an id-array:
	// v.array(v.id("<RefArray>")) / Id<"<RefArray>">[]. Mutually exclusive.
	RefArray string
	// Enum, when non-empty, makes this sub-field a string-literal union:
	// v.union(v.literal(…)) / "a" | "b". Mutually exclusive.
	Enum []string
}

// ObjectMapShape is a fixed-key object map: a known set of Keys, each mapping to
// an OPTIONAL value of the same Object shape (a list of NestedFields, which may
// themselves be rich — enum/id/id-array sub-fields). It models tab_privacy: a
// fixed set of 11 tab keys, each → the privacy object. Unlike a free v.record,
// the key set is closed (every key emitted as an optional object property), so
// the OpenAPI surface and the Terraform schema both stay typed.
type ObjectMapShape struct {
	Keys   []string
	Object []NestedField
}

func (e *ExposeField) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		e.Field = s
		e.Wire = toSnakeCase(s)
		return nil
	}
	var obj struct {
		Field     string          `json:"field"`
		Wire      string          `json:"wire"`
		Ref       string          `json:"ref"`
		RefArray  string          `json:"refArray"`
		ReadOnly  bool            `json:"readonly"`
		WriteOnly bool            `json:"writeOnly"`
		Clearable bool            `json:"clearable"`
		Enum      []string        `json:"enum"`
		Nested    json.RawMessage `json:"nested"`
		Object    json.RawMessage `json:"object"`
		ObjectMap json.RawMessage `json:"objectMap"`
		Immutable *bool           `json:"immutable"`
		Required  bool            `json:"required"`
	}
	if err := json.Unmarshal(b, &obj); err != nil {
		return fmt.Errorf("expose entry must be a string or {field,wire}: %w", err)
	}
	if obj.Field == "" {
		return fmt.Errorf("expose object missing required \"field\"")
	}
	e.Field = obj.Field
	e.Wire = obj.Wire
	e.Ref = obj.Ref
	e.RefArray = obj.RefArray
	e.ReadOnly = obj.ReadOnly
	e.WriteOnly = obj.WriteOnly
	e.Clearable = obj.Clearable
	e.Enum = obj.Enum
	e.Immutable = obj.Immutable
	e.Required = obj.Required
	shapes := 0
	for _, present := range []bool{len(obj.Nested) > 0, len(obj.Object) > 0, len(obj.ObjectMap) > 0} {
		if present {
			shapes++
		}
	}
	if shapes > 1 {
		return fmt.Errorf("expose field %q may declare at most one of \"nested\", \"object\", \"objectMap\"", obj.Field)
	}
	if len(obj.ObjectMap) > 0 {
		om, err := parseObjectMapShape(obj.ObjectMap)
		if err != nil {
			return fmt.Errorf("expose field %q objectMap: %w", obj.Field, err)
		}
		e.ObjectMap = om
	}
	if len(obj.Nested) > 0 {
		nf, err := parseNestedShape(obj.Nested)
		if err != nil {
			return fmt.Errorf("expose field %q nested: %w", obj.Field, err)
		}
		e.Nested = nf
		e.NestedSingle = false
	} else if len(obj.Object) > 0 {
		nf, err := parseNestedShape(obj.Object)
		if err != nil {
			return fmt.Errorf("expose field %q object: %w", obj.Field, err)
		}
		e.Nested = nf
		e.NestedSingle = true
	}
	if e.Wire == "" {
		e.Wire = toSnakeCase(obj.Field)
	}
	return nil
}

// parseNestedShape decodes a {"key": "type", …} object into ordered NestedFields,
// preserving JSON key declaration order (so emitted validators are stable). Each
// type is "string" | "number" | "boolean" with an optional trailing "?".
func parseNestedShape(raw json.RawMessage) ([]NestedField, error) {
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return nil, fmt.Errorf("nested shape must be a JSON object")
	}
	var out []NestedField
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		key, ok := keyTok.(string)
		if !ok {
			return nil, fmt.Errorf("nested key must be a string")
		}
		var typ string
		if err := dec.Decode(&typ); err != nil {
			return nil, fmt.Errorf("nested key %q type must be a string: %w", key, err)
		}
		nf, err := parseNestedFieldType(key, typ)
		if err != nil {
			return nil, err
		}
		out = append(out, nf)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("nested shape must declare at least one key")
	}
	return out, nil
}

// parseNestedFieldType decodes one per-key type string into a NestedField. The
// grammar (after stripping a trailing "?") is one of:
//
//	"string" | "number" | "boolean"   → scalar Type
//	"id:<table>"                       → Ref
//	"id[]:<table>"                     → RefArray
//	"enum:<a>|<b>|<c>"                 → Enum
func parseNestedFieldType(key, typ string) (NestedField, error) {
	optional := strings.HasSuffix(typ, "?")
	typ = strings.TrimSuffix(typ, "?")
	switch {
	case strings.HasPrefix(typ, "enum:"):
		raw := strings.TrimPrefix(typ, "enum:")
		var vals []string
		for _, v := range strings.Split(raw, "|") {
			if v = strings.TrimSpace(v); v != "" {
				vals = append(vals, v)
			}
		}
		if len(vals) == 0 {
			return NestedField{}, fmt.Errorf("nested key %q enum must list at least one value", key)
		}
		return NestedField{Key: key, Enum: vals, Optional: optional}, nil
	case strings.HasPrefix(typ, "id[]:"):
		table := strings.TrimSpace(strings.TrimPrefix(typ, "id[]:"))
		if table == "" {
			return NestedField{}, fmt.Errorf("nested key %q id[] must name a table", key)
		}
		return NestedField{Key: key, RefArray: table, Optional: optional}, nil
	case strings.HasPrefix(typ, "id:"):
		table := strings.TrimSpace(strings.TrimPrefix(typ, "id:"))
		if table == "" {
			return NestedField{}, fmt.Errorf("nested key %q id must name a table", key)
		}
		return NestedField{Key: key, Ref: table, Optional: optional}, nil
	case typ == "string" || typ == "number" || typ == "boolean":
		return NestedField{Key: key, Type: typ, Optional: optional}, nil
	default:
		return NestedField{}, fmt.Errorf(
			"nested key %q has unsupported type %q (want string|number|boolean|id:<t>|id[]:<t>|enum:a|b)",
			key, typ,
		)
	}
}

// parseObjectMapShape decodes a {"keys":[…], "object":{…}} fixed-key map. The
// object value reuses the rich nested-field grammar (so each tab value can be a
// privacy object with enum/id-array sub-fields).
func parseObjectMapShape(raw json.RawMessage) (*ObjectMapShape, error) {
	var obj struct {
		Keys   []string        `json:"keys"`
		Object json.RawMessage `json:"object"`
	}
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, fmt.Errorf("objectMap must be {keys,object}: %w", err)
	}
	if len(obj.Keys) == 0 {
		return nil, fmt.Errorf("objectMap must declare at least one key")
	}
	if len(obj.Object) == 0 {
		return nil, fmt.Errorf("objectMap must declare an \"object\" value shape")
	}
	fields, err := parseNestedShape(obj.Object)
	if err != nil {
		return nil, fmt.Errorf("objectMap object: %w", err)
	}
	return &ObjectMapShape{Keys: obj.Keys, Object: fields}, nil
}

// Ownership describes how a resource authorizes the actor against a row.
//
//	Row-local form (JSON string):   "ownerId"
//	  authorize by doc.ownerType === "user" && doc.<Field> === actor.
//
//	Parent-FK form (JSON object):    { "via": "vehicle_id", "table": "vehicles", "field": "ownerId" }
//	  authorize by loading the parent row referenced by the FK column (Via, a
//	  wire name or column) from Table and checking parent.<Field> === actor.
//
// An empty Ownership (zero value) means the resource is not owner-scoped.
type Ownership struct {
	Field string // owner column on the row (string form) or on the parent (object form)
	Via   string // FK wire/column on this row pointing at the parent (object form only)
	Table string // parent table to load (object form only)
}

// IsZero reports whether no ownership scoping is configured.
func (o Ownership) IsZero() bool { return o.Field == "" && o.Via == "" && o.Table == "" }

// ViaParent reports whether ownership authorizes through a loaded parent row.
func (o Ownership) ViaParent() bool { return o.Via != "" }

func (o *Ownership) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		o.Field = s
		return nil
	}
	var obj struct {
		Via   string `json:"via"`
		Table string `json:"table"`
		Field string `json:"field"`
	}
	if err := json.Unmarshal(b, &obj); err != nil {
		return fmt.Errorf("ownership must be a string or {via,table,field}: %w", err)
	}
	if obj.Via == "" {
		return fmt.Errorf("ownership object missing required \"via\"")
	}
	if obj.Field == "" {
		obj.Field = "ownerId"
	}
	o.Via, o.Table, o.Field = obj.Via, obj.Table, obj.Field
	return nil
}

// ComputedField is an output-only derived field: either a named primitive
// (As+From) or an escape hatch (Hatch) that calls a hand-written projection.
type ComputedField struct {
	As    string   // "iso8601" | "r2Urls" | "sum" | ...
	From  string   // source column (for primitives)
	Of    []string // source columns (for "sum")
	Hatch bool     // true when {"computed": true} (call <res>.projections.ts)
	Out   string   // explicit output validator token (required for hatch fields)
}

func (c *ComputedField) UnmarshalJSON(b []byte) error {
	// {"computed": true, "out": "v.array(v.string())"} hatch form
	var hatch struct {
		Computed bool   `json:"computed"`
		Out      string `json:"out"`
	}
	if err := json.Unmarshal(b, &hatch); err == nil && hatch.Computed {
		c.Hatch = true
		c.Out = hatch.Out
		return nil
	}
	var obj struct {
		As   string   `json:"as"`
		From string   `json:"from"`
		Of   []string `json:"of"`
	}
	if err := json.Unmarshal(b, &obj); err != nil {
		return fmt.Errorf("computed entry must be {as,from|of} or {computed:true}: %w", err)
	}
	c.As, c.From, c.Of = obj.As, obj.From, obj.Of
	return nil
}

// MediaSpec describes the presign → client-PUT → commit media escape hatch for
// a resource backed by an R2 object. When Presign is true the routes emitter
// adds a `POST /api/v1/<path>/presign` companion route that authorizes the
// actor (FK-via-parent ownership), calls a hand-written ownership check, runs
// the shared R2 presign action with the concrete entity/media constants, and
// returns the upload URL + storage key (and bucket when BucketField is set).
// The binary upload happens client-side against the presigned URL — bytes are
// never routed through Convex. The KeyField is a normal create field; the
// readable URL stays an output-only computed r2Urls field. A nil MediaSpec (no
// media block) leaves the resource a plain CRUD resource, unaffected.
type MediaSpec struct {
	// Presign turns on the presign companion route. When false (or the whole
	// block absent) the resource emits no media route.
	Presign bool `json:"presign"`
	// KeyField is the create column holding the R2 object key the client submits
	// after PUTting the bytes (the canonical store identifier).
	KeyField string `json:"keyField"`
	// BucketField, when non-empty, makes the presign response also echo the R2
	// bucket from the action result. Optional (public-bucket rows omit it).
	BucketField string `json:"bucketField"`
	// R2Action is the hand-written presign action the route calls, as a module
	// path + export ("r2/genericStructuredUpload.generateEntityUploadUrl"). It
	// is invoked via `api.<dotted>` so the route imports `api` alongside `internal`.
	R2Action string `json:"r2Action"`
	// EntityType / MediaType / IsPublic are the concrete constants the
	// hand-written presign route hard-codes, threaded straight into the R2 action
	// call (e.g. entityType "vehicle", mediaType "gallery", isPublic true).
	EntityType string `json:"entityType"`
	MediaType  string `json:"mediaType"`
	IsPublic   bool   `json:"isPublic"`
	// OwnershipCheck is the hand-written internal query the presign route calls to
	// verify the actor owns the parent + child scope before issuing an upload URL,
	// as a module path + export ("vehicles/photosApi.canUploadToAlbum"). It is
	// invoked via `internal.<dotted>` with the actor + the resource's FK fields.
	OwnershipCheck string `json:"ownershipCheck"`
}

// ResourceSpec is the curation overlay for one table.
type ResourceSpec struct {
	Table        string                   `json:"table"`
	Path         string                   `json:"path"`
	Module       string                   `json:"module"`
	Ownership    Ownership                `json:"ownership"`
	Verbs        []string                 `json:"verbs"`
	Expose       []ExposeField            `json:"expose"`
	WriteAliases map[string][]string      `json:"writeAliases"`
	Defaults     map[string]any           `json:"defaults"`
	Computed     map[string]ComputedField `json:"computed"`
	WritePath    map[string]string        `json:"writePath"`
	// SoftDelete names a boolean column that, when true, hides a row from the
	// public API (read/update/delete return not-found). Mirrors the hand-written
	// `!doc.deleted` ownership guard. The DELETE write-path helper is responsible
	// for setting this flag (the emitter only adds the read-side guard).
	SoftDelete string `json:"softDelete"`
	// MutationsModule overrides the internal module that backs the CREATE route's
	// `_create` mutation. By default the route targets
	// `internal.<domain>.<singular>Mutations._create` (beside the Api module). Set
	// this (a module path relative to the Convex root, e.g.
	// "maintenance/maintenanceApiMutations.ts") when the conventional name would
	// collide with a pre-existing, unrelated mutations file that should not host
	// the thin `_create`.
	MutationsModule string `json:"mutationsModule"`
	// CreateConstants are required _create arguments that the public API never
	// exposes: they are emitted into toCreateArgs as literal constant values
	// (e.g. {"modifications": [], "marketValue": 0, "engine": ""}). Keyed by the
	// Convex column name. Order is sorted for deterministic output.
	CreateConstants map[string]any `json:"createConstants"`
	// Media, when non-nil, opts the resource into the presign → PUT → commit media
	// escape hatch (a presign companion route). Absent → plain CRUD only.
	Media *MediaSpec `json:"media"`
	// Name overrides the singular symbol/file basename the emitter derives from
	// the table. By default every emitted symbol (the `<Singular>Api*` validators
	// + types, the `register<Singular>Routes` fn), the routes file
	// (`<table>Routes.ts`), and the wire-types file (`<lcSingular>Api.types.ts`)
	// are keyed on `singularize(table)`. When TWO resources share one table
	// (e.g. photos and glovebox documents both back vehiclePortfolioItems) the
	// table-derived names collide; set Name on each (e.g. "photo",
	// "gloveboxDocument") to disambiguate. The value is the camelCase singular —
	// the PascalCase symbol base is derived from it. Empty → table-derived.
	Name string `json:"name"`
	// Scope, when non-nil, narrows a shared table to one logical resource by an
	// indirect parent-album discriminator: a read returns not-found for any row
	// whose Via FK (an album) does not resolve to a parent row in Table whose
	// Field equals Equals. Needed only when two resources share one physical
	// table (photos vs glovebox documents on vehiclePortfolioItems) so a glovebox
	// read never leaks a gallery photo and vice-versa.
	Scope *ScopeSpec `json:"scope"`
}

// ScopeSpec narrows a shared table to one logical resource by an indirect
// discriminator on a parent row. The row's Via FK column points at a parent in
// Table; the resource only owns the row when that parent's Field equals Equals.
// For glovebox documents: {via:"albumId", table:"vehiclePortfolioAlbums",
// field:"albumType", equals:"documents"} — a portfolio item is a glovebox
// document only when its album is the vehicle's documents album.
type ScopeSpec struct {
	// Via is the FK column on THIS row pointing at the parent that carries the
	// discriminator (a column name, e.g. "albumId").
	Via string `json:"via"`
	// Table is the parent table the Via FK references (e.g.
	// "vehiclePortfolioAlbums").
	Table string `json:"table"`
	// Field is the discriminator column on the parent (e.g. "albumType").
	Field string `json:"field"`
	// Equals is the literal value Field must equal for the row to be in scope
	// (e.g. "documents").
	Equals string `json:"equals"`
}

// TerraformSpec is the whole convex-terraform-gen.json.
type TerraformSpec struct {
	Resources map[string]ResourceSpec `json:"resources"`
}

// LoadTerraformSpec reads and parses the curation overlay.
func LoadTerraformSpec(path string) (*TerraformSpec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading terraform spec %s: %w", path, err)
	}
	var spec TerraformSpec
	if err := json.Unmarshal(data, &spec); err != nil {
		return nil, fmt.Errorf("parsing terraform spec %s: %w", path, err)
	}
	return &spec, nil
}
