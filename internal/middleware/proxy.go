package middleware

import (
	"net"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
)

// TrustedProxies configures Echo to trust reverse proxy headers
// (X-Forwarded-For, X-Real-IP, X-Forwarded-Proto) from specific IP ranges.
//
// Chronicle runs behind Cosmos Cloud's reverse proxy. Without this config,
// c.RealIP() would always return the proxy's IP instead of the actual client.
// Rate limiting, audit logging, and abuse detection depend on accurate IPs.
//
// The trustedCIDRs parameter specifies which proxy IPs to trust. Common values:
//   - "127.0.0.1/8"   -- localhost (docker host)
//   - "10.0.0.0/8"    -- Docker default bridge network
//   - "172.16.0.0/12" -- Docker default bridge network (alternative range)
//   - "192.168.0.0/16" -- common LAN range
//   - "fd00::/8"      -- IPv6 private range
func TrustedProxies(e *echo.Echo, trustedCIDRs []string) {
	// Echo's IPExtractor determines how c.RealIP() resolves the client IP.
	// We use a custom extractor that checks X-Forwarded-For and X-Real-IP
	// headers only when the direct connection comes from a trusted proxy.
	e.IPExtractor = buildIPExtractor(trustedCIDRs)
}

// buildIPExtractor returns an Echo IPExtractor that trusts X-Forwarded-For
// and X-Real-IP headers only from connections originating in trusted CIDRs.
func buildIPExtractor(trustedCIDRs []string) echo.IPExtractor {
	// Parse trusted CIDRs into net.IPNet for fast matching.
	var trusted []*net.IPNet
	for _, cidr := range trustedCIDRs {
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			// Skip invalid CIDRs -- log would be better but this runs at startup.
			continue
		}
		trusted = append(trusted, network)
	}

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
