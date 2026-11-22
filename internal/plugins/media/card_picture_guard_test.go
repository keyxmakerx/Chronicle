package media

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// fakeCardPictureGuard answers the decks plugin's questions from fixed tables
// and records who it was asked about.
type fakeCardPictureGuard struct {
	isPic   bool
	isErr   error
	seen    map[string]bool // by user id
	seenErr error
	ids     []string
	idsErr  error
	uses    []CardPictureUse

	asked   int
	viewers []CardPictureViewer
}

func (f *fakeCardPictureGuard) IsCardPicture(context.Context, string, string) (bool, error) {
	return f.isPic, f.isErr
}

func (f *fakeCardPictureGuard) CardPictureSeenBy(_ context.Context, _, _ string, v CardPictureViewer) (bool, error) {
	f.asked++
	f.viewers = append(f.viewers, v)
	return f.seen[v.UserID], f.seenErr
}

func (f *fakeCardPictureGuard) CardPictureIDs(context.Context, string) ([]string, error) {
	return f.ids, f.idsErr
}

func (f *fakeCardPictureGuard) CardPictureUses(_ context.Context, _, _ string, v CardPictureViewer) ([]CardPictureUse, error) {
	if !v.Director {
		return nil, nil
	}
	return f.uses, nil
}

// cardGuardFixture is a private campaign "camp-1" with a player, a Scribe, the
// owner, a co-Director (a player holding a DM grant) and an outsider.
func cardGuardFixture(g *fakeCardPictureGuard) (*Handler, *stubMemberChecker, *URLSigner) {
	const camp = "camp-1"
	signer := NewURLSigner("test-secret")
	checker := &stubMemberChecker{
		members:   map[string]map[string]bool{camp: {"player": true, "scribe": true, "owner": true, "codm": true}},
		roles:     map[string]map[string]int{camp: {"player": 1, "scribe": 2, "owner": 3, "codm": 1}},
		dmGranted: map[string]map[string]bool{camp: {"codm": true}},
	}
	h := &Handler{signer: signer, memberChecker: checker, cardPictures: g,
		service: &fakeAccessMediaService{}, entityVisibility: &fakeEntityVisibilityFilter{}}
	return h, checker, signer
}

func sessionOf(user string) *auth.Session {
	if user == "" {
		return nil
	}
	return &auth.Session{UserID: user}
}

// capturedLog collects slog output so a test can see that a failure was logged.
func capturedLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return &buf
}

