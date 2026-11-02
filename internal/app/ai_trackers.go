package app

// ai_trackers.go lets AI Import keep the Trackers block's counters, steps
// and clocks: a hero's secret count can be made, hidden, put on the hero's
// page and changed with a note, all through the owner's review.

import (
	"context"

	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/records"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/plugins/trackers"
)

func init() {
	addAIKinds(166, func(d *aiDeps) []records.Kind {
		if d.Trackers == nil {
			return nil
		}
		return []records.Kind{records.TrackerKind{Svc: aiTrackersAdapter{svc: d.Trackers, ents: d.Entities}, Entities: d.Entities}}
	})
}

// aiTrackersAdapter implements records.TrackersAPI over the trackers
// service, as the operator; the service applies its Director rule, page
// access and limits.
type aiTrackersAdapter struct {
	svc  trackers.Service
	ents entities.EntityService
}

var _ records.TrackersAPI = aiTrackersAdapter{}

// trackerViewer is the operator as the trackers service sees them. The AI's
// routes are owner-only, and Director is the same rule records apply.
func trackerViewer(a records.Actor) trackers.Viewer {
	return trackers.Viewer{UserID: a.UserID, MemberRole: int(campaigns.RoleOwner), VisibilityRole: a.Role, Director: a.CanAuthorDmOnly()}
}

func trackerSetupIn(in records.TrackerSetup) trackers.KindInput {
	out := trackers.KindInput{Name: in.Name, Shape: in.Shape, Min: in.Min, Max: in.Max,
		Visibility: in.Visibility, EveryTypeID: in.EveryTypeID, Version: in.Version}
	for _, s := range in.Steps {
		out.Steps = append(out.Steps, trackers.Step{Name: s.Name, Text: s.Text})
	}
	for _, b := range in.Bands {
		out.Bands = append(out.Bands, trackers.Band{At: b.At, Text: b.Text})
	}
	return out
}

func trackerSetupOut(k trackers.Kind) records.TrackerSetup {
	out := records.TrackerSetup{ID: k.ID, Name: k.Name, Shape: k.Shape, Min: k.Min, Max: k.Max,
		Visibility: k.Visibility, EveryTypeID: k.EveryTypeID, Version: k.Version, Managed: k.Managed, InUse: k.InUse}
	for _, s := range k.Steps {
		out.Steps = append(out.Steps, records.TrackerStep{Name: s.Name, Text: s.Text})
	}
	for _, b := range k.Bands {
		out.Bands = append(out.Bands, records.TrackerBand{At: b.At, Text: b.Text})
	}
	return out
}

func (t aiTrackersAdapter) Check(in records.TrackerSetup) error {
	return trackers.CheckKind(trackerSetupIn(in))
}

func (t aiTrackersAdapter) Kinds(ctx context.Context, campaignID string, a records.Actor) ([]records.TrackerSetup, []records.TrackerCategory, error) {
	kinds, types, err := t.svc.Kinds(ctx, campaignID, trackerViewer(a))
	if err != nil {
		return nil, nil, err
	}
	out := make([]records.TrackerSetup, len(kinds))
	for i, k := range kinds {
		out[i] = trackerSetupOut(k)
	}
	cats := make([]records.TrackerCategory, len(types))
	for i, c := range types {
		cats[i] = records.TrackerCategory{ID: c.ID, Name: c.Name}
	}
	return out, cats, nil
}

func (t aiTrackersAdapter) CreateKind(ctx context.Context, campaignID string, a records.Actor, in records.TrackerSetup) (*records.TrackerSetup, error) {
	k, err := t.svc.CreateKind(ctx, campaignID, trackerViewer(a), trackerSetupIn(in))
	if err != nil {
		return nil, err
	}
	out := trackerSetupOut(*k)
	return &out, nil
}

func (t aiTrackersAdapter) UpdateKind(ctx context.Context, campaignID, kindID string, a records.Actor, in records.TrackerSetup) (*records.TrackerSetup, error) {
	k, err := t.svc.UpdateKind(ctx, campaignID, kindID, trackerViewer(a), trackerSetupIn(in))
	if err != nil {
		return nil, err
	}
	out := trackerSetupOut(*k)
	return &out, nil
}

func (t aiTrackersAdapter) DeleteKind(ctx context.Context, campaignID, kindID string, a records.Actor) error {
	return t.svc.DeleteKind(ctx, campaignID, kindID, trackerViewer(a))
}

func (t aiTrackersAdapter) Page(ctx context.Context, campaignID, pageID string, a records.Actor) (*records.TrackerPage, error) {
	pv, err := t.svc.Page(ctx, campaignID, pageID, trackerViewer(a))
	if err != nil {
		return nil, err
	}
	out := &records.TrackerPage{}
	for _, tv := range pv.Trackers {
		on := records.TrackerOnPage{ID: tv.ID, KindID: tv.KindID, Name: tv.Name, Shape: tv.Shape,
			Visibility: tv.Visibility, Value: tv.Value, Max: tv.Max, Now: tv.Now}
		for _, s := range tv.Steps {
			on.Steps = append(on.Steps, records.TrackerStep{Name: s.Name, Text: s.Text})
		}
		for _, e := range tv.Log {
			on.Log = append(on.Log, records.TrackerLogLine{Op: e.Op, Amount: e.Amount, Note: e.Note, When: e.When, Struck: e.Struck})
		}
		out.Trackers = append(out.Trackers, on)
	}
	return out, nil
}

func (t aiTrackersAdapter) Place(ctx context.Context, campaignID, pageID string, a records.Actor, in records.TrackerPlace) error {
	_, err := t.svc.Place(ctx, campaignID, pageID, trackerViewer(a), trackers.PlaceInput{KindID: in.KindID, Visibility: in.Visibility})
	return err
}

func (t aiTrackersAdapter) Log(ctx context.Context, campaignID, pageID string, a records.Actor, in records.TrackerChange) error {
	_, err := t.svc.Log(ctx, campaignID, pageID, trackerViewer(a), trackers.LogInput{
		TrackerID: in.TrackerID, KindID: in.KindID, Op: in.Op, Amount: in.Amount, Note: in.Note})
	return err
}

func (t aiTrackersAdapter) Pages(ctx context.Context, campaignID string, a records.Actor) ([]records.TrackerPageRef, error) {
	refs, err := t.svc.Pages(ctx, campaignID, trackerViewer(a))
	if err != nil {
		return nil, err
	}
	out := make([]records.TrackerPageRef, len(refs))
	for i, r := range refs {
		out[i] = records.TrackerPageRef{ID: r.ID, Name: r.Name}
	}
	return out, nil
}

func (t aiTrackersAdapter) HasBlock(ctx context.Context, campaignID, pageID string) (bool, error) {
	return categoryLayoutHas(ctx, t.ents, campaignID, pageID, trackers.BlockType)
}
