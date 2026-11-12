package app

import (
	"context"

	"github.com/keyxmakerx/chronicle/internal/patch"
	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/records"
	"github.com/keyxmakerx/chronicle/internal/plugins/designs"
)

// aiDesignsAdapter implements records.DesignsAPI over the designs service,
// which keeps every rule: the owner gate, the labels and the kind's own
// validator. The AI saves and renames; sharing stays with the owner.
type aiDesignsAdapter struct {
	svc     designs.Service
	gallery designs.Gallery
}

var _ records.DesignsAPI = aiDesignsAdapter{}

// The saved-designs block follows the shop room and storefront in the prompt.
func init() {
	addAIKinds(206, func(d *aiDeps) []records.Kind {
		if d.Designs == nil {
			return nil
		}
		return []records.Kind{records.DesignKind{Svc: d.Designs, Owners: aiMenuAdapter{campaigns: d.Campaigns, types: d.Entities}}}
	})
}

func (a aiDesignsAdapter) List(ctx context.Context, campaignID string, act records.Actor) ([]records.DesignRef, error) {
	list, err := a.svc.List(ctx, campaignID, act.UserID, "")
	if err != nil {
		return nil, err
	}
	out := make([]records.DesignRef, len(list))
	for i, d := range list {
		out[i] = records.DesignRef{ID: d.ID, Kind: d.Kind, Name: d.Name, Note: d.Note, SystemID: d.SystemID, Maker: d.MakerName, Origin: d.Origin, Shared: d.Shared, Data: d.Data}
	}
	return out, nil
}

func (a aiDesignsAdapter) Check(ctx context.Context, campaignID string, act records.Actor, v records.DesignValue) error {
	return a.svc.Check(ctx, campaignID, act.UserID, designs.SaveInput{Kind: v.Kind, Name: v.Name, Note: v.Note, Data: v.Data})
}

func (a aiDesignsAdapter) Save(ctx context.Context, campaignID string, act records.Actor, v records.DesignValue) error {
	_, err := a.svc.Save(ctx, campaignID, act.UserID, designs.SaveInput{Kind: v.Kind, Name: v.Name, Note: v.Note, Data: v.Data})
	return err
}

// Rename sends only what was written, so a rename leaves the note alone.
func (a aiDesignsAdapter) Rename(ctx context.Context, campaignID string, act records.Actor, id string, name, note *string) error {
	in := designs.UpdateDesignInput{}
	if name != nil {
		in.Name = patch.Of(*name)
	}
	if note != nil {
		in.Note = patch.Of(*note)
	}
	_, err := a.svc.Update(ctx, campaignID, act.UserID, id, in)
	return err
}

// Browse reads the server gallery for the `gallery` lookup. It is the only
// gallery call the AI has: no publish, report or moderation reaches it.
func (a aiDesignsAdapter) Browse(ctx context.Context, campaignID string, act records.Actor, kind, text string, allSystems bool) ([]records.GalleryRef, int, error) {
	if a.gallery == nil {
		return nil, 0, nil
	}
	page, err := a.gallery.Browse(ctx, campaignID, act.UserID, designs.BrowseInput{AllSystems: allSystems, Kind: kind, Q: text})
	if err != nil {
		return nil, 0, err
	}
	out := make([]records.GalleryRef, len(page.Entries))
	for i, g := range page.Entries {
		out[i] = records.GalleryRef{Name: g.Name, Note: g.Note, Kind: g.Kind, SystemID: g.SystemID, Maker: g.MakerName, Summary: g.Summary, Lacks: g.Lacks}
	}
	return out, page.Total, nil
}

// Package reads the game system package's designs for the `designs` lookup.
// It is read only: the service has no call here that changes a package.
func (a aiDesignsAdapter) Package(ctx context.Context, campaignID string, act records.Actor) (string, []records.DesignRef, error) {
	g, err := a.svc.Package(ctx, campaignID, act.UserID, "")
	if err != nil {
		return "", nil, err
	}
	out := make([]records.DesignRef, len(g.Designs))
	for i, d := range g.Designs {
		out[i] = records.DesignRef{ID: d.ID, Kind: d.Kind, Name: d.Name, Note: d.Note, SystemID: g.SystemID, Maker: d.MakerName, Origin: designs.OriginPackage, Data: d.Data}
	}
	return g.SystemName, out, nil
}
