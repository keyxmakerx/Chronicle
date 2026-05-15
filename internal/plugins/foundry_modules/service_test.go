package foundry_modules

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// --- mocks ---

type mockRepo struct {
	getVersionFn               func(ctx context.Context, version string) (*ModuleVersion, error)
	insertVersionFn            func(ctx context.Context, v *ModuleVersion) error
	listVersionsFn             func(ctx context.Context, includeWithdrawn bool) ([]*ModuleVersion, error)
	latestAvailableFn          func(ctx context.Context) (*ModuleVersion, error)
	setStatusFn                func(ctx context.Context, version string, status ModuleStatus) error
	getCampaignTokenFn         func(ctx context.Context, campaignID string) (*CampaignToken, error)
	upsertCampaignTokenFn      func(ctx context.Context, t *CampaignToken) error
	bumpCampaignTokenVersionFn func(ctx context.Context, campaignID string) (int, error)
	campaignsUsingVersionFn    func(ctx context.Context, version string) ([]CampaignUsage, error)
	campaignsOlderThanFn       func(ctx context.Context, version string, less func(a, b string) bool) ([]CampaignUsage, error)
}

func (m *mockRepo) InsertVersion(ctx context.Context, v *ModuleVersion) error {
	if m.insertVersionFn != nil {
		return m.insertVersionFn(ctx, v)
	}
	return nil
}
func (m *mockRepo) GetVersion(ctx context.Context, version string) (*ModuleVersion, error) {
	if m.getVersionFn != nil {
		return m.getVersionFn(ctx, version)
	}
	return nil, nil
}
func (m *mockRepo) GetVersionByID(_ context.Context, _ string) (*ModuleVersion, error) {
	return nil, nil
}
func (m *mockRepo) GetVersionByGitHubReleaseID(_ context.Context, _ int64) (*ModuleVersion, error) {
	return nil, nil
}
func (m *mockRepo) ListVersions(ctx context.Context, includeWithdrawn bool) ([]*ModuleVersion, error) {
	if m.listVersionsFn != nil {
		return m.listVersionsFn(ctx, includeWithdrawn)
	}
	return nil, nil
}
func (m *mockRepo) SetVersionStatus(ctx context.Context, version string, status ModuleStatus) error {
	if m.setStatusFn != nil {
		return m.setStatusFn(ctx, version, status)
	}
	return nil
}
func (m *mockRepo) LatestAvailable(ctx context.Context) (*ModuleVersion, error) {
	if m.latestAvailableFn != nil {
		return m.latestAvailableFn(ctx)
	}
	return nil, nil
}
func (m *mockRepo) GetCampaignToken(ctx context.Context, campaignID string) (*CampaignToken, error) {
	if m.getCampaignTokenFn != nil {
		return m.getCampaignTokenFn(ctx, campaignID)
	}
	return nil, nil
}
func (m *mockRepo) UpsertCampaignToken(ctx context.Context, t *CampaignToken) error {
	if m.upsertCampaignTokenFn != nil {
		return m.upsertCampaignTokenFn(ctx, t)
	}
	return nil
}
func (m *mockRepo) BumpCampaignTokenVersion(ctx context.Context, campaignID string) (int, error) {
	if m.bumpCampaignTokenVersionFn != nil {
		return m.bumpCampaignTokenVersionFn(ctx, campaignID)
	}
	return 0, nil
}
func (m *mockRepo) CampaignsUsingVersion(ctx context.Context, version string) ([]CampaignUsage, error) {
	if m.campaignsUsingVersionFn != nil {
		return m.campaignsUsingVersionFn(ctx, version)
	}
	return nil, nil
}
func (m *mockRepo) CampaignsOlderThan(ctx context.Context, version string, less func(a, b string) bool) ([]CampaignUsage, error) {
	if m.campaignsOlderThanFn != nil {
		return m.campaignsOlderThanFn(ctx, version, less)
	}
	return nil, nil
}

type mockSettings struct {
	pin    string
	exists bool
	setFn  func(ctx context.Context, campaignID, version string) error
}

