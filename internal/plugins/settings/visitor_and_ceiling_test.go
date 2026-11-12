package settings

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

func TestClientIPHeaderSetting(t *testing.T) {
	tests := []struct {
		name    string
		stored  string
		missing bool
		want    string
	}{
		{"never chosen reads as auto", "", true, ClientIPAuto},
		{"x-real-ip", ClientIPRealIP, false, ClientIPRealIP},
		{"x-forwarded-for", ClientIPForwardedFor, false, ClientIPForwardedFor},
		{"direct", ClientIPDirect, false, ClientIPDirect},
		{"garbage reads as auto", "leftmost", false, ClientIPAuto},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := &mockSettingsRepo{getFn: func(_ context.Context, key string) (string, error) {
				if key != KeyClientIPHeader {
					t.Errorf("read key %q", key)
				}
				if tc.missing {
					return "", apperror.NewNotFound("setting not found")
				}
				return tc.stored, nil
			}}
			got, err := NewSettingsService(repo).GetClientIPHeader(context.Background())
			if err != nil || got != tc.want {
				t.Errorf("got %q, %v; want %q", got, err, tc.want)
			}
		})
	}

	t.Run("update validates and stores", func(t *testing.T) {
		var stored string
		repo := &mockSettingsRepo{setFn: func(_ context.Context, key, value string) error {
			if key != KeyClientIPHeader {
				t.Errorf("wrote key %q", key)
			}
			stored = value
			return nil
		}}
		svc := NewSettingsService(repo)
		if err := svc.UpdateClientIPHeader(context.Background(), "nope"); err == nil {
			t.Error("accepted an unknown value")
		}
		if stored != "" {
			t.Errorf("stored %q for a rejected value", stored)
		}
		if err := svc.UpdateClientIPHeader(context.Background(), ClientIPForwardedFor); err != nil || stored != ClientIPForwardedFor {
			t.Errorf("stored %q, err %v", stored, err)
		}
	})
}

func TestMaxUploadCeiling(t *testing.T) {
	const mb = int64(1 << 20)
	i64 := func(v int64) *int64 { return &v }
	future := time.Now().Add(time.Hour)
	past := time.Now().Add(-time.Hour)

	tests := []struct {
		name   string
		global string // empty: unset (10 MB default).
		users  []UserStorageLimit
		want   int64
	}{
		{"site limit alone", strconv.FormatInt(5*mb, 10), nil, 5 * mb},
		{"unset site limit uses the 10 MB default", "", nil, 10 * mb},
		{"a larger personal limit raises it", strconv.FormatInt(5*mb, 10),
			[]UserStorageLimit{{MaxUploadSize: i64(50 * mb)}}, 50 * mb},
		{"a smaller personal limit does not lower it", strconv.FormatInt(5*mb, 10),
			[]UserStorageLimit{{MaxUploadSize: i64(1 * mb)}}, 5 * mb},
		{"an active bypass raises it", strconv.FormatInt(5*mb, 10),
			[]UserStorageLimit{{BypassMaxUpload: i64(80 * mb), BypassExpiresAt: &future}}, 80 * mb},
		{"an expired bypass is ignored", strconv.FormatInt(5*mb, 10),
			[]UserStorageLimit{{BypassMaxUpload: i64(80 * mb), BypassExpiresAt: &past}}, 5 * mb},
		{"the largest of several wins", strconv.FormatInt(5*mb, 10),
			[]UserStorageLimit{{MaxUploadSize: i64(20 * mb)}, {MaxUploadSize: i64(30 * mb)}, {BypassMaxUpload: i64(25 * mb), BypassExpiresAt: &future}}, 30 * mb},
		{"a zero personal limit resolves to the fallback", strconv.FormatInt(5*mb, 10),
			[]UserStorageLimit{{MaxUploadSize: i64(0)}}, 7 * mb},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := &mockSettingsRepo{
				getAllFn: func(context.Context) (map[string]string, error) {
					m := map[string]string{}
					if tc.global != "" {
						m[KeyMaxUploadSize] = tc.global
					}
					return m, nil
				},
				listUserLimitsFn: func(context.Context) ([]UserStorageLimitWithName, error) {
					var out []UserStorageLimitWithName
					for _, u := range tc.users {
						out = append(out, UserStorageLimitWithName{UserStorageLimit: u})
					}
					return out, nil
				},
			}
			got, err := NewSettingsService(repo).MaxUploadCeiling(context.Background(), 7*mb)
			if err != nil || got != tc.want {
				t.Errorf("ceiling = %d, %v; want %d", got, err, tc.want)
			}
		})
	}
}
