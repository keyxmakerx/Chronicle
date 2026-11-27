package dmscreen

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

func member(role campaigns.Role, dm bool) *campaigns.CampaignContext {
	return &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c1"}, MemberRole: role, IsMember: role > campaigns.RoleNone, IsDmGranted: dm}
}

func errCode(err error) int {
	var ae *apperror.AppError
	if errors.As(err, &ae) {
		return ae.Code
	}
	if err != nil {
		return -1
	}
	return 0
}

func TestScreenAllowed(t *testing.T) {
	tests := []struct {
		name   string
		cc     *campaigns.CampaignContext
		screen string
		want   int
	}{
		{"owner, dm", member(campaigns.RoleOwner, false), ScreenDM, 0},
		{"co-dm, dm", member(campaigns.RoleScribe, true), ScreenDM, 0},
		{"player with DM access, dm", member(campaigns.RolePlayer, true), ScreenDM, 0},
		{"player, dm", member(campaigns.RolePlayer, false), ScreenDM, http.StatusForbidden},
		{"scribe, dm", member(campaigns.RoleScribe, false), ScreenDM, http.StatusForbidden},
		{"player, player", member(campaigns.RolePlayer, false), ScreenPlayer, 0},
		{"scribe, player", member(campaigns.RoleScribe, false), ScreenPlayer, 0},
		{"owner, player", member(campaigns.RoleOwner, false), ScreenPlayer, http.StatusForbidden},
		{"co-dm, player", member(campaigns.RoleScribe, true), ScreenPlayer, http.StatusForbidden},
		{"non-member, player", member(campaigns.RoleNone, false), ScreenPlayer, http.StatusForbidden},
		{"site admin who isn't a member, dm", &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c1"}, IsSiteAdmin: true}, ScreenDM, http.StatusForbidden},
		{"no context", nil, ScreenPlayer, http.StatusForbidden},
		{"unknown screen", member(campaigns.RoleOwner, false), "admin", http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := errCode(ScreenAllowed(tt.cc, tt.screen)); got != tt.want {
				t.Errorf("ScreenAllowed = %d, want %d", got, tt.want)
			}
		})
	}
}

