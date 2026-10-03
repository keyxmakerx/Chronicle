package backup

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"strconv"
	"strings"
	"time"
)

// downloadTokenTTL is deliberately short: the token only bridges the gap
// between the re-confirmed POST and the browser following its redirect.
const downloadTokenTTL = 60 * time.Second

// downloadTokenPurpose domain-separates these MACs from any other use of the
// shared signing secret, so a signature minted elsewhere can never validate here.
const downloadTokenPurpose = "chronicle-backup-download-v1"

// downloadSigner issues and verifies short-lived download tokens bound to one
// user and one file name. It is stateless: everything needed to verify a
// token is in the token plus the key.
type downloadSigner struct {
	key []byte
	now func() time.Time
}

// newRandomDownloadSigner builds a signer with a per-process random key. That
// is an acceptable fallback because tokens live only seconds; it also means a
// handler that was never given the app's secret still fails closed rather
// than using a guessable key.
func newRandomDownloadSigner() *downloadSigner {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		// crypto/rand failing is unrecoverable; an all-zero key must never sign.
		panic("backup: crypto/rand unavailable: " + err.Error())
	}
	return &downloadSigner{key: key, now: time.Now}
}

// mac signs the purpose, expiry, user and name. Length-prefixing the variable
// fields keeps distinct (user, name) pairs from colliding on concatenation.
func (s *downloadSigner) mac(expiry int64, userID, name string) []byte {
	m := hmac.New(sha256.New, s.key)
	m.Write([]byte(downloadTokenPurpose))
	m.Write([]byte{0})
	m.Write([]byte(strconv.FormatInt(expiry, 10)))
	m.Write([]byte{0})
	m.Write([]byte(strconv.Itoa(len(userID)) + ":" + userID))
	m.Write([]byte{0})
	m.Write([]byte(strconv.Itoa(len(name)) + ":" + name))
	return m.Sum(nil)
}

// Issue returns a token of the form "<expiryUnix>.<base64url mac>".
func (s *downloadSigner) Issue(userID, name string) string {
	exp := s.now().Add(downloadTokenTTL).Unix()
	return strconv.FormatInt(exp, 10) + "." + base64.RawURLEncoding.EncodeToString(s.mac(exp, userID, name))
}

// Verify reports whether token is unexpired and was issued for exactly this
// user and file name. The expiry is checked server-side and the MAC is
// compared in constant time.
func (s *downloadSigner) Verify(token, userID, name string) bool {
	if userID == "" || token == "" {
		return false
	}
	expStr, sigStr, ok := strings.Cut(token, ".")
	if !ok {
		return false
	}
	exp, err := strconv.ParseInt(expStr, 10, 64)
	if err != nil {
		return false
	}
	sig, err := base64.RawURLEncoding.DecodeString(sigStr)
	if err != nil {
		return false
	}
	if !hmac.Equal(sig, s.mac(exp, userID, name)) {
		return false
	}
	return s.now().Unix() <= exp
}
