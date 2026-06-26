package main

import (
	"encoding/json"
	"strings"
	"testing"
)

// photosTable models the shared vehiclePortfolioItems table backing the
// hand-written photos media resource: a parent vehicle FK, an album FK, the R2
// object key, an optional caption, and the soft-delete timestamp column.
func photosTable() TableInfo {
	return TableInfo{Name: "vehiclePortfolioItems", Fields: []FieldInfo{
		{Name: "vehicleId", Type: "id", IsID: true, TableRef: "vehicles"},
		{Name: "albumId", Type: "id", IsID: true, TableRef: "vehiclePortfolioAlbums"},
		{Name: "key", Type: "string", Optional: true},
		{Name: "caption", Type: "string", Optional: true},
		{Name: "deletedAt", Type: "number", Optional: true},
	}}
}

// photosSpec is the proposed photos overlay: FK-via-parent ownership, soft
// delete on deletedAt, and a media:{presign:true,…} block describing the R2
// presign escape hatch the hand-written photosRoutes hard-codes.
func photosSpec() ResourceSpec {
	return ResourceSpec{
		Table: "vehiclePortfolioItems", Path: "photos",
		Module:     "vehicles/photosApi.ts",
		Ownership:  Ownership{Via: "vehicle_id", Table: "vehicles", Field: "ownerId"},
		Verbs:      []string{"create", "read", "update", "delete"},
		SoftDelete: "deletedAt",
		Media: &MediaSpec{
			Presign:        true,
			KeyField:       "key",
			BucketField:    "bucket",
			R2Action:       "r2/genericStructuredUpload.generateEntityUploadUrl",
			EntityType:     "vehicle",
			MediaType:      "gallery",
			IsPublic:       true,
			OwnershipCheck: "vehicles/photosApi.canUploadToAlbum",
		},
		Expose: []ExposeField{
			{Field: "vehicleId", Wire: "vehicle_id", Ref: "vehicles"},
			{Field: "albumId", Wire: "album_id", Ref: "vehiclePortfolioAlbums"},
			// The R2 key is write-only (accepted on create, never echoed) and
			// required (always carried from presign), matching the hand-written
			// photo commit contract; the readable url is the output projection.
			{Field: "key", Wire: "key", WriteOnly: true, Required: true},
			{Field: "caption", Wire: "caption"},
		},
		Computed: map[string]ComputedField{
			"url":        {As: "r2Url", From: "key"},
			"created_at": {As: "iso8601", From: "createdAt"},
		},
	}
}

// TestMediaSpecUnmarshal locks parsing of the media:{} overlay block: every
// concrete knob the hand-written presign route hard-codes round-trips, and
// presign:false / absent leaves the resource unaffected.
func TestMediaSpecUnmarshal(t *testing.T) {
	raw := `{
	  "table":"vehiclePortfolioItems","path":"photos","module":"vehicles/photosApi.ts",
	  "ownership":{"via":"vehicle_id","table":"vehicles","field":"ownerId"},
	  "verbs":["create","read","update","delete"],"softDelete":"deletedAt",
	  "media":{"presign":true,"keyField":"key","bucketField":"bucket",
	    "r2Action":"r2/genericStructuredUpload.generateEntityUploadUrl",
	    "entityType":"vehicle","mediaType":"gallery","isPublic":true,
	    "ownershipCheck":"vehicles/photosApi.canUploadToAlbum"},
	  "expose":[{"field":"vehicleId","wire":"vehicle_id","ref":"vehicles"},"key"]
	}`
	var spec ResourceSpec
	if err := json.Unmarshal([]byte(raw), &spec); err != nil {
		t.Fatalf("unmarshal media spec: %v", err)
	}
	if spec.Media == nil {
		t.Fatal("media block did not parse")
	}
	m := spec.Media
	if !m.Presign || m.KeyField != "key" || m.BucketField != "bucket" {
		t.Errorf("media core knobs wrong: %+v", m)
	}
	if m.R2Action != "r2/genericStructuredUpload.generateEntityUploadUrl" ||
		m.EntityType != "vehicle" || m.MediaType != "gallery" || !m.IsPublic ||
		m.OwnershipCheck != "vehicles/photosApi.canUploadToAlbum" {
		t.Errorf("media R2 knobs wrong: %+v", m)
	}
}

// TestMediaSpecAbsentIsNil locks the OPTIONAL contract: a resource with no media
// block parses with a nil Media (and so emits no presign route).
func TestMediaSpecAbsentIsNil(t *testing.T) {
	var spec ResourceSpec
	if err := json.Unmarshal([]byte(`{"table":"vehicles","path":"vehicles","module":"vehicles/vehiclesApi.ts","verbs":["read"],"expose":["make"]}`), &spec); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if spec.Media != nil {
		t.Errorf("absent media must parse to nil, got %+v", spec.Media)
	}
}

// TestResolveCarriesMedia locks that ResolveResource forwards the media block
// onto the resolved resource.
func TestResolveCarriesMedia(t *testing.T) {
	r := mustResolve(t, photosTable(), photosSpec())
	if r.Media == nil {
		t.Fatal("resolved resource lost its media block")
	}
	if !r.Media.Presign || r.Media.KeyField != "key" {
		t.Errorf("resolved media wrong: %+v", r.Media)
	}
}

