package pageblocks

import "context"

// checklistForViewer returns d as v may see it. Below Owner visibility a
// DM-only checklist reads as empty, DM items and every threshold are removed,
// and so is a link to a page v cannot open (the item stays, unlinked); the
// links that remain get the page's name and path. hidden reports that
// something was removed that v could not have put back, which makes the
// block read-only for v, as the table and page map are. A link to a page that
// no longer exists is dropped for everyone and locks nothing. It copies.
func (s *service) checklistForViewer(ctx context.Context, campaignID string, d Checklist, v Viewer) (out Checklist, hidden bool, err error) {
	out = d
	if !seesDMOnly(v) {
		if d.DMOnly {
			e := emptyChecklist()
			e.SideA, e.SideB = "", ""
			return e, true, nil
		}
		hidden = len(d.Thresholds) > 0
		out.Thresholds = []Threshold{}
		out.Items = make([]ChecklistItem, 0, len(d.Items))
		for _, it := range d.Items {
			if it.DM {
				hidden = true
				continue
			}
			out.Items = append(out.Items, it)
		}
	} else {
		out.Items = append([]ChecklistItem(nil), d.Items...)
	}
	ids := checklistEntityIDs(out)
	if len(ids) == 0 {
		return out, hidden, nil
	}
	links, err := s.entities.FilterViewable(ctx, campaignID, ids, v)
	if err != nil {
		return Checklist{}, false, apperrorInternal(err)
	}
	unseen, err := s.unseenLivePages(ctx, campaignID, ids, links)
	if err != nil {
		return Checklist{}, false, err
	}
	for i := range out.Items {
		it := &out.Items[i]
		if it.EntityID == "" {
			continue
		}
		l, ok := links[it.EntityID]
		if !ok {
			if unseen[it.EntityID] {
				hidden = true
			}
			it.EntityID = ""
			continue
		}
		it.EntityName, it.EntityURL = l.Name, l.URL
	}
	return out, hidden, nil
}

// checkChecklistWrite enforces, for a save by v, that v neither erases nor
// creates what they cannot see, and links only to pages they can open.
func (s *service) checkChecklistWrite(ctx context.Context, campaignID string, v Viewer, prevData []byte, found bool, next Checklist) error {
	var prev Checklist
	if found {
		var err error
		if prev, err = validateChecklist(prevData); err != nil {
			return apperrorInternal(err)
		}
	}
	if !seesDMOnly(v) {
		if hasDMChecklist(prev) {
			return errForbidden("only the DM can edit a checklist with DM-only parts")
		}
		if hasDMChecklist(next) {
			return errForbidden("only the DM can hide a checklist, its items or its thresholds")
		}
	}
	if ids := checklistEntityIDs(prev); len(ids) > 0 {
		links, err := s.entities.FilterViewable(ctx, campaignID, ids, v)
		if err != nil {
			return apperrorInternal(err)
		}
		unseen, err := s.unseenLivePages(ctx, campaignID, ids, links)
		if err != nil {
			return err
		}
		if len(unseen) > 0 {
			return errForbidden("this checklist links to pages you can't open, so you can't edit it")
		}
	}
	if ids := checklistEntityIDs(next); len(ids) > 0 {
		links, err := s.entities.FilterViewable(ctx, campaignID, ids, v)
		if err != nil {
			return apperrorInternal(err)
		}
		if len(links) < len(ids) {
			return errInvalid("an item links to a page that doesn't exist or that you can't open")
		}
	}
	return nil
}
