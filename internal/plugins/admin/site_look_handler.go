package admin

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/sitelook"
	"github.com/keyxmakerx/chronicle/internal/templates/layouts"
)

// SiteLookPageData is everything the Site look page renders. Draft is what
// the form shows (the saved look, or the rejected submission with its error);
// Saved is what the site currently uses.
type SiteLookPageData struct {
	Saved     sitelook.Settings
	Draft     sitelook.Settings
	Error     string
	Notice    string
	CSRFToken string
}

// SetSiteLookService wires the Site look service. Called after the settings
// and media services exist.
func (h *Handler) SetSiteLookService(svc SiteLookService) {
	h.siteLook = svc
}

// SiteLook handles GET /admin/site-look.
func (h *Handler) SiteLook(c echo.Context) error {
	if h.siteLook == nil {
		return apperror.NewMissingContext()
	}
	saved, err := h.siteLook.Get(c.Request().Context())
	if err != nil {
		return err
	}
	data := SiteLookPageData{Saved: saved, Draft: saved, CSRFToken: middleware.GetCSRFToken(c)}
	if c.QueryParam("saved") == "1" {
		data.Notice = "Saved. The change shows on every page's next load."
	}
	return middleware.Render(c, http.StatusOK, SiteLookPage(data))
}

// SaveSiteLook handles POST /admin/site-look: the whole form in one request,
// uploads included, so nothing is saved by half.
func (h *Handler) SaveSiteLook(c echo.Context) error {
	if h.siteLook == nil {
		return apperror.NewMissingContext()
	}
	ctx := c.Request().Context()

	sub := SiteLookSubmission{
		Draft: sitelook.Settings{
			Name:          c.FormValue("name"),
			LogoAsFavicon: c.FormValue("favicon") == "1",
			Look:          c.FormValue("look"),
			Background:    c.FormValue("background"),
			Welcome:       c.FormValue("welcome"),
			Move:          c.FormValue("move") == "1",
		},
		RemoveLogo:    c.FormValue("remove_logo") == "1",
		RemovePicture: c.FormValue("remove_picture") == "1",
	}

	var err error
	if sub.Logo, err = formUpload(c, "logo_file", sitelook.LogoMaxBytes); err != nil {
		return h.renderSiteLookError(c, sub, err)
	}
	if sub.Picture, err = formUpload(c, "picture_file", sitelook.PictureMaxBytes); err != nil {
		return h.renderSiteLookError(c, sub, err)
	}

	if _, err := h.siteLook.Save(ctx, auth.GetUserID(c), sub); err != nil {
		return h.renderSiteLookError(c, sub, err)
	}
	h.record(c, "sitelook.saved", "setting", "site_look", "")
	return c.Redirect(http.StatusSeeOther, "/admin/site-look?saved=1")
}

// renderSiteLookError re-renders the page with the submitted choices and the
// reason a Save was refused. Only a rejected input (400) is shown to the
// admin; anything else is a real failure and goes to the error page.
func (h *Handler) renderSiteLookError(c echo.Context, sub SiteLookSubmission, err error) error {
	var ae *apperror.AppError
	if !errors.As(err, &ae) || ae.Code != http.StatusBadRequest {
		return err
	}
	saved, gerr := h.siteLook.Get(c.Request().Context())
	if gerr != nil {
		return gerr
	}
	draft := sub.Draft
	draft.Configured = true
	draft.Logo, draft.Picture = saved.Logo, saved.Picture
	return middleware.Render(c, http.StatusBadRequest, SiteLookPage(SiteLookPageData{
		Saved: saved, Draft: draft, Error: ae.Message, CSRFToken: middleware.GetCSRFToken(c),
	}))
}

// formUpload reads one optional file field, refusing anything over limit
// without reading past it. nil means no file was chosen.
func formUpload(c echo.Context, field string, limit int64) (*SiteLookUpload, error) {
	fh, err := c.FormFile(field)
	if err != nil {
		if errors.Is(err, http.ErrMissingFile) || errors.Is(err, http.ErrNotMultipart) {
			return nil, nil
		}
		return nil, apperror.NewBadRequest("couldn't read the uploaded file")
	}
	if fh.Size > limit {
		return nil, apperror.NewBadRequest("that picture is too large")
	}
	src, err := fh.Open()
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	defer func() { _ = src.Close() }()
	data, err := io.ReadAll(io.LimitReader(src, limit+1))
	if err != nil {
		return nil, apperror.NewBadRequest("couldn't read the uploaded file")
	}
	if int64(len(data)) > limit {
		return nil, apperror.NewBadRequest("that picture is too large")
	}
	return &SiteLookUpload{Name: fh.Filename, Data: data}, nil
}

// siteLookBg is the sign-in background the preview shows: the chosen one, with
// "look" falling back to plain while no look is picked, as the real page does.
func siteLookBg(s sitelook.Settings) string {
	switch {
	case s.Background == sitelook.BackgroundLook && s.Look != "":
		return sitelook.BackgroundLook
	case s.Background == sitelook.BackgroundPicture && s.Picture != "":
		return sitelook.BackgroundPicture
	}
	return sitelook.BackgroundPlain
}

// siteLookVars is the preview's colour variables for the chosen look, from the
// fixed sitelook table, with Chronicle's shipped accent when none is chosen.
func siteLookVars(s sitelook.Settings) string {
	if l, ok := s.ActiveLook(); ok {
		return "--sa:" + l.Accent + ";--hd1:" + l.HeaderFrom + ";--hd2:" + l.HeaderTo
	}
	return "--sa:#6366f1"
}

// siteLookOn is "on" or "off", the preview's state attributes.
func siteLookOn(b bool) string {
	if b {
		return "on"
	}
	return "off"
}

// siteLookURL is the URL of a stored site picture, or "" for none.
func siteLookURL(ctx context.Context, name string) string {
	if name == "" {
		return ""
	}
	return layouts.MediaURL(ctx, name)
}

// itoa formats a pixel size for the page's templates.
func itoa(n int) string { return strconv.Itoa(n) }
