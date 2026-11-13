package designs

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// LinkPath is where a design link lives; the token follows it.
const LinkPath = "/designs/link/"

// BuildCode writes a design as a share code. It is unsigned: whoever reads it
// validates it again, so a hand-edited code gains nothing.
func BuildCode(d *Design) (string, error) {
	raw, err := json.Marshal(codeBody{Kind: d.Kind, SystemID: d.SystemID, Name: d.Name, MakerName: d.MakerName, Data: d.Data})
	if err != nil {
		return "", err
	}
	return CodePrefix + base64.RawURLEncoding.EncodeToString(raw), nil
}

// decodeCode reads a pasted code. Whitespace is dropped first, since a code
// copied from a message often arrives wrapped. Every refusal is a plain
// sentence; the size is checked before decoding so a huge paste costs nothing.
func decodeCode(code string) (*codeBody, error) {
	code = strings.Join(strings.Fields(code), "")
	if !strings.HasPrefix(code, CodePrefix) {
		return nil, apperror.NewBadRequest("that is not a Chronicle design code; it should start with " + CodePrefix)
	}
	enc := strings.TrimRight(strings.TrimPrefix(code, CodePrefix), "=")
	if len(enc) > MaxCodeBytes*4/3+4 {
		return nil, apperror.NewBadRequest("that design code is too large")
	}
	raw, err := base64.RawURLEncoding.DecodeString(enc)
	if err != nil {
		return nil, apperror.NewBadRequest("that design code is damaged; copy it again in full")
	}
	if len(raw) > MaxCodeBytes {
		return nil, apperror.NewBadRequest("that design code is too large")
	}
	var b codeBody
	if err := json.Unmarshal(raw, &b); err != nil {
		return nil, apperror.NewBadRequest("that design code is damaged; copy it again in full")
	}
	return &b, nil
}

func (s *service) state(ctx context.Context, d *Design) (*ShareState, error) {
	token, on, owners, err := s.repo.Share(ctx, d.ID)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	code, err := BuildCode(d)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	st := &ShareState{Code: code, LinkOn: on && token != "", Owners: owners}
	if st.LinkOn {
		st.LinkURL = LinkPath + token
	}
	return st, nil
}

// shareable is the gate for every sharing action on one design.
func (s *service) shareable(ctx context.Context, campaignID, userID, id string) (*Design, error) {
	if _, _, err := s.requireOwner(ctx, campaignID, userID); err != nil {
		return nil, err
	}
	return s.own(ctx, campaignID, id)
}

func (s *service) ShareState(ctx context.Context, campaignID, userID, id string) (*ShareState, error) {
	d, err := s.shareable(ctx, campaignID, userID, id)
	if err != nil {
		return nil, err
	}
	return s.state(ctx, d)
}

// SetLink turns the link on or off. Turning it on mints a fresh token, so a
// link that was switched off never works again; asking for on while it is
// already on changes nothing.
func (s *service) SetLink(ctx context.Context, campaignID, userID, id string, on bool) (*ShareState, error) {
	d, err := s.shareable(ctx, campaignID, userID, id)
	if err != nil {
		return nil, err
	}
	token, wasOn, _, err := s.repo.Share(ctx, d.ID)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	switch {
	case on && (!wasOn || token == ""):
		if token, err = newToken(); err != nil {
			return nil, apperror.NewInternal(err)
		}
		err = s.repo.SetLink(ctx, d.ID, token, true)
	case !on:
		err = s.repo.SetLink(ctx, d.ID, "", false)
	}
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	return s.state(ctx, d)
}

// SearchOwners finds other owners on this instance by display name. A query
// shorter than two characters finds nobody, so the list cannot be walked.
func (s *service) SearchOwners(ctx context.Context, campaignID, userID, query string) ([]OwnerMatch, error) {
	if _, _, err := s.requireOwner(ctx, campaignID, userID); err != nil {
		return nil, err
	}
	query = cleanText(query)
	if len([]rune(query)) < searchMinRunes {
		return []OwnerMatch{}, nil
	}
	out, err := s.repo.SearchOwners(ctx, query, userID, searchMaxOwners)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	if out == nil {
		out = []OwnerMatch{}
	}
	return out, nil
}

// AddOwner names another owner as a recipient. They must own a campaign on the
// design's game system, because that is the only place they could copy it to;
// anyone else is refused now rather than left with a design they cannot take.
func (s *service) AddOwner(ctx context.Context, campaignID, userID, id, ownerUserID string) (*ShareState, error) {
	d, err := s.shareable(ctx, campaignID, userID, id)
	if err != nil {
		return nil, err
	}
	if ownerUserID == "" || ownerUserID == userID {
		return nil, apperror.NewBadRequest("name another owner to share with")
	}
	camps, err := s.dir.OwnedCampaigns(ctx, ownerUserID)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	if len(camps) == 0 {
		return nil, apperror.NewBadRequest("that person does not own a campaign on this server")
	}
	same := false
	for _, c := range camps {
		same = same || c.SystemID == d.SystemID
	}
	if !same {
		name, _, _ := s.dir.Member(ctx, camps[0].ID, ownerUserID)
		if name == "" {
			name = "That owner"
		}
		return nil, apperror.NewBadRequest(fmt.Sprintf("%s has no campaign on this game system, so they could not copy this design", name))
	}
	if err := s.repo.AddOwner(ctx, d.ID, ownerUserID); err != nil {
		return nil, apperror.NewInternal(err)
	}
	return s.state(ctx, d)
}

func (s *service) RemoveOwner(ctx context.Context, campaignID, userID, id, ownerUserID string) (*ShareState, error) {
	d, err := s.shareable(ctx, campaignID, userID, id)
	if err != nil {
		return nil, err
	}
	if err := s.repo.RemoveOwner(ctx, d.ID, ownerUserID); err != nil {
		return nil, apperror.NewInternal(err)
	}
	return s.state(ctx, d)
}

// StopSharing turns the link off and removes every named owner. Copies already
// taken are their owners' own rows and stay.
func (s *service) StopSharing(ctx context.Context, campaignID, userID, id string) (*ShareState, error) {
	d, err := s.shareable(ctx, campaignID, userID, id)
	if err != nil {
		return nil, err
	}
	if err := s.repo.StopSharing(ctx, d.ID); err != nil {
		return nil, apperror.NewInternal(err)
	}
	return s.state(ctx, d)
}
