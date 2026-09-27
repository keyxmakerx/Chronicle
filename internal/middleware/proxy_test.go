package middleware

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Three things depend on the IP extractor: the rate limiter keys its bucket
// on c.RealIP(), media serving is limited per IP, and every audit row
// records it. A trusted-proxy list that doesn't cover the real reverse
// proxy's address collapses all of these onto one shared bucket/address.

// TestParseTrustedProxies_RejectsWhatItCannotParse pins the change from silent
// skip to startup error. The old loop dropped an unparseable entry and carried
// on, so a typo in the deployment environment quietly stopped client IPs
// resolving and nothing anywhere said so.
func TestParseTrustedProxies_RejectsWhatItCannotParse(t *testing.T) {
	for _, tc := range []struct {
		name    string
		entries []string
		wantErr bool
		wantLen int
	}{
		{"a CIDR block", []string{"10.0.0.0/8"}, false, 1},
		{"a bare v4 address is one host", []string{"100.64.0.10"}, false, 1},
		{"a bare v6 address is one host", []string{"fd00::1"}, false, 1},
		{"blanks and spacing are tolerated", []string{" 10.0.0.0/8 ", "", "::1/128"}, false, 2},
		{"a typo is an error, not a skip", []string{"10.0.0.0/8", "192.168.1.0/33"}, true, 0},
		{"prose is an error", []string{"the docker bridge"}, true, 0},
		{"an empty list trusts nothing", nil, false, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseTrustedProxies(tc.entries)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("parsed %v without complaint; a bad entry must fail startup, "+
						"because the alternative is a silently wrong client IP", tc.entries)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseTrustedProxies(%v): %v", tc.entries, err)
			}
			if len(got) != tc.wantLen {
				t.Fatalf("parsed %d networks from %v, want %d", len(got), tc.entries, tc.wantLen)
			}
		})
	}
}

// TestIPExtractor_TrustsOnlyTheNamedProxy is the behaviour the whole file exists
// for: headers are believed from a trusted peer and ignored from everyone else.
func TestIPExtractor_TrustsOnlyTheNamedProxy(t *testing.T) {
	const (
		client = "203.0.113.7"
		proxy  = "100.64.0.10"
	)
	for _, tc := range []struct {
		name    string
		trusted []string
		peer    string
		realIP  string
		xff     string
		want    string
	}{
		{
			name:    "an untrusted peer cannot dictate the client IP",
			trusted: []string{"10.0.0.0/8"},
			peer:    proxy, realIP: client,
			want: proxy,
		},
		{
			name:    "the named proxy is believed",
			trusted: []string{proxy},
			peer:    proxy, realIP: client,
			want: client,
		},
		{
			// A mesh-network proxy sits in 100.64.0.0/10 and the
			// shipped default stops at the private ranges, so without an entry
			// every visitor is recorded as the proxy.
			name:    "the shipped default does not cover a mesh proxy",
			trusted: DefaultTrustedProxies,
			peer:    proxy, realIP: client,
			want: proxy,
		},
		{
			// Naming one host must not hand the same power to its neighbours.
			name:    "a neighbour of the named proxy is not trusted",
			trusted: []string{proxy},
			peer:    "100.64.0.11", realIP: client,
			want: "100.64.0.11",
		},
		{
			name:    "X-Real-IP outranks X-Forwarded-For",
			trusted: []string{proxy},
			peer:    proxy, realIP: client, xff: "198.51.100.9",
			want: client,
		},
		{
			name:    "X-Forwarded-For is read leftmost when it is all there is",
			trusted: []string{proxy},
			peer:    proxy, xff: client + ", 198.51.100.9",
			want: client,
		},
		{
			name:    "a trusted peer sending no headers is itself the client",
			trusted: []string{proxy},
			peer:    proxy,
			want:    proxy,
		},
		{
			// IPv4 loopback was trusted and IPv6 loopback was not, while ::1 was
			// the single largest source in the measured logs.
			name:    "IPv6 loopback is trusted by the default, like its IPv4 twin",
			trusted: DefaultTrustedProxies,
			peer:    "::1", realIP: client,
			want: client,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			trusted, err := ParseTrustedProxies(tc.trusted)
			if err != nil {
				t.Fatalf("ParseTrustedProxies: %v", err)
			}
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.RemoteAddr = net.JoinHostPort(tc.peer, "51000")
			if tc.realIP != "" {
				req.Header.Set("X-Real-IP", tc.realIP)
			}
			if tc.xff != "" {
				req.Header.Set("X-Forwarded-For", tc.xff)
			}
			if got := buildIPExtractor(trusted)(req); got != tc.want {
				t.Errorf("client IP resolved to %q, want %q", got, tc.want)
			}
		})
	}
}

// TestIPExtractor_SurvivesAMalformedRemoteAddr: RemoteAddr is normally
// "host:port", but a test server or an odd transport can hand over a bare
// address. It must not become a trusted empty string.
func TestIPExtractor_SurvivesAMalformedRemoteAddr(t *testing.T) {
	trusted, err := ParseTrustedProxies([]string{"10.0.0.0/8"})
	if err != nil {
		t.Fatalf("ParseTrustedProxies: %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.5" // no port
	req.Header.Set("X-Real-IP", "203.0.113.7")
	if got := buildIPExtractor(trusted)(req); got != "203.0.113.7" {
		t.Errorf("resolved %q from a portless RemoteAddr, want the forwarded client", got)
	}
}
