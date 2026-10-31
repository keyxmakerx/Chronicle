package entities

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// fakeTrashKind is one kind of thing in the Trash, with what it was asked.
type fakeTrashKind struct {
	kind     string
	items    []TrashedThing
	listErr  error
	link     string
	restored []string
	cutoffs  []time.Time
}

func (f *fakeTrashKind) TrashKind() string { return f.kind }
func (f *fakeTrashKind) ListTrashed(context.Context, string) ([]TrashedThing, error) {
	return f.items, f.listErr
}
func (f *fakeTrashKind) RestoreTrashed(_ context.Context, _, id string) (string, error) {
	f.restored = append(f.restored, id)
	return f.link, nil
}
func (f *fakeTrashKind) PurgeTrashedBefore(_ context.Context, cutoff time.Time) (int, error) {
	f.cutoffs = append(f.cutoffs, cutoff)
	return 0, nil
}

func TestTrashKinds_ListRestorePurge(t *testing.T) {
	old, mid, recent := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC), time.Date(2026, 10, 9, 0, 0, 0, 0, time.UTC)
	nights := &fakeTrashKind{kind: "game-night", link: "/campaigns/c1/sessions/n1", items: []TrashedThing{
		{Kind: "game-night", ID: "n1", DeletedAt: old},
		{Kind: "game-night", ID: "n2", DeletedAt: recent},
	}}
	maps := &fakeTrashKind{kind: "map", items: []TrashedThing{{Kind: "map", ID: "m1", DeletedAt: mid}}}
	broken := &fakeTrashKind{kind: "picture", listErr: errors.New("db down")}
	p := &pageSafetyService{}
	p.AddTrashKind(nights)
	p.AddTrashKind(broken)
	p.AddTrashKind(maps)

	var ids []string
	for _, it := range p.ListOtherTrash(context.Background(), "c1") {
		ids = append(ids, it.ID)
	}
	if strings.Join(ids, ",") != "n2,m1,n1" {
		t.Errorf("listed %v, want newest first across kinds, without the broken kind", ids)
	}

	link, err := p.RestoreOther(context.Background(), "c1", "game-night", "n1")
	if err != nil || link != "/campaigns/c1/sessions/n1" || strings.Join(nights.restored, ",") != "n1" {
		t.Errorf("restore: link=%q err=%v restored=%v", link, err, nights.restored)
	}
	if _, err := p.RestoreOther(context.Background(), "c1", "spaceship", "x"); err == nil {
		t.Error("an unknown kind must be not found")
	}

	cutoff := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	p.purgeKinds(context.Background(), cutoff)
	for _, k := range []*fakeTrashKind{nights, broken, maps} {
		if len(k.cutoffs) != 1 || !k.cutoffs[0].Equal(cutoff) {
			t.Errorf("%s purged with %v, want once with the pages' cutoff", k.kind, k.cutoffs)
		}
	}
}

// trashLinkSvc answers RestoreOther with a fixed link.
type trashLinkSvc struct {
	PageSafetyService
	link string
}

func (s trashLinkSvc) RestoreOther(context.Context, string, string, string) (string, error) {
	return s.link, nil
}

func TestRestoreOtherFromTrash_StaysInTheCampaign(t *testing.T) {
	tests := []struct {
		name, link, want string
	}{
		{"a link in the campaign is used", "/campaigns/c1/sessions/n1", "/campaigns/c1/sessions/n1"},
		{"another campaign's link falls back to the Trash", "/campaigns/c2/sessions/n1", "/campaigns/c1/trash"},
		{"an outside link falls back to the Trash", "https://example.com/", "/campaigns/c1/trash"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := NewPageSafetyHandler(trashLinkSvc{link: tc.link}, nil, nil)
			e := echo.New()
			rec := httptest.NewRecorder()
			c := e.NewContext(httptest.NewRequest(http.MethodPost, "/", nil), rec)
			c.SetParamNames("id", "kind", "tid")
			c.SetParamValues("c1", "game-night", "n1")
			c.Set("campaign_context", &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c1"}, MemberRole: campaigns.RoleOwner})
			if err := h.RestoreOtherFromTrash(c); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(rec.Body.String(), `"url":"`+tc.want+`"`) {
				t.Errorf("body = %s, want url %s", rec.Body.String(), tc.want)
			}
		})
	}
}

func TestTrashPage_ListsOtherKinds(t *testing.T) {
	now := time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC)
	cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c1", Name: "Therin"}, MemberRole: campaigns.RoleOwner}
	d := TrashPageData{
		CampaignID: "c1", Retention: 30, Now: now,
		Items:  []TrashItem{{ID: "p1", Name: "Old page", TypeName: "NPC", DeletedAt: now.AddDate(0, 0, -5)}},
		Others: []TrashedThing{{Kind: "game-night", Label: "Game night", Icon: "fa-dice-d20", ID: "n1", Name: "Session 15", DeletedAt: now.AddDate(0, 0, -1)}},
	}
	var buf bytes.Buffer
	if err := TrashPage(cc, d).Render(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	html := buf.String()
	night, page := strings.Index(html, "Session 15"), strings.Index(html, "Old page")
	if night < 0 || page < 0 || night > page {
		t.Errorf("want the newer game night listed before the older page (night at %d, page at %d)", night, page)
	}
	for _, want := range []string{"(Game night)", `hx-post="/campaigns/c1/trash/game-night/n1/restore"`, `hx-post="/campaigns/c1/trash/p1/restore"`, "29 days left"} {
		if !strings.Contains(html, want) {
			t.Errorf("Trash page is missing %q", want)
		}
	}
}
