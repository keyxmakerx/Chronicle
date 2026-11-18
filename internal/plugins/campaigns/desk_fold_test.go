// desk_fold_test.go pins what a desk line unfolds into: the People rows and
// their role words, the rule that a fold never has more than a few rows, the
// plain fallback when a provider cannot answer, and that only the owner can
// ask for a fold or a sentence.

package campaigns

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/templates/layouts"
)

func members(n int, role Role) []CampaignMember {
	out := []CampaignMember{{UserID: "owner", Role: RoleOwner, DisplayName: "Key"}}
	for i := 0; i < n; i++ {
		out = append(out, CampaignMember{UserID: "u" + string(rune('a'+i)), Role: role, DisplayName: "Person " + string(rune('A'+i))})
	}
	return out
}

func TestPeopleFold(t *testing.T) {
	live := &Campaign{ID: "c1", Settings: "{}"}
	gone := &Campaign{ID: "c1", Settings: "{}", ArchivedAt: ptrTime(time.Now())}
	tests := []struct {
		name       string
		c          *Campaign
		in         DeskInputs
		wantRows   int
		wantMore   int
		wantInvite bool
		wantOff    bool
	}{
		{"owner alone", live, DeskInputs{Members: members(0, RolePlayer)}, 1, 0, true, false},
		{"three people", live, DeskInputs{Members: members(3, RolePlayer)}, 4, 0, true, false},
		{"exactly the limit", live, DeskInputs{Members: members(5, RolePlayer)}, 6, 0, true, false},
		{"over the limit leaves the rest to the page", live, DeskInputs{Members: members(9, RolePlayer)}, 6, 4, true, false},
		{"archived is read only", gone, DeskInputs{Members: members(2, RoleScribe)}, 3, 0, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := peopleFold(tt.c, tt.in)
			if len(f.Rows) != tt.wantRows || f.More != tt.wantMore {
				t.Fatalf("rows=%d more=%d, want %d and %d", len(f.Rows), f.More, tt.wantRows, tt.wantMore)
			}
			if (f.Invite != nil) != tt.wantInvite {
				t.Errorf("invite present = %v, want %v", f.Invite != nil, tt.wantInvite)
			}
			// The owner's own role is never a blank: the route refuses it.
			if f.Rows[0].Blank != nil || f.Rows[0].Lead != "You" {
				t.Errorf("first row must be the owner, plain: %+v", f.Rows[0])
			}
			for _, r := range f.Rows[1:] {
				if r.Blank == nil {
					t.Fatalf("%s has no role to change", r.Lead)
				}
				if (r.Blank.Off != "") != tt.wantOff {
					t.Errorf("blank off = %q, want off=%v", r.Blank.Off, tt.wantOff)
				}
				if r.Blank.Method != "PUT" || r.Blank.Field != "role" || r.Blank.Expect != "redirect" ||
					!strings.HasPrefix(r.Blank.URL, "/campaigns/c1/members/") || !strings.HasSuffix(r.Blank.URL, "/role") {
					t.Errorf("role blank must use the existing role route: %+v", r.Blank)
				}
			}
		})
	}
}

func TestPeopleFold_RoleWordsFollowTheCampaignsTerm(t *testing.T) {
	c := &Campaign{ID: "c1", Settings: `{"dm_grant_ids":["ua"]}`}
	in := DeskInputs{GMTerm: "Director", Members: []CampaignMember{
		{UserID: "owner", Role: RoleOwner},
		{UserID: "ua", Role: RoleScribe, DisplayName: "Ana"},
		{UserID: "ub", Role: RoleScribe, DisplayName: "Bo"},
		{UserID: "uc", Role: RolePlayer, DisplayName: "Cy"},
	}}
	f := peopleFold(c, in)
	want := []string{"co-Director", "scribe", "player"}
	for i, w := range want {
		if got := f.Rows[i+1].Blank.Shown; got != w {
			t.Errorf("row %d shows %q, want %q", i+1, got, w)
		}
	}
	var opts []string
	for _, o := range f.Rows[1].Blank.Options {
		opts = append(opts, o.Label)
	}
	if strings.Join(opts, ",") != "player,scribe,co-Director" {
		t.Errorf("options = %v", opts)
	}
}

