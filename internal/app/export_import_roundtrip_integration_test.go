// export_import_roundtrip_integration_test.go round-trips campaigns through
// the real export and import handlers against MariaDB, the way an owner
// does with "Export ZIP (with media)" and the import page.
//
// Skipped under -short. Run with `make test-int-local`.
package app

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io/fs"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/database"
	"github.com/keyxmakerx/chronicle/internal/plugins/addons"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/plugins/maps"
	"github.com/keyxmakerx/chronicle/internal/plugins/media"
	"github.com/keyxmakerx/chronicle/internal/widgets/relations"
	"github.com/keyxmakerx/chronicle/internal/widgets/tags"
)

// roundTripHarness holds the real services one campaign round trip needs,
// wired the way routes.go wires them.
type roundTripHarness struct {
	t         *testing.T
	db        *sql.DB
	campaigns campaigns.CampaignService
	entities  entities.EntityService
	addons    addons.AddonService
	media     media.MediaService
	maps      maps.MapService
	drawings  maps.DrawingService
	handler   *campaigns.ExportHandler
}

func newRoundTripHarness(t *testing.T) *roundTripHarness {
	t.Helper()
	db := openGalleryTestDB(t)
	sub, err := fs.Sub(maps.MigrationsFS, database.PluginMigrationsSubdir)
	if err != nil {
		t.Fatalf("maps sub-FS: %v", err)
	}
	for _, res := range database.RunPluginMigrations(db, []database.PluginSchema{{Slug: "maps", MigrationsFS: sub}}) {
		if !res.Healthy {
			t.Fatalf("%s plugin migrations did not apply: %v", res.Slug, res.Error)
		}
	}

	entitySvc := entities.NewEntityService(
		entities.NewEntityRepository(db), entities.NewEntityTypeRepository(db), entities.NewEntityPermissionRepository(db))
	campaignSvc := campaigns.NewCampaignService(campaigns.NewCampaignRepository(db), nil, nil, entitySvc, "http://localhost")
	mediaSvc := media.NewMediaService(media.NewMediaRepository(db), t.TempDir(), 10*1024*1024)
	addonSvc := addons.NewAddonService(addons.NewAddonRepository(db))
	if err := addonSvc.SeedInstalledAddons(context.Background()); err != nil {
		t.Fatalf("seed addons: %v", err)
	}
	addonSvc.SetPresetApplier(newPresetApplier(entitySvc))
	entitySvc.SetAddonChecker(addonSvc)
	entitySvc.SetMediaVerifier(&entityMediaVerifierAdapter{svc: mediaSvc})
	blocks := entities.NewBlockRegistry()
	entities.RegisterCoreBlocks(blocks)
	entitySvc.SetBlockRegistry(blocks)
	mapSvc := maps.NewMapService(maps.NewMapRepository(db))
	drawingSvc := maps.NewDrawingService(maps.NewDrawingRepository(db))
	entitySvc.SetMapVerifier(&entityMapVerifierAdapter{svc: mapSvc})

	exportSvc := campaigns.NewExportImportService(campaignSvc)
	tagSvc := tags.NewTagService(tags.NewTagRepository(db))
	relSvc := relations.NewRelationService(relations.NewRelationRepository(db))
	exportSvc.SetEntityExporter(&entityExportAdapter{entitySvc: entitySvc, tagSvc: tagSvc, relationSvc: relSvc})
	exportSvc.SetMapExporter(&mapExportAdapter{mapSvc: mapSvc, drawingSvc: drawingSvc})
	exportSvc.SetAddonExporter(&addonExportAdapter{svc: addonSvc})
	exportSvc.SetMediaExporter(&mediaExportAdapter{svc: mediaSvc})
	exportSvc.SetMediaBundler(&mediaBundleAdapter{svc: mediaSvc})
	exportSvc.SetEntityImporter(&entityImportAdapter{entitySvc: entitySvc, tagSvc: tagSvc, relationSvc: relSvc})
	exportSvc.SetMapImporter(&mapImportAdapter{mapSvc: mapSvc, drawingSvc: drawingSvc})
	exportSvc.SetAddonImporter(&addonImportAdapter{svc: addonSvc})

	return &roundTripHarness{
		t: t, db: db, campaigns: campaignSvc, entities: entitySvc, addons: addonSvc,
		media: mediaSvc, maps: mapSvc, drawings: drawingSvc,
		handler: campaigns.NewExportHandler(exportSvc),
	}
}

