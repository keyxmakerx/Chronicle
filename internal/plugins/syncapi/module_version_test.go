package syncapi

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

// fakeUsageService records UpdateKeyLastUsed calls; the embedded interface
// nil-derefs on any other method so a stray call fails loudly.
type fakeUsageService struct {
	SyncAPIService
	calls []usageCall
}

type usageCall struct {
	id      int
	ip      string
	version string
}

func (f *fakeUsageService) UpdateKeyLastUsed(_ context.Context, id int, ip, moduleVersion string) error {
	f.calls = append(f.calls, usageCall{id, ip, moduleVersion})
	return nil
}

func TestModuleVersionFromHeader(t *testing.T) {
	tests := []struct {
		name   string
		header map[string]string
		want   string
	}{
		{"valid semver", map[string]string{moduleVersionHeader: "1.4.2"}, "1.4.2"},
		{"valid prerelease and build", map[string]string{moduleVersionHeader: "2.0.0-rc.1+build5"}, "2.0.0-rc.1+build5"},
		{"absent", nil, ""},
		{"empty", map[string]string{moduleVersionHeader: ""}, ""},
		{"too long", map[string]string{moduleVersionHeader: strings.Repeat("1", 33)}, ""},
		{"max length", map[string]string{moduleVersionHeader: strings.Repeat("1", 32)}, strings.Repeat("1", 32)},
		{"markup", map[string]string{moduleVersionHeader: "<b>1</b>"}, ""},
		{"space", map[string]string{moduleVersionHeader: "1.0 beta"}, ""},
		{"underscore", map[string]string{moduleVersionHeader: "1_0"}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := http.Header{}
			for k, v := range tt.header {
				h.Set(k, v)
			}
			if got := moduleVersionFromHeader(h); got != tt.want {
				t.Errorf("moduleVersionFromHeader = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRecordKeyUsage(t *testing.T) {
	tests := []struct {
		name      string
		key       *APIKey
		version   string
		wantCalls []usageCall
	}{
		{"valid version stored", &APIKey{ID: 7}, "1.4.2", []usageCall{{7, "10.0.0.1", "1.4.2"}}},
		{"absent version still stamps usage with empty version", &APIKey{ID: 7}, "", []usageCall{{7, "10.0.0.1", ""}}},
		{"synthetic session key never written", &APIKey{ID: synthKeySessionID}, "1.4.2", nil},
		{"nil key never written", nil, "1.4.2", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &fakeUsageService{}
			if err := recordKeyUsage(context.Background(), svc, tt.key, "10.0.0.1", tt.version); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(svc.calls) != len(tt.wantCalls) {
				t.Fatalf("calls = %v, want %v", svc.calls, tt.wantCalls)
			}
			for i := range tt.wantCalls {
				if svc.calls[i] != tt.wantCalls[i] {
					t.Errorf("call %d = %v, want %v", i, svc.calls[i], tt.wantCalls[i])
				}
			}
		})
	}
}
