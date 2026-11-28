package cutscenes

import (
	"net/http"
	"strconv"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
)

// momentParam reads :m, the moment's place in the scene's list.
func momentParam(c *echo.Context) (int, error) {
	m, err := strconv.Atoi(c.Param("m"))
	if err != nil || m < 0 {
		return 0, apperror.NewBadRequest("no such moment")
	}
	return m, nil
}

// Answer handles POST /campaigns/:id/cutscenes/showings/:shid/moments/:m/answer,
// a viewer's answer from the scene on their screen: form fields way and
// text. Answers the caller's own answer as JSON.
func (h *Handler) Answer(c *echo.Context) error {
	cid, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	m, err := momentParam(c)
	if err != nil {
		return err
	}
	f, err := form(c)
	if err != nil {
		return err
	}
	out, err := h.svc.Answer(c.Request().Context(), cid, c.Param("shid"), m, v, AnswerInput{Way: f.Get("way"), Text: f.Get("text")})
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, out)
}

// Live handles GET /campaigns/:id/cutscenes/:sid/live, the Director's live
// moments panel: a fragment for HTMX, JSON counts otherwise.
func (h *Handler) Live(c *echo.Context) error {
	cid, v, err := director(c)
	if err != nil {
		return err
	}
	sid := c.Param("sid")
	return h.renderLive(c, cid, sid, v, "")
}

func (h *Handler) renderLive(c *echo.Context, cid, sid string, v Viewer, msg string) error {
	list, err := h.svc.LiveMoments(c.Request().Context(), cid, sid, v)
	if err != nil {
		return err
	}
	if !middleware.IsHTMX(c) {
		out := make([]map[string]any, 0, len(list))
		for _, lm := range list {
			out = append(out, map[string]any{"showingId": lm.Showing.ID, "moment": lm.Current, "waiting": lm.Waiting(), "answers": len(lm.Answers)})
		}
		return c.JSON(http.StatusOK, map[string]any{"data": out, "total": len(out)})
	}
	return middleware.Render(c, http.StatusOK, MomentsPanel(cid, sid, list, msg))
}

// showingScene finds which scene's panel a showing belongs to, for the
// redraw after a press.
func (h *Handler) showingScene(c *echo.Context, cid, shid string, v Viewer) (string, error) {
	list, err := h.svc.Recent(c.Request().Context(), cid, v)
	if err != nil {
		return "", err
	}
	for _, r := range list {
		if r.ShowingID == shid {
			return r.SceneID, nil
		}
	}
	return "", notFound()
}

// Resolve handles POST /campaigns/:id/cutscenes/showings/:shid/answers/:aid:
// the Director adds an answer to the story, changes it, or turns it down. A
// refusal redraws the panel with the reason in it.
func (h *Handler) Resolve(c *echo.Context) error {
	cid, v, err := director(c)
	if err != nil {
		return err
	}
	f, err := form(c)
	if err != nil {
		return err
	}
	shid := c.Param("shid")
	a, err := h.svc.Resolve(c.Request().Context(), cid, shid, c.Param("aid"), v, ResolveInput{Action: f.Get("action"), Line: f.Get("line"), Page: f.Get("page")})
	if !middleware.IsHTMX(c) {
		if err != nil {
			return err
		}
		return c.JSON(http.StatusOK, map[string]any{"answerId": a.ID, "status": a.Status})
	}
	msg := ""
	if err != nil {
		var ok bool
		if msg, ok = badRequest(err); !ok {
			return err
		}
	}
	sid, serr := h.showingScene(c, cid, shid, v)
	if serr != nil {
		return serr
	}
	return h.renderLive(c, cid, sid, v, msg)
}

// Go handles POST /campaigns/:id/cutscenes/showings/:shid/moments/:m/go: the
// Director lets the moment go on every viewer's screen.
func (h *Handler) Go(c *echo.Context) error {
	cid, v, err := director(c)
	if err != nil {
		return err
	}
	m, err := momentParam(c)
	if err != nil {
		return err
	}
	sh, err := h.svc.Go(c.Request().Context(), cid, c.Param("shid"), m, v)
	if err != nil {
		return err
	}
	if !middleware.IsHTMX(c) {
		return c.JSON(http.StatusOK, map[string]any{"showingId": sh.ID, "moment": m, "released": true})
	}
	return h.renderLive(c, cid, sh.SceneID, v, "")
}
