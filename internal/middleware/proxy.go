package middleware

import (
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync/atomic"

	"github.com/labstack/echo/v5"
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

// Values of the client-address setting. They match the settings plugin's
// stored values; middleware cannot import plugins, so app wires them across.
const (
	// ClientIPAuto reads X-Real-IP when present, otherwise X-Forwarded-For from
	// the right. It is what a site that never chose behaves as.
	ClientIPAuto = "auto"
	// ClientIPRealIP believes X-Real-IP only.
	ClientIPRealIP = "x-real-ip"
	// ClientIPForwardedFor believes X-Forwarded-For only, read from the right.
	ClientIPForwardedFor = "x-forwarded-for"
	// ClientIPDirect ignores every header and uses the connection's address.
	ClientIPDirect = "direct"
)

// IsClientIPMode reports whether mode is one of the four known values.
func IsClientIPMode(mode string) bool {
	switch mode {
	case ClientIPAuto, ClientIPRealIP, ClientIPForwardedFor, ClientIPDirect:
		return true
	}
	return false
}

var (
	// clientIPMode holds the current mode as a string. An atomic keeps the
	// per-request read free of locks and of any database call.
	clientIPMode atomic.Value
	// trustedState is what TrustedProxies last installed, kept so the Security
	// page can show the list and judge a request the way the extractor does.
	trustedState atomic.Pointer[trustedSnapshot]
)

type trustedSnapshot struct {
	entries []string
	nets    []*net.IPNet
}

// SetClientIPHeader chooses which header the extractor believes. An unknown
// value becomes auto, so a damaged setting never turns the headers off or
// back to reading the leftmost entry.
func SetClientIPHeader(mode string) {
	if !IsClientIPMode(mode) {
		mode = ClientIPAuto
	}
	clientIPMode.Store(mode)
}

// ClientIPHeader returns the mode in force.
func ClientIPHeader() string {
	if m, ok := clientIPMode.Load().(string); ok && m != "" {
		return m
	}
	return ClientIPAuto
}

// TrustedProxyList returns the trusted proxy entries in use, as configured.
func TrustedProxyList() []string {
	if t := trustedState.Load(); t != nil {
		return append([]string(nil), t.entries...)
	}
	return nil
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
// recorded. X-Forwarded-For is read from the right, so an entry the visitor
// wrote ahead of the proxy's own is never believed; X-Real-IP is believed as
// sent, so the proxy must overwrite it. Trust the specific proxy, and pick
// the header it really sets under Admin > Security > Visitor addresses.
func TrustedProxies(e *echo.Echo, trustedCIDRs []string) error {
	// Echo's IPExtractor determines how c.RealIP() resolves the client IP.
	// We use a custom extractor that reads the forwarding headers only when
	// the direct connection comes from a trusted proxy.
	trusted, err := ParseTrustedProxies(trustedCIDRs)
	if err != nil {
		return err
	}
	trustedState.Store(&trustedSnapshot{entries: append([]string(nil), trustedCIDRs...), nets: trusted})
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

// buildIPExtractor returns an Echo IPExtractor that reads forwarding headers
// only from connections originating in trusted networks, in the mode set by
// SetClientIPHeader.
func buildIPExtractor(trusted []*net.IPNet) echo.IPExtractor {
	return func(req *http.Request) string {
		return resolveClientIP(req, trusted, ClientIPHeader())
	}
}

// resolveClientIP is the whole decision: which address this request is from.
// Anything it cannot read as an address falls back to the connection's own.
func resolveClientIP(req *http.Request, trusted []*net.IPNet, mode string) string {
	directIP := extractDirectIP(req.RemoteAddr)

	// Only a proxy we trust gets to say who the visitor is.
	if mode == ClientIPDirect || !isTrusted(directIP, trusted) {
		return directIP
	}

	switch mode {
	case ClientIPRealIP:
		if ip := realIPHeader(req); ip != "" {
			return ip
		}
	case ClientIPForwardedFor:
		if ip := forwardedForClient(req, trusted); ip != "" {
			return ip
		}
	default:
		if ip := realIPHeader(req); ip != "" {
			return ip
		}
		if ip := forwardedForClient(req, trusted); ip != "" {
			return ip
		}
	}
	return directIP
}

// realIPHeader returns X-Real-IP when it holds an address, else "".
func realIPHeader(req *http.Request) string {
	v := strings.TrimSpace(req.Header.Get("X-Real-IP"))
	if net.ParseIP(v) == nil {
		return ""
	}
	return v
}

// forwardedForClient walks X-Forwarded-For from the right, skipping addresses
// inside the trusted list, and returns the first one outside it: a proxy
// appends the address it saw, so entries to the left of that were written by
// the visitor and prove nothing. When every entry is trusted (a visitor on the
// same private network) the leftmost is the originator. An entry that is not
// an address stops the walk with "", because nothing past it can be believed.
func forwardedForClient(req *http.Request, trusted []*net.IPNet) string {
	values := req.Header.Values("X-Forwarded-For")
	if len(values) == 0 {
		return ""
	}
	parts := strings.Split(strings.Join(values, ","), ",")
	leftmost := ""
	for i := len(parts) - 1; i >= 0; i-- {
		entry := strings.TrimSpace(parts[i])
		if net.ParseIP(entry) == nil {
			return ""
		}
		if !isTrusted(entry, trusted) {
			return entry
		}
		leftmost = entry
	}
	return leftmost
}

// ClientIPDiagnosis describes how one request was read, for the admin's
// Visitor addresses check.
type ClientIPDiagnosis struct {
	Mode string
	// Address is what Chronicle believes the visitor's address is.
	Address string
	// Direct is the connection's own address.
	Direct        string
	DirectTrusted bool
	// RealIP and ForwardedFor are the raw header values received.
	RealIP, ForwardedFor string
	// HeaderMissing is true when the header the mode relies on is absent or
	// unreadable on this request (never true for the direct mode).
	HeaderMissing bool
}

// DiagnoseClientIP reads req the way the extractor does and reports the inputs.
func DiagnoseClientIP(req *http.Request) ClientIPDiagnosis {
	var nets []*net.IPNet
	if t := trustedState.Load(); t != nil {
		nets = t.nets
	}
	mode := ClientIPHeader()
	direct := extractDirectIP(req.RemoteAddr)
	d := ClientIPDiagnosis{
		Mode:          mode,
		Address:       resolveClientIP(req, nets, mode),
		Direct:        direct,
		DirectTrusted: isTrusted(direct, nets),
		RealIP:        req.Header.Get("X-Real-IP"),
		ForwardedFor:  strings.Join(req.Header.Values("X-Forwarded-For"), ", "),
	}
	hasReal := realIPHeader(req) != ""
	hasXFF := forwardedForClient(req, nets) != ""
	switch mode {
	case ClientIPRealIP:
		d.HeaderMissing = !hasReal
	case ClientIPForwardedFor:
		d.HeaderMissing = !hasXFF
	case ClientIPAuto:
		d.HeaderMissing = !hasReal && !hasXFF
	}
	return d
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