func decodeInput(t *testing.T, body string) UpdateScreenPrefsInput {
	t.Helper()
	var in UpdateScreenPrefsInput
	if err := json.Unmarshal([]byte(body), &in); err != nil {
		t.Fatalf("decoding %s: %v", body, err)
	}
	return in
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// The PUT is partial: absent keeps, null clears, a value replaces.
func TestMergePrefs(t *testing.T) {
	home := Arrangement{Open: []string{"world", "party"}, Sizes: map[string]string{"party": "S"}}
	stored := ScreenPrefs{
		Current: "Fight night",
		Home:    &home,
		Live:    &Arrangement{Open: []string{"fight"}},
		Layouts: []Layout{{Name: "Fight night", Arrangement: Arrangement{Open: []string{"fight"}, Cols: 2}}},
		Style:   Style{Surface: "glass", Density: "compact"},
	}
	tests := []struct {
		name  string
		body  string
		check func(t *testing.T, p ScreenPrefs)
	}{
		{"empty body keeps everything", `{}`, func(t *testing.T, p ScreenPrefs) {
			if mustJSON(t, p) != mustJSON(t, stored) {
				t.Errorf("changed: %s", mustJSON(t, p))
			}
		}},
		{"style alone keeps the layouts", `{"style":{"surface":"felt"}}`, func(t *testing.T, p ScreenPrefs) {
			if p.Style.Surface != "felt" || p.Style.Density != "" || len(p.Layouts) != 1 || p.Current != "Fight night" || p.Home == nil {
				t.Errorf("got %s", mustJSON(t, p))
			}
		}},
		{"null style is the default", `{"style":null}`, func(t *testing.T, p ScreenPrefs) {
			if p.Style != (Style{}) {
				t.Errorf("style %+v", p.Style)
			}
		}},
		{"null current is Default and drops the live screen", `{"current":null}`, func(t *testing.T, p ScreenPrefs) {
			if p.Current != "" || p.Live != nil || p.Home == nil {
				t.Errorf("got %s", mustJSON(t, p))
			}
		}},
		{"null home is the starting arrangement", `{"home":null}`, func(t *testing.T, p ScreenPrefs) {
			if p.Home != nil || p.Current != "Fight night" {
				t.Errorf("got %s", mustJSON(t, p))
			}
		}},
		{"null layouts leaves Default current", `{"layouts":null}`, func(t *testing.T, p ScreenPrefs) {
			if len(p.Layouts) != 0 || p.Current != "" || p.Live != nil {
				t.Errorf("got %s", mustJSON(t, p))
			}
		}},
		{"home replaced", `{"home":{"open":["notes"],"leaves":[["rules"]]}}`, func(t *testing.T, p ScreenPrefs) {
			if p.Home == nil || strings.Join(p.Home.Open, ",") != "notes" || len(p.Home.Leaves) != 1 {
				t.Errorf("got %s", mustJSON(t, p))
			}
		}},
		{"current matched without case", `{"current":"fight NIGHT"}`, func(t *testing.T, p ScreenPrefs) {
			if p.Current != "Fight night" {
				t.Errorf("current %q", p.Current)
			}
		}},
		{"a missing current layout reads as Default", `{"current":"Downtime"}`, func(t *testing.T, p ScreenPrefs) {
			if p.Current != "" || p.Live != nil {
				t.Errorf("got %s", mustJSON(t, p))
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := mergePrefs(stored, decodeInput(t, tt.body))
			if err != nil {
				t.Fatalf("mergePrefs: %v", err)
			}
			tt.check(t, got)
		})
	}
}

// An arrangement is stored in one shape: repeats dropped, a card in one
// place, defaults left out.
func TestCleanArrangement(t *testing.T) {
	got, err := cleanArrangement(Arrangement{
		Open:   []string{"world", "party", "world", "rules"},
		Leaves: [][]string{{"rules", "rules"}, {}, {"notes"}},
		Sizes:  map[string]string{"world": "L", "party": "M"},
		Cols:   1,
	})
	if err != nil {
		t.Fatal(err)
	}
	want := `{"open":["world","party"],"leaves":[["rules"],["notes"]],"sizes":{"party":"M"}}`
	if s := mustJSON(t, got); s != want {
		t.Errorf("got %s\nwant %s", s, want)
	}
}

func TestCleanPrefsRefuses(t *testing.T) {
	many := make([]Layout, maxLayouts+1)
	for i := range many {
		many[i] = Layout{Name: strings.Repeat("x", i+1)}
	}
	big := make([]Layout, maxLayouts)
	for i := range big {
		ids := make([]string, maxSectionIDs)
		for j := range ids {
			ids[j] = "section-" + strings.Repeat("a", 18) + string(rune('a'+j%26)) + string(rune('a'+i))
		}
		big[i] = Layout{Name: "Layout " + string(rune('A'+i)), Arrangement: Arrangement{Open: ids, Folded: ids, Wide: ids, Hidden: ids}}
	}
	tests := []struct {
		name string
		p    ScreenPrefs
	}{
		{"a section id with markup", ScreenPrefs{Home: &Arrangement{Open: []string{`<img src=x>`}}}},
		{"an upper-case id", ScreenPrefs{Home: &Arrangement{Open: []string{"World"}}}},
		{"an over-long id", ScreenPrefs{Home: &Arrangement{Open: []string{strings.Repeat("a", 33)}}}},
		{"three leaves", ScreenPrefs{Home: &Arrangement{Leaves: [][]string{{"a"}, {"b"}, {"c"}}}}},
		{"a bad height", ScreenPrefs{Home: &Arrangement{Sizes: map[string]string{"world": "XL"}}}},
		{"a sized bad id", ScreenPrefs{Home: &Arrangement{Sizes: map[string]string{"<b>": "S"}}}},
		{"three columns", ScreenPrefs{Home: &Arrangement{Cols: 3}}},
		{"nine layouts", ScreenPrefs{Layouts: many}},
		{"an empty name", ScreenPrefs{Layouts: []Layout{{Name: "   "}}}},
		{"a name of 41 characters", ScreenPrefs{Layouts: []Layout{{Name: strings.Repeat("é", 41)}}}},
		{"a name with a control character", ScreenPrefs{Layouts: []Layout{{Name: "Fight\nnight"}}}},
		{"a layout called Default", ScreenPrefs{Layouts: []Layout{{Name: "default"}}}},
		{"two layouts with one name", ScreenPrefs{Layouts: []Layout{{Name: "Fight"}, {Name: "fight"}}}},
		{"a bad surface", ScreenPrefs{Style: Style{Surface: "neon"}}},
		{"a bad density", ScreenPrefs{Style: Style{Density: "tiny"}}},
		{"a bad tray level", ScreenPrefs{Style: Style{TrayLevel: "ultra"}}},
		{"a bad tray finish", ScreenPrefs{Style: Style{TrayFinish: "gold"}}},
		{"a bad tray shape", ScreenPrefs{Style: Style{TrayShape: "cup"}}},
		{"a bad Others' dice value", ScreenPrefs{Style: Style{TrayOthers: "maybe"}}},
		{"a tray key that is script", ScreenPrefs{Style: Style{TrayLevel: "<script>"}}},
		{"a document past the size cap", ScreenPrefs{Layouts: big}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := cleanPrefs(tt.p); errCode(err) != http.StatusBadRequest {
				t.Errorf("want 400, got %v", err)
			}
		})
	}
	// Forty characters is fine, counted as characters, not bytes.
	if _, err := cleanPrefs(ScreenPrefs{Layouts: []Layout{{Name: strings.Repeat("é", 40)}}}); err != nil {
		t.Errorf("40 characters refused: %v", err)
	}
}

// Keys the screen never writes are dropped on the way in.
func TestPrefsUnknownFieldsDropped(t *testing.T) {
	in := decodeInput(t, `{"home":{"open":["world"],"script":"alert(1)"},"style":{"surface":"felt","colour":"red"},"owner":"u2"}`)
	got, err := mergePrefs(ScreenPrefs{}, in)
	if err != nil {
		t.Fatal(err)
	}
	s := mustJSON(t, got)
	for _, bad := range []string{"script", "colour", "owner", "u2"} {
		if strings.Contains(s, bad) {
			t.Errorf("stored %q: %s", bad, s)
		}
	}
}

// fakePrefsRepo keeps rows by campaign, user and screen and records which
// rows were touched.
type fakePrefsRepo struct {
	rows    map[string][]byte
	touched []string
	err     error
}

func prefsKey(c, u, s string) string { return c + "|" + u + "|" + s }

func (f *fakePrefsRepo) Get(_ context.Context, c, u, s string) ([]byte, bool, error) {
	f.touched = append(f.touched, "get "+prefsKey(c, u, s))
	if f.err != nil {
		return nil, false, f.err
	}
	d, ok := f.rows[prefsKey(c, u, s)]
	return d, ok, nil
}

func (f *fakePrefsRepo) Update(_ context.Context, c, u, s string, fn func([]byte) ([]byte, error)) error {
	f.touched = append(f.touched, "put "+prefsKey(c, u, s))
	if f.err != nil {
		return f.err
	}
	next, err := fn(f.rows[prefsKey(c, u, s)])
	if err != nil {
		return err
	}
	f.rows[prefsKey(c, u, s)] = next
	return nil
}

type fakeKits struct{ info KitInfo }

func (k fakeKits) ScreenKit(context.Context, *campaigns.CampaignContext, string, string) (KitInfo, error) {
	return k.info, nil
}

// Every read and write is of the caller's own row.
func TestPrefsServiceOwnRowOnly(t *testing.T) {
	repo := &fakePrefsRepo{rows: map[string][]byte{
		prefsKey("c1", "u2", ScreenDM): []byte(`{"current":"Theirs","layouts":[{"name":"Theirs","open":["reveal"]}],"style":{"surface":"felt"}}`),
	}}
	svc := NewPrefsService(repo, fakeKits{KitInfo{Campaign: "sharp", CanPick: true, Part: "dmscreen"}})
	ctx := context.Background()
	owner := member(campaigns.RoleOwner, false)

	v, err := svc.Get(ctx, owner, "u1", ScreenDM)
	if err != nil {
		t.Fatal(err)
	}
	if v.Stored || len(v.Prefs.Layouts) != 0 || v.Prefs.Style.Surface != "" {
		t.Errorf("u1 saw another user's row: %+v", v)
	}
	if v.Kit == nil || v.Kit.Campaign != "sharp" {
		t.Errorf("kit info missing: %+v", v.Kit)
	}

	if _, err := svc.Update(ctx, owner, "u1", ScreenDM, decodeInput(t, `{"style":{"surface":"glass"}}`)); err != nil {
		t.Fatal(err)
	}
	if got := string(repo.rows[prefsKey("c1", "u2", ScreenDM)]); !strings.Contains(got, `"Theirs"`) || strings.Contains(got, "glass") {
		t.Errorf("u2's row changed: %s", got)
	}
	if got := string(repo.rows[prefsKey("c1", "u1", ScreenDM)]); !strings.Contains(got, `"surface":"glass"`) {
		t.Errorf("u1's row not written: %s", got)
	}
	for _, k := range repo.touched {
		if strings.Contains(k, "u2") {
			t.Errorf("touched another user's row: %s", k)
		}
	}

	v, err = svc.Get(ctx, owner, "u1", ScreenDM)
	if err != nil || !v.Stored || v.Prefs.Style.Surface != "glass" {
		t.Errorf("read back %+v, %v", v, err)
	}
}

// A player asking for the DM Screen's rows, or a Director for the players',
// is refused before the store is asked.
func TestPrefsServiceGates(t *testing.T) {
	tests := []struct {
		name   string
		cc     *campaigns.CampaignContext
		screen string
		want   int
	}{
		{"player asks for dm", member(campaigns.RolePlayer, false), ScreenDM, http.StatusForbidden},
		{"scribe asks for dm", member(campaigns.RoleScribe, false), ScreenDM, http.StatusForbidden},
		{"owner asks for player", member(campaigns.RoleOwner, false), ScreenPlayer, http.StatusForbidden},
		{"non-member asks for player", member(campaigns.RoleNone, false), ScreenPlayer, http.StatusForbidden},
		{"unknown screen", member(campaigns.RoleOwner, false), "x", http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakePrefsRepo{rows: map[string][]byte{}}
			svc := NewPrefsService(repo, nil)
			if _, err := svc.Get(context.Background(), tt.cc, "u1", tt.screen); errCode(err) != tt.want {
				t.Errorf("Get: want %d, got %v", tt.want, err)
			}
			if _, err := svc.Update(context.Background(), tt.cc, "u1", tt.screen, decodeInput(t, `{"style":{"surface":"felt"}}`)); errCode(err) != tt.want {
				t.Errorf("Update: want %d, got %v", tt.want, err)
			}
			if len(repo.touched) != 0 {
				t.Errorf("store reached: %v", repo.touched)
			}
		})
	}
}

