package campaigns

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
)

func TestHomeAudience(t *testing.T) {
	tests := []struct {
		name string
		cc   CampaignContext
		want string
	}{
		{"owner", CampaignContext{MemberRole: RoleOwner, IsMember: true}, HomeDirector},
		{"co-DM player", CampaignContext{MemberRole: RolePlayer, IsMember: true, IsDmGranted: true}, HomeDirector},
		{"scribe", CampaignContext{MemberRole: RoleScribe, IsMember: true}, HomeScribe},
		{"player", CampaignContext{MemberRole: RolePlayer, IsMember: true}, HomePlayer},
		{"signed-in visitor", CampaignContext{MemberRole: RoleNone}, HomeGuest},
		{"anonymous visitor", CampaignContext{MemberRole: RoleNone, IsAnonymous: true}, HomeGuest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cc.HomeAudience(); got != tt.want {
				t.Errorf("HomeAudience() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestHomeLayout_Fallbacks(t *testing.T) {
	rows := func(n int) string {
		s := `{"rows":[`
		for i := 0; i < n; i++ {
			if i > 0 {
				s += ","
			}
			s += `{"id":"r","columns":[]}`
		}
		return s + `]}`
	}
	keyed := func(parts string) *string { s := "{" + parts + "}"; return &s }
	legacy := rows(1)
	builtIn := len(DefaultDashboardLayout().Rows)
	guestBuiltIn := len(DefaultGuestDashboardLayout().Rows)

	tests := []struct {
		name      string
		column    *string
		audience  string
		wantRows  int
		wantSaved bool
	}{
		{"nothing saved, director", nil, HomeDirector, builtIn, false},
		{"nothing saved, guest", nil, HomeGuest, guestBuiltIn, false},
		{"legacy, director borrows it", &legacy, HomeDirector, 1, false},
		{"legacy, guest gets the starter home", &legacy, HomeGuest, guestBuiltIn, false},
		{"legacy, default is saved", &legacy, HomeDefault, 1, true},
		{"default saved, director borrows it", keyed(`"default":` + rows(2)), HomeDirector, 2, false},
		{"default saved, guest does not", keyed(`"default":` + rows(2)), HomeGuest, guestBuiltIn, false},
		{"director saved", keyed(`"default":` + rows(2) + `,"director":` + rows(3)), HomeDirector, 3, true},
		{"guest saved", keyed(`"guest":` + rows(4)), HomeGuest, 4, true},
		{"only guest saved, player gets built-in", keyed(`"guest":` + rows(4)), HomePlayer, builtIn, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Campaign{DashboardLayout: tt.column}
			got, saved := c.HomeLayout(tt.audience)
			if len(got.Rows) != tt.wantRows || saved != tt.wantSaved {
				t.Errorf("HomeLayout(%q) = %d rows, saved %v; want %d rows, saved %v",
					tt.audience, len(got.Rows), saved, tt.wantRows, tt.wantSaved)
			}
		})
	}
}

func TestDefaultGuestDashboardLayout_NoMemberBlocks(t *testing.T) {
	for _, row := range DefaultGuestDashboardLayout().Rows {
		for _, col := range row.Columns {
			for _, b := range col.Blocks {
				if b.Type == BlockQuickActions {
					t.Errorf("guest starter home holds %q, which only members can use", b.Type)
				}
			}
		}
	}
}

func TestSetRoleDashboardJSON_DirectorAndGuestRoundTrip(t *testing.T) {
	c := &Campaign{}
	layout := &DashboardLayout{Rows: []DashboardRow{{ID: "r"}}}
	for _, audience := range []string{HomeDirector, HomeGuest} {
		full, err := c.SetRoleDashboardJSON(audience, layout)
		if err != nil || full == nil {
			t.Fatalf("SetRoleDashboardJSON(%q) = %v, %v", audience, full, err)
		}
		c.DashboardLayout = full
		if !c.HasSavedHome(audience) {
			t.Errorf("HasSavedHome(%q) = false after saving", audience)
		}
	}
	if c.HasSavedHome(HomeDefault) {
		t.Error("saving the Director and guest homes saved a Default one")
	}
	full, err := c.RemoveRoleDashboardJSON(HomeDirector)
	if err != nil {
		t.Fatal(err)
	}
	c.DashboardLayout = full
	if c.HasSavedHome(HomeDirector) || !c.HasSavedHome(HomeGuest) {
		t.Error("removing the Director's home touched the wrong one")
	}
}

func TestHomeAudienceParam(t *testing.T) {
	tests := []struct {
		query   string
		want    string
		wantErr bool
	}{
		{"", HomeDefault, false},
		{"?role=director", HomeDirector, false},
		{"?role=guest", HomeGuest, false},
		{"?role=scribe", HomeScribe, false},
		{"?role=owner", "", true},
	}
	e := echo.New()
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/x"+tt.query, nil)
			got, err := homeAudienceParam(e.NewContext(req, httptest.NewRecorder()))
			if (err != nil) != tt.wantErr || got != tt.want {
				t.Errorf("homeAudienceParam(%q) = %q, %v", tt.query, got, err)
			}
		})
	}
}

func TestCustomizeHomePages_TabsAndBorrowedHints(t *testing.T) {
	h := &Handler{}
	render := func(column *string, key string) string {
		cc := &CampaignContext{Campaign: &Campaign{ID: "c1", DashboardLayout: column}, MemberRole: RoleOwner, IsMember: true}
		var buf strings.Builder
		if err := customizeHomePages(cc, key, h.homeHint(context.Background(), cc, key), h.homeEditorState(context.Background(), cc, key)).Render(context.Background(), &buf); err != nil {
			t.Fatal(err)
		}
		return buf.String()
	}
	fresh := render(nil, HomeDirector)
	for _, want := range []string{"/customize/home/director", "/customize/home/guest", "/customize/home/dm", "GM team", "Guests", "Overview", "co-GMs", "same as the Everyone home until you change it", `data-widget="customize-page-editor"`, `&#34;scope&#34;:&#34;home&#34;`} {
		if !strings.Contains(fresh, want) {
			t.Errorf("fresh campaign: missing %q", want)
		}
	}
	if !strings.Contains(render(nil, HomeGuest), "they get a starter home") {
		t.Error("guest tab: should say a fresh campaign's guests get a starter home")
	}
	saved := `{"director":{"rows":[{"id":"r","columns":[]}]},"guest":{"rows":[{"id":"r","columns":[]}]}}`
	for _, key := range []string{HomeDirector, HomeGuest} {
		out := render(&saved, key)
		for _, gone := range []string{"until you change it", "they get a starter home"} {
			if strings.Contains(out, gone) {
				t.Errorf("saved %s home: still says %q", key, gone)
			}
		}
	}
}