// The guard's decisions for a file that is a card's picture: only a Director,
// or someone a copy of the card is face up to, may open it, thumbnail or not,
// and any failure of the check refuses and logs.
func TestCheckCardPictureGuard(t *testing.T) {
	signer := NewURLSigner("test-secret")
	apiExp, apiSig := signMediaURL(t, signer, "file-1", ViewerAPIKey, time.Minute)
	anonExp, anonSig := signMediaURL(t, signer, "file-1", ViewerAnonymous, time.Minute)
	apiThumbExp, apiThumbSig := signThumbURL(t, signer, "file-1", "300", ViewerAPIKey, time.Minute)

	cases := []struct {
		name     string
		isPic    bool
		isErr    error
		seen     map[string]bool
		seenErr  error
		user     string
		query    map[string]string
		thumb    bool
		allowed  bool
		wantLog  string
		wantAsk  int // how often the decks side was asked whether the viewer sees a face
		pageSees bool
	}{
		{name: "not a card picture: player allowed", isPic: false, user: "player", allowed: true},
		{name: "face up to the player: allowed", isPic: true, seen: map[string]bool{"player": true}, user: "player", allowed: true, wantAsk: 1},
		{name: "face up to the player: thumbnail allowed", isPic: true, seen: map[string]bool{"player": true}, user: "player", thumb: true, allowed: true, wantAsk: 1},
		{name: "face down to the player: refused", isPic: true, user: "player", allowed: false, wantAsk: 1},
		{name: "face down to the player: thumbnail refused", isPic: true, user: "player", thumb: true, allowed: false, wantAsk: 1},
		{name: "a Scribe is a player here", isPic: true, user: "scribe", allowed: false, wantAsk: 1},
		{name: "face up to someone else only", isPic: true, seen: map[string]bool{"scribe": true}, user: "player", allowed: false, wantAsk: 1},
		{name: "owner allowed without asking", isPic: true, user: "owner", allowed: true},
		{name: "co-Director grant allowed without asking", isPic: true, user: "codm", allowed: true},
		{name: "non-member refused and never asked", isPic: true, seen: map[string]bool{"stranger": true}, user: "stranger", allowed: false},
		{name: "anonymous refused", isPic: true, user: "", allowed: false},
		{name: "anonymous-bound signed link refused", isPic: true, user: "", query: map[string]string{"expires": anonExp, "sig": anonSig}, allowed: false},
		{name: "owner's api-key link allowed (sync module)", isPic: true, user: "", query: map[string]string{"expires": apiExp, "sig": apiSig}, allowed: true},
		{name: "owner's api-key thumbnail link allowed", isPic: true, user: "", query: map[string]string{"expires": apiThumbExp, "sig": apiThumbSig}, thumb: true, allowed: true},
		{name: "api-key link presented by a player refused", isPic: true, user: "player", query: map[string]string{"expires": apiExp, "sig": apiSig}, allowed: false, wantAsk: 1},
		{name: "page the viewer sees keeps it visible", isPic: true, user: "player", pageSees: true, allowed: true, wantAsk: 1},
		{name: "the is-card-picture check erroring refuses and logs", isPic: true, isErr: errors.New("db down"), user: "player", allowed: false, wantLog: "card picture check failed"},
		{name: "the visibility check erroring refuses and logs", isPic: true, seen: map[string]bool{"player": true}, seenErr: errors.New("db down"), user: "player", allowed: false, wantAsk: 1, wantLog: "card picture check failed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := &fakeCardPictureGuard{isPic: tc.isPic, isErr: tc.isErr, seen: tc.seen, seenErr: tc.seenErr}
			h, _, _ := cardGuardFixture(g)
			if tc.pageSees {
				h.service = &fakeAccessMediaService{findReferencesFn: func(context.Context, string, string) ([]MediaRef, error) {
					return []MediaRef{{EntityID: "page-1"}}, nil
				}}
				h.entityVisibility = &fakeEntityVisibilityFilter{viewableIDs: map[string]bool{"page-1": true}}
			}
			logs := capturedLog(t)
			size := ""
			if tc.thumb {
				size = "300"
			}
			c := newAccessTestContext(tc.query, sessionOf(tc.user))
			err := accessErr(h.checkCardPictureGuard(c, privateMediaFile(), tc.thumb, size))
			if tc.allowed {
				mustAllow(t, err, tc.name)
			} else {
				mustDeny(t, err, tc.name)
			}
			if g.asked != tc.wantAsk {
				t.Errorf("the decks side was asked %d times, want %d", g.asked, tc.wantAsk)
			}
			if tc.wantLog != "" && !strings.Contains(logs.String(), tc.wantLog) {
				t.Errorf("failure was not logged; log = %q", logs.String())
			}
		})
	}
}

// A co-Director is described to the decks side as a Director, and a player as
// the member they are, from the one promotion formula.
func TestCardPictureViewerFromTheMemberChecker(t *testing.T) {
	g := &fakeCardPictureGuard{isPic: true, seen: map[string]bool{"player": true}}
	h, _, _ := cardGuardFixture(g)
	c := newAccessTestContext(nil, sessionOf("player"))
	mustAllow(t, accessErr(h.checkCardPictureGuard(c, privateMediaFile(), false, "")), "player")
	if len(g.viewers) != 1 {
		t.Fatalf("viewers = %+v", g.viewers)
	}
	if v := g.viewers[0]; v.UserID != "player" || v.MemberRole != 1 || v.VisibilityRole != 1 || v.Director {
		t.Errorf("player described as %+v", v)
	}
	v, ok := h.cardPictureViewer("camp-1", "codm")
	if !ok || !v.Director || v.VisibilityRole != int(campaigns.RoleOwner) || v.MemberRole != 1 {
		t.Errorf("co-Director described as %+v (member=%v)", v, ok)
	}
	if _, ok := h.cardPictureViewer("camp-1", "stranger"); ok {
		t.Error("a non-member was described as a viewer")
	}
}

