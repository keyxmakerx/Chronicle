package entities

import (
	"context"
	"fmt"
	"io"

	"github.com/a-h/templ"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/templates/layouts"
)

// page_editor.go decides where the page editor is offered. The page editor is
// how the Annals look edits one page where it stands: Edit page puts the page
// into editing, a pen beside each part opens that part, and its values become
// fields in place (static/js/widgets/page_editor.js). Other looks, the DM
// Screen's sheet and pages a game system draws keep the per-block controls.

type fullPageKey struct{}

// withFullPage marks a render as the page itself rather than the DM Screen's
// sheet, which draws the same body but is no place to edit it.
func withFullPage(c templ.Component) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		return c.Render(context.WithValue(ctx, fullPageKey{}, true), w)
	})
}

func isFullPage(ctx context.Context) bool {
	on, _ := ctx.Value(fullPageKey{}).(bool)
	return on
}

// pageEditorOn reports whether this viewer gets the page editor on this page:
// the Annals look, the page itself, someone who may change it (a Scribe or the
// player who claimed it), a page laid out in blocks rather than drawn by a game
// system, and the usual page rather than an era's version of it.
func pageEditorOn(ctx context.Context, cc *campaigns.CampaignContext, entity *Entity, entityType *EntityType) bool {
	if cc == nil || entity == nil || !isFullPage(ctx) || !layouts.PageLookAnnals(ctx) {
		return false
	}
	if hasEntityShowRenderer(entityType) || eraViewOf(ctx).Shown() {
		return false
	}
	return CanEditIdentity(cc.MemberRole, entity, layouts.GetUserID(ctx))
}

type pageEditorKey struct{}

// withPageEditor draws its children with the page's pageEditorOn answer, for
// the blocks, which are drawn without the entity type and, in the
// Customizer's previews, without the page.
func withPageEditor(on bool) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		children := templ.GetChildren(ctx)
		ctx = templ.ClearChildren(ctx)
		return children.Render(context.WithValue(ctx, pageEditorKey{}, on), w)
	})
}

// inPageEditor reports whether the blocks being drawn belong to the page
// editor, so they leave their own edit controls to it.
func inPageEditor(ctx context.Context) bool {
	on, _ := ctx.Value(pageEditorKey{}).(bool)
	return on
}

// pageEditorEndpoint is one of the page's own routes, for the page editor's
// saves. It is always the usual page: the page editor is not offered on an
// era's version.
func pageEditorEndpoint(cc *campaigns.CampaignContext, entity *Entity, leaf string) string {
	return fmt.Sprintf("/campaigns/%s/entities/%s/%s", cc.Campaign.ID, entity.ID, leaf)
}
