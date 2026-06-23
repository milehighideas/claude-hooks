package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTerraformConfigDefaults(t *testing.T) {
	c := &Config{Org: "@dashtag"}
	c.Convex.Path = "." // exists, passes validation
	applyConfigDefaults(c)
	if got := c.GetTerraformConfigPath(); got != "convex-terraform-gen.json" {
		t.Fatalf("default terraform config path = %q, want convex-terraform-gen.json", got)
	}
}

func TestTerraformGeneratorOffByDefault(t *testing.T) {
	c := &Config{Org: "@dashtag"}
	c.Convex.Path = "."
	applyConfigDefaults(c)
	if c.Generators.Terraform {
		t.Fatal("terraform generator must be opt-in (off by default)")
	}
}

func TestLoadTerraformSpecExposeForms(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "convex-terraform-gen.json")
	if err := os.WriteFile(p, []byte(`{
	  "resources": {
	    "vehicles": {
	      "table": "vehicles", "path": "vehicles", "ownership": "ownerId",
	      "verbs": ["create","read","update","delete"],
	      "expose": ["make", { "field": "forSale", "wire": "for_sale" }],
	      "computed": { "created_at": { "as": "iso8601", "from": "_creationTime" }, "x": { "computed": true } }
	    }
	  }
	}`), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	spec, err := LoadTerraformSpec(p)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	r := spec.Resources["vehicles"]
	if len(r.Expose) != 2 {
		t.Fatalf("expose len = %d, want 2", len(r.Expose))
	}
	if r.Expose[0].Field != "make" || r.Expose[0].Wire != "make" {
		t.Errorf("string expose => %+v, want field=make wire=make", r.Expose[0])
	}
	if r.Expose[1].Field != "forSale" || r.Expose[1].Wire != "for_sale" {
		t.Errorf("object expose => %+v, want field=forSale wire=for_sale", r.Expose[1])
	}
	if r.Computed["created_at"].As != "iso8601" || r.Computed["created_at"].From != "_creationTime" {
		t.Errorf("computed primitive parsed wrong: %+v", r.Computed["created_at"])
	}
	if !r.Computed["x"].Hatch {
		t.Errorf("computed:true should set Hatch=true: %+v", r.Computed["x"])
	}
}
