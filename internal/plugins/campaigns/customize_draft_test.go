package campaigns

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// memDraftRepo is an in-memory CustomizeDraftRepository with the same
// version rules as the SQL one.
type memDraftRepo struct {
	mu   sync.Mutex
	rows map[string]*CustomizeDraft
}

func newMemDraftRepo() *memDraftRepo { return &memDraftRepo{rows: map[string]*CustomizeDraft{}} }

func (r *memDraftRepo) Get(_ context.Context, campaignID, userID string) (*CustomizeDraft, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if d, ok := r.rows[campaignID+"|"+userID]; ok {
		c := *d
		return &c, nil
	}
	return nil, nil
}

func (r *memDraftRepo) Insert(_ context.Context, campaignID, userID string, draft []byte) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	k := campaignID + "|" + userID
	if _, ok := r.rows[k]; ok {
		return false, nil
	}
	r.rows[k] = &CustomizeDraft{Version: 1, Draft: append([]byte(nil), draft...), UpdatedAt: time.Now()}
	return true, nil
}

func (r *memDraftRepo) Update(_ context.Context, campaignID, userID string, version int, draft []byte) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	d, ok := r.rows[campaignID+"|"+userID]
	if !ok || d.Version != version {
		return false, nil
	}
	d.Version++
	d.Draft = append([]byte(nil), draft...)
	return true, nil
}

func (r *memDraftRepo) Delete(_ context.Context, campaignID, userID string, version int) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	k := campaignID + "|" + userID
	d, ok := r.rows[k]
	if !ok || (version > 0 && d.Version != version) {
		return false, nil
	}
	delete(r.rows, k)
	return true, nil
}

func TestCustomizeDraftService_Save(t *testing.T) {
	allowed := []string{"look", "categories"}
	big := `{"look":"` + strings.Repeat("x", maxCustomizeDraftBytes) + `"}`
	tests := []struct {
		name     string
		seed     string // draft already stored at version 1
		version  int
		body     string
		wantCode int // 0 = success
		wantVer  int
		wantRow  bool
	}{
		{"first save inserts", "", 0, `{"look":{"a":1}}`, 0, 1, true},
		{"save over version bumps it", `{"look":{}}`, 1, `{"look":{"a":2}}`, 0, 2, true},
		{"insert when a draft exists conflicts", `{"look":{}}`, 0, `{"look":{"a":1}}`, http.StatusConflict, 0, true},
		{"stale version conflicts", `{"look":{}}`, 5, `{"look":{"a":1}}`, http.StatusConflict, 0, true},
		{"version for a missing draft conflicts", "", 3, `{"look":{"a":1}}`, http.StatusConflict, 0, false},
		{"unknown section refused", "", 0, `{"nope":{}}`, http.StatusBadRequest, 0, false},
		{"array refused", "", 0, `[]`, http.StatusBadRequest, 0, false},
		{"string refused", "", 0, `"look"`, http.StatusBadRequest, 0, false},
		{"empty body refused", "", 0, ``, http.StatusBadRequest, 0, false},
		{"malformed refused", "", 0, `{"look":`, http.StatusBadRequest, 0, false},
		{"over one MiB refused", "", 0, big, http.StatusBadRequest, 0, false},
		{"negative version refused", "", -1, `{"look":{}}`, http.StatusBadRequest, 0, false},
		{"empty object deletes the row", `{"look":{}}`, 1, `{}`, 0, 0, false},
		{"empty object with a stale version conflicts and keeps the row", `{"look":{}}`, 4, `{}`, http.StatusConflict, 0, true},
		{"empty object with no draft is a no-op", "", 0, `{}`, 0, 0, false},
		{"empty object over an unseen draft conflicts", `{"look":{}}`, 0, `{}`, http.StatusConflict, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newMemDraftRepo()
			if tt.seed != "" {
				if _, err := repo.Insert(context.Background(), "c1", "u1", []byte(tt.seed)); err != nil {
					t.Fatal(err)
				}
			}
			svc := NewCustomizeDraftService(repo)
			got, err := svc.Save(context.Background(), "c1", "u1", tt.version, json.RawMessage(tt.body), allowed)
			if tt.wantCode != 0 {
				if apperror.SafeCode(err) != tt.wantCode {
					t.Fatalf("err = %v, want code %d", err, tt.wantCode)
				}
			} else if err != nil {
				t.Fatalf("unexpected error: %v", err)
			} else if got != tt.wantVer {
				t.Errorf("version = %d, want %d", got, tt.wantVer)
			}
			row, _ := repo.Get(context.Background(), "c1", "u1")
			if (row != nil) != tt.wantRow {
				t.Errorf("row present = %v, want %v", row != nil, tt.wantRow)
			}
		})
	}
}