func TestPeopleLine_WaitingInvites(t *testing.T) {
	c := &Campaign{ID: "c1", Settings: "{}"}
	tests := []struct {
		name    string
		members []CampaignMember
		invited int
		want    string
	}{
		{"none waiting", members(2, RolePlayer), 0, "3 people: you and 2 players."},
		{"one waiting", members(2, RolePlayer), 1, "3 people: you and 2 players, with 1 invited."},
		{"alone with some waiting", members(0, RolePlayer), 2, "Just you, so far, with 2 invited."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := deskText(peopleLine(c, tt.members, tt.invited)); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFoundryFold(t *testing.T) {
	now := time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)
	long := now.Add(-9 * 24 * time.Hour)
	tests := []struct {
		name string
		conn FoundryConnection
		want []string
		not  []string
	}{
		{"never set up", FoundryConnection{}, []string{"No key has been made for Foundry yet."}, nil},
		{"key never used", FoundryConnection{HasKey: true}, []string{"A key was made, but Foundry has not used it yet."}, nil},
		{"connected", FoundryConnection{Connected: true, HasKey: true, HubLastSeen: &now}, []string{"Foundry is connected and answering now."}, []string{"Nothing has come"}},
		{"silent for long", FoundryConnection{HasKey: true, KeyLastUsed: &long, ModuleVersion: "2.3.0", ServedVersion: "2.3.5"},
			[]string{"Nothing has come from Foundry since Monday 9 November at 12:00 UTC.", "The module there is 2.3.0, and Chronicle serves 2.3.5.", "Its key was last used 9 days ago."}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var all []string
			for _, r := range foundryFold(tt.conn, now).Rows {
				all = append(all, r.Text)
			}
			joined := strings.Join(all, "\n")
			for _, w := range tt.want {
				if !strings.Contains(joined, w) {
					t.Errorf("missing %q in\n%s", w, joined)
				}
			}
			for _, n := range tt.not {
				if strings.Contains(joined, n) {
					t.Errorf("must not say %q", n)
				}
			}
			if strings.Contains(joined, "tries") {
				t.Error("the connector does not measure missed tries, so the fold must not state a number of them")
			}
		})
	}
}

// foldStub is a provider that also fills a fold.
type foldStub struct {
	stubProvider
	fold func() (*DeskFold, error)
}

func (f foldStub) Fold(context.Context, DeskRequest) (*DeskFold, error) { return f.fold() }

func TestDeskService_Fold(t *testing.T) {
	c := &Campaign{ID: "c1", Settings: "{}"}
	in := DeskInputs{Campaign: c, Members: members(2, RolePlayer), Addons: map[string]bool{"sync-api": true}}
	trash := func(fn func() (*DeskFold, error)) DeskProvider {
		return foldStub{stubProvider{DeskTrash, func(context.Context) (DeskFacts, error) {
			return DeskFacts{Trash: &DeskTrashFacts{Items: 9, Days: 30}}, nil
		}}, fn}
	}
	rows := func(n int) []DeskFoldRow {
		out := make([]DeskFoldRow, n)
		for i := range out {
			out[i] = DeskFoldRow{Lead: "p"}
		}
		return out
	}
	tests := []struct {
		name         string
		provider     DeskProvider
		line         string
		wantOK       bool
		wantFallback bool
		wantRows     int
		wantMore     int
	}{
		{"people needs no provider", nil, "people", true, false, 3, 0},
		{"unknown line", nil, "nonsense", false, false, 0, 0},
		{"a line with no fold", nil, "keys", false, false, 0, 0},
		{"trash without a provider does not unfold", nil, "trash", false, false, 0, 0},
		{"trash rows are capped, the rest is counted", trash(func() (*DeskFold, error) { return &DeskFold{Rows: rows(9)}, nil }), "trash", true, false, 6, 3},
		{"provider errors, plain sentence and link", trash(func() (*DeskFold, error) { return nil, errors.New("db down") }), "trash", true, true, 0, 0},
		{"provider panics", trash(func() (*DeskFold, error) { panic("boom") }), "trash", true, true, 0, 0},
		{"provider returns nothing", trash(func() (*DeskFold, error) { return nil, nil }), "trash", true, true, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewDeskService()
			if tt.provider != nil {
				s.Register(tt.provider)
			}
			f, ok := s.Fold(context.Background(), DeskRequest{CampaignID: "c1", Now: time.Now()}, in, tt.line)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if !ok {
				return
			}
			if f.Fallback != tt.wantFallback || len(f.Rows) != tt.wantRows || f.More != tt.wantMore {
				t.Errorf("fallback=%v rows=%d more=%d", f.Fallback, len(f.Rows), f.More)
			}
			if f.Fallback && (f.FullHref != "/campaigns/c1/trash" || !strings.Contains(f.Note, "Trash")) {
				t.Errorf("a failed fold must say a plain sentence and link the full page: %+v", f)
			}
		})
	}
}

