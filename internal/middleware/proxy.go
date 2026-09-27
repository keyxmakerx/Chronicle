package middleware

import (
	"fmt"
	"net"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
)

// DefaultTrustedProxies is the trusted-proxy list used when TRUSTED_PROXY_CIDRS
// is not set: loopback plus the private ranges a container platform puts its
// bridges on. It deliberately contains no public or carrier-grade range, so an
// unconfigured deployment trusts nothing that could arrive from the internet.
//
// A deployment whose reverse proxy reaches Chronicle from outside these ranges
// -- a mesh VPN peer, for instance, which lands in 100.64.0.0/10 -- MUST name
// that proxy in TRUSTED_PROXY_CIDRS or every visitor is recorded as the proxy.
// Name the proxy's own address as a /32 rather than its whole range: every host
// inside a trusted range can dictate the client IP of every request it relays.
var DefaultTrustedProxies = []string{
	"127.0.0.0/8",    // IPv4 loopback
	"::1/128",        // IPv6 loopback -- the counterpart to the line above
	"10.0.0.0/8",     // Docker default bridge
	"172.16.0.0/12",  // Docker bridge (alternate range)
	"192.168.0.0/16", // Common LAN
	"fd00::/8",       // IPv6 private
}

// TrustedProxies configures Echo to trust reverse proxy headers
// (X-Forwarded-For, X-Real-IP) from specific IP ranges, and returns an error
// if any entry is unparseable. Without this, c.RealIP() returns the proxy's
// address for every request, collapsing per-IP rate limiting into one shared
// bucket and writing the same address into every audit row.
//
// Entries may be CIDR blocks ("10.0.0.0/8") or bare addresses ("100.64.0.10",
// read as a single host). An unparseable entry is a startup ERROR, never a
// silent skip, so a typo in the deployment environment can't silently stop
// client IPs resolving.
//
// SECURITY: once a peer is trusted, the headers it sends decide what gets
// recorded. If the proxy APPENDS to X-Forwarded-For rather than replacing it,
// a visitor can supply the leftmost entry and choose the address in their own
// audit row — trust the specific proxy, and verify it sets X-Real-IP or
// overwrites X-Forwarded-For.
func TrustedProxies(e *echo.Echo, trustedCIDRs []string) error {
	// Echo's IPExtractor determines how c.RealIP() resolves the client IP.
	// We use a custom extractor that checks X-Forwarded-For and X-Real-IP
	// headers only when the direct connection comes from a trusted proxy.
	trusted, err := ParseTrustedProxies(trustedCIDRs)
	if err != nil {
		return err
	}
	e.IPExtractor = buildIPExtractor(trusted)
	return nil
}

// ParseTrustedProxies turns the configured entries into networks, rejecting
// anything it cannot parse. A bare address is read as a single host, so an
// operator naming one proxy does not have to remember the /32 suffix.
func ParseTrustedProxies(trustedCIDRs []string) ([]*net.IPNet, error) {
	var trusted []*net.IPNet
	for _, entry := range trustedCIDRs {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if _, network, err := net.ParseCIDR(entry); err == nil {
			trusted = append(trusted, network)
			continue
		}
		// A bare address is a single host. Width depends on the family, so
		// derive it rather than assuming /32 and mangling IPv6.
		ip := net.ParseIP(entry)
		if ip == nil {
			return nil, fmt.Errorf("trusted proxy %q is neither a CIDR block nor an IP address", entry)
		}
		bits := 8 * net.IPv6len
		if v4 := ip.To4(); v4 != nil {
			ip, bits = v4, 8*net.IPv4len
		}
		trusted = append(trusted, &net.IPNet{IP: ip, Mask: net.CIDRMask(bits, bits)})
	}
	return trusted, nil
}

// buildIPExtractor returns an Echo IPExtractor that trusts X-Forwarded-For
// and X-Real-IP headers only from connections originating in trusted networks.
func buildIPExtractor(trusted []*net.IPNet) echo.IPExtractor {
	return func(req *http.Request) string {
		// Get the direct connection IP (peer address).
		directIP := extractDirectIP(req.RemoteAddr)

		// Only trust forwarding headers if the direct connection is from a proxy.
		if !isTrusted(directIP, trusted) {
			return directIP
		}

		// Try X-Real-IP first (set by many reverse proxies including nginx).
		if realIP := req.Header.Get("X-Real-IP"); realIP != "" {
			return strings.TrimSpace(realIP)
		}

		// Fall back to X-Forwarded-For (comma-separated list, leftmost = client).
		if xff := req.Header.Get("X-Forwarded-For"); xff != "" {
			// The leftmost IP is the original client (if all proxies are trusted).
			parts := strings.SplitN(xff, ",", 2)
			if len(parts) > 0 {
				return strings.TrimSpace(parts[0])
			}
		}

		return directIP
	}
}

// extractDirectIP extracts the IP address from a "host:port" RemoteAddr string.
func extractDirectIP(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return remoteAddr
	}
	return host
}

// isTrusted returns true if the given IP falls within any of the trusted CIDRs.
func isTrusted(ipStr string, trusted []*net.IPNet) bool {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}
	for _, network := range trusted {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}