// TestEmitRoutesPresignRoute is the core lock: a media:{presign:true} resource
// emits a POST /api/v1/<path>/presign companion route that authorizes write,
// validates the FK fields, calls the ownership check, runs the R2 presign action
// with the concrete entityType/mediaType/isPublic, and returns { upload_url, key }.
func TestEmitRoutesPresignRoute(t *testing.T) {
	src := EmitRoutesTS(mustResolve(t, photosTable(), photosSpec()))

	for _, frag := range []string{
		// the action import is required for ctx.runAction
		`import { internal, api } from "../_generated/api";`,
		// the presign route itself, at the collection path + /presign
		`path: "/api/v1/photos/presign",`,
		`method: "POST",`,
		// write-scoped, like every mutating route
		`const auth = await authorize(ctx, req, "photos:write");`,
		// FK validation: both id-ref fields are required in the presign body
		`if (typeof b.vehicle_id !== "string" || typeof b.album_id !== "string")`,
		// ownership check call with camelCased FK args
		`internal.vehicles.photosApi.canUploadToAlbum`,
		`actorId: auth.actor.userId,`,
		`vehicleId: b.vehicle_id as Id<"vehicles">,`,
		`albumId: b.album_id as Id<"vehiclePortfolioAlbums">,`,
		// R2 presign action with the concrete, overlay-driven constants
		`api.r2.genericStructuredUpload.generateEntityUploadUrl`,
		`entityType: "vehicle",`,
		`mediaType: "gallery",`,
		`isPublic: true,`,
		`entityId: b.vehicle_id,`,
		// extension derivation from content_type
		`fileExtension: extFromContentType(`,
		// the wire response (bucket form, since photosSpec sets bucketField)
		`upload_url: result.uploadUrl,`,
		`key: result.key,`,
	} {
		if !strings.Contains(src, frag) {
			t.Errorf("presign route missing fragment:\n  %s\nGOT:\n%s", frag, src)
		}
	}

	// extFromContentType helper must be emitted exactly once.
	if c := strings.Count(src, "function extFromContentType("); c != 1 {
		t.Errorf("extFromContentType must be emitted once, got %d:\n%s", c, src)
	}

	// The presign route must come BEFORE the plain CREATE route so the more
	// specific path is matched first (Convex matches exact `path` before
	// `pathPrefix`, but ordering keeps the generated file readable + stable).
	presignIdx := strings.Index(src, `path: "/api/v1/photos/presign",`)
	createIdx := strings.Index(src, "// CREATE")
	if presignIdx < 0 || createIdx < 0 || presignIdx >= createIdx {
		t.Errorf("presign route must be emitted before the CREATE route")
	}
}

// TestEmitRoutesPresignBucketReturned locks the bucketField knob: when
// media.bucketField is set the presign response also echoes the bucket from the
// R2 action result.
func TestEmitRoutesPresignBucketReturned(t *testing.T) {
	src := EmitRoutesTS(mustResolve(t, photosTable(), photosSpec()))
	if !strings.Contains(src, "bucket: result.bucket") {
		t.Errorf("bucketField set must echo result.bucket in the presign response:\n%s", src)
	}
}

// TestEmitRoutesPresignNoBucket locks that omitting bucketField drops the bucket
// from the presign response (key-only, like a public gallery row).
func TestEmitRoutesPresignNoBucket(t *testing.T) {
	spec := photosSpec()
	spec.Media.BucketField = ""
	src := EmitRoutesTS(mustResolve(t, photosTable(), spec))
	if strings.Contains(src, "bucket: result.bucket") {
		t.Errorf("no bucketField must not echo a bucket:\n%s", src)
	}
	if !strings.Contains(src, "return jsonOk({ upload_url: result.uploadUrl, key: result.key });") {
		t.Errorf("no-bucket response must still return upload_url + key:\n%s", src)
	}
}

// TestEmitRoutesKeyIsCreateField locks that the R2 object key is a NORMAL create
// input field on the commit route — the binary bytes are NEVER routed through
// Convex (no v.bytes), the client PUTs to the presigned URL and submits the key.
func TestEmitRoutesKeyIsCreateField(t *testing.T) {
	src := EmitRoutesTS(mustResolve(t, photosTable(), photosSpec()))
	// key is read off the JSON body in readInput as a required string field.
	if !strings.Contains(src, "typeof b.key !== \"string\"") {
		t.Errorf("key must be a required create field read from the body:\n%s", src)
	}
	// Bytes must never flow through the API surface.
	if strings.Contains(src, "v.bytes(") || strings.Contains(src, "ArrayBuffer") {
		t.Errorf("media bytes must never be routed through Convex:\n%s", src)
	}
}

