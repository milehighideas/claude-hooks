package main

import (
	"encoding/json"
	"fmt"
	"os"
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
	// ReadOnly marks a server-set, output-only column: it appears in the output
	// validator (and toApi as doc.<col>) but NOT in the input/patch validators,
	// so Terraform infers it as a computed attribute.
	ReadOnly bool
}

func (e *ExposeField) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		e.Field = s
		e.Wire = toSnakeCase(s)
		return nil
	}
	var obj struct {
		Field    string `json:"field"`
		Wire     string `json:"wire"`
		Ref      string `json:"ref"`
		ReadOnly bool   `json:"readonly"`
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
	e.ReadOnly = obj.ReadOnly
	if e.Wire == "" {
		e.Wire = toSnakeCase(obj.Field)
	}
	return nil
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
	// CreateConstants are required _create arguments that the public API never
	// exposes: they are emitted into toCreateArgs as literal constant values
	// (e.g. {"modifications": [], "marketValue": 0, "engine": ""}). Keyed by the
	// Convex column name. Order is sorted for deterministic output.
	CreateConstants map[string]any `json:"createConstants"`
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
