package auth

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"image/png"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/pquerna/otp/totp"
	"github.com/redis/go-redis/v9"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

const (
	recoveryCodeCount     = 8
	twoFactorSetupTTL     = 15 * time.Minute
	twoFactorChallengeTTL = 10 * time.Minute
	twoFactorMaxFailures  = 5
	twoFactorLockWindow   = 15 * time.Minute
	trustedDeviceTTL      = 30 * 24 * time.Hour
	// A code stays valid for one step either side of now, so a used one is
	// remembered a little longer than that window to stop a replay.
	totpReplayWindow = 2 * time.Minute
	twoFactorIssuer  = "Chronicle"

	totpSetupKeyPrefix      = "totp_setup:"
	loginChallengeKeyPrefix = "login_2fa:"
	totpFailuresKeyPrefix   = "totp_failures:"
	totpUsedKeyPrefix       = "totp_used:"
	trustedDeviceKeyPrefix  = "trusted_device:"
	userTrustedKeyPrefix    = "user_trusted_devices:"
)

// TwoFactorRequired is what Login returns when the password was right but
// the account also wants a code. Challenge names the half-finished sign-in
// for CompleteTwoFactorLogin; it is not a session.
type TwoFactorRequired struct {
	Challenge string
}

func (e *TwoFactorRequired) Error() string { return "two-factor code required" }

// TwoFactorSetup is what the account page shows while turning two-factor on.
type TwoFactorSetup struct {
	// Secret is the key spaced in groups of four for typing by hand.
	Secret string `json:"secret"`
	// QR is a PNG data URI of the otpauth address.
	QR string `json:"qr"`
}

// TwoFactorStatus is the account page's view of a person's two-factor.
type TwoFactorStatus struct {
	Enabled   bool
	CodesLeft int
}

// TwoFactorLoginInput is the second sign-in step.
type TwoFactorLoginInput struct {
	Challenge string
	// Code is a 6-digit authenticator code or a recovery code; the shape
	// tells them apart.
	Code      string
	Remember  bool
	IP        string
	UserAgent string
}

// TwoFactorLoginResult carries the new session and, when the person asked
// not to be asked again, the trusted-device token for its cookie.
type TwoFactorLoginResult struct {
	SessionToken  string
	TrustedDevice string
	User          *User
}

// ConfigureTwoFactor gives the service the key that encrypts stored
// authenticator secrets. It is derived from the site secret, so a database
// copy alone can't produce codes. Without it two-factor can't be turned on.
// HKDF gives this purpose its own key, separate from anything else the
// site secret signs.
func ConfigureTwoFactor(svc AuthService, siteSecret string) {
	if s, ok := svc.(*authService); ok && siteSecret != "" {
		k, err := hkdf.Key(sha256.New, []byte(siteSecret), nil, "chronicle-totp", 32)
		if err != nil {
			return
		}
		s.totpKey = k
	}
}

func (s *authService) sealSecret(secret string) (string, error) { return sealWith(s.totpKey, secret) }

func (s *authService) openSecret(sealed string) (string, error) { return openWith(s.totpKey, sealed) }

// sealWith encrypts with AES-256-GCM under key and returns base64 of
// nonce+ciphertext.
func sealWith(key []byte, secret string) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(gcm.Seal(nonce, nonce, []byte(secret), nil)), nil
}

