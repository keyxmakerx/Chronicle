package syncapi

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

func newTestLinkCodes(now *time.Time) *LinkCodes {
	s := NewLinkCodes()
	s.now = func() time.Time { return *now }
	return s
}

func TestLinkCodes_IssueAndRedeemOnce(t *testing.T) {
	now := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	s := newTestLinkCodes(&now)
	code, expires, err := s.Issue("c1", "u1")
	if err != nil {
		t.Fatal(err)
	}
	if len(code) != linkCodeLength || strings.Trim(code, linkCodeAlphabet) != "" {
		t.Fatalf("code %q is not %d characters of the alphabet", code, linkCodeLength)
	}
	if !expires.Equal(now.Add(linkCodeTTL)) {
		t.Fatalf("expires %v", expires)
	}
	typed := strings.ToLower(FormatLinkCode(code))
	user, err := s.Redeem("c1", " "+typed+" ")
	if err != nil || user != "u1" {
		t.Fatalf("redeem: %q %v", user, err)
	}
	if _, err := s.Redeem("c1", code); apperror.SafeCode(err) != http.StatusNotFound {
		t.Fatalf("second redeem: %v, want 404", err)
	}
}

func TestLinkCodes_Refusals(t *testing.T) {
	cases := []struct {
		name   string
		redeem func(s *LinkCodes, code string, now *time.Time) error
	}{
		{"another campaign", func(s *LinkCodes, code string, _ *time.Time) error {
			_, err := s.Redeem("c2", code)
			return err
		}},
		{"expired", func(s *LinkCodes, code string, now *time.Time) error {
			*now = now.Add(linkCodeTTL)
			_, err := s.Redeem("c1", code)
			return err
		}},
		{"replaced by a newer code", func(s *LinkCodes, code string, _ *time.Time) error {
			if _, _, err := s.Issue("c1", "u1"); err != nil {
				return err
			}
			_, err := s.Redeem("c1", code)
			return err
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			now := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
			s := newTestLinkCodes(&now)
			code, _, err := s.Issue("c1", "u1")
			if err != nil {
				t.Fatal(err)
			}
			if err := tc.redeem(s, code, &now); apperror.SafeCode(err) != http.StatusNotFound {
				t.Fatalf("got %v, want 404", err)
			}
		})
	}
}

func TestLinkCodes_OtherMembersCodesStay(t *testing.T) {
	now := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	s := newTestLinkCodes(&now)
	first, _, _ := s.Issue("c1", "u1")
	if _, _, err := s.Issue("c1", "u2"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Issue("c2", "u1"); err != nil {
		t.Fatal(err)
	}
	if user, err := s.Redeem("c1", first); err != nil || user != "u1" {
		t.Fatalf("u1's code: %q %v", user, err)
	}
}

func TestLinkCodes_TooManyWrongGuessesStopRedeems(t *testing.T) {
	now := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	s := newTestLinkCodes(&now)
	code, _, _ := s.Issue("c1", "u1")
	for i := 0; i < linkGuessMax; i++ {
		if _, err := s.Redeem("c1", "WRONG1"); apperror.SafeCode(err) != http.StatusNotFound {
			t.Fatalf("guess %d: %v", i, err)
		}
	}
	if _, err := s.Redeem("c1", code); apperror.SafeCode(err) != http.StatusTooManyRequests {
		t.Fatalf("right code while locked: %v, want 429", err)
	}
	if _, err := s.Redeem("c2", "WRONG1"); apperror.SafeCode(err) != http.StatusNotFound {
		t.Fatalf("another campaign is not locked: %v", err)
	}
	now = now.Add(linkGuessWindow)
	code, _, _ = s.Issue("c1", "u1")
	if user, err := s.Redeem("c1", code); err != nil || user != "u1" {
		t.Fatalf("after the window: %q %v", user, err)
	}
}

func TestLinkCodes_IssueNeedsAMember(t *testing.T) {
	s := NewLinkCodes()
	if _, _, err := s.Issue("c1", ""); apperror.SafeCode(err) != http.StatusBadRequest {
		t.Fatalf("got %v", err)
	}
}
