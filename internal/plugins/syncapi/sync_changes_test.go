package syncapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	ws "github.com/keyxmakerx/chronicle/internal/websocket"
)

// fakeChangeRepo is an in-memory SyncChangeRepository.
type fakeChangeRepo struct {
	rows      []SyncChange
	pruned    int64
	appendErr error
	appended  []SyncChange
}

func (f *fakeChangeRepo) Append(_ context.Context, _, rt, id, op string) (int64, error) {
	if f.appendErr != nil {
		return 0, f.appendErr
	}
	seq := int64(len(f.appended) + 1)
	f.appended = append(f.appended, SyncChange{Seq: seq, Type: rt, ResourceID: id, Op: op})
	return seq, nil
}

func (f *fakeChangeRepo) List(_ context.Context, _ string, since int64, limit int) ([]SyncChange, error) {
	out := []SyncChange{}
	for _, r := range f.rows {
		if r.Seq > since && len(out) < limit {
			out = append(out, r)
		}
	}
	return out, nil
}

func (f *fakeChangeRepo) PrunedThrough(context.Context, string) (int64, error) { return f.pruned, nil }

func (f *fakeChangeRepo) Head(context.Context, string) (int64, error) {
	if len(f.rows) == 0 {
		return 0, nil
	}
	return f.rows[len(f.rows)-1].Seq, nil
}

func (f *fakeChangeRepo) PruneOlderThan(context.Context, time.Time) (int64, error) { return 0, nil }

// changesCampaignSvc resolves a role and a co-DM grant for the handler.
type changesCampaignSvc struct {
	campaigns.CampaignService
	role    campaigns.Role
	granted bool
}

func (s *changesCampaignSvc) GetMember(context.Context, string, string) (*campaigns.CampaignMember, error) {
	return &campaigns.CampaignMember{Role: s.role}, nil
}

func (s *changesCampaignSvc) IsUserDmGranted(context.Context, string, string) (bool, error) {
	return s.granted, nil
}

func seqRows(n int) []SyncChange {
	rows := make([]SyncChange, n)
	for i := range rows {
		rows[i] = SyncChange{Seq: int64(i + 1), Type: "entity", ResourceID: "e", Op: "updated"}
	}
	return rows
}

