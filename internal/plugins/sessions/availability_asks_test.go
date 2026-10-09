package sessions

import (
	"context"
	"testing"
	"time"
)

func asksRoster() []overlayMemberInput {
	return []overlayMemberInput{
		{UserID: "dm", Name: "Dana", IsOwner: true},
		{UserID: "u1", Name: "Ash"},
		{UserID: "u2", Name: "Bo"},
	}
}

func TestPingMemberAvailability(t *testing.T) {
	tests := []struct {
		name    string
		target  string
		wantErr bool
		wantTo  []string
	}{
		{"one member", "u1", false, []string{"u1"}},
		{"not on the roster", "stranger", true, nil},
		{"not yourself", "dm", true, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var to []string
			var msg string
			repo := &mockSessionRepo{createNotificationFn: func(_ context.Context, n *Notification) error {
				to = append(to, n.UserID)
				msg = notificationMessage(*n)
				return nil
			}}
			svc := NewSessionService(repo, nil, nil)
			res, err := svc.PingMemberAvailability(context.Background(), "c1", "dm", "Dana", tt.target, "/l", asksRoster())
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				if len(to) != 0 {
					t.Errorf("notified %v on an error", to)
				}
				return
			}
			if len(to) != 1 || to[0] != tt.wantTo[0] || res.Notified[0] != "Ash" {
				t.Errorf("notified %v (%v), want %v", to, res.Notified, tt.wantTo)
			}
			if msg != "Dana asked you to check your times" {
				t.Errorf("message = %q", msg)
			}
		})
	}
}

func TestAskAllToConfirm_SkipsTheAsker(t *testing.T) {
	var to []string
	var kind string
	repo := &mockSessionRepo{createNotificationFn: func(_ context.Context, n *Notification) error {
		to = append(to, n.UserID)
		kind = n.Type
		return nil
	}}
	svc := NewSessionService(repo, nil, nil)
	res, asked, err := svc.AskAllToConfirm(context.Background(), "c1", "dm", "", "/l", asksRoster())
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(to) != 2 || to[0] != "u1" || to[1] != "u2" || len(asked) != 2 || len(res.Notified) != 2 {
		t.Errorf("notified %v, asked %v", to, asked)
	}
	if kind != NotifAvailabilityConfirm {
		t.Errorf("type = %q, want %q", kind, NotifAvailabilityConfirm)
	}
}

func TestConfirmMyAvailability(t *testing.T) {
	tests := []struct {
		name     string
		answered bool
		wantErr  bool
	}{
		{"moves the stamp", true, false},
		{"nothing saved to confirm", false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stamped time.Time
			repo := &mockSessionRepo{touchAvailabilityAnsweredFn: func(_ context.Context, _, _ string, at time.Time) (bool, error) {
				stamped = at
				return tt.answered, nil
			}}
			err := NewSessionService(repo, nil, nil).ConfirmMyAvailability(context.Background(), "c1", "u1")
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if time.Since(stamped) > time.Minute {
				t.Errorf("stamped %v, want now", stamped)
			}
		})
	}
}

func TestMarkMeAway(t *testing.T) {
	today := time.Now().UTC()
	day := func(n int) string { return today.AddDate(0, 0, n).Format("2006-01-02") }
	tests := []struct {
		name     string
		req      AwayRequest
		wantErr  bool
		wantDays int
	}{
		{"a week", AwayRequest{From: day(3), To: day(9), TZ: "America/Chicago"}, false, 7},
		{"one day", AwayRequest{From: day(3), To: day(3), TZ: "UTC"}, false, 1},
		{"backwards", AwayRequest{From: day(9), To: day(3), TZ: "UTC"}, true, 0},
		{"too long", AwayRequest{From: day(1), To: day(1 + maxAwayDays), TZ: "UTC"}, true, 0},
		{"bad zone", AwayRequest{From: day(1), To: day(2), TZ: "Mars/Base"}, true, 0},
		{"bad date", AwayRequest{From: "soon", To: day(2), TZ: "UTC"}, true, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var written []string
			repo := &mockSessionRepo{replaceDayExceptionsFn: func(_ context.Context, _, _, onDate string, excs []AvailabilityException) error {
				if len(excs) != 1 || !isWholeDayOff(excs[0]) {
					t.Errorf("%s written as %+v, want one whole day off", onDate, excs)
				}
				written = append(written, onDate)
				return nil
			}}
			err := NewSessionService(repo, nil, nil).MarkMeAway(context.Background(), "c1", "u1", tt.req)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if len(written) != tt.wantDays {
				t.Errorf("wrote %d days, want %d", len(written), tt.wantDays)
			}
		})
	}
}

func TestClearMeAway_KeepsHandShapedDays(t *testing.T) {
	today := time.Now().UTC()
	day := func(n int) string { return today.AddDate(0, 0, n).Format("2006-01-02") }
	var deleted []string
	repo := &mockSessionRepo{
		listUserExceptionsFn: func(_ context.Context, _, _ string) ([]AvailabilityException, error) {
			return []AvailabilityException{
				{ID: "off", OnDate: day(2), StartMinute: 0, EndMinute: 1440, State: AvailUnavailable},
				{ID: "shaped", OnDate: day(3), StartMinute: 1080, EndMinute: 1320, State: AvailAvailable},
				{ID: "outside", OnDate: day(20), StartMinute: 0, EndMinute: 1440, State: AvailUnavailable},
			}, nil
		},
		deleteExceptionFn: func(_ context.Context, _, _, id string) error { deleted = append(deleted, id); return nil },
	}
	err := NewSessionService(repo, nil, nil).ClearMeAway(context.Background(), "c1", "u1", AwayRequest{From: day(1), To: day(5)})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(deleted) != 1 || deleted[0] != "off" {
		t.Errorf("deleted %v, want only the whole day off inside the range", deleted)
	}
}
