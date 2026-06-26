package main

import (
	"strings"
	"testing"
)

// --- Feature 4: $actor create constant sentinel ---

// TestCreateConstantLiteralActor locks the $actor sentinel: it renders the
// authenticated-user param (ownerId) rather than a JSON string literal.
func TestCreateConstantLiteralActor(t *testing.T) {
	if got := createConstantLiteral("$actor"); got != "ownerId" {
		t.Errorf("createConstantLiteral(\"$actor\") = %q, want ownerId", got)
	}
}

// TestCreateConstantLiteralNonActor confirms a plain string is still rendered as
// a JSON-quoted literal (not mistaken for the sentinel).
func TestCreateConstantLiteralNonActor(t *testing.T) {
	if got := createConstantLiteral("draft"); got != `"draft" as const` {
		t.Errorf("createConstantLiteral(\"draft\") = %q, want \"draft\" as const", got)
	}
	if got := createConstantLiteral(1.0); got != "1 as const" {
		t.Errorf("createConstantLiteral(1.0) = %q, want 1 as const", got)
	}
	if got := createConstantLiteral([]any{}); got != "[]" {
		t.Errorf("createConstantLiteral([]) = %q, want []", got)
	}
}

// TestEmitApiTSActorCreateConstant is the emitter-level lock for feature 4: a
// createConstants entry of "$actor" injects the authenticated user id (the
// ownerId param of toCreateArgs), not a quoted "$actor" literal.
func TestEmitApiTSActorCreateConstant(t *testing.T) {
	tbl := albumsTable()
	tbl.Fields = append(tbl.Fields, FieldInfo{Name: "ownerId", Type: "id", IsID: true, TableRef: "users"})
	spec := albumsViaParentSpec()
	spec.CreateConstants = map[string]any{"ownerId": "$actor"}

	src := EmitApiTS(mustResolve(t, tbl, spec))
	createBody := between(src, "export function toCreateArgs(", "}")

	if !strings.Contains(createBody, "ownerId: ownerId,") {
		t.Errorf("$actor create constant must inject the authed-user param (ownerId: ownerId):\n%s", createBody)
	}
	if strings.Contains(createBody, `"$actor"`) {
		t.Errorf("$actor sentinel must NOT be emitted as a quoted literal:\n%s", createBody)
	}
}

// --- supporting token helpers exercised by the Wave A features ---

// TestFieldScalarTypeOverride locks fieldScalarType honoring an explicit TSScalar
// override (id-reference) over the validator-derived scalar.
func TestFieldScalarTypeOverride(t *testing.T) {
	idRef := ResolvedField{TSType: `v.id("vehicles")`, TSScalar: `Id<"vehicles">`}
	if got := fieldScalarType(idRef); got != `Id<"vehicles">` {
		t.Errorf("fieldScalarType(id-ref) = %q, want Id<\"vehicles\">", got)
	}
	plain := ResolvedField{TSType: "v.number()"}
	if got := fieldScalarType(plain); got != "number" {
		t.Errorf("fieldScalarType(number) = %q, want number", got)
	}
}

// TestInputHasIDRef locks the detector that drives the Id import in the wire-types
// file: true only when some input field carries an explicit scalar override.
func TestInputHasIDRef(t *testing.T) {
	withRef := ResolvedResource{InputFields: []ResolvedField{
		{TSType: "v.string()"},
		{TSType: `v.id("vehicles")`, TSScalar: `Id<"vehicles">`},
	}}
	if !inputHasIDRef(withRef) {
		t.Error("inputHasIDRef must be true when an input field has a TSScalar override")
	}
	without := ResolvedResource{InputFields: []ResolvedField{{TSType: "v.string()"}}}
	if inputHasIDRef(without) {
		t.Error("inputHasIDRef must be false when no input field has a TSScalar override")
	}
}
