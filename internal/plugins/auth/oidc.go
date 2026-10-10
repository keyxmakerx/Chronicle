package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/redis/go-redis/v9"
	"golang.org/x/oauth2"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// Sign-in modes a provider round trip can be for.
const (
	OIDCModeLogin = "login"
	OIDCModeLink  = "link"
	OIDCModeTest  = "test"
)

const (
	oidcStateKeyPrefix = "oidc_state:"
	oidcStateTTL       = 10 * time.Minute
	oidcCallbackPath   = "/login/oidc/callback"
	oidcDiscoverWait   = 10 * time.Second
	// oidcPasswordPrefix marks the password of an account made by a provider
	// sign-in: no password can match it until the person sets one through
	// Forgot password.
	oidcPasswordPrefix = "!oidc!"
)

// OIDCSettings is the provider setting as the admin page shows it. The
// secret itself never leaves the service.
type OIDCSettings struct {
	Enabled      bool
	ButtonName   string
	Issuer       string
	ClientID     string
	HasSecret    bool
	AllowSignup  bool
	HidePassword bool
	RedirectURL  string
	LastTestAt   *time.Time
	LastTestOK   bool
	LastTestNote string
}

// OIDCSettingsInput is what the admin form saves. An empty ClientSecret
// keeps the stored one.
type OIDCSettingsInput struct {
	Enabled      bool
	ButtonName   string
	Issuer       string
	ClientID     string
	ClientSecret string
	AllowSignup  bool
	HidePassword bool
}

// LoginOptions is what the sign-in page needs to know about the provider.
type LoginOptions struct {
	// ProviderName is the button's name; empty means no provider button.
	ProviderName string
	// HidePassword folds the password form away behind an "admins" link.
	HidePassword bool
}

// SignInMethods is the account page's Sign-in methods section.
type SignInMethods struct {
	ProviderName string
	Linked       []Identity
	HasPassword  bool
}

// OIDCCallbackInput is the provider's answer as the callback received it.
type OIDCCallbackInput struct {
	State         string
	Code          string
	ProviderError string
	SessionUserID string
	IP            string
	UserAgent     string
}

// OIDCResult says what a finished round trip did.
type OIDCResult struct {
	Mode         string
	SessionToken string
	User         *User
	Redirect     string
}

// oidcPending is a round trip in flight, kept in Redis under its state.
type oidcPending struct {
	Mode     string `json:"mode"`
	UserID   string `json:"userId,omitempty"`
	Redirect string `json:"redirect,omitempty"`
	Nonce    string `json:"nonce"`
	Verifier string `json:"verifier"`
}

// oidcClaims is what Chronicle reads from a verified ID token.
type oidcClaims struct {
	Subject           string
	Email             string
	EmailVerified     bool
	Name              string
	PreferredUsername string
}

// oidcRuntime holds the provider wiring; nil until ConfigureOIDC.
type oidcRuntime struct {
	store   OIDCStore
	key     []byte
	baseURL string

	mu        sync.Mutex
	providers map[string]*oidc.Provider
}

// ConfigureOIDC wires provider sign-in. The client secret is sealed under a
// key derived from the site secret, separate from the two-factor key.
func ConfigureOIDC(svc AuthService, store OIDCStore, siteSecret, baseURL string) {
	s, ok := svc.(*authService)
	if !ok || store == nil || siteSecret == "" {
		return
	}
	k := sha256.Sum256([]byte("chronicle-oidc:" + siteSecret))
	s.oidc = &oidcRuntime{store: store, key: k[:], baseURL: strings.TrimRight(baseURL, "/"),
		providers: map[string]*oidc.Provider{}}
}

func (s *authService) oidcReady() error {
	if s.oidc == nil {
		return apperror.NewInternal(errors.New("provider sign-in is not configured"))
	}
	return nil
}

func (rt *oidcRuntime) redirectURL() string { return rt.baseURL + oidcCallbackPath }

// OIDCSettings returns the provider setting for the admin page.
func (s *authService) OIDCSettings(ctx context.Context) (OIDCSettings, error) {
	if err := s.oidcReady(); err != nil {
		return OIDCSettings{}, err
	}
	row, err := s.oidc.store.GetOIDCSettings(ctx)
	if err != nil {
		return OIDCSettings{}, apperror.NewInternal(err)
	}
	return OIDCSettings{
		Enabled: row.Enabled, ButtonName: row.ButtonName, Issuer: row.Issuer, ClientID: row.ClientID,
		HasSecret: row.SealedSecret != nil && *row.SealedSecret != "", AllowSignup: row.AllowSignup,
		HidePassword: row.HidePassword, RedirectURL: s.oidc.redirectURL(),
		LastTestAt: row.LastTestAt, LastTestOK: row.LastTestOK, LastTestNote: row.LastTestNote,
	}, nil
}

