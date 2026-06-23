package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCatalogClassificationAndExclusion(t *testing.T) {
	cfg := &Config{AI: AIConfig{
		Deny:      []string{"admin/**", "**/*delete*"},
		ForceRead: []string{"search/**"},
	}}
	g := NewAICatalogGenerator(cfg)

	read := ConvexFunction{Name: "getEvent", Type: FunctionTypeQuery, Namespace: "events/eventQueries"}
	write := ConvexFunction{Name: "createEvent", Type: FunctionTypeMutation, Namespace: "events/eventMutations"}
	forced := ConvexFunction{Name: "geocode", Type: FunctionTypeAction, Namespace: "search/geo"}

	if got := g.classifyKind(catalogFnPath(read), read, cfg.AI.ForceRead); got != "read" {
		t.Errorf("query kind = %q, want read", got)
	}
	if got := g.classifyKind(catalogFnPath(write), write, cfg.AI.ForceRead); got != "write" {
		t.Errorf("mutation kind = %q, want write", got)
	}
	if got := g.classifyKind(catalogFnPath(forced), forced, cfg.AI.ForceRead); got != "read" {
		t.Errorf("forceRead action kind = %q, want read", got)
	}
	if catalogFnPath(write) != "events/eventMutations/createEvent" {
		t.Errorf("fnPath = %q", catalogFnPath(write))
	}
	if catalogAPIPath(write) != "events.eventMutations.createEvent" {
		t.Errorf("apiPath = %q", catalogAPIPath(write))
	}
}

func TestGenerateWritesCatalog(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{
		DataLayer: DataLayerConfig{Path: dir},
		AI:        AIConfig{OutputDir: "generated-ai", Deny: []string{"admin/**"}, Descriptions: map[string]string{"events/eventMutations/createEvent": "Create a new event."}},
	}
	g := NewAICatalogGenerator(cfg)
	fns := []ConvexFunction{
		{Name: "createEvent", Type: FunctionTypeMutation, Namespace: "events/eventMutations", Args: []ArgInfo{{Name: "name", Type: "string"}}},
		{Name: "banUser", Type: FunctionTypeMutation, Namespace: "admin/userAdmin"}, // denied
	}
	if err := g.Generate(fns); err != nil {
		t.Fatal(err)
	}
	out, err := os.ReadFile(filepath.Join(dir, "generated-ai", "catalog.ts"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if !strings.Contains(s, `"events/eventMutations/createEvent"`) {
		t.Error("catalog missing createEvent")
	}
	if strings.Contains(s, "banUser") {
		t.Error("catalog must exclude denied admin function")
	}
	if !strings.Contains(s, "Create a new event.") {
		t.Error("catalog must use description override")
	}
	if !strings.Contains(s, `"kind": "write"`) && !strings.Contains(s, `kind: "write"`) {
		t.Error("catalog missing write classification")
	}
}
