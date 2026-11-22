package decks

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v5"
)

// heldMoments is the in-memory moment store with open moments in it, so the
// routes can be read while something is waiting.
type heldMoments struct {
	memMoments
	list []Moment
}

func (h heldMoments) ListOpen(context.Context, string) ([]Moment, error) { return h.list, nil }
func (h heldMoments) Get(_ context.Context, _, id string) (Moment, bool, error) {
	for _, m := range h.list {
		if m.ID == id {
			return m, true, nil
		}
	}
	return Moment{}, false, nil
}
func (h heldMoments) InvitedFor(_ context.Context, _, deckID, userID string) (Moment, bool, error) {
	for _, m := range h.list {
		if m.DeckID == deckID && m.State == MomentInvited && m.InvitedUserID == userID {
			return m, true, nil
		}
	}
	return Moment{}, false, nil
}

// heldRig is a hero deck held by Tam whose card carries a test the Director
// kept to themselves, with a moment open for it or not.
type heldRig struct {
	*workshopRig
	deck, card, copyID string
}

// newHeldRig builds the world. holds says whether the hidden test holds;
// state is the moment's state, "" for none; invited is who it invites.
func newHeldRig(t *testing.T, holds bool, state, invited string) *heldRig {
	t.Helper()
	r := newWorkshopRig(t, true)
	fx := r.fx
	f := newFakeFacts()
	if holds {
		f.sheet("h-tam", map[string]float64{"hp": 2, "hpmax": 20})
		f.tags["h-tam"] = []TagFact{{ID: 8, Name: "Secretly-a-spy"}}
	} else {
		f.sheet("h-tam", map[string]float64{"hp": 20, "hpmax": 20})
	}
	newFactsFixture(t, fx, f)
	deck := fx.deck("Orrin's omens", KindHero, &SettingsInput{Knows: ptr("kinds"), Arrives: ptr("down"), OthersSee: ptr("face"), Draws: ptr("players")})
	_, err := fx.svc.Update(bg, camp, deck, fx.dir, UpdateDeckInput{Version: patchOf(1), HolderEntityID: patchOf("h-tam")})
	fx.must(err)
	fx.choose(deck, PlayerInput{UserID: "u-tam", DrawsLeft: ptr(5)}, PlayerInput{UserID: "u-ilse"})
	card := fx.card(deck, CreateCardInput{Name: "Moonwell", Words: "MOONWELL-WORDS", Copies: ptr(2),
		When: WhenList{
			{Test: TestNumber, Who: "h-tam", Field: "hp", Op: "gte", Value: fl(1), Open: true},
			{Test: TestTag, Who: "h-tam", Tag: it(8), Has: bl(true)},
		}})
	res, err := fx.svc.DealCard(bg, camp, deck, fx.dir, DealCardInput{CardID: card, To: "hand", HeroID: "h-tam"})
	fx.must(err)
	var open []Moment
	if state != "" {
		open = []Moment{{ID: "m0000000000000000000000000000001", CampaignID: camp, DeckID: deck, CardID: card, State: state,
			InvitedUserID: invited, CreatedAt: time.Now(), FitCount: 1}}
	}
	fx.svc = NewService(fx.repo, fx.heroes, fx.members, WithNotifier(fx.cap), WithBell(fx.cap), WithPictures(fx.pics),
		WithRandom(func(int) int { return 0 }), WithFacts(f), WithMoments(heldMoments{memMoments: memMoments{repo: fx.repo}, list: open}))
	e := echo.New()
	e.HTTPErrorHandler = r.e.HTTPErrorHandler
	RegisterRoutes(e, NewHandler(fx.svc), guardCampaigns{}, guardAuth{}, guardAddons{on: true})
	r.e = e
	return &heldRig{workshopRig: r, deck: deck, card: card, copyID: res.CopyID}
}

const panelPath = "/campaigns/camp-1/decks/moments/panel"

