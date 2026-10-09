package campaigns

// foundry_connect_test.go pins the connection row on the owner's Foundry page:
// the connect-line wire format, the status wording and 24-hour cut-off, and
// the credential handling of the two routes (owner-only mint, never a raw key
// on a GET).

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
)

func TestBuildFoundryConnectLine(t *testing.T) {
	tests := []struct {
		name    string
		base    string
		camp    string
		key     string
		want    string
		wantErr bool
	}{
		{"https", "https://chronicle.example.net", "camp-1", "chron_abc", "chronicle://chronicle.example.net/c/camp-1?key=chron_abc", false},
		{"http with port", "http://192.168.1.5:8080", "camp-1", "chron_abc", "chronicle+http://192.168.1.5:8080/c/camp-1?key=chron_abc", false},
		{"https with port", "https://host.example:8443", "camp-1", "k", "chronicle://host.example:8443/c/camp-1?key=k", false},
		{"sub-path", "http://host:8080/sub", "camp-1", "k", "chronicle+http://host:8080/sub/c/camp-1?key=k", false},
		{"nested sub-path", "https://host/a/b", "camp-1", "k", "chronicle://host/a/b/c/camp-1?key=k", false},
		{"trailing slash", "https://host/sub/", "camp-1", "k", "chronicle://host/sub/c/camp-1?key=k", false},
		{"bare trailing slash", "https://host/", "camp-1", "k", "chronicle://host/c/camp-1?key=k", false},
		{"key needing escaping", "https://host", "camp-1", "a b&c=d/é", "chronicle://host/c/camp-1?key=a+b%26c%3Dd%2F%C3%A9", false},
		{"campaign id needing escaping", "https://host", "a/b", "k", "chronicle://host/c/a%2Fb?key=k", false},
		{"unsupported scheme", "ftp://host", "camp-1", "k", "", true},
		{"no host", "https://", "camp-1", "k", "", true},
		{"empty base", "", "camp-1", "k", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := BuildFoundryConnectLine(tt.base, tt.camp, tt.key)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("line = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestComputeFoundryStatus(t *testing.T) {
	now := time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC)
	ago := func(d time.Duration) *time.Time { t := now.Add(-d); return &t }

	tests := []struct {
		name      string
		conn      FoundryConnection
		wantLevel FoundryStatusLevel
		wantText  string
	}{
		{"connected now wins over stale keys", FoundryConnection{Connected: true, KeyLastUsed: ago(72 * time.Hour)}, FoundryConnectedNow, "Connected now"},
		{"hub under a minute", FoundryConnection{HubLastSeen: ago(20 * time.Second)}, FoundrySeenRecently, "Last seen just now"},
		{"hub minutes", FoundryConnection{HubLastSeen: ago(5 * time.Minute)}, FoundrySeenRecently, "Last seen 5 minutes ago"},
		{"singular hour", FoundryConnection{KeyLastUsed: ago(61 * time.Minute)}, FoundrySeenRecently, "Last seen 1 hour ago"},
		{"just under 24h is green", FoundryConnection{KeyLastUsed: ago(24*time.Hour - time.Minute)}, FoundrySeenRecently, "Last seen 23 hours ago"},
		{"exactly 24h is amber", FoundryConnection{KeyLastUsed: ago(24 * time.Hour)}, FoundrySeenLongAgo, "Last seen 1 day ago"},
		{"days", FoundryConnection{KeyLastUsed: ago(5 * 24 * time.Hour)}, FoundrySeenLongAgo, "Last seen 5 days ago"},
		{"never", FoundryConnection{}, FoundryNeverSeen, "Never connected"},
		{"hub newer than key", FoundryConnection{HubLastSeen: ago(10 * time.Minute), KeyLastUsed: ago(3 * 24 * time.Hour)}, FoundrySeenRecently, "Last seen 10 minutes ago"},
		{"key newer than hub", FoundryConnection{HubLastSeen: ago(3 * 24 * time.Hour), KeyLastUsed: ago(2 * time.Hour)}, FoundrySeenRecently, "Last seen 2 hours ago"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ComputeFoundryStatus(now, tt.conn)
			if got.Level != tt.wantLevel || got.Text != tt.wantText {
				t.Errorf("status = %+v, want level %q text %q", got, tt.wantLevel, tt.wantText)
			}
		})
	}
}

func TestFoundryRowData_Labels(t *testing.T) {
	tests := []struct {
		name        string
		data        FoundryRowData
		wantVersion string
		wantButton  string
	}{
		{"version known, key exists", FoundryRowData{ModuleVersion: "1.4.2", HasKey: true}, "module 1.4.2", "Make a new connect line"},
		{"version unknown, no key", FoundryRowData{}, "module version unknown", "Make a connect line"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.data.moduleVersionText(); got != tt.wantVersion {
				t.Errorf("version text = %q, want %q", got, tt.wantVersion)
			}
			if got := tt.data.connectButtonLabel(); got != tt.wantButton {
				t.Errorf("button = %q, want %q", got, tt.wantButton)
			}
		})
	}
}