// A signature proves a link is genuine, not that its holder may see a
// face-down card, so the guard sits behind the base check in checkMediaAccess.
func TestSignedLinksDoNotBypassTheCardPictureGuard(t *testing.T) {
	g := &fakeCardPictureGuard{isPic: true, seen: map[string]bool{}}
	h, _, signer := cardGuardFixture(g)

	// A link minted for the player's own session.
	exp, sig := signMediaURL(t, signer, "file-1", ViewerSession("player"), time.Minute)
	c := newAccessTestContext(map[string]string{"expires": exp, "sig": sig}, sessionOf("player"))
	mustDeny(t, accessErr(h.checkMediaAccess(c, privateMediaFile(), false, "")), "player's own signed link, card face down")

	// The same link once a copy is face up to them.
	g.seen["player"] = true
	mustAllow(t, accessErr(h.checkMediaAccess(c, privateMediaFile(), false, "")), "player's own signed link, card face up")

	// A link minted for the owner's session, replayed in the player's session.
	g.seen["player"] = false
	oExp, oSig := signMediaURL(t, signer, "file-1", ViewerSession("owner"), time.Minute)
	c = newAccessTestContext(map[string]string{"expires": oExp, "sig": oSig}, sessionOf("player"))
	mustDeny(t, accessErr(h.checkMediaAccess(c, privateMediaFile(), false, "")), "a Director's session link used by a player")

	// A thumbnail link for the player, card face down.
	tExp, tSig := signThumbURL(t, signer, "file-1", "300", ViewerSession("player"), time.Minute)
	c = newAccessTestContext(map[string]string{"expires": tExp, "sig": tSig}, sessionOf("player"))
	mustDeny(t, accessErr(h.checkMediaAccess(c, privateMediaFile(), true, "300")), "player's signed thumbnail link, card face down")

	// Unsigned, as a member loads it by session: the "no page references it"
	// branch would admit any member without the guard.
	c = newAccessTestContext(nil, sessionOf("player"))
	mustDeny(t, accessErr(h.checkMediaAccess(c, privateMediaFile(), false, "")), "unsigned member request for a face-down card")
}

// The page rule: a card picture that a page the viewer can see also shows is
// still visible through the page, and one shown only on a page the viewer
// cannot see is not.
func TestCardPictureAlsoOnAPage(t *testing.T) {
	refs := func(context.Context, string, string) ([]MediaRef, error) {
		return []MediaRef{{EntityID: "page-1", EntityName: "Relic"}}, nil
	}
	cases := []struct {
		name    string
		vis     EntityVisibilityFilter
		refsFn  func(context.Context, string, string) ([]MediaRef, error)
		allowed bool
	}{
		{"on a page the player can see", &fakeEntityVisibilityFilter{viewableIDs: map[string]bool{"page-1": true}}, refs, true},
		{"on a page the player cannot see", &fakeEntityVisibilityFilter{viewableIDs: map[string]bool{}}, refs, false},
		{"the reference lookup erroring", &fakeEntityVisibilityFilter{}, func(context.Context, string, string) ([]MediaRef, error) { return nil, errors.New("db") }, false},
		{"the visibility filter erroring", &fakeEntityVisibilityFilter{err: errors.New("db")}, refs, false},
		{"no visibility filter wired", nil, refs, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := &fakeCardPictureGuard{isPic: true}
			h, _, signer := cardGuardFixture(g)
			h.service = &fakeAccessMediaService{findReferencesFn: tc.refsFn}
			h.entityVisibility = tc.vis
			// Through checkMediaAccess with a signed link, which skips the
			// page rule in the base check: the guard must apply it itself.
			exp, sig := signMediaURL(t, signer, "file-1", ViewerSession("player"), time.Minute)
			c := newAccessTestContext(map[string]string{"expires": exp, "sig": sig}, sessionOf("player"))
			err := accessErr(h.checkMediaAccess(c, privateMediaFile(), false, ""))
			if tc.allowed {
				mustAllow(t, err, tc.name)
			} else {
				mustDeny(t, err, tc.name)
			}
		})
	}

	t.Run("an anonymous visitor of a public campaign through a public page", func(t *testing.T) {
		g := &fakeCardPictureGuard{isPic: true}
		h, _, _ := cardGuardFixture(g)
		h.service = &fakeAccessMediaService{findReferencesFn: refs}
		h.entityVisibility = &fakeEntityVisibilityFilter{viewableIDs: map[string]bool{"page-1": true}}
		c := newAccessTestContext(nil, nil)
		mustAllow(t, accessErr(h.checkCardPictureGuard(c, publicMediaFileIn("camp-1"), false, "")), "public campaign")
		mustDeny(t, accessErr(h.checkCardPictureGuard(c, privateMediaFile(), false, "")), "private campaign")
	})
}

