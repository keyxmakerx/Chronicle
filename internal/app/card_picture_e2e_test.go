package app

// card_picture_e2e_test.go runs a card's picture through the real pieces: the
// real media server and its signed links, the real decks service and SQL, and
// the real media and campaign services over a scratch database. The decks and
// media plugins never import each other, so only the app package can put them
// in one request.
//
// Skipped under -short and without CHRONICLE_TEST_DB_DSN.

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/database"
	"github.com/keyxmakerx/chronicle/internal/database/dbtest"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/decks"
	"github.com/keyxmakerx/chronicle/internal/plugins/media"
	"github.com/keyxmakerx/chronicle/internal/plugins/syncapi"
)

// cardHeroes is the decks.Heroes the test needs: which page each player owns.
type cardHeroes struct{ owner map[string]string }

func (h cardHeroes) Owned(_ context.Context, _, userID string) ([]decks.Hero, error) {
	if id := h.owner[userID]; id != "" {
		return []decks.Hero{{ID: id, Name: "Hero", OwnerUserID: userID}}, nil
	}
	return nil, nil
}
func (h cardHeroes) InCampaign(_ context.Context, _, id string) (decks.Hero, bool, error) {
	return decks.Hero{ID: id, Name: "Hero"}, true, nil
}
func (cardHeroes) Viewable(context.Context, string, string, decks.Viewer) (bool, error) {
	return true, nil
}

// cardPageFilter says which pages a viewer can see.
type cardPageFilter struct{ visible map[string]bool }

func (f cardPageFilter) FilterViewableEntityIDs(_ context.Context, _ string, ids []string, _ int, _ string) (map[string]bool, error) {
	out := map[string]bool{}
	for _, id := range ids {
		if f.visible[id] {
			out[id] = true
		}
	}
	return out, nil
}

// brokenCardGuard fails every question, as a decks outage would.
type brokenCardGuard struct{ media.CardPictureGuard }

func (brokenCardGuard) IsCardPicture(context.Context, string, string) (bool, error) {
	return true, context.DeadlineExceeded
}

type cardWorld struct {
	t        *testing.T
	db       *sql.DB
	dir      string
	signer   *media.URLSigner
	mediaSvc media.MediaService
	handler  *media.Handler
	decks    decks.Service
	filter   cardPageFilter
	e        *echo.Echo

	camp, otherCamp                                 string
	owner, tam, ilse, scribe, outsider, otherMember string
	heroTam, heroIlse                               string
	dir_                                            decks.Viewer
	tamV, ilseV                                     decks.Viewer
}

func (w *cardWorld) exec(q string, args ...any) { w.t.Helper(); mustGalleryExec(w.t, w.db, q, args...) }

