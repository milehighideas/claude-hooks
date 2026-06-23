package main

import "testing"

func TestAICatalogConfigDefaults(t *testing.T) {
	c := &Config{DataLayer: DataLayerConfig{Path: "packages/data-layer/src"}}
	applyConfigDefaults(c) // free function invoked by LoadConfig; call the same path
	if c.AI.OutputDir != "generated-ai" {
		t.Fatalf("OutputDir default = %q, want generated-ai", c.AI.OutputDir)
	}
	if c.AI.DescriptionSource != "fallback" {
		t.Fatalf("DescriptionSource default = %q, want fallback", c.AI.DescriptionSource)
	}
	if got := c.GetAICatalogOutputDir(); got != "packages/data-layer/src/generated-ai" {
		t.Fatalf("GetAICatalogOutputDir = %q", got)
	}
	if c.Generators.AICatalog {
		t.Fatalf("AICatalog must default to false for backwards compatibility")
	}
}
