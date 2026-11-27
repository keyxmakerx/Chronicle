// The HTTP surface for cut scenes. Handlers bind, call the service and render;
// the service owns every rule, so a mis-wired route cannot widen access.
package cutscenes

import (
	"errors"
	"net/http"
	"net/url"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// Handler serves the cut scene routes.
type Handler struct{ svc Service }

// NewHandler builds the handler.
func NewHandler(svc Service) *Handler { return &Handler{svc: svc} }

func viewerFrom(c *echo.Context) (string, Viewer, error) {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return "", Viewer{}, apperror.NewMissingContext()
	}
	return cc.Campaign.ID, Viewer{UserID: auth.GetUserID(c), Director: cc.CanAuthorDmOnly()}, nil
}

// director returns the campaign and the viewer, or the 404 a non-Director
// gets before anything is read or rendered. The service checks again; this
// one keeps a player's malformed form from ever drawing the editor.
func director(c *echo.Context) (string, Viewer, error) {
	cid, v, err := viewerFrom(c)
	if err != nil {
		return "", Viewer{}, err
	}
	if !v.Director {
		return "", Viewer{}, notFound()
	}
	return cid, v, nil
}

// form reads a posted form of at most MaxBodyBytes.
func form(c *echo.Context) (url.Values, error) {
	r := c.Request()
	r.Body = http.MaxBytesReader(c.Response(), r.Body, MaxBodyBytes)
	f, err := c.FormValues()
	if err != nil {
		return nil, apperror.NewBadRequest("the form could not be read")
	}
	return f, nil
}

// badRequest is the refusal an editor shows in place; anything else goes to
// the site's error page.
func badRequest(err error) (string, bool) {
	var ae *apperror.AppError
	if errors.As(err, &ae) && (ae.Code == http.StatusBadRequest || ae.Code == http.StatusConflict) {
		return ae.Message, true
	}
	return "", false
}

// foundry says whether the editor offers Preview in Foundry.
func (h *Handler) foundry(c *echo.Context, cid string, v Viewer) bool {
	return h.svc.FoundryPreviewOffered(c.Request().Context(), cid, v)
}

func sceneURL(campaignID, sceneID string) string {
	return "/campaigns/" + campaignID + "/cutscenes/" + sceneID
}

// redirect sends a page or an HTMX request to url.
func redirect(c *echo.Context, url string) error {
	if middleware.IsHTMX(c) {
		c.Response().Header().Set("HX-Redirect", url)
		return c.NoContent(http.StatusNoContent)
	}
	return c.Redirect(http.StatusSeeOther, url)
}

// List handles GET /campaigns/:id/cutscenes, the Cut scenes page.
func (h *Handler) List(c *echo.Context) error {
	cid, v, err := director(c)
	if err != nil {
		return err
	}
	ctx := c.Request().Context()
	scenes, err := h.svc.List(ctx, cid, v)
	if err != nil {
		return err
	}
	recent, err := h.svc.Recent(ctx, cid, v)
	if err != nil {
		return err
	}
	return middleware.Render(c, http.StatusOK, ListPage(listPage{CampaignID: cid, Scenes: scenes, Recent: recent, Now: nowUTC()}))
}

// New handles GET /campaigns/:id/cutscenes/new.
func (h *Handler) New(c *echo.Context) error {
	cid, v, err := director(c)
	if err != nil {
		return err
	}
	return middleware.Render(c, http.StatusOK, EditorPage(editorPage{CampaignID: cid, Scene: NewScene(), New: true, Foundry: h.foundry(c, cid, v)}))
}

// Create handles POST /campaigns/:id/cutscenes.
func (h *Handler) Create(c *echo.Context) error {
	cid, v, err := director(c)
	if err != nil {
		return err
	}
	f, err := form(c)
	if err != nil {
		return err
	}
	in, err := SceneFromForm(f)
	if err == nil {
		var sc *Scene
		if sc, err = h.svc.Create(c.Request().Context(), cid, v, in); err == nil {
			return redirect(c, sceneURL(cid, sc.ID))
		}
	}
	msg, ok := badRequest(err)
	if !ok {
		return err
	}
	p := editorPage{CampaignID: cid, Scene: in, New: true, Error: msg, Foundry: h.foundry(c, cid, v)}
	if middleware.IsHTMX(c) {
		return middleware.Render(c, http.StatusOK, editorForm(p))
	}
	return middleware.Render(c, http.StatusOK, EditorPage(p))
}