func newCardWorld(t *testing.T) *cardWorld {
	t.Helper()
	if testing.Short() {
		t.Skip("DB integration test")
	}
	sub, err := fs.Sub(decks.MigrationsFS, database.PluginMigrationsSubdir)
	if err != nil {
		t.Fatal(err)
	}
	db := dbtest.Open(t, dbtest.Options{Prefix: "chronicle_cardpic_",
		Plugins: []dbtest.PluginMigrations{{Slug: decks.PluginSlug, FS: sub}}})
	w := &cardWorld{t: t, db: db, dir: t.TempDir(), filter: cardPageFilter{visible: map[string]bool{}}}
	w.camp, w.otherCamp = galleryTestUUID(t), galleryTestUUID(t)
	for _, p := range []*string{&w.owner, &w.tam, &w.ilse, &w.scribe, &w.outsider, &w.otherMember} {
		*p = galleryTestUUID(t)
		w.exec(`INSERT INTO users (id, email, display_name, password_hash) VALUES (?, ?, 'U', 'x')`, *p, *p+"@example.test")
	}
	for i, c := range []string{w.camp, w.otherCamp} {
		w.exec(`INSERT INTO campaigns (id, name, slug, created_by) VALUES (?, ?, ?, ?)`, c, "C"+c[:4], "c-"+c[:8], w.owner)
		w.exec(`INSERT INTO entity_types (campaign_id, slug, name, name_plural) VALUES (?, 'hero', 'Hero', 'Heroes')`, c)
		_ = i
	}
	w.exec(`INSERT INTO campaign_members (campaign_id, user_id, role) VALUES (?, ?, 'owner'), (?, ?, 'player'), (?, ?, 'player'), (?, ?, 'scribe')`,
		w.camp, w.owner, w.camp, w.tam, w.camp, w.ilse, w.camp, w.scribe)
	w.exec(`INSERT INTO campaign_members (campaign_id, user_id, role) VALUES (?, ?, 'owner'), (?, ?, 'player')`,
		w.otherCamp, w.owner, w.otherCamp, w.otherMember)
	w.heroTam = w.page("Tam", "")
	w.heroIlse = w.page("Ilse", "")

	campSvc := campaigns.NewCampaignService(campaigns.NewCampaignRepository(db), trashUserFinder{}, nil, nil, "http://localhost")
	w.mediaSvc = media.NewMediaService(media.NewMediaRepository(db), w.dir, 1<<20)
	w.signer = media.NewURLSigner("test-secret")
	w.handler = media.NewHandler(w.mediaSvc)
	w.handler.SetURLSigner(w.signer)
	w.handler.SetMemberChecker(&mediaMemberCheckerAdapter{svc: campSvc})
	w.handler.SetEntityVisibilityFilter(&w.filter)

	w.decks = decks.NewService(decks.NewRepository(db),
		cardHeroes{owner: map[string]string{w.tam: w.heroTam, w.ilse: w.heroIlse}},
		&decksMemberAdapter{svc: campSvc},
		decks.WithPictures(&decksPictureAdapter{svc: w.mediaSvc}))
	wireCardPictures(w.decks, w.mediaSvc, w.handler, syncapi.NewMediaAPIHandler(nil, w.mediaSvc))

	w.dir_ = decks.Viewer{UserID: w.owner, MemberRole: 3, VisibilityRole: 3, Director: true}
	w.tamV = decks.Viewer{UserID: w.tam, MemberRole: 1, VisibilityRole: 1}
	w.ilseV = decks.Viewer{UserID: w.ilse, MemberRole: 1, VisibilityRole: 1}

	w.e = echo.New()
	// The real error handler renders pages; the status is all this test reads.
	w.e.HTTPErrorHandler = func(c *echo.Context, err error) {
		code := apperror.SafeCode(err)
		if code == 0 {
			code = echo.StatusCode(err)
		}
		if code == 0 {
			code = http.StatusInternalServerError
		}
		_ = c.NoContent(code)
	}
	asUser := func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			if u := c.Request().Header.Get("X-User"); u != "" {
				auth.SetSession(c, &auth.Session{UserID: u})
			}
			return next(c)
		}
	}
	w.e.GET("/media/:id", w.handler.Serve, asUser)
	w.e.GET("/media/:id/thumb/:size", w.handler.ServeThumbnail, asUser)
	return w
}

// page inserts a hero page; html is its body, which is how a page "uses" a file.
func (w *cardWorld) page(name, html string) string {
	id := galleryTestUUID(w.t)
	w.exec(`INSERT INTO entities (id, campaign_id, entity_type_id, name, slug, is_private, visibility, created_by, entry_html, created_at, updated_at)
		VALUES (?, ?, (SELECT id FROM entity_types WHERE campaign_id = ? AND slug = 'hero'), ?, ?, false, 'default', ?, ?, NOW(), NOW())`,
		id, w.camp, w.camp, name, "p-"+id[:8], w.owner, html)
	return id
}

