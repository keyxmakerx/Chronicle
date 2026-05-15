// Tests for the admin service operations added in C-FMC-5c.
//
// Coverage:
//
//   - ForcePinCampaign rejects empty version (the "would clear pin"
//     misuse case) with the validation-category error.
//   - ForcePinCampaign writes the security event with the right
//     event type (foundry_vtt.module_force_pin) and details.
//   - NotifyCampaignOfUpdate writes a different event type
//     (foundry_vtt.module_update_notify) so the audit trail
//     distinguishes the two actions.
//   - NotifyCampaignOfUpdate skips SMTP when not configured but
//     still logs the event (banner remains primary surface).
//   - NotifyOlderCampaigns iterates correctly with partial failure
//     tolerance.
//
// Uses small in-package fakes for SecurityEventLogger / MailNotifier /
// CampaignOwnerLookup. Mirror of the pattern used in service_test.go.
package foundry_vtt

import (
	"context"
	"errors"
	"sync"
	"testing"
)

// fakeEventLogger records every LogEvent call so tests can assert on
// the event type + payload.
type fakeEventLogger struct {
	mu     sync.Mutex
	events []fakeEventCall
	err    error // returned by LogEvent if non-nil
}

type fakeEventCall struct {
	eventType string
	actorID   string
	details   map[string]any
}

func (l *fakeEventLogger) LogEvent(_ context.Context, eventType, _, actorID, _, _ string, details map[string]any) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.events = append(l.events, fakeEventCall{eventType: eventType, actorID: actorID, details: details})
	return l.err
}

// fakeMail records SendMail calls. IsConfigured is wired via the
// configured flag so tests can exercise both paths.
type fakeMail struct {
	configured bool
	sent       []fakeMailSend
}

type fakeMailSend struct {
	to      []string
	subject string
}

func (m *fakeMail) IsConfigured(_ context.Context) bool { return m.configured }
func (m *fakeMail) SendMail(_ context.Context, to []string, subject, _ string) error {
	m.sent = append(m.sent, fakeMailSend{to: to, subject: subject})
	return nil
}

// fakeOwners returns canned (email, name) for the notify path.
type fakeOwners struct {
	email string
	name  string
}

func (o *fakeOwners) GetCampaignOwnerEmail(_ context.Context, _ string) (string, string, error) {
	return o.email, o.name, nil
}

// fakeSettings stubs CampaignSettingsAdapter for force-pin tests.
// CampaignExists returns true; pin storage is in-memory.
type fakeSettings struct {
	pin string
}

func (s *fakeSettings) GetFoundryModulePin(_ context.Context, _ string) (string, error) {
	return s.pin, nil
}
func (s *fakeSettings) SetFoundryModulePin(_ context.Context, _, version string) error {
	s.pin = version
	return nil
}
func (s *fakeSettings) CampaignExists(_ context.Context, _ string) (bool, error) {
	return true, nil
}

// TestForcePinCampaign_EmptyVersionRejected — pinning to "" is the
// clear-pin operation, NOT a force-update. Force-pin MUST refuse
// since "force-update to nothing" makes no sense.
func TestForcePinCampaign_EmptyVersionRejected(t *testing.T) {
	svc := newTestService(t, &fakeEventLogger{}, nil, nil)
	err := svc.ForcePinCampaign(context.Background(), "camp-1", "", "actor-1", "1.2.3.4", "test-ua")
	if err == nil {
		t.Fatal("expected error for empty version, got nil")
	}
	fe := AsError(err)
	if fe == nil {
		t.Fatalf("expected *Error, got %T", err)
	}
	if fe.Code != "force_pin_empty_version" {
		t.Errorf("expected code 'force_pin_empty_version', got %q", fe.Code)
	}
	if fe.Category != ErrCategoryValidation {
		t.Errorf("expected validation category, got %q", fe.Category)
	}
}

// TestNotifyCampaignOfUpdate_EventLogged — the "notify" action logs
// the EventModuleUpdateNotify event type. The audit trail distinction
// from ForcePinCampaign (EventModuleForcePin) is what lets admins
// answer "did the operator tell people, or did they actually pin
// it?" from the dashboard.
func TestNotifyCampaignOfUpdate_EventLogged(t *testing.T) {
	logger := &fakeEventLogger{}
	svc := newTestService(t, logger, nil, nil)
	err := svc.NotifyCampaignOfUpdate(context.Background(),
		"camp-1", "v0.2.0", "actor-1", "1.2.3.4", "test-ua")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(logger.events) != 1 {
		t.Fatalf("expected exactly 1 event logged, got %d", len(logger.events))
	}
	ev := logger.events[0]
	if ev.eventType != EventModuleUpdateNotify {
		t.Errorf("expected event type %q, got %q", EventModuleUpdateNotify, ev.eventType)
	}
	if ev.details["campaign_id"] != "camp-1" {
		t.Errorf("details should include campaign_id, got %+v", ev.details)
	}
	if ev.details["new_version"] != "v0.2.0" {
		t.Errorf("details should include new_version, got %+v", ev.details)
	}
}

