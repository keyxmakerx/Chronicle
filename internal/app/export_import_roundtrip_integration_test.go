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
	"github.com/keyxmakerx/chronicle/internal/patch"
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
	places    entities.PlaceService
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

	placeSvc := entities.NewPlaceService(entities.NewEntityRepository(db), entities.NewPlaceRepository(db))
	exportSvc := campaigns.NewExportImportService(campaignSvc)
	tagSvc := tags.NewTagService(tags.NewTagRepository(db))
	relSvc := relations.NewRelationService(relations.NewRelationRepository(db))
	exportSvc.SetEntityExporter(&entityExportAdapter{entitySvc: entitySvc, tagSvc: tagSvc, relationSvc: relSvc, placeSvc: placeSvc})
	exportSvc.SetMapExporter(&mapExportAdapter{mapSvc: mapSvc, drawingSvc: drawingSvc})
	exportSvc.SetAddonExporter(&addonExportAdapter{svc: addonSvc})
	exportSvc.SetMediaExporter(&mediaExportAdapter{svc: mediaSvc})
	exportSvc.SetMediaBundler(&mediaBundleAdapter{svc: mediaSvc})
	exportSvc.SetEntityImporter(&entityImportAdapter{entitySvc: entitySvc, tagSvc: tagSvc, relationSvc: relSvc, placeSvc: placeSvc})
	exportSvc.SetMapImporter(&mapImportAdapter{mapSvc: mapSvc, drawingSvc: drawingSvc})
	exportSvc.SetAddonImporter(&addonImportAdapter{svc: addonSvc})
	exportSvc.SetMediaImporter(&mediaImportAdapter{svc: mediaSvc})

	return &roundTripHarness{
		t: t, db: db, campaigns: campaignSvc, entities: entitySvc, places: placeSvc, addons: addonSvc,
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
	return h.export(campaign, userID, "?include_media=1", "application/zip")
}

