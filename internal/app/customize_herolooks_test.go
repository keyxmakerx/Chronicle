package app

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities/cardborders"
	"github.com/keyxmakerx/chronicle/internal/systems"
)

// recordingLooks stands in for the card look service and records the call.
type recordingLooks struct {
	calls    int
	campaign string
	user     string
	changes  []systems.CardLookChange
	err      error
}

func (r *recordingLooks) Apply(_ context.Context, campaignID, userID string, changes []systems.CardLookChange) error {
	r.calls++
	r.campaign, r.user, r.changes = campaignID, userID, changes
	return r.err
}

func ccWith(role campaigns.Role, dmGranted bool) *campaigns.CampaignContext {
	return &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1"}, MemberRole: role, IsDmGranted: dmGranted, IsMember: true}
}

func TestHeroLooksSection_OwnerOnlyAndShape(t *testing.T) {
	raw := `{"looks":[
		{"field":"ancestry","source":"package","key":"hakaan","border":"fire","colour":"#58c4d8"},
		{"field":"ancestry","source":"package","key":"devil","border":"","colour":""},
		{"field":"class","source":"package","key":"fury","reset":true},
		{"field":"ancestry","source":"campaign","key":"42","border":"vine","colour":""}],
		"_changes":[{"label":"x","href":"y"}],"_other":1,"campaignId":"camp-evil","campaign_id":"camp-evil","unknown":true}`
	want := []systems.CardLookChange{
		{FieldKey: "ancestry", Source: "package", Key: "hakaan", Border: "fire", Colour: "#58c4d8"},
		{FieldKey: "ancestry", Source: "package", Key: "devil"},
		{FieldKey: "class", Source: "package", Key: "fury", Reset: true},
		{FieldKey: "ancestry", Source: "campaign", Key: "42", Border: "vine"},
	}
	tests := []struct {
		name     string
		cc       *campaigns.CampaignContext
		raw      string
		wantCode int
		wantCall bool
	}{
		{"the owner applies", ccWith(campaigns.RoleOwner, true), raw, 0, true},
		{"a co-Director is refused", ccWith(campaigns.RoleScribe, true), raw, http.StatusForbidden, false},
		{"a player is refused", ccWith(campaigns.RolePlayer, false), raw, http.StatusForbidden, false},
		{"a non-member is refused", &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1"}, MemberRole: campaigns.RoleNone}, raw, http.StatusForbidden, false},
		{"an empty section applies nothing", ccWith(campaigns.RoleOwner, true), `{"looks":[],"_changes":[]}`, 0, false},
		{"a section that is not an object", ccWith(campaigns.RoleOwner, true), `[1,2]`, http.StatusBadRequest, false},
		{"a wrong-typed item", ccWith(campaigns.RoleOwner, true), `{"looks":[{"field":"ancestry","border":5}]}`, http.StatusBadRequest, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := &recordingLooks{}
			rem, err := newHeroLooksSection(rec).Apply(context.Background(), tt.cc, "owner-1", json.RawMessage(tt.raw))
			if rem != nil {
				t.Errorf("remaining = %s, want nil", rem)
			}
			var ae *apperror.AppError
			got := 0
			if err != nil {
				if !asAppError(err, &ae) {
					t.Fatalf("err = %v", err)
				}
				got = ae.Code
			}
			if got != tt.wantCode {
				t.Fatalf("code = %d (%v), want %d", got, err, tt.wantCode)
			}
			if (rec.calls == 1) != tt.wantCall {
				t.Fatalf("service calls = %d, want call %v", rec.calls, tt.wantCall)
			}
			if tt.wantCall {
				if rec.campaign != "camp-1" || rec.user != "owner-1" {
					t.Errorf("campaign %q user %q: the campaign comes from the request, never the body", rec.campaign, rec.user)
				}
				if !reflect.DeepEqual(rec.changes, want) {
					t.Errorf("changes = %+v", rec.changes)
				}
			}
		})
	}
}

func asAppError(err error, target **apperror.AppError) bool {
	ae, ok := err.(*apperror.AppError)
	if ok {
		*target = ae
	}
	return ok
}

// The section in front of the real service refuses hostile drafts and writes
// nothing; the service's own table covers each rule, this proves the wiring.
type memLookRepo struct{ writes int }

