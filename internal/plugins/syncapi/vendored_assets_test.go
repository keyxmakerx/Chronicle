package syncapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	stderrors "errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"github.com/labstack/echo/v5"
	"golang.org/x/crypto/bcrypt"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

func sha12(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])[:12]
}

func TestHashVendoredAssets(t *testing.T) {
	sky := []byte("sky engine bytes")
	shop := []byte("shop bytes")
	disk := fstest.MapFS{
		"js/widgets/sky_gl.js": {Data: sky},
		"js/widgets/shop.js":   {Data: shop},
	}
	tests := []struct {
		name  string
		files []VendoredAssetFile
		want  map[string]string
	}{
		{"hash is the sha256 prefix of the bytes",
			[]VendoredAssetFile{{Name: "sky_gl.js", FS: disk, Path: "js/widgets/sky_gl.js"}},
			map[string]string{"sky_gl.js": sha12(sky)}},
		{"two files from one FS",
			[]VendoredAssetFile{{Name: "sky_gl.js", FS: disk, Path: "js/widgets/sky_gl.js"}, {Name: "shop.js", FS: disk, Path: "js/widgets/shop.js"}},
			map[string]string{"sky_gl.js": sha12(sky), "shop.js": sha12(shop)}},
		{"a missing file is left out",
			[]VendoredAssetFile{{Name: "sky_gl.js", FS: disk, Path: "js/widgets/sky_gl.js"}, {Name: "gone.js", FS: disk, Path: "js/widgets/gone.js"}},
			map[string]string{"sky_gl.js": sha12(sky)}},
		{"a file with no served FS is left out",
			[]VendoredAssetFile{{Name: "orphan.css", Path: "css/orphan.css"}},
			map[string]string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := HashVendoredAssets(tt.files)
			if len(got) != len(tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
			for k, v := range tt.want {
				if got[k] != v {
					t.Errorf("%s = %q, want %q", k, got[k], v)
				}
			}
		})
	}
}

func TestVendoredAssetsHandler_SetAssetsCopies(t *testing.T) {
	h := NewVendoredAssetsHandler()
	in := map[string]string{"sky_gl.js": "0f2322e880a5"}
	h.SetAssets(in)
	in["sky_gl.js"] = "tampered"
	if got := (*h.assets.Load())["sky_gl.js"]; got != "0f2322e880a5" {
		t.Fatalf("served %q after the caller changed its map", got)
	}
}

