package wire

import (
	"strings"
	"testing"
)

// These pin the pure helpers behind the wire-contract snapshot, so a change to
// name normalisation or auth classification is caught here rather than as a
// confusing wholesale snapshot diff.

func TestNormaliseFuncName(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"plugin function", "github.com/keyxmakerx/chronicle/internal/plugins/auth.RequireAuth", "plugins/auth.RequireAuth"},
		{"constructor closure", "github.com/keyxmakerx/chronicle/internal/plugins/auth.RequireAuth.func1", "plugins/auth.RequireAuth"},
		{"nested closure", "github.com/keyxmakerx/chronicle/internal/plugins/auth.RequireAuth.func1.2", "plugins/auth.RequireAuth"},
		{"method value", "github.com/keyxmakerx/chronicle/internal/plugins/auth.(*Handler).Login-fm", "plugins/auth.(*Handler).Login"},
		{"closure in method", "github.com/keyxmakerx/chronicle/internal/app.(*App).RegisterRoutes.func42", "app.(*App).RegisterRoutes"},
		{"third party keeps its path", "github.com/labstack/echo/v4/middleware.BodyLimitWithConfig.func1", "github.com/labstack/echo/v4/middleware.BodyLimitWithConfig"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normaliseFuncName(tt.in); got != tt.want {
				t.Errorf("normaliseFuncName(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestAuthClass(t *testing.T) {
	tests := []struct {
		name  string
		chain []string
		want  string
	}{
		{"empty chain", nil, authClassNone},
		{"rate limit only", []string{"middleware.RateLimit"}, authClassNone},
		{"session", []string{"plugins/auth.RequireAuth", "plugins/campaigns.RequireCampaignAccess"}, "session"},
		{"optional session", []string{"plugins/auth.OptionalAuth"}, "optional-session"},
		{"public capable beats optional", []string{"plugins/auth.OptionalAuth", "plugins/campaigns.AllowPublicCampaignAccess"}, "public-capable"},
		{"session or bearer", []string{"plugins/syncapi.RequireAuthOrAPIKey", "plugins/syncapi.RateLimit"}, "session-or-bearer"},
		{"site admin beats session", []string{"plugins/auth.RequireAuth", "plugins/auth.RequireSiteAdmin"}, "site-admin"},
		{"order in chain does not matter", []string{"plugins/auth.RequireSiteAdmin", "plugins/auth.RequireAuth"}, "site-admin"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := authClass(tt.chain); got != tt.want {
				t.Errorf("authClass(%v) = %q, want %q", tt.chain, got, tt.want)
			}
		})
	}
}

func TestRouteRecordString(t *testing.T) {
	tests := []struct {
		name string
		in   routeRecord
		want string
	}{
		{
			"no middleware",
			routeRecord{Method: "GET", Path: "/health", Handler: "app.RegisterRoutes"},
			"GET\t/health\tnone\tapp.RegisterRoutes\t-",
		},
		{
			"chain keeps execution order",
			routeRecord{
				Method:  "POST",
				Path:    "/campaigns/:id/x",
				Handler: "plugins/x.(*Handler).Do",
				Chain:   []string{"plugins/auth.RequireAuth", "plugins/campaigns.RequireRole"},
			},
			"POST\t/campaigns/:id/x\tsession\tplugins/x.(*Handler).Do\tplugins/auth.RequireAuth > plugins/campaigns.RequireRole",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.in.String(); got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestDescribeDrift pins that a stripped or added middleware is reported by
// name against its route, and that an unchanged snapshot reports nothing.
func TestDescribeDrift(t *testing.T) {
	line := func(method, path, handler string, chain ...string) string {
		return routeRecord{Method: method, Path: path, Handler: handler, Chain: chain}.String()
	}
	base := []string{
		line("GET", "/a", "h.A", "plugins/auth.RequireAuth", "middleware.RateLimit"),
		line("GET", "/b", "h.B"),
	}

	tests := []struct {
		name         string
		got          []string
		wantContains []string
		wantEmpty    bool
	}{
		{"identical", base, nil, true},
		{
			"middleware removed",
			[]string{line("GET", "/a", "h.A", "middleware.RateLimit"), base[1]},
			[]string{"middleware removed: plugins/auth.RequireAuth", "auth class: session -> none", "~ GET /a #1"},
			false,
		},
		{
			"middleware added",
			[]string{base[0], line("GET", "/b", "h.B", "middleware.RateLimit")},
			[]string{"middleware added:   middleware.RateLimit", "~ GET /b #1"},
			false,
		},
		{
			"handler repointed",
			[]string{line("GET", "/a", "h.Other", "plugins/auth.RequireAuth", "middleware.RateLimit"), base[1]},
			[]string{"handler:    h.A -> h.Other"},
			false,
		},
		{
			"order changed",
			[]string{line("GET", "/a", "h.A", "middleware.RateLimit", "plugins/auth.RequireAuth"), base[1]},
			[]string{"middleware order or repeat count changed"},
			false,
		},
		{
			"route added",
			append(append([]string{}, base...), line("POST", "/c", "h.C")),
			[]string{"Routes added since snapshot (1)", "+ POST\t/c"},
			false,
		},
		{
			"route removed",
			base[:1],
			[]string{"Routes removed since snapshot (1)", "- GET\t/b"},
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// The message always opens with the headline; compare the body.
			got := describeDrift(base, tt.got)
			body := strings.TrimPrefix(got, "wire-contract drift detected.\n\n")
			if tt.wantEmpty {
				if body != "" {
					t.Fatalf("expected no drift, got:\n%s", got)
				}
				return
			}
			for _, want := range tt.wantContains {
				if !strings.Contains(got, want) {
					t.Errorf("drift report missing %q; got:\n%s", want, got)
				}
			}
		})
	}
}
