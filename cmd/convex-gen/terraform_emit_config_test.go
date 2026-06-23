package main

import (
	"strings"
	"testing"
)

func TestEmitGeneratorConfigMatchesGolden(t *testing.T) {
	r, _ := ResolveResource(vehiclesTable(), vehiclesSpec())
	got := strings.TrimSpace(EmitGeneratorConfig([]ResolvedResource{r}))
	want := strings.TrimSpace(`provider:
  name: dashtag
resources:
  vehicle:
    create:
      path: /api/v1/vehicles
      method: POST
    read:
      path: /api/v1/vehicles/{id}
      method: GET
    update:
      path: /api/v1/vehicles/{id}
      method: PATCH
    delete:
      path: /api/v1/vehicles/{id}
      method: DELETE`)
	if got != want {
		t.Errorf("generator_config mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

// TestEmitGeneratorConfigOmitsDeleteVerb locks verb gating in the
// generator_config.yml emitter: a resource whose verbs omit "delete" must NOT
// emit the delete operation block, while the remaining operations stay intact.
func TestEmitGeneratorConfigOmitsDeleteVerb(t *testing.T) {
	spec := vehiclesSpec()
	spec.Verbs = []string{"create", "read", "update"} // no delete
	got := EmitGeneratorConfig([]ResolvedResource{mustResolve(t, vehiclesTable(), spec)})

	if strings.Contains(got, "    delete:\n") {
		t.Errorf("delete-omitted resource must NOT emit a delete operation:\n%s", got)
	}
	if strings.Contains(got, "method: DELETE") {
		t.Errorf("delete-omitted resource must NOT emit method: DELETE:\n%s", got)
	}
	// Remaining operations are still present.
	for _, frag := range []string{
		"    create:\n      path: /api/v1/vehicles\n      method: POST",
		"    read:\n      path: /api/v1/vehicles/{id}\n      method: GET",
		"    update:\n      path: /api/v1/vehicles/{id}\n      method: PATCH",
	} {
		if !strings.Contains(got, frag) {
			t.Errorf("delete-omitted config must keep other operations, missing:\n%s\nGOT:\n%s", frag, got)
		}
	}
}

// TestEmitGeneratorConfigOmitsUpdateVerb is the symmetric gate for "update":
// omitting it suppresses the update (PATCH) operation while keeping the others.
func TestEmitGeneratorConfigOmitsUpdateVerb(t *testing.T) {
	spec := vehiclesSpec()
	spec.Verbs = []string{"create", "read", "delete"} // no update
	got := EmitGeneratorConfig([]ResolvedResource{mustResolve(t, vehiclesTable(), spec)})

	if strings.Contains(got, "    update:\n") {
		t.Errorf("update-omitted resource must NOT emit an update operation:\n%s", got)
	}
	if strings.Contains(got, "method: PATCH") {
		t.Errorf("update-omitted resource must NOT emit method: PATCH:\n%s", got)
	}
	// delete (the verb that would have followed update in emitOrder) is still
	// emitted — proving the skip is targeted, not a truncation.
	if !strings.Contains(got, "    delete:\n      path: /api/v1/vehicles/{id}\n      method: DELETE") {
		t.Errorf("update-omitted config must still emit the delete operation:\n%s", got)
	}
}
