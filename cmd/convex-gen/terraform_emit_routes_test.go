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

// TestEmitRoutesTSCreate404Wrapper locks the Wave A carry-forward fix: the CREATE
// handler wraps its create mutation + read-back in try/catch so an ownership /
// not-found ConvexError surfaces as a clean 404 instead of bubbling to a 500. On
// the happy path the 201 jsonOk return stays intact, inside the try.
func TestEmitRoutesTSCreate404Wrapper(t *testing.T) {
	src := EmitRoutesTS(mustResolve(t, albumsTable(), albumsViaParentSpec()))
	createBody := between(src, "// CREATE", "// READ")

	if !strings.Contains(createBody, "try {") {
		t.Errorf("CREATE handler must wrap the create call in try/catch:\n%s", createBody)
	}
	// The 404 catch must use the resource's not-found message (not a 500 bubble).
	if !strings.Contains(createBody, `} catch {`) ||
		!strings.Contains(createBody, `return jsonError(404, "not_found", "Album not found");`) {
		t.Errorf("CREATE handler must catch into a 404 not_found:\n%s", createBody)
	}
	// The 201 success path must remain, inside the try.
	if !strings.Contains(createBody, "return jsonOk(album, 201);") {
		t.Errorf("CREATE handler must keep the 201 success return:\n%s", createBody)
	}
	// Guard ordering: the success return must come before the catch.
	tryIdx := strings.Index(createBody, "try {")
	okIdx := strings.Index(createBody, "return jsonOk(album, 201);")
	catchIdx := strings.Index(createBody, "} catch {")
	if tryIdx < 0 || tryIdx >= okIdx || okIdx >= catchIdx {
		t.Errorf("CREATE try/jsonOk/catch must be in order:\n%s", createBody)
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

// TestMutationsModuleOverride locks the overlay MutationsModule override: the
// CREATE route's _create target uses the explicit module path instead of the
// conventional internal.<domain>.<singular>Mutations, so a generated resource
// can avoid colliding with a pre-existing unrelated mutations file.
func TestMutationsModuleOverride(t *testing.T) {
	// Default (no override): conventional path beside the Api module.
	def := mutationsModulePath(ResolvedResource{
		Table: "maintenanceRecords", Module: "maintenance/maintenanceApi.ts",
	})
	if def != "internal.maintenance.maintenanceRecordMutations" {
		t.Errorf("default mutations module = %q", def)
	}

	// Override: explicit module path wins.
	over := mutationsModulePath(ResolvedResource{
		Table: "maintenanceRecords", Module: "maintenance/maintenanceApi.ts",
		MutationsModule: "maintenance/maintenanceApiMutations.ts",
	})
	if over != "internal.maintenance.maintenanceApiMutations" {
		t.Errorf("overridden mutations module = %q", over)
	}

	// The override flows into the emitted CREATE route.
	r := mustResolve(t, maintenanceRoutesTable(), maintenanceRoutesSpec())
	src := EmitRoutesTS(r)
	if !strings.Contains(src, "internal.maintenance.maintenanceApiMutations._create") {
		t.Errorf("CREATE route must target the overridden _create:\n%s", src)
	}
}

func maintenanceRoutesTable() TableInfo {
	return TableInfo{Name: "maintenanceRecords", Fields: []FieldInfo{
		{Name: "vehicleId", Type: "id", IsID: true, TableRef: "vehicles"},
		{Name: "serviceName", Type: "string"},
	}}
}

func maintenanceRoutesSpec() ResourceSpec {
	return ResourceSpec{
		Table: "maintenanceRecords", Path: "maintenance",
		Module:          "maintenance/maintenanceApi.ts",
		MutationsModule: "maintenance/maintenanceApiMutations.ts",
		Ownership:       Ownership{Via: "vehicle_id", Table: "vehicles", Field: "ownerId"},
		Verbs:           []string{"create", "read", "update", "delete"},
		Expose: []ExposeField{
			{Field: "vehicleId", Wire: "vehicle_id", Ref: "vehicles"},
			{Field: "serviceName", Wire: "service_name"},
		},
	}
}