func (m *mockSettings) SetFoundryModulePin(ctx context.Context, campaignID, version string) error {
	if m.setFn != nil {
		return m.setFn(ctx, campaignID, version)
	}
	m.pin = version
	return nil
}
func (m *mockSettings) GetFoundryModulePin(_ context.Context, _ string) (string, error) {
	return m.pin, nil
}
func (m *mockSettings) CampaignExists(_ context.Context, _ string) (bool, error) {
	return m.exists, nil
}

type capturedEvent struct {
	eventType string
	userID    string
	actorID   string
	details   map[string]any
}

type mockEvents struct {
	events []capturedEvent
}

func (m *mockEvents) LogEvent(_ context.Context, eventType, userID, actorID, _, _ string, details map[string]any) error {
	m.events = append(m.events, capturedEvent{eventType, userID, actorID, details})
	return nil
}

type mockMail struct {
	configured bool
	sent       int
}

func (m *mockMail) IsConfigured(_ context.Context) bool { return m.configured }
func (m *mockMail) SendMail(_ context.Context, _ []string, _, _ string) error {
	m.sent++
	return nil
}

type mockOwners struct {
	email, name string
	err         error
}

func (m *mockOwners) GetCampaignOwnerEmail(_ context.Context, _ string) (string, string, error) {
	return m.email, m.name, m.err
}

// --- helpers ---

func newTestService(t *testing.T, repo *mockRepo, settings *mockSettings, events *mockEvents, mail *mockMail, owners *mockOwners) (Service, string) {
	t.Helper()
	dir, err := os.MkdirTemp("", "foundry-modules-test-*")
	if err != nil {
		t.Fatalf("mktemp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	tokens := NewTokenSigner("test-secret-key-32-bytes-long-----")
	svc := NewService(repo, tokens, settings, events, mail, owners, dir, "https://chronicle.test")
	return svc, dir
}

func assertConflict(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected conflict, got nil")
	}
	var ae *apperror.AppError
	if !errors.As(err, &ae) || ae.Code != http.StatusConflict {
		t.Fatalf("expected 409 conflict, got %v", err)
	}
}

func assertNotFound(t *testing.T, err error) {
	t.Helper()
	var ae *apperror.AppError
	if !errors.As(err, &ae) || ae.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %v", err)
	}
}

func assertBadRequest(t *testing.T, err error) {
	t.Helper()
	var ae *apperror.AppError
	if !errors.As(err, &ae) || ae.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %v", err)
	}
}

// buildTestZip produces a minimal Foundry module zip with the supplied
// module.json contents. Used to test the upload path end-to-end.
func buildTestZip(t *testing.T, manifest map[string]any) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	f, err := zw.Create("module.json")
	if err != nil {
		t.Fatalf("zip create: %v", err)
	}
	if err := json.NewEncoder(f).Encode(manifest); err != nil {
		t.Fatalf("encode manifest: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zip close: %v", err)
	}
	return buf.Bytes()
}

// --- upload ---

// Dispatch checklist: "Duplicate version upload rejected with clear error".
func TestUploadVersion_DuplicateRejected(t *testing.T) {
	repo := &mockRepo{
		getVersionFn: func(_ context.Context, v string) (*ModuleVersion, error) {
			if v == "0.1.0" {
				return &ModuleVersion{Version: "0.1.0"}, nil
			}
			return nil, nil
		},
	}
	svc, _ := newTestService(t, repo, &mockSettings{}, &mockEvents{}, &mockMail{}, &mockOwners{})

	zipBytes := buildTestZip(t, map[string]any{"id": "chronicle-sync", "version": "0.1.0"})
	_, err := svc.UploadVersion(context.Background(), UploadVersionInput{
		File:       bytes.NewReader(zipBytes),
		FileSize:   int64(len(zipBytes)),
		UploaderID: "admin-1",
	})
	assertConflict(t, err)
}

// Reject zips without id or version in module.json with a clear 400.
func TestUploadVersion_MissingManifestFieldsRejected(t *testing.T) {
	svc, _ := newTestService(t, &mockRepo{}, &mockSettings{}, &mockEvents{}, &mockMail{}, &mockOwners{})

	zipBytes := buildTestZip(t, map[string]any{"version": "0.1.0"}) // no id
	_, err := svc.UploadVersion(context.Background(), UploadVersionInput{
		File:       bytes.NewReader(zipBytes),
		FileSize:   int64(len(zipBytes)),
		UploaderID: "admin-1",
	})
	assertBadRequest(t, err)
}