// TestEmitApiKeyIsCreateFieldUrlComputed locks the Api.ts side: the media key is
// a writable create field (in the input validator + toCreateArgs), while the
// readable URL is an output-only computed r2Urls field derived FROM the key.
func TestEmitApiKeyIsCreateFieldUrlComputed(t *testing.T) {
	src := EmitApiTS(mustResolve(t, photosTable(), photosSpec()))

	// key is a writable input field.
	inputBlock := between(src, "export const vehiclePortfolioItemApiInput = {", "};")
	if !strings.Contains(inputBlock, "key: v.string()") {
		t.Errorf("key must be a create input field:\n%s", inputBlock)
	}
	// key is written in toCreateArgs.
	createBody := between(src, "export function toCreateArgs(", "skipAuth: true,")
	if !strings.Contains(createBody, "key: input.key,") {
		t.Errorf("toCreateArgs must write the key column:\n%s", createBody)
	}
	// url is a computed single-url (r2Url) output, derived from the key — one
	// media row resolves to one readable URL, matching the hand-written photo
	// projection (`getR2Url(doc.key) ?? ""`).
	outputBlock := between(src, "ApiOutput = v.object({", "});")
	if !strings.Contains(outputBlock, "url: v.string()") {
		t.Errorf("url must be a computed r2Url (single string) output:\n%s", outputBlock)
	}
	if strings.Contains(outputBlock, "url: v.array(v.string())") {
		t.Errorf("media url must be a single string, not an array:\n%s", outputBlock)
	}
}

// mediaTypePhotosTable models vehiclePortfolioItems with the real mediaType
// column: a required v.union(v.literal("image"), v.literal("video")) — there is
// NO "document" mediaType in the schema, and content_type is NOT a column. A
// VIDEO uploaded via the API must persist as mediaType "video", so the column
// has to be client-declarable on create rather than hardcoded.
func mediaTypePhotosTable() TableInfo {
	t := photosTable()
	t.Fields = append(t.Fields, FieldInfo{
		Name: "mediaType", Type: "union", Literals: []string{"image", "video"},
	})
	return t
}

// mediaTypePhotosSpec is the fixed photos overlay: instead of a hardcoded
// createConstant mediaType:"image", media_type is an exposed CREATE input enum
// [image, video] defaulted to image. The client declares the media kind so a
// video commit persists as a video.
func mediaTypePhotosSpec() ResourceSpec {
	spec := photosSpec()
	spec.Expose = append(spec.Expose, ExposeField{
		Field: "mediaType", Wire: "media_type", Enum: []string{"image", "video"},
	})
	spec.Defaults = map[string]any{"mediaType": "image"}
	return spec
}

// TestEmitApiMediaTypeIsClientDeclaredCreateField is the core Wave C
// carry-forward lock: media_type is a client-declarable CREATE input enum that
// flows into the mediaType column (defaulted to "image"), and is NOT a hardcoded
// constant. Before the fix toCreateArgs emitted `mediaType: "image" as const`,
// silently coercing every VIDEO to an image.
func TestEmitApiMediaTypeIsClientDeclaredCreateField(t *testing.T) {
	src := EmitApiTS(mustResolve(t, mediaTypePhotosTable(), mediaTypePhotosSpec()))

	// media_type is an OPTIONAL enum input (optional because it defaults), so the
	// client may declare image|video but is not forced to.
	inputBlock := between(src, "ApiInput = {", "};")
	wantInput := `media_type: v.optional(v.union(v.literal("image"), v.literal("video")))`
	if !strings.Contains(inputBlock, wantInput) {
		t.Errorf("media_type must be an optional image|video enum input, want %q:\n%s", wantInput, inputBlock)
	}

	// toCreateArgs must write the CLIENT's value, defaulted to "image" — never a
	// hardcoded constant.
	createBody := between(src, "export function toCreateArgs(", "skipAuth: true,")
	wantCreate := `mediaType: input.media_type ?? "image",`
	if !strings.Contains(createBody, wantCreate) {
		t.Errorf("toCreateArgs must persist the client media_type defaulted to image, want %q:\n%s", wantCreate, createBody)
	}
	if strings.Contains(createBody, `mediaType: "image" as const`) {
		t.Errorf("toCreateArgs must NOT hardcode mediaType — a video would persist as image:\n%s", createBody)
	}
}

// TestEmitRoutesNoMediaUnaffected is the OPTIONAL-feature regression lock: a
// resource WITHOUT a media block emits NO presign route, NO extFromContentType
// helper, and keeps the plain `internal`-only api import.
func TestEmitRoutesNoMediaUnaffected(t *testing.T) {
	src := EmitRoutesTS(mustResolve(t, albumsTable(), albumsViaParentSpec()))
	for _, banned := range []string{
		"/presign",
		"function extFromContentType(",
		"generateEntityUploadUrl",
		`import { internal, api } from "../_generated/api";`,
	} {
		if strings.Contains(src, banned) {
			t.Errorf("non-media resource must NOT emit %q:\n%s", banned, src)
		}
	}
	// the plain internal-only import must remain.
	if !strings.Contains(src, `import { internal } from "../_generated/api";`) {
		t.Errorf("non-media resource must keep the internal-only api import:\n%s", src)
	}
}
