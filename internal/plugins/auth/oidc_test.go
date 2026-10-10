package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc/oidctest"
	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// memOIDCStore is an in-memory OIDCStore.
type memOIDCStore struct {
	mu    sync.Mutex
	row   oidcSettingsRow
	links map[string]string // issuer|subject -> user id
	tests []string
}

func (m *memOIDCStore) GetOIDCSettings(context.Context) (oidcSettingsRow, error) { return m.row, nil }
func (m *memOIDCStore) SaveOIDCSettings(_ context.Context, r oidcSettingsRow) error {
	m.row = r
	return nil
}
func (m *memOIDCStore) RecordOIDCTest(_ context.Context, ok bool, note string) error {
	m.row.LastTestOK, m.row.LastTestNote = ok, note
	now := time.Now()
	m.row.LastTestAt = &now
	m.tests = append(m.tests, note)
	return nil
}
func (m *memOIDCStore) FindIdentity(_ context.Context, iss, sub string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if id, ok := m.links[iss+"|"+sub]; ok {
		return id, nil
	}
	return "", apperror.NewNotFound("no linked account")
}
func (m *memOIDCStore) LinkIdentity(_ context.Context, userID, iss, sub, _ string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.links == nil {
		m.links = map[string]string{}
	}
	if _, ok := m.links[iss+"|"+sub]; ok {
		return apperror.NewConflict("that account is already linked to another Chronicle account")
	}
	m.links[iss+"|"+sub] = userID
	return nil
}
func (m *memOIDCStore) TouchIdentity(context.Context, string, string) error { return nil }
func (m *memOIDCStore) ListIdentities(_ context.Context, userID string) ([]Identity, error) {
	var out []Identity
	for k, v := range m.links {
		if v == userID {
			iss, sub, _ := strings.Cut(k, "|")
			out = append(out, Identity{Issuer: iss, Subject: sub})
		}
	}
	return out, nil
}
func (m *memOIDCStore) UnlinkIdentities(_ context.Context, userID, iss string) error {
	for k, v := range m.links {
		if v == userID && strings.HasPrefix(k, iss+"|") {
			delete(m.links, k)
		}
	}
	return nil
}

// fakeProvider is a provider on httptest: discovery and keys from oidctest,
// plus a token endpoint that checks PKCE and signs the claims the test sets.
type fakeProvider struct {
	srv       *httptest.Server
	key       *rsa.PrivateKey
	challenge string // code_challenge from the last authorize URL
	nonce     string
	claims    map[string]any
	audience  string
}

func newFakeProvider(t *testing.T) *fakeProvider {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	fp := &fakeProvider{key: key, audience: "chronicle"}
	op := &oidctest.Server{PublicKeys: []oidctest.PublicKey{{PublicKey: key.Public(), KeyID: "k1", Algorithm: "RS256"}}}
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		sum := sha256.Sum256([]byte(r.PostForm.Get("code_verifier")))
		if base64.RawURLEncoding.EncodeToString(sum[:]) != fp.challenge || r.PostForm.Get("code") != "good-code" {
			http.Error(w, `{"error":"invalid_grant"}`, http.StatusBadRequest)
			return
		}
		claims := map[string]any{"iss": fp.srv.URL, "aud": fp.audience, "nonce": fp.nonce,
			"iat": time.Now().Unix(), "exp": time.Now().Add(time.Hour).Unix()}
		for k, v := range fp.claims {
			claims[k] = v
		}
		raw, _ := json.Marshal(claims)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "at", "token_type": "Bearer",
			"id_token": oidctest.SignIDToken(key, "k1", "RS256", string(raw))})
	})
	mux.Handle("/", op)
	fp.srv = httptest.NewServer(mux)
	op.SetIssuer(fp.srv.URL)
	t.Cleanup(fp.srv.Close)
	return fp
}

// readAuthURL notes the challenge and nonce the provider would receive.
func (fp *fakeProvider) readAuthURL(t *testing.T, raw string) {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	if q.Get("code_challenge_method") != "S256" || q.Get("client_id") != "chronicle" {
		t.Fatalf("authorize URL lacks PKCE or client: %s", raw)
	}
	fp.challenge, fp.nonce = q.Get("code_challenge"), q.Get("nonce")
}

type oidcRig struct {
	*twoFactorRig
	store *memOIDCStore
	op    *fakeProvider
	users map[string]*User // by email, for FindByEmail and Create
}