func TestDeskService_FoldFlagOnLines(t *testing.T) {
	c := &Campaign{ID: "c1", Settings: "{}"}
	in := DeskInputs{Campaign: c, Addons: map[string]bool{"sync-api": true}}
	s := NewDeskService()
	s.Register(foldStub{stubProvider{DeskTrash, func(context.Context) (DeskFacts, error) { return DeskFacts{}, nil }},
		func() (*DeskFold, error) { return &DeskFold{}, nil }})
	d := s.Build(context.Background(), DeskRequest{Now: time.Now()}, in)
	for id, want := range map[string]bool{"people": true, "trash": true, "foundry": false, "ai": false, "changes": false} {
		l := lineByID(d, id)
		if l == nil || l.Fold != want {
			t.Errorf("line %s unfolds = %v, want %v", id, l != nil && l.Fold, want)
		}
	}
}

func TestDeskService_Line(t *testing.T) {
	c := &Campaign{ID: "c1", Settings: "{}"}
	s := NewDeskService()
	s.Register(stubProvider{DeskTrash, func(context.Context) (DeskFacts, error) {
		return DeskFacts{Trash: &DeskTrashFacts{Items: 2, Days: 30}}, nil
	}})
	in := DeskInputs{Campaign: c, Members: members(1, RoleScribe), Invited: 1, Addons: map[string]bool{}}
	tests := []struct {
		id     string
		want   string
		wantOK bool
	}{
		{"people", "2 people: you and 1 scribe, with 1 invited.", true},
		{"trash", "Trash holds 2 items for 30 days.", true},
		{"media", "", false}, // its add-on is off
		{"foundry", "", false},
		{"nonsense", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			l, ok := s.Line(context.Background(), DeskRequest{}, in, tt.id)
			if ok != tt.wantOK || (ok && deskText(l) != tt.want) {
				t.Errorf("got %q ok=%v, want %q ok=%v", deskText(l), ok, tt.want, tt.wantOK)
			}
		})
	}
}

