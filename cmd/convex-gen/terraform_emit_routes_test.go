package main

import (
	"strings"
	"testing"
)

// TestEmitRoutesTSOmitsDeleteVerb locks verb gating in the routes emitter: a
// resource whose verbs omit "delete" must NOT register a DELETE http.route.
func TestEmitRoutesTSOmitsDeleteVerb(t *testing.T) {
	spec := vehiclesSpec()
	spec.Verbs = []string{"create", "read", "update"} // no delete
	src := EmitRoutesTS(mustResolve(t, vehiclesTable(), spec))

	if strings.Contains(src, "// DELETE") || strings.Contains(src, "method: \"DELETE\"") {
		t.Errorf("delete-omitted resource must not emit a DELETE route:\n%s", src)
	}
	if !strings.Contains(src, "method: \"PATCH\"") {
		t.Errorf("update verb must still emit a PATCH route:\n%s", src)
	}
	if !strings.Contains(src, "method: \"GET\"") || !strings.Contains(src, "method: \"POST\"") {
		t.Errorf("create/read routes must remain:\n%s", src)
	}
}

// TestEmitRoutesTSOmitsUpdateVerb is the symmetric gate for "update": omitting
// it suppresses the PATCH route while keeping the others.
func TestEmitRoutesTSOmitsUpdateVerb(t *testing.T) {
	spec := vehiclesSpec()
	spec.Verbs = []string{"create", "read", "delete"} // no update
	src := EmitRoutesTS(mustResolve(t, vehiclesTable(), spec))

	if strings.Contains(src, "// UPDATE") || strings.Contains(src, "method: \"PATCH\"") {
		t.Errorf("update-omitted resource must not emit a PATCH route:\n%s", src)
	}
	if !strings.Contains(src, "method: \"DELETE\"") {
		t.Errorf("delete verb must still emit a DELETE route:\n%s", src)
	}
}

func TestEmitRoutesTS(t *testing.T) {
	r, _ := ResolveResource(vehiclesTable(), vehiclesSpec())
	src := EmitRoutesTS(r)
	must := []string{
		"DO NOT EDIT",
		"export function registerVehicleRoutes(http: HttpRouter): void {",
		"const PREFIX = \"/api/v1/vehicles/\";",
		"method: \"POST\"",
		"method: \"PATCH\"",
		"method: \"DELETE\"",
		"\"vehicles:write\"",
		"\"vehicles:read\"",
		"if (typeof b.for_sale === \"boolean\") patch.for_sale = b.for_sale;",
		// Values (toCreateArgs) come from the Api module …
		"import { toCreateArgs } from \"../vehicles/vehiclesApi\";",
		// … while the wire types come from the domain's types/ dir.
		"import type {\n  VehicleApiInput,\n  VehicleApiPatch,\n} from \"../vehicles/types/vehicleApi.types\";",
	}
	for _, frag := range must {
		if !strings.Contains(src, frag) {
			t.Errorf("EmitRoutesTS missing: %s", frag)
		}
	}

	// The routes file must NOT pull the wire types from the Api module anymore.
	if strings.Contains(src, "type VehicleApiInput,\n") && strings.Contains(src, "} from \"../vehicles/vehiclesApi\"") {
		t.Errorf("EmitRoutesTS must not import wire types from the Api module:\n%s", src)
	}
}
