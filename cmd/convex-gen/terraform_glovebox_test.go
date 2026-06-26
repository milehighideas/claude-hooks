package main

import (
	"strings"
	"testing"
)

// gloveboxTable models the shared vehiclePortfolioItems table for the glovebox
// documents resource: the same physical table photos uses, plus the optional
// document-metadata columns.
func gloveboxTable() TableInfo {
	return TableInfo{Name: "vehiclePortfolioItems", Fields: []FieldInfo{
		{Name: "vehicleId", Type: "id", IsID: true, TableRef: "vehicles"},
		{Name: "albumId", Type: "id", IsID: true, TableRef: "vehiclePortfolioAlbums"},
		{Name: "key", Type: "string", Optional: true},
		{Name: "caption", Type: "string", Optional: true},
		{Name: "documentCategory", Type: "string", Optional: true},
		{Name: "documentExpiresAt", Type: "number", Optional: true},
		{Name: "documentNotes", Type: "string", Optional: true},
		{Name: "deletedAt", Type: "number", Optional: true},
	}}
}

// gloveboxSpec is the proposed glovebox_documents overlay: a Name override (so
// its symbols/files don't collide with photos on the shared table), a scope
// block (so a read never leaks a gallery photo), FK-via-parent ownership, soft
// delete, and the media presign block.
func gloveboxSpec() ResourceSpec {
	immutableTrue := true
	return ResourceSpec{
		Table: "vehiclePortfolioItems", Path: "glovebox_documents",
		Module:     "vehicles/gloveboxApi.ts",
		Name:       "gloveboxDocument",
		Ownership:  Ownership{Via: "vehicle_id", Table: "vehicles", Field: "ownerId"},
		Verbs:      []string{"create", "read", "update", "delete"},
		SoftDelete: "deletedAt",
		Scope: &ScopeSpec{
			Via: "albumId", Table: "vehiclePortfolioAlbums",
			Field: "albumType", Equals: "documents",
		},
		Media: &MediaSpec{
			Presign:        true,
			KeyField:       "key",
			BucketField:    "bucket",
			R2Action:       "r2/genericStructuredUpload.generateEntityUploadUrl",
			EntityType:     "vehicle",
			MediaType:      "document",
			IsPublic:       false,
			OwnershipCheck: "vehicles/photosApi.canUploadToAlbum",
		},
		Expose: []ExposeField{
			{Field: "vehicleId", Wire: "vehicle_id", Ref: "vehicles"},
			{Field: "albumId", Wire: "album_id", Ref: "vehiclePortfolioAlbums", ReadOnly: true, Immutable: &immutableTrue},
			{Field: "key", Wire: "key", WriteOnly: true, Required: true},
			{Field: "caption", Wire: "caption"},
			{Field: "documentCategory", Wire: "document_category", Enum: []string{
				"title", "registration", "insurance", "purchase", "warranty",
				"inspection", "maintenance", "recall", "permit", "other",
			}},
			{Field: "documentExpiresAt", Wire: "expires_at"},
			{Field: "documentNotes", Wire: "notes"},
		},
		CreateConstants: map[string]any{"mediaType": "image"},
		Computed: map[string]ComputedField{
			"url":        {As: "r2Url", From: "key"},
			"created_at": {As: "iso8601", From: "createdAt"},
		},
	}
}

// TestNameOverrideDrivesSymbols locks that the Name override drives every emitted
// symbol (validators + types + register fn) so two resources on one table do not
// collide on table-derived names.
func TestNameOverrideDrivesSymbols(t *testing.T) {
	r := mustResolve(t, gloveboxTable(), gloveboxSpec())
	api := EmitApiTS(r)
	routes := EmitRoutesTS(r)
	types := EmitApiTypesTS(r)

	for _, frag := range []string{
		"export const gloveboxDocumentApiInput = {",
		"export const gloveboxDocumentApiOutput = v.object({",
	} {
		if !strings.Contains(api, frag) {
			t.Errorf("Api.ts missing name-driven symbol %q", frag)
		}
	}
	if strings.Contains(api, "vehiclePortfolioItemApiInput") {
		t.Errorf("Api.ts must NOT use the table-derived symbol when Name is set:\n%s", api)
	}
	if !strings.Contains(types, "export type GloveboxDocumentApiInput") {
		t.Errorf("types file missing name-driven type:\n%s", types)
	}
	if !strings.Contains(routes, "export function registerGloveboxDocumentRoutes(") {
		t.Errorf("routes missing name-driven register fn:\n%s", routes)
	}
	if !strings.Contains(routes, `import type {
  GloveboxDocumentApiInput,
  GloveboxDocumentApiPatch,
} from "../vehicles/types/gloveboxDocumentApi.types";`) {
		t.Errorf("routes must import the name-driven wire-types file:\n%s", routes)
	}
}