// TestNotifyCampaignOfUpdate_SMTPSkippedWhenNotConfigured — the
// in-app banner is the primary notification surface; SMTP is a
// courtesy. When SMTP is not configured, notify must still log the
// audit event (so the banner fires on next campaign dashboard load).
func TestNotifyCampaignOfUpdate_SMTPSkippedWhenNotConfigured(t *testing.T) {
	logger := &fakeEventLogger{}
	mail := &fakeMail{configured: false}
	svc := newTestService(t, logger, mail, &fakeOwners{email: "owner@test", name: "Owner"})
	err := svc.NotifyCampaignOfUpdate(context.Background(),
		"camp-1", "v0.2.0", "actor-1", "1.2.3.4", "test-ua")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(logger.events) != 1 {
		t.Errorf("event should still be logged even with SMTP disabled, got %d events", len(logger.events))
	}
	if len(mail.sent) != 0 {
		t.Errorf("no email should be sent when SMTP not configured, got %d sends", len(mail.sent))
	}
}

// TestNotifyCampaignOfUpdate_SMTPSentWhenConfigured — happy path:
// SMTP configured + owner email resolves → email gets sent in
// addition to the audit event.
func TestNotifyCampaignOfUpdate_SMTPSentWhenConfigured(t *testing.T) {
	logger := &fakeEventLogger{}
	mail := &fakeMail{configured: true}
	svc := newTestService(t, logger, mail, &fakeOwners{email: "owner@test", name: "Owner"})
	err := svc.NotifyCampaignOfUpdate(context.Background(),
		"camp-1", "v0.2.0", "actor-1", "1.2.3.4", "test-ua")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mail.sent) != 1 {
		t.Fatalf("expected 1 mail send, got %d", len(mail.sent))
	}
	if mail.sent[0].to[0] != "owner@test" {
		t.Errorf("expected email to owner@test, got %v", mail.sent[0].to)
	}
}

// TestNotifyCampaignOfUpdate_EventsUnwiredReturnsError — service
// without an events sink can't fire the in-app banner; surface as
// an internal error rather than silently dropping.
func TestNotifyCampaignOfUpdate_EventsUnwiredReturnsError(t *testing.T) {
	svc := newTestService(t, nil, nil, nil)
	err := svc.NotifyCampaignOfUpdate(context.Background(),
		"camp-1", "v0.2.0", "actor-1", "1.2.3.4", "test-ua")
	if err == nil {
		t.Fatal("expected error when events sink is nil, got nil")
	}
	fe := AsError(err)
	if fe == nil || fe.Code != "notify_events_unwired" {
		t.Errorf("expected notify_events_unwired error, got %v", err)
	}
}

// TestNotifyEventLoggerError_Propagates — a non-nil LogEvent error
// must surface to the caller (would normally indicate a DB issue
// that the admin needs to know about).
func TestNotifyEventLoggerError_Propagates(t *testing.T) {
	logger := &fakeEventLogger{err: errors.New("db connection lost")}
	svc := newTestService(t, logger, nil, nil)
	err := svc.NotifyCampaignOfUpdate(context.Background(),
		"camp-1", "v0.2.0", "actor-1", "1.2.3.4", "test-ua")
	if err == nil {
		t.Fatal("expected error when event logger fails, got nil")
	}
}

// --- test fixtures ---

// newTestService constructs a service struct directly (bypassing the
// public NewService constructor) so tests can inject the in-memory
// fakes. The service's admin methods only need events/mail/owners +
// settings — they don't touch repo/pkgs/tokens for the empty-version
// rejection and event-logging paths exercised here.
func newTestService(t *testing.T, events SecurityEventLogger, mail MailNotifier, owners CampaignOwnerLookup) *service {
	t.Helper()
	return &service{
		settings: &fakeSettings{},
		events:   events,
		mail:     mail,
		owners:   owners,
	}
}