func (m *memLookRepo) List(context.Context, string, string, string) (map[string]systems.StoredLook, error) {
	return map[string]systems.StoredLook{}, nil
}
func (m *memLookRepo) Apply(_ context.Context, u, d []systems.CardLookRow) error {
	m.writes += len(u) + len(d)
	return nil
}
func (m *memLookRepo) DeleteDerived(context.Context, string, string, string) (int, error) {
	return 0, nil
}

type listChoices struct {
	systems.CharacterChoiceService
}

func (listChoices) CharacterChoiceList(_ context.Context, _, field string) (*systems.ChoiceList, error) {
	return &systems.ChoiceList{FieldKey: field, SystemID: "sys", Choices: []systems.Choice{
		{Key: "devil", Slug: "devil", Name: "Devil", Source: systems.ChoiceSourcePackage},
	}}, nil
}

func TestHeroLooksSection_HostileDraftWritesNothing(t *testing.T) {
	item := func(border, colour string) string {
		return `{"looks":[{"field":"ancestry","source":"package","key":"devil","border":"` + border + `","colour":"` + colour + `"}]}`
	}
	var many strings.Builder
	many.WriteString(`{"looks":[`)
	for i := 0; i <= systems.MaxCardLookChanges; i++ {
		if i > 0 {
			many.WriteString(",")
		}
		many.WriteString(`{"field":"ancestry","source":"package","key":"k` + strings.Repeat("a", i%7) + string(rune('a'+i%26)) + string(rune('a'+(i/26)%26)) + string(rune('a'+(i/676)%26)) + `","border":"fire"}`)
	}
	many.WriteString(`]}`)
	tests := map[string]string{
		"css colour":    item("fire", `red;background:url(x)`),
		"bad hex":       item("fire", `#12345g`),
		"unknown":       item("nope", ``),
		"bad field":     `{"looks":[{"field":"a b","source":"package","key":"devil","border":"fire"}]}`,
		"duplicate":     `{"looks":[{"field":"ancestry","source":"package","key":"devil","border":"fire"},{"field":"ancestry","source":"package","key":"devil","border":"ice"}]}`,
		"501 items":     many.String(),
		"colour no bdr": item("", "#aabbcc"),
	}
	for name, raw := range tests {
		t.Run(name, func(t *testing.T) {
			repo := &memLookRepo{}
			svc := systems.NewCardLookService(repo, listChoices{})
			_, err := newHeroLooksSection(svc).Apply(context.Background(), ccWith(campaigns.RoleOwner, true), "owner-1", json.RawMessage(raw))
			var ae *apperror.AppError
			if !asAppError(err, &ae) || ae.Code != http.StatusUnprocessableEntity {
				t.Fatalf("err = %v, want a validation error", err)
			}
			if repo.writes != 0 {
				t.Errorf("a refused draft wrote %d rows", repo.writes)
			}
		})
	}
	// An upper-case colour is accepted; the stale item beside it is dropped.
	repo := &memLookRepo{}
	svc := systems.NewCardLookService(repo, listChoices{})
	raw := `{"looks":[{"field":"ancestry","source":"package","key":"devil","border":"fire","colour":"#ABCDEF"},{"field":"ancestry","source":"package","key":"gone","border":"fire"}]}`
	if _, err := newHeroLooksSection(svc).Apply(context.Background(), ccWith(campaigns.RoleOwner, true), "owner-1", json.RawMessage(raw)); err != nil || repo.writes != 1 {
		t.Fatalf("err = %v, writes = %d, want 1 row", err, repo.writes)
	}
}