// --- pin validation ---

// "Withdrawn version doesn't appear in owner's selectable list" is
// enforced two places: includeWithdrawn=false in ListVersions, AND
// ValidatePinnable rejecting a withdrawn target.
func TestValidatePinnable_RejectsWithdrawn(t *testing.T) {
	repo := &mockRepo{
		getVersionFn: func(_ context.Context, _ string) (*ModuleVersion, error) {
			return &ModuleVersion{Version: "0.1.5", Status: StatusWithdrawn}, nil
		},
	}
	svc, _ := newTestService(t, repo, &mockSettings{}, &mockEvents{}, &mockMail{}, &mockOwners{})

	err := svc.ValidatePinnable(context.Background(), "0.1.5")
	assertBadRequest(t, err)
}

func TestValidatePinnable_RejectsUnknown(t *testing.T) {
	svc, _ := newTestService(t, &mockRepo{}, &mockSettings{}, &mockEvents{}, &mockMail{}, &mockOwners{})
	err := svc.ValidatePinnable(context.Background(), "9.9.9")
	assertNotFound(t, err)
}

func TestValidatePinnable_AcceptsDeprecated(t *testing.T) {
	repo := &mockRepo{
		getVersionFn: func(_ context.Context, _ string) (*ModuleVersion, error) {
			return &ModuleVersion{Version: "0.1.5", Status: StatusDeprecated}, nil
		},
	}
	svc, _ := newTestService(t, repo, &mockSettings{}, &mockEvents{}, &mockMail{}, &mockOwners{})
	if err := svc.ValidatePinnable(context.Background(), "0.1.5"); err != nil {
		t.Errorf("deprecated should be pinnable: %v", err)
	}
}

// --- manifest resolution ---

// "Manifest endpoint returns pinned version (not latest) when pin set"
func TestGetVersionForCampaign_HonorsPin(t *testing.T) {
	pinned := &ModuleVersion{Version: "0.1.5", Status: StatusAvailable, ManifestJSON: json.RawMessage(`{"id":"x"}`)}
	latest := &ModuleVersion{Version: "0.2.0", Status: StatusAvailable, ManifestJSON: json.RawMessage(`{"id":"x"}`)}
	repo := &mockRepo{
		getVersionFn: func(_ context.Context, v string) (*ModuleVersion, error) {
			if v == "0.1.5" {
				return pinned, nil
			}
			return latest, nil
		},
		latestAvailableFn: func(_ context.Context) (*ModuleVersion, error) {
			return latest, nil
		},
	}
	settings := &mockSettings{pin: "0.1.5"}
	svc, _ := newTestService(t, repo, settings, &mockEvents{}, &mockMail{}, &mockOwners{})

	v, err := svc.GetVersionForCampaign(context.Background(), "camp-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v.Version != "0.1.5" {
		t.Errorf("expected pinned 0.1.5, got %s", v.Version)
	}
}

// "Manifest endpoint returns latest available when pin empty"
func TestGetVersionForCampaign_FallsBackToLatest(t *testing.T) {
	latest := &ModuleVersion{Version: "0.2.0", Status: StatusAvailable, ManifestJSON: json.RawMessage(`{"id":"x"}`)}
	repo := &mockRepo{
		latestAvailableFn: func(_ context.Context) (*ModuleVersion, error) {
			return latest, nil
		},
	}
	svc, _ := newTestService(t, repo, &mockSettings{pin: ""}, &mockEvents{}, &mockMail{}, &mockOwners{})

	v, err := svc.GetVersionForCampaign(context.Background(), "camp-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v.Version != "0.2.0" {
		t.Errorf("expected latest 0.2.0, got %s", v.Version)
	}
}