// --- route-level behaviour ---

const (
	testRawKey    = "chron_0123456789abcdef0123456789abcdef"
	testKeyPrefix = "chron_01"
)

type fakeFoundryConnector struct {
	minted int
	userID string
	err    error
}

func (f *fakeFoundryConnector) FoundryConnection(context.Context, string) (FoundryConnection, error) {
	return FoundryConnection{
		HasKey:        true,
		KeyPrefix:     testKeyPrefix,
		ModuleVersion: "1.4.2",
		LinePreview:   "chronicle://host/c/camp-1?key=" + testKeyPrefix + "…",
	}, nil
}

func (f *fakeFoundryConnector) NewFoundryConnectLine(_ context.Context, campaignID, userID string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	f.minted++
	f.userID = userID
	return "chronicle://host/c/" + campaignID + "?key=" + testRawKey, nil
}

type fakeSessionAuth struct {
	auth.AuthService
	session *auth.Session
}

func (f *fakeSessionAuth) ValidateSession(context.Context, string) (*auth.Session, error) {
	return f.session, nil
}

// newFoundryRouteFixture mounts the real RegisterRoutes so the test pins the
// route table's owner gate, not a hand-copied group.
func newFoundryRouteFixture(role Role, connector FoundryConnector) *echo.Echo {
	svc := &stubPublicSvc{
		campaign: &Campaign{ID: "camp-1", Name: "Test"},
		member:   &CampaignMember{UserID: "user-1", Role: role},
	}
	h := NewHandler(svc)
	if connector != nil {
		h.SetFoundryConnector(connector)
	}
	e := echo.New()
	e.HTTPErrorHandler = func(err error, c echo.Context) {
		var ae *apperror.AppError
		if errors.As(err, &ae) {
			_ = c.NoContent(ae.Code)
			return
		}
		_ = c.NoContent(http.StatusInternalServerError)
	}
	RegisterRoutes(e, h, svc, &fakeSessionAuth{session: &auth.Session{UserID: "user-1"}})
	return e
}

func foundryRequest(method, target string) *http.Request {
	req := httptest.NewRequest(method, target, nil)
	req.AddCookie(&http.Cookie{Name: "chronicle_session", Value: "tok"})
	return req
}

func TestNewFoundryConnectLine_OwnerOnly(t *testing.T) {
	tests := []struct {
		name       string
		role       Role
		wantStatus int
		wantMinted int
	}{
		{"owner mints", RoleOwner, http.StatusOK, 1},
		{"scribe refused", RoleScribe, http.StatusForbidden, 0},
		{"player refused", RolePlayer, http.StatusForbidden, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fc := &fakeFoundryConnector{}
			e := newFoundryRouteFixture(tt.role, fc)
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, foundryRequest(http.MethodPost, "/campaigns/camp-1/extensions/foundry/connect-line"))
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if fc.minted != tt.wantMinted {
				t.Errorf("keys minted = %d, want %d", fc.minted, tt.wantMinted)
			}
			if tt.wantMinted == 0 && strings.Contains(rec.Body.String(), testRawKey) {
				t.Error("refused response leaked the key")
			}
		})
	}
}

