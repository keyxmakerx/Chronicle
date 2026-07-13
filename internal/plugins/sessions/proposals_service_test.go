package sessions

import (
	"context"
	"testing"
	"time"
)

func mustUTC(t *testing.T, s string) time.Time {
	t.Helper()
	tm, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatalf("parse %q: %v", s, err)
	}
	return tm.UTC()
}

// --- renderLocalSlot: UTC instant -> viewer zone (cross-zone + DST) ---

func TestRenderLocalSlot_CrossZoneAndDST(t *testing.T) {
	ny, _ := time.LoadLocation("America/New_York")

	// Summer (EDT, UTC-4): 23:00Z -> 7:00 PM.
	summer := renderLocalSlot(mustUTC(t, "2026-07-18T23:00:00Z"), mustUTC(t, "2026-07-19T01:00:00Z"), ny, "America/New_York")
	if summer.DateLabel != "Sat, Jul 18" {
		t.Errorf("summer DateLabel = %q, want 'Sat, Jul 18'", summer.DateLabel)
	}
	if summer.TimeLabel != "7:00 PM – 9:00 PM" {
		t.Errorf("summer TimeLabel = %q, want '7:00 PM – 9:00 PM'", summer.TimeLabel)
	}

	// Winter (EST, UTC-5): the SAME 23:00Z wall-clock is now 6:00 PM — proves the
	// render honors the DST offset for the instant, not a fixed offset.
	winter := renderLocalSlot(mustUTC(t, "2026-01-10T23:00:00Z"), mustUTC(t, "2026-01-11T01:00:00Z"), ny, "America/New_York")
	if winter.TimeLabel != "6:00 PM – 8:00 PM" {
		t.Errorf("winter TimeLabel = %q, want '6:00 PM – 8:00 PM' (DST offset applied)", winter.TimeLabel)
	}

	// A slot crossing local midnight names the end date.
	crossing := renderLocalSlot(mustUTC(t, "2026-07-19T03:00:00Z"), mustUTC(t, "2026-07-19T05:00:00Z"), ny, "America/New_York")
	// 03:00Z -> 11:00 PM Sat; 05:00Z -> 1:00 AM Sun.
	if crossing.TimeLabel != "11:00 PM – 1:00 AM (Sun, Jul 19)" {
		t.Errorf("crossing TimeLabel = %q, want the cross-midnight form", crossing.TimeLabel)
	}
}

// --- CreateProposal: validation + wall-clock -> UTC conversion ---

func TestCreateProposal_ConvertsWallClockToUTC(t *testing.T) {
	var captured []SlotProposalOption
	repo := &mockSessionRepo{
		createProposalFn: func(_ context.Context, _ *SlotProposal, options []SlotProposalOption) error {
			captured = options
			return nil
		},
	}
	svc := NewSessionService(repo, nil)
	// 7pm–11pm EDT on 2026-07-18 -> 23:00Z .. 03:00Z(+1).
	_, err := svc.CreateProposal(context.Background(), "camp-1", "dm-1", CreateProposalRequest{
		Title: "Next session",
		TZ:    "America/New_York",
		Options: []ProposalOptionInput{
			{Date: "2026-07-18", StartMinute: 19 * 60, EndMinute: 23 * 60},
		},
	})
	if err != nil {
		t.Fatalf("CreateProposal error: %v", err)
	}
	if len(captured) != 1 {
		t.Fatalf("expected 1 option, got %d", len(captured))
	}
	if !captured[0].StartsAtUTC.Equal(mustUTC(t, "2026-07-18T23:00:00Z")) {
		t.Errorf("StartsAtUTC = %v, want 2026-07-18T23:00:00Z", captured[0].StartsAtUTC)
	}
	if !captured[0].EndsAtUTC.Equal(mustUTC(t, "2026-07-19T03:00:00Z")) {
		t.Errorf("EndsAtUTC = %v, want 2026-07-19T03:00:00Z", captured[0].EndsAtUTC)
	}
	if captured[0].Ordinal != 1 {
		t.Errorf("Ordinal = %d, want 1", captured[0].Ordinal)
	}
}