// openWith reverses sealWith.
func openWith(key []byte, sealed string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(sealed)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(raw) < gcm.NonceSize() {
		return "", errors.New("sealed secret too short")
	}
	out, err := gcm.Open(nil, raw[:gcm.NonceSize()], raw[gcm.NonceSize():], nil)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func (s *authService) twoFactorReady() error {
	if s.totpKey == nil || s.redis == nil {
		return apperror.NewInternal(errors.New("two-factor is not configured"))
	}
	return nil
}

// TwoFactorStatus reports whether two-factor is on and how many recovery
// codes are left.
func (s *authService) TwoFactorStatus(ctx context.Context, userID string) (TwoFactorStatus, error) {
	user, err := s.repo.FindByID(ctx, userID)
	if err != nil {
		return TwoFactorStatus{}, err
	}
	if !user.TOTPEnabled {
		return TwoFactorStatus{}, nil
	}
	n, err := s.repo.CountRecoveryCodes(ctx, userID)
	if err != nil {
		return TwoFactorStatus{Enabled: true}, err
	}
	return TwoFactorStatus{Enabled: true, CodesLeft: n}, nil
}

// BeginTwoFactor checks the password and makes a new authenticator key. The
// key waits in Redis until EnableTwoFactor proves the app reads it, so a
// half-finished setup never locks anyone out.
func (s *authService) BeginTwoFactor(ctx context.Context, userID, password string) (*TwoFactorSetup, error) {
	if err := s.twoFactorReady(); err != nil {
		return nil, err
	}
	user, err := s.repo.FindByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if user.TOTPEnabled {
		return nil, apperror.NewConflict("two-factor is already on")
	}
	if !verifyPassword(password, user.PasswordHash) {
		return nil, apperror.NewBadRequest("that password isn't right")
	}
	key, err := totp.Generate(totp.GenerateOpts{Issuer: twoFactorIssuer, AccountName: user.Email})
	if err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("generating key: %w", err))
	}
	img, err := key.Image(220, 220)
	if err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("drawing QR code: %w", err))
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("encoding QR code: %w", err))
	}
	if err := s.redis.Set(ctx, totpSetupKeyPrefix+userID, key.Secret(), twoFactorSetupTTL).Err(); err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("storing setup: %w", err))
	}
	return &TwoFactorSetup{
		Secret: groupsOfFour(key.Secret()),
		QR:     "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes()),
	}, nil
}

// EnableTwoFactor turns two-factor on once the code from the new key checks
// out, and returns the recovery codes. They are shown this once; only their
// hashes are kept.
func (s *authService) EnableTwoFactor(ctx context.Context, userID, code string) ([]string, error) {
	if err := s.twoFactorReady(); err != nil {
		return nil, err
	}
	secret, err := s.redis.Get(ctx, totpSetupKeyPrefix+userID).Result()
	if errors.Is(err, redis.Nil) {
		return nil, apperror.NewBadRequest("that setup expired; start again")
	}
	if err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("reading setup: %w", err))
	}
	if !totp.Validate(digitsOnly(code), secret) {
		return nil, apperror.NewBadRequest("that code isn't right; check the time on your phone is correct")
	}
	sealed, err := s.sealSecret(secret)
	if err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("encrypting key: %w", err))
	}
	codes, hashes, err := newRecoveryCodes()
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	if err := s.repo.ReplaceRecoveryCodes(ctx, userID, hashes); err != nil {
		return nil, apperror.NewInternal(err)
	}
	if err := s.repo.SetTOTP(ctx, userID, &sealed, true); err != nil {
		return nil, apperror.NewInternal(err)
	}
	s.redis.Del(ctx, totpSetupKeyPrefix+userID)
	s.redis.Set(ctx, totpUsedKeyPrefix+userID+":"+digitsOnly(code), "1", totpReplayWindow)
	slog.Info("two-factor turned on", slog.String("user_id", userID))
	return codes, nil
}

// DisableTwoFactor turns two-factor off after checking the password and a
// code, so a stolen session plus a known password can't strip it; it
// forgets the recovery codes and trusted devices with it.
func (s *authService) DisableTwoFactor(ctx context.Context, userID, password, code string) error {
	user, err := s.repo.FindByID(ctx, userID)
	if err != nil {
		return err
	}
	if !verifyPassword(password, user.PasswordHash) {
		return apperror.NewBadRequest("that password isn't right")
	}
	if err := s.verifySecondFactorIfOn(ctx, user, code); err != nil {
		return err
	}
	return s.clearTwoFactor(ctx, userID)
}