// file inserts a media row and its bytes, with a 300 thumbnail.
func (w *cardWorld) file(campaign, usage, mime string) string {
	w.t.Helper()
	id := galleryTestUUID(w.t)
	for _, name := range []string{id + ".png", id + "_300.png"} {
		if err := os.WriteFile(filepath.Join(w.dir, name), []byte("png-bytes"), 0o600); err != nil {
			w.t.Fatal(err)
		}
	}
	w.exec(`INSERT INTO media_files (id, campaign_id, uploaded_by, filename, original_name, mime_type, file_size, usage_type, thumbnail_paths)
		VALUES (?, ?, ?, ?, 'art.png', ?, 9, ?, ?)`, id, campaign, w.owner, id+".png", mime, usage, `{"300":"`+id+`_300.png"}`)
	return id
}

// get requests a path as a user ("" is no session) and returns the status.
func (w *cardWorld) get(user, path string) int {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if user != "" {
		req.Header.Set("X-User", user)
	}
	rec := httptest.NewRecorder()
	w.e.ServeHTTP(rec, req)
	return rec.Code
}

// reach is every way a player can ask for the picture: the plain URL, the
// thumbnail, and links freshly signed for them. It returns the distinct
// statuses seen.
func (w *cardWorld) reach(user, fileID string) map[int]bool {
	w.t.Helper()
	viewer := media.ViewerSession(user)
	out := map[int]bool{}
	for _, p := range []string{
		"/media/" + fileID,
		"/media/" + fileID + "/thumb/300",
		w.signer.Sign(fileID, viewer, media.SignedURLTTL),
		w.signer.SignThumb(fileID, "300", viewer, media.SignedURLTTL),
	} {
		out[w.get(user, p)] = true
	}
	return out
}

func (w *cardWorld) expect(user, fileID string, want int, label string) {
	w.t.Helper()
	got := w.reach(user, fileID)
	if len(got) != 1 || !got[want] {
		w.t.Errorf("%s: statuses %v, want only %d", label, got, want)
	}
}

func (w *cardWorld) deck(name string, in *decks.SettingsInput) string {
	w.t.Helper()
	v, err := w.decks.Create(context.Background(), w.camp, w.dir_, decks.CreateDeckInput{Name: name, Kind: decks.KindEvent, Settings: in})
	if err != nil {
		w.t.Fatal(err)
	}
	return v.(decks.DirectorDeck).ID
}

func (w *cardWorld) choose(deck string, users ...string) {
	w.t.Helper()
	d, err := w.decks.Get(context.Background(), w.camp, deck, w.dir_)
	if err != nil {
		w.t.Fatal(err)
	}
	var ps []decks.PlayerInput
	for _, u := range users {
		ps = append(ps, decks.PlayerInput{UserID: u})
	}
	if _, err := w.decks.SetPlayers(context.Background(), w.camp, deck, w.dir_, decks.SetPlayersInput{Version: d.(decks.DirectorDeck).Version, Players: ps}); err != nil {
		w.t.Fatal(err)
	}
}

func (w *cardWorld) card(deck, name, picture string) string {
	w.t.Helper()
	v, err := w.decks.CreateCard(context.Background(), w.camp, deck, w.dir_, decks.CreateCardInput{Name: name, Picture: picture})
	if err != nil {
		w.t.Fatal(err)
	}
	for _, c := range v.(decks.DirectorDeck).Cards {
		if c.Name == name {
			return c.ID
		}
	}
	w.t.Fatal("card not stored")
	return ""
}

// deal gives a card face down to a hero and returns the copy's id.
func (w *cardWorld) deal(deck, card, hero string) string {
	w.t.Helper()
	res, err := w.decks.DealCard(context.Background(), w.camp, deck, w.dir_, decks.DealCardInput{CardID: card, To: "hand", HeroID: hero})
	if err != nil {
		w.t.Fatal(err)
	}
	return res.CopyID
}

