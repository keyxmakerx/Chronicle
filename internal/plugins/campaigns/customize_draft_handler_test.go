package campaigns

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
)

type auditRecorder struct{ events []map[string]any }

func (a *auditRecorder) LogEvent(_ context.Context, _, _, action string, details map[string]any) error {
	d := map[string]any{"action": action}
	for k, v := range details {
		d[k] = v
	}
	a.events = append(a.events, d)
	return nil
}

type draftEnv struct {
	h     *Handler
	repo  *memDraftRepo
	audit *auditRecorder
	// applied records each section Apply was called with, in order.
	applied []string
}

func newDraftEnv() *draftEnv {
	env := &draftEnv{repo: newMemDraftRepo(), audit: &auditRecorder{}}
	env.h = &Handler{}
	env.h.SetCustomizeDrafts(NewCustomizeDraftService(env.repo))
	env.h.SetAuditLogger(env.audit)
	return env
}

// section registers a section that records its call and returns what fn says.
func (e *draftEnv) section(key string, fn func(raw json.RawMessage) (json.RawMessage, error)) {
	e.h.RegisterCustomizeSection(CustomizeSection{Key: key, Apply: func(_ context.Context, _ *CampaignContext, _ string, raw json.RawMessage) (json.RawMessage, error) {
		e.applied = append(e.applied, key)
		return fn(raw)
	}})
}

type draftReply struct {
	code int
	body map[string]any
	err  error
}

func (e *draftEnv) call(t *testing.T, handler echo.HandlerFunc, method, target string, role Role, body any) draftReply {
	t.Helper()
	var rd *bytes.Reader
	switch b := body.(type) {
	case nil:
		rd = bytes.NewReader(nil)
	case string:
		rd = bytes.NewReader([]byte(b))
	default:
		raw, _ := json.Marshal(b)
		rd = bytes.NewReader(raw)
	}
	req := httptest.NewRequest(method, target, rd)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.Set(contextKeyCampaign, &CampaignContext{Campaign: &Campaign{ID: "camp-1"}, MemberRole: role})
	auth.SetSession(c, &auth.Session{UserID: "owner-1"})
	err := handler(c)
	r := draftReply{err: err, code: rec.Code}
	var ae *apperror.AppError
	if errors.As(err, &ae) {
		r.code = ae.Code
	}
	if rec.Body.Len() > 0 {
		_ = json.Unmarshal(rec.Body.Bytes(), &r.body)
	}
	return r
}

