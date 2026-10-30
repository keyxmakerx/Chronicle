// The browser HTTP surface for crafting. Handlers bind, call the service and
// shape the response; the service owns every rule, so a mis-wired route
// cannot widen access.
package crafting

import (
	"context"
	"encoding/json"
	"io"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// CalendarSource names the campaign's default calendar as the viewer may
// see it, "" when there is none; the rules editor reads its moons and days.
type CalendarSource interface {
	DefaultCalendarID(ctx context.Context, campaignID string, role int, userID string) string
}

// Handler serves the crafting routes.
type Handler struct {
	svc Service
	cal CalendarSource
}

// NewHandler builds the handler; cal may be nil.
func NewHandler(svc Service, cal CalendarSource) *Handler { return &Handler{svc: svc, cal: cal} }

func viewerFrom(c echo.Context) (*campaigns.CampaignContext, Viewer, error) {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return nil, Viewer{}, apperror.NewMissingContext()
	}
	return cc, Viewer{
		UserID:         auth.GetUserID(c),
		MemberRole:     int(cc.MemberRole),
		VisibilityRole: cc.VisibilityRole(),
		Director:       cc.CanAuthorDmOnly(),
	}, nil
}

// bind reads a JSON body of at most MaxBodyBytes into dst.
func bind(c echo.Context, dst any) error {
	raw, err := io.ReadAll(io.LimitReader(c.Request().Body, MaxBodyBytes+1))
	if err != nil || len(raw) == 0 || len(raw) > MaxBodyBytes {
		return apperror.NewBadRequest("request body is missing or too large")
	}
	if err := json.Unmarshal(raw, dst); err != nil {
		return apperror.NewBadRequest("request body is not valid JSON")
	}
	return nil
}

// Page renders GET /campaigns/:id/crafting: the Director's editors, or a
// player's recipe book. The script reads everything else from the API.
func (h *Handler) Page(c echo.Context) error {
	cc, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	calID := ""
	if h.cal != nil {
		calID = h.cal.DefaultCalendarID(c.Request().Context(), cc.Campaign.ID, v.VisibilityRole, v.UserID)
	}
	return middleware.Render(c, http.StatusOK, CraftingPage(cc, v.Director, calID))
}

// Library handles GET /campaigns/:id/crafting/library.
func (h *Handler) Library(c echo.Context) error {
	cc, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	out, err := h.svc.Library(c.Request().Context(), cc.Campaign.ID, v)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, out)
}

// Catalog handles GET /campaigns/:id/crafting/catalog.
func (h *Handler) Catalog(c echo.Context) error {
	cc, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	out, err := h.svc.Catalog(c.Request().Context(), cc.Campaign.ID, v)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, out)
}

// SaveSettings handles PUT /campaigns/:id/crafting/settings.
func (h *Handler) SaveSettings(c echo.Context) error {
	cc, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	var in Settings
	if err := bind(c, &in); err != nil {
		return err
	}
	out, err := h.svc.SaveSettings(c.Request().Context(), cc.Campaign.ID, v, in)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, out)
}

// ListRecipes handles GET /campaigns/:id/crafting/recipes.
func (h *Handler) ListRecipes(c echo.Context) error {
	cc, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	out, err := h.svc.ListRecipes(c.Request().Context(), cc.Campaign.ID, v)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, out)
}

// GetRecipe handles GET /campaigns/:id/crafting/recipes/:rid.
func (h *Handler) GetRecipe(c echo.Context) error {
	cc, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	out, err := h.svc.GetRecipe(c.Request().Context(), cc.Campaign.ID, c.Param("rid"), v)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, out)
}

// CreateRecipe handles POST /campaigns/:id/crafting/recipes.
func (h *Handler) CreateRecipe(c echo.Context) error {
	cc, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	var in RecipeInput
	if err := bind(c, &in); err != nil {
		return err
	}
	out, err := h.svc.CreateRecipe(c.Request().Context(), cc.Campaign.ID, v, in)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusCreated, out)
}

// UpdateRecipe handles PUT /campaigns/:id/crafting/recipes/:rid.
func (h *Handler) UpdateRecipe(c echo.Context) error {
	cc, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	var in RecipeInput
	if err := bind(c, &in); err != nil {
		return err
	}
	out, err := h.svc.UpdateRecipe(c.Request().Context(), cc.Campaign.ID, c.Param("rid"), v, in)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, out)
}