// Without the plugin's table, reads say nothing is stored and saves are
// refused, so the screen keeps its layouts in the browser.
func TestPrefsServiceWithoutStore(t *testing.T) {
	svc := NewPrefsService(nil, nil)
	v, err := svc.Get(context.Background(), member(campaigns.RolePlayer, false), "u1", ScreenPlayer)
	if err != nil || v.Stored || v.Kit != nil {
		t.Errorf("Get = %+v, %v", v, err)
	}
	if _, err := svc.Update(context.Background(), member(campaigns.RolePlayer, false), "u1", ScreenPlayer, decodeInput(t, `{}`)); errCode(err) != http.StatusInternalServerError {
		t.Errorf("Update: want 500, got %v", err)
	}
}

// A refused document is never stored.
func TestPrefsServiceRefusalStoresNothing(t *testing.T) {
	repo := &fakePrefsRepo{rows: map[string][]byte{prefsKey("c1", "u1", ScreenDM): []byte(`{"style":{"surface":"felt"}}`)}}
	svc := NewPrefsService(repo, nil)
	_, err := svc.Update(context.Background(), member(campaigns.RoleOwner, false), "u1", ScreenDM, decodeInput(t, `{"layouts":[{"name":"Default","open":[]}]}`))
	if errCode(err) != http.StatusBadRequest {
		t.Fatalf("want 400, got %v", err)
	}
	if got := string(repo.rows[prefsKey("c1", "u1", ScreenDM)]); got != `{"style":{"surface":"felt"}}` {
		t.Errorf("row changed: %s", got)
	}
}