// SaveOIDCSettings validates and stores the provider setting. Switching it
// on needs every field, so a half-filled form can't put a dead button on the
// sign-in page.
func (s *authService) SaveOIDCSettings(ctx context.Context, in OIDCSettingsInput) error {
	if err := s.oidcReady(); err != nil {
		return err
	}
	cur, err := s.oidc.store.GetOIDCSettings(ctx)
	if err != nil {
		return apperror.NewInternal(err)
	}
	row := oidcSettingsRow{
		Enabled:      in.Enabled,
		ButtonName:   strings.TrimSpace(in.ButtonName),
		Issuer:       strings.TrimSpace(in.Issuer),
		ClientID:     strings.TrimSpace(in.ClientID),
		SealedSecret: cur.SealedSecret,
		AllowSignup:  in.AllowSignup,
		HidePassword: in.HidePassword,
	}
	if len([]rune(row.ButtonName)) > 40 {
		return apperror.NewBadRequest("keep the button name to 40 characters")
	}
	if row.Issuer != "" {
		if err := checkIssuerURL(row.Issuer); err != nil {
			return err
		}
	}
	if len(row.ClientID) > 255 || len(row.Issuer) > 255 {
		return apperror.NewBadRequest("the provider address and client ID must be under 255 characters")
	}
	if secret := strings.TrimSpace(in.ClientSecret); secret != "" {
		sealed, err := sealWith(s.oidc.key, secret)
		if err != nil {
			return apperror.NewInternal(fmt.Errorf("encrypting client secret: %w", err))
		}
		row.SealedSecret = &sealed
	}
	if row.Enabled && (row.ButtonName == "" || row.Issuer == "" || row.ClientID == "" || row.SealedSecret == nil) {
		return apperror.NewBadRequest("fill in the button name, provider address, client ID and client secret before switching it on")
	}
	if err := s.oidc.store.SaveOIDCSettings(ctx, row); err != nil {
		return apperror.NewInternal(err)
	}
	s.oidc.mu.Lock()
	s.oidc.providers = map[string]*oidc.Provider{}
	s.oidc.mu.Unlock()
	return nil
}

// checkIssuerURL insists on https, except for a provider on this machine,
// so the client secret is never sent in the clear.
func checkIssuerURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return apperror.NewBadRequest("the provider address must be a full address, like https://auth.example.com/application/o/chronicle/")
	}
	host := u.Hostname()
	if u.Scheme == "https" || (u.Scheme == "http" && (host == "localhost" || host == "127.0.0.1" || host == "::1")) {
		return nil
	}
	return apperror.NewBadRequest("the provider address must start with https://")
}

// LoginOptions tells the sign-in page whether to show the provider button.
// A failed read shows the password form alone, so nobody is locked out.
func (s *authService) LoginOptions(ctx context.Context) LoginOptions {
	if s.oidc == nil {
		return LoginOptions{}
	}
	row, err := s.oidc.store.GetOIDCSettings(ctx)
	if err != nil {
		slog.Warn("reading sign-in provider", slog.Any("error", err))
		return LoginOptions{}
	}
	if !row.Enabled {
		return LoginOptions{}
	}
	return LoginOptions{ProviderName: row.ButtonName, HidePassword: row.HidePassword}
}

// SignInMethods lists how a person can sign in, for the account page.
// ProviderName is empty when no provider is switched on.
func (s *authService) SignInMethods(ctx context.Context, userID string) (SignInMethods, error) {
	user, err := s.repo.FindByID(ctx, userID)
	if err != nil {
		return SignInMethods{}, err
	}
	out := SignInMethods{HasPassword: hasUsablePassword(user)}
	opts := s.LoginOptions(ctx)
	if opts.ProviderName == "" {
		return out, nil
	}
	out.ProviderName = opts.ProviderName
	row, err := s.oidc.store.GetOIDCSettings(ctx)
	if err != nil {
		return out, apperror.NewInternal(err)
	}
	all, err := s.oidc.store.ListIdentities(ctx, userID)
	if err != nil {
		return out, apperror.NewInternal(err)
	}
	for _, id := range all {
		if id.Issuer == row.Issuer {
			out.Linked = append(out.Linked, id)
		}
	}
	return out, nil
}

