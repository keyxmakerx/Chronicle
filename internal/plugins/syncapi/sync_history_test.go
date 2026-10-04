package syncapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// fakeHistoryRepo records inserts and returns canned lists.
type fakeHistoryRepo struct {
	mu       sync.Mutex
	inserted []SyncEvent
	listed   SyncHistoryFilter
	list     []SyncEvent
	done     chan struct{}
	// The call flow's reads.
	rows     []SyncEvent
	latest   map[bool]*SyncEvent
	failures []SyncEvent
}

func (f *fakeHistoryRepo) Insert(_ context.Context, _ string, ev *SyncEvent) (int64, error) {
	f.mu.Lock()
	f.inserted = append(f.inserted, *ev)
	f.mu.Unlock()
	if f.done != nil {
		f.done <- struct{}{}
	}
	return int64(len(f.inserted)), nil
}

func (f *fakeHistoryRepo) List(_ context.Context, _ string, flt SyncHistoryFilter) ([]SyncEvent, error) {
	f.listed = flt
	return f.list, nil
}

func (f *fakeHistoryRepo) Get(_ context.Context, _ string, id int64) (*SyncEvent, error) {
	for i := range f.rows {
		if f.rows[i].ID == id {
			return &f.rows[i], nil
		}
	}
	return nil, apperror.NewNotFound("that history entry was not found")
}

func (f *fakeHistoryRepo) Window(_ context.Context, _ string, from, to time.Time, _ int) ([]SyncEvent, error) {
	var out []SyncEvent
	for _, ev := range f.rows {
		if !ev.OccurredAt.Before(from) && !ev.OccurredAt.After(to) {
			out = append(out, ev)
		}
	}
	return out, nil
}

func (f *fakeHistoryRepo) Latest(_ context.Context, _ string, _ time.Time, failedOnly bool) (*SyncEvent, error) {
	return f.latest[failedOnly], nil
}

func (f *fakeHistoryRepo) Failures(context.Context, string, string, string, time.Time, int) ([]SyncEvent, error) {
	return f.failures, nil
}

func (f *fakeHistoryRepo) PruneOlderThan(context.Context, time.Time) (int64, error) { return 0, nil }

func TestClassifyHistoryCall(t *testing.T) {
	cases := []struct {
		name, method, path string
		status             int
		want               bool
		kind, action       string
	}{
		{"page update", "PUT", "/api/v1/campaigns/:id/entities/:entityID", 200, true, "page", "page updated"},
		{"failed write still recorded", "PUT", "/api/v1/campaigns/:id/entities/:entityID", 409, true, "page", "page updated"},
		{"read is not history", "GET", "/api/v1/campaigns/:id/entities", 200, false, "", ""},
		{"read the server failed is", "GET", "/api/v1/campaigns/:id/entities", 500, true, "entities", "read failed"},
		{"token drag skipped", "PATCH", "/api/v1/campaigns/:id/maps/:mapID/tokens/:tokenID/position", 200, false, "", ""},
		{"module reports skipped", "POST", "/api/v1/campaigns/:id/sync/history", 200, false, "", ""},
		{"unlisted write gets a generic name", "POST", "/api/v1/campaigns/:id/handouts/items", 201, true, "handouts", "handouts created"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r, _, ok := classifyHistoryCall(tc.method, tc.path, tc.status)
			if ok != tc.want {
				t.Fatalf("recorded = %v, want %v", ok, tc.want)
			}
			if ok && (r.kind != tc.kind || r.action != tc.action) {
				t.Fatalf("got %q/%q, want %q/%q", r.kind, r.action, tc.kind, tc.action)
			}
		})
	}
}

func TestHistoryFilterFrom(t *testing.T) {
	cases := []struct {
		name, query string
		wantErr     bool
		check       func(SyncHistoryFilter) bool
	}{
		{"defaults", "", false, func(f SyncHistoryFilter) bool { return f.Limit == historyPageSize && f.Direction == "" }},
		{"all filters", "before=9&after=2&limit=10&direction=to_foundry&failed=1&q=+Ren+", false, func(f SyncHistoryFilter) bool {
			return f.Before == 9 && f.After == 2 && f.Limit == 10 && f.Direction == DirToFoundry && f.FailedOnly && f.Query == "Ren"
		}},
		{"limit above max ignored", "limit=5000", false, func(f SyncHistoryFilter) bool { return f.Limit == historyPageSize }},
		{"bad direction", "direction=sideways", true, nil},
		{"negative cursor", "before=-1", true, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/?"+tc.query, nil), httptest.NewRecorder())
			f, err := historyFilterFrom(c)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if err == nil && !tc.check(f) {
				t.Fatalf("unexpected filter %+v", f)
			}
		})
	}
}

