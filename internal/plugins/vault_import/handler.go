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

// Handler serves Manage > Import. It binds the request and renders; the rules
// live in Service.
type Handler struct {
	svc *Service
}

// NewHandler wires the handler to the import service.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// maxFormOverhead is room in the request body for the multipart framing around
// the zip itself.
const maxFormOverhead = 1 << 20

// Page renders Manage > Import, resuming the progress bar if an import for this
// campaign is still running.
func (h *Handler) Page(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	var active *JobView
	if j, ok := h.svc.ActiveJob(cc.Campaign.ID); ok {
		active = &j
	}
	return middleware.Render(c, http.StatusOK, ImportPage(cc, active, h.svc.Limits()))
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
	p, err := h.svc.Preview(cc.Campaign.ID, userID, zipPath, cleanUploadName(fh.Filename))
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
