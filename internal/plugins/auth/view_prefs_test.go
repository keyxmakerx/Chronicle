// view_prefs_test.go pins the My view choices: validation against the
// allowed lists, partial-update semantics (absent preserves, null/"" resets,
// a value replaces), tolerant reads of the stored column, and that the
// handler only ever writes the signed-in person's own row.

package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
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
	stored := ViewPrefs{Theme: "dark", Motion: "calm", TextSize: "larger", Contrast: "high"}
	tests := []struct {
		name    string
		body    string
		cur     ViewPrefs
		want    ViewPrefs
		wantErr bool
	}{
		{"absent keys preserve everything", `{}`, stored, stored, false},
		{"a theme-only body keeps the other three", `{"theme":"light"}`, stored, ViewPrefs{"light", "calm", "larger", "high"}, false},
		{"each value replaces", `{"theme":"device","motion":"owner","textSize":"largest","contrast":"standard"}`, stored, ViewPrefs{"device", "owner", "largest", "standard"}, false},
		{"explicit null resets just that choice", `{"textSize":null}`, stored, ViewPrefs{"dark", "calm", "standard", "high"}, false},
		{"empty string means the default", `{"motion":""}`, stored, ViewPrefs{"dark", "owner", "larger", "high"}, false},
		{"unknown theme refused", `{"theme":"sepia"}`, stored, stored, true},
		{"off is a motion choice", `{"motion":"off"}`, stored, ViewPrefs{"dark", "off", "larger", "high"}, false},
		{"unknown motion refused", `{"motion":"none"}`, stored, stored, true},
		{"unknown size refused", `{"textSize":"huge"}`, stored, stored, true},
		{"unknown contrast refused", `{"contrast":"max"}`, stored, stored, true},
		{"case matters", `{"theme":"Dark"}`, stored, stored, true},
		{"one bad key refuses the whole body", `{"theme":"light","contrast":"x"}`, stored, stored, true},
		{"defaults from nothing", `{"theme":"dark"}`, DefaultViewPrefs(), ViewPrefs{"dark", "owner", "standard", "standard"}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := decodeViewPrefsInput(t, tc.body).ApplyTo(tc.cur)
			if tc.wantErr {
				assertAppError(t, err, http.StatusBadRequest)
			} else if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
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
		{"full row", `{"theme":"light","motion":"calm","textSize":"largest","contrast":"high"}`, ViewPrefs{"light", "calm", "largest", "high"}},
		{"partial row fills defaults", `{"theme":"dark"}`, ViewPrefs{"dark", "owner", "standard", "standard"}},
		{"invalid value falls back for that choice only", `{"theme":"neon","contrast":"high"}`, ViewPrefs{"device", "owner", "standard", "high"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := ParseViewPrefs([]byte(tc.raw)); got != tc.want {
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
	if want := (ViewPrefs{"dark", "owner", "standard", "high"}); got != want {
		t.Errorf("second update returned %+v, want %+v (the theme must survive a contrast-only save)", got, want)
	}
	if read, _ := svc.GetViewPrefs(ctx, "u1"); read != got {
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