// AdminDisableTwoFactor switches two-factor off for someone who lost both
// their phone and their codes, and emails them so a switch-off they didn't
// ask for doesn't go unnoticed. Nobody, admins included, can read the key.
func (s *authService) AdminDisableTwoFactor(ctx context.Context, userID string) error {
	user, err := s.repo.FindByID(ctx, userID)
	if err != nil {
		return err
	}
	if !user.TOTPEnabled {
		return apperror.NewBadRequest("two-factor is already off for this person")
	}
	if err := s.clearTwoFactor(ctx, userID); err != nil {
		return err
	}
	if s.mail != nil && s.mail.IsConfigured(ctx) {
		body := "A site admin switched off two-factor sign-in for your Chronicle account, " +
			"usually because you asked after losing your phone.\n\n" +
			"You can sign in with your password alone until you turn it back on under Account Settings.\n\n" +
			"If you didn't ask for this, change your password and contact your site admin."
		if err := s.mail.SendMail(ctx, []string{user.Email}, "Two-factor sign-in was switched off — Chronicle", body); err != nil {
			slog.Warn("sending two-factor switch-off email", slog.String("user_id", userID), slog.Any("error", err))
		}
	}
	return nil
}

func (s *authService) clearTwoFactor(ctx context.Context, userID string) error {
	if err := s.repo.SetTOTP(ctx, userID, nil, false); err != nil {
		return apperror.NewInternal(err)
	}
	if err := s.repo.ReplaceRecoveryCodes(ctx, userID, nil); err != nil {
		slog.Warn("removing recovery codes", slog.String("user_id", userID), slog.Any("error", err))
	}
	s.forgetTrustedDevices(ctx, userID)
	slog.Info("two-factor turned off", slog.String("user_id", userID))
	return nil
}

// RegenerateRecoveryCodes replaces every recovery code with a new set after
// checking the password and a code. The old ones stop working at once.
func (s *authService) RegenerateRecoveryCodes(ctx context.Context, userID, password, code string) ([]string, error) {
	user, err := s.repo.FindByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if !user.TOTPEnabled {
		return nil, apperror.NewBadRequest("two-factor is off")
	}
	if !verifyPassword(password, user.PasswordHash) {
		return nil, apperror.NewBadRequest("that password isn't right")
	}
	if err := s.verifySecondFactorIfOn(ctx, user, code); err != nil {
		return nil, err
	}
	codes, hashes, err := newRecoveryCodes()
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	if err := s.repo.ReplaceRecoveryCodes(ctx, userID, hashes); err != nil {
		return nil, apperror.NewInternal(err)
	}
	return codes, nil
}

// CompleteTwoFactorLogin finishes a sign-in Login paused for a code. Wrong
// codes count against the account, not the challenge, so starting over
// with the password doesn't reset the count.
func (s *authService) CompleteTwoFactorLogin(ctx context.Context, in TwoFactorLoginInput) (*TwoFactorLoginResult, error) {
	if s.redis == nil {
		return nil, apperror.NewInternal(errors.New("two-factor needs Redis"))
	}
	chKey := loginChallengeKeyPrefix + hashToken(in.Challenge)
	stored, err := s.redis.Get(ctx, chKey).Result()
	if errors.Is(err, redis.Nil) || in.Challenge == "" {
		return nil, apperror.NewUnauthorized("that sign-in took too long; start again")
	}
	if err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("reading sign-in: %w", err))
	}
	userID, passwordMark, _ := strings.Cut(stored, "|")
	user, err := s.repo.FindByID(ctx, userID)
	// A password changed since the first step ends the half-finished sign-in.
	if err != nil || passwordMark != challengePasswordMark(user) {
		return nil, apperror.NewUnauthorized("that sign-in took too long; start again")
	}
	if user.IsDisabled {
		return nil, apperror.NewForbidden("your account has been disabled")
	}
	// An admin may have switched two-factor off since the password step;
	// the password was right, so the sign-in goes ahead.
	if user.TOTPEnabled {
		if err := s.checkSecondFactor(ctx, user, in.Code); err != nil {
			return nil, err
		}
	}
	s.redis.Del(ctx, chKey)

	token, err := s.createSession(ctx, user, in.IP, in.UserAgent)
	if err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("creating session: %w", err))
	}
	if err := s.repo.UpdateLastLogin(ctx, user.ID); err != nil {
		slog.Warn("failed to update last login", slog.String("user_id", user.ID), slog.Any("error", err))
	}
	out := &TwoFactorLoginResult{SessionToken: token, User: user}
	if in.Remember {
		if dev, err := s.trustDevice(ctx, user.ID); err == nil {
			out.TrustedDevice = dev
		} else {
			slog.Warn("remembering device", slog.String("user_id", user.ID), slog.Any("error", err))
		}
	}
	slog.Info("user logged in with two-factor", slog.String("user_id", user.ID))
	return out, nil
}