// Editor handles GET /campaigns/:id/cutscenes/:sid.
func (h *Handler) Editor(c *echo.Context) error {
	cid, v, err := director(c)
	if err != nil {
		return err
	}
	ctx := c.Request().Context()
	sc, err := h.svc.Get(ctx, cid, c.Param("sid"), v)
	if err != nil {
		return err
	}
	live, err := h.svc.LiveFor(ctx, cid, sc.ID, v)
	if err != nil {
		return err
	}
	return middleware.Render(c, http.StatusOK, EditorPage(editorPage{CampaignID: cid, Scene: *sc, Live: live, Foundry: h.foundry(c, cid, v)}))
}

// Save handles POST /campaigns/:id/cutscenes/:sid. The editor posts the whole
// scene; an HTMX save answers with the form redrawn as stored.
func (h *Handler) Save(c *echo.Context) error {
	cid, v, err := director(c)
	if err != nil {
		return err
	}
	f, err := form(c)
	if err != nil {
		return err
	}
	sid := c.Param("sid")
	in, err := SceneFromForm(f)
	in.ID = sid
	var sc *Scene
	if err == nil {
		sc, err = h.svc.Save(c.Request().Context(), cid, sid, v, in)
	}
	if err != nil {
		msg, ok := badRequest(err)
		if !ok {
			return err
		}
		p := editorPage{CampaignID: cid, Scene: in, Error: msg, Foundry: h.foundry(c, cid, v)}
		if middleware.IsHTMX(c) {
			return middleware.Render(c, http.StatusOK, editorForm(p))
		}
		return middleware.Render(c, http.StatusOK, EditorPage(p))
	}
	if middleware.IsHTMX(c) {
		return middleware.Render(c, http.StatusOK, editorForm(editorPage{CampaignID: cid, Scene: *sc, Saved: true, Foundry: h.foundry(c, cid, v)}))
	}
	return c.Redirect(http.StatusSeeOther, sceneURL(cid, sid))
}

// Delete handles POST /campaigns/:id/cutscenes/:sid/delete. Showings keep
// their copy of the scene, so people can still watch it again.
func (h *Handler) Delete(c *echo.Context) error {
	cid, v, err := director(c)
	if err != nil {
		return err
	}
	if err := h.svc.Delete(c.Request().Context(), cid, c.Param("sid"), v); err != nil {
		return err
	}
	return redirect(c, "/campaigns/"+cid+"/cutscenes")
}

// Preview handles POST /campaigns/:id/cutscenes/preview: the editor's form as
// the scene JSON the engine plays, checked exactly as a save would be, so
// Preview shows unsaved changes and never something a save would refuse.
func (h *Handler) Preview(c *echo.Context) error {
	cid, v, err := director(c)
	if err != nil {
		return err
	}
	f, err := form(c)
	if err != nil {
		return err
	}
	in, err := SceneFromForm(f)
	if err != nil {
		return err
	}
	sc, err := h.svc.Check(c.Request().Context(), cid, v, in)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, sc)
}

// PreviewInFoundry handles POST /campaigns/:id/cutscenes/preview/foundry: the
// editor's form, checked as Preview checks it, played on the Director's own
// Foundry screen. Answers {"sent": true}; a refusal carries the reason.
func (h *Handler) PreviewInFoundry(c *echo.Context) error {
	cid, v, err := director(c)
	if err != nil {
		return err
	}
	f, err := form(c)
	if err != nil {
		return err
	}
	in, err := SceneFromForm(f)
	if err != nil {
		return err
	}
	if _, err := h.svc.PreviewInFoundry(c.Request().Context(), cid, v, in); err != nil {
		return err
	}
	return c.JSON(http.StatusOK, map[string]bool{"sent": true})
}