// stubPrefsSvc records whether a handler reached it.
type stubPrefsSvc struct {
	calls int
	in    UpdateScreenPrefsInput
	user  string
}

func (s *stubPrefsSvc) Get(_ context.Context, _ *campaigns.CampaignContext, u, sc string) (*PrefsView, error) {
	s.calls++
	s.user = u
	return &PrefsView{Screen: sc}, nil
}
func (s *stubPrefsSvc) Update(_ context.Context, _ *campaigns.CampaignContext, u, sc string, in UpdateScreenPrefsInput) (*PrefsView, error) {
	s.calls++
	s.user = u
	s.in = in
	return &PrefsView{Screen: sc, Stored: true}, nil
}
func (s *stubPrefsSvc) Allowed(cc *campaigns.CampaignContext, sc string) error {
	return ScreenAllowed(cc, sc)
}

// The routes put screenGate in front of each handler; a player is refused on
// the DM Screen's routes before the handler runs, and the user id is always
// the session's, whatever the body says.
func TestPrefsHandlerGate(t *testing.T) {
	tests := []struct {
		name   string
		cc     *campaigns.CampaignContext
		screen string
		method string
		body   string
		want   int
	}{
		{"player reads dm", member(campaigns.RolePlayer, false), ScreenDM, http.MethodGet, "", http.StatusForbidden},
		{"player writes dm", member(campaigns.RolePlayer, false), ScreenDM, http.MethodPut, `{"style":{"surface":"felt"}}`, http.StatusForbidden},
		{"co-dm writes player", member(campaigns.RoleScribe, true), ScreenPlayer, http.MethodPut, `{}`, http.StatusForbidden},
		{"owner reads dm", member(campaigns.RoleOwner, false), ScreenDM, http.MethodGet, "", 0},
		{"player writes player", member(campaigns.RolePlayer, false), ScreenPlayer, http.MethodPut, `{"style":{"surface":"felt"},"userId":"u2"}`, 0},
		{"a body that is not JSON", member(campaigns.RoleOwner, false), ScreenDM, http.MethodPut, `{`, http.StatusBadRequest},
		{"an oversized body", member(campaigns.RoleOwner, false), ScreenDM, http.MethodPut, `{"home":{"open":["` + strings.Repeat("a", maxPrefsBody) + `"]}}`, http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &stubPrefsSvc{}
			h := NewPrefsHandler(svc)
			e := echo.New()
			req := httptest.NewRequest(tt.method, "/", strings.NewReader(tt.body))
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.Set("campaign_context", tt.cc)
			c.Set("auth_user_id", "u1")
			next := h.Get(tt.screen)
			if tt.method == http.MethodPut {
				next = h.Put(tt.screen)
			}
			err := screenGate(tt.screen)(next)(c)
			if got := errCode(err); got != tt.want {
				t.Fatalf("want %d, got %v", tt.want, err)
			}
			if tt.want == http.StatusForbidden && svc.calls != 0 {
				t.Errorf("the service was reached %d times", svc.calls)
			}
			if tt.want == 0 && svc.user != "u1" {
				t.Errorf("service got user %q, want the session's", svc.user)
			}
			if tt.name == "player writes player" && (!svc.in.Style.Present() || svc.in.Layouts.Present() || svc.in.Home.Present()) {
				t.Errorf("presence not kept: %+v", svc.in)
			}
		})
	}
}

