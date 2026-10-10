package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// fakeGuestCodes is one campaign with codes that each work once.
type fakeGuestCodes struct {
	live     map[string]bool // normalized code -> unused
	admitted map[string]string
	released int
	admitErr error
}

func (f *fakeGuestCodes) ClaimGuestCode(_ context.Context, code string) (string, string, error) {
	if !f.live[code] {
		return "", "", apperror.NewNotFound("that code isn't right")
	}
	f.live[code] = false
	return "code-" + code, "camp-1", nil
}

func (f *fakeGuestCodes) ReleaseGuestCode(_ context.Context, codeID string) {
	f.released++
	f.live[strings.TrimPrefix(codeID, "code-")] = true
}

func (f *fakeGuestCodes) AdmitWithGuestCode(_ context.Context, _, campaignID, userID string) error {
	if f.admitErr != nil {
		return f.admitErr
	}
	f.admitted[userID] = campaignID
	return nil
}

type guestRig struct {
	*twoFactorRig
	codes *fakeGuestCodes
	users map[string]*User // guests and kept accounts, by id
	moved []string
}

func newGuestRig(t *testing.T) *guestRig {
	t.Helper()
	tf := newTwoFactorRig(t)
	r := &guestRig{twoFactorRig: tf, users: map[string]*User{},
		codes: &fakeGuestCodes{live: map[string]bool{"ABCD2345": true, "WXYZ6789": true}, admitted: map[string]string{}}}
	byID := tf.repo.findByIDFn
	tf.repo.findByIDFn = func(ctx context.Context, id string) (*User, error) {
		if u, ok := r.users[id]; ok {
			cp := *u
			return &cp, nil
		}
		return byID(ctx, id)
	}
	tf.repo.createFn = func(_ context.Context, u *User) error {
		cp := *u
		r.users[u.ID] = &cp
		return nil
	}
	tf.repo.keepGuestFn = func(_ context.Context, id, email, hash string) error {
		u := r.users[id]
		if u == nil || u.GuestCampaignID == nil {
			return apperror.NewNotFound("guest account not found")
		}
		if email == "mara@example.com" {
			return apperror.NewConflict("that email already has an account")
		}
		u.Email, u.PasswordHash, u.GuestCampaignID = email, hash, nil
		return nil
	}
	ConfigureGuests(tf.svc, r.codes)
	OnGuestMerged(tf.svc, func(_ context.Context, campaignID, guestID, targetID string) error {
		r.moved = append(r.moved, campaignID+":"+guestID+"->"+targetID)
		return nil
	})
	return r
}

func (r *guestRig) join(t *testing.T, code, name string) *JoinResult {
	t.Helper()
	res, err := r.svc.JoinWithGuestCode(context.Background(), JoinInput{Code: code, Name: name})
	if err != nil {
		t.Fatalf("JoinWithGuestCode: %v", err)
	}
	return res
}

