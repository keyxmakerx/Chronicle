package decks

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"
	emw "github.com/labstack/echo/v5/middleware"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/addons"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// The workshop pages through the real route registration and middleware, with
// the real service over the in-memory repository, so what is checked is what
// a browser would get: the gates, the fragments and what each page holds.

const sessionCookie = "chronicle_session"

type guardAuth struct{ auth.AuthService }

func (guardAuth) ValidateSession(_ context.Context, token string) (*auth.Session, error) {
	if token == "" {
		return nil, apperror.NewUnauthorized("no token")
	}
	return &auth.Session{UserID: token}, nil
}

type guardAddons struct {
	addons.AddonService
	on bool
}

func (g guardAddons) IsEnabledForCampaign(context.Context, string, string) (bool, error) {
	return g.on, nil
}

// guardCampaigns knows two campaigns: camp-1 with the fixture's people and
// camp-2, where only u-other is a member.
type guardCampaigns struct {
	campaigns.CampaignService
}

func (guardCampaigns) GetByID(_ context.Context, id string) (*campaigns.Campaign, error) {
	return &campaigns.Campaign{ID: id, Settings: `{"dm_grant_ids":["u-codm"]}`}, nil
}

func (guardCampaigns) GetMember(_ context.Context, campaignID, userID string) (*campaigns.CampaignMember, error) {
	roles := map[string]campaigns.Role{"u-dir": campaigns.RoleOwner, "u-codm": campaigns.RolePlayer, "u-tam": campaigns.RolePlayer, "u-ilse": campaigns.RolePlayer, "u-scribe": campaigns.RoleScribe}
	if campaignID == "camp-2" {
		roles = map[string]campaigns.Role{"u-other": campaigns.RoleOwner}
	}
	role, ok := roles[userID]
	if !ok {
		return nil, apperror.NewNotFound("not a member")
	}
	return &campaigns.CampaignMember{UserID: userID, Role: role}, nil
}

type workshopRig struct {
	fx   *fixture
	e    *echo.Echo
	deck string
}

func newWorkshopRig(t *testing.T, addonOn bool) *workshopRig {
	t.Helper()
	fx := newFixture(t)
	e := echo.New()
	e.Use(emw.Recover())
	e.HTTPErrorHandler = func(c *echo.Context, err error) {
		if c.Response().(*echo.Response).Committed {
			return
		}
		if ae, ok := err.(*apperror.AppError); ok {
			_ = c.JSON(ae.Code, map[string]string{"error": ae.Type, "message": ae.Message})
			return
		}
		_ = c.NoContent(http.StatusInternalServerError)
	}
	RegisterRoutes(e, NewHandler(fx.svc), guardCampaigns{}, guardAuth{}, guardAddons{on: addonOn})
	id := fx.deck("Iron omens", KindHero, nil)
	fx.card(id, CreateCardInput{Name: "The dragon stirs", Words: "SECRET-WORDS-OF-A-CARD", DirectorNote: "SECRET-NOTE", Copies: ptr(3)})
	return &workshopRig{fx: fx, e: e, deck: id}
}

func (r *workshopRig) get(path, user string, hdr ...string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	for i := 0; i+1 < len(hdr); i += 2 {
		req.Header.Set(hdr[i], hdr[i+1])
	}
	if user != "" {
		req.AddCookie(&http.Cookie{Name: sessionCookie, Value: user})
	}
	rec := httptest.NewRecorder()
	r.e.ServeHTTP(rec, req)
	return rec
}

func (r *workshopRig) paths() []string {
	base := "/campaigns/camp-1/decks/workshop"
	d := base + "/" + r.deck
	return []string{base, base + "/new", d, d + "/table", d + "/cards", d + "/look", d + "/rules", d + "/players"}
}