func hasUsablePassword(u *User) bool { return strings.HasPrefix(u.PasswordHash, "$argon2id$") }

// UnlinkOIDC removes the person's link to the current provider. Someone
// with no password of their own can't unlink, or they'd have no way in.
func (s *authService) UnlinkOIDC(ctx context.Context, userID string) error {
	if err := s.oidcReady(); err != nil {
		return err
	}
	user, err := s.repo.FindByID(ctx, userID)
	if err != nil {
		return err
	}
	if !hasUsablePassword(user) {
		return apperror.NewBadRequest("set a password first (sign out and use Forgot password), so you can still sign in")
	}
	row, err := s.oidc.store.GetOIDCSettings(ctx)
	if err != nil {
		return apperror.NewInternal(err)
	}
	return s.oidc.store.UnlinkIdentities(ctx, userID, row.Issuer)
}

// provider returns the discovered provider, cached until the setting is
// saved again.
func (rt *oidcRuntime) provider(ctx context.Context, issuer string) (*oidc.Provider, error) {
	rt.mu.Lock()
	p := rt.providers[issuer]
	rt.mu.Unlock()
	if p != nil {
		return p, nil
	}
	dctx, cancel := context.WithTimeout(ctx, oidcDiscoverWait)
	defer cancel()
	p, err := oidc.NewProvider(dctx, issuer)
	if err != nil {
		return nil, apperror.NewBadRequest("Chronicle couldn't reach the provider at " + issuer + ". Check the address.")
	}
	rt.mu.Lock()
	rt.providers[issuer] = p
	rt.mu.Unlock()
	return p, nil
}

func (rt *oidcRuntime) oauthConfig(p *oidc.Provider, row oidcSettingsRow, secret string) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     row.ClientID,
		ClientSecret: secret,
		Endpoint:     p.Endpoint(),
		RedirectURL:  rt.redirectURL(),
		Scopes:       []string{oidc.ScopeOpenID, "email", "profile"},
	}
}

// BeginOIDC starts a round trip to the provider and returns where to send
// the browser and the state the callback must bring back. Linking and
// testing remember who asked, so the callback can check it's still them.
func (s *authService) BeginOIDC(ctx context.Context, mode, userID, redirect string) (string, string, error) {
	if err := s.oidcReady(); err != nil {
		return "", "", err
	}
	if s.redis == nil {
		return "", "", apperror.NewInternal(errors.New("provider sign-in needs Redis"))
	}
	row, err := s.oidc.store.GetOIDCSettings(ctx)
	if err != nil {
		return "", "", apperror.NewInternal(err)
	}
	if mode != OIDCModeTest && !row.Enabled {
		return "", "", apperror.NewNotFound("sign-in with a provider is switched off")
	}
	if row.Issuer == "" || row.ClientID == "" || row.SealedSecret == nil {
		return "", "", apperror.NewBadRequest("fill in and save the provider setting first")
	}
	secret, err := openWith(s.oidc.key, *row.SealedSecret)
	if err != nil {
		return "", "", apperror.NewInternal(fmt.Errorf("opening client secret: %w", err))
	}
	p, err := s.oidc.provider(ctx, row.Issuer)
	if err != nil {
		if mode == OIDCModeTest {
			_ = s.oidc.store.RecordOIDCTest(ctx, false, "Couldn't reach the provider address")
		}
		return "", "", err
	}
	pending := oidcPending{Mode: mode, UserID: userID, Redirect: redirect, Nonce: randomHex(16), Verifier: oauth2.GenerateVerifier()}
	state := randomHex(24)
	raw, _ := json.Marshal(pending)
	if err := s.redis.Set(ctx, oidcStateKeyPrefix+hashToken(state), raw, oidcStateTTL).Err(); err != nil {
		return "", "", apperror.NewInternal(fmt.Errorf("storing sign-in state: %w", err))
	}
	authURL := s.oidc.oauthConfig(p, row, secret).AuthCodeURL(state,
		oidc.Nonce(pending.Nonce), oauth2.S256ChallengeOption(pending.Verifier))
	return authURL, state, nil
}

