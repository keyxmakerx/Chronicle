package records

import (
	"context"
	"fmt"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/aiplug"
	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// GalleryAPI is the server gallery as the `gallery` lookup needs it, adapted in
// app wiring. It only browses: publishing, reporting and moderating stay with
// people, so nothing here can change the gallery.
type GalleryAPI interface {
	// Browse lists visible entries, newest first. kind is "", rooms or crafting.
	Browse(ctx context.Context, campaignID string, a Actor, kind, text string, allSystems bool) ([]GalleryRef, error)
}

// GalleryRef is a gallery entry as the lookup shows it. It carries no design
// data: a person copies an entry from the gallery's own sheet.
type GalleryRef struct {
	Name, Note, Kind, SystemID, Maker, Summary string
	Lacks                                      []string
}

// GalleryLookup answers `what: gallery`: what other owners shared to this
// server. Owner-only, like the library it sits beside.
type GalleryLookup struct {
	Svc    GalleryAPI
	Owners CampaignOwnerAPI
}

func (GalleryLookup) LookupInfo() aiplug.LookupInfo {
	return aiplug.LookupInfo{
		What: "gallery", Aliases: []string{"design-gallery", "shared-designs"}, Keys: "optional `kind` (rooms or crafting), `q` (search name or maker), `all` (yes for every game system)",
		Brief: "designs other owners shared to this server's gallery: name, note, kind, game system, maker and what this campaign lacks (owner only, read only)",
		Tool: aiplug.Tool{
			Name: "design_gallery", Title: "Shared designs", Did: "Browsed the shared designs", Owner: true,
			Desc: "The server gallery: designs other owners shared, filtered to this campaign's game system unless all is yes. Read only; a person copies one from the gallery.",
		},
	}
}

func (l GalleryLookup) Answer(ctx context.Context, campaignID string, a Actor, r Record) (string, string, error) {
	chip := "Shared designs"
	owner := false
	if a.Role >= permissions.RoleOwner && l.Owners != nil {
		ok, err := l.Owners.IsOwner(ctx, campaignID, a.UserID)
		owner = err == nil && ok
	}
	if !owner {
		return chip, "", badRequestf("the design gallery is the campaign owner's")
	}
	all := strings.EqualFold(r.Str("all"), "yes")
	list, err := l.Svc.Browse(ctx, campaignID, a, strings.ToLower(r.Str("kind")), r.Str("q"), all)
	if err != nil {
		return chip, "", err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "## Shared designs (%d)\n\n", len(list))
	if len(list) == 0 {
		b.WriteString("Nothing matches in the gallery.\n")
	}
	for _, g := range list {
		fmt.Fprintf(&b, "- **%s** (%s, game system %s) by %s", g.Name, g.Kind, nonEmpty(g.SystemID, "none"), nonEmpty(g.Maker, "unknown"))
		if g.Note != "" {
			b.WriteString(": " + g.Note)
		}
		if g.Summary != "" {
			b.WriteString(" [" + g.Summary + "]")
		}
		if len(g.Lacks) > 0 {
			b.WriteString(" Lacks here: " + strings.Join(g.Lacks, " "))
		}
		b.WriteString("\n")
	}
	return chip, b.String(), nil
}
