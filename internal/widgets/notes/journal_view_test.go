package notes

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/templates/layouts"
)

type stubCharacters struct {
	chars []ClaimedCharacter
	err   error
	asked []string
}

func (s *stubCharacters) ClaimedCharacters(_ context.Context, campaignID, userID string) ([]ClaimedCharacter, error) {
	s.asked = append(s.asked, campaignID+"/"+userID)
	return s.chars, s.err
}

func TestJournalView(t *testing.T) {
	kessa := ClaimedCharacter{ID: "e1", Name: "Kessa Vantry", TypeName: "Player Character"}

	t.Run("a player sees their character and the deep-linked id", func(t *testing.T) {
		chars := &stubCharacters{chars: []ClaimedCharacter{kessa, {ID: "e2", Name: "Old PC"}}}
		h := NewHandler(newAccessSvc())
		h.SetCharacterLister(chars)
		id := "0a1b2c3d-0000-4000-8000-000000000001"
		c, _ := ctxFor(http.MethodGet, "/", "u-player", campaigns.RolePlayer, map[string]string{"noteId": id}, nil)
		v := h.journalView(c, campaigns.GetCampaignContext(c))
		if v.IsGM || v.NoteID != id || v.Character == nil || v.Character.Name != "Kessa Vantry" {
			t.Fatalf("got %+v", v)
		}
		if len(chars.asked) != 1 || chars.asked[0] != "c1/u-player" {
			t.Errorf("looked up %v", chars.asked)
		}
	})

	t.Run("the GM gets no claim card and no lookup", func(t *testing.T) {
		chars := &stubCharacters{chars: []ClaimedCharacter{kessa}}
		h := NewHandler(newAccessSvc())
		h.SetCharacterLister(chars)
		c, _ := ctxFor(http.MethodGet, "/", "u-gm", campaigns.RoleOwner, nil, nil)
		v := h.journalView(c, campaigns.GetCampaignContext(c))
		if !v.IsGM || v.Character != nil || len(chars.asked) != 0 {
			t.Fatalf("got %+v, asked %v", v, chars.asked)
		}
	})

	t.Run("a co-DM is a GM here too", func(t *testing.T) {
		h := NewHandler(newAccessSvc())
		c, _ := ctxFor(http.MethodGet, "/", "u-co", campaigns.RolePlayer, nil, nil)
		cc := campaigns.GetCampaignContext(c)
		cc.IsDmGranted = true
		if v := h.journalView(c, cc); !v.IsGM {
			t.Fatal("a co-DM must get the GM view")
		}
	})

	t.Run("a failed lookup or a malformed id degrades quietly", func(t *testing.T) {
		h := NewHandler(newAccessSvc())
		h.SetCharacterLister(&stubCharacters{err: errors.New("db down")})
		c, _ := ctxFor(http.MethodGet, "/", "u-player", campaigns.RolePlayer, map[string]string{"noteId": "<script>"}, nil)
		v := h.journalView(c, campaigns.GetCampaignContext(c))
		if v.Character != nil || v.NoteID != "" {
			t.Fatalf("got %+v", v)
		}
	})
}

func TestInitials(t *testing.T) {
	for in, want := range map[string]string{
		"Kessa Vantry":          "KV",
		"doran":                 "D",
		"  Sable  the Ashryn  ": "ST",
		"Élodie Brand":          "ÉB",
		"— ...":                 "?",
		"":                      "?",
	} {
		if got := initials(in); got != want {
			t.Errorf("initials(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestShowJournal_RendersThroughTheLayoutInjector: the page must pick up the
// session and campaign for its chrome and for the widget's data-user-id.
// Rendering the component directly skipped the injector and drew the
// signed-out layout.
func TestShowJournal_RendersThroughTheLayoutInjector(t *testing.T) {
	prev := middleware.LayoutInjector
	t.Cleanup(func() { middleware.LayoutInjector = prev })
	middleware.LayoutInjector = func(_ echo.Context, ctx context.Context) context.Context {
		return layouts.SetUserID(ctx, "u-player")
	}
	h := NewHandler(newAccessSvc())
	c, rec := ctxFor(http.MethodGet, "/", "u-player", campaigns.RolePlayer, nil, nil)
	c.Request().Header.Set("HX-Request", "true")
	if err := h.ShowJournal(c); err != nil {
		t.Fatal(err)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `data-user-id="u-player"`) || !strings.Contains(body, `data-widget="journal"`) {
		t.Fatalf("the Journal must render with the viewer's id:\n%.400s", body)
	}
	if !strings.Contains(body, "Shared with GM") || strings.Contains(body, "GM only") {
		t.Error("a player's GM chip says Shared with GM")
	}
}
