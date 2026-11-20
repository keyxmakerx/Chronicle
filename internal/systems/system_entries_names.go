package systems

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// Old names and notes: what keeps a renamed entry reachable from what heroes
// and links stored, and what the players were told about a change.

// refuseOldName refuses a name that is an old name of another entry, saying
// where it leads, so a hero who stored it is never silently moved. selfID is
// the entry being renamed (0 for a new one): its own old names are free to it.
func (s *systemEntryService) refuseOldName(ctx context.Context, campaignID, systemID, fieldKey, name string, selfID int64) error {
	old, err := s.repo.FindOldName(ctx, campaignID, systemID, fieldKey, name)
	if err != nil || old == nil || old.EntryID == selfID {
		return err
	}
	target, err := s.repo.Get(ctx, campaignID, old.EntryID)
	if err != nil {
		return err
	}
	return apperror.NewConflict(fmt.Sprintf("%s is an old name of %s and still leads there. Free the old name first to use it again.", name, target.Name))
}

// recordRename keeps the name an entry gave up as an old name, and takes
// back an old name the entry has just returned to.
func (s *systemEntryService) recordRename(ctx context.Context, e *SystemEntry, prevName string) error {
	if back, err := s.repo.FindOldName(ctx, e.CampaignID, e.SystemID, e.FieldKey, e.Name); err != nil {
		return err
	} else if back != nil && back.EntryID == e.ID {
		if _, err := s.repo.DeleteOldName(ctx, e.CampaignID, back.ID); err != nil {
			return err
		}
	}
	if strings.TrimSpace(prevName) == "" {
		return nil
	}
	return s.repo.AddOldName(ctx, &EntryOldName{CampaignID: e.CampaignID, SystemID: e.SystemID, FieldKey: e.FieldKey, Name: prevName, EntryID: e.ID})
}

func (s *systemEntryService) OldNames(ctx context.Context, campaignID, fieldKey string, actor EntryActor) ([]EntryOldName, error) {
	m := s.manifestOrNil(ctx, campaignID)
	if m == nil || (fieldKey != "" && !ValidChoiceFieldKey(fieldKey)) {
		return []EntryOldName{}, nil
	}
	names, err := s.repo.ListOldNames(ctx, campaignID, m.ID, fieldKey)
	if err != nil || actor.IsDirector {
		return names, err
	}
	// An old name of an entry the reader cannot see would give that entry away.
	entries, err := s.repo.List(ctx, campaignID, m.ID, fieldKey, false)
	if err != nil {
		return nil, err
	}
	if entries, err = s.visible(ctx, campaignID, entries, actor); err != nil {
		return nil, err
	}
	seen := map[int64]bool{}
	for _, e := range entries {
		seen[e.ID] = true
	}
	out := []EntryOldName{}
	for _, n := range names {
		if seen[n.EntryID] {
			out = append(out, n)
		}
	}
	return out, nil
}

func (s *systemEntryService) FreeOldName(ctx context.Context, campaignID string, actor EntryActor, id int64) error {
	if err := requireDirector(actor); err != nil {
		return err
	}
	ok, err := s.repo.DeleteOldName(ctx, campaignID, id)
	if err != nil {
		return err
	}
	if !ok {
		return apperror.NewNotFound("old name not found")
	}
	return nil
}

func (s *systemEntryService) AddNote(ctx context.Context, campaignID string, actor EntryActor, entryID int64, line string) (*EntryNote, error) {
	if err := requireDirector(actor); err != nil {
		return nil, err
	}
	line = strings.Join(strings.Fields(line), " ")
	if line == "" {
		return nil, apperror.NewValidation("Write the line the players will read.")
	}
	if utf8.RuneCountInString(line) > maxNoteLine {
		return nil, apperror.NewValidation(fmt.Sprintf("The line can be at most %d characters.", maxNoteLine))
	}
	if _, err := s.repo.Get(ctx, campaignID, entryID); err != nil {
		return nil, err
	}
	n := &EntryNote{CampaignID: campaignID, EntryID: entryID, Line: line, CreatedBy: actor.UserID}
	if err := s.repo.AddNote(ctx, n); err != nil {
		return nil, err
	}
	return n, nil
}

func (s *systemEntryService) Notes(ctx context.Context, campaignID string, actor EntryActor, entryID int64) ([]EntryNote, error) {
	e, err := s.repo.Get(ctx, campaignID, entryID)
	if err != nil {
		return nil, err
	}
	seen, err := s.visible(ctx, campaignID, []SystemEntry{*e}, actor)
	if err != nil {
		return nil, err
	}
	if len(seen) == 0 {
		return nil, apperror.NewNotFound("entry not found")
	}
	return s.repo.ListNotes(ctx, campaignID, entryID, maxNotesShown)
}
