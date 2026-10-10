package maps

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/patch"
	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// gateRepo records whether a drawing write reached persistence. It embeds the
// IDOR fixture's repo for the rest of the interface; its drawings live on
// "map-1" so the gate, not the IDOR guard, decides these cases.
type gateRepo struct {
	idorRepo
	created bool
	updated bool
}

// gateAuthor made every drawing gateRepo serves, so the ownership rule passes
// and the draw gate alone decides.
const gateAuthor = "u-gate"

func (r *gateRepo) GetDrawing(_ context.Context, id string) (*Drawing, error) {
	author := gateAuthor
	return &Drawing{ID: id, MapID: "map-1", CreatedBy: &author}, nil
}

func (r *gateRepo) CreateDrawing(context.Context, *Drawing) error { r.created = true; return nil }
func (r *gateRepo) UpdateDrawing(context.Context, *Drawing) error { r.updated = true; return nil }

func gateService(repo *gateRepo, policy func(context.Context, string) (string, error)) DrawingService {
	s := NewDrawingService(repo)
	if policy != nil {
		s.SetDrawPolicyLookup(policy)
	}
	return s
}

func fixedPolicy(who string) func(context.Context, string) (string, error) {
	return func(context.Context, string) (string, error) { return who, nil }
}

// The server-side "who can draw" rule, for create and update, across every
// role. This is the enforcement the UI only reflects: a scribe who POSTs by
// hand to an owners-only map must be refused.
func TestDrawingWrites_EnforceWhoCanDraw(t *testing.T) {
	cases := []struct {
		name   string
		policy func(context.Context, string) (string, error)
		role   int
		allow  bool
	}{
		{"owners-only: owner may draw", fixedPolicy(DrawWhoOwners), permissions.RoleOwner, true},
		{"owners-only: scribe is refused", fixedPolicy(DrawWhoOwners), permissions.RoleScribe, false},
		{"owners-only: player is refused", fixedPolicy(DrawWhoOwners), permissions.RolePlayer, false},
		{"owners-only: no role is refused", fixedPolicy(DrawWhoOwners), permissions.RoleNone, false},
		{"scribes: owner may draw", fixedPolicy(DrawWhoScribes), permissions.RoleOwner, true},
		{"scribes: scribe may draw (today's behaviour)", fixedPolicy(DrawWhoScribes), permissions.RoleScribe, true},
		{"scribes: player is refused", fixedPolicy(DrawWhoScribes), permissions.RolePlayer, false},
		{"scribes: no role is refused", fixedPolicy(DrawWhoScribes), permissions.RoleNone, false},
		{"unwired policy defaults to scribes: scribe may draw", nil, permissions.RoleScribe, true},
		{"unwired policy: no role is still refused", nil, permissions.RoleNone, false},
		{
			"a failing policy lookup fails closed, even for an owner",
			func(context.Context, string) (string, error) { return "", errors.New("db down") },
			permissions.RoleOwner, false,
		},
	}
	for _, tc := range cases {
		t.Run("create/"+tc.name, func(t *testing.T) {
			repo := &gateRepo{}
			_, err := gateService(repo, tc.policy).CreateDrawing(context.Background(), CreateDrawingInput{
				MapID: "map-1", DrawingType: "freehand", Points: json.RawMessage(`[[1,1],[2,2]]`),
				CallerRole: tc.role,
			})
			assertGate(t, err, tc.allow, repo.created)
		})
		t.Run("update/"+tc.name, func(t *testing.T) {
			repo := &gateRepo{}
			err := gateService(repo, tc.policy).UpdateDrawing(context.Background(), "d-1", "map-1", gateAuthor, tc.role, tc.role >= permissions.RoleOwner,
				UpdateDrawingInput{StrokeColor: patch.Of("#ff0000")})
			assertGate(t, err, tc.allow, repo.updated)
		})
	}
}