// A player, a scribe and a stranger are told the fragment is not there, and
// with the add-on off nobody is given it.
func TestTheMomentsPanelIsTheDirectorsAlone(t *testing.T) {
	r := newHeldRig(t, true, MomentWaiting, "")
	rec := r.get(panelPath, "u-dir")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Moonwell") {
		t.Fatalf("the Director got %d: %.300s", rec.Code, rec.Body.String())
	}
	for _, u := range []string{"u-tam", "u-ilse", "u-scribe"} {
		if rec := r.get(panelPath, u); rec.Code != http.StatusNotFound || strings.Contains(rec.Body.String(), "Moonwell") {
			t.Errorf("%s got %d: %.200s", u, rec.Code, rec.Body.String())
		}
	}
	if rec := r.get(panelPath, "u-other"); rec.Code != http.StatusForbidden {
		t.Errorf("a non-member got %d", rec.Code)
	}
	off := newWorkshopRig(t, false)
	if rec := off.get(panelPath, "u-dir"); rec.Code != http.StatusNotFound && rec.Code != http.StatusSeeOther {
		t.Errorf("with the add-on off the Director got %d", rec.Code)
	}
}

// What a player is sent of the deck, the hand and an opened card is the same
// whether a hidden test holds and whether a moment is open for it.
func TestAPlayersHandIsTheSameWhateverAMomentOrAHiddenTestSays(t *testing.T) {
	read := func(holds bool, state string) map[string]string {
		r := newHeldRig(t, holds, state, "")
		out := map[string]string{}
		for _, u := range []string{"u-tam", "u-ilse"} {
			for label, path := range map[string]string{"hand": handBase, "opened": handBase + "/" + r.deck + "/copies/" + r.copyID} {
				for _, hdr := range [][]string{nil, {"HX-Request", "true"}} {
					rec := r.get(path, u, hdr...)
					body := strings.ReplaceAll(rec.Body.String(), r.deck, "DECK")
					body = strings.ReplaceAll(body, r.copyID, "COPY")
					body = strings.ReplaceAll(body, r.card, "CARD")
					out[u+"/"+label+"/"+string(rune('0'+len(hdr)))] = string(rune('0'+rec.Code/100)) + body
				}
			}
		}
		return out
	}
	base := read(false, "")
	for name, world := range map[string]map[string]string{"holds": read(true, ""), "moment": read(true, MomentWaiting)} {
		for k, v := range base {
			if got := stripIDs(world[k]); got != stripIDs(v) {
				t.Errorf("%s differs when a hidden test %s", k, name)
			}
		}
	}
	for k, v := range base {
		for _, bad := range append([]string{"Secretly-a-spy", "fitsSince", "moment", "Waiting on you"}, "whenJson") {
			if strings.Contains(v, bad) {
				t.Errorf("%s names %q", k, bad)
			}
		}
	}
}

// The invited player is told so, with the draw control; the other player is not.
func TestOnlyTheInvitedPlayerIsToldToDraw(t *testing.T) {
	r := newHeldRig(t, true, MomentInvited, "u-tam")
	tam := r.get(handBase, "u-tam").Body.String()
	if !strings.Contains(tam, "Your Director invites you to draw.") {
		t.Errorf("the invited player's hand lacks the invitation: %.400s", tam)
	}
	if ilse := r.get(handBase, "u-ilse").Body.String(); strings.Contains(ilse, "invites you to draw") {
		t.Errorf("another player was told of the invitation")
	}
}

func TestThePrintedHintCountsTheYoursAloneTests(t *testing.T) {
	cases := []struct {
		in   []WhenView
		want string
	}{
		{[]WhenView{{Open: true}}, ""},
		{[]WhenView{{}}, ""},
		{[]WhenView{{Open: true}, {}}, "The second condition is yours alone, so the card shows only the first."},
		{[]WhenView{{Open: true}, {}, {}}, "The second and third conditions are yours alone, so the card shows only the first."},
	}
	for _, c := range cases {
		if got := printedHint(c.in); got != c.want {
			t.Errorf("printedHint(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}