func newOIDCRig(t *testing.T) *oidcRig {
	t.Helper()
	tf := newTwoFactorRig(t)
	r := &oidcRig{twoFactorRig: tf, store: &memOIDCStore{}, op: newFakeProvider(t), users: map[string]*User{}}
	byID := tf.repo.findByIDFn
	tf.repo.findByIDFn = func(ctx context.Context, id string) (*User, error) {
		for _, u := range r.users {
			if u.ID == id {
				return u, nil
			}
		}
		return byID(ctx, id)
	}
	tf.repo.findByEmailFn = func(ctx context.Context, email string) (*User, error) {
		if u, ok := r.users[email]; ok {
			return u, nil
		}
		if strings.HasSuffix(email, "@example.com") {
			return tf.repo.findByIDFn(ctx, strings.TrimSuffix(email, "@example.com"))
		}
		return nil, apperror.NewNotFound("user not found")
	}
	tf.repo.createFn = func(_ context.Context, u *User) error {
		r.users[u.Email] = u
		return nil
	}
	tf.repo.countUsersFn = func(context.Context) (int, error) { return 3, nil }
	ConfigureOIDC(tf.svc, r.store, "test-site-secret-of-at-least-thirty-two", "https://chronicle.example.com")
	if err := tf.svc.SaveOIDCSettings(context.Background(), OIDCSettingsInput{
		Enabled: true, ButtonName: "Authentik", Issuer: r.op.srv.URL, ClientID: "chronicle", ClientSecret: "s3cret",
	}); err != nil {
		t.Fatalf("SaveOIDCSettings: %v", err)
	}
	return r
}

// roundTrip starts a sign-in and comes back with the given code.
func (r *oidcRig) roundTrip(t *testing.T, mode, userID, sessionUser, code string) (*OIDCResult, error) {
	t.Helper()
	ctx := context.Background()
	authURL, state, err := r.svc.BeginOIDC(ctx, mode, userID, "/campaigns/x")
	if err != nil {
		t.Fatalf("BeginOIDC: %v", err)
	}
	r.op.readAuthURL(t, authURL)
	return r.svc.FinishOIDC(ctx, OIDCCallbackInput{State: state, Code: code, SessionUserID: sessionUser})
}

func TestOIDCSettings(t *testing.T) {
	r := newOIDCRig(t)
	ctx := context.Background()
	got, _ := r.svc.OIDCSettings(ctx)
	if !got.HasSecret || got.RedirectURL != "https://chronicle.example.com/login/oidc/callback" {
		t.Fatalf("settings = %+v", got)
	}
	if strings.Contains(*r.store.row.SealedSecret, "s3cret") {
		t.Fatal("client secret stored in the clear")
	}
	// Saving without a secret keeps the stored one.
	if err := r.svc.SaveOIDCSettings(ctx, OIDCSettingsInput{Enabled: true, ButtonName: "Authentik", Issuer: r.op.srv.URL, ClientID: "chronicle"}); err != nil {
		t.Fatal(err)
	}
	if opened, _ := openWith(r.svc.oidc.key, *r.store.row.SealedSecret); opened != "s3cret" {
		t.Fatalf("secret after keep = %q", opened)
	}

	tests := []struct {
		name string
		in   OIDCSettingsInput
	}{
		{"plain http", OIDCSettingsInput{Issuer: "http://auth.example.com"}},
		{"not an address", OIDCSettingsInput{Issuer: "auth.example.com"}},
		{"on without a client id", OIDCSettingsInput{Enabled: true, ButtonName: "A", Issuer: "https://auth.example.com"}},
		{"long name", OIDCSettingsInput{ButtonName: strings.Repeat("x", 41)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertAppError(t, r.svc.SaveOIDCSettings(ctx, tt.in), http.StatusBadRequest)
		})
	}
}

