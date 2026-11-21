// view_prefs_test.go pins the My view choices: validation against the
// allowed lists, partial-update semantics (absent preserves, null/"" resets,
// a value replaces), tolerant reads of the stored column, and that the
// handler only ever writes the signed-in person's own row.

package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"
)

func decodeViewPrefsInput(t *testing.T, body string) UpdateViewPrefsInput {
	t.Helper()
	var in UpdateViewPrefsInput
	if err := json.Unmarshal([]byte(body), &in); err != nil {
		t.Fatalf("decode %s: %v", body, err)
	}
	return in
}

func TestUpdateViewPrefsInput_ApplyTo(t *testing.T) {
	stored := ViewPrefs{Theme: "dark", Motion: "calm", TextSize: "larger", Contrast: "high", BrushHidden: []string{}}
	tests := []struct {
		name    string
		body    string
		cur     ViewPrefs
		want    ViewPrefs
		wantErr bool
	}{
		{"absent keys preserve everything", `{}`, stored, stored, false},
		{"a theme-only body keeps the other three", `{"theme":"light"}`, stored, ViewPrefs{"light", "calm", "larger", "high", []string{}}, false},
		{"each value replaces", `{"theme":"device","motion":"owner","textSize":"largest","contrast":"standard"}`, stored, ViewPrefs{"device", "owner", "largest", "standard", []string{}}, false},
		{"explicit null resets just that choice", `{"textSize":null}`, stored, ViewPrefs{"dark", "calm", "standard", "high", []string{}}, false},
		{"empty string means the default", `{"motion":""}`, stored, ViewPrefs{"dark", "owner", "larger", "high", []string{}}, false},
		{"unknown theme refused", `{"theme":"sepia"}`, stored, stored, true},
		{"off is a motion choice", `{"motion":"off"}`, stored, ViewPrefs{"dark", "off", "larger", "high", []string{}}, false},
		{"unknown motion refused", `{"motion":"none"}`, stored, stored, true},
		{"unknown size refused", `{"textSize":"huge"}`, stored, stored, true},
		{"unknown contrast refused", `{"contrast":"max"}`, stored, stored, true},
		{"case matters", `{"theme":"Dark"}`, stored, stored, true},
		{"one bad key refuses the whole body", `{"theme":"light","contrast":"x"}`, stored, stored, true},
		{"defaults from nothing", `{"theme":"dark"}`, DefaultViewPrefs(), ViewPrefs{"dark", "owner", "standard", "standard", []string{}}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := decodeViewPrefsInput(t, tc.body).ApplyTo(tc.cur)
			if tc.wantErr {
				assertAppError(t, err, http.StatusBadRequest)
			} else if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestParseViewPrefs(t *testing.T) {
	def := DefaultViewPrefs()
	tests := []struct {
		name string
		raw  string
		want ViewPrefs
	}{
		{"NULL column", ``, def},
		{"malformed JSON", `{nope`, def},
		{"not an object", `["dark"]`, def},
		{"full row", `{"theme":"light","motion":"calm","textSize":"largest","contrast":"high"}`, ViewPrefs{"light", "calm", "largest", "high", []string{}}},
		{"partial row fills defaults", `{"theme":"dark"}`, ViewPrefs{"dark", "owner", "standard", "standard", []string{}}},
		{"hidden in two campaigns", `{"brushHidden":["c-1","c-2"]}`, ViewPrefs{"device", "owner", "standard", "standard", []string{"c-1", "c-2"}}},
		{"an account-wide hidden from before is not carried over", `{"theme":"dark","brush":"hidden"}`, ViewPrefs{"dark", "owner", "standard", "standard", []string{}}},
		{"duplicates and malformed ids are dropped", `{"brushHidden":["c-1","c-1","<x>","",7]}`, ViewPrefs{"device", "owner", "standard", "standard", []string{"c-1"}}},
		{"a brush list of the wrong type is ignored", `{"brushHidden":"c-1"}`, def},
		{"invalid value falls back for that choice only", `{"theme":"neon","contrast":"high"}`, ViewPrefs{"device", "owner", "standard", "high", []string{}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := ParseViewPrefs([]byte(tc.raw)); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestService_UpdateViewPrefs_MergesAndPersists(t *testing.T) {
	repo := &mockUserRepo{}
	svc := newTestAuthService(repo)
	ctx := context.Background()

	if _, err := svc.UpdateViewPrefs(ctx, "u1", decodeViewPrefsInput(t, `{"theme":"dark"}`)); err != nil {
		t.Fatalf("first update: %v", err)
	}
	got, err := svc.UpdateViewPrefs(ctx, "u1", decodeViewPrefsInput(t, `{"contrast":"high"}`))
	if err != nil {
		t.Fatalf("second update: %v", err)
	}
	if want := (ViewPrefs{"dark", "owner", "standard", "high", []string{}}); !reflect.DeepEqual(got, want) {
		t.Errorf("second update returned %+v, want %+v (the theme must survive a contrast-only save)", got, want)
	}
	if read, _ := svc.GetViewPrefs(ctx, "u1"); !reflect.DeepEqual(read, got) {
		t.Errorf("GetViewPrefs = %+v, want what was saved %+v", read, got)
	}

	before := string(repo.viewPrefs)
	if _, err := svc.UpdateViewPrefs(ctx, "u1", decodeViewPrefsInput(t, `{"theme":"bogus"}`)); err == nil {
		t.Fatal("invalid value must be refused")
	}
	if string(repo.viewPrefs) != before {
		t.Errorf("a refused update wrote to the store: %s -> %s", before, repo.viewPrefs)
	}
}

// stubViewPrefsService records which user each update targets.
type stubViewPrefsService struct {
	AuthService
	updates map[string]UpdateViewPrefsInput
}

func (s *stubViewPrefsService) UpdateViewPrefs(_ context.Context, userID string, in UpdateViewPrefsInput) (ViewPrefs, error) {
	if s.updates == nil {
		s.updates = map[string]UpdateViewPrefsInput{}
	}
	s.updates[userID] = in
	return in.ApplyTo(DefaultViewPrefs())
}

func TestHandler_UpdateViewPrefsAPI(t *testing.T) {
	tests := []struct {
		name       string
		userID     string // "" = visitor
		body       string
		wantErr    bool
		wantStatus int
		wantUser   string
	}{
		{"visitor refused", "", `{"theme":"dark"}`, true, 0, ""},
		{"campaign owner saves their own", "owner-1", `{"theme":"dark"}`, false, http.StatusOK, "owner-1"},
		{"player saves their own", "player-1", `{"motion":"calm"}`, false, http.StatusOK, "player-1"},
		{"a userId in the body is ignored", "player-1", `{"theme":"light","userId":"owner-1"}`, false, http.StatusOK, "player-1"},
		{"malformed body refused", "player-1", `{`, true, 0, ""},
		{"invalid value refused", "player-1", `{"textSize":"huge"}`, true, 0, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := &stubViewPrefsService{}
			h := &Handler{service: &validatingViewPrefsService{stubViewPrefsService: svc}}
			e := echo.New()
			req := httptest.NewRequest(http.MethodPut, "/account/view-prefs", strings.NewReader(tc.body))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			if tc.userID != "" {
				c.Set(contextKeyUserID, tc.userID)
			}
			err := h.UpdateViewPrefsAPI(c)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected an error")
				}
				if len(svc.updates) != 0 && tc.userID == "" {
					t.Error("service reached for a visitor")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if rec.Code != tc.wantStatus {
				t.Errorf("status %d, want %d", rec.Code, tc.wantStatus)
			}
			if len(svc.updates) != 1 {
				t.Fatalf("updates for %d users, want exactly 1: %v", len(svc.updates), svc.updates)
			}
			if _, ok := svc.updates[tc.wantUser]; !ok {
				t.Errorf("update did not target %q: %v", tc.wantUser, svc.updates)
			}
		})
	}
}

// validatingViewPrefsService makes the stub refuse invalid values the way the
// real service does, so the handler's error path is exercised.
type validatingViewPrefsService struct{ *stubViewPrefsService }

func (v *validatingViewPrefsService) UpdateViewPrefs(ctx context.Context, userID string, in UpdateViewPrefsInput) (ViewPrefs, error) {
	if _, err := in.ApplyTo(DefaultViewPrefs()); err != nil {
		return DefaultViewPrefs(), err
	}
	return v.stubViewPrefsService.UpdateViewPrefs(ctx, userID, in)
}

func TestViewPrefGroups_MatchAllowedLists(t *testing.T) {
	// The account card's buttons must offer exactly the values the service
	// accepts, default first, or a tap would be refused (or a value unreachable).
	allowed := map[string][]string{"theme": viewThemes, "motion": viewMotions, "textSize": viewTextSizes, "contrast": viewContrasts}
	for _, g := range viewPrefGroups {
		want := allowed[g.Key]
		if len(g.Options) != len(want) {
			t.Errorf("%s: %d buttons for %d allowed values", g.Key, len(g.Options), len(want))
			continue
		}
		seen := map[string]bool{}
		for _, o := range g.Options {
			seen[o.Value] = true
		}
		for _, v := range want {
			if !seen[v] {
				t.Errorf("%s: no button for %q", g.Key, v)
			}
		}
	}
}

func TestViewPrefs_WithBrushHidden(t *testing.T) {
	tests := []struct {
		name   string
		cur    []string
		id     string
		hidden bool
		want   []string
		bad    bool
	}{
		{"hide in one campaign", []string{}, "c-1", true, []string{"c-1"}, false},
		{"hiding again does not duplicate", []string{"c-1"}, "c-1", true, []string{"c-1"}, false},
		{"hiding one leaves the other hidden", []string{"c-1"}, "c-2", true, []string{"c-1", "c-2"}, false},
		{"showing one leaves the others hidden", []string{"c-1", "c-2"}, "c-1", false, []string{"c-2"}, false},
		{"showing one never hidden is harmless", []string{"c-1"}, "c-9", false, []string{"c-1"}, false},
		{"empty id refused", []string{}, "", true, nil, true},
		{"markup refused", []string{}, `"><script>`, true, nil, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cur := DefaultViewPrefs()
			cur.BrushHidden = tc.cur
			got, err := cur.WithBrushHidden(tc.id, tc.hidden)
			if tc.bad {
				assertAppError(t, err, http.StatusBadRequest)
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got.BrushHidden, tc.want) {
				t.Errorf("BrushHidden = %v, want %v", got.BrushHidden, tc.want)
			}
			if got.HidesBrushIn(tc.id) != tc.hidden {
				t.Errorf("HidesBrushIn(%q) = %v, want %v", tc.id, !tc.hidden, tc.hidden)
			}
		})
	}
}

func TestViewPrefs_BrushListIsBounded(t *testing.T) {
	p := DefaultViewPrefs()
	var err error
	for i := 0; i < maxBrushHidden+5; i++ {
		if p, err = p.WithBrushHidden(fmt.Sprintf("c-%d", i), true); err != nil {
			t.Fatal(err)
		}
	}
	if len(p.BrushHidden) != maxBrushHidden || p.HidesBrushIn("c-0") || !p.HidesBrushIn(fmt.Sprintf("c-%d", maxBrushHidden+4)) {
		t.Errorf("list not bounded to the newest %d: len %d", maxBrushHidden, len(p.BrushHidden))
	}
}

func TestService_SetBrushHidden_PerCampaignAndKeepsOtherChoices(t *testing.T) {
	repo := &mockUserRepo{}
	svc := newTestAuthService(repo)
	ctx := context.Background()
	if _, err := svc.UpdateViewPrefs(ctx, "u1", decodeViewPrefsInput(t, `{"theme":"dark"}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetBrushHidden(ctx, "u1", "c-1", true); err != nil {
		t.Fatal(err)
	}
	got, err := svc.SetBrushHidden(ctx, "u1", "c-2", true)
	if err != nil {
		t.Fatal(err)
	}
	if got.Theme != "dark" || !got.HidesBrushIn("c-1") || !got.HidesBrushIn("c-2") {
		t.Errorf("got %+v", got)
	}
	got, _ = svc.SetBrushHidden(ctx, "u1", "c-1", false)
	if read, _ := svc.GetViewPrefs(ctx, "u1"); got.HidesBrushIn("c-1") || !read.HidesBrushIn("c-2") || read.HidesBrushIn("c-1") {
		t.Errorf("showing c-1 must leave c-2 hidden: %+v", read)
	}
	// A later theme-only save keeps the brush choices.
	after, _ := svc.UpdateViewPrefs(ctx, "u1", decodeViewPrefsInput(t, `{"contrast":"high"}`))
	if !after.HidesBrushIn("c-2") {
		t.Errorf("a contrast-only save dropped the brush choice: %+v", after)
	}
}

// brushStub records who a brush save targets.
type brushStub struct {
	AuthService
	user, campaign string
	hidden         bool
}

func (s *brushStub) SetBrushHidden(_ context.Context, userID, campaignID string, hidden bool) (ViewPrefs, error) {
	s.user, s.campaign, s.hidden = userID, campaignID, hidden
	return DefaultViewPrefs(), nil
}

func TestHandler_SetBrushAPI(t *testing.T) {
	tests := []struct {
		name    string
		userID  string
		body    string
		wantErr bool
		wantHid bool
	}{
		{"visitor refused", "", `{"brush":"hidden"}`, true, false},
		{"hide", "u1", `{"brush":"hidden"}`, false, true},
		{"show", "u1", `{"brush":"shown"}`, false, false},
		{"unknown value refused", "u1", `{"brush":"off"}`, true, false},
		{"malformed refused", "u1", `{`, true, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			stub := &brushStub{}
			h := &Handler{service: stub}
			e := echo.New()
			req := httptest.NewRequest(http.MethodPut, "/account/brush/c-7", strings.NewReader(tc.body))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.SetPathValues(echo.PathValues{{Name: "campaignID", Value: "c-7"}})
			if tc.userID != "" {
				c.Set(contextKeyUserID, tc.userID)
			}
			err := h.SetBrushAPI(c)
			if tc.wantErr {
				if err == nil || stub.user != "" {
					t.Fatalf("want a refusal that never reaches the service; err=%v user=%q", err, stub.user)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if stub.user != tc.userID || stub.campaign != "c-7" || stub.hidden != tc.wantHid {
				t.Errorf("saved %+v", stub)
			}
		})
	}
}
