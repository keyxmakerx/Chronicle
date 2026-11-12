package middleware

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestResolveClientIP_Modes pins each setting value, and above all that an
// X-Forwarded-For entry written by the visitor ahead of the proxy's own is
// never the address Chronicle records.
func TestResolveClientIP_Modes(t *testing.T) {
	const (
		proxy   = "10.0.0.5"
		hop     = "10.0.0.6"
		visitor = "203.0.113.7"
		spoof   = "198.51.100.66"
	)
	trusted, err := ParseTrustedProxies([]string{"10.0.0.0/8"})
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name   string
		mode   string
		peer   string
		realIP string
		xff    []string
		want   string
	}{
		{"auto: X-Real-IP wins", ClientIPAuto, proxy, visitor, []string{spoof}, visitor},
		{"auto: XFF from the right when no X-Real-IP", ClientIPAuto, proxy, "", []string{spoof + ", " + visitor}, visitor},
		{"auto: spoofed leftmost XFF ignored", ClientIPAuto, proxy, "", []string{spoof + ", " + visitor}, visitor},
		{"auto: garbage X-Real-IP falls through to XFF", ClientIPAuto, proxy, "not-an-ip", []string{visitor}, visitor},
		{"auto: nothing sent is the peer", ClientIPAuto, proxy, "", nil, proxy},
		{"unknown mode reads as auto", "bogus", proxy, visitor, nil, visitor},

		{"x-real-ip: believed", ClientIPRealIP, proxy, visitor, []string{spoof}, visitor},
		{"x-real-ip: XFF is not consulted", ClientIPRealIP, proxy, "", []string{visitor}, proxy},
		{"x-real-ip: garbage is the peer", ClientIPRealIP, proxy, "<script>", nil, proxy},
		{"x-real-ip: empty is the peer", ClientIPRealIP, proxy, "  ", nil, proxy},

		{"xff: first untrusted from the right", ClientIPForwardedFor, proxy, spoof, []string{spoof + ", " + visitor}, visitor},
		{"xff: several trusted hops are skipped", ClientIPForwardedFor, proxy, "", []string{spoof + ", " + visitor + ", " + hop}, visitor},
		{"xff: repeated header lines read as one list", ClientIPForwardedFor, proxy, "", []string{spoof, visitor + ", " + hop}, visitor},
		{"xff: all trusted gives the leftmost", ClientIPForwardedFor, proxy, "", []string{"10.1.1.1, " + hop}, "10.1.1.1"},
		{"xff: garbage entry is the peer", ClientIPForwardedFor, proxy, "", []string{visitor + ", junk"}, proxy},
		{"xff: empty header is the peer", ClientIPForwardedFor, proxy, "", []string{""}, proxy},
		{"xff: IPv6 visitor", ClientIPForwardedFor, proxy, "", []string{"2001:db8::1"}, "2001:db8::1"},

		{"direct: headers ignored", ClientIPDirect, proxy, visitor, []string{visitor}, proxy},

		{"untrusted peer ignores X-Real-IP", ClientIPRealIP, "192.0.2.9", visitor, nil, "192.0.2.9"},
		{"untrusted peer ignores XFF", ClientIPForwardedFor, "192.0.2.9", "", []string{visitor}, "192.0.2.9"},
		{"untrusted peer ignores headers in auto", ClientIPAuto, "192.0.2.9", visitor, []string{spoof}, "192.0.2.9"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.RemoteAddr = net.JoinHostPort(tc.peer, "4000")
			if tc.realIP != "" {
				req.Header.Set("X-Real-IP", tc.realIP)
			}
			for _, v := range tc.xff {
				req.Header.Add("X-Forwarded-For", v)
			}
			if got := resolveClientIP(req, trusted, tc.mode); got != tc.want {
				t.Errorf("resolved %q, want %q", got, tc.want)
			}
		})
	}
}

// TestSetClientIPHeader checks the live switch the extractor reads per request.
func TestSetClientIPHeader(t *testing.T) {
	t.Cleanup(func() { SetClientIPHeader(ClientIPAuto) })
	trusted, _ := ParseTrustedProxies([]string{"10.0.0.0/8"})
	extract := buildIPExtractor(trusted)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.5:1"
	req.Header.Set("X-Real-IP", "203.0.113.7")

	for _, tc := range []struct{ set, want string }{
		{ClientIPDirect, "10.0.0.5"},
		{ClientIPRealIP, "203.0.113.7"},
		{"nonsense", "203.0.113.7"},
	} {
		SetClientIPHeader(tc.set)
		if got := extract(req); got != tc.want {
			t.Errorf("after setting %q: %q, want %q", tc.set, got, tc.want)
		}
	}
	if ClientIPHeader() != ClientIPAuto {
		t.Errorf("unknown value should read as auto, got %q", ClientIPHeader())
	}
}

// TestDiagnoseClientIP covers the inputs the Security page's warnings use.
func TestDiagnoseClientIP(t *testing.T) {
	t.Cleanup(func() { SetClientIPHeader(ClientIPAuto) })
	nets, _ := ParseTrustedProxies([]string{"10.0.0.0/8"})
	trustedState.Store(&trustedSnapshot{entries: []string{"10.0.0.0/8"}, nets: nets})
	t.Cleanup(func() { trustedState.Store(nil) })

	for _, tc := range []struct {
		name        string
		mode        string
		peer        string
		realIP, xff string
		wantAddr    string
		wantTrusted bool
		wantMissing bool
	}{
		{"real ip present", ClientIPRealIP, "10.0.0.5", "203.0.113.7", "", "203.0.113.7", true, false},
		{"real ip missing", ClientIPRealIP, "10.0.0.5", "", "203.0.113.7", "10.0.0.5", true, true},
		{"xff missing", ClientIPForwardedFor, "10.0.0.5", "203.0.113.7", "", "10.0.0.5", true, true},
		{"auto with neither", ClientIPAuto, "10.0.0.5", "", "", "10.0.0.5", true, true},
		{"direct never missing", ClientIPDirect, "10.0.0.5", "", "", "10.0.0.5", true, false},
		{"untrusted peer", ClientIPRealIP, "192.0.2.9", "203.0.113.7", "", "192.0.2.9", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			SetClientIPHeader(tc.mode)
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.RemoteAddr = net.JoinHostPort(tc.peer, "1")
			if tc.realIP != "" {
				req.Header.Set("X-Real-IP", tc.realIP)
			}
			if tc.xff != "" {
				req.Header.Set("X-Forwarded-For", tc.xff)
			}
			d := DiagnoseClientIP(req)
			if d.Address != tc.wantAddr || d.DirectTrusted != tc.wantTrusted || d.HeaderMissing != tc.wantMissing {
				t.Errorf("got %+v", d)
			}
			if d.Direct != tc.peer {
				t.Errorf("direct = %q, want %q", d.Direct, tc.peer)
			}
		})
	}
}