// soleCopy is the id of the deck's only copy (a returned copy gets a new id).
func (w *cardWorld) soleCopy(deck string) string {
	w.t.Helper()
	d, err := w.decks.Get(context.Background(), w.camp, deck, w.dir_)
	if err != nil {
		w.t.Fatal(err)
	}
	copies := d.(decks.DirectorDeck).Copies
	if len(copies) != 1 {
		w.t.Fatalf("deck has %d copies, want 1", len(copies))
	}
	return copies[0].ID
}

// refused checks that nothing opens: every route answers 403 or 404, never a
// file. A signed-in non-member's plain request has always been a 403 from the
// base check; links signed for them reach this guard and are a 404.
func (w *cardWorld) refused(user, fileID, label string) {
	w.t.Helper()
	for st := range w.reach(user, fileID) {
		if st != http.StatusNotFound && st != http.StatusForbidden {
			w.t.Errorf("%s: a route answered %d, want 403 or 404", label, st)
		}
	}
}

func (w *cardWorld) act(deck, copyID, verb string, v decks.Viewer) {
	w.t.Helper()
	if _, err := w.decks.Act(context.Background(), w.camp, deck, copyID, verb, v, decks.ActInput{}); err != nil {
		w.t.Fatalf("%s: %v", verb, err)
	}
}

// A card dealt face down shows no picture to its holder by any route; turning
// it over opens it; sending it back and dealing it face down closes it again.
func TestCardPictureFollowsTheFaceThroughTheMediaServer(t *testing.T) {
	w := newCardWorld(t)
	pic := w.file(w.camp, media.UsageAttachment, "image/png")
	down := "down"
	deck := w.deck("Omens", &decks.SettingsInput{Arrives: &down})
	w.choose(deck, w.tam)
	card := w.card(deck, "The Drowned Bell", pic)

	w.expect(w.owner, pic, http.StatusOK, "Director before any deal")
	w.expect(w.tam, pic, http.StatusNotFound, "a card in the pile")

	copyID := w.deal(deck, card, w.heroTam)
	w.expect(w.tam, pic, http.StatusNotFound, "dealt face down to their own hero")

	w.act(deck, copyID, "turn", w.tamV)
	w.expect(w.tam, pic, http.StatusOK, "turned over by its holder")
	w.expect(w.ilse, pic, http.StatusNotFound, "another player while it is turned (private)")
	w.expect(w.scribe, pic, http.StatusNotFound, "a Scribe who holds no hero")

	w.act(deck, copyID, "take-back", w.dir_)
	w.expect(w.tam, pic, http.StatusNotFound, "back in the pile")
	w.deal(deck, card, w.heroTam)
	w.expect(w.tam, pic, http.StatusNotFound, "dealt again face down (the same urls and fresh links)")
	w.act(deck, w.soleCopy(deck), "turn", w.tamV)
	w.expect(w.tam, pic, http.StatusOK, "turned over again")

	// A link minted for the Director does not open the picture in a player's session.
	w.act(deck, w.soleCopy(deck), "take-back", w.dir_)
	w.deal(deck, card, w.heroTam)
	exp := w.signer.Sign(pic, media.ViewerSession(w.owner), media.SignedURLTTL)
	if got := w.get(w.tam, exp); got != http.StatusNotFound {
		t.Errorf("a Director's signed link in a player's session answered %d, want 404", got)
	}
	if got := w.get(w.owner, exp); got != http.StatusOK {
		t.Errorf("the Director's own signed link answered %d, want 200", got)
	}

	// Nobody outside the campaign gets it, face up or not.
	w.act(deck, w.soleCopy(deck), "take-back", w.dir_)
	w.deal(deck, card, w.heroTam)
	w.act(deck, w.soleCopy(deck), "turn", w.tamV)
	w.refused(w.outsider, pic, "a non-member")
	w.refused(w.otherMember, pic, "a member of another campaign")
	if got := w.get("", "/media/"+pic); got != http.StatusNotFound {
		t.Errorf("a visitor with no session answered %d, want 404", got)
	}
}

