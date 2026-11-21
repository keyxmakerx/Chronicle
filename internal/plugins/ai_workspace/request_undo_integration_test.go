package ai_workspace

// Undo of an applied request against a real database and the real entity
// service: the page text really goes back through the page history's restore,
// a created page really lands in the Trash, and a page someone edited since is
// left alone. Skipped without CHRONICLE_TEST_DB_DSN.
//
//	tools/start-test-db.sh
//	CHRONICLE_TEST_DB_DSN='root@tcp(127.0.0.1:13306)/' go test ./internal/plugins/ai_workspace/ -run TestUndoIntegration

import (
	"context"
	crand "crypto/rand"
	"database/sql"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/database/dbtest"
	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/aiexport"
	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/connector"
	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/importer"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// realPages is PageUndo over the real services, as the app wires it.
type realPages struct {
	svc    entities.EntityService
	safety entities.PageSafetyService
}

func (p realPages) Read(ctx context.Context, campaignID, id string) (*entities.Entity, error) {
	e, err := p.svc.GetByID(ctx, id)
	if err != nil || e == nil || e.CampaignID != campaignID {
		return nil, nil
	}
	return e, nil
}
func (p realPages) RestoreText(ctx context.Context, id, name string, entry, html *string) error {
	return p.safety.RestoreText(ctx, id, name, entry, html)
}
func (p realPages) Trash(ctx context.Context, id string) error { return p.svc.Delete(ctx, id) }
func (p realPages) RestoreFromTrash(ctx context.Context, c, id string) error {
	_, err := p.safety.RestoreFromTrash(ctx, c, id)
	return err
}

type realRig struct {
	h          *Handler
	db         *sql.DB
	svc        entities.EntityService
	safety     entities.PageSafetyService
	repo       connector.Repository
	camp, user string
	mill       *entities.Entity
	reqID      int64
}

func uuid(t *testing.T) string {
	t.Helper()
	b := make([]byte, 16)
	if _, err := crand.Read(b); err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func newRealRig(t *testing.T) *realRig {
	t.Helper()
	sub, err := fs.Sub(connector.MigrationsFS, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	db := dbtest.Open(t, dbtest.Options{Prefix: "chronicle_undo_", StrictMigrations: true,
		Plugins: []dbtest.PluginMigrations{{Slug: connector.SchemaSlug, FS: sub}}})
	ctx := context.Background()
	user, camp := uuid(t), uuid(t)
	for _, q := range [][]any{
		{`INSERT INTO users (id, email, display_name, password_hash) VALUES (?,?,?,?)`, user, user + "@example.test", "Ana", "x"},
		{`INSERT INTO campaigns (id, name, slug, created_by) VALUES (?,?,?,?)`, camp, "Table", camp, user},
	} {
		if _, err := db.Exec(q[0].(string), q[1:]...); err != nil {
			t.Fatal(err)
		}
	}
	typeRepo := entities.NewEntityTypeRepository(db)
	et := &entities.EntityType{CampaignID: camp, Slug: "places", Name: "Place", NamePlural: "Places", Icon: "fa-map",
		Color: "#111111", Fields: []entities.FieldDefinition{}, Layout: entities.DefaultLayout(), SortOrder: 1, Enabled: true}
	if err := typeRepo.Create(ctx, et); err != nil {
		t.Fatal(err)
	}
	entityRepo := entities.NewEntityRepository(db)
	safetyRepo := entities.NewPageSafetyRepository(db)
	svc := entities.NewEntityService(entityRepo, typeRepo, entities.NewEntityPermissionRepository(db))
	svc.SetPageSafety(safetyRepo)
	safety := entities.NewPageSafetyService(safetyRepo, entityRepo, svc, nil)

	mill, err := svc.Create(entities.WithActor(ctx, user), camp, user, entities.CreateEntityInput{Name: "Old Mill", EntityTypeID: et.ID})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.UpdateEntry(entities.WithActor(ctx, user), mill.ID, `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"The cellar door is open."}]}]}`, "<p>The cellar door is open.</p>"); err != nil {
		t.Fatal(err)
	}

	repo := connector.NewRepository(db)
	csvc := connector.NewService(repo, anyOwner{})
	csvc.SetUndo(connector.NewUndoRepository(db))
	h := NewHandler(nil)
	h.SetImportLookup(svc)
	h.SetImportCommitter(importer.NewCommitter(svc))
	h.SetConnector(csvc, "https://c.example")
	h.SetPageUndo(realPages{svc: svc, safety: safety})

	now := time.Now().UTC().Truncate(time.Second)
	if _, err := repo.Replace(ctx, &connector.Connection{CampaignID: camp, UserID: user, KeyHash: fmt.Sprintf("%064d", 1), KeyHint: "0001",
		Privacy: aiexport.PrivacyModePermitted, CanAsk: true, CreatedAt: now, ExpiresAt: now.Add(connector.KeyLifetime)}); err != nil {
		t.Fatal(err)
	}
	conn, _ := repo.Current(ctx, camp, user)
	id, err := repo.AddRequest(ctx, &connector.Request{CampaignID: camp, ConnectionID: conn.ID, Summary: "Mill news", Counts: "1 to add, 1 to change", Blocks: 2, Body: "b", CreatedAt: now}, 20)
	if err != nil {
		t.Fatal(err)
	}
	return &realRig{h: h, db: db, svc: svc, safety: safety, repo: repo, camp: camp, user: user, mill: mill, reqID: id}
}

func (r *realRig) call(t *testing.T, fn func(*echo.Context) error, form url.Values, pathValues echo.PathValues) (int, string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.SetPathValues(pathValues)
	fakeContext(c, &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: r.camp, Name: "Table"}, MemberRole: campaigns.RoleOwner})
	c.Set("auth_user_id", r.user)
	if err := fn(c); err != nil {
		return -1, err.Error()
	}
	return rec.Code, rec.Body.String()
}

const millNews = "---\nname: Old Mill\naction: update\ntype: places\n---\n\nThe cellar door is barred.\n\n---\nname: Sable Quay\ntype: places\n---\n\nA harbour town.\n"

func (r *realRig) apply(t *testing.T) {
	t.Helper()
	code, body := r.call(t, r.h.CommitImport, url.Values{
		"markdown_source": {millNews}, "ai_request": {"1"},
		"page_0_include": {"on"}, "page_0_action": {"update"}, "page_0_category": {"places"},
		"page_1_include": {"on"}, "page_1_category": {"places"},
	}, echo.PathValues{{Name: "id", Value: r.camp}})
	if code != http.StatusOK {
		t.Fatalf("apply: %d %s", code, body)
	}
}

func (r *realRig) undo(t *testing.T) (int, string) {
	return r.call(t, r.h.UndoRequest, url.Values{}, echo.PathValues{{Name: "id", Value: r.camp}, {Name: "rid", Value: "1"}})
}

func TestUndoIntegration_PagesBackThroughHistoryAndTrash(t *testing.T) {
	r := newRealRig(t)
	ctx := context.Background()
	r.apply(t)
	got, _ := r.svc.GetByID(ctx, r.mill.ID)
	if !strings.Contains(*got.EntryHTML, "barred") {
		t.Fatalf("the AI's text did not land: %s", *got.EntryHTML)
	}
	quay, err := r.svc.GetBySlug(ctx, r.camp, "sable-quay")
	if err != nil || quay == nil {
		t.Fatalf("created page: %v", err)
	}

	code, body := r.undo(t)
	if code != http.StatusOK || !strings.Contains(body, "Undone today") {
		t.Fatalf("undo: %d %s", code, body)
	}
	got, _ = r.svc.GetByID(ctx, r.mill.ID)
	if !strings.Contains(*got.EntryHTML, "is open") || strings.Contains(*got.EntryHTML, "barred") {
		t.Errorf("text not put back: %s", *got.EntryHTML)
	}
	// The put-back is its own history row, so it can be undone too.
	hist, _, _ := r.safety.History(ctx, r.mill.ID, "", nil)
	if len(hist) == 0 || hist[0].Kind != entities.VersionRestore {
		t.Errorf("latest history kind = %+v", hist)
	}
	// The created page is in the Trash, not gone for good.
	if _, err := r.svc.GetByID(ctx, quay.ID); err == nil {
		t.Error("the AI's page is still live")
	}
	items, _ := r.safety.ListTrash(ctx, r.camp)
	if len(items) != 1 || items[0].ID != quay.ID {
		t.Errorf("trash = %+v", items)
	}
	// A second press changes nothing.
	if code, _ := r.undo(t); code == http.StatusOK {
		t.Error("undo ran twice")
	}
}

func TestUndoIntegration_EditedSinceIsLeftAlone(t *testing.T) {
	r := newRealRig(t)
	ctx := context.Background()
	r.apply(t)
	// A scribe fixes a word after the AI's change.
	if err := r.svc.UpdateEntry(entities.WithActor(ctx, "someone"), r.mill.ID,
		`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"The cellar door is barred, with a lamp."}]}]}`,
		"<p>The cellar door is barred, with a lamp.</p>"); err != nil {
		t.Fatal(err)
	}
	code, body := r.undo(t)
	if code != http.StatusOK || !strings.Contains(body, "Nothing was put back") || !strings.Contains(body, "“Old Mill”") {
		t.Fatalf("refusal: %d %s", code, body)
	}
	got, _ := r.svc.GetByID(ctx, r.mill.ID)
	if !strings.Contains(*got.EntryHTML, "with a lamp") {
		t.Errorf("later work overwritten: %s", *got.EntryHTML)
	}
	// Nothing was moved to the Trash either: the check is for the whole request.
	if items, _ := r.safety.ListTrash(ctx, r.camp); len(items) != 0 {
		t.Errorf("a refused undo trashed %+v", items)
	}
	if _, err := r.svc.GetBySlug(ctx, r.camp, "sable-quay"); err != nil {
		t.Errorf("a refused undo took the created page away: %v", err)
	}
}