func TestGuestMayReach(t *testing.T) {
	const camp = "11111111-2222-3333-4444-555555555555"
	tests := []struct {
		path string
		want bool
	}{
		{"/campaigns/" + camp, true},
		{"/campaigns/" + camp + "/entities/x", true},
		{"/campaigns/" + camp + "x", false},
		{"/campaigns/" + camp + "/../other/members", false},
		{"/campaigns/other", false},
		{"/campaigns", false},
		{"/campaigns/new", false},
		{"/dashboard", false},
		{"/account", false},
		{"/account/password", false},
		{"/account/guest", true},
		{"/account/guest/keep", true},
		{"/join", true},
		{"/join/ABC123", false},
		{"/media/abc", true},
		{"/notifications/badge", true},
		{"/logout", true},
		{"/admin", false},
		{"/api/v1/campaigns/" + camp + "/entities", false},
		{"/embed/campaigns/" + camp + "/notes/x", true},
	}
	for _, tt := range tests {
		if got := guestMayReach(tt.path, camp); got != tt.want {
			t.Errorf("guestMayReach(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

func TestJoinWithGuestCode(t *testing.T) {
	r := newGuestRig(t)
	ctx := context.Background()

	res := r.join(t, "abcd-2345", "  Rook ")
	if res.SessionToken == "" || res.CampaignID != "camp-1" || r.codes.admitted[res.User.ID] != "camp-1" {
		t.Fatalf("join = %+v", res)
	}
	u := r.users[res.User.ID]
	if u.GuestCampaignID == nil || *u.GuestCampaignID != "camp-1" || u.DisplayName != "Rook" ||
		!IsGuestEmail(u.Email) || verifyPassword("", u.PasswordHash) || u.IsAdmin {
		t.Fatalf("guest account = %+v", u)
	}
	sess, err := r.svc.ValidateSession(ctx, res.SessionToken)
	if err != nil || sess.GuestCampaignID != "camp-1" {
		t.Fatalf("guest session = %+v, %v", sess, err)
	}

	// The same code works once.
	_, err = r.svc.JoinWithGuestCode(ctx, JoinInput{Code: "ABCD2345", Name: "Sam"})
	assertAppError(t, err, http.StatusNotFound)

	// A guest can't use a second code.
	_, err = r.svc.JoinWithGuestCode(ctx, JoinInput{Code: "WXYZ6789", SessionUserID: res.User.ID})
	assertAppError(t, err, http.StatusConflict)

	// Someone signed in joins with their own account and no new session.
	own, err := r.svc.JoinWithGuestCode(ctx, JoinInput{Code: "WXYZ6789", SessionUserID: "mara"})
	if err != nil || own.SessionToken != "" || r.codes.admitted["mara"] != "camp-1" {
		t.Fatalf("signed-in join = %+v, %v", own, err)
	}
}

func TestJoinWithGuestCodeRefusals(t *testing.T) {
	tests := []struct {
		name     string
		code     string
		guest    string
		regMode  string
		admitErr error
		want     int
		released int
	}{
		{"no code", "", "Rook", "", nil, http.StatusBadRequest, 0},
		{"short name", "ABCD2345", "R", "", nil, http.StatusBadRequest, 0},
		{"closed site", "ABCD2345", "Rook", "closed", nil, http.StatusForbidden, 0},
		{"invite-only site still takes guests", "ABCD2345", "Rook", "invite", nil, 0, 0},
		{"admit fails", "ABCD2345", "Rook", "", apperror.NewConflict("already in"), http.StatusConflict, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newGuestRig(t)
			r.codes.admitErr = tt.admitErr
			if tt.regMode != "" {
				r.svc.regPolicy = fakeRegPolicy{mode: tt.regMode}
			}
			_, err := r.svc.JoinWithGuestCode(context.Background(), JoinInput{Code: tt.code, Name: tt.guest})
			if tt.want == 0 {
				if err != nil {
					t.Fatalf("join: %v", err)
				}
				return
			}
			assertAppError(t, err, tt.want)
			if r.codes.released != tt.released {
				t.Fatalf("released %d codes, want %d", r.codes.released, tt.released)
			}
			if tt.released > 0 && (len(r.repo.anonymized) != 1 || !r.codes.live["ABCD2345"]) {
				t.Fatalf("a failed join left the guest or the code behind: %v", r.repo.anonymized)
			}
		})
	}
}

func TestKeepGuestAccount(t *testing.T) {
	r := newGuestRig(t)
	ctx := context.Background()
	res := r.join(t, "ABCD2345", "Rook")

	_, err := r.svc.KeepGuestAccount(ctx, res.User.ID, KeepGuestInput{Email: "rook@guest.invalid", Password: "long enough"}, "", "")
	assertAppError(t, err, http.StatusBadRequest)
	_, err = r.svc.KeepGuestAccount(ctx, res.User.ID, KeepGuestInput{Email: "rook@else.org", Password: "short"}, "", "")
	assertAppError(t, err, http.StatusBadRequest)
	_, err = r.svc.KeepGuestAccount(ctx, res.User.ID, KeepGuestInput{Email: "mara@example.com", Password: "long enough"}, "", "")
	assertAppError(t, err, http.StatusConflict)

	token, err := r.svc.KeepGuestAccount(ctx, res.User.ID, KeepGuestInput{Email: " Rook@Else.org ", Password: "long enough"}, "", "")
	if err != nil {
		t.Fatalf("KeepGuestAccount: %v", err)
	}
	if _, err := r.svc.ValidateSession(ctx, res.SessionToken); err == nil {
		t.Fatal("the guest session outlived keeping the account")
	}
	sess, err := r.svc.ValidateSession(ctx, token)
	if err != nil || sess.GuestCampaignID != "" {
		t.Fatalf("kept session = %+v, %v", sess, err)
	}
	u := r.users[res.User.ID]
	if u.Email != "rook@else.org" || !verifyPassword("long enough", u.PasswordHash) {
		t.Fatalf("kept account = %+v", u)
	}

	// A full account can't be "kept" again.
	_, err = r.svc.KeepGuestAccount(ctx, res.User.ID, KeepGuestInput{Email: "x@else.org", Password: "long enough"}, "", "")
	assertAppError(t, err, http.StatusBadRequest)
}

func TestMergeGuest(t *testing.T) {
	r := newGuestRig(t)
	ctx := context.Background()
	res := r.join(t, "ABCD2345", "Rook")

	_, _, err := r.svc.MergeGuest(ctx, res.User.ID, MergeGuestInput{Email: "mara@example.com", Password: "wrong"}, "", "")
	assertAppError(t, err, http.StatusUnauthorized)
	if len(r.moved) != 0 {
		t.Fatal("a wrong password moved things")
	}

	// Two-factor on the account merged into is asked for.
	secret, _ := r.turnOn(t, "mara")
	_, _, err = r.svc.MergeGuest(ctx, res.User.ID, MergeGuestInput{Email: "mara@example.com", Password: "correct horse"}, "", "")
	assertAppError(t, err, http.StatusBadRequest)

	token, user, err := r.svc.MergeGuest(ctx, res.User.ID, MergeGuestInput{
		Email: "mara@example.com", Password: "correct horse", Code: earlierCode(secret),
	}, "", "")
	if err != nil || user.ID != "mara" {
		t.Fatalf("MergeGuest: %+v, %v", user, err)
	}
	if len(r.moved) != 1 || r.moved[0] != "camp-1:"+res.User.ID+"->mara" {
		t.Fatalf("moved = %v", r.moved)
	}
	if len(r.repo.anonymized) != 1 || !strings.HasPrefix(r.repo.anonymized[0], res.User.ID+"|") {
		t.Fatalf("guest not ended: %v", r.repo.anonymized)
	}
	if _, err := r.svc.ValidateSession(ctx, res.SessionToken); err == nil {
		t.Fatal("the guest session outlived the merge")
	}
	if sess, err := r.svc.ValidateSession(ctx, token); err != nil || sess.UserID != "mara" {
		t.Fatalf("merged session = %+v, %v", sess, err)
	}
}

func TestMergeGuestStopsWhenAMoveFails(t *testing.T) {
	r := newGuestRig(t)
	res := r.join(t, "ABCD2345", "Rook")
	OnGuestMerged(r.svc, func(context.Context, string, string, string) error { return errors.New("db down") })
	_, _, err := r.svc.MergeGuest(context.Background(), res.User.ID, MergeGuestInput{Email: "sam@example.com", Password: "correct horse"}, "", "")
	assertAppError(t, err, http.StatusInternalServerError)
	if len(r.repo.anonymized) != 0 {
		t.Fatal("the guest was ended although their things didn't move")
	}
}

func TestEndGuestLeavesFullAccounts(t *testing.T) {
	r := newGuestRig(t)
	res := r.join(t, "ABCD2345", "Rook")
	r.svc.EndGuest(context.Background(), "mara")
	if len(r.repo.anonymized) != 0 {
		t.Fatal("EndGuest emptied a full account")
	}
	r.svc.EndGuest(context.Background(), res.User.ID)
	if len(r.repo.anonymized) != 1 {
		t.Fatal("EndGuest left the guest")
	}
}

func TestRequireAuthFencesGuests(t *testing.T) {
	r := newGuestRig(t)
	res := r.join(t, "ABCD2345", "Rook")
	mw := RequireAuth(r.svc)
	ok := func(c echo.Context) error { return c.NoContent(http.StatusOK) }
	tests := []struct {
		name, method, path string
		htmx               bool
		wantCode           int
		wantLoc            string
	}{
		{"own campaign", http.MethodGet, "/campaigns/camp-1/members", false, http.StatusOK, ""},
		{"dashboard", http.MethodGet, "/dashboard", false, http.StatusSeeOther, "/campaigns/camp-1"},
		{"new campaign by htmx", http.MethodGet, "/campaigns/new", true, http.StatusNoContent, ""},
		{"create campaign", http.MethodPost, "/campaigns", false, http.StatusForbidden, ""},
		{"api outside", http.MethodGet, "/api/keys", false, http.StatusForbidden, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: res.SessionToken})
			if tt.htmx {
				req.Header.Set("HX-Request", "true")
			}
			rec := httptest.NewRecorder()
			c := echo.New().NewContext(req, rec)
			if err := mw(ok)(c); err != nil {
				t.Fatal(err)
			}
			if rec.Code != tt.wantCode || rec.Header().Get("Location") != tt.wantLoc {
				t.Fatalf("got %d to %q", rec.Code, rec.Header().Get("Location"))
			}
			if tt.htmx && rec.Header().Get("HX-Redirect") != "/campaigns/camp-1" {
				t.Fatalf("HX-Redirect = %q", rec.Header().Get("HX-Redirect"))
			}
		})
	}
}