// A Director's plan says what the game system gives a card and that the owner
// changed it; a player's plan carries neither.
func TestHeroChoices_SystemLookOnlyForDirectors(t *testing.T) {
	in := []systems.Choice{{
		Key: "hakaan", Slug: "hakaan", Name: "Hakaan", Source: systems.ChoiceSourcePackage,
		Properties:  map[string]any{"card_border": "fire", "card_colour": "#112233"},
		SystemLook:  &cardLookIce,
		LookChanged: true,
	}, {
		Key: "devil", Slug: "devil", Name: "Devil", Source: systems.ChoiceSourcePackage,
		Properties: map[string]any{}, LookChanged: true,
	}, {
		Key: "12", Slug: "x", Name: "X", Source: systems.ChoiceSourceCampaign, Status: "sent",
	}}
	dir := heroChoices(systems.WithChoiceViewer(context.Background(), true), in)
	if !dir[0].LookChanged || dir[0].SystemLook == nil || dir[0].SystemLook.Border != "ice" || dir[0].SystemLook.Colour != "#aabbcc" {
		t.Errorf("director's hakaan: %+v", dir[0])
	}
	if !dir[1].LookChanged || dir[1].SystemLook != nil {
		t.Errorf("a card the system gives no border: %+v", dir[1])
	}
	if dir[0].Look == nil || dir[0].Look.Border != "fire" {
		t.Errorf("the look served is the owner's: %+v", dir[0].Look)
	}
	for name, ctx := range map[string]context.Context{"player": systems.WithChoiceViewer(context.Background(), false), "bare": context.Background()} {
		raw, _ := json.Marshal(heroChoices(ctx, in))
		if strings.Contains(string(raw), "systemLook") || strings.Contains(string(raw), "lookChanged") {
			t.Errorf("%s's plan leaks the owner's setup: %s", name, raw)
		}
	}
	raw, _ := json.Marshal(dir)
	for _, want := range []string{`"systemLook":{"border":"ice","colour":"#aabbcc"}`, `"lookChanged":true`, `"status":"sent"`, `"source":"campaign"`, `"key":"12"`} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("plan JSON lacks %s: %s", want, raw)
		}
	}
}

// The AI's lookup reads the same list the creator does, so the owner's choice
// reaches it with no adapter work. This uses the real choice service over a
// real custom system, with only the looks table stood in.
type fixedLooks map[string]systems.StoredLook

func (f fixedLooks) CardLooks(context.Context, string, string, string) (map[string]systems.StoredLook, error) {
	return f, nil
}

func TestAISystemEntries_ReportsOwnersCardLook(t *testing.T) {
	manifest := `{"id":"looks-test","name":"Looks Test","api_version":"1","status":"available","categories":[{"slug":"ancestries","name":"Ancestries"}],
		"entity_presets":[{"slug":"hero","name":"Hero","category":"character","fields":[{"key":"ancestry","label":"Ancestry","type":"string"}]}]}`
	data := `[{"slug":"hakaan","name":"Hakaan","properties":{"card_border":"ice","card_colour":"#112233"}},{"slug":"human","name":"Human"}]`
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range map[string]string{"manifest.json": manifest, "data/ancestries.json": data} {
		w, _ := zw.Create(name)
		_, _ = w.Write([]byte(body))
	}
	_ = zw.Close()
	mgr := systems.NewCampaignSystemManager(t.TempDir())
	if _, err := mgr.Install("camp", bytes.NewReader(buf.Bytes()), int64(buf.Len())); err != nil {
		t.Fatal(err)
	}
	svc := systems.NewCharacterChoiceService(nil, mgr)
	ai := aiSystemEntriesAdapter{choices: svc}

	read := func() map[string][2]string {
		_, list, err := ai.Entries(context.Background(), "camp", "ancestry", false)
		if err != nil {
			t.Fatal(err)
		}
		out := map[string][2]string{}
		for _, e := range list {
			out[e.Slug] = [2]string{e.Border, e.Colour}
		}
		return out
	}
	if got := read()["hakaan"]; got != [2]string{"ice", "#112233"} {
		t.Fatalf("before any owner choice: %v", got)
	}
	svc.SetChoiceLooks(fixedLooks{"hakaan": {Border: "fire", Colour: "#ABCDEF"}, "human": {Border: "vine"}})
	got := read()
	if got["hakaan"] != [2]string{"fire", "#abcdef"} {
		t.Errorf("hakaan = %v, want the owner's fire in #abcdef", got["hakaan"])
	}
	if got["human"] != [2]string{"vine", "#6fae4f"} {
		t.Errorf("human = %v, want vine in its own colour", got["human"])
	}
	svc.SetChoiceLooks(fixedLooks{"hakaan": {}})
	if got := read()["hakaan"]; got != [2]string{"", ""} {
		t.Errorf("no border = %v, want none", got)
	}
}

var cardLookIce = cardborders.Look{Border: "ice", Colour: "#aabbcc"}
