package systems

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// bookEditorAllowed is who may edit a campaign's book: the owner, or a member
// the owner made a co-Director. It is deliberately not the reader's
// Director-view rule: seeing Director content is not permission to rewrite
// what every player reads.
func bookEditorAllowed(cc *campaigns.CampaignContext) bool {
	return cc != nil && cc.CanAuthorDmOnly()
}

// bookEditorRequest is what every editor route needs before it does anything:
// the editor check (first, so a player learns nothing about the system), the
// system, and its loaded package book.
type bookEditorRequest struct {
	cc  *campaigns.CampaignContext
	mod System
	pkg *BookPackage
}

func (h *SystemHandler) bookEditor(c echo.Context) (*bookEditorRequest, error) {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return nil, apperror.NewMissingContext()
	}
	if !bookEditorAllowed(cc) {
		return nil, apperror.NewForbidden("only the campaign's Directors can edit the rulebook")
	}
	if h.bookEdits == nil {
		return nil, apperror.NewNotFound("this system has no editable book")
	}
	mod := h.resolveSystem(c)
	if mod == nil {
		return nil, apperror.NewNotFound("system not found")
	}
	// A custom system's id has no length limit, but the edit tables hold 64.
	if len(mod.Info().ID) > maxBookSystemID {
		return nil, apperror.NewValidation("This game system's id is too long for its rulebook to be edited.")
	}
	sysDir := h.systemDir(c, mod)
	if !HasBook(sysDir) {
		return nil, apperror.NewNotFound("this system has no book")
	}
	pkg, err := LoadBookPackage(sysDir, mod.Info())
	if err != nil {
		return nil, apperror.NewBadRequest("The rulebook could not be opened. " + err.Error())
	}
	return &bookEditorRequest{cc: cc, mod: mod, pkg: pkg}, nil
}

func bookBaseURL(cc *campaigns.CampaignContext, mod System) string {
	return "/campaigns/" + cc.Campaign.ID + "/systems/" + mod.Info().ID
}

// readBookBody reads a request body under the same 1 MB cap as a book file.
func readBookBody(c echo.Context) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(c.Request().Body, maxBookFileBytes+1))
	if err != nil {
		return nil, apperror.NewBadRequest("the request could not be read")
	}
	if len(data) > maxBookFileBytes {
		return nil, apperror.NewBadRequest(fmt.Sprintf("the page is larger than %d bytes", maxBookFileBytes))
	}
	return data, nil
}

// readPageEnvelope returns the raw "page" of {"page": {...}}. The page itself
// is decoded by the service with the strict book decoder.
func readPageEnvelope(c echo.Context) ([]byte, error) {
	data, err := readBookBody(c)
	if err != nil {
		return nil, err
	}
	var env struct {
		Page json.RawMessage `json:"page"`
	}
	if err := json.Unmarshal(data, &env); err != nil {
		return nil, apperror.NewBadRequest("the request is not valid JSON")
	}
	return env.Page, nil
}

// BookEditPage renders the full-screen editor.
//
// GET /campaigns/:id/systems/:mod/book/edit
func (h *SystemHandler) BookEditPage(c echo.Context) error {
	r, err := h.bookEditor(c)
	if err != nil {
		return err
	}
	base := bookBaseURL(r.cc, r.mod)
	if middleware.IsHTMX(c) {
		return middleware.Render(c, http.StatusOK, BookEditorContent(r.cc, r.mod.Info(), base))
	}
	return middleware.Render(c, http.StatusOK, BookEditorPage(r.cc, r.mod.Info(), base))
}

// BookSourceAPI is the editor's starting state: the merged edition with
// every page's state and the package's own page where it differs.
//
// GET /campaigns/:id/systems/:mod/book/source
func (h *SystemHandler) BookSourceAPI(c echo.Context) error {
	r, err := h.bookEditor(c)
	if err != nil {
		return err
	}
	src, err := h.bookEdits.EditorSource(c.Request().Context(), r.cc.Campaign.ID, r.pkg)
	if err != nil {
		return err
	}
	src.ExportURL = bookBaseURL(r.cc, r.mod) + "/book/export"
	c.Response().Header().Set("Cache-Control", "private, no-store")
	return c.JSON(http.StatusOK, src)
}

