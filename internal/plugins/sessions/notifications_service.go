package sessions

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// Scheduler-scoped notification business logic (C-SCHED-P2). The scheduler is
// the only writer this slice: new proposals notify members; received responses
// notify the proposer. The store itself is generic (T-B2) but no other feature
// subscribes yet — no prefs, no digests, no per-user websockets (RC-12.5).

// notificationPayload is the small render context stored as JSON on each row.
type notificationPayload struct {
	Message string `json:"message"`
	Kind    string `json:"kind"`
}

// marshalPayload builds the JSON payload string for a notification.
func marshalPayload(message, kind string) *string {
	b, err := json.Marshal(notificationPayload{Message: message, Kind: kind})
	if err != nil {
		return nil
	}
	s := string(b)
	return &s
}

// proposalLink is the in-app URL a proposal notification points to.
func proposalLink(campaignID, proposalID string) string {
	return fmt.Sprintf("/campaigns/%s/proposals/%s", campaignID, proposalID)
}

// NotifyProposalCreated writes a "new proposal" notification to each recipient
// (the handler supplies the member list, minus the creator).
func (s *sessionService) NotifyProposalCreated(ctx context.Context, campaignID, proposalID, title string, recipientIDs []string) error {
	link := proposalLink(campaignID, proposalID)
	message := fmt.Sprintf("New scheduling proposal: %q", title)
	payload := marshalPayload(message, NotifProposalCreated)
	now := time.Now().UTC()
	cid := campaignID
	for _, uid := range recipientIDs {
		if uid == "" {
			continue
		}
		n := &Notification{
			ID:         generateUUID(),
			UserID:     uid,
			CampaignID: &cid,
			Type:       NotifProposalCreated,
			Payload:    payload,
			Link:       &link,
			CreatedAt:  now,
		}
		if err := s.repo.CreateNotification(ctx, n); err != nil {
			return apperror.NewInternal(fmt.Errorf("writing proposal notification: %w", err))
		}
	}
	return nil
}

// NotifyProposalResponse writes a single notification to the proposal's creator
// that a member responded. Looks the proposal up (campaign-scoped) to find the
// recipient and title.
func (s *sessionService) NotifyProposalResponse(ctx context.Context, campaignID, proposalID, responderName, response string) error {
	p, _, err := s.repo.GetProposal(ctx, campaignID, proposalID)
	if err != nil {
		return err
	}
	if responderName == "" {
		responderName = "A player"
	}
	message := fmt.Sprintf("%s responded %q to %q", responderName, response, p.Title)
	link := proposalLink(campaignID, proposalID)
	cid := campaignID
	n := &Notification{
		ID:         generateUUID(),
		UserID:     p.CreatedBy,
		CampaignID: &cid,
		Type:       NotifProposalResponse,
		Payload:    marshalPayload(message, NotifProposalResponse),
		Link:       &link,
		CreatedAt:  time.Now().UTC(),
	}
	if err := s.repo.CreateNotification(ctx, n); err != nil {
		return apperror.NewInternal(fmt.Errorf("writing response notification: %w", err))
	}
	return nil
}

// NotifyProposalConfirmed writes a "session confirmed" notification to every
// distinct member who responded to the proposal, linking to the newly-created
// session (C-SCHED-P3). Reuses the P2 notification store — no new infra, no
// time-based reminder jobs (none exist; adding one would be a stop-and-flag).
func (s *sessionService) NotifyProposalConfirmed(ctx context.Context, campaignID, proposalID, sessionID string) error {
	p, _, err := s.repo.GetProposal(ctx, campaignID, proposalID)
	if err != nil {
		return err
	}
	responses, err := s.repo.ListProposalResponses(ctx, proposalID)
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("loading responders: %w", err))
	}
	link := fmt.Sprintf("/campaigns/%s/sessions/%s", campaignID, sessionID)
	message := fmt.Sprintf("Session time confirmed for %q", p.Title)
	payload := marshalPayload(message, NotifProposalConfirmed)
	now := time.Now().UTC()
	cid := campaignID
	seen := make(map[string]bool)
	for _, r := range responses {
		if r.UserID == "" || seen[r.UserID] {
			continue
		}
		seen[r.UserID] = true
		n := &Notification{
			ID:         generateUUID(),
			UserID:     r.UserID,
			CampaignID: &cid,
			Type:       NotifProposalConfirmed,
			Payload:    payload,
			Link:       &link,
			CreatedAt:  now,
		}
		if err := s.repo.CreateNotification(ctx, n); err != nil {
			return apperror.NewInternal(fmt.Errorf("writing confirm notification: %w", err))
		}
	}
	return nil
}

// ListMyNotifications returns the current user's notifications (newest first).
func (s *sessionService) ListMyNotifications(ctx context.Context, userID string, limit int) ([]Notification, error) {
	ns, err := s.repo.ListNotifications(ctx, userID, limit)
	if err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("listing notifications: %w", err))
	}
	return ns, nil
}

// CountMyUnreadNotifications returns the current user's unread count.
func (s *sessionService) CountMyUnreadNotifications(ctx context.Context, userID string) (int, error) {
	n, err := s.repo.CountUnreadNotifications(ctx, userID)
	if err != nil {
		return 0, apperror.NewInternal(fmt.Errorf("counting notifications: %w", err))
	}
	return n, nil
}

// MarkNotificationRead marks one of the current user's notifications read.
func (s *sessionService) MarkNotificationRead(ctx context.Context, userID, notificationID string) error {
	if err := s.repo.MarkNotificationRead(ctx, userID, notificationID); err != nil {
		return apperror.NewInternal(fmt.Errorf("marking notification read: %w", err))
	}
	return nil
}

// MarkAllNotificationsRead marks all of the current user's notifications read.
func (s *sessionService) MarkAllNotificationsRead(ctx context.Context, userID string) error {
	if err := s.repo.MarkAllNotificationsRead(ctx, userID); err != nil {
		return apperror.NewInternal(fmt.Errorf("marking all read: %w", err))
	}
	return nil
}
