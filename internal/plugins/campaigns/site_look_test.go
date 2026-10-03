package campaigns

import (
	"context"
	"errors"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/sitelook"
)

// fakeSiteLook is a SiteLookSource returning a fixed look or error.
type fakeSiteLook struct {
	look sitelook.Settings
	err  error
}

func (f fakeSiteLook) GetSiteLook(context.Context) (sitelook.Settings, error) { return f.look, f.err }

// TestSiteLookIDsMatchCampaignLooks keeps the site's look table and the
// campaign Customize looks in step, so a look offered for the site is one a
// campaign can start from.
func TestSiteLookIDsMatchCampaignLooks(t *testing.T) {
	if len(sitelook.Looks) != len(AppearanceLooks) {
		t.Fatalf("sitelook has %d looks, campaigns has %d", len(sitelook.Looks), len(AppearanceLooks))
	}
	for i, l := range sitelook.Looks {
		if l.ID != AppearanceLooks[i] {
			t.Errorf("look %d: sitelook %q, campaigns %q", i, l.ID, AppearanceLooks[i])
		}
	}
}

func TestCreate_StartsWithSiteLook(t *testing.T) {
	tests := []struct {
		name     string
		source   SiteLookSource
		wantLook string // Appearance.Look of the new campaign's settings; "" for none.
		wantJSON string // Exact settings, when it is the empty default.
	}{
		{"no source wired", nil, "", "{}"},
		{"never saved", fakeSiteLook{}, "", "{}"},
		{"look left as it is", fakeSiteLook{look: sitelook.Settings{Configured: true}}, "", "{}"},
		{"ember", fakeSiteLook{look: sitelook.Settings{Configured: true, Look: "ember"}}, "ember", ""},
		{"parchment", fakeSiteLook{look: sitelook.Settings{Configured: true, Look: "parchment"}}, "parchment", ""},
		{"classic is the default so only the accent defaults are stored", fakeSiteLook{look: sitelook.Settings{Configured: true, Look: "classic"}}, "", "{}"},
		{"unknown stored look ignored", fakeSiteLook{look: sitelook.Settings{Look: "neon"}}, "", "{}"},
		{"read failure does not block creation", fakeSiteLook{err: errors.New("db down")}, "", "{}"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var created *Campaign
			repo := &mockCampaignRepo{createFn: func(_ context.Context, c *Campaign) error { created = c; return nil }}
			svc := newTestCampaignServiceFull(repo, &mockUserFinder{}, nil, &mockSeeder{})
			if tc.source != nil {
				svc.SetSiteLookSource(tc.source)
			}
			if _, err := svc.Create(context.Background(), "user-1", CreateCampaignInput{Name: "Test"}); err != nil {
				t.Fatalf("Create: %v", err)
			}
			if created == nil {
				t.Fatal("repo.Create not called")
			}
			if tc.wantJSON != "" && created.Settings != tc.wantJSON {
				t.Errorf("settings = %s, want %s", created.Settings, tc.wantJSON)
			}
			var got string
			s := created.ParseSettings()
			if s.Appearance != nil {
				got = s.Appearance.Look
			}
			if got != tc.wantLook {
				t.Errorf("appearance look = %q, want %q", got, tc.wantLook)
			}
			if tc.wantLook == "ember" {
				// The full look, so the campaign renders in it at once.
				if s.AccentColor != "#c2410c" || s.TopbarStyle == nil || s.TopbarStyle.Mode != "gradient" ||
					s.Appearance.HeadingFont != "cinzel" || s.Appearance.ButtonStyle != "press" || s.Appearance.Elevation != "dramatic" {
					t.Errorf("ember not fully seeded: accent=%q topbar=%+v appearance=%+v", s.AccentColor, s.TopbarStyle, s.Appearance)
				}
			}
		})
	}
}
