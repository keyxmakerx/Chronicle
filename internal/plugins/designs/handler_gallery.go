package designs

import (
	"net/http"
	"strconv"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
)

// ActivityRecorder is the slice of the admin change log the moderation routes
// write to. It is declared here so this plugin imports neither admin nor auth
// for it; app wiring supplies an adapter that stamps the signed-in admin. It
// is the site-wide log, because the campaign audit log needs a campaign and a
// gallery decision belongs to none.
type ActivityRecorder interface {
	RecordAdminChange(c *echo.Context, action, targetType, targetID, label string)
}

// GalleryHandler is the JSON surface for the gallery, thin like Handler.
type GalleryHandler struct {
	svc      Gallery
	activity ActivityRecorder
}

// NewGalleryHandler creates the handler; the recorder may be nil.
func NewGalleryHandler(svc Gallery, activity ActivityRecorder) *GalleryHandler {
	return &GalleryHandler{svc: svc, activity: activity}
}

// Publish handles POST /campaigns/:id/designs/:did/gallery.
func (h *GalleryHandler) Publish(c *echo.Context) error {
	cid, uid, err := ctxOf(c)
	if err != nil {
		return err
	}
	var req struct {
		Name      string `json:"name"`
		Note      string `json:"note"`
		MakerName string `json:"makerName"`
	}
	if err := bind(c, &req); err != nil {
		return err
	}
	m, err := h.svc.Publish(c.Request().Context(), cid, uid, c.Param("did"), PublishInput{Name: req.Name, Note: req.Note, MakerName: req.MakerName})
	if err != nil {
		return err
	}
	return c.JSON(http.StatusCreated, m)
}

// Republish handles PUT /campaigns/:id/designs/:did/gallery, a partial update:
// only the keys sent change.
func (h *GalleryHandler) Republish(c *echo.Context) error {
	cid, uid, err := ctxOf(c)
	if err != nil {
		return err
	}
	var in RepublishInput
	if err := bind(c, &in); err != nil {
		return err
	}
	m, err := h.svc.Republish(c.Request().Context(), cid, uid, c.Param("did"), in)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, m)
}

// Unpublish handles DELETE /campaigns/:id/designs/gallery/:gid.
func (h *GalleryHandler) Unpublish(c *echo.Context) error {
	cid, uid, err := ctxOf(c)
	if err != nil {
		return err
	}
	if err := h.svc.Unpublish(c.Request().Context(), cid, uid, c.Param("gid")); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

// Mine handles GET /campaigns/:id/designs/gallery/mine.
func (h *GalleryHandler) Mine(c *echo.Context) error {
	cid, uid, err := ctxOf(c)
	if err != nil {
		return err
	}
	out, err := h.svc.Mine(c.Request().Context(), cid, uid)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, map[string]any{"entries": out})
}

// Browse handles GET /campaigns/:id/designs/gallery?system=all&kind=&q=&page=.
func (h *GalleryHandler) Browse(c *echo.Context) error {
	cid, uid, err := ctxOf(c)
	if err != nil {
		return err
	}
	page, _ := strconv.Atoi(c.QueryParam("page"))
	out, err := h.svc.Browse(c.Request().Context(), cid, uid, BrowseInput{
		AllSystems: c.QueryParam("system") == "all", Kind: c.QueryParam("kind"), Q: c.QueryParam("q"), Page: page,
	})
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, out)
}

// Copy handles POST /campaigns/:id/designs/gallery/:gid/copy.
func (h *GalleryHandler) Copy(c *echo.Context) error {
	cid, uid, err := ctxOf(c)
	if err != nil {
		return err
	}
	d, err := h.svc.Copy(c.Request().Context(), cid, uid, c.Param("gid"))
	if err != nil {
		return err
	}
	return c.JSON(http.StatusCreated, d)
}

// Report handles POST /campaigns/:id/designs/gallery/:gid/report.
func (h *GalleryHandler) Report(c *echo.Context) error {
	cid, uid, err := ctxOf(c)
	if err != nil {
		return err
	}
	var in ReportInput
	if err := bind(c, &in); err != nil {
		return err
	}
	if err := h.svc.Report(c.Request().Context(), cid, uid, c.Param("gid"), in); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

// AdminPage handles GET /admin/design-gallery: the page shell, which loads its
// queue as JSON from AdminEntries.
func (h *GalleryHandler) AdminPage(c *echo.Context) error {
	return middleware.Render(c, http.StatusOK, GalleryAdminPage())
}

// AdminEntries handles GET /admin/design-gallery/entries?status=&page=.
func (h *GalleryHandler) AdminEntries(c *echo.Context) error {
	page, _ := strconv.Atoi(c.QueryParam("page"))
	out, err := h.svc.AdminQueue(c.Request().Context(), c.QueryParam("status"), page)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, out)
}

// adminLabels are the admin change log's action words for each decision.
var adminLabels = map[string]string{
	DecisionApprove: "designgallery.approved",
	DecisionReject:  "designgallery.rejected",
	DecisionRemove:  "designgallery.removed",
}

// decide returns the handler for one moderation decision and logs it once the
// service has applied it.
func (h *GalleryHandler) decide(decision string) echo.HandlerFunc {
	return func(c *echo.Context) error {
		uid := auth.GetUserID(c)
		if uid == "" {
			return apperror.NewForbidden("admin access required")
		}
		e, err := h.svc.AdminDecide(c.Request().Context(), uid, c.Param("gid"), decision)
		if err != nil {
			return err
		}
		if h.activity != nil {
			h.activity.RecordAdminChange(c, adminLabels[decision], "design_gallery", e.ID, e.Name)
		}
		return c.JSON(http.StatusOK, e)
	}
}

// Approve, Reject and Remove handle POST /admin/design-gallery/:gid/<decision>.
func (h *GalleryHandler) Approve(c *echo.Context) error { return h.decide(DecisionApprove)(c) }
func (h *GalleryHandler) Reject(c *echo.Context) error  { return h.decide(DecisionReject)(c) }
func (h *GalleryHandler) Remove(c *echo.Context) error  { return h.decide(DecisionRemove)(c) }