func TestListChanges(t *testing.T) {
	tests := []struct {
		name       string
		query      string
		rows       []SyncChange
		pruned     int64
		role       campaigns.Role
		granted    bool
		noKey      bool
		wantStatus int
		wantLen    int
		wantNext   int64
		wantMore   bool
		wantReset  bool
	}{
		{name: "owner from start", query: "since=0", rows: seqRows(3), role: campaigns.RoleOwner, wantStatus: 200, wantLen: 3, wantNext: 3},
		{name: "since skips seen", query: "since=2", rows: seqRows(3), role: campaigns.RoleOwner, wantStatus: 200, wantLen: 1, wantNext: 3},
		{name: "empty keeps cursor", query: "since=3", rows: seqRows(3), role: campaigns.RoleOwner, wantStatus: 200, wantLen: 0, wantNext: 3},
		{name: "limit sets hasMore", query: "limit=2", rows: seqRows(5), role: campaigns.RoleOwner, wantStatus: 200, wantLen: 2, wantNext: 2, wantMore: true},
		{name: "exact fit no more", query: "limit=3", rows: seqRows(3), role: campaigns.RoleOwner, wantStatus: 200, wantLen: 3, wantNext: 3},
		{name: "limit below one clamps to one", query: "limit=0", rows: seqRows(3), role: campaigns.RoleOwner, wantStatus: 200, wantLen: 1, wantNext: 1, wantMore: true},
		{name: "limit above max clamps", query: "limit=999999", rows: seqRows(1001), role: campaigns.RoleOwner, wantStatus: 200, wantLen: 1000, wantNext: 1000, wantMore: true},
		{name: "default limit is 500", query: "", rows: seqRows(501), role: campaigns.RoleOwner, wantStatus: 200, wantLen: 500, wantNext: 500, wantMore: true},
		{name: "since below watermark resets to head", query: "since=2", rows: seqRows(10), pruned: 5, role: campaigns.RoleOwner, wantStatus: 200, wantLen: 0, wantNext: 10, wantReset: true},
		{name: "since at watermark is replayable", query: "since=5", rows: seqRows(7), pruned: 5, role: campaigns.RoleOwner, wantStatus: 200, wantLen: 2, wantNext: 7},
		{name: "co-DM grant allowed", query: "since=0", rows: seqRows(1), role: campaigns.RolePlayer, granted: true, wantStatus: 200, wantLen: 1, wantNext: 1},
		{name: "player forbidden", query: "since=0", rows: seqRows(1), role: campaigns.RolePlayer, wantStatus: 403},
		{name: "no key forbidden", query: "since=0", rows: seqRows(1), noKey: true, wantStatus: 403},
		{name: "bad since", query: "since=abc", rows: seqRows(1), role: campaigns.RoleOwner, wantStatus: 400},
		{name: "negative since", query: "since=-1", rows: seqRows(1), role: campaigns.RoleOwner, wantStatus: 400},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeChangeRepo{rows: tt.rows, pruned: tt.pruned}
			h := NewSyncChangesHandler(repo, &changesCampaignSvc{role: tt.role, granted: tt.granted})
			e := echo.New()
			req := httptest.NewRequest(http.MethodGet, "/api/v1/campaigns/camp-1/sync/changes?"+tt.query, nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.SetParamNames("id")
			c.SetParamValues("camp-1")
			if !tt.noKey {
				c.Set(apiKeyContextKey, &APIKey{ID: 1, CampaignID: "camp-1", UserID: "u1"})
			}

			err := h.ListChanges(c)
			if tt.wantStatus != http.StatusOK {
				var ae *apperror.AppError
				if !errors.As(err, &ae) || ae.Code != tt.wantStatus {
					t.Fatalf("want status %d, got %v", tt.wantStatus, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			var got struct {
				Changes       []SyncChange `json:"changes"`
				Next          int64        `json:"next"`
				HasMore       bool         `json:"hasMore"`
				ResetRequired bool         `json:"resetRequired"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if len(got.Changes) != tt.wantLen || got.Next != tt.wantNext || got.HasMore != tt.wantMore || got.ResetRequired != tt.wantReset {
				t.Fatalf("got len=%d next=%d more=%v reset=%v; want len=%d next=%d more=%v reset=%v",
					len(got.Changes), got.Next, got.HasMore, got.ResetRequired, tt.wantLen, tt.wantNext, tt.wantMore, tt.wantReset)
			}
		})
	}
}

type capturingBus struct{ msgs []*ws.Message }

func (b *capturingBus) Publish(m *ws.Message) { b.msgs = append(b.msgs, m) }

func TestRecordingEventBus(t *testing.T) {
	tests := []struct {
		name         string
		msg          *ws.Message
		appendErr    error
		wantRecorded *SyncChange
		wantSeq      int64
	}{
		{name: "entity created", msg: ws.NewMessage(ws.MsgEntityCreated, "c1", "e1", nil), wantRecorded: &SyncChange{Seq: 1, Type: "entity", ResourceID: "e1", Op: "created"}, wantSeq: 1},
		{name: "token moved is update", msg: ws.NewMessage(ws.MsgTokenMoved, "c1", "t1", nil), wantRecorded: &SyncChange{Seq: 1, Type: "token", ResourceID: "t1", Op: "updated"}, wantSeq: 1},
		{name: "map updated", msg: ws.NewMessage(ws.MsgMapUpdated, "c1", "m1", nil), wantRecorded: &SyncChange{Seq: 1, Type: "map", ResourceID: "m1", Op: "updated"}, wantSeq: 1},
		{name: "calendar event deleted", msg: ws.NewMessage(ws.MsgCalendarEventDeleted, "c1", "ev1", nil), wantRecorded: &SyncChange{Seq: 1, Type: "calendar_event", ResourceID: "ev1", Op: "deleted"}, wantSeq: 1},
		{name: "entity_type is its own type", msg: ws.NewMessage(ws.MsgEntityTypeUpdated, "c1", "3", nil), wantRecorded: &SyncChange{Seq: 1, Type: "entity_type", ResourceID: "3", Op: "updated"}, wantSeq: 1},
		{name: "relation keyed on its source entity", msg: ws.NewMessage(ws.MsgRelationCreated, "c1", "hero", nil), wantRecorded: &SyncChange{Seq: 1, Type: "relation", ResourceID: "hero", Op: "created"}, wantSeq: 1},
		{name: "relation metadata write is update", msg: ws.NewMessage(ws.MsgRelationMetadataUpdated, "c1", "hero", nil), wantRecorded: &SyncChange{Seq: 1, Type: "relation", ResourceID: "hero", Op: "updated"}, wantSeq: 1},
		{name: "relation deleted", msg: ws.NewMessage(ws.MsgRelationDeleted, "c1", "hero", nil), wantRecorded: &SyncChange{Seq: 1, Type: "relation", ResourceID: "hero", Op: "deleted"}, wantSeq: 1},
		{name: "entity_note not allowlisted", msg: ws.NewMessage(ws.MsgEntityNoteCreated, "c1", "n1", nil)},
		{name: "calendar date not allowlisted", msg: ws.NewMessage(ws.MsgCalendarDateAdvanced, "c1", "", nil)},
		{name: "sync control not allowlisted", msg: ws.NewMessage(ws.MsgSyncStatus, "c1", "", nil)},
		{name: "missing campaign not recorded", msg: ws.NewMessage(ws.MsgEntityCreated, "", "e1", nil)},
		{name: "record failure still publishes", msg: ws.NewMessage(ws.MsgEntityUpdated, "c1", "e1", nil), appendErr: errors.New("db down")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeChangeRepo{appendErr: tt.appendErr}
			next := &capturingBus{}
			orig := *tt.msg
			NewRecordingEventBus(next, repo).Publish(tt.msg)

			if len(next.msgs) != 1 {
				t.Fatalf("want exactly 1 live publish, got %d", len(next.msgs))
			}
			if next.msgs[0].Seq != tt.wantSeq {
				t.Fatalf("seq = %d, want %d", next.msgs[0].Seq, tt.wantSeq)
			}
			if tt.msg.Seq != orig.Seq {
				t.Fatal("caller's message was mutated")
			}
			switch {
			case tt.wantRecorded == nil && len(repo.appended) != 0:
				t.Fatalf("unexpected record: %+v", repo.appended)
			case tt.wantRecorded != nil && (len(repo.appended) != 1 || repo.appended[0] != *tt.wantRecorded):
				t.Fatalf("recorded %+v, want %+v", repo.appended, *tt.wantRecorded)
			}
		})
	}
}