// verifySecondFactorIfOn checks a code for an action that asks again, such
// as deleting the account, when the person has two-factor on. It shares the
// sign-in lock.
func (s *authService) verifySecondFactorIfOn(ctx context.Context, user *User, code string) error {
	if !user.TOTPEnabled {
		return nil
	}
	if strings.TrimSpace(code) == "" {
		return apperror.NewBadRequest("type a code from your authenticator app or a recovery code")
	}
	return s.checkSecondFactor(ctx, user, code)
}

// checkSecondFactor accepts a current authenticator code or an unused
// recovery code. Five wrong codes pause the account's code checks for 15
// minutes; a right one clears the count.
//
// Every attempt takes its number before the code is checked, in one
// round trip with the expiry, so parallel guesses can't slip past the
// limit and the counter can never be left without a TTL.
func (s *authService) checkSecondFactor(ctx context.Context, user *User, code string) error {
	if s.redis == nil || s.totpKey == nil {
		return apperror.NewInternal(errors.New("two-factor is not configured"))
	}
	failKey := totpFailuresKeyPrefix + user.ID
	var attempt *redis.IntCmd
	if _, err := s.redis.TxPipelined(ctx, func(p redis.Pipeliner) error {
		attempt = p.Incr(ctx, failKey)
		p.Expire(ctx, failKey, twoFactorLockWindow)
		return nil
	}); err != nil {
		return apperror.NewInternal(fmt.Errorf("counting code attempts: %w", err))
	}
	if attempt.Val() > twoFactorMaxFailures {
		return apperror.NewTooManyRequests("too many wrong codes; try again in 15 minutes")
	}
	ok, err := s.matchSecondFactor(ctx, user, code)
	if err != nil {
		return err
	}
	if !ok {
		if attempt.Val() >= twoFactorMaxFailures {
			return apperror.NewTooManyRequests("too many wrong codes; try again in 15 minutes")
		}
		return apperror.NewUnauthorized("that code isn't right")
	}
	s.redis.Del(ctx, failKey)
	return nil
}

func (s *authService) matchSecondFactor(ctx context.Context, user *User, code string) (bool, error) {
	if d := digitsOnly(code); len(d) == 6 && len(strings.TrimSpace(code)) <= 7 {
		if user.TOTPSecret == nil {
			return false, nil
		}
		secret, err := s.openSecret(*user.TOTPSecret)
		if err != nil {
			return false, apperror.NewInternal(fmt.Errorf("opening two-factor key: %w", err))
		}
		if !totp.Validate(d, secret) {
			return false, nil
		}
		// Setting only if absent makes a code good for one use even inside
		// its time window.
		err = s.redis.SetArgs(ctx, totpUsedKeyPrefix+user.ID+":"+d, "1",
			redis.SetArgs{Mode: "NX", TTL: totpReplayWindow}).Err()
		if errors.Is(err, redis.Nil) {
			return false, nil
		}
		if err != nil {
			return false, apperror.NewInternal(err)
		}
		return true, nil
	}
	norm := normalizeRecoveryCode(code)
	if norm == "" {
		return false, nil
	}
	used, err := s.repo.UseRecoveryCode(ctx, user.ID, hashToken(norm))
	if err != nil {
		return false, apperror.NewInternal(err)
	}
	if used {
		slog.Info("recovery code used", slog.String("user_id", user.ID))
	}
	return used, nil
}

// startTwoFactorChallenge records a sign-in whose password was right and
// returns the token the code step carries. Only its hash is stored.
func (s *authService) startTwoFactorChallenge(ctx context.Context, user *User) (string, error) {
	token, err := generateSessionToken()
	if err != nil {
		return "", err
	}
	val := user.ID + "|" + challengePasswordMark(user)
	if err := s.redis.Set(ctx, loginChallengeKeyPrefix+hashToken(token), val, twoFactorChallengeTTL).Err(); err != nil {
		return "", err
	}
	return token, nil
}