func TestOIDCLogin(t *testing.T) {
	tests := []struct {
		name      string
		claims    map[string]any
		code      string
		signup    bool
		wantCode  int // 0 = signed in
		wantEmail string
	}{
		{"verified email matches an account", map[string]any{"sub": "s1", "email": "mara@example.com", "email_verified": true}, "good-code", false, 0, "mara@example.com"},
		{"verified as a string", map[string]any{"sub": "s2", "email": "mara@example.com", "email_verified": "true"}, "good-code", false, 0, "mara@example.com"},
		{"unverified email never matches", map[string]any{"sub": "s3", "email": "mara@example.com", "email_verified": false}, "good-code", false, http.StatusForbidden, ""},
		{"unknown person, sign-up off", map[string]any{"sub": "s4", "email": "new@else.org", "email_verified": true}, "good-code", false, http.StatusForbidden, ""},
		{"unknown person, sign-up on", map[string]any{"sub": "s5", "email": "new@else.org", "email_verified": true, "name": "Rook"}, "good-code", true, 0, "new@else.org"},
		{"bad code", map[string]any{"sub": "s6"}, "stolen", false, http.StatusUnauthorized, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newOIDCRig(t)
			r.store.row.AllowSignup = tt.signup
			r.op.claims = tt.claims
			res, err := r.roundTrip(t, OIDCModeLogin, "", "", tt.code)
			if tt.wantCode != 0 {
				assertAppError(t, err, tt.wantCode)
				return
			}
			if err != nil {
				t.Fatalf("FinishOIDC: %v", err)
			}
			if res.SessionToken == "" || res.User.Email != tt.wantEmail || res.Redirect != "/campaigns/x" {
				t.Fatalf("result = %+v", res)
			}
			if tt.signup && (strings.HasPrefix(res.User.PasswordHash, "$argon2id$") || res.User.DisplayName != "Rook" || res.User.IsAdmin) {
				t.Fatalf("new account = %+v", res.User)
			}
			// The link is remembered: the same subject signs in again even
			// though the email no longer matches.
			r.op.claims = map[string]any{"sub": tt.claims["sub"], "email": "changed@else.org"}
			again, err := r.roundTrip(t, OIDCModeLogin, "", "", "good-code")
			if err != nil || again.User.ID != res.User.ID {
				t.Fatalf("second sign-in: %+v, %v", again, err)
			}
		})
	}
}

func TestOIDCLoginSkipsTwoFactorAndStateIsSingleUse(t *testing.T) {
	r := newOIDCRig(t)
	ctx := context.Background()
	r.turnOn(t, "mara")
	r.op.claims = map[string]any{"sub": "s1", "email": "mara@example.com", "email_verified": true}

	authURL, state, err := r.svc.BeginOIDC(ctx, OIDCModeLogin, "", "")
	if err != nil {
		t.Fatal(err)
	}
	r.op.readAuthURL(t, authURL)
	res, err := r.svc.FinishOIDC(ctx, OIDCCallbackInput{State: state, Code: "good-code"})
	if err != nil || res.SessionToken == "" {
		t.Fatalf("provider sign-in with two-factor on: %+v, %v", res, err)
	}
	_, err = r.svc.FinishOIDC(ctx, OIDCCallbackInput{State: state, Code: "good-code"})
	assertAppError(t, err, http.StatusUnauthorized)

	// A wrong nonce (an ID token minted for another sign-in) is refused.
	authURL, state, _ = r.svc.BeginOIDC(ctx, OIDCModeLogin, "", "")
	r.op.readAuthURL(t, authURL)
	r.op.nonce = "someone-elses"
	_, err = r.svc.FinishOIDC(ctx, OIDCCallbackInput{State: state, Code: "good-code"})
	assertAppError(t, err, http.StatusUnauthorized)

	// A token for another client is refused.
	r.op.audience = "other-app"
	authURL, state, _ = r.svc.BeginOIDC(ctx, OIDCModeLogin, "", "")
	r.op.readAuthURL(t, authURL)
	_, err = r.svc.FinishOIDC(ctx, OIDCCallbackInput{State: state, Code: "good-code"})
	assertAppError(t, err, http.StatusUnauthorized)
}

func TestOIDCLinkAndUnlink(t *testing.T) {
	r := newOIDCRig(t)
	ctx := context.Background()
	// The provider's email doesn't match, so only linking connects them.
	r.op.claims = map[string]any{"sub": "s9", "email": "mara.work@corp.example", "email_verified": true}

	_, err := r.roundTrip(t, OIDCModeLink, "mara", "sam", "good-code")
	assertAppError(t, err, http.StatusUnauthorized)

	if _, err := r.roundTrip(t, OIDCModeLink, "mara", "mara", "good-code"); err != nil {
		t.Fatalf("link: %v", err)
	}
	res, err := r.roundTrip(t, OIDCModeLogin, "", "", "good-code")
	if err != nil || res.User.ID != "mara" {
		t.Fatalf("sign-in through link: %+v, %v", res, err)
	}
	_, err = r.roundTrip(t, OIDCModeLink, "sam", "sam", "good-code")
	assertAppError(t, err, http.StatusConflict)

	m, _ := r.svc.SignInMethods(ctx, "mara")
	if m.ProviderName != "Authentik" || len(m.Linked) != 1 || !m.HasPassword {
		t.Fatalf("methods = %+v", m)
	}
	if err := r.svc.UnlinkOIDC(ctx, "mara"); err != nil {
		t.Fatal(err)
	}
	if m, _ := r.svc.SignInMethods(ctx, "mara"); len(m.Linked) != 0 {
		t.Fatal("still linked")
	}
}