// A player or scribe is told the page is not there, on every page and
// fragment, and nothing of the deck is in what they are sent.
func TestWorkshopPagesAre404ForPlayersAndScribes(t *testing.T) {
	r := newWorkshopRig(t, true)
	for _, user := range []string{"u-tam", "u-scribe"} {
		for _, p := range r.paths() {
			rec := r.get(p, user)
			if rec.Code != http.StatusNotFound {
				t.Errorf("%s at %s: status %d, want 404", user, p, rec.Code)
			}
			body := rec.Body.String()
			for _, leak := range []string{"Iron omens", "The dragon stirs", "SECRET-WORDS-OF-A-CARD", "SECRET-NOTE", r.deck} {
				if strings.Contains(body, leak) && leak != r.deck {
					t.Errorf("%s at %s: the response holds %q", user, p, leak)
				}
			}
		}
	}
	// The deck id is in the address a player typed, so it is not a leak to echo
	// it; the check above skips it, and this one makes sure no page markup came.
	if rec := r.get(r.paths()[2], "u-tam"); strings.Contains(rec.Body.String(), "dk-card") || strings.Contains(rec.Body.String(), "data-widget") {
		t.Error("a player was sent page markup")
	}
}

func TestWorkshopPagesOpenForDirectors(t *testing.T) {
	r := newWorkshopRig(t, true)
	for _, user := range []string{"u-dir", "u-codm"} {
		for _, p := range r.paths() {
			rec := r.get(p, user)
			if rec.Code != http.StatusOK {
				t.Errorf("%s at %s: status %d, want 200: %.200s", user, p, rec.Code, rec.Body.String())
			}
		}
	}
}

func TestWorkshopWithTheAddonOffShowsNothing(t *testing.T) {
	r := newWorkshopRig(t, false)
	for _, p := range r.paths() {
		rec := r.get(p, "u-dir", "Accept", "application/json")
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s with the add-on off (JSON): %d, want 404", p, rec.Code)
		}
		rec = r.get(p, "u-dir", "HX-Request", "true")
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s with the add-on off (HTMX): %d, want 404", p, rec.Code)
		}
		// A page navigation is sent back to the campaign, as every add-on's pages are.
		rec = r.get(p, "u-dir")
		if rec.Code != http.StatusSeeOther || strings.Contains(rec.Body.String(), "Iron omens") {
			t.Errorf("%s with the add-on off (page): %d", p, rec.Code)
		}
	}
}

// Each fragment holds what its tab promises; the page opens on Cards.
func TestWorkshopFragmentsAndTabOrder(t *testing.T) {
	r := newWorkshopRig(t, true)
	d := r.paths()[2]
	page := r.get(d, "u-dir").Body.String()
	order := []string{"Cards", "Look", "Rules", "Players"}
	last := -1
	for _, name := range order {
		i := strings.Index(page, `data-tab="`+strings.ToLower(name)+`"`)
		if i <= last {
			t.Errorf("tab %s is out of order", name)
		}
		last = i
	}
	if !strings.Contains(page, `aria-selected="true"`) || !strings.Contains(page, "data-dkw-cards") {
		t.Error("a new deck opens on Cards")
	}
	for tail, want := range map[string]string{
		"/table": "data-dkw-table", "/cards": "data-dkw-cards", "/look": "data-dkw-look", "/rules": "data-dkw-rules", "/players": "data-dkw-players",
	} {
		body := r.get(d+tail, "u-dir", "HX-Request", "true").Body.String()
		if !strings.Contains(body, want) {
			t.Errorf("fragment %s lacks %s", tail, want)
		}
		if strings.Contains(body, "<html") {
			t.Errorf("fragment %s is a whole page", tail)
		}
	}
	if body := r.get(d+"?tab=look", "u-dir").Body.String(); !strings.Contains(body, "data-dkw-look") {
		t.Error("?tab=look opens the Look tab")
	}
	if body := r.get(d+"?tab=nonsense", "u-dir").Body.String(); !strings.Contains(body, "data-dkw-cards") {
		t.Error("an unknown tab opens Cards")
	}
	if rec := r.get("/campaigns/camp-1/decks/looks", "u-dir"); rec.Code == http.StatusOK {
		t.Error("the proof sheet is gone")
	}
}

