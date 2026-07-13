package sessions

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/timeutil"
)

const (
	maxProposalTitleLen = 200
	maxProposalNoteLen  = 2000
	proposalTokenTTL    = 7 * 24 * time.Hour
)

// validateResponse normalizes and checks a per-option response value.
func validateResponse(r string) (string, error) {
	switch r {
	case ResponseYes, ResponseNo, ResponseMaybe:
		return r, nil
	default:
		return "", apperror.NewBadRequest("response must be yes, no, or maybe")
	}
}

// renderLocalSlot projects a UTC option range into a viewer zone for display.
// Pure and zone-driven so the cross-zone / DST render is directly unit-testable
// (RC-12.5: options are UTC instants; the viewer's local time is derived here).
func renderLocalSlot(startUTC, endUTC time.Time, loc *time.Location, tzLabel string) LocalSlot {
	ls := startUTC.In(loc)
	le := endUTC.In(loc)
	timeLabel := fmt.Sprintf("%s – %s", formatClock(ls), formatClock(le))
	// A slot that crosses local midnight names both dates so the range reads
	// unambiguously.
	if ls.YearDay() != le.YearDay() || ls.Year() != le.Year() {
		timeLabel = fmt.Sprintf("%s – %s (%s)", formatClock(ls), formatClock(le), le.Format("Mon, Jan 2"))
	}
	return LocalSlot{
		StartsAtUTC: startUTC.UTC().Format(time.RFC3339),
		EndsAtUTC:   endUTC.UTC().Format(time.RFC3339),
		DateLabel:   ls.Format("Mon, Jan 2"),
		TimeLabel:   timeLabel,
		TZLabel:     tzLabel,
	}
}

// formatClock renders a wall-clock time as e.g. "7:00 PM".
func formatClock(t time.Time) string {
	return t.Format("3:04 PM")
}

// renderLocalSlotForTZ is a convenience wrapper that resolves an IANA zone label
// and projects an option into it — used by the email builder, which works in
// each member's own stored zone.
func renderLocalSlotForTZ(o SlotProposalOption, tz string) LocalSlot {
	return renderLocalSlot(o.StartsAtUTC, o.EndsAtUTC, timeutil.LoadLocation(tz), tz)
}

// CreateProposal validates and stores a DM's proposal with 1..5 candidate slots
// (UTC instants). Notification fan-out is driven by the handler (which has the
// member list); this method only persists.
func (s *sessionService) CreateProposal(ctx context.Context, campaignID, createdBy string, req CreateProposalRequest) (*SlotProposal, error) {
	title := strings.TrimSpace(req.Title)
	if title == "" {
		return nil, apperror.NewBadRequest("a proposal title is required")
	}
	if len(title) > maxProposalTitleLen {
		return nil, apperror.NewBadRequest("proposal title is too long")
	}
	if len(req.Note) > maxProposalNoteLen {
		return nil, apperror.NewBadRequest("proposal note is too long")
	}
	if len(req.Options) < 1 || len(req.Options) > maxProposalOptions {
		return nil, apperror.NewBadRequest("a proposal needs between 1 and 5 candidate slots")
	}
	if !timeutil.IsValidLocation(req.TZ) {
		return nil, apperror.NewBadRequest("a valid IANA timezone is required")
	}
	loc := timeutil.LoadLocation(req.TZ)

	now := time.Now().UTC()
	p := &SlotProposal{
		ID:         generateUUID(),
		CampaignID: campaignID,
		CreatedBy:  createdBy,
		Title:      title,
		Status:     ProposalOpen,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if note := strings.TrimSpace(req.Note); note != "" {
		p.Note = &note
	}

	options := make([]SlotProposalOption, 0, len(req.Options))
	for i, in := range req.Options {
		cd, err := timeutil.ParseCivilDate(in.Date)
		if err != nil {
			return nil, apperror.NewBadRequest("each slot needs a valid date")
		}
		if in.StartMinute < 0 || in.EndMinute <= in.StartMinute || in.EndMinute > timeutil.MinutesPerDay {
			return nil, apperror.NewBadRequest("each slot needs a valid time range")
		}
		// Resolve the viewer-zone wall-clock to an absolute UTC instant — the
		// DST-correct conversion (RC-12.5). endMinute may be 1440 (local
		// midnight); WallClockInstant normalizes it to the next day's 00:00.
		start := timeutil.WallClockInstant(loc, cd.Year, cd.Month, cd.Day, in.StartMinute).UTC()
		end := timeutil.WallClockInstant(loc, cd.Year, cd.Month, cd.Day, in.EndMinute).UTC()
		if !end.After(start) {
			return nil, apperror.NewBadRequest("a slot's end must be after its start")
		}
		options = append(options, SlotProposalOption{
			ID:          generateUUID(),
			ProposalID:  p.ID,
			StartsAtUTC: start,
			EndsAtUTC:   end,
			Ordinal:     i + 1,
		})
	}

	if err := s.repo.CreateProposal(ctx, p, options); err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("creating proposal: %w", err))
	}
	return p, nil
}

