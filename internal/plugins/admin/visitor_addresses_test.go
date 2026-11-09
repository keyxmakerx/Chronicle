package admin

import (
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/middleware"
)

func TestVisitorAddressWarnings(t *testing.T) {
	tests := []struct {
		name string
		d    middleware.ClientIPDiagnosis
		want string // Substring of the one expected warning; empty for none.
	}{
		{"header present", middleware.ClientIPDiagnosis{Mode: middleware.ClientIPRealIP, DirectTrusted: true}, ""},
		{"x-real-ip chosen but missing", middleware.ClientIPDiagnosis{Mode: middleware.ClientIPRealIP, DirectTrusted: true, HeaderMissing: true}, "Your proxy isn't setting this header"},
		{"x-forwarded-for chosen but missing", middleware.ClientIPDiagnosis{Mode: middleware.ClientIPForwardedFor, DirectTrusted: true, HeaderMissing: true}, "X-Forwarded-For did not arrive"},
		{"automatic with neither", middleware.ClientIPDiagnosis{Mode: middleware.ClientIPAuto, DirectTrusted: true, HeaderMissing: true}, "Neither X-Real-IP nor X-Forwarded-For"},
		{"peer not trusted", middleware.ClientIPDiagnosis{Mode: middleware.ClientIPRealIP, Direct: "192.0.2.9"}, "Your proxy's address isn't in the trusted list"},
		{"peer not trusted, automatic", middleware.ClientIPDiagnosis{Mode: middleware.ClientIPAuto, Direct: "192.0.2.9"}, "isn't in the trusted list"},
		{"no proxy chosen: nothing to warn", middleware.ClientIPDiagnosis{Mode: middleware.ClientIPDirect, Direct: "192.0.2.9"}, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := visitorAddressWarnings(tc.d)
			if tc.want == "" {
				if len(got) != 0 {
					t.Fatalf("warnings = %v, want none", got)
				}
				return
			}
			if len(got) != 1 || !strings.Contains(got[0], tc.want) {
				t.Fatalf("warnings = %v, want one containing %q", got, tc.want)
			}
		})
	}
}

func TestVisitorAddressesCardRenders(t *testing.T) {
	v := VisitorAddressesData{
		Diagnosis: middleware.ClientIPDiagnosis{
			Mode: middleware.ClientIPForwardedFor, Address: "203.0.113.7", Direct: "10.0.0.5",
			DirectTrusted: true, ForwardedFor: "<b>x</b>, 203.0.113.7",
		},
		Trusted:  []string{"10.0.0.0/8"},
		Warnings: []string{"Your proxy isn't setting this header."},
	}
	var sb strings.Builder
	if err := visitorAddressesCard(v, "tok").Render(context.Background(), &sb); err != nil {
		t.Fatal(err)
	}
	out := sb.String()
	for _, want := range []string{
		"Visitor addresses",
		"Chronicle sees you as", "203.0.113.7",
		"10.0.0.5", "10.0.0.0/8",
		"X-Real-IP received", "not sent",
		`hx-post="/admin/security/visitor-addresses"`,
		`value="tok"`,
		`value="x-forwarded-for" selected`,
		"Your proxy isn&#39;t setting this header.",
		"No proxy: use the connection&#39;s address",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("card is missing %q", want)
		}
	}
	if strings.Contains(out, "<b>x</b>") {
		t.Error("a raw header value was rendered unescaped")
	}
}

func TestNormalizeSecurityTabKeepsAddresses(t *testing.T) {
	if got := normalizeSecurityTab("addresses"); got != SecurityTabAddresses {
		t.Errorf("normalizeSecurityTab(addresses) = %q", got)
	}
}
