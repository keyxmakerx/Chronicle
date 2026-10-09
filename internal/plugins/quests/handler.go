// The browser HTTP surface for quests and notice boards. Handlers bind, call
// the service and shape the response; the services own every rule, so a
// mis-wired route cannot widen access.
package quests

import (
	"io"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/templates/layouts"
)

// Handler serves the quest, notice-board and picker routes.
type Handler struct {
	quests QuestService
	boards BoardService
	picker PickerService
}

// NewHandler builds the handler.
func NewHandler(q QuestService, b BoardService, p PickerService) *Handler {
	return &Handler{quests: q, boards: b, picker: p}
}

// viewerFrom builds the Viewer from the campaign context. DM is the owner or
// a member with DM access; VisibilityRole promotes a co-DM for page visibility.
func viewerFrom(c echo.Context) (campaignID string, v Viewer, err error) {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return "", Viewer{}, apperror.NewMissingContext()
	}
	return cc.Campaign.ID, Viewer{
		UserID:         auth.GetUserID(c),
		MemberRole:     int(cc.MemberRole),
		VisibilityRole: cc.VisibilityRole(),
		IsDM:           cc.CanControlWorldState(),
	}, nil
}

// homeFrom reads the board home from the route: /category-boards/:tid is a
// category, /notice-boards/:eid a page. The same handlers serve both, so the
// two route families cannot drift apart. A malformed category id is a 404,
// the same as a category that does not exist.
func homeFrom(c echo.Context) (Home, error) {
	if raw := c.Param("tid"); raw != "" {
		tid, err := strconv.Atoi(raw)
		if err != nil || tid <= 0 {
			return Home{}, errNotFound("category")
		}
		return TypeHome(tid), nil
	}
	return PageHome(c.Param("eid")), nil
}

// GetQuest handles GET /campaigns/:id/quests/:eid.
func (h *Handler) GetQuest(c echo.Context) error {
	cid, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	out, err := h.quests.Get(c.Request().Context(), cid, c.Param("eid"), v)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, out)
}

// PutQuest handles PUT /campaigns/:id/quests/:eid.
func (h *Handler) PutQuest(c echo.Context) error {
	cid, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	// One byte past the cap so an oversize body is refused, not truncated.
	raw, rerr := io.ReadAll(io.LimitReader(c.Request().Body, MaxBodyBytes+1))
	if rerr != nil || len(raw) > MaxBodyBytes {
		return apperror.NewBadRequest("request body is missing or too large")
	}
	var p QuestPatch
	if err := jsonUnmarshal(raw, &p); err != nil {
		return apperror.NewBadRequest("request body is not valid JSON")
	}
	out, err := h.quests.Put(c.Request().Context(), cid, c.Param("eid"), v, p)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, out)
}

// Picker handles GET /campaigns/:id/quests/picker.
func (h *Handler) Picker(c echo.Context) error {
	cid, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	items, err := h.picker.Search(c.Request().Context(), cid, v, c.QueryParam("kind"), c.QueryParam("q"))
	if err != nil {
		return err
	}
	ctx := c.Request().Context()
	for i := range items {
		if items[i].ImagePath != "" {
			items[i].ImageURL = layouts.MediaThumbURL(ctx, items[i].ImagePath, "300")
		}
	}
	return c.JSON(http.StatusOK, items)
}

// GetBoards handles GET /campaigns/:id/notice-boards/:eid and
// /campaigns/:id/category-boards/:tid.
func (h *Handler) GetBoards(c echo.Context) error {
	cid, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	home, err := homeFrom(c)
	if err != nil {
		return err
	}
	ctx := c.Request().Context()
	out, err := h.boards.View(ctx, cid, home, v)
	if err != nil {
		return err
	}
	for bi := range out.Boards {
		for ii := range out.Boards[bi].Items {
			it := &out.Boards[bi].Items[ii]
			if it.ImagePath != "" {
				it.ImageURL = layouts.MediaThumbURL(ctx, it.ImagePath, "300")
			}
		}
	}
	return c.JSON(http.StatusOK, out)
}

