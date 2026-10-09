package sessions

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// Scheduler-scoped notification business logic: new proposals notify
// members; received responses notify the proposer. The store itself is
// generic (see NotifyUsers) but no other feature subscribes yet — no prefs,
// no digests, no per-user websockets.

// notificationPayload is the small render context stored as JSON on each row.
type notificationPayload struct {
	Message string `json:"message"`
	Kind    string `json:"kind"`
	// Detail is an optional second line, shown under the message.
	Detail string `json:"detail,omitempty"`
}

// marshalPayload builds the JSON payload string for a notification.
func marshalPayload(message, kind string) *string {
	return marshalPayloadDetail(message, kind, "")
}

// marshalPayloadDetail is marshalPayload with the optional second line.
func marshalPayloadDetail(message, kind, detail string) *string {
	b, err := json.Marshal(notificationPayload{Message: message, Kind: kind, Detail: detail})
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

// NotifyUsers writes one notification per recipient with a caller-supplied type,
// message, and in-app link. The generic entry point: it lets a non-scheduler
// feature use the notification store without the scheduler growing a bespoke
// NotifyX per feature and without that feature touching the repository.
// Callers own their own type constant.
//
// Empty recipient ids are skipped rather than rejected, so a caller can pass a
// roster slice that may contain a blank without pre-filtering. A blank ntype is
// rejected: an untyped row would be invisible to any consumer that filters.
func (s *sessionService) NotifyUsers(ctx context.Context, userIDs []string, campaignID, ntype, message, link string) error {
	return s.NotifyUsersWithDetail(ctx, userIDs, campaignID, ntype, message, "", link)
}

// NotifyUsersWithDetail is NotifyUsers with a second line under the message.
func (s *sessionService) NotifyUsersWithDetail(ctx context.Context, userIDs []string, campaignID, ntype, message, detail, link string) error {
	if ntype == "" {
		return apperror.NewValidation("notification type is required")
	}
	payload := marshalPayloadDetail(message, ntype, detail)
	now := time.Now().UTC()
	cid := campaignID
	for _, uid := range userIDs {
		if uid == "" {
			continue
		}
		n := &Notification{
			ID:         generateUUID(),
			UserID:     uid,
			CampaignID: &cid,
			Type:       ntype,
			Payload:    payload,
			CreatedAt:  now,
		}
		if link != "" {
			l := link
			n.Link = &l
		}
		if err := s.repo.CreateNotification(ctx, n); err != nil {
			return apperror.NewInternal(fmt.Errorf("writing notification: %w", err))
		}
	}
	return nil
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

// NotifyProposalResponse tells the proposal's creator that members are
// answering. A player ticks yes/no on every option, so one notification per
// tick buried the creator; instead the creator keeps a single unread row per
// proposal that names the latest responder and how many have answered. The
// creator answering their own proposal notifies nobody.
func (s *sessionService) NotifyProposalResponse(ctx context.Context, campaignID, proposalID, responderID, responderName string) error {
	p, _, err := s.repo.GetProposal(ctx, campaignID, proposalID)
	if err != nil {
		return err
	}
	if responderID == p.CreatedBy {
		return nil
	}
	responses, err := s.repo.ListProposalResponses(ctx, proposalID)
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("loading responders: %w", err))
	}
	if responderName == "" {
		responderName = "A player"
	}
	message := proposalResponseMessage(responderName, p.Title, responses, responderID, p.CreatedBy)
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
	if err := s.repo.MergeUnreadNotification(ctx, n); err != nil {
		return apperror.NewInternal(fmt.Errorf("writing response notification: %w", err))
	}
	return nil
}

// proposalResponseMessage words the creator's one row: the latest responder,
// plus how many other members (never the creator) have answered so far.
func proposalResponseMessage(latest, title string, responses []SlotProposalResponse, latestID, creatorID string) string {
	others := map[string]bool{}
	for _, r := range responses {
		if r.UserID != latestID && r.UserID != creatorID {
			others[r.UserID] = true
		}
	}
	switch len(others) {
	case 0:
		return fmt.Sprintf("%s answered %q", latest, title)
	case 1:
		return fmt.Sprintf("%s and 1 other have answered %q", latest, title)
	default:
		return fmt.Sprintf("%s and %d others have answered %q", latest, len(others), title)
	}
}

// NotifyProposalConfirmed writes a "session confirmed" notification to every
// distinct member who responded to the proposal, linking to the newly-created
// session. Reuses the same notification store — no new infra, no time-based
// reminder jobs (none exist in this product).
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
