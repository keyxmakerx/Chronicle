package entities

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
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// advHandlerEnv is a Handler over the real entity service with one ordinary
// category (1) and the DM-only Adventures category (9), and one page in each.
// Using the real service means the 404s below come from the code that ships,
// not from a stub that restates the rule.
func advHandlerEnv() *Handler {
	types := map[int]*EntityType{
		1: {ID: 1, CampaignID: "c1", Slug: "npc", Name: "NPC", NamePlural: "NPCs", Enabled: true},
		9: {ID: 9, CampaignID: "c1", Slug: "adventures", Name: "Adventure", NamePlural: "Adventures", Enabled: true, PresetCategory: advPreset()},
	}
	entities := map[string]*Entity{
		"npc-page": {ID: "npc-page", CampaignID: "c1", EntityTypeID: 1, Name: "Barkeep", Visibility: VisibilityDefault},
		"adv-page": {ID: "adv-page", CampaignID: "c1", EntityTypeID: 9, Name: "The Sunken Crypt", Visibility: VisibilityDefault},
	}
	typeRepo := &mockEntityTypeRepo{
		findByIDFn: func(_ context.Context, id int) (*EntityType, error) {
			if et, ok := types[id]; ok {
				cp := *et
				return &cp, nil
			}
			return nil, apperror.NewNotFound("entity type not found")
		},
		listByCampaignFn: func(context.Context, string) ([]EntityType, error) {
			return []EntityType{*types[1], *types[9]}, nil
		},
	}
	entityRepo := &mockEntityRepo{
		findByIDFn: func(_ context.Context, id string) (*Entity, error) {
			if e, ok := entities[id]; ok {
				cp := *e
				return &cp, nil
			}
			return nil, apperror.NewNotFound("entity not found")
		},
		filterViewableFn: func(ids []string) (map[string]bool, error) {
			// The repository's SQL hides the Adventures page from a player;
			// the integration test proves that predicate against MariaDB.
			out := map[string]bool{}
			for _, id := range ids {
				out[id] = id != "adv-page"
			}
			return out, nil
		},
	}
	return &Handler{service: NewEntityService(entityRepo, typeRepo, &mockPermissionRepo{})}
}

func advCtx(role campaigns.Role, dmGranted bool, method, target string) (echo.Context, *httptest.ResponseRecorder) {
	e := echo.New()
	req := httptest.NewRequest(method, target, nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("campaign_context", &campaigns.CampaignContext{
		Campaign: &campaigns.Campaign{ID: "c1"}, MemberRole: role, IsDmGranted: dmGranted, IsMember: true,
	})
	auth.SetSession(c, &auth.Session{UserID: "u1"})
	return c, rec
}

func notFound(err error) bool {
	var ae *apperror.AppError
	return errors.As(err, &ae) && ae.Code == http.StatusNotFound
}

func TestShow_AdventuresPageIs404ForPlayers(t *testing.T) {
	tests := []struct {
		name string
		role campaigns.Role
		dm   bool
	}{
		{"player", campaigns.RolePlayer, false},
		{"guest member", campaigns.RoleNone, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := advCtx(tt.role, tt.dm, http.MethodGet, "/campaigns/c1/entities/adv-page")
			c.SetParamNames("id", "eid")
			c.SetParamValues("c1", "adv-page")
			if err := advHandlerEnv().Show(c); !notFound(err) {
				t.Fatalf("Show err = %v, want a 404", err)
			}
		})
	}
}

// Every per-page read endpoint gates on CheckEntityAccess, so a player gets the
// same 404 as a page that does not exist; the table covers the ones a page
// widget or the editor calls.
func TestPageReadEndpoints_AdventuresPageIs404ForPlayers(t *testing.T) {
	h := advHandlerEnv()
	endpoints := map[string]func(echo.Context) error{
		"entry":    h.GetEntry,
		"fields":   h.GetFieldsAPI,
		"preview":  h.PreviewAPI,
		"backlink": h.BacklinksFragment,
		"aliases":  h.GetAliasesAPI,
	}
	for name, fn := range endpoints {
		t.Run(name, func(t *testing.T) {
			c, _ := advCtx(campaigns.RolePlayer, false, http.MethodGet, "/x")
			c.SetParamNames("id", "eid")
			c.SetParamValues("c1", "adv-page")
			if err := fn(c); !notFound(err) {
				t.Fatalf("%s err = %v, want a 404", name, err)
			}
		})
	}
}

func TestIndex_AdventuresCategoryIs404ForPlayers(t *testing.T) {
	tests := []struct {
		name string
		path string
	}{
		{"by type id", "/campaigns/c1/entities?type=9"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := advCtx(campaigns.RolePlayer, false, http.MethodGet, tt.path)
			c.SetParamNames("id")
			c.SetParamValues("c1")
			if err := advHandlerEnv().Index(c); !notFound(err) {
				t.Fatalf("Index err = %v, want a 404", err)
			}
		})
	}
	t.Run("by slug shortcut", func(t *testing.T) {
		h := advHandlerEnv()
		// The slug shortcut resolves through the type repository, which the env
		// does not wire for slugs; resolve it the way the router would.
		svc := h.service.(*entityService)
		svc.types = &slugTypes{mockEntityTypeRepo: svc.types.(*mockEntityTypeRepo)}
		c, _ := advCtx(campaigns.RolePlayer, false, http.MethodGet, "/campaigns/c1/adventures")
		c.SetParamNames("id")
		c.SetParamValues("c1")
		c.Set("entity_type_slug", "adventures")
		if err := h.Index(c); !notFound(err) {
			t.Fatalf("Index err = %v, want a 404", err)
		}
	})
}

