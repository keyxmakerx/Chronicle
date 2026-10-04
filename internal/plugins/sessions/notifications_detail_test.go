package sessions

import "testing"

// The bell's second line comes from the payload; older rows without one, and
// rows whose payload is not JSON, show none.
func TestNotificationDetail(t *testing.T) {
	str := func(s string) *string { return &s }
	tests := []struct {
		name    string
		payload *string
		want    string
	}{
		{"with detail", marshalPayloadDetail("Your GM gave you Rope", "item_given", "On Bren"), "On Bren"},
		{"without", marshalPayload("New scheduling proposal", NotifProposalCreated), ""},
		{"no payload", nil, ""},
		{"not json", str("{"), ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			n := Notification{Type: "item_given", Payload: tc.payload}
			if got := notificationDetail(n); got != tc.want {
				t.Fatalf("detail %q, want %q", got, tc.want)
			}
		})
	}
	if got := notificationMessage(Notification{Payload: marshalPayloadDetail("Your GM gave you Rope", "item_given", "On Bren")}); got != "Your GM gave you Rope" {
		t.Fatalf("message %q", got)
	}
}