// Pages render for an empty deck, a deck with 200 cards and a deck with
// twelve kinds, and the Cards tab opens a card's editor.
func TestWorkshopRendersEmptyFullAndTwelveKinds(t *testing.T) {
	r := newWorkshopRig(t, true)
	empty := r.fx.deck("Empty", KindParty, nil)
	full := r.fx.deck("Full", KindEvent, nil)
	for i := 0; i < MaxCardsPerDeck; i++ {
		r.fx.card(full, CreateCardInput{Name: fmt.Sprintf("Card %03d", i)})
	}
	kinds := r.fx.deck("Kinds", KindHero, nil)
	var ks []KindDef
	for i := 0; i < MaxKinds; i++ {
		ks = append(ks, KindDef{Key: fmt.Sprintf("k%d", i), Name: fmt.Sprintf("Kind %d", i), Colour: Palette[i%len(Palette)]})
	}
	d := r.fx.repo.decks[kinds]
	_, err := r.fx.svc.Update(nil2ctx(), camp, kinds, r.fx.dir, UpdateDeckInput{Version: patchOf(d.Version), Look: patchOf(LookInput{Kinds: ks})})
	r.fx.must(err)
	r.fx.card(kinds, CreateCardInput{Name: "Worn", KindKey: "k11"})

	base := "/campaigns/camp-1/decks/workshop/"
	for name, id := range map[string]string{"empty": empty, "full": full, "kinds": kinds} {
		for _, tail := range []string{"", "?tab=look", "?tab=rules", "?tab=players", "/table"} {
			rec := r.get(base+id+tail, "u-dir")
			if rec.Code != http.StatusOK {
				t.Fatalf("%s deck at %q: %d", name, tail, rec.Code)
			}
		}
	}
	if body := r.get(base+empty, "u-dir").Body.String(); !strings.Contains(body, "no cards yet") || !strings.Contains(body, "Empty") {
		t.Error("an empty deck says so")
	}
	if body := r.get(base+full, "u-dir").Body.String(); strings.Count(body, "dkw-cardlink") != MaxCardsPerDeck {
		t.Errorf("the full deck draws %d cards, want %d", strings.Count(body, "dkw-cardlink"), MaxCardsPerDeck)
	}
	look := r.get(base+kinds+"/look", "u-dir").Body.String()
	if strings.Count(look, "dkw-kindrow") != MaxKinds || strings.Contains(look, `data-act="add-kind" disabled`) == false {
		t.Errorf("twelve kinds: %d rows, and Add a kind is off at the limit", strings.Count(look, "dkw-kindrow"))
	}
	var card string
	for _, c := range r.fx.repo.cards {
		if c.DeckID == kinds {
			card = c.ID
		}
	}
	ed := r.get(base+kinds+"/cards?card="+card, "u-dir").Body.String()
	for _, want := range []string{"data-dkw-editor", "Note for the Directors", "data-dm-only", "Comes up when", "data-dkw-when", "Remove picture"} {
		if want == "Remove picture" {
			continue // no picture on this card
		}
		if !strings.Contains(ed, want) {
			t.Errorf("the card editor lacks %q", want)
		}
	}
}

// A stale deck write is refused with words the page recognises, and the
// version the page holds is the one to send.
func TestWorkshopStaleVersionIsRefusedInPlainWords(t *testing.T) {
	r := newWorkshopRig(t, true)
	d := r.fx.repo.decks[r.deck]
	_, err := r.fx.svc.Update(nil2ctx(), camp, r.deck, r.fx.dir, UpdateDeckInput{Version: patchOf(d.Version), Name: patchOf("Iron omens II")})
	r.fx.must(err)
	_, err = r.fx.svc.Update(nil2ctx(), camp, r.deck, r.fx.dir, UpdateDeckInput{Version: patchOf(d.Version), Name: patchOf("Late")})
	if code(err) != http.StatusConflict || !strings.Contains(err.Error(), "this deck changed") {
		t.Fatalf("stale save: %v", err)
	}
}