type createBoardRequest struct {
	Name string `json:"name"`
	Who  string `json:"who"`
}

// CreateBoard handles POST .../notice-boards/:eid/boards.
func (h *Handler) CreateBoard(c echo.Context) error {
	cid, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	home, err := homeFrom(c)
	if err != nil {
		return err
	}
	var req createBoardRequest
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}
	out, err := h.boards.CreateBoard(c.Request().Context(), cid, home, v, req.Name, req.Who)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusCreated, out)
}

// PatchBoard handles PATCH .../boards/:bid.
func (h *Handler) PatchBoard(c echo.Context) error {
	cid, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	home, err := homeFrom(c)
	if err != nil {
		return err
	}
	var p BoardPatch
	if err := c.Bind(&p); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}
	out, err := h.boards.PatchBoard(c.Request().Context(), cid, home, c.Param("bid"), v, p)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, out)
}

// DeleteBoard handles DELETE .../boards/:bid.
func (h *Handler) DeleteBoard(c echo.Context) error {
	cid, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	home, err := homeFrom(c)
	if err != nil {
		return err
	}
	if err := h.boards.DeleteBoard(c.Request().Context(), cid, home, c.Param("bid"), v); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

type orderRequest struct {
	IDs []string `json:"ids"`
}

// SetOrder handles PUT .../notice-boards/:eid/order.
func (h *Handler) SetOrder(c echo.Context) error {
	cid, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	home, err := homeFrom(c)
	if err != nil {
		return err
	}
	var req orderRequest
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}
	if err := h.boards.SetOrder(c.Request().Context(), cid, home, v, req.IDs); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

// SetLooks handles PUT .../notice-boards/:eid/looks.
func (h *Handler) SetLooks(c echo.Context) error {
	cid, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	home, err := homeFrom(c)
	if err != nil {
		return err
	}
	var p LooksPatch
	if err := c.Bind(&p); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}
	out, err := h.boards.SetLooks(c.Request().Context(), cid, home, v, p)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, out)
}

// ClearPlayerItems handles DELETE .../boards/:bid/player-items.
func (h *Handler) ClearPlayerItems(c echo.Context) error {
	cid, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	home, err := homeFrom(c)
	if err != nil {
		return err
	}
	n, err := h.boards.ClearPlayerItems(c.Request().Context(), cid, home, c.Param("bid"), v)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, map[string]int{"removed": n})
}

// itemResponse resolves the image path the service could not turn into a URL.
func itemResponse(c echo.Context, iv *ItemView) *ItemView {
	if iv.ImagePath != "" {
		iv.ImageURL = layouts.MediaThumbURL(c.Request().Context(), iv.ImagePath, "300")
	}
	return iv
}

// CreateItem handles POST .../boards/:bid/items.
func (h *Handler) CreateItem(c echo.Context) error {
	cid, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	home, err := homeFrom(c)
	if err != nil {
		return err
	}
	var in ItemInput
	if err := c.Bind(&in); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}
	out, err := h.boards.CreateItem(c.Request().Context(), cid, home, c.Param("bid"), v, in)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusCreated, itemResponse(c, out))
}

// PatchItem handles PATCH .../boards/:bid/items/:iid.
func (h *Handler) PatchItem(c echo.Context) error {
	cid, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	home, err := homeFrom(c)
	if err != nil {
		return err
	}
	var p ItemPatch
	if err := c.Bind(&p); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}
	out, err := h.boards.PatchItem(c.Request().Context(), cid, home, c.Param("bid"), c.Param("iid"), v, p)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, itemResponse(c, out))
}

// DeleteItem handles DELETE .../boards/:bid/items/:iid.
func (h *Handler) DeleteItem(c echo.Context) error {
	cid, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	home, err := homeFrom(c)
	if err != nil {
		return err
	}
	if err := h.boards.DeleteItem(c.Request().Context(), cid, home, c.Param("bid"), c.Param("iid"), v); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}
