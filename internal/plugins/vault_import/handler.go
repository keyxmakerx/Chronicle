package vault_import

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// Handler serves Settings > Import. It binds the request and renders; the
// rules live in Service.
type Handler struct {
	svc *Service
}

// NewHandler wires the handler to the import service.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// maxFormOverhead is room in the request body for the multipart framing around
// the zip itself.
const maxFormOverhead = 1 << 20

// RequestLimit is the largest request body the upload route accepts: the zip
// plus the multipart framing. The app applies it as a global middleware, ahead
// of the CSRF check, because that check parses the form (and so spools the
// body to disk) before the route's own limit could run.
func RequestLimit(l Limits) int64 { return l.MaxUploadBytes + maxFormOverhead }

// Page sends the old Import page address to Settings > Import, so bookmarks
// and links written before the move still land on the import.
func (h *Handler) Page(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	return c.Redirect(http.StatusSeeOther, "/campaigns/"+cc.Campaign.ID+"/settings?tab="+SettingsTabID)
}

// SettingsTabID is the Import tab's id on the campaign Settings page.
const SettingsTabID = "import"

// SettingsTabFactory returns the Settings > Import tab for the campaigns
// plugin's tab registry. It sits right after Data, is owner-only like
// every import route, and resumes the progress bar of a running import.
func (h *Handler) SettingsTabFactory() func(*campaigns.CampaignContext) campaigns.SettingsTab {
	return func(cc *campaigns.CampaignContext) campaigns.SettingsTab {
		var active *JobView
		if j, ok := h.svc.ActiveJob(cc.Campaign.ID); ok {
			active = &j
		}
		return campaigns.SettingsTab{
			ID:        SettingsTabID,
			Label:     "Import",
			Icon:      "fa-solid fa-file-import",
			MinRole:   campaigns.RoleOwner,
			SortOrder: 55,
			Content:   ImportTab(cc, active, h.svc.Limits()),
		}
	}
}

// DropZone renders the upload step on its own, for "choose a different file"
// and "import another".
func (h *Handler) DropZone(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	return middleware.Render(c, http.StatusOK, DropZone(cc.Campaign.ID, h.svc.Limits()))
}

// Preview takes the uploaded zip and answers with what an import would do.
// Nothing is written to the campaign.
func (h *Handler) Preview(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	userID := auth.GetUserID(c)
	if userID == "" {
		return apperror.NewUnauthorized("authentication required")
	}
	req := c.Request()
	req.Body = http.MaxBytesReader(c.Response(), req.Body, h.svc.Limits().MaxUploadBytes+maxFormOverhead)

	fh, err := c.FormFile("file")
	if err != nil {
		return apperror.NewBadRequest("Choose a .zip file to import.")
	}
	if !strings.HasSuffix(strings.ToLower(fh.Filename), ".zip") {
		return apperror.NewBadRequest("That isn't a .zip file. Zip the vault folder and try again.")
	}
	src, err := fh.Open()
	if err != nil {
		return apperror.NewBadRequest("The upload could not be read. Try again.")
	}
	defer func() { _ = src.Close() }()

	zipPath, err := h.svc.SaveUpload(src)
	if err != nil {
		return err
	}
	p, err := h.svc.Preview(c.Request().Context(), cc.Campaign.ID, userID, zipPath, cleanUploadName(fh.Filename))
	if err != nil {
		return err
	}
	return middleware.Render(c, http.StatusOK, PreviewPanel(cc.Campaign.ID, p))
}

// Start begins the import of a previewed upload and answers with its progress.
func (h *Handler) Start(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	userID := auth.GetUserID(c)
	if userID == "" {
		return apperror.NewUnauthorized("authentication required")
	}
	j, err := h.svc.Start(cc.Campaign.ID, userID, c.FormValue("upload"))
	if err != nil {
		return err
	}
	return middleware.Render(c, http.StatusOK, ProgressPanel(cc.Campaign.ID, *j, 0))
}

// Job answers a poll with the current progress, or the result once it ends. A
// job the server no longer knows (it restarted) gets a plain explanation and
// no further polling.
func (h *Handler) Job(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	from, _ := strconv.Atoi(c.QueryParam("from"))
	if from < 0 || from > 100 {
		from = 0
	}
	j, ok := h.svc.Job(cc.Campaign.ID, c.Param("job"))
	if !ok {
		return middleware.Render(c, http.StatusOK, ImportLost(cc.Campaign.ID))
	}
	return middleware.Render(c, http.StatusOK, ProgressPanel(cc.Campaign.ID, j, from))
}

// cleanUploadName reduces a browser-supplied name to something fit to show
// back: no folders, bounded length.
func cleanUploadName(name string) string {
	if i := strings.LastIndexAny(name, `/\`); i >= 0 {
		name = name[i+1:]
	}
	name = strings.TrimSpace(name)
	if len(name) > 120 {
		name = name[:120]
	}
	return strings.ToValidUTF8(name, "")
}