func assertGate(t *testing.T, err error, allow, wrote bool) {
	t.Helper()
	if allow {
		if err != nil {
			t.Fatalf("want allowed, got %v", err)
		}
		if !wrote {
			t.Error("allowed, but nothing was written")
		}
		return
	}
	if err == nil {
		t.Fatal("want a refusal, got nil")
	}
	if wrote {
		t.Error("refused, but the write reached persistence")
	}
}

// The gate asks the policy for the map in the URL, so a drawing id from another
// map cannot be used to dodge a different map's rule.
func TestDrawingGate_UsesTheRequestedMap(t *testing.T) {
	var asked string
	policy := func(_ context.Context, mapID string) (string, error) { asked = mapID; return DrawWhoScribes, nil }
	repo := &gateRepo{}
	_, err := gateService(repo, policy).CreateDrawing(context.Background(), CreateDrawingInput{
		MapID: "map-7", DrawingType: "freehand", Points: json.RawMessage(`[[1,1]]`), CallerRole: permissions.RoleScribe,
	})
	if err != nil {
		t.Fatal(err)
	}
	if asked != "map-7" {
		t.Errorf("policy asked about %q, want map-7", asked)
	}
}

// The web handlers pass the caller's campaign role into the service, so the
// rule holds on the real routes and not only in the service unit tests. A
// scribe POSTing or PUTting by hand to an owners-only map gets a 403 and
// nothing is written.
func TestDrawingHandlers_EnforceWhoCanDraw(t *testing.T) {
	ownersOnly := &Map{ID: "map-1", CampaignID: "camp-1", Display: &DisplaySettings{Draw: &DrawDisplay{Who: DrawWhoOwners}}}
	scribesToo := &Map{ID: "map-1", CampaignID: "camp-1"}
	cases := []struct {
		name   string
		m      *Map
		role   campaigns.Role
		status int
	}{
		{"owners-only, owner", ownersOnly, campaigns.RoleOwner, http.StatusOK},
		{"owners-only, scribe", ownersOnly, campaigns.RoleScribe, http.StatusForbidden},
		{"default, scribe", scribesToo, campaigns.RoleScribe, http.StatusOK},
	}
	for _, tc := range cases {
		for _, verb := range []string{"create", "update"} {
			t.Run(verb+"/"+tc.name, func(t *testing.T) {
				mapSvc := NewMapService(&mockMapRepo{getMapFn: func(context.Context, string) (*Map, error) { return tc.m, nil }})
				repo := &gateRepo{}
				drawSvc := NewDrawingService(repo)
				drawSvc.SetDrawPolicyLookup(func(ctx context.Context, id string) (string, error) {
					m, err := mapSvc.GetMap(ctx, id)
					if err != nil {
						return "", err
					}
					return m.DrawWho(), nil
				})
				h := NewDrawingHandler(mapSvc, drawSvc)

				e := echo.New()
				var req *http.Request
				if verb == "create" {
					req = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"drawing_type":"freehand","points":[[1,1],[2,2]]}`))
				} else {
					req = httptest.NewRequest(http.MethodPut, "/", strings.NewReader(`{"stroke_color":"#ff0000"}`))
				}
				req.Header.Set("Content-Type", "application/json")
				rec := httptest.NewRecorder()
				c := e.NewContext(req, rec)
				c.SetParamNames("id", "mid", "did")
				c.SetParamValues("camp-1", "map-1", "d-1")
				c.Set("campaign_context", dmWriteCampaignCtx(tc.role, false))
				auth.SetSession(c, &auth.Session{UserID: gateAuthor})

				var err error
				if verb == "create" {
					err = h.CreateDrawing(c)
				} else {
					err = h.UpdateDrawing(c)
				}
				got := http.StatusOK
				if err != nil {
					var ae *apperror.AppError
					if !errors.As(err, &ae) {
						t.Fatalf("unexpected error: %v", err)
					}
					got = ae.Code
				}
				if got != tc.status {
					t.Errorf("status = %d, want %d", got, tc.status)
				}
				if tc.status == http.StatusForbidden && (repo.created || repo.updated) {
					t.Error("refused, but a write reached persistence")
				}
			})
		}
	}
}