// newUser inserts a user row and returns its id.
func (h *roundTripHarness) newUser(label string) string {
	h.t.Helper()
	id := galleryTestUUID(h.t)
	mustGalleryExec(h.t, h.db, `INSERT INTO users (id, email, display_name, password_hash) VALUES (?, ?, ?, ?)`,
		id, "roundtrip-"+label+"-"+id+"@example.test", "Round Trip "+label, "x")
	return id
}

// png returns a small, valid PNG whose pixel colour is set by seed, so each
// picture hashes differently and none is deduplicated into another.
func roundTripPNG(t *testing.T, seed uint8) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for x := 0; x < 4; x++ {
		for y := 0; y < 4; y++ {
			img.Set(x, y, color.RGBA{R: seed, G: 255 - seed, B: seed / 2, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}

func (h *roundTripHarness) upload(campaignID, userID string, seed uint8) string {
	h.t.Helper()
	b := roundTripPNG(h.t, seed)
	f, err := h.media.Upload(context.Background(), media.UploadInput{
		CampaignID: campaignID, UploadedBy: userID, OriginalName: fmt.Sprintf("pic-%d.png", seed),
		MimeType: "image/png", FileSize: int64(len(b)), UsageType: "attachment", FileBytes: b,
	})
	if err != nil {
		h.t.Fatalf("upload: %v", err)
	}
	return f.ID
}

// exportZip runs the real export handler with ?include_media=1.
func (h *roundTripHarness) exportZip(campaign *campaigns.Campaign, userID string) []byte {
	h.t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/campaigns/"+campaign.ID+"/export?include_media=1", nil)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.Set("auth_user_id", userID)
	c.Set("campaign_context", &campaigns.CampaignContext{Campaign: campaign, MemberRole: campaigns.RoleOwner, IsMember: true})
	if err := h.handler.ExportCampaign(c); err != nil {
		h.t.Fatalf("ExportCampaign: %v", err)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/zip" {
		h.t.Fatalf("export Content-Type = %q, want application/zip", ct)
	}
	return rec.Body.Bytes()
}

// importBlob runs the real import handler and returns the new campaign id
// plus the response body (the loss report when the import was partial).
func (h *roundTripHarness) importBlob(userID, filename string, blob []byte) (string, string) {
	h.t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	part, err := mw.CreateFormFile("file", filename)
	if err != nil {
		h.t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write(blob); err != nil {
		h.t.Fatalf("write form file: %v", err)
	}
	_ = mw.Close()
	req := httptest.NewRequest(http.MethodPost, "/campaigns/import", &body)
	req.Header.Set(echo.HeaderContentType, mw.FormDataContentType())
	req.Header.Set("HX-Request", "true")
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.Set("auth_user_id", userID)
	if err := h.handler.ImportCampaign(c); err != nil {
		h.t.Fatalf("ImportCampaign: %v", err)
	}
	var newID string
	if err := h.db.QueryRow(`SELECT id FROM campaigns WHERE created_by = ? ORDER BY created_at DESC, id LIMIT 1`, userID).Scan(&newID); err != nil {
		h.t.Fatalf("find imported campaign: %v", err)
	}
	return newID, rec.Body.String()
}

// mediaCampaign returns the campaign a media id belongs to, or "" when the
// id names no file.
func (h *roundTripHarness) mediaCampaign(id string) string {
	h.t.Helper()
	var cid sql.NullString
	err := h.db.QueryRow(`SELECT campaign_id FROM media_files WHERE id = ?`, id).Scan(&cid)
	if err == sql.ErrNoRows {
		return ""
	}
	if err != nil {
		h.t.Fatalf("media lookup: %v", err)
	}
	return cid.String
}

// sourceWithPlayerCharacter creates a campaign the way an owner does: the
// default categories, the Player Character addon on (which premakes its
// category under Characters), and one character on it.
func (h *roundTripHarness) sourceWithPlayerCharacter(owner string) (*campaigns.Campaign, string) {
	h.t.Helper()
	ctx := context.Background()
	src, err := h.campaigns.Create(ctx, owner, campaigns.CreateCampaignInput{Name: "Round Trip Source"})
	if err != nil {
		h.t.Fatalf("create source campaign: %v", err)
	}
	if err := h.addons.EnableForCampaignBySlug(ctx, src.ID, entities.AddonPlayerCharacterClaiming, owner); err != nil {
		h.t.Fatalf("enable PC addon: %v", err)
	}
	pcTypes, err := h.entities.GetEntityTypesByPresetCategory(ctx, src.ID, entities.PresetCategoryPlayerCharacter)
	if err != nil || len(pcTypes) != 1 {
		h.t.Fatalf("source PC types = %d (err %v), want 1", len(pcTypes), err)
	}
	hero, err := h.entities.Create(ctx, src.ID, owner, entities.CreateEntityInput{Name: "Aria", EntityTypeID: pcTypes[0].ID})
	if err != nil {
		h.t.Fatalf("create PC: %v", err)
	}
	return src, hero.ID
}

// typeRow is one entity type as the database holds it.
type typeRow struct {
	slug, name string
	preset     sql.NullString
	parentSlug sql.NullString
	claimable  sql.NullBool
}

func (h *roundTripHarness) types(campaignID string) []typeRow {
	h.t.Helper()
	rows, err := h.db.Query(`
		SELECT t.slug, t.name, t.preset_category, p.slug, t.claimable
		  FROM entity_types t LEFT JOIN entity_types p ON p.id = t.parent_type_id
		 WHERE t.campaign_id = ? ORDER BY t.slug`, campaignID)
	if err != nil {
		h.t.Fatalf("list types: %v", err)
	}
	defer rows.Close()
	var out []typeRow
	for rows.Next() {
		var r typeRow
		if err := rows.Scan(&r.slug, &r.name, &r.preset, &r.parentSlug, &r.claimable); err != nil {
			h.t.Fatalf("scan type: %v", err)
		}
		out = append(out, r)
	}
	return out
}

// TestCampaignExportImport_PlayerCharacters_DBRoundTrip: a campaign with the
// Player Character addon comes back with its characters on the one Player
// Character category, nested where it was, and with each default category
// once. Before add-ons were restored first, the category was refused (the
// addon was still off), every character on it was dropped, and the import
// also carried a second copy of every default category.
func TestCampaignExportImport_PlayerCharacters_DBRoundTrip(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test requires a database; skipped under -short")
	}
	h := newRoundTripHarness(t)
	owner := h.newUser("owner")
	src, _ := h.sourceWithPlayerCharacter(owner)

	newID, body := h.importBlob(h.newUser("importer"), "campaign.zip", h.exportZip(src, owner))
	if newID == src.ID {
		t.Fatalf("import made no new campaign")
	}

	want, got := h.types(src.ID), h.types(newID)
	if len(got) != len(want) {
		t.Errorf("imported campaign has %d categories, source has %d:\n got %+v\nwant %+v", len(got), len(want), got, want)
	}
	for i := range want {
		if i >= len(got) {
			break
		}
		if got[i] != want[i] {
			t.Errorf("category %d = %+v, want %+v", i, got[i], want[i])
		}
	}

	var preset sql.NullString
	if err := h.db.QueryRow(`
		SELECT et.preset_category FROM entities e JOIN entity_types et ON et.id = e.entity_type_id
		 WHERE e.campaign_id = ? AND e.name = 'Aria'`, newID).Scan(&preset); err != nil {
		t.Fatalf("imported character not found (response: %s): %v", body, err)
	}
	if preset.String != entities.PresetCategoryPlayerCharacter {
		t.Errorf("imported character's category preset = %q, want %q", preset.String, entities.PresetCategoryPlayerCharacter)
	}
	if strings.Contains(body, "Imported with losses") {
		t.Errorf("round trip reported losses:\n%s", body)
	}
}