func TestCustomizeDraftService_Discard(t *testing.T) {
	tests := []struct {
		name     string
		seed     bool
		version  int
		wantCode int
		wantRow  bool
	}{
		{"matching version removes", true, 1, 0, false},
		{"version 0 removes unconditionally", true, 0, 0, false},
		{"stale version conflicts", true, 9, http.StatusConflict, true},
		{"already gone is fine", false, 1, 0, false},
		{"negative refused", true, -1, http.StatusBadRequest, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newMemDraftRepo()
			if tt.seed {
				_, _ = repo.Insert(context.Background(), "c1", "u1", []byte(`{"look":{}}`))
			}
			err := NewCustomizeDraftService(repo).Discard(context.Background(), "c1", "u1", tt.version)
			if tt.wantCode != 0 {
				if apperror.SafeCode(err) != tt.wantCode {
					t.Fatalf("err = %v, want code %d", err, tt.wantCode)
				}
			} else if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if row, _ := repo.Get(context.Background(), "c1", "u1"); (row != nil) != tt.wantRow {
				t.Errorf("row present = %v, want %v", row != nil, tt.wantRow)
			}
		})
	}
}

func TestCustomizeDraftService_PerOwnerAndCampaign(t *testing.T) {
	repo := newMemDraftRepo()
	svc := NewCustomizeDraftService(repo)
	ctx := context.Background()
	keys := []string{"look"}
	if _, err := svc.Save(ctx, "c1", "u1", 0, json.RawMessage(`{"look":{"who":"u1c1"}}`), keys); err != nil {
		t.Fatal(err)
	}
	// Another owner of the same campaign, and the same owner in another
	// campaign, each start with no draft.
	for _, other := range [][2]string{{"c1", "u2"}, {"c2", "u1"}} {
		if d, err := svc.Get(ctx, other[0], other[1]); err != nil || d != nil {
			t.Errorf("%v sees a draft: %v, %v", other, d, err)
		}
	}
}

func TestRegisterCustomizeSection(t *testing.T) {
	noop := func(context.Context, *CampaignContext, string, json.RawMessage) (json.RawMessage, error) {
		return nil, nil
	}
	tests := []struct {
		name string
		key  string
		fn   bool
		want bool
	}{
		{"plain key", "categories", true, true},
		{"dash and underscore", "my_sec-2", true, true},
		{"empty", "", true, false},
		{"leading digit", "1abc", true, false},
		{"uppercase", "Look", true, false},
		{"space", "a b", true, false},
		{"too long", strings.Repeat("a", 33), true, false},
		{"longest", strings.Repeat("a", 32), true, true},
		{"no apply", "ok", false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &Handler{}
			s := CustomizeSection{Key: tt.key}
			if tt.fn {
				s.Apply = noop
			}
			h.RegisterCustomizeSection(s)
			if got := len(h.CustomizeSectionKeys()) == 1; got != tt.want {
				t.Errorf("registered = %v, want %v", got, tt.want)
			}
		})
	}

	t.Run("duplicate ignored and order kept", func(t *testing.T) {
		h := &Handler{}
		for _, k := range []string{"b", "a", "b"} {
			h.RegisterCustomizeSection(CustomizeSection{Key: k, Apply: noop})
		}
		if got := strings.Join(h.CustomizeSectionKeys(), ","); got != "b,a" {
			t.Errorf("keys = %q, want b,a", got)
		}
	})

	t.Run("NewHandler registers look first", func(t *testing.T) {
		h := NewHandler(nil)
		h.RegisterCustomizeSection(CustomizeSection{Key: "categories", Apply: noop})
		if got := strings.Join(h.CustomizeSectionKeys(), ","); got != "look,categories" {
			t.Errorf("keys = %q", got)
		}
	})
}