func TestOIDCUnlinkNeedsAPassword(t *testing.T) {
	r := newOIDCRig(t)
	r.store.row.AllowSignup = true
	r.op.claims = map[string]any{"sub": "s5", "email": "new@else.org", "email_verified": true}
	res, err := r.roundTrip(t, OIDCModeLogin, "", "", "good-code")
	if err != nil {
		t.Fatal(err)
	}
	assertAppError(t, r.svc.UnlinkOIDC(context.Background(), res.User.ID), http.StatusBadRequest)
}

func TestOIDCTestRecordsWithoutSigningIn(t *testing.T) {
	r := newOIDCRig(t)
	r.store.row.Enabled = false
	r.op.claims = map[string]any{"sub": "s1", "email": "mara@example.com", "email_verified": true, "name": "Mara"}
	res, err := r.roundTrip(t, OIDCModeTest, "admin", "", "good-code")
	if err != nil || res.SessionToken != "" {
		t.Fatalf("test: %+v, %v", res, err)
	}
	if !r.store.row.LastTestOK || r.store.row.LastTestNote != "Signed in as Mara (mara@example.com, email verified)" {
		t.Fatalf("test note = %v %q", r.store.row.LastTestOK, r.store.row.LastTestNote)
	}
	_, _ = r.roundTrip(t, OIDCModeTest, "admin", "", "bad")
	if r.store.row.LastTestOK {
		t.Fatal("a failed test recorded as working")
	}
}

func TestHiddenPasswordSignInIsAdminsOnly(t *testing.T) {
	r := newOIDCRig(t)
	ctx := context.Background()
	r.store.row.HidePassword = true
	_, _, err := r.svc.Login(ctx, LoginInput{Email: "mara@example.com", Password: "correct horse"})
	assertAppError(t, err, http.StatusForbidden)

	byID := r.repo.findByIDFn
	r.repo.findByIDFn = func(ctx context.Context, id string) (*User, error) {
		u, err := byID(ctx, id)
		if u != nil {
			u.IsAdmin = true
		}
		return u, err
	}
	if tok, _, err := r.svc.Login(ctx, LoginInput{Email: "mara@example.com", Password: "correct horse"}); err != nil || tok == "" {
		t.Fatalf("admin password sign-in refused: %v", err)
	}
	if opts := r.svc.LoginOptions(ctx); opts.ProviderName != "Authentik" || !opts.HidePassword {
		t.Fatalf("options = %+v", opts)
	}
}

func TestOIDCDisplayName(t *testing.T) {
	tests := []struct {
		c    oidcClaims
		want string
	}{
		{oidcClaims{Name: "Mara Vey", PreferredUsername: "mara"}, "Mara Vey"},
		{oidcClaims{PreferredUsername: "mara"}, "mara"},
		{oidcClaims{Email: "rook@example.com"}, "rook"},
		{oidcClaims{Email: "x@example.com"}, "New player"},
		{oidcClaims{Name: strings.Repeat("é", 120)}, strings.Repeat("é", 100)},
	}
	for _, tt := range tests {
		if got := oidcDisplayName(tt.c); got != tt.want {
			t.Errorf("oidcDisplayName(%+v) = %q, want %q", tt.c, got, tt.want)
		}
	}
}

// TestOIDCCallbackNeedsThisBrowsersState pins that a callback whose state
// doesn't match the cookie set when the sign-in started is refused before
// the service is asked, so a stolen callback link can't sign someone in.
func TestOIDCCallbackNeedsThisBrowsersState(t *testing.T) {
	tests := []struct {
		name, cookie, state string
	}{
		{"no cookie", "", "abc"},
		{"different cookie", "xyz", "abc"},
		{"no state", "abc", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := echo.New()
			req := httptest.NewRequest(http.MethodGet, "/login/oidc/callback?code=c&state="+tt.state, nil)
			if tt.cookie != "" {
				req.AddCookie(&http.Cookie{Name: oidcStateCookieName, Value: tt.cookie})
			}
			rec := httptest.NewRecorder()
			// The stub's FinishOIDC is nil: reaching it would panic.
			h := NewHandler(redirectStubService{}, time.Hour)
			if err := h.OIDCCallback(e.NewContext(req, rec)); err != nil {
				t.Fatal(err)
			}
			if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "didn&#39;t start in this browser") {
				t.Fatalf("got %d: %.200s", rec.Code, rec.Body.String())
			}
		})
	}
}
