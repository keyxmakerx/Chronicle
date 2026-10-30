package foundry_vtt

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/packages"
)

// holdRecorder is the update service's admin hold, recording each campaign held.
type holdRecorder struct {
	fakeUpdateService
	held []string
}

func (f *holdRecorder) SetAdminHold(_ context.Context, campaignID, _ string, hold bool, a packages.ActorInfo) (*packages.CampaignPackageState, error) {
	if hold && a.Admin {
		f.held = append(f.held, campaignID)
	}
	return nil, nil
}

type forcePinSvc struct {
	bindingSvc
	older []string
}

func (s *forcePinSvc) ForcePinAllToVersion(context.Context, string, string, string, string) ([]string, error) {
	return s.older, nil
}

// TestForcePinSetsAdminHold pins that a force-update, of one campaign or of
// every older one, leaves the admin hold on, so the owner can't move straight
// back to the version the admin moved them off.
func TestForcePinSetsAdminHold(t *testing.T) {
	svc := &forcePinSvc{bindingSvc: bindingSvc{pinned: map[string]string{}}, older: []string{"c2", "c3"}}
	upd := &holdRecorder{}
	rd := &stubPackageReader{pkgs: []packages.Package{{
		ID: "m1", Type: packages.PackageTypeFoundryModule, InstalledVersion: "2.9.0", Status: packages.StatusApproved,
	}}}
	h := NewHandler(svc)
	h.SetOwnerUpdates(NewOwnerUpdates(upd, rd))

	run := func(path string, names, values []string, fn func(echo.Context) error) {
		t.Helper()
		rec := httptest.NewRecorder()
		c := echo.New().NewContext(httptest.NewRequest(http.MethodPost, path, nil), rec)
		c.SetParamNames(names...)
		c.SetParamValues(values...)
		auth.SetSession(c, &auth.Session{UserID: "admin", IsAdmin: true})
		if err := fn(c); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
	}
	run("/force-pin", []string{"version", "cid"}, []string{"2.9.0", "c1"}, h.AdminForcePinCampaignHandler)
	run("/force-pin-older", []string{"version"}, []string{"2.9.0"}, h.AdminForcePinOlderHandler)

	want := []string{"c1", "c2", "c3"}
	if len(upd.held) != len(want) {
		t.Fatalf("held %v, want %v", upd.held, want)
	}
	for i := range want {
		if upd.held[i] != want[i] {
			t.Fatalf("held %v, want %v", upd.held, want)
		}
	}
}