// challengePasswordMark is the tail of the stored password hash, so a
// challenge stops working once the password changes. The stored value is
// already a salted Argon2id digest (or a random placeholder), so its tail
// changes with every new password without hashing it again.
func challengePasswordMark(user *User) string {
	h := user.PasswordHash
	if len(h) > 16 {
		h = h[len(h)-16:]
	}
	return h
}

func (s *authService) trustDevice(ctx context.Context, userID string) (string, error) {
	token, err := generateSessionToken()
	if err != nil {
		return "", err
	}
	h := hashToken(token)
	if err := s.redis.Set(ctx, trustedDeviceKeyPrefix+h, userID, trustedDeviceTTL).Err(); err != nil {
		return "", err
	}
	setKey := userTrustedKeyPrefix + userID
	s.redis.SAdd(ctx, setKey, h)
	s.redis.Expire(ctx, setKey, trustedDeviceTTL)
	return token, nil
}

// isTrustedDevice reports whether the device cookie was issued to this
// person and hasn't been revoked.
func (s *authService) isTrustedDevice(ctx context.Context, userID, token string) bool {
	if token == "" || s.redis == nil {
		return false
	}
	owner, err := s.redis.Get(ctx, trustedDeviceKeyPrefix+hashToken(token)).Result()
	return err == nil && owner == userID
}

// forgetTrustedDevices makes every device ask for a code again. It runs
// whenever all sessions end (password change or reset, admin sign-out) and
// when two-factor is turned off.
func (s *authService) forgetTrustedDevices(ctx context.Context, userID string) {
	if s.redis == nil {
		return
	}
	setKey := userTrustedKeyPrefix + userID
	hashes, err := s.redis.SMembers(ctx, setKey).Result()
	if err != nil {
		slog.Warn("listing trusted devices", slog.String("user_id", userID), slog.Any("error", err))
		return
	}
	for _, h := range hashes {
		s.redis.Del(ctx, trustedDeviceKeyPrefix+h)
	}
	s.redis.Del(ctx, setKey)
}

// recoveryAlphabet leaves out look-alike characters (0/o, 1/l/i) so a code
// copied onto paper reads back unambiguously.
const recoveryAlphabet = "abcdefghjkmnpqrstuvwxyz23456789"

// recoveryCodeLen is twelve characters (about 59 bits), so the stored
// hashes can't practically be reversed from a database copy.
const recoveryCodeLen = 12

// newRecoveryCodes returns the codes to show (xxxx-xxxx-xxxx) and the hashes
// to store, which are taken over the code without its dashes.
func newRecoveryCodes() ([]string, []string, error) {
	codes := make([]string, 0, recoveryCodeCount)
	hashes := make([]string, 0, recoveryCodeCount)
	// Bytes at or above the largest multiple of the alphabet size are
	// skipped, so every character is equally likely.
	limit := byte(256 - 256%len(recoveryAlphabet))
	one := make([]byte, 1)
	for len(codes) < recoveryCodeCount {
		var b strings.Builder
		for n := 0; n < recoveryCodeLen; {
			if _, err := rand.Read(one); err != nil {
				return nil, nil, fmt.Errorf("generating recovery codes: %w", err)
			}
			if one[0] >= limit {
				continue
			}
			if n > 0 && n%4 == 0 {
				b.WriteByte('-')
			}
			b.WriteByte(recoveryAlphabet[int(one[0])%len(recoveryAlphabet)])
			n++
		}
		code := b.String()
		codes = append(codes, code)
		hashes = append(hashes, hashToken(normalizeRecoveryCode(code)))
	}
	return codes, hashes, nil
}

// normalizeRecoveryCode lowercases and drops spaces and dashes, so a code
// typed as "K7QE 2M9X" matches "k7qe-2m9x".
func normalizeRecoveryCode(code string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(code) {
		if r == '-' || r == ' ' {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func digitsOnly(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func groupsOfFour(s string) string {
	var b strings.Builder
	for i, r := range s {
		if i > 0 && i%4 == 0 {
			b.WriteByte(' ')
		}
		b.WriteRune(r)
	}
	return b.String()
}