// FinishOIDC completes a round trip: the state is used up, the code is
// exchanged (with PKCE) and the ID token verified against the nonce.
func (s *authService) FinishOIDC(ctx context.Context, in OIDCCallbackInput) (*OIDCResult, error) {
	if err := s.oidcReady(); err != nil {
		return nil, err
	}
	if s.redis == nil || in.State == "" {
		return nil, apperror.NewUnauthorized("that sign-in took too long; start again")
	}
	key := oidcStateKeyPrefix + hashToken(in.State)
	raw, err := s.redis.GetDel(ctx, key).Result()
	if errors.Is(err, redis.Nil) {
		return nil, apperror.NewUnauthorized("that sign-in took too long; start again")
	}
	if err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("reading sign-in state: %w", err))
	}
	var pending oidcPending
	if err := json.Unmarshal([]byte(raw), &pending); err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("reading sign-in state: %w", err))
	}
	res := &OIDCResult{Mode: pending.Mode, Redirect: pending.Redirect}

	row, err := s.oidc.store.GetOIDCSettings(ctx)
	if err != nil {
		return res, apperror.NewInternal(err)
	}
	claims, err := s.exchangeOIDC(ctx, row, pending, in)
	if pending.Mode == OIDCModeTest {
		s.recordOIDCTest(ctx, claims, err)
		return res, err
	}
	if err != nil {
		return res, err
	}

	switch pending.Mode {
	case OIDCModeLink:
		if in.SessionUserID == "" || in.SessionUserID != pending.UserID {
			return res, apperror.NewUnauthorized("sign in to Chronicle again, then link your account")
		}
		owner, err := s.oidc.store.FindIdentity(ctx, row.Issuer, claims.Subject)
		if err == nil && owner == pending.UserID {
			return res, nil
		}
		if err := s.oidc.store.LinkIdentity(ctx, pending.UserID, row.Issuer, claims.Subject, claims.Email); err != nil {
			return res, err
		}
		slog.Info("provider account linked", slog.String("user_id", pending.UserID))
		return res, nil
	case OIDCModeLogin:
		user, err := s.oidcAccount(ctx, row, claims)
		if err != nil {
			return res, err
		}
		if user.IsDisabled {
			return res, apperror.NewForbidden("your account has been disabled")
		}
		// The provider does its own checks, so two-factor isn't asked here.
		token, err := s.createSession(ctx, user, in.IP, in.UserAgent)
		if err != nil {
			return res, apperror.NewInternal(fmt.Errorf("creating session: %w", err))
		}
		if err := s.repo.UpdateLastLogin(ctx, user.ID); err != nil {
			slog.Warn("failed to update last login", slog.String("user_id", user.ID), slog.Any("error", err))
		}
		res.SessionToken, res.User = token, user
		slog.Info("user logged in with provider", slog.String("user_id", user.ID))
		return res, nil
	}
	return res, apperror.NewBadRequest("unknown sign-in")
}

// exchangeOIDC trades the code for tokens and returns the verified claims.
func (s *authService) exchangeOIDC(ctx context.Context, row oidcSettingsRow, pending oidcPending, in OIDCCallbackInput) (oidcClaims, error) {
	if in.ProviderError != "" {
		return oidcClaims{}, apperror.NewUnauthorized("the provider didn't sign you in (" + in.ProviderError + ")")
	}
	if in.Code == "" || row.Issuer == "" || row.SealedSecret == nil {
		return oidcClaims{}, apperror.NewBadRequest("the provider sent no sign-in code")
	}
	secret, err := openWith(s.oidc.key, *row.SealedSecret)
	if err != nil {
		return oidcClaims{}, apperror.NewInternal(fmt.Errorf("opening client secret: %w", err))
	}
	p, err := s.oidc.provider(ctx, row.Issuer)
	if err != nil {
		return oidcClaims{}, err
	}
	tok, err := s.oidc.oauthConfig(p, row, secret).Exchange(ctx, in.Code, oauth2.VerifierOption(pending.Verifier))
	if err != nil {
		slog.Warn("provider code exchange failed", slog.Any("error", err))
		return oidcClaims{}, apperror.NewUnauthorized("the provider refused the sign-in; check the client ID and secret")
	}
	rawID, _ := tok.Extra("id_token").(string)
	if rawID == "" {
		return oidcClaims{}, apperror.NewUnauthorized("the provider sent no ID token")
	}
	idt, err := p.Verifier(&oidc.Config{ClientID: row.ClientID}).Verify(ctx, rawID)
	if err != nil {
		slog.Warn("provider ID token rejected", slog.Any("error", err))
		return oidcClaims{}, apperror.NewUnauthorized("the provider's answer couldn't be checked")
	}
	if idt.Nonce != pending.Nonce {
		return oidcClaims{}, apperror.NewUnauthorized("the provider's answer doesn't belong to this sign-in")
	}
	var c struct {
		Email             string          `json:"email"`
		EmailVerified     json.RawMessage `json:"email_verified"`
		Name              string          `json:"name"`
		PreferredUsername string          `json:"preferred_username"`
	}
	if err := idt.Claims(&c); err != nil {
		return oidcClaims{}, apperror.NewUnauthorized("the provider's answer couldn't be read")
	}
	// Some providers send email_verified as the string "true".
	verified := strings.Trim(strings.ToLower(string(c.EmailVerified)), `"`) == "true"
	return oidcClaims{Subject: idt.Subject, Email: strings.ToLower(strings.TrimSpace(c.Email)),
		EmailVerified: verified, Name: c.Name, PreferredUsername: c.PreferredUsername}, nil
}

