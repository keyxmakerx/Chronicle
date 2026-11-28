package designs

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

func (s *service) List(ctx context.Context, campaignID, userID, kind string) ([]Design, error) {
	if _, _, err := s.requireOwner(ctx, campaignID, userID); err != nil {
		return nil, err
	}
	out, err := s.repo.List(ctx, campaignID, kind)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	if out == nil {
		out = []Design{}
	}
	return out, nil
}

func (s *service) Get(ctx context.Context, campaignID, userID, id string) (*Design, error) {
	if _, _, err := s.requireOwner(ctx, campaignID, userID); err != nil {
		return nil, err
	}
	return s.own(ctx, campaignID, id)
}

// Check runs Save's checks without saving, for a caller that reviews before it
// writes (the AI's review step).
func (s *service) Check(ctx context.Context, campaignID, userID string, in SaveInput) error {
	_, _, _, _, _, err := s.prepare(ctx, campaignID, userID, in)
	return err
}

// prepare is Save's checks: the Owner gate, the labels, and the kind's own
// validation of the data.
func (s *service) prepare(ctx context.Context, campaignID, userID string, in SaveInput) (c *CampaignInfo, maker, name, note string, data json.RawMessage, err error) {
	if c, maker, err = s.requireOwner(ctx, campaignID, userID); err != nil {
		return
	}
	if name, err = checkLabel("name", in.Name, MaxNameLen, true); err != nil {
		return
	}
	if note, err = checkLabel("note", in.Note, MaxNoteLen, false); err != nil {
		return
	}
	data, err = s.validData(in.Kind, in.Data)
	return
}

// Save puts a design in the library. The campaign's game system is copied onto
// it now, so the design keeps meaning what it meant if the campaign later
// changes system.
func (s *service) Save(ctx context.Context, campaignID, userID string, in SaveInput) (*Design, error) {
	c, maker, name, note, data, err := s.prepare(ctx, campaignID, userID, in)
	if err != nil {
		return nil, err
	}
	return s.store(ctx, c, newDesign{
		kind: in.Kind, name: name, note: note, data: data,
		makerUser: userID, makerName: maker, origin: OriginMade,
	})
}

// newDesign is a validated design waiting to be stored in a campaign.
type newDesign struct {
	kind, name, note     string
	data                 json.RawMessage
	makerUser, makerName string
	origin, ref          string
}

// validData runs the kind's own validator; its refusal is returned as written
// so the Owner is told which part was not known.
func (s *service) validData(kind string, raw json.RawMessage) (json.RawMessage, error) {
	k, err := s.kind(kind)
	if err != nil {
		return nil, err
	}
	if len(raw) == 0 || string(raw) == "null" || len(raw) > MaxDataBytes {
		return nil, apperror.NewBadRequest("design data is missing or too large")
	}
	out, err := k.Validate(raw)
	if err != nil {
		return nil, refusal(err)
	}
	if len(out) > MaxDataBytes {
		return nil, apperror.NewBadRequest("design data is too large")
	}
	return out, nil
}

// refusal turns a kind's plain error into a 400 with its own words: a kind
// that cannot accept the data is refusing it, never faulting the server.
func refusal(err error) error {
	var ae *apperror.AppError
	if errors.As(err, &ae) {
		return err
	}
	return apperror.NewBadRequest(err.Error())
}

// cleanData is what every share and add path runs a payload through: the
// kind's Stripper first, so no Director-only part survives, then Validate, so
// the stripped output is what gets checked and kept.
func (s *service) cleanData(kind string, raw json.RawMessage) (json.RawMessage, error) {
	k, err := s.kind(kind)
	if err != nil {
		return nil, err
	}
	if len(raw) == 0 || string(raw) == "null" || len(raw) > MaxDataBytes {
		return nil, apperror.NewBadRequest("design data is missing or too large")
	}
	stripped, err := stripKind(k, raw)
	if err != nil {
		return nil, refusal(err)
	}
	return s.validData(kind, stripped)
}