// slugTypes answers the slug lookup with the Adventures category.
type slugTypes struct{ *mockEntityTypeRepo }

func (s *slugTypes) FindBySlug(ctx context.Context, _ string, slug string) (*EntityType, error) {
	if slug == "adventures" {
		return s.FindByID(ctx, 9)
	}
	return s.FindByID(ctx, 1)
}

func TestEntityTypesAPI_HidesAdventuresFromPlayers(t *testing.T) {
	tests := []struct {
		name      string
		role      campaigns.Role
		dm        bool
		wantAdv   bool
		wantTotal int
	}{
		{"player", campaigns.RolePlayer, false, false, 1},
		{"scribe", campaigns.RoleScribe, false, true, 2},
		{"owner", campaigns.RoleOwner, false, true, 2},
		{"player with a DM grant", campaigns.RolePlayer, true, true, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, rec := advCtx(tt.role, tt.dm, http.MethodGet, "/campaigns/c1/entity-types")
			c.SetParamNames("id")
			c.SetParamValues("c1")
			if err := advHandlerEnv().EntityTypesAPI(c); err != nil {
				t.Fatal(err)
			}
			var got []map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if len(got) != tt.wantTotal {
				t.Fatalf("got %d categories, want %d", len(got), tt.wantTotal)
			}
			if has := strings.Contains(rec.Body.String(), "Adventures"); has != tt.wantAdv {
				t.Errorf("Adventures in response = %v, want %v", has, tt.wantAdv)
			}
		})
	}
}

type recordingFavorites struct {
	toggled bool
	items   []FavoriteItem
	ids     map[string]bool
}

func (r *recordingFavorites) Toggle(context.Context, string, string, string) (bool, error) {
	r.toggled = true
	return true, nil
}
func (r *recordingFavorites) List(context.Context, string, string) ([]FavoriteItem, error) {
	return append([]FavoriteItem(nil), r.items...), nil
}
func (r *recordingFavorites) IsFavorite(context.Context, string, string) (bool, error) {
	return false, nil
}
func (r *recordingFavorites) ListIDs(context.Context, string, string) (map[string]bool, error) {
	return r.ids, nil
}

func TestFavorites_NeverCarryAdventuresForPlayers(t *testing.T) {
	t.Run("a player cannot bookmark an Adventures page", func(t *testing.T) {
		h := advHandlerEnv()
		fav := &recordingFavorites{}
		h.favoriteRepo = fav
		c, _ := advCtx(campaigns.RolePlayer, false, http.MethodPost, "/campaigns/c1/entities/adv-page/favorite")
		c.SetParamNames("id", "eid")
		c.SetParamValues("c1", "adv-page")
		if err := h.ToggleFavoriteAPI(c); !notFound(err) {
			t.Fatalf("err = %v, want a 404", err)
		}
		if fav.toggled {
			t.Error("the bookmark must not be written")
		}
	})

	t.Run("an ordinary page can still be bookmarked", func(t *testing.T) {
		h := advHandlerEnv()
		fav := &recordingFavorites{}
		h.favoriteRepo = fav
		c, _ := advCtx(campaigns.RolePlayer, false, http.MethodPost, "/campaigns/c1/entities/npc-page/favorite")
		c.SetParamNames("id", "eid")
		c.SetParamValues("c1", "npc-page")
		if err := h.ToggleFavoriteAPI(c); err != nil || !fav.toggled {
			t.Fatalf("err=%v toggled=%v", err, fav.toggled)
		}
	})

	t.Run("a stale bookmark is dropped from the list", func(t *testing.T) {
		h := advHandlerEnv()
		h.favoriteRepo = &recordingFavorites{items: []FavoriteItem{{EntityID: "npc-page", Name: "Barkeep"}, {EntityID: "adv-page", Name: "The Sunken Crypt"}}}
		c, rec := advCtx(campaigns.RolePlayer, false, http.MethodGet, "/campaigns/c1/favorites")
		c.SetParamNames("id")
		c.SetParamValues("c1")
		if err := h.ListFavoritesAPI(c); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(rec.Body.String(), "Sunken Crypt") || !strings.Contains(rec.Body.String(), "Barkeep") {
			t.Errorf("list = %s", rec.Body)
		}
	})

	t.Run("a stale bookmark is dropped from the id set", func(t *testing.T) {
		h := advHandlerEnv()
		h.favoriteRepo = &recordingFavorites{ids: map[string]bool{"npc-page": true, "adv-page": true}}
		c, rec := advCtx(campaigns.RolePlayer, false, http.MethodGet, "/campaigns/c1/favorite-ids")
		c.SetParamNames("id")
		c.SetParamValues("c1")
		if err := h.FavoriteIDsAPI(c); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(rec.Body.String(), "adv-page") || !strings.Contains(rec.Body.String(), "npc-page") {
			t.Errorf("ids = %s", rec.Body)
		}
	})
}