type fixedNamer map[string]string

func (f fixedNamer) EntityName(_ context.Context, _, id string) (string, bool) {
	n, ok := f[id]
	return n, ok
}

type fixedEditor struct{ uid string }

func (f fixedEditor) LastEditor(context.Context, string, string, time.Time) (string, bool) {
	return f.uid, f.uid != ""
}

func TestToEvent_ClampsAndNamesEditor(t *testing.T) {
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	h := &SyncHistoryHandler{editors: fixedEditor{uid: "editor-1"}, namer: fixedNamer{"e1": "Harbour"}, now: func() time.Time { return now }}
	key := &APIKey{ID: 7, UserID: "gm-1", CampaignID: "c1"}
	cases := []struct {
		name    string
		in      reportedEvent
		wantAt  time.Time
		wantWho string
		wantErr bool
	}{
		{"future clock replaced by now", reportedEvent{Direction: DirLink, At: now.Add(time.Hour)}, now, "gm-1", false},
		{"week-old time replaced by now", reportedEvent{Direction: DirLink, At: now.Add(-8 * 24 * time.Hour)}, now, "gm-1", false},
		{"recent time kept", reportedEvent{Direction: DirLink, At: now.Add(-time.Minute)}, now.Add(-time.Minute), "gm-1", false},
		{"to Foundry names the Chronicle editor", reportedEvent{Direction: DirToFoundry, ResourceID: "e1", At: now}, now, "editor-1", false},
		{"unknown direction refused", reportedEvent{Direction: "up"}, time.Time{}, "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ev, err := h.toEvent(context.Background(), "c1", key, tc.in, now)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v", err)
			}
			if err != nil {
				return
			}
			if !ev.OccurredAt.Equal(tc.wantAt) || *ev.UserID != tc.wantWho || ev.ReportedBy != reportedByClient || !ev.OK {
				t.Fatalf("got at=%v who=%v by=%s ok=%v", ev.OccurredAt, *ev.UserID, ev.ReportedBy, ev.OK)
			}
		})
	}
	named, _ := h.toEvent(context.Background(), "c1", key, reportedEvent{Direction: DirToFoundry, Kind: "page", ResourceID: "e1"}, now)
	kept, _ := h.toEvent(context.Background(), "c1", key, reportedEvent{Direction: DirToFoundry, Kind: "page", ResourceID: "e1", Name: "Sent"}, now)
	if named.ResourceName != "Harbour" || kept.ResourceName != "Sent" {
		t.Fatalf("page naming: got %q and %q", named.ResourceName, kept.ResourceName)
	}
	long := strings.Repeat("é", 600)
	ev, _ := h.toEvent(context.Background(), "c1", key, reportedEvent{Direction: DirLink, Message: long, DurationMs: -5}, now)
	if len([]rune(ev.Message)) != 500 || ev.DurationMs != 0 {
		t.Fatalf("message not truncated or duration not clamped: %d runes, %d ms", len([]rune(ev.Message)), ev.DurationMs)
	}
}