// "Manifest endpoint returns 404 when pinned version is withdrawn"
func TestGetVersionForCampaign_PinnedToWithdrawnReturns404(t *testing.T) {
	repo := &mockRepo{
		getVersionFn: func(_ context.Context, _ string) (*ModuleVersion, error) {
			return &ModuleVersion{Version: "0.1.5", Status: StatusWithdrawn}, nil
		},
	}
	settings := &mockSettings{pin: "0.1.5"}
	svc, _ := newTestService(t, repo, settings, &mockEvents{}, &mockMail{}, &mockOwners{})

	_, err := svc.GetVersionForCampaign(context.Background(), "camp-1")
	assertNotFound(t, err)
}

// --- token rotation invalidates old tokens (service-level) ---

// "Token rotation invalidates old tokens" — the service rejects a
// token whose embedded version doesn't match the repo's current value.
func TestVerifyManifestToken_OldVersionRejected(t *testing.T) {
	repo := &mockRepo{
		getCampaignTokenFn: func(_ context.Context, _ string) (*CampaignToken, error) {
			return &CampaignToken{CampaignID: "camp-1", TokenVersion: 2, RotatedAt: time.Now()}, nil
		},
	}
	svc, _ := newTestService(t, repo, &mockSettings{}, &mockEvents{}, &mockMail{}, &mockOwners{})

	// Sign a token at version=1 (the "old" version).
	signer := NewTokenSigner("test-secret-key-32-bytes-long-----")
	oldToken := signer.Sign("camp-1", 1)

	err := svc.VerifyManifestToken(context.Background(), "camp-1", oldToken)
	assertNotFound(t, err) // 404 — we don't tell Foundry it was a token-version mismatch specifically
}

func TestVerifyManifestToken_CurrentVersionAccepted(t *testing.T) {
	repo := &mockRepo{
		getCampaignTokenFn: func(_ context.Context, _ string) (*CampaignToken, error) {
			return &CampaignToken{CampaignID: "camp-1", TokenVersion: 3, RotatedAt: time.Now()}, nil
		},
	}
	svc, _ := newTestService(t, repo, &mockSettings{}, &mockEvents{}, &mockMail{}, &mockOwners{})

	signer := NewTokenSigner("test-secret-key-32-bytes-long-----")
	tok := signer.Sign("camp-1", 3)

	if err := svc.VerifyManifestToken(context.Background(), "camp-1", tok); err != nil {
		t.Errorf("current-version token should verify: %v", err)
	}
}

// --- force-pin audit-trail distinction ---

// "Force-pin creates foundry.module_force_pin event (not _notify)".
func TestForcePinCampaign_LogsForcePinEvent(t *testing.T) {
	repo := &mockRepo{
		getVersionFn: func(_ context.Context, _ string) (*ModuleVersion, error) {
			return &ModuleVersion{Version: "0.1.5", Status: StatusAvailable}, nil
		},
	}
	events := &mockEvents{}
	svc, _ := newTestService(t, repo, &mockSettings{}, events, &mockMail{}, &mockOwners{})

	err := svc.ForcePinCampaign(context.Background(), "camp-1", "0.1.5",
		"admin-1", "127.0.0.1", "test-agent")
	if err != nil {
		t.Fatalf("force-pin failed: %v", err)
	}

	if len(events.events) != 1 {
		t.Fatalf("expected 1 event logged, got %d", len(events.events))
	}
	if events.events[0].eventType != EventFoundryModuleForcePin {
		t.Errorf("expected event type %q, got %q", EventFoundryModuleForcePin, events.events[0].eventType)
	}
}

// --- notify only-older campaigns ---