// ShowSheet handles GET /campaigns/:id/cutscenes/:sid/show, the "Show to…"
// sheet (an HTMX fragment).
func (h *Handler) ShowSheet(c *echo.Context) error {
	cid, v, err := director(c)
	if err != nil {
		return err
	}
	ctx := c.Request().Context()
	sc, err := h.svc.Get(ctx, cid, c.Param("sid"), v)
	if err != nil {
		return err
	}
	rows, err := h.svc.Audience(ctx, cid, v)
	if err != nil {
		return err
	}
	return middleware.Render(c, http.StatusOK, ShowSheet(showSheet{CampaignID: cid, Scene: *sc, Rows: rows, Rewatch: DefaultRewatch}))
}

// Show handles POST /campaigns/:id/cutscenes/:sid/show. It answers with the
// live row; a refusal redraws the sheet with the reason in it.
func (h *Handler) Show(c *echo.Context) error {
	cid, v, err := director(c)
	if err != nil {
		return err
	}
	f, err := form(c)
	if err != nil {
		return err
	}
	ctx := c.Request().Context()
	sid := c.Param("sid")
	in := ShowFromForm(f)
	sh, err := h.svc.Show(ctx, cid, sid, v, in)
	if err != nil {
		msg, ok := badRequest(err)
		if !ok {
			return err
		}
		sc, gerr := h.svc.Get(ctx, cid, sid, v)
		if gerr != nil {
			return gerr
		}
		rows, aerr := h.svc.Audience(ctx, cid, v)
		if aerr != nil {
			return aerr
		}
		picked := map[string]bool{}
		for _, id := range in.To {
			picked[id] = true
		}
		c.Response().Header().Set("HX-Retarget", "#"+sheetID(sid))
		c.Response().Header().Set("HX-Reswap", "innerHTML")
		return middleware.Render(c, http.StatusOK, ShowSheet(showSheet{CampaignID: cid, Scene: *sc, Rows: rows, Rewatch: in.Rewatch, Picked: picked, Error: msg}))
	}
	return middleware.Render(c, http.StatusOK, ShownRow(cid, *sh, true))
}

// Stop handles POST /campaigns/:id/cutscenes/showings/:shid/stop.
func (h *Handler) Stop(c *echo.Context) error {
	cid, v, err := director(c)
	if err != nil {
		return err
	}
	sh, err := h.svc.Stop(c.Request().Context(), cid, c.Param("shid"), v)
	if err != nil {
		return err
	}
	if !middleware.IsHTMX(c) {
		return c.JSON(http.StatusOK, map[string]any{"showingId": sh.ID, "stopped": true})
	}
	// The same stop is pressed from a scene's live list or the "Shown
	// lately" list; each gets its own row back.
	if c.QueryParam("from") == "recent" {
		return middleware.Render(c, http.StatusOK, RecentRow(cid, ReportOf(*sh, nowUTC()), nowUTC()))
	}
	return middleware.Render(c, http.StatusOK, StoppedRow(*sh, nowUTC()))
}

// Showings handles GET /campaigns/:id/cutscenes/showings, the Director's
// recent showings: a fragment for HTMX, JSON otherwise.
func (h *Handler) Showings(c *echo.Context) error {
	cid, v, err := director(c)
	if err != nil {
		return err
	}
	list, err := h.svc.Recent(c.Request().Context(), cid, v)
	if err != nil {
		return err
	}
	if middleware.IsHTMX(c) {
		return middleware.Render(c, http.StatusOK, RecentList(cid, list, nowUTC()))
	}
	return c.JSON(http.StatusOK, map[string]any{"data": list, "total": len(list)})
}

// Mine handles GET /campaigns/:id/cutscenes/mine: the caller's showings still
// inside their window, newest first.
func (h *Handler) Mine(c *echo.Context) error {
	cid, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	list, err := h.svc.Mine(c.Request().Context(), cid, v)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, map[string]any{"data": list, "total": len(list)})
}

// ReadShowing handles GET /campaigns/:id/cutscenes/showings/:shid.
func (h *Handler) ReadShowing(c *echo.Context) error {
	cid, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	out, err := h.svc.Read(c.Request().Context(), cid, c.Param("shid"), v)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, out)
}

// Watched handles POST /campaigns/:id/cutscenes/showings/:shid/watched.
func (h *Handler) Watched(c *echo.Context) error {
	cid, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	if err := h.svc.Watched(c.Request().Context(), cid, c.Param("shid"), v); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}
