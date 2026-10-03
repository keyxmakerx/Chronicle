package app

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/keyxmakerx/chronicle/internal/plugins/syncapi"
)

type fakeConnectorKeys struct {
	syncapi.SyncAPIService
	keys    []syncapi.APIKey
	created []syncapi.CreateAPIKeyInput
	userID  string
}

func (f *fakeConnectorKeys) ListKeysByCampaign(context.Context, string) ([]syncapi.APIKey, error) {
	return f.keys, nil
}

func (f *fakeConnectorKeys) CreateKey(_ context.Context, userID string, in syncapi.CreateAPIKeyInput) (*syncapi.CreateAPIKeyResult, error) {
	f.created = append(f.created, in)
	f.userID = userID
	return &syncapi.CreateAPIKeyResult{Key: &syncapi.APIKey{ID: 9}, RawKey: "chron_rawsecret"}, nil
}

type fakeHub struct {
	seen      *time.Time
	connected bool
}

func (f fakeHub) FoundryPresence(string) (*time.Time, bool) { return f.seen, f.connected }

func strp(s string) *string { return &s }

func TestFoundryConnectorAdapter_FoundryConnection(t *testing.T) {
	now := time.Now()
	at := func(d time.Duration) *time.Time { t := now.Add(-d); return &t }
	expired := now.Add(-time.Hour)

	tests := []struct {
		name        string
		keys        []syncapi.APIKey
		hub         fakeHub
		wantHasKey  bool
		wantPrefix  string
		wantVersion string
		wantKeyUsed bool
		wantConn    bool
	}{
		{"no keys", nil, fakeHub{}, false, "", "", false, false},
		{
			"inactive and expired keys are ignored",
			[]syncapi.APIKey{
				{KeyPrefix: "chron_aa", IsActive: false, LastUsedAt: at(time.Minute), ModuleVersion: strp("9.9.9")},
				{KeyPrefix: "chron_bb", IsActive: true, ExpiresAt: &expired, LastUsedAt: at(time.Minute), ModuleVersion: strp("8.8.8")},
			},
			fakeHub{}, false, "", "", false, false,
		},
		{
			"newest created key previews, most recently used key gives the version",
			[]syncapi.APIKey{
				{KeyPrefix: "chron_new", IsActive: true},
				{KeyPrefix: "chron_old", IsActive: true, LastUsedAt: at(time.Hour), ModuleVersion: strp("1.4.2")},
				{KeyPrefix: "chron_older", IsActive: true, LastUsedAt: at(48 * time.Hour), ModuleVersion: strp("1.0.0")},
			},
			fakeHub{}, true, "chron_new", "1.4.2", true, false,
		},
		{
			"custom-tagged keys belong to other tools and are ignored",
			[]syncapi.APIKey{
				{KeyPrefix: "chron_bot", IsActive: true, VTTTag: strp("custom"), LastUsedAt: at(time.Minute), ModuleVersion: strp("6.6.6")},
				{KeyPrefix: "chron_fvtt", IsActive: true, VTTTag: strp("foundry"), LastUsedAt: at(72 * time.Hour), ModuleVersion: strp("1.4.2")},
			},
			fakeHub{}, true, "chron_fvtt", "1.4.2", true, false,
		},
		{
			"hub presence passes through",
			[]syncapi.APIKey{{KeyPrefix: "chron_aa", IsActive: true}},
			fakeHub{seen: at(time.Second), connected: true}, true, "chron_aa", "", false, true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &foundryConnectorAdapter{keys: &fakeConnectorKeys{keys: tt.keys}, hub: tt.hub, baseURL: "https://chronicle.example.net"}
			got, err := a.FoundryConnection(context.Background(), "camp-1")
			if err != nil {
				t.Fatal(err)
			}
			if got.HasKey != tt.wantHasKey || got.KeyPrefix != tt.wantPrefix || got.ModuleVersion != tt.wantVersion ||
				(got.KeyLastUsed != nil) != tt.wantKeyUsed || got.Connected != tt.wantConn {
				t.Errorf("got %+v", got)
			}
			if tt.wantHasKey {
				want := "chronicle://chronicle.example.net/c/camp-1?key=" + tt.wantPrefix + "…"
				if got.LinePreview != want {
					t.Errorf("preview = %q, want %q", got.LinePreview, want)
				}
			} else if got.LinePreview != "" {
				t.Errorf("preview = %q, want none", got.LinePreview)
			}
		})
	}
}

func TestFoundryConnectorAdapter_NewFoundryConnectLine(t *testing.T) {
	t.Run("mints a read/write/sync key and returns the full line", func(t *testing.T) {
		keys := &fakeConnectorKeys{}
		a := &foundryConnectorAdapter{keys: keys, baseURL: "http://host:8080/sub"}
		line, err := a.NewFoundryConnectLine(context.Background(), "camp-1", "user-1")
		if err != nil {
			t.Fatal(err)
		}
		if want := "chronicle+http://host:8080/sub/c/camp-1?key=chron_rawsecret"; line != want {
			t.Errorf("line = %q, want %q", line, want)
		}
		if len(keys.created) != 1 || keys.userID != "user-1" {
			t.Fatalf("created = %+v by %q", keys.created, keys.userID)
		}
		in := keys.created[0]
		if in.Name != "Foundry connect line" || in.VTTTag != "foundry" || in.CampaignID != "camp-1" || len(in.Permissions) != 3 {
			t.Errorf("input = %+v", in)
		}
	})
	t.Run("unusable base url mints nothing", func(t *testing.T) {
		keys := &fakeConnectorKeys{}
		a := &foundryConnectorAdapter{keys: keys, baseURL: "not a url"}
		if _, err := a.NewFoundryConnectLine(context.Background(), "camp-1", "user-1"); err == nil || strings.Contains(err.Error(), "chron_") {
			t.Errorf("err = %v, want a refusal", err)
		}
		if len(keys.created) != 0 {
			t.Error("a key was minted despite an unusable base URL")
		}
	})
}
