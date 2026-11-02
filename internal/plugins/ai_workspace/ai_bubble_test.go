package ai_workspace

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/prompt"
	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/records"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// twoPages is a campaign with a page players see and one only the owner
// does; a list as a player never returns the second.
type twoPages struct{}

func (twoPages) GetEntityTypes(context.Context, string) ([]entities.EntityType, error) {
	return []entities.EntityType{{ID: 1, Name: "Place", Slug: "place", Enabled: true}}, nil
}
func (twoPages) List(_ context.Context, _ string, _ int, role int, _ string, _ entities.ListOptions) ([]entities.Entity, int, error) {
	out := []entities.Entity{{ID: "pg-harbor", CampaignID: "camp-1", Name: "Saltmarch Harbor", Slug: "saltmarch-harbor", EntityTypeID: 1}}
	if role >= int(campaigns.RoleOwner) {
		out = append(out, entities.Entity{ID: "pg-queen", CampaignID: "camp-1", Name: "The Hidden Queen", Slug: "the-hidden-queen", EntityTypeID: 1, IsPrivate: true})
	}
	return out, len(out), nil
}

func copyAs(t *testing.T, h *Handler, query string) (string, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/campaigns/camp-1/ai/copy?"+query, nil), rec)
	ownerCtx(c)
	if err := h.CopyForAI(c); err != nil {
		t.Fatal(err)
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Error("the copied text may be cached")
	}
	if ct := rec.Header().Get(echo.HeaderContentType); !strings.HasPrefix(ct, "text/plain") {
		t.Errorf("content type %q", ct)
	}
	return rec.Body.String(), rec.Header().Get("X-AI-Page")
}

// "Copy this page" carries the page only as the remembered privacy choice
// allows: with "what players see" a page players can't open is left out and
// never named, as if the owner were on no page.
func TestCopyForAI_PageFollowsPrivacy(t *testing.T) {
	h := recordsHandler(&memTables{})
	h.SetPromptBuilder(prompt.NewService(twoPages{}, nil, nil))
	h.SetLookups(&records.Lookups{Pages: twoPages{}})

	for _, tt := range []struct {
		name, query string
		want, never []string
		leftOut     string
	}{
		{"a page players see", "privacy=safe&page=pg-harbor", []string{"# The page I'm on", "Saltmarch Harbor", copyPageAsk}, nil, ""},
		{"a hidden page, players' view", "privacy=safe&page=pg-queen", nil, []string{"# The page I'm on", copyPageAsk}, "left-out"},
		{"a hidden page, owner's view", "privacy=permitted&page=pg-queen", []string{"# The page I'm on", "The Hidden Queen"}, nil, ""},
		{"no page", "privacy=safe", nil, []string{"# The page I'm on"}, ""},
		{"a made-up id", "privacy=permitted&page=nope", nil, []string{"# The page I'm on"}, "left-out"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			body, page := copyAs(t, h, tt.query)
			if page != tt.leftOut {
				t.Errorf("X-AI-Page = %q, want %q (the bubble must not say a left-out page was copied)", page, tt.leftOut)
			}
			if !strings.Contains(body, "## What I want you to generate") {
				t.Errorf("no standing instructions:\n%s", body)
			}
			for _, w := range tt.want {
				if !strings.Contains(body, w) {
					t.Errorf("missing %q", w)
				}
			}
			for _, n := range tt.never {
				if strings.Contains(body, n) {
					t.Errorf("should not carry %q", n)
				}
			}
			if strings.Contains(tt.query, "safe") && strings.Contains(body, "Hidden Queen") {
				t.Error("the players' view named a page players can't see")
			}
		})
	}
}

// The bubble: Copy and Paste answer in one thin row on top, each waiting
// request below with Review, the remembered privacy choice, and a way to
// the full page that the widget can hold back for unsaved changes.
func TestAIBubble(t *testing.T) {
	h, _, _, _ := requestRig(t)
	html := getAs(t, h.AIBubble, "/campaigns/camp-1/ai/bubble").Body.String()
	for _, want := range []string{`id="ai-bubble-heading"`, `data-ai-copy="/campaigns/camp-1/ai/copy"`, "data-ai-paste", "data-ai-tool",
		"Waiting for you · 1", "Claude asks for 1 change", "data-ai-review=", "data-ai-full", `href="/campaigns/camp-1/ai"`, "ai-sees-camp-1"} {
		if !strings.Contains(html, want) {
			t.Errorf("bubble missing %q", want)
		}
	}
	if strings.Index(html, "data-ai-paste") > strings.Index(html, "data-ai-review=") {
		t.Error("copy and paste should sit above the requests")
	}
	if strings.Contains(html, "<textarea name=") || strings.Contains(html, `id="ai-import-paste-area"`) {
		t.Error("the bubble has a paste box of its own; only the fallback box may be there, and only the side sheet's carries the review's id")
	}
	bare := recordsHandler(&memTables{})
	html = getAs(t, bare.AIBubble, "/campaigns/camp-1/ai/bubble").Body.String()
	if !strings.Contains(html, "Nothing waiting") || !strings.Contains(html, "data-ai-copy=") {
		t.Error("without the connector the bubble still copies and pastes")
	}
}

// The side sheet opens on a review by default and holds the copy-and-paste
// tool only when asked for it, so the paste box's id is on the page once.
func TestAIPanel_ToolOnlyWhenAsked(t *testing.T) {
	h, _, _, _ := requestRig(t)
	html := getAs(t, h.AIPanel, "/campaigns/camp-1/ai/panel").Body.String()
	if strings.Contains(html, "When you copy, what may your AI see?") {
		t.Error("the review sheet still carries the copy-and-paste tool")
	}
	if strings.Count(html, `id="ai-import-paste-area"`) != 1 || strings.Count(html, `id="ai-import-review-host"`) != 1 {
		t.Error("the review sheet needs exactly one paste box and review host")
	}
	html = getAs(t, h.AIPanel, "/campaigns/camp-1/ai/panel?tool=1").Body.String()
	if !strings.Contains(html, "When you copy, what may your AI see?") || strings.Count(html, `id="ai-import-paste-area"`) != 1 {
		t.Error("tool=1 should hold the copy-and-paste tool, with one paste box")
	}
	if strings.Contains(html, `id="ai-requests"`) {
		t.Error("the tool sheet should not repeat the requests")
	}
}
