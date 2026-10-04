package systemstate

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// fakeRepo applies the same per-half keep/replace rule as the SQL upsert so the
// service's partial-update behaviour can be observed end to end.
type fakeRepo struct {
	rows    map[string]*State
	upserts int
}

func newFakeRepo() *fakeRepo { return &fakeRepo{rows: map[string]*State{}} }

func rowKey(campaignID, entityID, systemID, key string) string {
	return campaignID + "/" + entityID + "/" + systemID + "/" + key
}

func (f *fakeRepo) Get(_ context.Context, campaignID, entityID, systemID, key string) (*State, error) {
	if r, ok := f.rows[rowKey(campaignID, entityID, systemID, key)]; ok {
		cp := *r
		return &cp, nil
	}
	return nil, nil
}

func (f *fakeRepo) Upsert(_ context.Context, w Write) error {
	f.upserts++
	k := rowKey(w.CampaignID, w.EntityID, w.SystemID, w.Key)
	row, ok := f.rows[k]
	if !ok {
		now := time.Now()
		row = &State{GM: json.RawMessage(`{}`), Public: json.RawMessage(`{}`), UpdatedAt: &now}
		f.rows[k] = row
	}
	if w.GM != nil {
		row.GM = w.GM
	}
	if w.Public != nil {
		row.Public = w.Public
	}
	return nil
}

type fakeEntities map[string]string // entity id -> campaign id

func (f fakeEntities) EntityCampaignID(_ context.Context, id string) (string, error) {
	c, ok := f[id]
	if !ok {
		return "", apperror.NewNotFound("entity not found")
	}
	return c, nil
}

type fakeSystems map[string]bool // "campaign/system" -> enabled

func (f fakeSystems) IsSystemEnabled(_ context.Context, campaignID, systemID string) (bool, error) {
	return f[campaignID+"/"+systemID], nil
}

type fakePublisher struct{ sent []string }

func (f *fakePublisher) PublishSystemStateUpdated(campaignID, entityID, systemID, key string) {
	f.sent = append(f.sent, campaignID+"/"+entityID+"/"+systemID+"/"+key)
}

func newTestService() (Service, *fakeRepo, *fakePublisher) {
	repo := newFakeRepo()
	pub := &fakePublisher{}
	svc := NewService(repo,
		fakeEntities{"npc-1": "camp-1", "other-npc": "camp-2"},
		fakeSystems{"camp-1/drawsteel": true},
		pub)
	return svc, repo, pub
}

func raw(s string) *json.RawMessage { r := json.RawMessage(s); return &r }

func errCode(err error) int {
	var ae *apperror.AppError
	if errors.As(err, &ae) {
		return ae.Code
	}
	return 0
}

func TestService_PutValidation(t *testing.T) {
	big := `{"k":"` + strings.Repeat("x", MaxHalfBytes) + `"}`
	tests := []struct {
		name     string
		campaign string
		entity   string
		system   string
		key      string
		gm       *json.RawMessage
		public   *json.RawMessage
		wantCode int
	}{
		{"ok both", "camp-1", "npc-1", "drawsteel", "negotiation", raw(`{"a":1}`), raw(`{"b":2}`), 0},
		{"ok one half", "camp-1", "npc-1", "drawsteel", "negotiation", nil, raw(`{"b":2}`), 0},
		{"key uppercase", "camp-1", "npc-1", "drawsteel", "Negotiation", raw(`{}`), nil, http.StatusUnprocessableEntity},
		{"key leading dash", "camp-1", "npc-1", "drawsteel", "-x", raw(`{}`), nil, http.StatusUnprocessableEntity},
		{"key with slash", "camp-1", "npc-1", "drawsteel", "a/b", raw(`{}`), nil, http.StatusUnprocessableEntity},
		{"key too long", "camp-1", "npc-1", "drawsteel", strings.Repeat("a", 65), raw(`{}`), nil, http.StatusUnprocessableEntity},
		{"empty key", "camp-1", "npc-1", "drawsteel", "", raw(`{}`), nil, http.StatusUnprocessableEntity},
		{"bad system id", "camp-1", "npc-1", "Draw Steel", "k", raw(`{}`), nil, http.StatusUnprocessableEntity},
		{"gm is array", "camp-1", "npc-1", "drawsteel", "k", raw(`[1]`), nil, http.StatusUnprocessableEntity},
		{"public is scalar", "camp-1", "npc-1", "drawsteel", "k", nil, raw(`3`), http.StatusUnprocessableEntity},
		{"public is string", "camp-1", "npc-1", "drawsteel", "k", nil, raw(`"x"`), http.StatusUnprocessableEntity},
		{"gm is null", "camp-1", "npc-1", "drawsteel", "k", raw(`null`), nil, http.StatusUnprocessableEntity},
		{"truncated json", "camp-1", "npc-1", "drawsteel", "k", raw(`{"a":`), nil, http.StatusUnprocessableEntity},
		{"half too big", "camp-1", "npc-1", "drawsteel", "k", raw(big), nil, http.StatusUnprocessableEntity},
		{"entity from another campaign", "camp-1", "other-npc", "drawsteel", "k", raw(`{}`), nil, http.StatusNotFound},
		{"unknown entity", "camp-1", "ghost", "drawsteel", "k", raw(`{}`), nil, http.StatusNotFound},
		{"system not enabled", "camp-1", "npc-1", "dnd5e", "k", raw(`{}`), nil, http.StatusUnprocessableEntity},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, repo, pub := newTestService()
			_, err := svc.Put(context.Background(), tt.campaign, tt.entity, tt.system, tt.key, tt.gm, tt.public, "user-1")
			if got := errCode(err); got != tt.wantCode {
				t.Fatalf("code = %d (err %v), want %d", got, err, tt.wantCode)
			}
			if tt.wantCode != 0 {
				if repo.upserts != 0 {
					t.Error("a rejected write must not reach the repository")
				}
				if len(pub.sent) != 0 {
					t.Error("a rejected write must not notify")
				}
			}
		})
	}
}