// GetProposalView assembles the proposal detail for one viewer: options in the
// viewer's zone with response tallies, the viewer's own choice per option, and —
// when includeDetail (owner) — the per-responder breakdown (names left blank for
// the handler to resolve from the member directory).
func (s *sessionService) GetProposalView(ctx context.Context, campaignID, proposalID, viewerID, viewerTZ string, includeDetail bool) (*ProposalView, error) {
	p, opts, err := s.repo.GetProposal(ctx, campaignID, proposalID)
	if err != nil {
		return nil, err
	}
	responses, err := s.repo.ListProposalResponses(ctx, proposalID)
	if err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("loading responses: %w", err))
	}
	byOption := make(map[string][]SlotProposalResponse)
	for _, r := range responses {
		byOption[r.OptionID] = append(byOption[r.OptionID], r)
	}

	loc := timeutil.LoadLocation(viewerTZ)
	view := &ProposalView{
		Proposal:      *p,
		ViewerTZ:      viewerTZ,
		IncludeDetail: includeDetail,
		Options:       make([]ProposalOptionView, 0, len(opts)),
	}
	for _, o := range opts {
		ov := ProposalOptionView{
			Option: o,
			Local:  renderLocalSlot(o.StartsAtUTC, o.EndsAtUTC, loc, viewerTZ),
		}
		for _, r := range byOption[o.ID] {
			switch r.Response {
			case ResponseYes:
				ov.YesCount++
			case ResponseNo:
				ov.NoCount++
			case ResponseMaybe:
				ov.MaybeCount++
			}
			if r.UserID == viewerID {
				ov.MyResponse = r.Response
			}
			if includeDetail {
				ov.Responders = append(ov.Responders, ProposalResponderView{
					UserID:   r.UserID,
					Response: r.Response,
				})
			}
		}
		view.Options = append(view.Options, ov)
	}
	return view, nil
}

// ListProposalSummaries returns the proposal list for a campaign with per-row
// option/responder counts and whether the viewer has responded.
func (s *sessionService) ListProposalSummaries(ctx context.Context, campaignID, viewerID string) ([]ProposalSummary, error) {
	proposals, err := s.repo.ListProposals(ctx, campaignID)
	if err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("listing proposals: %w", err))
	}
	out := make([]ProposalSummary, 0, len(proposals))
	for i := range proposals {
		p := proposals[i]
		opts, err := s.repo.ListProposalOptions(ctx, p.ID)
		if err != nil {
			return nil, apperror.NewInternal(fmt.Errorf("listing options: %w", err))
		}
		responses, err := s.repo.ListProposalResponses(ctx, p.ID)
		if err != nil {
			return nil, apperror.NewInternal(fmt.Errorf("listing responses: %w", err))
		}
		responders := make(map[string]bool)
		myResponded := false
		for _, r := range responses {
			responders[r.UserID] = true
			if r.UserID == viewerID {
				myResponded = true
			}
		}
		out = append(out, ProposalSummary{
			Proposal:    p,
			OptionCount: len(opts),
			ResponderN:  len(responders),
			MyResponded: myResponded,
		})
	}
	return out, nil
}

// RespondToOption records a member's response to one option after verifying the
// option belongs to the named proposal and the proposal belongs to the campaign
// (IDOR guards) and is still open.
func (s *sessionService) RespondToOption(ctx context.Context, campaignID, proposalID, optionID, userID, response string) error {
	resp, err := validateResponse(response)
	if err != nil {
		return err
	}
	p, _, err := s.repo.GetProposal(ctx, campaignID, proposalID)
	if err != nil {
		return err
	}
	if p.Status == ProposalClosed {
		return apperror.NewBadRequest("this proposal is closed")
	}
	opt, err := s.repo.FindOption(ctx, optionID)
	if err != nil {
		return err
	}
	if opt.ProposalID != proposalID {
		return apperror.NewNotFound("option not found")
	}
	return s.repo.UpsertProposalResponse(ctx, &SlotProposalResponse{
		ID:        generateUUID(),
		OptionID:  optionID,
		UserID:    userID,
		Response:  resp,
		UpdatedAt: time.Now().UTC(),
	})
}

// CreateProposalTokens mints one-click email response tokens (yes/no/maybe) for
// one member on one option. Returns response -> token.
func (s *sessionService) CreateProposalTokens(ctx context.Context, optionID, userID string) (map[string]string, error) {
	now := time.Now().UTC()
	expires := now.Add(proposalTokenTTL)
	out := make(map[string]string, 3)
	for _, resp := range []string{ResponseYes, ResponseNo, ResponseMaybe} {
		tok := generateToken()
		if err := s.repo.CreateProposalToken(ctx, &SlotProposalToken{
			Token:     tok,
			OptionID:  optionID,
			UserID:    userID,
			Response:  resp,
			ExpiresAt: expires,
			CreatedAt: now,
		}); err != nil {
			return nil, apperror.NewInternal(fmt.Errorf("creating proposal token: %w", err))
		}
		out[resp] = tok
	}
	return out, nil
}

// RedeemProposalToken applies a one-click email response and consumes the token.
func (s *sessionService) RedeemProposalToken(ctx context.Context, tokenStr string) (*SlotProposalToken, error) {
	token, err := s.repo.FindProposalToken(ctx, tokenStr)
	if err != nil {
		return nil, err
	}
	if token.UsedAt != nil {
		return nil, apperror.NewBadRequest("this response link has already been used")
	}
	if time.Now().UTC().After(token.ExpiresAt) {
		return nil, apperror.NewBadRequest("this response link has expired")
	}
	if err := s.repo.UpsertProposalResponse(ctx, &SlotProposalResponse{
		ID:        generateUUID(),
		OptionID:  token.OptionID,
		UserID:    token.UserID,
		Response:  token.Response,
		UpdatedAt: time.Now().UTC(),
	}); err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("applying response: %w", err))
	}
	if err := s.repo.MarkProposalTokenUsed(ctx, tokenStr); err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("marking token used: %w", err))
	}
	return token, nil
}