// BookPageSave replaces a page's content.
//
// PUT /campaigns/:id/systems/:mod/book/chapters/:chapter/pages/:key
func (h *SystemHandler) BookPageSave(c echo.Context) error {
	r, err := h.bookEditor(c)
	if err != nil {
		return err
	}
	raw, err := readPageEnvelope(c)
	if err != nil {
		return err
	}
	e, err := h.bookEdits.SavePage(c.Request().Context(), r.cc.Campaign.ID, auth.GetUserID(c), r.pkg, c.Param("chapter"), c.Param("key"), raw)
	if err != nil {
		return err
	}
	return bookJSON(c, map[string]any{"entry": e})
}

// BookPageAdd appends a page of the campaign's own to a chapter.
//
// POST /campaigns/:id/systems/:mod/book/chapters/:chapter/pages
func (h *SystemHandler) BookPageAdd(c echo.Context) error {
	r, err := h.bookEditor(c)
	if err != nil {
		return err
	}
	raw, err := readPageEnvelope(c)
	if err != nil {
		return err
	}
	e, err := h.bookEdits.AddPage(c.Request().Context(), r.cc.Campaign.ID, auth.GetUserID(c), r.pkg, c.Param("chapter"), raw)
	if err != nil {
		return err
	}
	return bookJSON(c, map[string]any{"entry": e})
}

// BookPageDelete drops the campaign's copy of a package page, or deletes a
// page the campaign added.
//
// DELETE /campaigns/:id/systems/:mod/book/chapters/:chapter/pages/:key
func (h *SystemHandler) BookPageDelete(c echo.Context) error {
	r, err := h.bookEditor(c)
	if err != nil {
		return err
	}
	e, err := h.bookEdits.DeletePage(c.Request().Context(), r.cc.Campaign.ID, r.pkg, c.Param("chapter"), c.Param("key"))
	if err != nil {
		return err
	}
	return bookJSON(c, map[string]any{"entry": e})
}

// BookPageKeep settles a page the package changed after the campaign edited
// it, in favour of the campaign's copy.
//
// POST /campaigns/:id/systems/:mod/book/chapters/:chapter/pages/:key/keep
func (h *SystemHandler) BookPageKeep(c echo.Context) error {
	r, err := h.bookEditor(c)
	if err != nil {
		return err
	}
	e, err := h.bookEdits.KeepPage(c.Request().Context(), r.cc.Campaign.ID, auth.GetUserID(c), r.pkg, c.Param("chapter"), c.Param("key"))
	if err != nil {
		return err
	}
	return bookJSON(c, map[string]any{"entry": e})
}

// BookChapterCreate adds a house-rules chapter.
//
// POST /campaigns/:id/systems/:mod/book/chapters
func (h *SystemHandler) BookChapterCreate(c echo.Context) error {
	r, err := h.bookEditor(c)
	if err != nil {
		return err
	}
	data, err := readBookBody(c)
	if err != nil {
		return err
	}
	var in struct {
		Title string `json:"title"`
	}
	if err := json.Unmarshal(data, &in); err != nil {
		return apperror.NewBadRequest("the request is not valid JSON")
	}
	ch, err := h.bookEdits.CreateChapter(c.Request().Context(), r.cc.Campaign.ID, auth.GetUserID(c), r.pkg, in.Title)
	if err != nil {
		return err
	}
	return bookJSON(c, map[string]any{"chapter": ch})
}