func TestService_PutIsPartial(t *testing.T) {
	svc, _, pub := newTestService()
	ctx := context.Background()
	put := func(gm, public *json.RawMessage) State {
		t.Helper()
		st, err := svc.Put(ctx, "camp-1", "npc-1", "drawsteel", "negotiation", gm, public, "user-1")
		if err != nil {
			t.Fatal(err)
		}
		return st
	}

	st := put(raw(`{"secret": 1}`), raw(`{"shown": 1}`))
	if string(st.GM) != `{"secret":1}` || string(st.Public) != `{"shown":1}` {
		t.Fatalf("initial = gm %s public %s", st.GM, st.Public)
	}
	// Public only: the gm half is preserved.
	st = put(nil, raw(`{"shown": 2}`))
	if string(st.GM) != `{"secret":1}` || string(st.Public) != `{"shown":2}` {
		t.Fatalf("public-only put changed gm: gm %s public %s", st.GM, st.Public)
	}
	// Gm only: the public half is preserved.
	st = put(raw(`{"secret": 2}`), nil)
	if string(st.GM) != `{"secret":2}` || string(st.Public) != `{"shown":2}` {
		t.Fatalf("gm-only put changed public: gm %s public %s", st.GM, st.Public)
	}
	// An empty object is a present half and clears it.
	st = put(raw(`{}`), nil)
	if string(st.GM) != `{}` || string(st.Public) != `{"shown":2}` {
		t.Fatalf("clearing gm: gm %s public %s", st.GM, st.Public)
	}
	if len(pub.sent) != 4 || pub.sent[0] != "camp-1/npc-1/drawsteel/negotiation" {
		t.Errorf("notifications = %v", pub.sent)
	}
}

func TestService_GetEmptyAndScoped(t *testing.T) {
	svc, _, _ := newTestService()
	ctx := context.Background()

	st, err := svc.Get(ctx, "camp-1", "npc-1", "drawsteel", "negotiation")
	if err != nil {
		t.Fatal(err)
	}
	if string(st.GM) != `{}` || string(st.Public) != `{}` || st.UpdatedAt != nil {
		t.Errorf("empty read = %+v", st)
	}

	if _, err := svc.Put(ctx, "camp-1", "npc-1", "drawsteel", "negotiation", raw(`{"a":1}`), nil, "u"); err != nil {
		t.Fatal(err)
	}
	// The same entity id read through another campaign is a 404, not data.
	if _, err := svc.Get(ctx, "camp-2", "npc-1", "drawsteel", "negotiation"); errCode(err) != http.StatusNotFound {
		t.Errorf("cross-campaign get code = %d, want 404 (err %v)", errCode(err), err)
	}
	if _, err := svc.Get(ctx, "camp-1", "npc-1", "BAD", "negotiation"); errCode(err) != http.StatusUnprocessableEntity {
		t.Errorf("bad system id get code = %d", errCode(err))
	}
}
