package app

import (
	"context"
	"encoding/json"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/records"
	"github.com/keyxmakerx/chronicle/internal/plugins/armory"
)

// aiItemLooksAdapter implements records.ItemLooksAPI over the item look
// service, which keeps every check: the fixed lists, the made-from page and
// who may see a page.
type aiItemLooksAdapter struct {
	svc   armory.ItemLookService
	names armory.LookEntityFinder
}

var _ records.ItemLooksAPI = aiItemLooksAdapter{}

func (l aiItemLooksAdapter) Get(ctx context.Context, campaignID, pageID string, a records.Actor) (*records.ItemLookRef, error) {
	view, err := l.svc.GetLook(ctx, campaignID, pageID, a.Role, a.UserID)
	if err != nil || view.Look == nil {
		return nil, err
	}
	var own armory.ItemLook
	if err := json.Unmarshal(view.Look, &own); err != nil {
		return nil, apperror.NewInternal(err)
	}
	name, _, err := l.names.EntityName(ctx, campaignID, pageID)
	if err != nil {
		return nil, err
	}
	ref := &records.ItemLookRef{PageID: pageID, Page: name, Value: lookValue(own)}
	if view.Source != nil {
		ref.MadeFrom = view.Source.Name
	}
	return ref, nil
}

func (l aiItemLooksAdapter) List(ctx context.Context, campaignID string, a records.Actor) ([]records.ItemLookRef, bool, error) {
	list, more, err := l.svc.ListLooks(ctx, campaignID, a.Role, a.UserID)
	if err != nil {
		return nil, false, err
	}
	out := make([]records.ItemLookRef, 0, len(list))
	for _, p := range list {
		out = append(out, records.ItemLookRef{PageID: p.EntityID, Page: p.Name, Value: lookValue(p.Look), MadeFrom: p.MadeFromName})
	}
	return out, more, nil
}

func (l aiItemLooksAdapter) Check(ctx context.Context, campaignID, pageID string, v records.ItemLookValue) error {
	raw, err := lookJSON(v)
	if err != nil {
		return err
	}
	_, err = l.svc.CheckLook(ctx, campaignID, pageID, raw)
	return err
}

func (l aiItemLooksAdapter) Save(ctx context.Context, campaignID, pageID string, a records.Actor, v records.ItemLookValue) error {
	raw, err := lookJSON(v)
	if err != nil {
		return err
	}
	_, err = l.svc.SaveLook(ctx, campaignID, pageID, a.UserID, raw)
	return err
}

func lookValue(l armory.ItemLook) records.ItemLookValue {
	v := records.ItemLookValue{Look: l.Look, Tint: l.Tint, Sound: l.Sound, Words: l.Words, MadeFromID: l.MadeFrom}
	if len(l.Spots) > 0 {
		v.Spots = make(map[string]records.ItemLookSpot, len(l.Spots))
		for k, s := range l.Spots {
			v.Spots[k] = records.ItemLookSpot{Look: s.Look, Tint: s.Tint}
		}
	}
	return v
}

func lookJSON(v records.ItemLookValue) ([]byte, error) {
	l := armory.ItemLook{Look: v.Look, Tint: v.Tint, Sound: v.Sound, Words: v.Words, MadeFrom: v.MadeFromID}
	if len(v.Spots) > 0 {
		l.Spots = make(map[string]armory.ItemLookSpot, len(v.Spots))
		for k, s := range v.Spots {
			l.Spots[k] = armory.ItemLookSpot{Look: s.Look, Tint: s.Tint}
		}
	}
	raw, err := json.Marshal(l)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	return raw, nil
}