// export runs the real export handler and checks the response type.
func (h *roundTripHarness) export(campaign *campaigns.Campaign, userID, query, wantType string) []byte {
	h.t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/campaigns/"+campaign.ID+"/export"+query, nil)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.Set("auth_user_id", userID)
	c.Set("campaign_context", &campaigns.CampaignContext{Campaign: campaign, MemberRole: campaigns.RoleOwner, IsMember: true})
	if err := h.handler.ExportCampaign(c); err != nil {
		h.t.Fatalf("ExportCampaign: %v", err)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, wantType) {
		h.t.Fatalf("export Content-Type = %q, want %s", ct, wantType)
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

// typeID returns the id of a campaign's category by slug.
func (h *roundTripHarness) typeID(campaignID, slug string) int {
	h.t.Helper()
	var id int
	if err := h.db.QueryRow(`SELECT id FROM entity_types WHERE campaign_id = ? AND slug = ?`, campaignID, slug).Scan(&id); err != nil {
		h.t.Fatalf("category %q: %v", slug, err)
	}
	return id
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

// TestCampaignExportImport_Pictures_DBRoundTrip: every picture a page or map
// uses comes back pointing at a file the new campaign owns: the portrait,
// the cover, a picture inside the page text, a map background and a token.
// A /media/ address that names no file in the zip is left as it was.
func TestCampaignExportImport_Pictures_DBRoundTrip(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test requires a database; skipped under -short")
	}
	h := newRoundTripHarness(t)
	ctx := context.Background()
	owner := h.newUser("owner")
	src, heroID := h.sourceWithPlayerCharacter(owner)

	portrait := h.upload(src.ID, owner, 10)
	cover := h.upload(src.ID, owner, 40)
	inline := h.upload(src.ID, owner, 80)
	background := h.upload(src.ID, owner, 120)
	tokenPic := h.upload(src.ID, owner, 160)

	if _, err := h.entities.Update(ctx, heroID, entities.UpdateEntityInput{
		Entry: patch.Of(`<p>Her sigil:</p><p><img src="/media/` + inline + `"></p><p>Unrelated: /media/not-a-restored-id</p>`),
	}); err != nil {
		t.Fatalf("set entry: %v", err)
	}
	// A page written in the editor keeps its editor document and HTML.
	editorPage, err := h.entities.Create(ctx, src.ID, owner, entities.CreateEntityInput{Name: "Sigil Notes", EntityTypeID: h.typeID(src.ID, "note")})
	if err != nil {
		t.Fatalf("create editor page: %v", err)
	}
	doc := `{"type":"doc","content":[{"type":"image","attrs":{"src":"/media/` + inline + `"}}]}`
	if err := h.entities.UpdateEntry(ctx, editorPage.ID, doc, `<p><img src="/media/`+inline+`"></p>`); err != nil {
		t.Fatalf("set editor entry: %v", err)
	}
	if err := h.entities.UpdateImage(ctx, heroID, portrait); err != nil {
		t.Fatalf("set portrait: %v", err)
	}
	if err := h.entities.UpdateCoverImage(ctx, heroID, cover); err != nil {
		t.Fatalf("set cover: %v", err)
	}
	m, err := h.maps.CreateMap(ctx, maps.CreateMapInput{CampaignID: src.ID, Name: "Coast", ImageID: &background, ImageWidth: 4, ImageHeight: 4})
	if err != nil {
		t.Fatalf("create map: %v", err)
	}
	if _, err := h.drawings.CreateToken(ctx, maps.CreateTokenInput{
		MapID: m.ID, Name: "Aria token", ImagePath: &tokenPic, X: 10, Y: 10, Width: 1, Height: 1, Scale: 1, CreatedBy: owner,
	}); err != nil {
		t.Fatalf("create token: %v", err)
	}

	newID, body := h.importBlob(h.newUser("importer"), "campaign.zip", h.exportZip(src, owner))
	if newID == src.ID {
		t.Fatalf("import made no new campaign")
	}

	var imagePath, coverPath, entryHTML sql.NullString
	if err := h.db.QueryRow(`SELECT image_path, cover_image_path, entry_html FROM entities WHERE campaign_id = ? AND name = 'Aria'`,
		newID).Scan(&imagePath, &coverPath, &entryHTML); err != nil {
		t.Fatalf("imported character not found (response: %s): %v", body, err)
	}
	if got := h.mediaCampaign(imagePath.String); got != newID {
		t.Errorf("portrait %q belongs to campaign %q, want the imported one", imagePath.String, got)
	}
	if got := h.mediaCampaign(coverPath.String); got != newID {
		t.Errorf("cover %q belongs to campaign %q, want the imported one", coverPath.String, got)
	}
	html := entryHTML.String
	if i := strings.Index(html, `<img src="/media/`); i < 0 || strings.Contains(html, inline) {
		t.Errorf("inline picture was not re-pointed at the restored file: %s", html)
	} else {
		start := i + len(`<img src="/media/`)
		if newInline := html[start : start+36]; h.mediaCampaign(newInline) != newID {
			t.Errorf("inline picture %q does not belong to the imported campaign", newInline)
		}
	}
	if !strings.Contains(html, "/media/not-a-restored-id") {
		t.Errorf("a /media/ address with no file in the zip was rewritten: %s", html)
	}

	var editorDoc, editorHTML sql.NullString
	if err := h.db.QueryRow(`SELECT entry, entry_html FROM entities WHERE campaign_id = ? AND name = 'Sigil Notes'`,
		newID).Scan(&editorDoc, &editorHTML); err != nil {
		t.Fatalf("imported editor page not found: %v", err)
	}
	if !strings.HasPrefix(editorDoc.String, `{"type":"doc"`) || strings.Contains(editorDoc.String, inline) ||
		!strings.Contains(editorHTML.String, `<img src="/media/`) || strings.Contains(editorHTML.String, inline) {
		t.Errorf("editor page did not come back as an editor document with its picture re-pointed:\n entry: %s\n html: %s",
			editorDoc.String, editorHTML.String)
	}

	var newMapID string
	var mapImage sql.NullString
	if err := h.db.QueryRow(`SELECT id, image_id FROM maps WHERE campaign_id = ?`, newID).Scan(&newMapID, &mapImage); err != nil {
		t.Fatalf("imported map not found: %v", err)
	}
	if got := h.mediaCampaign(mapImage.String); got != newID {
		t.Errorf("map background %q belongs to campaign %q, want the imported one", mapImage.String, got)
	}
	var tokenImage sql.NullString
	if err := h.db.QueryRow(`SELECT image_path FROM map_tokens WHERE map_id = ?`, newMapID).Scan(&tokenImage); err != nil {
		t.Fatalf("imported token not found: %v", err)
	}
	if got := h.mediaCampaign(tokenImage.String); got != newID {
		t.Errorf("token picture %q belongs to campaign %q, want the imported one", tokenImage.String, got)
	}

	if strings.Contains(body, "Imported with losses") {
		t.Errorf("round trip reported losses:\n%s", body)
	}

	// The JSON export carries no picture bytes; importing it says so, and the
	// new campaign's map and token do not point at the source campaign's files.
	jsonID, jsonBody := h.importBlob(h.newUser("json-importer"), "campaign.json", h.export(src, owner, "", "application/json"))
	for _, want := range []string{"Imported with losses", "5 media files", "2 page pictures", "2 map pictures"} {
		if !strings.Contains(jsonBody, want) {
			t.Errorf("JSON import response does not mention %q:\n%s", want, jsonBody)
		}
	}
	var jsonMapImage, jsonTokenImage sql.NullString
	if err := h.db.QueryRow(`SELECT m.image_id, t.image_path FROM maps m JOIN map_tokens t ON t.map_id = m.id WHERE m.campaign_id = ?`,
		jsonID).Scan(&jsonMapImage, &jsonTokenImage); err != nil {
		t.Fatalf("JSON-imported map not found: %v", err)
	}
	if jsonMapImage.Valid || jsonTokenImage.Valid {
		t.Errorf("JSON import kept the source campaign's map pictures: map %q, token %q", jsonMapImage.String, jsonTokenImage.String)
	}
}

// TestCampaignExportImport_PlacesInTree_DBRoundTrip: the extra places a page is
// listed in come back with the campaign, under the imported copies of the
// same pages, while each page keeps its real parent.
func TestCampaignExportImport_PlacesInTree_DBRoundTrip(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test requires a database; skipped under -short")
	}
	h := newRoundTripHarness(t)
	ctx := context.Background()
	owner := h.newUser("owner")
	src, err := h.campaigns.Create(ctx, owner, campaigns.CreateCampaignInput{Name: "Places Source"})
	if err != nil {
		t.Fatal(err)
	}
	typ := h.typeID(src.ID, "character")
	mk := func(name, parent string) *entities.Entity {
		e, err := h.entities.Create(ctx, src.ID, owner, entities.CreateEntityInput{Name: name, EntityTypeID: typ, ParentID: parent})
		if err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		return e
	}
	city, guild := mk("Port City", ""), mk("Thieves Guild", "")
	cook := mk("Mira the Cook", city.ID)
	if err := h.places.AddPlace(ctx, src.ID, cook.ID, guild.ID, owner); err != nil {
		t.Fatal(err)
	}

	newID, body := h.importBlob(h.newUser("importer"), "campaign.zip", h.exportZip(src, owner))
	var n int
	if err := h.db.QueryRow(`
		SELECT COUNT(*) FROM entity_places ep
		  JOIN entities e ON e.id = ep.entity_id AND e.name = 'Mira the Cook' AND e.campaign_id = ?
		  JOIN entities p ON p.id = ep.parent_entity_id AND p.name = 'Thieves Guild' AND p.campaign_id = ?
		 WHERE ep.campaign_id = ?`, newID, newID, newID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("imported campaign has %d extra places for Mira, want 1 (response: %s)", n, body)
	}
	var parentName string
	if err := h.db.QueryRow(`
		SELECT p.name FROM entities e JOIN entities p ON p.id = e.parent_id
		 WHERE e.name = 'Mira the Cook' AND e.campaign_id = ?`, newID).Scan(&parentName); err != nil || parentName != "Port City" {
		t.Fatalf("Mira's real parent after import = %q (%v), want Port City", parentName, err)
	}
}