func (e *draftEnv) stored(t *testing.T) map[string]json.RawMessage {
	t.Helper()
	d, _ := e.repo.Get(context.Background(), "camp-1", "owner-1")
	if d == nil {
		return nil
	}
	m, err := parseCustomizeDraft(d.Draft)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestDraftAPI_OwnerOnly(t *testing.T) {
	env := newDraftEnv()
	env.section("look", func(json.RawMessage) (json.RawMessage, error) { return nil, nil })
	routes := []struct {
		name    string
		handler echo.HandlerFunc
		method  string
		body    any
	}{
		{"get", env.h.GetCustomizeDraftAPI, http.MethodGet, nil},
		{"put", env.h.PutCustomizeDraftAPI, http.MethodPut, map[string]any{"version": 0, "draft": map[string]any{"look": 1}}},
		{"delete", env.h.DeleteCustomizeDraftAPI, http.MethodDelete, nil},
		{"publish", env.h.PublishCustomizeAPI, http.MethodPost, map[string]any{"version": 0}},
	}
	for _, role := range []Role{RolePlayer, RoleScribe} {
		for _, rt := range routes {
			r := env.call(t, rt.handler, rt.method, "/campaigns/camp-1/customize/draft", role, rt.body)
			if r.code != http.StatusForbidden {
				t.Errorf("%s as role %v = %d, want 403", rt.name, role, r.code)
			}
		}
	}
	if len(env.applied) != 0 || env.stored(t) != nil {
		t.Error("a forbidden request changed something")
	}
}

func TestDraftAPI_RoutesRequireOwner(t *testing.T) {
	src, err := readRoutesSource()
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range []string{
		`cg.GET("/customize/draft", h.GetCustomizeDraftAPI, RequireRole(RoleOwner))`,
		`cg.PUT("/customize/draft", h.PutCustomizeDraftAPI, RequireRole(RoleOwner))`,
		`cg.DELETE("/customize/draft", h.DeleteCustomizeDraftAPI, RequireRole(RoleOwner))`,
		`cg.POST("/customize/publish", h.PublishCustomizeAPI, RequireRole(RoleOwner))`,
	} {
		if !strings.Contains(src, line) {
			t.Errorf("routes.go lacks %s", line)
		}
	}
}

func TestDraftAPI_PutGetDelete(t *testing.T) {
	env := newDraftEnv()
	env.section("look", func(json.RawMessage) (json.RawMessage, error) { return nil, nil })
	const path = "/campaigns/camp-1/customize/draft"

	r := env.call(t, env.h.GetCustomizeDraftAPI, http.MethodGet, path, RoleOwner, nil)
	if r.code != 200 || r.body["version"] != float64(0) || !reflect.DeepEqual(r.body["draft"], map[string]any{}) {
		t.Fatalf("empty get = %d %v", r.code, r.body)
	}

	r = env.call(t, env.h.PutCustomizeDraftAPI, http.MethodPut, path, RoleOwner, map[string]any{"version": 0, "draft": map[string]any{"look": map[string]any{"a": 1}}})
	if r.code != 200 || r.body["version"] != float64(1) {
		t.Fatalf("first put = %d %v", r.code, r.body)
	}
	r = env.call(t, env.h.PutCustomizeDraftAPI, http.MethodPut, path, RoleOwner, map[string]any{"version": 1, "draft": map[string]any{"look": map[string]any{"a": 2}}})
	if r.code != 200 || r.body["version"] != float64(2) {
		t.Fatalf("second put = %d %v", r.code, r.body)
	}

	r = env.call(t, env.h.GetCustomizeDraftAPI, http.MethodGet, path, RoleOwner, nil)
	if r.body["version"] != float64(2) || !reflect.DeepEqual(r.body["draft"], map[string]any{"look": map[string]any{"a": float64(2)}}) {
		t.Fatalf("get = %v", r.body)
	}

	r = env.call(t, env.h.DeleteCustomizeDraftAPI, http.MethodDelete, path+"?version=1", RoleOwner, nil)
	if r.code != http.StatusConflict || r.body["version"] != float64(2) {
		t.Fatalf("stale delete = %d %v", r.code, r.body)
	}
	r = env.call(t, env.h.DeleteCustomizeDraftAPI, http.MethodDelete, path+"?version=x", RoleOwner, nil)
	if r.code != http.StatusBadRequest {
		t.Fatalf("bad version = %d", r.code)
	}
	r = env.call(t, env.h.DeleteCustomizeDraftAPI, http.MethodDelete, path+"?version=2", RoleOwner, nil)
	if r.code != http.StatusNoContent || env.stored(t) != nil {
		t.Fatalf("delete = %d, stored %v", r.code, env.stored(t))
	}
}

func TestDraftAPI_PutConflictAndUnknownKey(t *testing.T) {
	env := newDraftEnv()
	env.section("look", func(json.RawMessage) (json.RawMessage, error) { return nil, nil })
	const path = "/campaigns/camp-1/customize/draft"
	env.call(t, env.h.PutCustomizeDraftAPI, http.MethodPut, path, RoleOwner, map[string]any{"version": 0, "draft": map[string]any{"look": "mine"}})

	// A second tab that still believes there is no draft.
	r := env.call(t, env.h.PutCustomizeDraftAPI, http.MethodPut, path, RoleOwner, map[string]any{"version": 0, "draft": map[string]any{"look": "theirs"}})
	if r.code != http.StatusConflict {
		t.Fatalf("code = %d, want 409", r.code)
	}
	if r.body["error"] != "conflict" || r.body["message"] == "" || r.body["version"] != float64(1) ||
		!reflect.DeepEqual(r.body["draft"], map[string]any{"look": "mine"}) {
		t.Errorf("conflict body = %v", r.body)
	}
	if got := string(env.stored(t)["look"]); got != `"mine"` {
		t.Errorf("the losing write changed the draft: %s", got)
	}

	r = env.call(t, env.h.PutCustomizeDraftAPI, http.MethodPut, path, RoleOwner, map[string]any{"version": 1, "draft": map[string]any{"bogus": 1}})
	if r.code != http.StatusBadRequest {
		t.Errorf("unknown key = %d, want 400", r.code)
	}
	r = env.call(t, env.h.PutCustomizeDraftAPI, http.MethodPut, path, RoleOwner, `not json`)
	if r.code != http.StatusBadRequest {
		t.Errorf("bad body = %d, want 400", r.code)
	}
}

func TestPublish_Conflict(t *testing.T) {
	env := newDraftEnv()
	env.section("look", func(json.RawMessage) (json.RawMessage, error) { return nil, nil })
	const path = "/campaigns/camp-1/customize/publish"
	env.call(t, env.h.PutCustomizeDraftAPI, http.MethodPut, path, RoleOwner, map[string]any{"version": 0, "draft": map[string]any{"look": 1}})

	for _, v := range []int{0, 7} {
		r := env.call(t, env.h.PublishCustomizeAPI, http.MethodPost, path, RoleOwner, map[string]any{"version": v})
		if r.code != http.StatusConflict || r.body["version"] != float64(1) {
			t.Errorf("version %d: %d %v", v, r.code, r.body)
		}
	}
	if len(env.applied) != 0 {
		t.Errorf("a conflicting publish applied %v", env.applied)
	}
	if env.stored(t) == nil {
		t.Error("a conflicting publish removed the draft")
	}
}

func TestPublish_UsesStoredDraftNotRequestBody(t *testing.T) {
	env := newDraftEnv()
	var got string
	env.section("look", func(raw json.RawMessage) (json.RawMessage, error) { got = string(raw); return nil, nil })
	env.call(t, env.h.PutCustomizeDraftAPI, http.MethodPut, "/x", RoleOwner, map[string]any{"version": 0, "draft": map[string]any{"look": "stored"}})
	r := env.call(t, env.h.PublishCustomizeAPI, http.MethodPost, "/x", RoleOwner, map[string]any{"version": 1, "draft": map[string]any{"look": "smuggled"}})
	if r.code != 200 || got != `"stored"` {
		t.Fatalf("code %d, applied %q", r.code, got)
	}
}

func TestPublish_PartialFailureKeepsFailedSection(t *testing.T) {
	env := newDraftEnv()
	env.section("look", func(json.RawMessage) (json.RawMessage, error) { return nil, nil })
	env.section("categories", func(raw json.RawMessage) (json.RawMessage, error) {
		return json.RawMessage(`{"items":{"7":{"name":"left"}}}`), apperror.NewConflict("Town still has 2 pages")
	})
	env.section("extra", func(json.RawMessage) (json.RawMessage, error) {
		return nil, errors.New("pq: table secret_tbl does not exist")
	})
	env.call(t, env.h.PutCustomizeDraftAPI, http.MethodPut, "/x", RoleOwner, map[string]any{"version": 0, "draft": map[string]any{
		"look": map[string]any{"input": 1}, "categories": map[string]any{"items": 1}, "extra": map[string]any{"k": "v"},
	}})

	r := env.call(t, env.h.PublishCustomizeAPI, http.MethodPost, "/x", RoleOwner, map[string]any{"version": 1})
	if r.code != 200 {
		t.Fatalf("code = %d (%v)", r.code, r.err)
	}
	if !reflect.DeepEqual(r.body["applied"], []any{"look"}) {
		t.Errorf("applied = %v", r.body["applied"])
	}
	failed, _ := r.body["failed"].([]any)
	if len(failed) != 2 {
		t.Fatalf("failed = %v", r.body["failed"])
	}
	f0 := failed[0].(map[string]any)
	f1 := failed[1].(map[string]any)
	if f0["key"] != "categories" || f0["message"] != "Town still has 2 pages" {
		t.Errorf("failed[0] = %v", f0)
	}
	if f1["key"] != "extra" || strings.Contains(f1["message"].(string), "secret_tbl") {
		t.Errorf("a raw error leaked: %v", f1)
	}
	if r.body["version"] != float64(2) {
		t.Errorf("version = %v, want 2", r.body["version"])
	}
	wantDraft := map[string]any{
		"categories": map[string]any{"items": map[string]any{"7": map[string]any{"name": "left"}}},
		"extra":      map[string]any{"k": "v"},
	}
	if !reflect.DeepEqual(r.body["draft"], wantDraft) {
		t.Errorf("draft = %v", r.body["draft"])
	}
	stored := env.stored(t)
	if _, ok := stored["look"]; ok || len(stored) != 2 {
		t.Errorf("stored sections = %v", stored)
	}
	if !reflect.DeepEqual(env.applied, []string{"look", "categories", "extra"}) {
		t.Errorf("apply order = %v", env.applied)
	}
	if len(env.audit.events) != 1 || env.audit.events[0]["action"] != "campaign.customize.published" {
		t.Errorf("audit = %v", env.audit.events)
	}
}

func TestPublish_AllAppliedDeletesDraft(t *testing.T) {
	env := newDraftEnv()
	env.section("look", func(json.RawMessage) (json.RawMessage, error) { return nil, nil })
	env.call(t, env.h.PutCustomizeDraftAPI, http.MethodPut, "/x", RoleOwner, map[string]any{"version": 0, "draft": map[string]any{"look": 1}})
	r := env.call(t, env.h.PublishCustomizeAPI, http.MethodPost, "/x", RoleOwner, map[string]any{"version": 1})
	if r.code != 200 || r.body["version"] != float64(0) || !reflect.DeepEqual(r.body["draft"], map[string]any{}) {
		t.Fatalf("reply = %d %v", r.code, r.body)
	}
	if env.stored(t) != nil {
		t.Error("the draft survived a full publish")
	}
	if failed, _ := r.body["failed"].([]any); len(failed) != 0 {
		t.Errorf("failed = %v", r.body["failed"])
	}
}

func TestPublish_FailureWithNilRemainingKeepsRaw(t *testing.T) {
	env := newDraftEnv()
	env.section("look", func(json.RawMessage) (json.RawMessage, error) {
		return nil, apperror.NewBadRequest("that picture doesn't belong to this campaign")
	})
	env.call(t, env.h.PutCustomizeDraftAPI, http.MethodPut, "/x", RoleOwner, map[string]any{"version": 0, "draft": map[string]any{"look": map[string]any{"input": "x"}}})
	r := env.call(t, env.h.PublishCustomizeAPI, http.MethodPost, "/x", RoleOwner, map[string]any{"version": 1})
	if r.code != 200 || r.body["version"] != float64(1) {
		t.Fatalf("reply = %d %v", r.code, r.body)
	}
	if got := string(env.stored(t)["look"]); got != `{"input":"x"}` {
		t.Errorf("look = %s", got)
	}
}

func TestPublish_KeepsEditsMadeDuringApply(t *testing.T) {
	env := newDraftEnv()
	svc := NewCustomizeDraftService(env.repo)
	env.section("look", func(json.RawMessage) (json.RawMessage, error) {
		// The owner edits the look in another tab while it is being applied.
		_, err := svc.Save(context.Background(), "camp-1", "owner-1", 1, json.RawMessage(`{"look":"newer","other":"o"}`), []string{"look", "other"})
		return nil, err
	})
	env.section("other", func(json.RawMessage) (json.RawMessage, error) { return nil, nil })
	env.call(t, env.h.PutCustomizeDraftAPI, http.MethodPut, "/x", RoleOwner, map[string]any{"version": 0, "draft": map[string]any{"look": "older", "other": "o"}})

	r := env.call(t, env.h.PublishCustomizeAPI, http.MethodPost, "/x", RoleOwner, map[string]any{"version": 1})
	if r.code != 200 {
		t.Fatalf("code = %d (%v)", r.code, r.err)
	}
	stored := env.stored(t)
	if string(stored["look"]) != `"newer"` {
		t.Errorf("the newer look edit was lost: %v", stored)
	}
	if _, ok := stored["other"]; ok {
		t.Errorf("an applied, unchanged section stayed: %v", stored)
	}
}

func TestPublish_NoDraft(t *testing.T) {
	env := newDraftEnv()
	env.section("look", func(json.RawMessage) (json.RawMessage, error) { return nil, nil })
	r := env.call(t, env.h.PublishCustomizeAPI, http.MethodPost, "/x", RoleOwner, map[string]any{"version": 0})
	if r.code != 200 || r.body["version"] != float64(0) {
		t.Fatalf("reply = %d %v", r.code, r.body)
	}
	if len(env.audit.events) != 0 {
		t.Errorf("audited a no-op: %v", env.audit.events)
	}
}

func readRoutesSource() (string, error) {
	b, err := os.ReadFile("routes.go")
	return string(b), err
}

func TestCustomizeDraftFor(t *testing.T) {
	env := newDraftEnv()
	env.section("look", func(json.RawMessage) (json.RawMessage, error) { return nil, nil })
	if v, d := env.h.CustomizeDraftFor(context.Background(), "camp-1", "owner-1"); v != 0 || string(d) != `{}` {
		t.Errorf("none = %d %s", v, d)
	}
	env.call(t, env.h.PutCustomizeDraftAPI, http.MethodPut, "/x", RoleOwner, map[string]any{"version": 0, "draft": map[string]any{"look": 1}})
	if v, d := env.h.CustomizeDraftFor(context.Background(), "camp-1", "owner-1"); v != 1 || string(d) != `{"look":1}` {
		t.Errorf("stored = %d %s", v, d)
	}
	if v, d := (&Handler{}).CustomizeDraftFor(context.Background(), "camp-1", "owner-1"); v != 0 || string(d) != `{}` {
		t.Errorf("unwired = %d %s", v, d)
	}
}
