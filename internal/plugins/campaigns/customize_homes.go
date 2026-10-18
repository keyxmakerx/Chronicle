package campaigns

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
)

// customizeSectionHomes is the draft key for the campaign's home pages: the
// campaign home for each audience and the DM dashboard, arranged in the
// Customizer's page editor and saved with the rest of the draft.
const customizeSectionHomes = "homes"

// HomeDM names the DM dashboard among the homes the Customizer arranges. It
// is not a home audience (ValidHomeAudience): it has its own column and is
// only ever seen by the DM team.
const HomeDM = "dm"

// homeKeys are the homes in the order the Customizer offers them.
var homeKeys = []string{HomeDefault, HomePlayer, HomeScribe, HomeDirector, HomeGuest, HomeDM}

// validHomeKey reports whether key names a home the Customizer arranges.
func validHomeKey(key string) bool {
	return key == HomeDM || ValidHomeAudience(key)
}

// homeItem is one home's change in the draft. A nil Layout is no change, so
// a hand-edited draft can never blank a home by leaving the field out.
type homeItem struct {
	Layout *DashboardLayout `json:"layout,omitempty"`
}

// homesSectionValue is the section as the page stores it. Changes is the
// page's own list of labels for its save bar, carried through untouched.
type homesSectionValue struct {
	Items   map[string]homeItem `json:"items"`
	Changes json.RawMessage     `json:"_changes,omitempty"`
}

// applyHomesSection saves each home's layout through the same checks as the
// dashboard layout endpoints, and keeps in the draft only the homes that
// failed, so one bad layout never loses the others.
func (h *Handler) applyHomesSection(ctx context.Context, cc *CampaignContext, userID string, raw json.RawMessage) (json.RawMessage, error) {
	if cc.MemberRole < RoleOwner {
		return nil, apperror.NewForbidden("only campaign owners can arrange the home pages")
	}
	var sec homesSectionValue
	if err := json.Unmarshal(raw, &sec); err != nil {
		return nil, apperror.NewBadRequest("the home pages in the draft are not valid")
	}

	keys := make([]string, 0, len(sec.Items))
	for k := range sec.Items {
		if !validHomeKey(k) {
			return nil, apperror.NewBadRequest("unknown home: " + k)
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)

	remaining := map[string]homeItem{}
	var firstErr error
	for _, k := range keys {
		it := sec.Items[k]
		if it.Layout == nil {
			continue
		}
		if err := h.saveHomeLayout(ctx, cc.Campaign.ID, k, it.Layout); err != nil {
			remaining[k] = it
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		h.logAuditCtx(ctx, cc.Campaign.ID, userID, "dashboard_layout_updated", map[string]any{"role": k, "via": "customize"})
	}
	if len(remaining) == 0 {
		return nil, nil
	}
	out, err := json.Marshal(homesSectionValue{Items: remaining, Changes: sec.Changes})
	if err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("marshaling the remaining homes: %w", err))
	}
	return out, firstErr
}

// saveHomeLayout stores one home's layout: the DM dashboard in its own
// column, an audience's home merged into the role-keyed campaign home.
func (h *Handler) saveHomeLayout(ctx context.Context, campaignID, key string, layout *DashboardLayout) error {
	if key == HomeDM {
		return h.service.UpdateOwnerDashboardLayout(ctx, campaignID, layout)
	}
	if err := validateDashboardLayout(ctx, campaignID, layout); err != nil {
		return err
	}
	campaign, err := h.service.GetByID(ctx, campaignID)
	if err != nil {
		return err
	}
	full, err := campaign.SetRoleDashboardJSON(key, layout)
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("marshaling role layout: %w", err))
	}
	return h.service.UpdateDashboardLayoutRaw(ctx, campaignID, full)
}

// homeTab is one home the Customizer's Home pages area offers.
type homeTab struct {
	Key, Label, Icon, Hint string
}

// homeTabs are the Home pages area's tabs, in homeKeys order.
var homeTabs = []homeTab{
	{HomeDefault, "Everyone", "fa-users", "Every member sees this home unless their role has one of its own."},
	{HomePlayer, "Players", "fa-user", "Players see this instead of the Everyone home."},
	{HomeScribe, "Scribes", "fa-feather", "Scribes see this instead of the Everyone home."},
	{HomeDirector, "The Director", "fa-crown", "You and your co-DMs land here instead of the Everyone home."},
	{HomeGuest, "Guests", "fa-door-open", "Visitors who aren't members see this. It never falls back to a members' home."},
	{HomeDM, "DM dashboard", "fa-user-shield", "Your Overview under Manage, seen only by you and your co-DMs."},
}

// homeTabFor returns the tab for key, or the Everyone tab.
func homeTabFor(key string) homeTab {
	for _, t := range homeTabs {
		if t.Key == key {
			return t
		}
	}
	return homeTabs[0]
}