func TestCreateProposal_Validation(t *testing.T) {
	svc := NewSessionService(&mockSessionRepo{}, nil)
	base := func() CreateProposalRequest {
		return CreateProposalRequest{Title: "T", TZ: "UTC", Options: []ProposalOptionInput{{Date: "2026-07-18", StartMinute: 60, EndMinute: 120}}}
	}
	cases := []struct {
		name string
		mut  func(*CreateProposalRequest)
	}{
		{"empty title", func(r *CreateProposalRequest) { r.Title = "  " }},
		{"no options", func(r *CreateProposalRequest) { r.Options = nil }},
		{"too many options", func(r *CreateProposalRequest) {
			r.Options = make([]ProposalOptionInput, 6)
			for i := range r.Options {
				r.Options[i] = ProposalOptionInput{Date: "2026-07-18", StartMinute: 60, EndMinute: 120}
			}
		}},
		{"bad tz", func(r *CreateProposalRequest) { r.TZ = "Not/AZone" }},
		{"bad date", func(r *CreateProposalRequest) { r.Options[0].Date = "nope" }},
		{"inverted range", func(r *CreateProposalRequest) { r.Options[0].StartMinute = 120; r.Options[0].EndMinute = 60 }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := base()
			tc.mut(&req)
			if _, err := svc.CreateProposal(context.Background(), "c", "u", req); err == nil {
				t.Errorf("%s: expected error, got nil", tc.name)
			}
		})
	}
}

// --- RespondToOption: response validation, IDOR, closed ---

func TestRespondToOption_IDORAndClosed(t *testing.T) {
	openProposal := &SlotProposal{ID: "p1", CampaignID: "c1", Status: ProposalOpen}
	closedProposal := &SlotProposal{ID: "p1", CampaignID: "c1", Status: ProposalClosed}

	t.Run("option from another proposal is rejected", func(t *testing.T) {
		repo := &mockSessionRepo{
			getProposalFn: func(_ context.Context, _, _ string) (*SlotProposal, []SlotProposalOption, error) {
				return openProposal, nil, nil
			},
			findOptionFn: func(_ context.Context, _ string) (*SlotProposalOption, error) {
				return &SlotProposalOption{ID: "o9", ProposalID: "OTHER"}, nil
			},
		}
		svc := NewSessionService(repo, nil)
		if err := svc.RespondToOption(context.Background(), "c1", "p1", "o9", "u1", ResponseYes); err == nil {
			t.Error("expected IDOR rejection for mismatched option/proposal")
		}
	})

	t.Run("closed proposal rejects response", func(t *testing.T) {
		repo := &mockSessionRepo{
			getProposalFn: func(_ context.Context, _, _ string) (*SlotProposal, []SlotProposalOption, error) {
				return closedProposal, nil, nil
			},
		}
		svc := NewSessionService(repo, nil)
		if err := svc.RespondToOption(context.Background(), "c1", "p1", "o1", "u1", ResponseYes); err == nil {
			t.Error("expected rejection for closed proposal")
		}
	})

	t.Run("invalid response value rejected", func(t *testing.T) {
		svc := NewSessionService(&mockSessionRepo{}, nil)
		if err := svc.RespondToOption(context.Background(), "c1", "p1", "o1", "u1", "perhaps"); err == nil {
			t.Error("expected rejection for invalid response value")
		}
	})

	t.Run("valid response upserts", func(t *testing.T) {
		var upserted *SlotProposalResponse
		repo := &mockSessionRepo{
			getProposalFn: func(_ context.Context, _, _ string) (*SlotProposal, []SlotProposalOption, error) {
				return openProposal, nil, nil
			},
			findOptionFn: func(_ context.Context, id string) (*SlotProposalOption, error) {
				return &SlotProposalOption{ID: id, ProposalID: "p1"}, nil
			},
			upsertProposalResponseFn: func(_ context.Context, r *SlotProposalResponse) error { upserted = r; return nil },
		}
		svc := NewSessionService(repo, nil)
		if err := svc.RespondToOption(context.Background(), "c1", "p1", "o1", "u1", ResponseMaybe); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if upserted == nil || upserted.Response != ResponseMaybe || upserted.UserID != "u1" || upserted.OptionID != "o1" {
			t.Errorf("upserted = %+v, want option o1/user u1/maybe", upserted)
		}
	})
}

