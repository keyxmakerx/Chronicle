package entities

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/patch"
	"github.com/keyxmakerx/chronicle/internal/plugins/audit"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/templates/layouts"
)

// maxFamilyEditBody bounds a Save's JSON, far above what MaxFamilyEdits and
// MaxFamilyNewPages can fill.
const maxFamilyEditBody = 64 << 10

// SetFamilyEdit injects the Family tree block's write side. Called during app
// wiring; without it the routes answer 404 rather than half-work.
func (h *Handler) SetFamilyEdit(svc FamilyEditService) { h.familyEdit = svc }

// RequireFamilyDirector is the route gate for the Family tree's edit routes:
// the campaign owner or a granted co-Director, not a Scribe. The service
// checks again on every call.
func RequireFamilyDirector() echo.MiddlewareFunc {
	return campaigns.RequireCapability((*campaigns.CampaignContext).CanAuthorDmOnly, "only the Director can edit the family")
}

func familyActorOf(c echo.Context, cc *campaigns.CampaignContext) FamilyActor {
	return FamilyActor{UserID: auth.GetUserID(c), MemberRole: int(cc.MemberRole), DmGranted: cc.IsDmGranted}
}

type familyCandidateWire struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Sub  string `json:"sub"`
	Img  string `json:"img,omitempty"`
	DM   bool   `json:"dm,omitempty"`
}

// FamilyCandidatesAPI lists the pages the Director may add as family, bounded
// and narrowed by ?q= (GET /campaigns/:id/entities/:eid/family/candidates).
func (h *Handler) FamilyCandidatesAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	if h.familyEdit == nil {
		return apperror.NewNotFound("not found")
	}
	got, err := h.familyEdit.Candidates(c.Request().Context(), cc.Campaign.ID, c.Param("eid"), familyActorOf(c, cc), c.QueryParam("q"))
	if err != nil {
		return err
	}
	ctx := middleware.MediaContext(c)
	out := make([]familyCandidateWire, 0, len(got.Pages))
	for _, p := range got.Pages {
		w := familyCandidateWire{ID: p.ID, Name: p.Name, Sub: p.Type, DM: p.DMOnly}
		if p.Image != "" {
			w.Img = layouts.MediaURL(ctx, p.Image)
		}
		out = append(out, w)
	}
	return c.JSON(http.StatusOK, map[string]any{"pages": out, "more": got.More, "max": FamilyPickerMax})
}

// FamilyEditAPI saves the Director's edit of a page's family
// (POST /campaigns/:id/entities/:eid/family). It answers 200 when every change
// saved and 500 when some did not; both carry the fresh tree and which changes
// did not save, so the page never has to guess.
func (h *Handler) FamilyEditAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	if h.familyEdit == nil {
		return apperror.NewNotFound("not found")
	}
	body, err := io.ReadAll(io.LimitReader(c.Request().Body, maxFamilyEditBody+1))
	if err != nil || len(body) > maxFamilyEditBody {
		return apperror.NewBadRequest("the request is too large")
	}
	var req FamilyEditRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return apperror.NewBadRequest("invalid JSON body")
	}

	// The same default visibility a page made on the new-page form gets.
	private := cc.Campaign.ParseSettings().ResolveNewEntityPrivacy(patch.Absent[bool]())
	pageID := c.Param("eid")
	res, err := h.familyEdit.Apply(c.Request().Context(), cc.Campaign.ID, pageID, familyActorOf(c, cc), private, req)
	if err != nil {
		return err
	}
	for _, p := range res.Created {
		h.logAudit(c, cc.Campaign.ID, audit.ActionEntityCreated, p.ID, p.Name)
	}
	status := http.StatusOK
	if !res.Saved {
		status = http.StatusInternalServerError
	}
	return c.JSON(status, map[string]any{
		"saved": res.Saved, "created": res.Created, "failed": res.Failed,
		"written": res.Written, "rolledBack": res.RolledBack,
		"tree": familyWireOf(middleware.MediaContext(c), cc.Campaign.ID, res.Tree),
	})
}
