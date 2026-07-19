// audit_log_test.go covers V2 Wave 0 PR 4 (C-CAL-V2-AUDIT-LOG-INTEGRATION)
// handler-layer audit emission for the timeline plugin. Same harness
// pattern as calendar/audit_log_test.go — capture audit calls,
// exercise handler with a stub TimelineService.

package timeline

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/audit"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// --- Capturing audit recorder ---

type mockAuditService struct {
	mu      sync.Mutex
	entries []audit.AuditEntry
}

func (m *mockAuditService) Log(_ context.Context, entry *audit.AuditEntry) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries = append(m.entries, *entry)
	return nil
}
func (m *mockAuditService) GetCampaignActivity(_ context.Context, _ string, _ int) ([]audit.AuditEntry, int, error) {
	return nil, 0, nil
}
func (m *mockAuditService) GetEntityHistory(_ context.Context, _, _ string) ([]audit.AuditEntry, error) {
	return nil, nil
}
func (m *mockAuditService) GetCampaignStats(_ context.Context, _ string) (*audit.CampaignStats, error) {
	return nil, nil
}

func (m *mockAuditService) findByAction(action string) audit.AuditEntry {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, e := range m.entries {
		if e.Action == action {
			return e
		}
	}
	return audit.AuditEntry{}
}

// --- Stub TimelineService ---

type stubTimelineSvc struct {
	TimelineService

	tl              *Timeline
	createdTimeline *Timeline
	createdEvt      *TimelineEvent
	createdGroup    *EntityGroup
	createdConn     *EventConnection
}

func (s *stubTimelineSvc) GetTimeline(_ context.Context, id string) (*Timeline, error) {
	if s.tl != nil && s.tl.ID == id {
		return s.tl, nil
	}
	return nil, nil
}
func (s *stubTimelineSvc) CreateTimeline(_ context.Context, _ string, _ CreateTimelineInput) (*Timeline, error) {
	if s.createdTimeline != nil {
		return s.createdTimeline, nil
	}
	return &Timeline{ID: "tl-1", Name: "New Timeline"}, nil
}
func (s *stubTimelineSvc) UpdateTimeline(_ context.Context, _ string, _ UpdateTimelineInput) error {
	return nil
}
func (s *stubTimelineSvc) DeleteTimeline(_ context.Context, _ string) error { return nil }
func (s *stubTimelineSvc) LinkEvent(_ context.Context, _, _ string, _ LinkEventInput) (*EventLink, error) {
	return &EventLink{}, nil
}
func (s *stubTimelineSvc) UnlinkEvent(_ context.Context, _, _ string) error { return nil }
func (s *stubTimelineSvc) CreateStandaloneEvent(_ context.Context, _ string, _ CreateTimelineEventInput) (*TimelineEvent, error) {
	if s.createdEvt != nil {
		return s.createdEvt, nil
	}
	return &TimelineEvent{ID: "se-1", Name: "Standalone"}, nil
}
func (s *stubTimelineSvc) DeleteStandaloneEvent(_ context.Context, _, _ string) error { return nil }
func (s *stubTimelineSvc) CreateEntityGroup(_ context.Context, _ string, _ CreateEntityGroupInput) (*EntityGroup, error) {
	if s.createdGroup != nil {
		return s.createdGroup, nil
	}
	return &EntityGroup{ID: 1, Name: "Group A"}, nil
}
func (s *stubTimelineSvc) UpdateEntityGroup(_ context.Context, _ string, _ int, _ UpdateEntityGroupInput) error {
	return nil
}
func (s *stubTimelineSvc) DeleteEntityGroup(_ context.Context, _ string, _ int) error {
	return nil
}
func (s *stubTimelineSvc) AddGroupMember(_ context.Context, _ string, _ int, _ string) error {
	return nil
}
func (s *stubTimelineSvc) RemoveGroupMember(_ context.Context, _ string, _ int, _ string) error {
	return nil
}
func (s *stubTimelineSvc) CreateConnection(_ context.Context, _ string, _ CreateConnectionInput) (*EventConnection, error) {
	if s.createdConn != nil {
		return s.createdConn, nil
	}
	return &EventConnection{ID: 1}, nil
}
func (s *stubTimelineSvc) DeleteConnection(_ context.Context, _ string, _ int) error {
	return nil
}

// --- Helpers ---