func TestNewFoundryConnectLine_ShowsLineOnceAndIsNotCached(t *testing.T) {
	fc := &fakeFoundryConnector{}
	e := newFoundryRouteFixture(RoleOwner, fc)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, foundryRequest(http.MethodPost, "/campaigns/camp-1/extensions/foundry/connect-line"))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", got)
	}
	body := rec.Body.String()
	if n := strings.Count(body, "key="+testRawKey); n != 1 {
		t.Errorf("full line appears %d times, want exactly 1", n)
	}
	for _, want := range []string{"Paste this one line into Chronicle Sync", "be shown again", "foundry-connect-line", "Copy"} {
		if !strings.Contains(body, want) {
			t.Errorf("response missing %q", want)
		}
	}
	if strings.Contains(body, "<script") {
		t.Error("swapped fragment must not rely on a script sibling")
	}
	if fc.userID != "user-1" {
		t.Errorf("minted for %q, want the session user", fc.userID)
	}
}

func TestNewFoundryConnectLine_NoConnectorOrError(t *testing.T) {
	t.Run("no connector is a 404", func(t *testing.T) {
		e := newFoundryRouteFixture(RoleOwner, nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, foundryRequest(http.MethodPost, "/campaigns/camp-1/extensions/foundry/connect-line"))
		if rec.Code != http.StatusNotFound {
			t.Errorf("status = %d, want 404", rec.Code)
		}
	})
	t.Run("mint failure surfaces and shows no line", func(t *testing.T) {
		e := newFoundryRouteFixture(RoleOwner, &fakeFoundryConnector{err: apperror.NewInternal(errors.New("db down"))})
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, foundryRequest(http.MethodPost, "/campaigns/camp-1/extensions/foundry/connect-line"))
		if rec.Code != http.StatusInternalServerError {
			t.Errorf("status = %d, want 500", rec.Code)
		}
	})
}

func TestFoundryPage_GETNeverContainsRawKey(t *testing.T) {
	fc := &fakeFoundryConnector{}
	e := newFoundryRouteFixture(RoleOwner, fc)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, foundryRequest(http.MethodGet, "/campaigns/camp-1/foundry"))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if strings.Contains(body, testRawKey) {
		t.Error("page contains the raw key")
	}
	for _, want := range []string{
		"key=" + testKeyPrefix + "…",
		"The key is hidden after it is made, so this shows only its start.",
		"module 1.4.2",
		"Make a new connect line",
		"All keys",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("page missing %q", want)
		}
	}
	if fc.minted != 0 {
		t.Errorf("a GET minted %d keys, want 0", fc.minted)
	}
}

func TestFoundryPage_NoConnectorSaysUnavailable(t *testing.T) {
	e := newFoundryRouteFixture(RoleOwner, nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, foundryRequest(http.MethodGet, "/campaigns/camp-1/foundry"))
	body := rec.Body.String()
	if !strings.Contains(body, "Foundry sync is not available") || strings.Contains(body, "data-foundry-row") {
		t.Error("without a connector the page should say Foundry sync is unavailable and show no row")
	}
}

// The Foundry page is the owner's: a Scribe or Player is refused, not shown
// the connect line's start or what Foundry reports.
func TestFoundryPage_OwnerOnly(t *testing.T) {
	for _, role := range []Role{RoleScribe, RolePlayer} {
		e := newFoundryRouteFixture(role, &fakeFoundryConnector{})
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, foundryRequest(http.MethodGet, "/campaigns/camp-1/foundry"))
		if rec.Code != http.StatusForbidden {
			t.Errorf("role %v: status = %d, want 403", role, rec.Code)
		}
	}
}

func TestFoundryConnectRow_NoKeyHasNoPasteBox(t *testing.T) {
	cc := &CampaignContext{Campaign: &Campaign{ID: "camp-1"}}
	var sb strings.Builder
	data := FoundryRowData{Wired: true, Status: FoundryStatus{Level: FoundryNeverSeen, Text: "Never connected"}}
	if err := foundryConnectRow(cc, data, "csrf").Render(context.Background(), &sb); err != nil {
		t.Fatal(err)
	}
	html := sb.String()
	if strings.Contains(html, "data-foundry-preview") || strings.Contains(html, "hidden after it is made") {
		t.Error("no key should mean no paste box")
	}
	for _, want := range []string{"Make a connect line", "Never connected", "module version unknown"} {
		if !strings.Contains(html, want) {
			t.Errorf("row missing %q", want)
		}
	}
}
