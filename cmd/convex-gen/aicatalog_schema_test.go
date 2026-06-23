package main

import "testing"

func TestArgsToJSONSchema(t *testing.T) {
	fn := ConvexFunction{
		Type: FunctionTypeMutation,
		Args: []ArgInfo{
			{Name: "name", Type: "string", Optional: false},
			{Name: "communityId", Type: "Id<\"communities\">", IsID: true, TableName: "communities"},
			{Name: "tags", Type: "string[]", Optional: true},
		},
	}
	got := argsToJSONSchema(fn)
	// Stable, alphabetical key order from encoding/json on maps.
	want := `{"additionalProperties":false,"properties":{"communityId":{"description":"Id of a \"communities\" document","type":"string"},"name":{"type":"string"},"tags":{"items":{"type":"string"},"type":"array"}},"required":["name","communityId"],"type":"object"}`
	if got != want {
		t.Fatalf("argsToJSONSchema mismatch:\n got: %s\nwant: %s", got, want)
	}
}

func TestArgsToJSONSchemaComplexFallback(t *testing.T) {
	fn := ConvexFunction{Type: FunctionTypeMutation, UseFunctionArgs: true}
	got := argsToJSONSchema(fn)
	want := `{"additionalProperties":true,"type":"object"}`
	if got != want {
		t.Fatalf("complex args = %s, want %s", got, want)
	}
}

func TestFallbackDescription(t *testing.T) {
	fn := ConvexFunction{Type: FunctionTypeMutation, Args: []ArgInfo{{Name: "name"}, {Name: "startsAt"}}}
	got := fallbackDescription("events/eventMutations/createEvent", fn)
	want := `events/eventMutations/createEvent (mutation). Arguments: name, startsAt.`
	if got != want {
		t.Fatalf("fallbackDescription = %q, want %q", got, want)
	}
}