// place adds a validated design to the campaign. A kind with an Adder makes
// the thing itself and the library is left alone; every other kind is copied
// into the library as before.
func (s *service) place(ctx context.Context, c *CampaignInfo, userID string, n newDesign) (*Taken, error) {
	if k, err := s.kind(n.kind); err == nil {
		if a, ok := k.(Adder); ok {
			res, err := a.Add(ctx, c.ID, userID, n.data)
			if err != nil {
				var ae *apperror.AppError
				if errors.As(err, &ae) {
					return nil, err
				}
				return nil, apperror.NewInternal(err)
			}
			if res.LeftToPoint == nil {
				res.LeftToPoint = []string{}
			}
			return &Taken{ID: res.ID, LeftToPoint: res.LeftToPoint}, nil
		}
	}
	d, err := s.store(ctx, c, n)
	if err != nil {
		return nil, err
	}
	return &Taken{Design: d, ID: d.ID, LeftToPoint: []string{}}, nil
}

// StoreCatalog is what the Store reads to build its rail and name systems.
func (s *service) StoreCatalog(ctx context.Context, campaignID, userID string) (*StoreCatalog, error) {
	if _, _, err := s.requireOwner(ctx, campaignID, userID); err != nil {
		return nil, err
	}
	out := &StoreCatalog{Kinds: storeGroups(s.kinds), Systems: map[string]string{}, Things: map[string]StoreThing{}}
	for _, d := range describedKinds(s.kinds) {
		if d.Group != "" {
			out.Things[d.kind] = StoreThing{Group: d.Group, One: d.One}
		}
	}
	if s.pkgs != nil {
		for id, name := range s.pkgs.SystemNames(ctx) {
			out.Systems[id] = name
		}
	}
	return out, nil
}

// store inserts a new, independent row in the campaign. Taking a design always
// comes through here, so a copy never shares a row, or a key, with its source.
func (s *service) store(ctx context.Context, c *CampaignInfo, n newDesign) (*Design, error) {
	count, err := s.repo.Count(ctx, c.ID)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	if count >= MaxLibrary {
		return nil, apperror.NewConflict("this campaign's design library is full; delete a design first")
	}
	d := &Design{
		ID: newID(), CampaignID: c.ID, Kind: n.kind, SystemID: c.SystemID,
		Name: n.name, Note: n.note, MakerUserID: n.makerUser, MakerName: n.makerName,
		Data: n.data, Origin: n.origin, OriginRef: n.ref,
	}
	if err := s.repo.Insert(ctx, d); err != nil {
		return nil, apperror.NewInternal(err)
	}
	return s.repo.Get(ctx, d.ID)
}

// Update renames a design or edits its note. Only the keys sent change.
func (s *service) Update(ctx context.Context, campaignID, userID, id string, in UpdateDesignInput) (*Design, error) {
	if _, _, err := s.requireOwner(ctx, campaignID, userID); err != nil {
		return nil, err
	}
	d, err := s.own(ctx, campaignID, id)
	if err != nil {
		return nil, err
	}
	// The merged values are what get checked: an absent name is not an empty one.
	name, err := checkLabel("name", in.Name.Val(d.Name), MaxNameLen, true)
	if err != nil {
		return nil, err
	}
	note := d.Note
	if in.Note.IsNull() {
		note = ""
	} else {
		note = in.Note.Val(note)
	}
	if note, err = checkLabel("note", note, MaxNoteLen, false); err != nil {
		return nil, err
	}
	if err := s.repo.Update(ctx, id, name, note); err != nil {
		return nil, apperror.NewInternal(err)
	}
	return s.repo.Get(ctx, id)
}

// Delete removes the design and its share state. Copies others took stay.
func (s *service) Delete(ctx context.Context, campaignID, userID, id string) error {
	if _, _, err := s.requireOwner(ctx, campaignID, userID); err != nil {
		return err
	}
	if _, err := s.own(ctx, campaignID, id); err != nil {
		return err
	}
	if err := s.repo.Delete(ctx, id); err != nil {
		return apperror.NewInternal(err)
	}
	return nil
}