// "Notify only fires for campaigns with pin < new version".
func TestNotifyOlderCampaigns_OnlyOlder(t *testing.T) {
	repo := &mockRepo{
		getVersionFn: func(_ context.Context, _ string) (*ModuleVersion, error) {
			return &ModuleVersion{Version: "0.2.0", Status: StatusAvailable}, nil
		},
		campaignsOlderThanFn: func(_ context.Context, _ string, less func(a, b string) bool) ([]CampaignUsage, error) {
			// Simulate the repo doing the filter via the supplied
			// semver-comparator. Pass through two older + one same.
			all := []CampaignUsage{
				{CampaignID: "older-a", PinnedTo: "0.1.0", OwnerUserID: "u1"},
				{CampaignID: "older-b", PinnedTo: "0.1.5", OwnerUserID: "u2"},
				{CampaignID: "same", PinnedTo: "0.2.0", OwnerUserID: "u3"},
			}
			var out []CampaignUsage
			for _, c := range all {
				if less(c.PinnedTo, "0.2.0") {
					out = append(out, c)
				}
			}
			return out, nil
		},
	}
	events := &mockEvents{}
	svc, _ := newTestService(t, repo, &mockSettings{}, events, &mockMail{}, &mockOwners{})

	notified, err := svc.NotifyOlderCampaigns(context.Background(), "0.2.0", "admin-1", "127.0.0.1", "test-agent")
	if err != nil {
		t.Fatalf("notify failed: %v", err)
	}
	if notified != 2 {
		t.Errorf("expected 2 notified (older only), got %d", notified)
	}
	for _, e := range events.events {
		if e.eventType != EventFoundryModuleUpdateNotify {
			t.Errorf("expected %q event, got %q", EventFoundryModuleUpdateNotify, e.eventType)
		}
	}
}

// --- token rotation logs event ---

func TestRotateCampaignToken_LogsEventAndReturnsURL(t *testing.T) {
	repo := &mockRepo{
		bumpCampaignTokenVersionFn: func(_ context.Context, _ string) (int, error) {
			return 5, nil
		},
	}
	settings := &mockSettings{exists: true}
	events := &mockEvents{}
	svc, _ := newTestService(t, repo, settings, events, &mockMail{}, &mockOwners{})

	url, err := svc.RotateCampaignToken(context.Background(), "camp-1", "owner-1", "127.0.0.1", "ua")
	if err != nil {
		t.Fatalf("rotate failed: %v", err)
	}
	if url == "" {
		t.Error("expected new install URL")
	}
	if len(events.events) != 1 || events.events[0].eventType != EventFoundryModuleTokenRotated {
		t.Errorf("expected token-rotated event, got %+v", events.events)
	}
}

// --- BuildManifestForCampaign rewrites URLs ---

func TestBuildManifestForCampaign_RewritesURLs(t *testing.T) {
	original := map[string]any{
		"id":       "chronicle-sync",
		"version":  "0.1.5",
		"manifest": "https://github.com/example/releases/latest/module.json",
		"download": "https://github.com/example/releases/latest/module.zip",
	}
	rawManifest, _ := json.Marshal(original)
	v := &ModuleVersion{
		Version:      "0.1.5",
		Status:       StatusAvailable,
		ManifestJSON: rawManifest,
		FilePath:     "/tmp/whatever.zip",
	}
	repo := &mockRepo{
		latestAvailableFn: func(_ context.Context) (*ModuleVersion, error) { return v, nil },
		getCampaignTokenFn: func(_ context.Context, _ string) (*CampaignToken, error) {
			return &CampaignToken{CampaignID: "camp-1", TokenVersion: 1}, nil
		},
	}
	svc, _ := newTestService(t, repo, &mockSettings{}, &mockEvents{}, &mockMail{}, &mockOwners{})

	manifest, downloadPath, err := svc.BuildManifestForCampaign(context.Background(), "camp-1")
	if err != nil {
		t.Fatalf("BuildManifest failed: %v", err)
	}
	if downloadPath != "/tmp/whatever.zip" {
		t.Errorf("expected file path passthrough, got %q", downloadPath)
	}

	var rewritten map[string]any
	if err := json.Unmarshal(manifest, &rewritten); err != nil {
		t.Fatalf("decode rewritten: %v", err)
	}
	manifestURL, _ := rewritten["manifest"].(string)
	downloadURL, _ := rewritten["download"].(string)
	if !contains(manifestURL, "chronicle.test") || !contains(manifestURL, "camp-1") {
		t.Errorf("manifest URL not rewritten: %q", manifestURL)
	}
	if !contains(downloadURL, "chronicle.test") || !contains(downloadURL, "module.zip") {
		t.Errorf("download URL not rewritten: %q", downloadURL)
	}
}

// --- duplicate-on-disk cleanup ---

