// entity_gate_test.go pins that every post write route (create, update,
// delete, reorder) binds :eid to the URL campaign and the caller's view
// access before the service is reached.
package posts

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

type writeRecorderService struct {
	PostService
	post  *Post
	calls int
}

func (s *writeRecorderService) Create(context.Context, string, string, string, string, CreatePostRequest) (*Post, error) {
	s.calls++
	return &Post{ID: "new"}, nil
}
func (s *writeRecorderService) GetByID(context.Context, string) (*Post, error) { return s.post, nil }
func (s *writeRecorderService) Update(context.Context, string, UpdatePostRequest) (*Post, error) {
	s.calls++
	return &Post{ID: "p1"}, nil
}
func (s *writeRecorderService) Delete(context.Context, string) error { s.calls++; return nil }
func (s *writeRecorderService) Reorder(context.Context, string, string, []string) error {
	s.calls++
	return nil
}

func TestPostWrites_EntityGate(t *testing.T) {
	gate := fakeEntityGate{
		campaignOf: map[string]string{"ok": "camp-1", "priv": "camp-1", "foreign": "camp-2", "other": "camp-1"},
		canView:    map[string]bool{"ok": true, "priv": false, "foreign": true, "other": true},
	}
	ownPost := &Post{ID: "p1", EntityID: "ok", CampaignID: "camp-1"}
	tests := []struct {
		name      string
		eid       string
		post      *Post
		call      func(h *Handler, c echo.Context) error
		wantErr   bool
		wantCalls int
	}{
		{"create ok", "ok", nil, (*Handler).CreatePost, false, 1},
		{"create foreign entity", "foreign", nil, (*Handler).CreatePost, true, 0},
		{"create hidden entity", "priv", nil, (*Handler).CreatePost, true, 0},
		{"reorder ok", "ok", nil, (*Handler).ReorderPosts, false, 1},
		{"reorder foreign entity", "foreign", nil, (*Handler).ReorderPosts, true, 0},
		{"reorder hidden entity", "priv", nil, (*Handler).ReorderPosts, true, 0},
		{"update ok", "ok", ownPost, (*Handler).UpdatePost, false, 1},
		{"update hidden entity", "priv", &Post{ID: "p1", EntityID: "priv", CampaignID: "camp-1"}, (*Handler).UpdatePost, true, 0},
		{"update post of another entity", "ok", &Post{ID: "p1", EntityID: "other", CampaignID: "camp-1"}, (*Handler).UpdatePost, true, 0},
		{"delete ok", "ok", ownPost, (*Handler).DeletePost, false, 1},
		{"delete hidden entity", "priv", &Post{ID: "p1", EntityID: "priv", CampaignID: "camp-1"}, (*Handler).DeletePost, true, 0},
		{"delete post of another entity", "ok", &Post{ID: "p1", EntityID: "other", CampaignID: "camp-1"}, (*Handler).DeletePost, true, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &writeRecorderService{post: tt.post}
			h := NewHandler(svc)
			h.SetEntityGate(gate)
			e := echo.New()
			req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"x","postIds":["p1"]}`))
			req.Header.Set("Content-Type", "application/json")
			c := e.NewContext(req, httptest.NewRecorder())
			c.SetParamNames("id", "eid", "pid")
			c.SetParamValues("camp-1", tt.eid, "p1")
			c.Set("campaign_context", &campaigns.CampaignContext{
				Campaign:   &campaigns.Campaign{ID: "camp-1"},
				MemberRole: campaigns.RoleScribe,
				IsMember:   true,
			})
			auth.SetSession(c, &auth.Session{UserID: "u1"})

			err := tt.call(h, c)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				if _, ok := err.(*apperror.AppError); !ok {
					t.Errorf("err type = %T, want *apperror.AppError", err)
				}
			}
			if svc.calls != tt.wantCalls {
				t.Errorf("mutating service calls = %d, want %d", svc.calls, tt.wantCalls)
			}
		})
	}
}