// BookChapterUpdate is a partial update of a house-rules chapter: only the
// keys sent change.
//
// PUT /campaigns/:id/systems/:mod/book/chapters/:chapter
func (h *SystemHandler) BookChapterUpdate(c echo.Context) error {
	r, err := h.bookEditor(c)
	if err != nil {
		return err
	}
	data, err := readBookBody(c)
	if err != nil {
		return err
	}
	var in UpdateBookChapterInput
	if err := json.Unmarshal(data, &in); err != nil {
		return apperror.NewBadRequest("the request is not valid JSON")
	}
	ch, err := h.bookEdits.UpdateChapter(c.Request().Context(), r.cc.Campaign.ID, r.pkg, c.Param("chapter"), in)
	if err != nil {
		return err
	}
	return bookJSON(c, map[string]any{"chapter": ch})
}

// BookChapterDelete removes a house-rules chapter and its pages.
//
// DELETE /campaigns/:id/systems/:mod/book/chapters/:chapter
func (h *SystemHandler) BookChapterDelete(c echo.Context) error {
	r, err := h.bookEditor(c)
	if err != nil {
		return err
	}
	if err := h.bookEdits.DeleteChapter(c.Request().Context(), r.cc.Campaign.ID, r.pkg, c.Param("chapter")); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

var exportNameUnsafe = regexp.MustCompile(`[^A-Za-z0-9_-]+`)

// BookExport downloads the campaign's edition as the package's own file
// format, ready to drop into a package's book/ folder.
//
// GET /campaigns/:id/systems/:mod/book/export
func (h *SystemHandler) BookExport(c echo.Context) error {
	r, err := h.bookEditor(c)
	if err != nil {
		return err
	}
	data, err := h.bookEdits.Export(c.Request().Context(), r.cc.Campaign.ID, r.pkg)
	if err != nil {
		return err
	}
	name := exportNameUnsafe.ReplaceAllString(r.mod.Info().ID, "_") + "-book.zip"
	c.Response().Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	c.Response().Header().Set("Cache-Control", "private, no-store")
	return c.Blob(http.StatusOK, "application/zip", data)
}

// bookJSON answers an editor call. Never cached: the body is the campaign's
// private draft state.
func bookJSON(c echo.Context, body any) error {
	c.Response().Header().Set("Cache-Control", "private, no-store")
	return c.JSON(http.StatusOK, body)
}

// CampaignBookPackage loads the editable book of the game system a campaign
// uses, for callers outside an editor request (AI Import). systemID is the
// campaign's settings value; the lookups match the editor routes', and the
// caller does its own Director check (bookEditorAllowed's rule).
func (h *SystemHandler) CampaignBookPackage(campaignID, systemID string) (*BookPackage, BookEditService, error) {
	if h.bookEdits == nil || systemID == "" {
		return nil, nil, apperror.NewNotFound("this campaign has no editable rulebook")
	}
	mod := FindSystem(systemID)
	if mod == nil && h.campaignSystems != nil {
		// Only the campaign's own system when it is the one asked for, as
		// the editor route's resolveSystem does.
		if own := h.campaignSystems.GetSystem(campaignID); own != nil && own.Info().ID == systemID {
			mod = own
		}
	}
	if mod == nil || len(mod.Info().ID) > maxBookSystemID {
		return nil, nil, apperror.NewNotFound("this campaign has no editable rulebook")
	}
	sysDir := Dir(mod.Info().ID)
	if sysDir == "" && h.campaignSystems != nil {
		sysDir = h.campaignSystems.Dir(campaignID)
	}
	if !HasBook(sysDir) {
		return nil, nil, apperror.NewNotFound("this campaign's game system has no rulebook")
	}
	pkg, err := LoadBookPackage(sysDir, mod.Info())
	if err != nil {
		// The loader's text names files and paths; keep it in the log.
		slog.Error("book package load failed", "campaign", campaignID, "system", mod.Info().ID, "error", err)
		return nil, nil, apperror.NewBadRequest("The rulebook could not be opened. Try again, or ask the server's operator to check the game system's files.")
	}
	return pkg, h.bookEdits, nil
}