// Another hero's turned-but-not-shown card stays closed to other players;
// once shown it opens only on a deck where others see faces.
func TestCardPictureOfAnotherHerosCardThroughTheMediaServer(t *testing.T) {
	for _, tt := range []struct {
		others string
		want   int
	}{{"face", http.StatusOK}, {"back", http.StatusNotFound}} {
		t.Run(tt.others, func(t *testing.T) {
			w := newCardWorld(t)
			pic := w.file(w.camp, media.UsageAttachment, "image/png")
			others, down := tt.others, "down"
			deck := w.deck("Omens", &decks.SettingsInput{OthersSee: &others, Arrives: &down})
			w.choose(deck, w.tam, w.ilse)
			card := w.card(deck, "Relic", pic)
			copyID := w.deal(deck, card, w.heroTam)
			w.act(deck, copyID, "turn", w.tamV)
			w.expect(w.ilse, pic, http.StatusNotFound, "turned but not shown")
			w.act(deck, copyID, "show", w.tamV)
			w.expect(w.ilse, pic, tt.want, "after show")
			w.expect(w.tam, pic, http.StatusOK, "the holder")
		})
	}
}

// A card's picture that a page also shows stays visible through the page when
// the player can see the page, and is closed when they cannot.
func TestCardPictureAlsoOnAPageThroughTheMediaServer(t *testing.T) {
	w := newCardWorld(t)
	pic := w.file(w.camp, media.UsageAttachment, "image/png")
	down := "down"
	deck := w.deck("Omens", &decks.SettingsInput{Arrives: &down})
	w.choose(deck, w.tam)
	card := w.card(deck, "Relic", pic)
	w.deal(deck, card, w.heroTam) // face down

	page := w.page("Shrine", `<p><img src="/media/`+pic+`"></p>`)
	w.expect(w.tam, pic, http.StatusNotFound, "on a page the player cannot see")
	w.filter.visible[page] = true
	w.expect(w.tam, pic, http.StatusOK, "on a page the player can see")
	// The page rule gives nothing to someone outside the campaign.
	w.refused(w.outsider, pic, "a non-member, page visible")
}

// A failing check refuses with the same 404 and logs.
func TestCardPictureGuardErrorRefusesAndLogs(t *testing.T) {
	w := newCardWorld(t)
	pic := w.file(w.camp, media.UsageAttachment, "image/png")
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })

	w.handler.SetCardPictureGuard(brokenCardGuard{})
	if got := w.get(w.tam, "/media/"+pic); got != http.StatusNotFound {
		t.Errorf("a failing guard answered %d, want 404", got)
	}
	if !strings.Contains(buf.String(), "card picture check failed") {
		t.Errorf("the failure was not logged: %q", buf.String())
	}
}