// Only the owner can ask for a fold or a sentence, and a lower role never
// reaches a provider.
func TestDeskFragments_OnlyTheOwner(t *testing.T) {
	routes := []struct {
		name string
		call func(*Handler, *echo.Context) error
		path string
	}{
		{"fold", (*Handler).DeskLineFold, "/campaigns/camp-1/dashboard/lines/trash/fold"},
		{"sentence", (*Handler).DeskLineSentence, "/campaigns/camp-1/dashboard/lines/trash/sentence"},
		{"needs count", (*Handler).DeskNeedsCount, "/campaigns/camp-1/dashboard/needs"},
	}
	roles := []struct {
		name      string
		role      Role
		dmGranted bool
		wantOpen  bool
	}{
		{"player", RolePlayer, false, false},
		{"scribe", RoleScribe, false, false},
		{"co-dm", RolePlayer, true, false},
		{"owner", RoleOwner, false, true},
	}
	for _, rt := range routes {
		for _, r := range roles {
			t.Run(rt.name+" as "+r.name, func(t *testing.T) {
				h := NewHandler(deskStubService{})
				called := false
				h.RegisterDeskProvider(foldStub{stubProvider{DeskTrash, func(context.Context) (DeskFacts, error) {
					called = true
					return DeskFacts{Trash: &DeskTrashFacts{Items: 7, Days: 30}}, nil
				}}, func() (*DeskFold, error) {
					called = true
					return &DeskFold{Rows: []DeskFoldRow{{Lead: "Old Vess Tollhouse", Act: &DeskFoldAct{Label: "Bring back", URL: "/x", Method: "POST"}}}}, nil
				}})
				e := echo.New()
				req := httptest.NewRequest(http.MethodGet, rt.path, nil)
				rec := httptest.NewRecorder()
				c := e.NewContext(req, rec)
				c.SetPathValues(echo.PathValues{{Name: "id", Value: "camp-1"}, {Name: "line", Value: "trash"}})
				c.Set(contextKeyCampaign, &CampaignContext{
					Campaign: &Campaign{ID: "camp-1", Name: "Therin", Settings: "{}"}, MemberRole: r.role, IsDmGranted: r.dmGranted, IsMember: true,
				})
				auth.SetSession(c, &auth.Session{UserID: "u1"})
				err := rt.call(h, c)
				if r.wantOpen {
					if err != nil || rec.Code != http.StatusOK {
						t.Fatalf("owner: err=%v code=%d", err, rec.Code)
					}
					if body := rec.Body.String(); rt.name == "fold" && !strings.Contains(body, "Bring back") {
						t.Errorf("the fold must carry its row's action: %s", body)
					}
					return
				}
				var ae *apperror.AppError
				if !errors.As(err, &ae) || ae.Code != http.StatusForbidden {
					t.Fatalf("want forbidden, got %v", err)
				}
				if called || rec.Body.Len() > 0 {
					t.Error("a lower role reached the desk's numbers")
				}
			})
		}
	}
}

func TestDeskFragments_UnknownLineIsNotFound(t *testing.T) {
	h := NewHandler(deskStubService{})
	for _, call := range []func(*Handler, *echo.Context) error{(*Handler).DeskLineFold, (*Handler).DeskLineSentence} {
		e := echo.New()
		c := e.NewContext(httptest.NewRequest(http.MethodGet, "/x", nil), httptest.NewRecorder())
		c.SetPathValues(echo.PathValues{{Name: "id", Value: "camp-1"}, {Name: "line", Value: "nonsense"}})
		c.Set(contextKeyCampaign, &CampaignContext{Campaign: &Campaign{ID: "camp-1", Settings: "{}"}, MemberRole: RoleOwner, IsMember: true})
		auth.SetSession(c, &auth.Session{UserID: "u1"})
		var ae *apperror.AppError
		if err := call(h, c); !errors.As(err, &ae) || ae.Code != http.StatusNotFound {
			t.Errorf("want not found, got %v", err)
		}
	}
}

func ptrTime(t time.Time) *time.Time { return &t }

// deskSentenceService answers what ManageSentence reads.
type deskSentenceService struct{ CampaignService }

func (deskSentenceService) GetByID(_ context.Context, id string) (*Campaign, error) {
	return &Campaign{ID: id, Name: "Therin", Settings: "{}"}, nil
}
func (deskSentenceService) ListMembers(context.Context, string) ([]CampaignMember, error) {
	return members(3, RolePlayer), nil
}

func sentenceCtx(role int, in bool) context.Context {
	ctx := layouts.SetUserID(context.Background(), "owner")
	if in {
		ctx = layouts.SetCampaignID(ctx, "c1")
		ctx = layouts.SetCampaignRole(ctx, role)
	}
	return ctx
}