// TestRoutesFileBasenameDisambiguated locks that a Name override keys the routes
// file on the singular base (not the shared table), so photos and glovebox do
// not overwrite each other.
func TestRoutesFileBasenameDisambiguated(t *testing.T) {
	gb := mustResolve(t, gloveboxTable(), gloveboxSpec())
	if got := routesFileBasename(gb); got != "gloveboxDocumentRoutes.ts" {
		t.Errorf("glovebox routes basename = %q, want gloveboxDocumentRoutes.ts", got)
	}
	// Photos WITH a Name override keys on the singular base too — so the two
	// resources on vehiclePortfolioItems write distinct routes files.
	ph := photosSpec()
	ph.Name = "photo"
	phr := mustResolve(t, photosTable(), ph)
	if got := routesFileBasename(phr); got != "photoRoutes.ts" {
		t.Errorf("photos routes basename = %q, want photoRoutes.ts", got)
	}
	// A resource WITHOUT a Name override keeps the historical <table>Routes.ts.
	al := mustResolve(t, albumsTable(), albumsViaParentSpec())
	if got := routesFileBasename(al); got != "albumsRoutes.ts" {
		t.Errorf("no-override basename = %q, want albumsRoutes.ts", got)
	}
}

// TestScopeGuardEmitted locks the scope discriminator: the read/update/delete
// handlers append an async inScope guard that resolves the album parent and
// checks its albumType, so a glovebox read never returns a gallery photo.
func TestScopeGuardEmitted(t *testing.T) {
	src := EmitApiTS(mustResolve(t, gloveboxTable(), gloveboxSpec()))
	for _, frag := range []string{
		"async function inScopeGloveboxDocument(",
		`const scopeParent = await ctx.db.get("vehiclePortfolioAlbums", doc.albumId);`,
		`return !!scopeParent && scopeParent.albumType === "documents";`,
		"|| !(await inScopeGloveboxDocument(ctx, doc))",
	} {
		if !strings.Contains(src, frag) {
			t.Errorf("scope guard missing fragment:\n  %s\nGOT:\n%s", frag, src)
		}
	}
}

// TestScopeAbsentNoGuard locks the OPTIONAL contract: a resource without a scope
// block emits no inScope helper or guard (photos shares the table but is the
// default scope).
func TestScopeAbsentNoGuard(t *testing.T) {
	src := EmitApiTS(mustResolve(t, photosTable(), photosSpec()))
	for _, banned := range []string{"inScope", "scopeParent"} {
		if strings.Contains(src, banned) {
			t.Errorf("non-scoped resource must NOT emit %q:\n%s", banned, src)
		}
	}
}

// TestGloveboxCreateForwardsConstantsNotAlbum locks the create surface: the
// album_id FK is readonly (resolved by the hand-written _create via the
// documents-album ensure), so toCreateArgs forwards vehicle_id + the document
// metadata + the mediaType constant, but never album_id.
func TestGloveboxCreateForwardsConstantsNotAlbum(t *testing.T) {
	src := EmitApiTS(mustResolve(t, gloveboxTable(), gloveboxSpec()))
	createBody := between(src, "export function toCreateArgs(", "skipAuth: true,")
	for _, frag := range []string{
		"actorId: ownerId,",
		"vehicleId: input.vehicle_id,",
		"key: input.key,",
		"documentCategory: input.document_category,",
		"documentExpiresAt: input.expires_at,",
		"documentNotes: input.notes,",
		`mediaType: "image" as const,`,
	} {
		if !strings.Contains(createBody, frag) {
			t.Errorf("toCreateArgs missing %q:\n%s", frag, createBody)
		}
	}
	if strings.Contains(createBody, "albumId: input.album_id") {
		t.Errorf("album_id is readonly and must NOT be in toCreateArgs:\n%s", createBody)
	}
}

// TestMediaKeyWriteOnlyUrlOutput locks the media key/url surface partition: the
// R2 key is a write-only input (in the input validator + toCreateArgs, NOT in
// the output), while the readable url is an output-only computed string —
// matching the hand-written photoApiInput (has key) / photoApiOutput (has url).
func TestMediaKeyWriteOnlyUrlOutput(t *testing.T) {
	src := EmitApiTS(mustResolve(t, gloveboxTable(), gloveboxSpec()))
	inputBlock := between(src, "export const gloveboxDocumentApiInput = {", "};")
	if !strings.Contains(inputBlock, "key: v.string()") {
		t.Errorf("key must be in the input validator:\n%s", inputBlock)
	}
	outputBlock := between(src, "gloveboxDocumentApiOutput = v.object({", "});")
	if strings.Contains(outputBlock, "key:") {
		t.Errorf("write-only key must NOT appear in the output:\n%s", outputBlock)
	}
	if !strings.Contains(outputBlock, "url: v.string()") {
		t.Errorf("url must be the output-only computed string:\n%s", outputBlock)
	}
}

// TestGloveboxPresignDocumentConstants locks the glovebox presign route uses the
// document media constants (mediaType "document", isPublic false) — distinct
// from the photos gallery presign.
func TestGloveboxPresignDocumentConstants(t *testing.T) {
	src := EmitRoutesTS(mustResolve(t, gloveboxTable(), gloveboxSpec()))
	for _, frag := range []string{
		`path: "/api/v1/glovebox_documents/presign",`,
		`mediaType: "document",`,
		"isPublic: false,",
		`api.r2.genericStructuredUpload.generateEntityUploadUrl`,
		`internal.vehicles.photosApi.canUploadToAlbum`,
	} {
		if !strings.Contains(src, frag) {
			t.Errorf("glovebox presign missing %q:\n%s", frag, src)
		}
	}
}