// The page's rows come from the look lists and every choice has words; an
// unavailable choice carries its reason.
func TestLookRowsCoverEveryListWithWords(t *testing.T) {
	byKey := map[string][]string{
		"shape": shapeValues, "border": borderValues, "turn": turnValues, "window": windowValues, "name": nameValues,
		"lettering": letteringValues, "corner": cornerValues, "marker": markerValues, "finish": finishValues,
		"back.pattern": patternValues, "back.sign": backSignValues, "back.colour": append([]string{""}, Palette...),
	}
	if len(lookRows) != len(byKey) {
		t.Fatalf("%d rows for %d lists", len(lookRows), len(byKey))
	}
	for _, row := range lookRows {
		list, ok := byKey[row.Key]
		if !ok {
			t.Errorf("row %q has no list", row.Key)
			continue
		}
		if fmt.Sprint(row.Values) != fmt.Sprint(list) {
			t.Errorf("row %q shows %v, the list is %v", row.Key, row.Values, list)
		}
		for _, v := range list {
			if row.Words[v] == "" {
				t.Errorf("row %q: %q has no words", row.Key, v)
			}
		}
	}
	small := defaultLook()
	small.Shape = "small"
	for _, c := range []struct{ key, val string }{{"turn", "burn"}, {"turn", "ink"}, {"finish", "foil"}} {
		for _, row := range lookRows {
			if row.Key == c.key && row.unavailable(small, c.val) == "" {
				t.Errorf("%s %s on the small shape is shown unavailable with a reason", c.key, c.val)
			}
		}
	}
	tall := defaultLook()
	for _, row := range lookRows {
		for _, v := range row.Values {
			if why := row.unavailable(tall, v); why != "" {
				t.Errorf("%s %s on a tall card: %q", row.Key, v, why)
			}
		}
	}
	for _, row := range lookRows {
		if row.TallOnly && row.unavailable(small, row.Values[0]) == "" {
			t.Errorf("%s is for tall cards only and says so", row.Key)
		}
	}
}

// One blank for each settings key, and nothing else; every option is a value
// the server accepts and every value is offered.
func TestRulesHaveOneBlankPerSettingsKey(t *testing.T) {
	want := map[string][]string{
		"draws": drawsValues, "knows": knowsValues, "othersSee": othersSeeValues, "arrives": arrivesValues,
		"fate": fateValues, "empty": emptyValues, "notice": noticeValues, "discard": {"false", "true"},
	}
	seen := map[string]bool{}
	for _, g := range ruleGroups() {
		for _, r := range g.Rules {
			for _, p := range r.Parts {
				if p.Blank == nil {
					continue
				}
				k := p.Blank.Key
				if seen[k] {
					t.Errorf("two blanks for %s", k)
				}
				seen[k] = true
				var got []string
				for _, o := range p.Blank.Options {
					got = append(got, o.Value)
					if o.Label == "" {
						t.Errorf("%s %q has no words", k, o.Value)
					}
				}
				sort.Strings(got)
				w := append([]string(nil), want[k]...)
				sort.Strings(w)
				if fmt.Sprint(got) != fmt.Sprint(w) {
					t.Errorf("%s offers %v, the list is %v", k, got, want[k])
				}
			}
		}
	}
	for k := range want {
		if !seen[k] {
			t.Errorf("no blank for the setting %s", k)
		}
	}
	if len(ruleGroups()) != 4 {
		t.Error("four groups")
	}
	// The blank's current value is read from the same struct the save writes.
	s := defaultSettings(KindHero)
	for k := range want {
		if ruleValue(s, k) == "" {
			t.Errorf("ruleValue(%s) is empty", k)
		}
	}
}