// A Manage page's own sentence is the owner's alone and is the same words the
// desk shows, from the same provider.
func TestManageSentence(t *testing.T) {
	h := NewHandler(deskSentenceService{})
	h.RegisterDeskProvider(stubProvider{DeskTrash, func(context.Context) (DeskFacts, error) {
		return DeskFacts{Trash: &DeskTrashFacts{Items: 2, Days: 30}}, nil
	}})
	tests := []struct {
		name string
		ctx  context.Context
		line string
		want string // "" means no sentence at all
	}{
		{"owner, people", sentenceCtx(int(RoleOwner), true), "people", "4 people: you and 3 players."},
		{"owner, trash", sentenceCtx(int(RoleOwner), true), "trash", "Trash holds 2 items for 30 days."},
		{"owner, a line with no page", sentenceCtx(int(RoleOwner), true), "keys", ""},
		{"scribe", sentenceCtx(int(RoleScribe), true), "people", ""},
		{"player", sentenceCtx(int(RolePlayer), true), "people", ""},
		{"owner viewing as a player reads as player", sentenceCtx(int(RolePlayer), true), "trash", ""},
		{"outside a campaign", sentenceCtx(0, false), "people", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			comp := h.ManageSentence(tt.ctx, tt.line)
			if tt.want == "" {
				if comp != nil {
					t.Fatal("this viewer or line must get no sentence")
				}
				return
			}
			if comp == nil {
				t.Fatal("want a sentence")
			}
			var sb strings.Builder
			if err := comp.Render(tt.ctx, &sb); err != nil {
				t.Fatal(err)
			}
			out := sb.String()
			if !strings.Contains(out, tt.want) || !strings.Contains(out, `data-manage-sentence`) || !strings.Contains(out, `data-line="`+tt.line+`"`) {
				t.Errorf("sentence = %s, want it to say %q", out, tt.want)
			}
		})
	}
}

// Only the lines whose page opens with their own sentence carry it; the rest
// are plain links.
func TestDeskLine_CarriesOnlyWhereThePageOpensWithTheSentence(t *testing.T) {
	c := &Campaign{ID: "c1", Settings: "{}"}
	carries := map[string]bool{"people": true, "changes": true, "game": true, "media": true, "foundry": true, "ai": true, "trash": true, "settings": true}
	in := DeskInputs{Campaign: c, Members: members(1, RolePlayer), Addons: map[string]bool{"media-gallery": true, "sync-api": true}}
	d := NewDeskService().Build(context.Background(), DeskRequest{Now: time.Now()}, in)
	for _, g := range d.Groups {
		for _, l := range g.Lines {
			want := carries[l.ID]
			got := false
			for _, a := range l.Acts {
				got = got || a.Carry
			}
			if got != want {
				t.Errorf("line %s carries = %v, want %v", l.ID, got, want)
			}
		}
	}
}

func TestDeskService_NeedsCount(t *testing.T) {
	tests := []struct {
		name   string
		addons map[string]bool
		ai     int
		want   int
	}{
		{"nothing waits", map[string]bool{}, 0, 0},
		{"the AI has a request waiting", map[string]bool{}, 2, 1},
		{"Foundry facts of a campaign without the add-on are not counted", map[string]bool{"sync-api": false}, 1, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewDeskService()
			s.Register(stubProvider{DeskAI, func(context.Context) (DeskFacts, error) {
				return DeskFacts{AI: &DeskAIFacts{Waiting: tt.ai}}, nil
			}})
			// A provider outside Needs-you must not be asked.
			s.Register(stubProvider{DeskTrash, func(context.Context) (DeskFacts, error) {
				t.Error("the trash provider was read for a count")
				return DeskFacts{}, nil
			}})
			in := DeskInputs{Campaign: &Campaign{ID: "c1"}, Addons: tt.addons}
			if got := s.NeedsCount(context.Background(), DeskRequest{Now: time.Now()}, in); got != tt.want {
				t.Fatalf("count = %d, want %d", got, tt.want)
			}
		})
	}
}