// --- Token redemption ---

func TestRedeemProposalToken(t *testing.T) {
	future := time.Now().UTC().Add(time.Hour)
	past := time.Now().UTC().Add(-time.Hour)
	used := time.Now().UTC().Add(-time.Minute)

	t.Run("expired rejected", func(t *testing.T) {
		repo := &mockSessionRepo{findProposalTokenFn: func(_ context.Context, _ string) (*SlotProposalToken, error) {
			return &SlotProposalToken{Token: "x", OptionID: "o1", UserID: "u1", Response: ResponseYes, ExpiresAt: past}, nil
		}}
		svc := NewSessionService(repo, nil)
		if _, err := svc.RedeemProposalToken(context.Background(), "x"); err == nil {
			t.Error("expected expiry rejection")
		}
	})

	t.Run("already used rejected", func(t *testing.T) {
		repo := &mockSessionRepo{findProposalTokenFn: func(_ context.Context, _ string) (*SlotProposalToken, error) {
			return &SlotProposalToken{Token: "x", OptionID: "o1", UserID: "u1", Response: ResponseYes, ExpiresAt: future, UsedAt: &used}, nil
		}}
		svc := NewSessionService(repo, nil)
		if _, err := svc.RedeemProposalToken(context.Background(), "x"); err == nil {
			t.Error("expected used-token rejection")
		}
	})

	t.Run("valid applies response and marks used", func(t *testing.T) {
		var upserted *SlotProposalResponse
		marked := ""
		repo := &mockSessionRepo{
			findProposalTokenFn: func(_ context.Context, _ string) (*SlotProposalToken, error) {
				return &SlotProposalToken{Token: "tok", OptionID: "o1", UserID: "u1", Response: ResponseNo, ExpiresAt: future}, nil
			},
			upsertProposalResponseFn: func(_ context.Context, r *SlotProposalResponse) error { upserted = r; return nil },
			markProposalTokenUsedFn:  func(_ context.Context, tok string) error { marked = tok; return nil },
		}
		svc := NewSessionService(repo, nil)
		if _, err := svc.RedeemProposalToken(context.Background(), "tok"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if upserted == nil || upserted.Response != ResponseNo {
			t.Errorf("upserted = %+v, want response no", upserted)
		}
		if marked != "tok" {
			t.Errorf("marked = %q, want 'tok'", marked)
		}
	})
}

func TestCreateProposalTokens_ThreeResponses(t *testing.T) {
	var created []*SlotProposalToken
	repo := &mockSessionRepo{createProposalTokenFn: func(_ context.Context, tok *SlotProposalToken) error {
		created = append(created, tok)
		return nil
	}}
	svc := NewSessionService(repo, nil)
	toks, err := svc.CreateProposalTokens(context.Background(), "o1", "u1")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	for _, r := range []string{ResponseYes, ResponseNo, ResponseMaybe} {
		if toks[r] == "" {
			t.Errorf("missing token for %q", r)
		}
	}
	if len(created) != 3 {
		t.Errorf("created %d tokens, want 3", len(created))
	}
}

// --- GetProposalView: tallies, own response, owner detail gating ---

func TestGetProposalView_TalliesAndDetailGating(t *testing.T) {
	opts := []SlotProposalOption{
		{ID: "o1", ProposalID: "p1", StartsAtUTC: mustUTC(t, "2026-07-18T23:00:00Z"), EndsAtUTC: mustUTC(t, "2026-07-19T01:00:00Z"), Ordinal: 1},
	}
	responses := []SlotProposalResponse{
		{OptionID: "o1", UserID: "u1", Response: ResponseYes},
		{OptionID: "o1", UserID: "u2", Response: ResponseNo},
		{OptionID: "o1", UserID: "viewer", Response: ResponseMaybe},
	}
	newRepo := func() *mockSessionRepo {
		return &mockSessionRepo{
			getProposalFn: func(_ context.Context, _, _ string) (*SlotProposal, []SlotProposalOption, error) {
				return &SlotProposal{ID: "p1", CampaignID: "c1", Title: "T", Status: ProposalOpen}, opts, nil
			},
			listProposalResponsesFn: func(_ context.Context, _ string) ([]SlotProposalResponse, error) { return responses, nil },
		}
	}

	t.Run("member sees counts + own response but no responders", func(t *testing.T) {
		svc := NewSessionService(newRepo(), nil)
		v, err := svc.GetProposalView(context.Background(), "c1", "p1", "viewer", "UTC", false)
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		o := v.Options[0]
		if o.YesCount != 1 || o.NoCount != 1 || o.MaybeCount != 1 {
			t.Errorf("tallies = %d/%d/%d, want 1/1/1", o.YesCount, o.NoCount, o.MaybeCount)
		}
		if o.MyResponse != ResponseMaybe {
			t.Errorf("MyResponse = %q, want maybe", o.MyResponse)
		}
		if len(o.Responders) != 0 {
			t.Errorf("non-owner leaked %d responders", len(o.Responders))
		}
	})

	t.Run("owner sees per-responder detail", func(t *testing.T) {
		svc := NewSessionService(newRepo(), nil)
		v, err := svc.GetProposalView(context.Background(), "c1", "p1", "viewer", "UTC", true)
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if len(v.Options[0].Responders) != 3 {
			t.Errorf("owner Responders = %d, want 3", len(v.Options[0].Responders))
		}
	})
}

// --- Notifications ---

func TestNotifyProposalCreated_WritesPerRecipient(t *testing.T) {
	var written []*Notification
	repo := &mockSessionRepo{createNotificationFn: func(_ context.Context, n *Notification) error {
		written = append(written, n)
		return nil
	}}
	svc := NewSessionService(repo, nil)
	if err := svc.NotifyProposalCreated(context.Background(), "c1", "p1", "Next session", []string{"a", "b", ""}); err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(written) != 2 {
		t.Fatalf("wrote %d notifications, want 2 (empty id skipped)", len(written))
	}
	for _, n := range written {
		if n.Type != NotifProposalCreated || n.Link == nil || *n.Link != "/campaigns/c1/proposals/p1" {
			t.Errorf("notification = %+v, want proposal_created linked to the proposal", n)
		}
	}
}

func TestNotifyProposalResponse_NotifiesCreator(t *testing.T) {
	var written *Notification
	repo := &mockSessionRepo{
		getProposalFn: func(_ context.Context, _, _ string) (*SlotProposal, []SlotProposalOption, error) {
			return &SlotProposal{ID: "p1", CampaignID: "c1", CreatedBy: "dm-1", Title: "T"}, nil, nil
		},
		createNotificationFn: func(_ context.Context, n *Notification) error { written = n; return nil },
	}
	svc := NewSessionService(repo, nil)
	if err := svc.NotifyProposalResponse(context.Background(), "c1", "p1", "Bianca", ResponseYes); err != nil {
		t.Fatalf("error: %v", err)
	}
	if written == nil || written.UserID != "dm-1" || written.Type != NotifProposalResponse {
		t.Errorf("notification = %+v, want a proposal_response to dm-1", written)
	}
}
