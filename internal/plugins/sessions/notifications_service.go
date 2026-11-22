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
// generic (see NotifyUsers), and people's notification choices filter every
// write (bellRecipients). No digests, no per-user websockets.

// notificationPayload is the small render context stored as JSON on each row.
type notificationPayload struct {
	Message string `json:"message"`
	Kind    string `json:"kind"`
	// Detail is an optional second line, shown under the message.
	Detail string `json:"detail,omitempty"`
	// Action is an optional label for the row's link, shown under the detail
	// ("Give Brannoc to someone").
	Action string `json:"action,omitempty"`
	// State is opaque text the writer of a folded row keeps with it, so its
	// next write can fold into this one without parsing the words above. It is
	// never shown or served: the bell list reads only the fields above.
	State string `json:"state,omitempty"`
}

// marshalPayload builds the JSON payload string for a notification.
func marshalPayload(message, kind string) *string {
	return marshalPayloadDetail(message, kind, "")
}

// marshalPayloadDetail is marshalPayload with the optional second line.
func marshalPayloadDetail(message, kind, detail string) *string {
	return marshalPayloadAction(message, kind, detail, "")
}

// marshalPayloadAction is marshalPayloadDetail with a label for the link.
func marshalPayloadAction(message, kind, detail, action string) *string {
	b, err := json.Marshal(notificationPayload{Message: message, Kind: kind, Detail: detail, Action: action})
	if err != nil {
		return nil
	}
	s := string(b)
	return &s
}

// marshalPayloadState is marshalPayloadAction with the writer's kept state.
func marshalPayloadState(message, kind, detail, state string) *string {
	b, err := json.Marshal(notificationPayload{Message: message, Kind: kind, Detail: detail, State: state})
	if err != nil {
		return nil
	}
	s := string(b)
	return &s
}

// foldAttempts bounds the retries when a recipient's row changes under a fold.
const foldAttempts = 3

// FoldUnreadNotification keeps one unread row of ntype and link per
// recipient, however often it is written: when the recipient already has an
// unread row, fold gets the state that row kept and its answer rewrites the
// row (and moves it to the newest); otherwise fold gets "" and its answer is
// a new row. Once the recipient has read the row, the next write starts a
// fresh one. fold returns the message, the second line and the state to keep,
// and is called once per recipient, so each keeps their own.
//
// The store knows nothing of what is folded: a feature outside the scheduler
// owns the meaning of the state, as it owns its type constant.
func (s *sessionService) FoldUnreadNotification(ctx context.Context, userIDs []string, campaignID, ntype, link string, fold func(kept string) (message, detail, state string)) error {
	if ntype == "" || link == "" {
		return apperror.NewValidation("notification type and link are required")
	}
	for _, uid := range s.bellRecipients(ctx, userIDs, ntype) {
		if uid == "" {
			continue
		}
		if err := s.foldFor(ctx, uid, campaignID, ntype, link, fold); err != nil {
			return err
		}
	}
	return nil
}

func (s *sessionService) foldFor(ctx context.Context, uid, campaignID, ntype, link string, fold func(string) (string, string, string)) error {
	for range foldAttempts {
		cur, err := s.repo.FindUnreadNotification(ctx, uid, ntype, link)
		if err != nil {
			return apperror.NewInternal(fmt.Errorf("finding unread notification: %w", err))
		}
		if cur == nil {
			break
		}
		var kept notificationPayload
		if cur.Payload != nil {
			// A row whose payload can't be read starts over rather than
			// failing the writer's save.
			_ = json.Unmarshal([]byte(*cur.Payload), &kept)
		}
		message, detail, state := fold(kept.State)
		ok, err := s.repo.RewriteUnreadNotification(ctx, cur.ID, cur.Payload, marshalPayloadState(message, ntype, detail, state), time.Now().UTC())
		if err != nil {
			return apperror.NewInternal(fmt.Errorf("folding notification: %w", err))
		}
		if ok {
			return nil
		}
	}
	message, detail, state := fold("")
	cid, l := campaignID, link
	n := &Notification{
		ID:         generateUUID(),
		UserID:     uid,
		CampaignID: &cid,
		Type:       ntype,
		Payload:    marshalPayloadState(message, ntype, detail, state),
		Link:       &l,
		CreatedAt:  time.Now().UTC(),
	}
	if err := s.repo.CreateNotification(ctx, n); err != nil {
		return apperror.NewInternal(fmt.Errorf("writing notification: %w", err))
	}
	return nil
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
	return s.NotifyUsersWithAction(ctx, userIDs, campaignID, ntype, message, detail, "", link)
}

// NotifyUsersWithAction is NotifyUsersWithDetail with a label for the link,
// for a note whose point is the place it leads to.
func (s *sessionService) NotifyUsersWithAction(ctx context.Context, userIDs []string, campaignID, ntype, message, detail, action, link string) error {
	if ntype == "" {
		return apperror.NewValidation("notification type is required")
	}
	payload := marshalPayloadAction(message, ntype, detail, action)
	now := time.Now().UTC()
	cid := campaignID
	for _, uid := range s.bellRecipients(ctx, userIDs, ntype) {
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
	for _, uid := range s.bellRecipients(ctx, recipientIDs, NotifProposalCreated) {
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
	if len(s.bellRecipients(ctx, []string{p.CreatedBy}, NotifProposalResponse)) == 0 {
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
	var ids []string
	for _, r := range responses {
		if r.UserID == "" || seen[r.UserID] {
			continue
		}
		seen[r.UserID] = true
		ids = append(ids, r.UserID)
	}
	for _, uid := range s.bellRecipients(ctx, ids, NotifProposalConfirmed) {
		n := &Notification{
			ID:         generateUUID(),
			UserID:     uid,
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