// The picker (GET /campaigns/:id/media/list): a Scribe never gets a card
// picture, a Director does, and totals are right for both.
func TestCardPictureIsLeftOutOfThePicker(t *testing.T) {
	w := newCardWorld(t)
	var plain []string
	for i := 0; i < 3; i++ {
		plain = append(plain, w.file(w.camp, media.UsageAttachment, "image/png"))
	}
	pic := w.file(w.camp, media.UsageAttachment, "image/png")
	w.card(w.deck("Omens", &decks.SettingsInput{}), "Relic", pic)

	list := func(cc *campaigns.CampaignContext, user string, perPage, page string) (ids []string, total int) {
		req := httptest.NewRequest(http.MethodGet, "/campaigns/"+w.camp+"/media/list?perPage="+perPage+"&page="+page, nil)
		rec := httptest.NewRecorder()
		c := w.e.NewContext(req, rec)
		c.Set("campaign_context", cc)
		auth.SetSession(c, &auth.Session{UserID: user})
		if err := w.handler.CampaignMediaList(c); err != nil {
			t.Fatal(err)
		}
		var body struct {
			Items []struct {
				ID string `json:"id"`
			} `json:"items"`
			Total int `json:"total"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		for _, it := range body.Items {
			ids = append(ids, it.ID)
		}
		return ids, body.Total
	}
	scribeCC := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: w.camp}, MemberRole: campaigns.RoleScribe, IsMember: true}
	ownerCC := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: w.camp}, MemberRole: campaigns.RoleOwner, IsMember: true}

	var got []string
	for page := 1; page <= 2; page++ {
		ids, total := list(scribeCC, w.scribe, "2", itoaPage(page))
		if total != 3 {
			t.Errorf("Scribe page %d total = %d, want 3", page, total)
		}
		got = append(got, ids...)
	}
	if len(got) != 3 {
		t.Errorf("Scribe pages hold %v, want the 3 plain files", got)
	}
	for _, id := range got {
		if id == pic {
			t.Error("a Scribe's picker lists a card's picture")
		}
	}
	var all []string
	for page := 1; page <= 2; page++ {
		ids, total := list(ownerCC, w.owner, "2", itoaPage(page))
		if total != 4 {
			t.Errorf("Director page %d total = %d, want 4", page, total)
		}
		all = append(all, ids...)
	}
	found := false
	for _, id := range all {
		found = found || id == pic
	}
	if len(all) != 4 || !found {
		t.Errorf("the Director's picker holds %v, want 4 files including the card picture", all)
	}

	// A page the Scribe can see also showing it does not bring it back into the
	// picker (the fetch keeps that exception; a listing does not).
	page := w.page("Shrine", `<img src="/media/`+pic+`">`)
	w.filter.visible[page] = true
	if _, total := list(scribeCC, w.scribe, "50", "1"); total != 3 {
		t.Errorf("Scribe total with the picture on a visible page = %d, want 3", total)
	}
}

func itoaPage(n int) string { return string(rune('0' + n)) }

// Saving a card refuses, in the same words, every file that cannot be a card's
// picture, with the real media rows behind the adapter.
func TestCardPictureSaveRefusalsAgainstRealMedia(t *testing.T) {
	w := newCardWorld(t)
	deck := w.deck("Omens", &decks.SettingsInput{})
	good := w.file(w.camp, media.UsageAttachment, "image/png")
	trashed := w.file(w.camp, media.UsageAttachment, "image/png")
	if err := w.mediaSvc.DeleteCampaignMedia(context.Background(), w.camp, trashed); err != nil {
		t.Fatal(err)
	}
	bad := map[string]string{
		"another campaign's file": w.file(w.otherCamp, media.UsageAttachment, "image/png"),
		"a non-image":             w.file(w.camp, media.UsageAttachment, "text/plain"),
		"a page file":             w.file(w.camp, media.UsagePageFile, "image/png"),
		"a note picture":          w.file(w.camp, media.UsageNoteImage, "image/png"),
		"a trashed file":          trashed,
		"a URL":                   "https://example.test/media/" + good,
		"a path":                  "/media/" + good,
		"a made-up id":            galleryTestUUID(t),
	}
	var first string
	for name, pic := range bad {
		_, err := w.decks.CreateCard(context.Background(), w.camp, deck, w.dir_, decks.CreateCardInput{Name: "C " + name, Picture: pic})
		if err == nil {
			t.Errorf("%s: accepted", name)
			continue
		}
		if first == "" {
			first = err.Error()
		} else if err.Error() != first {
			t.Errorf("%s: refusal %q differs from %q", name, err.Error(), first)
		}
	}
	if _, err := w.decks.CreateCard(context.Background(), w.camp, deck, w.dir_, decks.CreateCardInput{Name: "Good", Picture: good}); err != nil {
		t.Errorf("an image of the campaign was refused: %v", err)
	}
}