// SaveRules handles PUT /campaigns/:id/crafting/recipes/:rid/rules.
func (h *Handler) SaveRules(c echo.Context) error {
	cc, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	var in RulesInput
	if err := bind(c, &in); err != nil {
		return err
	}
	out, err := h.svc.SaveRules(c.Request().Context(), cc.Campaign.ID, c.Param("rid"), v, in)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, out)
}

// Approve handles POST /campaigns/:id/crafting/recipes/:rid/approve.
func (h *Handler) Approve(c echo.Context) error {
	cc, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	out, err := h.svc.Approve(c.Request().Context(), cc.Campaign.ID, c.Param("rid"), v)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, out)
}

// DeleteRecipe handles DELETE /campaigns/:id/crafting/recipes/:rid.
func (h *Handler) DeleteRecipe(c echo.Context) error {
	cc, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	if err := h.svc.DeleteRecipe(c.Request().Context(), cc.Campaign.ID, c.Param("rid"), v); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

// Book handles GET /campaigns/:id/crafting/book.
func (h *Handler) Book(c echo.Context) error {
	cc, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	out, err := h.svc.Book(c.Request().Context(), cc.Campaign.ID, v)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, out)
}

type tagsBody struct {
	Tags []string `json:"tags"`
}

// SetTags handles PUT /campaigns/:id/crafting/materials/:eid.
func (h *Handler) SetTags(c echo.Context) error {
	cc, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	var in tagsBody
	if err := bind(c, &in); err != nil {
		return err
	}
	out, err := h.svc.SetTags(c.Request().Context(), cc.Campaign.ID, c.Param("eid"), v, in.Tags)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, tagsBody{Tags: out})
}

// ListBenches handles GET /campaigns/:id/crafting/benches.
func (h *Handler) ListBenches(c echo.Context) error {
	cc, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	out, err := h.svc.ListBenches(c.Request().Context(), cc.Campaign.ID, v)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, out)
}

// GetBench handles GET /campaigns/:id/crafting/benches/:bid.
func (h *Handler) GetBench(c echo.Context) error {
	cc, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	out, err := h.svc.GetBench(c.Request().Context(), cc.Campaign.ID, c.Param("bid"), v)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, out)
}

// CreateBench handles POST /campaigns/:id/crafting/benches.
func (h *Handler) CreateBench(c echo.Context) error {
	cc, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	var in BenchInput
	if err := bind(c, &in); err != nil {
		return err
	}
	out, err := h.svc.CreateBench(c.Request().Context(), cc.Campaign.ID, v, in)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusCreated, out)
}

// UpdateBench handles PUT /campaigns/:id/crafting/benches/:bid.
func (h *Handler) UpdateBench(c echo.Context) error {
	cc, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	var in BenchInput
	if err := bind(c, &in); err != nil {
		return err
	}
	out, err := h.svc.UpdateBench(c.Request().Context(), cc.Campaign.ID, c.Param("bid"), v, in)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, out)
}

// SaveUpgrades handles PUT /campaigns/:id/crafting/benches/:bid/upgrades.
func (h *Handler) SaveUpgrades(c echo.Context) error {
	cc, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	var in UpgradesInput
	if err := bind(c, &in); err != nil {
		return err
	}
	out, err := h.svc.SaveUpgrades(c.Request().Context(), cc.Campaign.ID, c.Param("bid"), v, in)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, out)
}

// SaveArrangement handles PUT /campaigns/:id/crafting/benches/:bid/look/:skin.
func (h *Handler) SaveArrangement(c echo.Context) error {
	cc, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	var in ArrangementInput
	if err := bind(c, &in); err != nil {
		return err
	}
	out, err := h.svc.SaveArrangement(c.Request().Context(), cc.Campaign.ID, c.Param("bid"), c.Param("skin"), v, in)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, out)
}

// DeleteBench handles DELETE /campaigns/:id/crafting/benches/:bid.
func (h *Handler) DeleteBench(c echo.Context) error {
	cc, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	if err := h.svc.DeleteBench(c.Request().Context(), cc.Campaign.ID, c.Param("bid"), v); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}
