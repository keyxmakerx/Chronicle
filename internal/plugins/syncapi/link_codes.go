// link_codes.go — short codes a player uses to link their Chronicle account
// to their Foundry user without the GM.
//
// The player asks Chronicle for a code while signed in (only they see it),
// then types it in Foundry; the GM's Foundry client redeems it with the
// campaign's sync key and links the Foundry user who typed it to that
// member. A code names one member of one campaign, lasts ten minutes and
// works once. Codes live in memory: losing them on a restart costs a player
// one more press, and a link is Foundry's to keep, not Chronicle's.

package syncapi

import (
	"crypto/rand"
	"math/big"
	"strings"
	"sync"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

const (
	// linkCodeTTL is how long a code works.
	linkCodeTTL = 10 * time.Minute
	// linkCodeLength is the code's characters, shown as two groups of three.
	linkCodeLength = 6
	// linkCodeAlphabet leaves out 0/O, 1/I/L so a code read off one screen
	// is typed right on another.
	linkCodeAlphabet = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"
	// A campaign's wrong guesses within linkGuessWindow beyond linkGuessMax
	// stop all redeems for it until the window passes, so a stolen sync key
	// cannot walk the code space.
	linkGuessMax    = 10
	linkGuessWindow = 10 * time.Minute
)

type linkCode struct {
	campaignID string
	userID     string
	expires    time.Time
}

// LinkCodes issues and redeems link codes. Safe for concurrent use.
type LinkCodes struct {
	mu      sync.Mutex
	byCode  map[string]linkCode
	guesses map[string][]time.Time
	now     func() time.Time
}

// NewLinkCodes creates an empty store.
func NewLinkCodes() *LinkCodes {
	return &LinkCodes{byCode: map[string]linkCode{}, guesses: map[string][]time.Time{}, now: time.Now}
}

// Issue makes a new code for this member of this campaign. Any earlier code
// they had for the campaign stops working, so only the code on screen does.
func (s *LinkCodes) Issue(campaignID, userID string) (string, time.Time, error) {
	if campaignID == "" || userID == "" {
		return "", time.Time{}, apperror.NewBadRequest("a link code needs a campaign member")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	for code, lc := range s.byCode {
		if !now.Before(lc.expires) || (lc.campaignID == campaignID && lc.userID == userID) {
			delete(s.byCode, code)
		}
	}
	for {
		code, err := randomLinkCode()
		if err != nil {
			return "", time.Time{}, apperror.NewInternal(err)
		}
		if _, taken := s.byCode[code]; taken {
			continue
		}
		expires := now.Add(linkCodeTTL)
		s.byCode[code] = linkCode{campaignID: campaignID, userID: userID, expires: expires}
		return code, expires, nil
	}
}

// Redeem returns the member a code names and ends the code. A code from
// another campaign is the same as a wrong one.
func (s *LinkCodes) Redeem(campaignID, code string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	recent := s.guesses[campaignID][:0]
	for _, at := range s.guesses[campaignID] {
		if now.Sub(at) < linkGuessWindow {
			recent = append(recent, at)
		}
	}
	s.guesses[campaignID] = recent
	if len(recent) >= linkGuessMax {
		return "", apperror.NewTooManyRequests("too many wrong link codes; try again in a few minutes")
	}
	key := NormalizeLinkCode(code)
	lc, ok := s.byCode[key]
	if !ok || lc.campaignID != campaignID || !now.Before(lc.expires) {
		s.guesses[campaignID] = append(recent, now)
		return "", apperror.NewNotFound("that link code is wrong or has expired")
	}
	delete(s.byCode, key)
	return lc.userID, nil
}

// NormalizeLinkCode drops spaces and dashes and upper-cases, so "k7q-2mx"
// and "K7Q2MX" are one code.
func NormalizeLinkCode(code string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(code) {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// FormatLinkCode shows a code as two groups of three: K7Q-2MX.
func FormatLinkCode(code string) string {
	if len(code) != linkCodeLength {
		return code
	}
	return code[:3] + "-" + code[3:]
}

func randomLinkCode() (string, error) {
	max := big.NewInt(int64(len(linkCodeAlphabet)))
	out := make([]byte, linkCodeLength)
	for i := range out {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		out[i] = linkCodeAlphabet[n.Int64()]
	}
	return string(out), nil
}
