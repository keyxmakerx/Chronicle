package settings

import (
	"context"
	"errors"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/sitelook"
)

func TestGetSiteLook(t *testing.T) {
	tests := []struct {
		name string
		rows map[string]string
		want sitelook.Settings
	}{
		{"never saved renders as shipped", map[string]string{}, sitelook.Settings{LogoAsFavicon: true}},
		{"saved defaults", map[string]string{KeySiteName: "", KeySiteLogoFavicon: "1"},
			sitelook.Settings{Configured: true, Background: sitelook.BackgroundPlain, LogoAsFavicon: true}},
		{"favicon off", map[string]string{KeySiteName: "Hold", KeySiteLogoFavicon: "0"},
			sitelook.Settings{Configured: true, Name: "Hold", Background: sitelook.BackgroundPlain}},
		{"full", map[string]string{
			KeySiteName: "Hold", KeySiteLogo: "2026/10/a.png", KeySiteLogoFavicon: "1", KeySiteLook: "forest",
			KeySiteSigninBackground: "picture", KeySiteSigninPicture: "2026/10/b.jpg", KeySiteWelcome: "Hi",
		}, sitelook.Settings{Configured: true, Name: "Hold", Logo: "2026/10/a.png", LogoAsFavicon: true, Look: "forest",
			Background: sitelook.BackgroundPicture, Picture: "2026/10/b.jpg", Welcome: "Hi"}},
		{"move on", map[string]string{KeySiteName: "Hold", KeySiteLook: "ember", KeySiteSigninBackground: "look", KeySiteMove: "1", KeySiteLogoFavicon: "1"},
			sitelook.Settings{Configured: true, Name: "Hold", Look: "ember", Background: sitelook.BackgroundLook, Move: true, LogoAsFavicon: true}},
		{"move absent is off", map[string]string{KeySiteName: "Hold", KeySiteLook: "ember", KeySiteSigninBackground: "look", KeySiteLogoFavicon: "1"},
			sitelook.Settings{Configured: true, Name: "Hold", Look: "ember", Background: sitelook.BackgroundLook, LogoAsFavicon: true}},
		{"stored move over a plain background is ignored", map[string]string{KeySiteName: "Hold", KeySiteMove: "1", KeySiteLogoFavicon: "1"},
			sitelook.Settings{Configured: true, Name: "Hold", Background: sitelook.BackgroundPlain, LogoAsFavicon: true}},
		{"hand-edited bad value falls back to shipped", map[string]string{KeySiteName: "Hold", KeySiteLook: "neon"},
			sitelook.Settings{}},
		{"hand-edited svg logo falls back to shipped", map[string]string{KeySiteName: "Hold", KeySiteLogo: "2026/10/a.svg"},
			sitelook.Settings{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := NewSettingsService(&mockSettingsRepo{getAllFn: func(context.Context) (map[string]string, error) { return tc.rows, nil }})
			got, err := svc.GetSiteLook(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Errorf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestGetSiteLook_ReadErrorAndCache(t *testing.T) {
	reads := 0
	rows := map[string]string{KeySiteName: "One"}
	var fail error
	repo := &mockSettingsRepo{getAllFn: func(context.Context) (map[string]string, error) {
		reads++
		return rows, fail
	}}
	svc := NewSettingsService(repo)
	ctx := context.Background()

	if _, err := svc.GetSiteLook(ctx); err != nil {
		t.Fatal(err)
	}
	rows[KeySiteName] = "Two"
	got, _ := svc.GetSiteLook(ctx)
	if reads != 1 || got.Name != "One" {
		t.Errorf("second read should come from the cache: reads=%d name=%q", reads, got.Name)
	}

	// A save drops the cache so the admin sees their change at once.
	if _, err := svc.UpdateSiteLook(ctx, sitelook.Settings{Name: "Three"}); err != nil {
		t.Fatal(err)
	}
	rows[KeySiteName] = "Three"
	if got, _ = svc.GetSiteLook(ctx); got.Name != "Three" || reads != 2 {
		t.Errorf("after save: name=%q reads=%d", got.Name, reads)
	}

	// A read error is returned, not cached as an empty look.
	svc2 := NewSettingsService(&mockSettingsRepo{getAllFn: func(context.Context) (map[string]string, error) { return nil, errors.New("db down") }})
	if _, err := svc2.GetSiteLook(ctx); err == nil {
		t.Error("expected the read error")
	}
}

func TestUpdateSiteLook(t *testing.T) {
	tests := []struct {
		name     string
		in       sitelook.Settings
		wantErr  bool
		wantRows map[string]string
		wantLast string
	}{
		{"rejects a bad look and writes nothing", sitelook.Settings{Look: "neon"}, true, nil, ""},
		{"rejects an svg logo and writes nothing", sitelook.Settings{Logo: "2026/10/a.svg"}, true, nil, ""},
		{"stores every key, name last", sitelook.Settings{Name: "Hold", Look: "ember", Background: "look", Welcome: "Hi", LogoAsFavicon: true, Logo: "2026/10/a.png"}, false,
			map[string]string{KeySiteName: "Hold", KeySiteLogo: "2026/10/a.png", KeySiteLogoFavicon: "1", KeySiteLook: "ember",
				KeySiteSigninBackground: "look", KeySiteSigninPicture: "", KeySiteWelcome: "Hi", KeySiteMove: "0"}, KeySiteName},
		{"move is stored when on", sitelook.Settings{Look: "ember", Background: "look", Move: true}, false,
			map[string]string{KeySiteMove: "1", KeySiteSigninBackground: "look"}, KeySiteName},
		{"move is stored off over a plain background", sitelook.Settings{Background: "plain", Move: true}, false,
			map[string]string{KeySiteMove: "0"}, KeySiteName},
		{"defaults are stored as empty", sitelook.Settings{Name: "Chronicle"}, false,
			map[string]string{KeySiteName: "", KeySiteLogo: "", KeySiteLogoFavicon: "0", KeySiteLook: "",
				KeySiteSigninBackground: "plain", KeySiteSigninPicture: "", KeySiteWelcome: ""}, KeySiteName},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rows := map[string]string{}
			var order []string
			repo := &mockSettingsRepo{setFn: func(_ context.Context, k, v string) error {
				rows[k] = v
				order = append(order, k)
				return nil
			}}
			_, err := NewSettingsService(repo).UpdateSiteLook(context.Background(), tc.in)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if tc.wantErr {
				if len(rows) != 0 {
					t.Errorf("wrote %v despite the error", rows)
				}
				return
			}
			for k, want := range tc.wantRows {
				if got, ok := rows[k]; !ok || got != want {
					t.Errorf("%s = %q (present %v), want %q", k, got, ok, want)
				}
			}
			if order[len(order)-1] != tc.wantLast {
				t.Errorf("last write = %s, want %s", order[len(order)-1], tc.wantLast)
			}
		})
	}
}