// The dice tray's choices are stored only when they differ from the starting
// ones (Detailed, Paper, tower, On), and each stored one survives a merge with
// a style change that does not name it.
func TestPrefsTrayStyle(t *testing.T) {
	tests := []struct {
		name string
		body string
		want Style
	}{
		{"the starting values are not stored", `{"style":{"trayLevel":"detailed","trayFinish":"paper","trayShape":"tower","trayOthers":"on"}}`, Style{}},
		{"every other value is kept", `{"style":{"trayLevel":"flat","trayFinish":"kit","trayShape":"bowl","trayOthers":"off"}}`, Style{TrayLevel: "flat", TrayFinish: "kit", TrayShape: "bowl", TrayOthers: "off"}},
		{"simple is kept", `{"style":{"trayLevel":"simple"}}`, Style{TrayLevel: "simple"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := mergePrefs(ScreenPrefs{}, decodeInput(t, tt.body))
			if err != nil {
				t.Fatal(err)
			}
			if got.Style != tt.want {
				t.Errorf("style = %+v, want %+v", got.Style, tt.want)
			}
		})
	}
	// A body that names an unknown value is refused through the same path a
	// PUT takes.
	if _, err := mergePrefs(ScreenPrefs{}, decodeInput(t, `{"style":{"trayLevel":"ultra"}}`)); errCode(err) != http.StatusBadRequest {
		t.Errorf("unknown level: want 400, got %v", err)
	}
}