// TestSyncHistory_OnlyOwnerOrDMAccess pins who may read or write the
// history over the API: the owner and DM-access members, never a player.
func TestSyncHistory_OnlyOwnerOrDMAccess(t *testing.T) {
	cases := []struct {
		name    string
		role    campaigns.Role
		granted bool
		want    int
	}{
		{"owner", campaigns.RoleOwner, false, http.StatusOK},
		{"player with DM access", campaigns.RolePlayer, true, http.StatusOK},
		{"scribe without DM access", campaigns.RoleScribe, false, http.StatusForbidden},
		{"player", campaigns.RolePlayer, false, http.StatusForbidden},
	}
	for _, tc := range cases {
		for _, method := range []string{http.MethodGet, http.MethodPost} {
			t.Run(tc.name+" "+method, func(t *testing.T) {
				repo := &fakeHistoryRepo{}
				h := NewSyncHistoryHandler(repo, &stubCampaignSvcForDmGrant{role: tc.role, granted: tc.granted}, nil, nil)
				e := echo.New()
				var body *strings.Reader
				if method == http.MethodPost {
					body = strings.NewReader(`{"events":[{"direction":"link","action":"connected"}]}`)
				} else {
					body = strings.NewReader("")
				}
				req := httptest.NewRequest(method, "/", body)
				req.Header.Set("Content-Type", "application/json")
				rec := httptest.NewRecorder()
				c := e.NewContext(req, rec)
				c.SetParamNames("id")
				c.SetParamValues("c1")
				c.Set(apiKeyContextKey, &APIKey{ID: 3, UserID: "u1", CampaignID: "c1", IsActive: true})
				handler := h.ListHistory
				if method == http.MethodPost {
					handler = h.ReportHistory
				}
				err := handler(c)
				got := rec.Code
				if err != nil {
					got = apperror.SafeCode(err)
				}
				if got != tc.want {
					t.Fatalf("status %d, want %d (err %v)", got, tc.want, err)
				}
				if tc.want == http.StatusForbidden && len(repo.inserted) > 0 {
					t.Fatal("a refused report was stored")
				}
			})
		}
	}
}

func TestReportHistory_StoresParentWithSteps(t *testing.T) {
	repo := &fakeHistoryRepo{}
	h := NewSyncHistoryHandler(repo, &stubCampaignSvcForDmGrant{role: campaigns.RoleOwner}, nil, nil)
	body := `{"events":[{"direction":"link","action":"catch-up","children":[
		{"direction":"to_foundry","name":"Ren","action":"page updated","ok":true},
		{"direction":"to_foundry","name":"Map","action":"page updated","ok":false,"message":"folder missing"}]}]}`
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("c1")
	c.Set(apiKeyContextKey, &APIKey{ID: 3, UserID: "u1", CampaignID: "c1"})
	if err := h.ReportHistory(c); err != nil {
		t.Fatal(err)
	}
	var out map[string]int
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if out["stored"] != 1 || len(repo.inserted) != 1 || len(repo.inserted[0].Children) != 2 || repo.inserted[0].Children[1].OK {
		t.Fatalf("unexpected store: %+v / %+v", out, repo.inserted)
	}
}

// TestRecordSyncHistory covers the recorder end to end on a real route:
// a key's write is recorded with its result, a session (Chronicle's own
// pages) is not.
func TestRecordSyncHistory(t *testing.T) {
	cases := []struct {
		name      string
		key       *APIKey
		handler   echo.HandlerFunc
		want      bool
		wantOK    bool
		wantState string
	}{
		{"key write recorded", &APIKey{ID: 4, UserID: "u1", CampaignID: "c1"}, func(c echo.Context) error {
			noteSyncResource(c, "e1", "Ren")
			return c.NoContent(http.StatusOK)
		}, true, true, "200"},
		{"failed write recorded with its error", &APIKey{ID: 4, UserID: "u1", CampaignID: "c1"}, func(c echo.Context) error {
			return apperror.NewConflict("page changed since you read it")
		}, true, false, "409"},
		{"session call skipped", &APIKey{ID: synthKeySessionID, UserID: "u1", CampaignID: "c1"}, func(c echo.Context) error {
			return c.NoContent(http.StatusOK)
		}, false, false, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeHistoryRepo{done: make(chan struct{}, 1)}
			e := echo.New()
			e.PUT("/api/v1/campaigns/:id/entities/:entityID", tc.handler, func(next echo.HandlerFunc) echo.HandlerFunc {
				return func(c echo.Context) error { c.Set(apiKeyContextKey, tc.key); return next(c) }
			}, RecordSyncHistory(repo, nil))
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, httptest.NewRequest(http.MethodPut, "/api/v1/campaigns/c1/entities/e1", nil))
			if !tc.want {
				select {
				case <-repo.done:
					t.Fatal("session call was recorded")
				case <-time.After(50 * time.Millisecond):
				}
				return
			}
			select {
			case <-repo.done:
			case <-time.After(2 * time.Second):
				t.Fatal("nothing recorded")
			}
			ev := repo.inserted[0]
			if ev.OK != tc.wantOK || ev.Status != tc.wantState || ev.Direction != DirToChronicle || ev.Call != "PUT /entities/:entityID" || ev.ResourceID != "e1" {
				t.Fatalf("unexpected event %+v", ev)
			}
			if !tc.wantOK && ev.Message == "" {
				t.Fatal("failure has no message")
			}
		})
	}
}