func newReqWithCC(method, path string, body []byte, params map[string]string, userID string) (echo.Context, *campaigns.CampaignContext) {
	e := echo.New()
	var req *http.Request
	if body != nil {
		req = httptest.NewRequest(method, path, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	names := make([]string, 0, len(params))
	values := make([]string, 0, len(params))
	for k, v := range params {
		names = append(names, k)
		values = append(values, v)
	}
	c.SetParamNames(names...)
	c.SetParamValues(values...)
	cc := &campaigns.CampaignContext{
		Campaign:   &campaigns.Campaign{ID: "camp-1"},
		MemberRole: campaigns.RoleOwner,
	}
	c.Set("campaign_context", cc)
	c.Set("auth_user_id", userID)
	return c, cc
}

func makeHandler(svc TimelineService) (*Handler, *mockAuditService) {
	h := NewHandler(svc)
	rec := &mockAuditService{}
	h.SetAuditService(rec)
	return h, rec
}

// --- Tests ---

func TestCreateAPI_EmitsTimelineCreated(t *testing.T) {
	svc := &stubTimelineSvc{createdTimeline: &Timeline{ID: "tl-7", Name: "Saga"}}
	h, rec := makeHandler(svc)

	form := "name=Saga&visibility=everyone&color=%23ff0000&icon=fa-clock&calendar_id="
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/api", strings.NewReader(form))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recHTTP := httptest.NewRecorder()
	c := e.NewContext(req, recHTTP)
	c.SetParamNames("id")
	c.SetParamValues("camp-1")
	c.Set("campaign_context", &campaigns.CampaignContext{
		Campaign: &campaigns.Campaign{ID: "camp-1"}, MemberRole: campaigns.RoleOwner,
	})
	c.Set("auth_user_id", "u-1")

	if err := h.CreateForm(c); err != nil {
		t.Fatalf("CreateForm: %v", err)
	}
	got := rec.findByAction(audit.ActionTimelineCreated)
	if got.EntityID != "tl-7" {
		t.Errorf("EntityID=%q; want tl-7", got.EntityID)
	}
	if got.UserID != "u-1" {
		t.Errorf("UserID=%q; want u-1", got.UserID)
	}
}

func TestDeleteAPI_EmitsTimelineDeleted(t *testing.T) {
	svc := &stubTimelineSvc{tl: &Timeline{ID: "tl-5", CampaignID: "camp-1", Name: "Doomed"}}
	h, rec := makeHandler(svc)
	c, _ := newReqWithCC(http.MethodDelete, "/api", nil, map[string]string{"id": "camp-1", "tid": "tl-5"}, "u-1")
	if err := h.DeleteAPI(c); err != nil {
		t.Fatalf("DeleteAPI: %v", err)
	}
	got := rec.findByAction(audit.ActionTimelineDeleted)
	if got.EntityID != "tl-5" || got.EntityName != "Doomed" {
		t.Errorf("Delete audit got id=%q name=%q; want tl-5/Doomed", got.EntityID, got.EntityName)
	}
}

func TestLinkEventAPI_EmitsEventLinked(t *testing.T) {
	svc := &stubTimelineSvc{tl: &Timeline{ID: "tl-1", CampaignID: "camp-1"}}
	h, rec := makeHandler(svc)
	body := []byte(`{"event_id":"evt-3"}`)
	c, _ := newReqWithCC(http.MethodPost, "/api", body, map[string]string{"id": "camp-1", "tid": "tl-1"}, "u-1")
	if err := h.LinkEventAPI(c); err != nil {
		t.Fatalf("LinkEventAPI: %v", err)
	}
	got := rec.findByAction(audit.ActionTimelineEventLinked)
	if got.EntityID != "evt-3" {
		t.Errorf("EntityID=%q; want evt-3", got.EntityID)
	}
	if got.Details["timeline_id"] != "tl-1" {
		t.Errorf("Details[timeline_id]=%v; want tl-1", got.Details["timeline_id"])
	}
}

func TestUnlinkEventAPI_EmitsEventUnlinked(t *testing.T) {
	svc := &stubTimelineSvc{tl: &Timeline{ID: "tl-1", CampaignID: "camp-1"}}
	h, rec := makeHandler(svc)
	c, _ := newReqWithCC(http.MethodDelete, "/api", nil,
		map[string]string{"id": "camp-1", "tid": "tl-1", "eid": "evt-3"}, "u-1")
	if err := h.UnlinkEventAPI(c); err != nil {
		t.Fatalf("UnlinkEventAPI: %v", err)
	}
	got := rec.findByAction(audit.ActionTimelineEventUnlinked)
	if got.EntityID != "evt-3" {
		t.Errorf("EntityID=%q; want evt-3", got.EntityID)
	}
}

func TestCreateEntityGroupAPI_EmitsEntityGroupCreated(t *testing.T) {
	svc := &stubTimelineSvc{tl: &Timeline{ID: "tl-1", CampaignID: "camp-1"}, createdGroup: &EntityGroup{ID: 42, Name: "Nobles"}}
	h, rec := makeHandler(svc)
	c, _ := newReqWithCC(http.MethodPost, "/api", []byte(`{"name":"Nobles","color":"#aabbcc"}`),
		map[string]string{"id": "camp-1", "tid": "tl-1"}, "u-1")
	if err := h.CreateEntityGroupAPI(c); err != nil {
		t.Fatalf("CreateEntityGroupAPI: %v", err)
	}
	got := rec.findByAction(audit.ActionTimelineEntityGroupCreated)
	if got.EntityID != "42" || got.EntityName != "Nobles" {
		t.Errorf("EntityGroup audit got id=%q name=%q; want 42/Nobles", got.EntityID, got.EntityName)
	}
}

// TestAuditEmitNilSafe — no auditor wired → no panic, primary op
// still succeeds.
func TestAuditEmitNilSafe(t *testing.T) {
	svc := &stubTimelineSvc{tl: &Timeline{ID: "tl-1", CampaignID: "camp-1", Name: "T"}}
	h := NewHandler(svc) // no SetAuditService
	c, _ := newReqWithCC(http.MethodDelete, "/api", nil, map[string]string{"id": "camp-1", "tid": "tl-1"}, "u-1")
	if err := h.DeleteAPI(c); err != nil {
		t.Fatalf("DeleteAPI: %v", err)
	}
}