func publicMediaFileIn(campaign string) *MediaFile {
	f := privateMediaFile()
	f.CampaignID = &campaign
	f.CampaignIsPublic = boolPtr(true)
	return f
}

// Unset, the guard changes nothing, and the wiring flag says so; files with
// no campaign are never sent to it.
func TestCardPictureGuardWiredFlagAndUnwired(t *testing.T) {
	h := &Handler{}
	if h.CardPictureGuardWired() {
		t.Error("CardPictureGuardWired must be false when unset")
	}
	mustAllow(t, accessErr(h.checkCardPictureGuard(newAccessTestContext(nil, sessionOf("player")), privateMediaFile(), false, "")), "unwired guard")
	g := &fakeCardPictureGuard{isPic: true}
	h.SetCardPictureGuard(g)
	if !h.CardPictureGuardWired() {
		t.Error("CardPictureGuardWired must be true once set")
	}
	f := privateMediaFile()
	f.CampaignID = nil
	mustAllow(t, accessErr(h.checkCardPictureGuard(newAccessTestContext(nil, sessionOf("player")), f, false, "")), "a file with no campaign")
}

// listService records how the picker asked for its page.
type listService struct {
	fakeAccessMediaService
	plain, excluding int
	excluded         []string
}

func (l *listService) ListCampaignMedia(context.Context, string, int, int) ([]MediaFile, int, error) {
	l.plain++
	return nil, 0, nil
}

func (l *listService) ListCampaignMediaExcluding(_ context.Context, _ string, ids []string, _, _ int) ([]MediaFile, int, error) {
	l.excluding++
	l.excluded = ids
	return nil, 0, nil
}

func pickerContext(cc *campaigns.CampaignContext, user string) (*echo.Context, *httptest.ResponseRecorder) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/campaigns/"+cc.Campaign.ID+"/media/list", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPathValues(echo.PathValues{{Name: "id", Value: cc.Campaign.ID}})
	c.Set("campaign_context", cc)
	auth.SetSession(c, &auth.Session{UserID: user})
	return c, rec
}

// The picker leaves every card picture out for a Scribe, even one a visible
// page also shows (a listing is not how a page shows a picture, and the
// exception would cost a reference scan per picture); a Director keeps them all,
// and a failure refuses the whole list.
func TestPickerLeavesOutCardPictures(t *testing.T) {
	scribe := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1"}, MemberRole: campaigns.RoleScribe, IsMember: true}
	director := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1"}, MemberRole: campaigns.RoleOwner, IsMember: true}
	coDM := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1"}, MemberRole: campaigns.RoleScribe, IsMember: true, IsDmGranted: true}

	build := func(g *fakeCardPictureGuard) (*Handler, *listService) {
		g.ids = []string{"pic-a", "pic-b", "pic-c"}
		svc := &listService{}
		h, _, _ := cardGuardFixture(g)
		h.service = svc
		return h, svc
	}

	t.Run("a Scribe loses every card picture and no page is scanned", func(t *testing.T) {
		h, svc := build(&fakeCardPictureGuard{})
		svc.findReferencesFn = func(context.Context, string, string) ([]MediaRef, error) {
			t.Error("the picker scanned page references")
			return nil, nil
		}
		c, rec := pickerContext(scribe, "scribe")
		if err := h.CampaignMediaList(c); err != nil {
			t.Fatal(err)
		}
		if svc.excluding != 1 || strings.Join(svc.excluded, ",") != "pic-a,pic-b,pic-c" {
			t.Errorf("excluded %v (excluding calls %d), want all three", svc.excluded, svc.excluding)
		}
		if rec.Code != http.StatusOK {
			t.Errorf("status %d", rec.Code)
		}
	})
	t.Run("a Director and a co-Director lose nothing and ask nothing", func(t *testing.T) {
		for _, cc := range []*campaigns.CampaignContext{director, coDM} {
			h, svc := build(&fakeCardPictureGuard{idsErr: errors.New("must not be asked")})
			c, _ := pickerContext(cc, "owner")
			if err := h.CampaignMediaList(c); err != nil {
				t.Fatalf("Director list: %v", err)
			}
			if svc.plain != 1 || svc.excluding != 0 {
				t.Errorf("plain=%d excluding=%d, want the plain list", svc.plain, svc.excluding)
			}
		}
	})
	t.Run("with no card pictures it is the plain list", func(t *testing.T) {
		g := &fakeCardPictureGuard{}
		h, svc := build(g)
		g.ids = nil
		c, _ := pickerContext(scribe, "scribe")
		if err := h.CampaignMediaList(c); err != nil {
			t.Fatal(err)
		}
		if svc.plain != 1 || svc.excluding != 0 {
			t.Errorf("plain=%d excluding=%d", svc.plain, svc.excluding)
		}
	})
	t.Run("a failure refuses the whole list", func(t *testing.T) {
		h, svc := build(&fakeCardPictureGuard{idsErr: errors.New("db")})
		logs := capturedLog(t)
		c, _ := pickerContext(scribe, "scribe")
		if err := h.CampaignMediaList(c); err == nil {
			t.Error("the list was served despite the failure")
		}
		if svc.plain+svc.excluding != 0 {
			t.Error("media was listed despite the failure")
		}
		if !strings.Contains(logs.String(), "refusing the media list") {
			t.Errorf("failure not logged: %q", logs.String())
		}
	})
}