// oidcAccount finds or makes the Chronicle account for a provider sign-in:
// a linked account first; then, only when the provider vouches for the
// email, the account with that email (linking it); then, when the admin
// allows it and sign-up is open, a new account.
func (s *authService) oidcAccount(ctx context.Context, row oidcSettingsRow, c oidcClaims) (*User, error) {
	if userID, err := s.oidc.store.FindIdentity(ctx, row.Issuer, c.Subject); err == nil {
		_ = s.oidc.store.TouchIdentity(ctx, row.Issuer, c.Subject)
		return s.repo.FindByID(ctx, userID)
	} else if !isNotFound(err, new(*apperror.AppError)) {
		return nil, apperror.NewInternal(err)
	}
	if !c.EmailVerified || c.Email == "" {
		return nil, apperror.NewForbidden("your provider didn't confirm your email, so Chronicle can't match you to an account. Sign in with your password and link it under Account Settings.")
	}
	if user, err := s.repo.FindByEmail(ctx, c.Email); err == nil {
		if err := s.oidc.store.LinkIdentity(ctx, user.ID, row.Issuer, c.Subject, c.Email); err != nil {
			return nil, err
		}
		slog.Info("provider account linked by verified email", slog.String("user_id", user.ID))
		return user, nil
	}
	if !row.AllowSignup {
		return nil, apperror.NewForbidden("no Chronicle account uses this sign-in yet. Ask your site admin for an invite.")
	}
	mode, err := s.registrationMode(ctx)
	if err != nil || mode != registrationOpen {
		return nil, apperror.NewForbidden("this site isn't taking new sign-ups. Ask your site admin for an invite.")
	}
	n, err := s.repo.CountUsers(ctx)
	if err != nil || n == 0 {
		return nil, apperror.NewForbidden("the first account on a new site is made with a password")
	}
	user := &User{
		ID:           generateUUID(),
		Email:        c.Email,
		DisplayName:  oidcDisplayName(c),
		PasswordHash: oidcPasswordPrefix + randomHex(32),
		CreatedAt:    time.Now().UTC(),
	}
	if err := s.repo.Create(ctx, user); err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("creating user: %w", err))
	}
	if err := s.oidc.store.LinkIdentity(ctx, user.ID, row.Issuer, c.Subject, c.Email); err != nil {
		return nil, err
	}
	slog.Info("user registered through provider", slog.String("user_id", user.ID))
	return user, nil
}

// oidcDisplayName picks a name for a new account, trimmed to the 100
// characters the users table holds.
func oidcDisplayName(c oidcClaims) string {
	name := strings.TrimSpace(c.Name)
	if name == "" {
		name = strings.TrimSpace(c.PreferredUsername)
	}
	if name == "" {
		name, _, _ = strings.Cut(c.Email, "@")
	}
	if r := []rune(name); len(r) > 100 {
		name = string(r[:100])
	}
	if len([]rune(name)) < 2 {
		name = "New player"
	}
	return name
}

func (s *authService) recordOIDCTest(ctx context.Context, c oidcClaims, err error) {
	note := ""
	if err != nil {
		note = apperror.UserMessage(err, "the test sign-in failed")
	} else {
		v := "email not verified"
		if c.EmailVerified {
			v = "email verified"
		}
		who := oidcDisplayName(c)
		if c.Email != "" {
			who += " (" + c.Email + ", " + v + ")"
		}
		note = "Signed in as " + who
	}
	if rerr := s.oidc.store.RecordOIDCTest(ctx, err == nil, note); rerr != nil {
		slog.Warn("recording provider test", slog.Any("error", rerr))
	}
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