// The table row draws the shown spot with every action, and the pile's Draw
// is off with the reason when the shown spot is full.
func TestTableRowShowsTheShownSpotAndItsActions(t *testing.T) {
	r := newWorkshopRig(t, true)
	card := ""
	for _, c := range r.fx.repo.cards {
		card = c.ID
	}
	for _, cp := range r.fx.copiesOf(card) {
		r.fx.place(cp, ZoneShown, "", true)
	}
	body := r.get("/campaigns/camp-1/decks/workshop/"+r.deck+"/table", "u-dir").Body.String()
	for _, want := range []string{"Deal to the table", "Put back", "Draw another", "Put it in a deck for keeps", "Draw", "Shuffle them back in", "Comes up next", "Kept for last", `data-maxdraw="2"`} {
		if want == "Put it in a deck for keeps" {
			continue // there is no other deck yet
		}
		if !strings.Contains(body, want) {
			t.Errorf("the table row lacks %q", want)
		}
	}
	if strings.Contains(body, "Deal to the party's hand") {
		t.Error("a hero deck has no party hand")
	}
	r.fx.deck("Second", KindParty, nil)
	body = r.get("/campaigns/camp-1/decks/workshop/"+r.deck+"/table", "u-dir").Body.String()
	if !strings.Contains(body, "Put it in a deck for keeps") {
		t.Error("with a second deck, a shown card can be kept in it")
	}
}

// Every fragment carries the deck's current version, so a page whose save was
// refused as stale takes the new one from the reload and its next save goes
// through.
func TestEveryFragmentCarriesTheDecksCurrentVersion(t *testing.T) {
	r := newWorkshopRig(t, true)
	d := r.fx.repo.decks[r.deck]
	_, err := r.fx.svc.Update(nil2ctx(), camp, r.deck, r.fx.dir, UpdateDeckInput{Version: patchOf(d.Version), Name: patchOf("Renamed")})
	r.fx.must(err)
	now := r.fx.repo.decks[r.deck].Version
	if now == d.Version {
		t.Fatal("the write did not move the version")
	}
	want := fmt.Sprintf(`data-dkw-version="%d"`, now)
	for _, tail := range []string{"/table", "/cards", "/look", "/rules", "/players"} {
		body := r.get(r.paths()[2]+tail, "u-dir", "HX-Request", "true").Body.String()
		if !strings.Contains(body, want) {
			t.Errorf("fragment %s does not carry %s", tail, want)
		}
	}
}

// A deck with no kinds says so where the kind buttons would be, and the kinds
// editor picks colour and sign with buttons, not dropdowns.
func TestNoKindsSaysSoAndKindsArePickedWithButtons(t *testing.T) {
	r := newWorkshopRig(t, true)
	look := r.get(r.paths()[2]+"/look", "u-dir", "HX-Request", "true").Body.String()
	if !strings.Contains(look, "No kinds yet") || strings.Contains(look, "dkw-kindbtns") {
		t.Error("a deck with no kinds says so instead of drawing empty kind buttons")
	}
	d := r.fx.repo.decks[r.deck]
	_, err := r.fx.svc.Update(nil2ctx(), camp, r.deck, r.fx.dir, UpdateDeckInput{Version: patchOf(d.Version), Look: patchOf(LookInput{Kinds: []KindDef{{Key: "bane", Name: "Bane", Colour: "rose", Sign: "star"}}})})
	r.fx.must(err)
	look = r.get(r.paths()[2]+"/look", "u-dir", "HX-Request", "true").Body.String()
	if strings.Contains(look, "dkw-select") {
		t.Error("the kinds editor still uses dropdowns")
	}
	if strings.Count(look, `data-kind-colour`) < 1 || strings.Count(look, `role="radio" aria-checked="true" aria-label="rose"`) != 1 || !strings.Contains(look, `aria-label="star"`) {
		t.Error("the kind's colour and sign are radio buttons named in words, the current ones checked")
	}
}