// homeHint says who sees a home and, while it has no layout of its own, what
// they see instead, so the owner knows a borrowed home is not yet its own.
func (h *Handler) homeHint(cc *CampaignContext, key string) string {
	hint := homeTabFor(key).Hint
	switch {
	case key == HomeDM:
		if cc.Campaign.ParseOwnerDashboardLayout() == nil {
			hint += " Until you arrange it, the Overview shows its built-in links."
		}
	case key == HomeGuest:
		if !cc.Campaign.HasSavedHome(key) {
			hint += " Until you arrange it, they get a starter home: the about, the pages open to them and the latest of those."
		}
	case key != HomeDefault:
		if !cc.Campaign.HasSavedHome(key) {
			hint += " It is the same as the Everyone home until you change it."
		}
	}
	return hint
}

// defaultDMDashboardLayout is where the DM dashboard's editor starts while
// nothing is saved: the categories and the latest pages, which the Overview's
// built-in page also shows.
func defaultDMDashboardLayout() *DashboardLayout {
	full := DefaultDashboardLayout()
	out := &DashboardLayout{}
	for _, row := range full.Rows {
		if row.ID == "row-categories" || row.ID == "row-recent" {
			out.Rows = append(out.Rows, row)
		}
	}
	return out
}

// homeEditorState is what the page editor draws a home from: the layout the
// audience sees now and whether it is that home's own (unsaved means the
// editor starts from what it borrows, and Save stores it as its own).
func (h *Handler) homeEditorState(cc *CampaignContext, key string) string {
	base := fmt.Sprintf("/campaigns/%s", cc.Campaign.ID)
	var layout *DashboardLayout
	saved := false
	if key == HomeDM {
		layout = cc.Campaign.ParseOwnerDashboardLayout()
		saved = layout != nil
		if layout == nil {
			layout = defaultDMDashboardLayout()
		}
	} else {
		layout, saved = cc.Campaign.HomeLayout(key)
	}
	if layout == nil {
		layout = DefaultDashboardLayout()
	}
	tab := homeTabFor(key)
	b, err := json.Marshal(map[string]any{
		"scope":      "home",
		"unsaved":    !saved,
		"campaignId": cc.Campaign.ID,
		"key":        key,
		"name":       tab.Label,
		"layout":     layout,
		"blockTypes": base + "/entity-types/block-types?context=dashboard",
		"preview":    base + "/customize/home-preview",
	})
	if err != nil {
		return "{}"
	}
	return string(b)
}

const (
	// homePreviewMaxBlocks bounds one preview request; a home holds at most
	// maxDashboardRows rows of a few blocks or a tile section each.
	homePreviewMaxBlocks = 200
	// homePreviewMaxBody bounds the request body.
	homePreviewMaxBody = 128 << 10
)

// HomePreview draws the blocks the Customizer sends as the campaign home
// draws them, so the editor shows the real home: POST
// /campaigns/:id/customize/home-preview with {"blocks":[{id,type,config}]}.
// It answers {"blocks":{id: html}}; a block that draws nothing is left out.
func (h *Handler) HomePreview(c echo.Context) error {
	cc := GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	var body struct {
		Blocks []DashboardBlock `json:"blocks"`
	}
	if err := json.NewDecoder(io.LimitReader(c.Request().Body, homePreviewMaxBody)).Decode(&body); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}
	if len(body.Blocks) > homePreviewMaxBlocks {
		return apperror.NewBadRequest("too many blocks to preview at once")
	}
	ctx := c.Request().Context()
	// Blocks such as the category grid read the sidebar's data from the
	// render context, which middleware.Render normally fills in.
	rctx := ctx
	if middleware.LayoutInjector != nil {
		rctx = middleware.LayoutInjector(c, ctx)
	}
	var recent []RecentEntity
	if h.recentLister != nil {
		recent, _ = h.recentLister.ListRecentForDashboard(ctx, cc.Campaign.ID, int(cc.VisibilityRole()), auth.GetUserID(c), 8)
	}
	out := map[string]string{}
	for _, b := range body.Blocks {
		if b.ID == "" || out[b.ID] != "" || !ValidBlockTypes[b.Type] {
			continue
		}
		if html := renderHomePreviewBlock(rctx, cc, b, recent); strings.TrimSpace(html) != "" {
			out[b.ID] = html
		}
	}
	return c.JSON(http.StatusOK, map[string]any{"blocks": out})
}

// renderHomePreviewBlock draws one block as the home does. A renderer that
// fails or panics yields nothing, so one broken block cannot blank the editor.
func renderHomePreviewBlock(ctx context.Context, cc *CampaignContext, b DashboardBlock, recent []RecentEntity) (html string) {
	defer func() {
		if r := recover(); r != nil {
			slog.Warn("home preview: block renderer panicked", slog.String("block_type", b.Type), slog.Any("panic", r))
			html = ""
		}
	}()
	if b.Config == nil {
		b.Config = map[string]any{}
	}
	var sb strings.Builder
	if err := DashboardBlockSwitch(cc, b, recent).Render(ctx, &sb); err != nil {
		slog.Warn("home preview: block render failed", slog.String("block_type", b.Type), slog.Any("error", err))
		return ""
	}
	return sb.String()
}
