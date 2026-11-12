package designs

import (
	"context"
	"encoding/json"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// incoming is a design on its way into a campaign, from a code or from another
// campaign's library. Its data has already been through the kind's validator.
type incoming struct {
	kind, name, note, makerName, makerUser string
	systemID                               string
	data                                   json.RawMessage
}

// preview checks an incoming design against the campaign it would join and
// says what that campaign lacks. The kind validates again here, never trusting
// what an earlier preview, a stored row or a pasted code claims.
func (s *service) preview(ctx context.Context, target *CampaignInfo, in incoming) (*Preview, json.RawMessage, error) {
	data, err := s.validData(in.kind, in.data)
	if err != nil {
		return nil, nil, err
	}
	k, _ := s.kind(in.kind)
	lacks, err := k.Lacks(ctx, target.ID, data)
	if err != nil {
		return nil, nil, apperror.NewInternal(err)
	}
	if lacks == nil {
		lacks = []string{}
	}
	return &Preview{
		Name: in.name, Note: in.note, Kind: in.kind, MakerName: in.makerName,
		SystemID: in.systemID, SameSystem: in.systemID == target.SystemID, Lacks: lacks,
	}, data, nil
}

// fromCode turns a decoded share code into an incoming design, cleaning every
// label: the code is unsigned, so its text is whatever the sender typed.
func fromCode(b *codeBody) (incoming, error) {
	name, err := checkLabel("the design's name", b.Name, MaxNameLen, true)
	if err != nil {
		return incoming{}, err
	}
	maker, err := checkLabel("the maker's name", b.MakerName, MaxMakerNameLen, false)
	if err != nil {
		return incoming{}, err
	}
	return incoming{kind: b.Kind, name: name, makerName: maker, systemID: cleanText(b.SystemID), data: b.Data}, nil
}

func (s *service) codeFor(ctx context.Context, campaignID, userID, code string) (*CampaignInfo, incoming, error) {
	c, _, err := s.requireOwner(ctx, campaignID, userID)
	if err != nil {
		return nil, incoming{}, err
	}
	body, err := decodeCode(code)
	if err != nil {
		return nil, incoming{}, err
	}
	in, err := fromCode(body)
	return c, in, err
}

func (s *service) PreviewCode(ctx context.Context, campaignID, userID, code string) (*Preview, error) {
	c, in, err := s.codeFor(ctx, campaignID, userID, code)
	if err != nil {
		return nil, err
	}
	p, _, err := s.preview(ctx, c, in)
	return p, err
}

// TakeCode adds a code's design to the campaign as its own copy. A code from
// another game system is allowed (the preview warns), because the parts may
// still exist here; the kind's check is what refuses an unknown part.
func (s *service) TakeCode(ctx context.Context, campaignID, userID, code string) (*Design, error) {
	c, in, err := s.codeFor(ctx, campaignID, userID, code)
	if err != nil {
		return nil, err
	}
	p, data, err := s.preview(ctx, c, in)
	if err != nil {
		return nil, err
	}
	// The maker of a code is only what its sender typed, so no account is linked.
	return s.store(ctx, c, newDesign{kind: in.kind, name: p.Name, data: data, makerName: in.makerName, origin: OriginCode})
}

// fromDesign reads a stored design as an incoming one; callers check the
// reader's access first.
func fromDesign(d *Design) incoming {
	return incoming{kind: d.Kind, name: d.Name, note: d.Note, makerName: d.MakerName, makerUser: d.MakerUserID, systemID: d.SystemID, data: d.Data}
}

// copyInto stores a copy of a design the reader may see into a campaign they
// own. The game system must match: a design shared by link or by name is
// meant for the same game, and refusing here is what keeps a room theme from
// landing in a game it was not built for.
func (s *service) copyInto(ctx context.Context, target *CampaignInfo, src *Design) (*Design, error) {
	if src.SystemID != target.SystemID {
		return nil, apperror.NewBadRequest(msgOtherSystem)
	}
	in := fromDesign(src)
	p, data, err := s.preview(ctx, target, in)
	if err != nil {
		return nil, err
	}
	ref := ""
	if from, _ := s.dir.Campaign(ctx, src.CampaignID); from != nil {
		ref = from.Name
	}
	return s.store(ctx, target, newDesign{
		kind: in.kind, name: p.Name, note: in.note, data: data,
		makerUser: in.makerUser, makerName: in.makerName, origin: OriginShared, ref: ref,
	})
}

// byLink is the design behind a live link, or NotFound. A link that is off, a
// token that never existed and a token that was replaced all read the same.
func (s *service) byLink(ctx context.Context, token string) (*Design, error) {
	d, on, err := s.repo.FindByToken(ctx, token)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	if d == nil || !on {
		return nil, apperror.NewNotFound(msgLinkGone)
	}
	return d, nil
}

// linkReader checks the reader owns a campaign at all before the token is
// looked at, so a player learns nothing about whether a token is live.
func (s *service) linkReader(ctx context.Context, userID string) ([]CampaignInfo, error) {
	if userID == "" {
		return nil, apperror.NewForbidden(msgNotAnOwner)
	}
	camps, err := s.dir.OwnedCampaigns(ctx, userID)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	if len(camps) == 0 {
		return nil, apperror.NewForbidden(msgNotAnOwner)
	}
	return camps, nil
}

// PreviewLink shows what a link holds and which of the reader's campaigns can
// take it: their own, on the design's game system.
func (s *service) PreviewLink(ctx context.Context, userID, token string) (*LinkPreview, error) {
	camps, err := s.linkReader(ctx, userID)
	if err != nil {
		return nil, err
	}
	d, err := s.byLink(ctx, token)
	if err != nil {
		return nil, err
	}
	out := &LinkPreview{Targets: []LinkTarget{}}
	out.Preview = Preview{Name: d.Name, Note: d.Note, Kind: d.Kind, MakerName: d.MakerName, SystemID: d.SystemID, Lacks: []string{}}
	for _, c := range camps {
		if c.SystemID != d.SystemID {
			continue
		}
		p, _, err := s.preview(ctx, &c, fromDesign(d))
		if err != nil {
			return nil, err
		}
		out.Targets = append(out.Targets, LinkTarget{CampaignInfo: c, Lacks: p.Lacks})
	}
	out.SameSystem = len(out.Targets) > 0
	return out, nil
}

func (s *service) TakeLink(ctx context.Context, userID, token, targetCampaignID string) (*Design, error) {
	if _, err := s.linkReader(ctx, userID); err != nil {
		return nil, err
	}
	d, err := s.byLink(ctx, token)
	if err != nil {
		return nil, err
	}
	target, _, err := s.requireOwner(ctx, targetCampaignID, userID)
	if err != nil {
		return nil, err
	}
	return s.copyInto(ctx, target, d)
}

func (s *service) SharedWithMe(ctx context.Context, campaignID, userID, kind string) ([]SharedDesign, error) {
	c, _, err := s.requireOwner(ctx, campaignID, userID)
	if err != nil {
		return nil, err
	}
	out, err := s.repo.SharedWith(ctx, userID, c.SystemID, kind, campaignID)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	return out, nil
}

// named is a design shared to this user by name, or NotFound: a design that is
// not shared to them is never confirmed to exist.
func (s *service) named(ctx context.Context, userID, designID string) (*Design, error) {
	d, err := s.repo.Get(ctx, designID)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	if d == nil {
		return nil, apperror.NewNotFound(msgNotFound)
	}
	ok, err := s.repo.IsNamedOwner(ctx, d.ID, userID)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	if !ok {
		return nil, apperror.NewNotFound(msgNotFound)
	}
	return d, nil
}

func (s *service) PreviewShared(ctx context.Context, campaignID, userID, designID string) (*Preview, error) {
	c, _, err := s.requireOwner(ctx, campaignID, userID)
	if err != nil {
		return nil, err
	}
	d, err := s.named(ctx, userID, designID)
	if err != nil {
		return nil, err
	}
	p, _, err := s.preview(ctx, c, fromDesign(d))
	return p, err
}

func (s *service) TakeShared(ctx context.Context, campaignID, userID, designID string) (*Design, error) {
	c, _, err := s.requireOwner(ctx, campaignID, userID)
	if err != nil {
		return nil, err
	}
	d, err := s.named(ctx, userID, designID)
	if err != nil {
		return nil, err
	}
	return s.copyInto(ctx, c, d)
}
