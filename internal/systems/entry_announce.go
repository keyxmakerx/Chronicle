package systems

import "context"

// EntryChangeAnnouncer hears the campaign and field of every campaign entry
// that is made, saved or deleted, so a connected Foundry world can refresh
// the items it made from that field's entries. It is told the field only,
// never the entry, because a Directors-only entry's name is not for players.
type EntryChangeAnnouncer interface {
	EntryChanged(campaignID, fieldKey string)
}

// WithEntryChanges announces every write to the campaign's entries. It wraps
// the repository, so every path that writes an entry announces it, including
// ones added later.
func WithEntryChanges(a EntryChangeAnnouncer) EntryServiceOption {
	return func(s *systemEntryService) {
		if a != nil {
			s.repo = announcingEntryRepo{SystemEntryRepository: s.repo, announce: a}
		}
	}
}

// announcingEntryRepo announces each write after it succeeds.
type announcingEntryRepo struct {
	SystemEntryRepository
	announce EntryChangeAnnouncer
}

func (r announcingEntryRepo) Create(ctx context.Context, e *SystemEntry) error {
	if err := r.SystemEntryRepository.Create(ctx, e); err != nil {
		return err
	}
	r.announce.EntryChanged(e.CampaignID, e.FieldKey)
	return nil
}

func (r announcingEntryRepo) Update(ctx context.Context, e *SystemEntry) error {
	if err := r.SystemEntryRepository.Update(ctx, e); err != nil {
		return err
	}
	r.announce.EntryChanged(e.CampaignID, e.FieldKey)
	return nil
}

// Delete reads the entry first for its field; an entry that can't be read is
// still deleted, just not announced.
func (r announcingEntryRepo) Delete(ctx context.Context, campaignID string, id int64) (bool, error) {
	e, _ := r.Get(ctx, campaignID, id)
	ok, err := r.SystemEntryRepository.Delete(ctx, campaignID, id)
	if err == nil && ok && e != nil {
		r.announce.EntryChanged(campaignID, e.FieldKey)
	}
	return ok, err
}
