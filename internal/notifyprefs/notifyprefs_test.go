package notifyprefs

import (
	"encoding/json"
	"testing"
)

func TestAllows(t *testing.T) {
	tests := []struct {
		name     string
		raw      string
		category string
		ch       Channel
		want     bool
	}{
		{"default invite email on", "", GameNightInvites, Email, true},
		{"default availability email off", "", AvailabilityAsks, Email, false},
		{"default bell on", "", ItemsGiven, Bell, true},
		{"switched off", `{"choices":{"gameNightInvites":{"email":false}}}`, GameNightInvites, Email, false},
		{"pause stops email", `{"pauseEmail":true}`, GameNightInvites, Email, false},
		{"pause leaves bell", `{"pauseEmail":true}`, GameNightInvites, Bell, true},
		{"unknown category always sent", "", "somethingNew", Email, true},
		{"channel the category lacks always sent", "", ItemsGiven, Email, true},
		{"malformed row falls back to defaults", `{not json`, GameNightInvites, Email, true},
		{"unknown stored key ignored", `{"choices":{"bogus":{"email":false}}}`, GameNightInvites, Email, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Parse([]byte(tt.raw)).Allows(tt.category, tt.ch); got != tt.want {
				t.Fatalf("Allows(%s, %s) = %v, want %v", tt.category, tt.ch, got, tt.want)
			}
		})
	}
}

func TestUpdateIsPartial(t *testing.T) {
	cur := Default()
	cur.Choices[ItemsGiven][Bell] = false
	var u Update
	if err := json.Unmarshal([]byte(`{"choices":{"gameNightInvites":{"email":false}}}`), &u); err != nil {
		t.Fatal(err)
	}
	next, err := u.ApplyTo(cur)
	if err != nil {
		t.Fatal(err)
	}
	if next.Choices[GameNightInvites][Email] {
		t.Error("sent switch did not change")
	}
	if next.Choices[ItemsGiven][Bell] {
		t.Error("a switch that was not sent changed")
	}
	if !cur.Choices[GameNightInvites][Email] {
		t.Error("ApplyTo modified its input")
	}
}

func TestUpdateRefusesUnknown(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"unknown category", `{"choices":{"bogus":{"bell":true}}}`},
		{"channel not sent for category", `{"choices":{"itemsGiven":{"email":true}}}`},
		{"unknown channel", `{"choices":{"itemsGiven":{"sms":true}}}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var u Update
			if err := json.Unmarshal([]byte(tt.body), &u); err != nil {
				t.Fatal(err)
			}
			if _, err := u.ApplyTo(Default()); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestEveryBellTypeHasABellRow(t *testing.T) {
	for ntype, key := range bellTypes {
		c, ok := find(key)
		if !ok || !c.Has(Bell) {
			t.Errorf("bell type %q maps to %q, which has no bell switch", ntype, key)
		}
	}
}