// newVendoredRouteFixture drives RegisterAPIRoutes so the test covers the
// real chain (key, campaign match, sync-API add-on gate, permission).
func newVendoredRouteFixture(t *testing.T, perms []APIKeyPermission, addons slugChecker, h *VendoredAssetsHandler) (*echo.Echo, string) {
	t.Helper()
	rawKey := "chron_vend00123456789012345678901234567890123456789012345678901"
	hash, err := bcrypt.GenerateFromPassword([]byte(rawKey), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	repo := &mockSyncAPIRepo{
		findKeyByPrefixFn: func(_ context.Context, prefix string) (*APIKey, error) {
			if prefix != rawKey[:keyPrefixLen] {
				return nil, apperror.NewNotFound("key not found")
			}
			return &APIKey{ID: 9, KeyHash: string(hash), KeyPrefix: prefix, CampaignID: "camp-1",
				UserID: "gm-user", Permissions: perms, RateLimit: 60, IsActive: true}, nil
		},
		isIPBlockedFn:      func(context.Context, string) (bool, error) { return false, nil },
		logRequestFn:       func(context.Context, *APIRequestLog) error { return nil },
		logSecurityEventFn: func(context.Context, *SecurityEvent) error { return nil },
	}
	syncSvc := NewSyncAPIService(repo)
	authSvc := &fakeAuthService{validateSessionFn: func(context.Context, string) (*auth.Session, error) {
		return nil, stderrors.New("no session")
	}}
	campSvc := &fakeCampaignService{getMemberFn: func(_ context.Context, cid, uid string) (*campaigns.CampaignMember, error) {
		return &campaigns.CampaignMember{CampaignID: cid, UserID: uid, Role: campaigns.RoleOwner}, nil
	}, getByIDFn: func(_ context.Context, id string) (*campaigns.Campaign, error) {
		return &campaigns.Campaign{ID: id}, nil
	}}
	e := echo.New()
	e.HTTPErrorHandler = func(c *echo.Context, err error) {
		var ae *apperror.AppError
		if stderrors.As(err, &ae) {
			_ = c.JSON(ae.Code, map[string]string{"error": ae.Type})
			return
		}
		var he *echo.HTTPError
		if stderrors.As(err, &he) {
			_ = c.JSON(he.Code, map[string]string{"error": "http"})
			return
		}
		_ = c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal"})
	}
	api := NewAPIHandler(syncSvc, nil, campSvc, nil)
	RegisterAPIRoutes(e, api, nil, nil, nil, nil, nil, nil, nil, nil, syncSvc, addons, authSvc, campSvc, WithVendoredAssets(h))
	return e, rawKey
}

func TestVendoredAssetsRoute(t *testing.T) {
	assets := map[string]string{"sky_gl.js": "0f2322e880a5", "spoils.css": "eed01b5e6662"}
	syncOn := slugChecker{SyncAPIAddonSlug: true}
	tests := []struct {
		name     string
		perms    []APIKeyPermission
		addons   slugChecker
		campaign string
		bearer   bool
		wantCode int
	}{
		{"read key for the campaign", []APIKeyPermission{PermRead}, syncOn, "camp-1", true, http.StatusOK},
		{"key without read is refused", []APIKeyPermission{PermWrite, PermSync}, syncOn, "camp-1", true, http.StatusForbidden},
		{"key for another campaign is refused", []APIKeyPermission{PermRead}, syncOn, "camp-2", true, http.StatusForbidden},
		{"sync API switched off is refused", []APIKeyPermission{PermRead}, slugChecker{}, "camp-1", true, http.StatusForbidden},
		{"no key is refused", []APIKeyPermission{PermRead}, syncOn, "camp-1", false, http.StatusUnauthorized},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewVendoredAssetsHandler()
			h.SetAssets(assets)
			e, rawKey := newVendoredRouteFixture(t, tt.perms, tt.addons, h)
			req := httptest.NewRequest(http.MethodGet, "/api/v1/campaigns/"+tt.campaign+"/vendored-assets", nil)
			if tt.bearer {
				req.Header.Set("Authorization", "Bearer "+rawKey)
			}
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)
			if rec.Code != tt.wantCode {
				t.Fatalf("status %d, want %d (body %s)", rec.Code, tt.wantCode, rec.Body.String())
			}
			if tt.wantCode != http.StatusOK {
				if containsIgnoreCase(rec.Body.String(), "0f2322e880a5") {
					t.Fatal("refused response carried a hash")
				}
				return
			}
			// The wire shape is exactly {"assets": {name: hash}}.
			var body map[string]map[string]string
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode %s: %v", rec.Body.String(), err)
			}
			if len(body) != 1 || len(body["assets"]) != 2 || body["assets"]["sky_gl.js"] != "0f2322e880a5" || body["assets"]["spoils.css"] != "eed01b5e6662" {
				t.Fatalf("body = %s", rec.Body.String())
			}
		})
	}
}

func TestVendoredAssetsRoute_EmptyIsObject(t *testing.T) {
	e, rawKey := newVendoredRouteFixture(t, []APIKeyPermission{PermRead}, slugChecker{SyncAPIAddonSlug: true}, NewVendoredAssetsHandler())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/campaigns/camp-1/vendored-assets", nil)
	req.Header.Set("Authorization", "Bearer "+rawKey)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || rec.Body.String() != "{\"assets\":{}}\n" {
		t.Fatalf("status %d body %q", rec.Code, rec.Body.String())
	}
}