// "Where is this used" names a card to a Director only; a Scribe still gets
// the pages they can see and no card.
func TestMediaRefsNameCardsToDirectorsOnly(t *testing.T) {
	g := &fakeCardPictureGuard{uses: []CardPictureUse{{DeckName: "Omens", CardName: "The Drowned Bell"}}}
	h, _, _ := cardGuardFixture(g)
	svc := &fakeAccessMediaService{findReferencesFn: func(context.Context, string, string) ([]MediaRef, error) {
		return []MediaRef{{EntityID: "p", EntityName: "Town Square", EntitySlug: "town", RefType: "content"}}, nil
	}}
	h.service = svc
	h.entityVisibility = &fakeEntityVisibilityFilter{viewableIDs: map[string]bool{"p": true}}

	for _, tc := range []struct {
		name string
		cc   *campaigns.CampaignContext
		want bool
	}{
		{"owner", &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1"}, MemberRole: campaigns.RoleOwner}, true},
		{"co-Director", &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1"}, MemberRole: campaigns.RolePlayer, IsDmGranted: true}, true},
		{"scribe", &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1"}, MemberRole: campaigns.RoleScribe}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, rec := newCampaignRefsTestContext(tc.cc, "u")
			if err := h.CampaignMediaRefs(c); err != nil {
				t.Fatal(err)
			}
			body := rec.Body.String()
			if got := strings.Contains(body, "The Drowned Bell"); got != tc.want {
				t.Errorf("card named = %v, want %v; body=%s", got, tc.want, body)
			}
			if tc.want && !strings.Contains(body, "Omens") {
				t.Errorf("the deck is not named: %s", body)
			}
			if !strings.Contains(body, "Town Square") {
				t.Errorf("the page reference went missing: %s", body)
			}
		})
	}
}

// The cookieless links the notes app is given open the original with no
// session, so a card's picture gets one only while the card rule admits that
// member.
func TestSignedLinksForMemberRespectTheCardRule(t *testing.T) {
	path := "/media/" + linkNoteAudio
	for _, tc := range []struct {
		name  string
		guard *fakeCardPictureGuard
		want  bool
	}{
		{"not a card picture", &fakeCardPictureGuard{}, true},
		{"face down to the member", &fakeCardPictureGuard{isPic: true}, false},
		{"face up to the member", &fakeCardPictureGuard{isPic: true, seen: map[string]bool{"player": true}}, true},
		{"the check failing", &fakeCardPictureGuard{isErr: errors.New("db")}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newLinkHandler()
			h.cardPictures = tc.guard
			got := h.SignedLinksForMember(context.Background(), "camp-l", "player", []string{path})
			if (got[path] != "") != tc.want {
				t.Errorf("link given = %v, want %v", got[path] != "", tc.want)
			}
		})
	}
}