// Failed insert (e.g. race on duplicate) must remove the on-disk zip
// so retries don't see orphaned files. Exercises the cleanup branch
// inside UploadVersion.
func TestUploadVersion_FailedInsertRemovesFile(t *testing.T) {
	repo := &mockRepo{
		// GetVersion returns nil so we proceed past the early dup check,
		// then InsertVersion fails — exercising the cleanup path.
		insertVersionFn: func(_ context.Context, _ *ModuleVersion) error {
			return ErrVersionExists
		},
	}
	svc, dir := newTestService(t, repo, &mockSettings{}, &mockEvents{}, &mockMail{}, &mockOwners{})

	zipBytes := buildTestZip(t, map[string]any{"id": "chronicle-sync", "version": "0.9.9"})
	_, err := svc.UploadVersion(context.Background(), UploadVersionInput{
		File:       bytes.NewReader(zipBytes),
		FileSize:   int64(len(zipBytes)),
		UploaderID: "admin-1",
	})
	assertConflict(t, err)

	// Confirm the zip file was cleaned up.
	if _, statErr := os.Stat(dir + "/0.9.9.zip"); !os.IsNotExist(statErr) {
		t.Error("expected zip file to be removed after failed insert")
	}
}

// --- banner status ---

// Banner status is the dashboard banner driver. The semantics:
//
//   - Pin older than latest available → HasUpdate=true.
//   - Pin equal to / newer than latest → HasUpdate=false.
//   - Unpinned (latest-tracking) campaign → HasUpdate=false (their
//     manifest endpoint already resolves to latest each fetch).
//   - Empty catalog → HasUpdate=false.
func TestGetBannerStatus_OlderPinShowsBanner(t *testing.T) {
	repo := &mockRepo{
		latestAvailableFn: func(_ context.Context) (*ModuleVersion, error) {
			return &ModuleVersion{Version: "0.2.0", Status: StatusAvailable}, nil
		},
	}
	settings := &mockSettings{pin: "0.1.5"}
	svc, _ := newTestService(t, repo, settings, &mockEvents{}, &mockMail{}, &mockOwners{})

	b, err := svc.GetBannerStatus(context.Background(), "camp-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !b.HasUpdate || b.CurrentVersion != "0.1.5" || b.LatestVersion != "0.2.0" {
		t.Errorf("expected HasUpdate=true with 0.1.5→0.2.0, got %+v", b)
	}
}

func TestGetBannerStatus_LatestTrackingCampaignSuppressesBanner(t *testing.T) {
	repo := &mockRepo{
		latestAvailableFn: func(_ context.Context) (*ModuleVersion, error) {
			return &ModuleVersion{Version: "0.2.0", Status: StatusAvailable}, nil
		},
	}
	settings := &mockSettings{pin: ""}
	svc, _ := newTestService(t, repo, settings, &mockEvents{}, &mockMail{}, &mockOwners{})

	b, _ := svc.GetBannerStatus(context.Background(), "camp-1")
	if b.HasUpdate {
		t.Error("latest-tracking campaigns should never see the banner")
	}
}

func TestGetBannerStatus_PinEqualToLatestSuppressesBanner(t *testing.T) {
	repo := &mockRepo{
		latestAvailableFn: func(_ context.Context) (*ModuleVersion, error) {
			return &ModuleVersion{Version: "0.2.0", Status: StatusAvailable}, nil
		},
	}
	settings := &mockSettings{pin: "0.2.0"}
	svc, _ := newTestService(t, repo, settings, &mockEvents{}, &mockMail{}, &mockOwners{})

	b, _ := svc.GetBannerStatus(context.Background(), "camp-1")
	if b.HasUpdate {
		t.Error("pin == latest should not show banner")
	}
}

func TestGetBannerStatus_EmptyCatalogSuppressesBanner(t *testing.T) {
	// LatestAvailable returns (nil, nil) when the catalog is empty.
	svc, _ := newTestService(t, &mockRepo{}, &mockSettings{pin: "0.1.0"}, &mockEvents{}, &mockMail{}, &mockOwners{})
	b, _ := svc.GetBannerStatus(context.Background(), "camp-1")
	if b.HasUpdate {
		t.Error("empty catalog should suppress banner")
	}
}

// helper: substring check that doesn't depend on strings package for the
// tiny number of uses in this file. Keeps imports minimal.
func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

