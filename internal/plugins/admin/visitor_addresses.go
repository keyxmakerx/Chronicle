package admin

import (
	"net/http"

	"github.com/keyxmakerx/chronicle/internal/middleware"
)

// VisitorAddressesData feeds the Visitor addresses section of the Security
// page: the setting, the trusted proxies in use, and how this very request
// was read, so an admin can see whether their proxy is set up as chosen.
type VisitorAddressesData struct {
	Diagnosis middleware.ClientIPDiagnosis
	Trusted   []string
	Warnings  []string
}

// visitorHeaderOption is one choice in the header select. Values match the
// settings plugin's stored values.
type visitorHeaderOption struct {
	Value, Label string
}

// visitorHeaderOptions are the choices, in the order shown.
var visitorHeaderOptions = []visitorHeaderOption{
	{middleware.ClientIPAuto, "Automatic"},
	{middleware.ClientIPRealIP, "X-Real-IP (your proxy sets it)"},
	{middleware.ClientIPForwardedFor, "X-Forwarded-For, read from the right"},
	{middleware.ClientIPDirect, "No proxy: use the connection's address"},
}

// visitorHeaderLabel names a stored value in plain words, for the change log.
func visitorHeaderLabel(value string) string {
	for _, o := range visitorHeaderOptions {
		if o.Value == value {
			return o.Label
		}
	}
	return value
}

// visitorAddressesFor reads req the way the server does and gathers the
// warnings that apply.
func visitorAddressesFor(req *http.Request) VisitorAddressesData {
	d := middleware.DiagnoseClientIP(req)
	return VisitorAddressesData{
		Diagnosis: d,
		Trusted:   middleware.TrustedProxyList(),
		Warnings:  visitorAddressWarnings(d),
	}
}

// visitorAddressWarnings are the plain-words problems with this request. A
// direct connection with the "no proxy" choice has none to report.
func visitorAddressWarnings(d middleware.ClientIPDiagnosis) []string {
	if d.Mode == middleware.ClientIPDirect {
		return nil
	}
	var out []string
	if !d.DirectTrusted {
		out = append(out, "Your proxy's address isn't in the trusted list. This request reached Chronicle from "+
			d.Direct+", so the headers were ignored. Add that address to TRUSTED_PROXY_CIDRS, or choose \"No proxy\" if nothing sits in front of Chronicle.")
	} else if d.HeaderMissing {
		switch d.Mode {
		case middleware.ClientIPRealIP:
			out = append(out, "Your proxy isn't setting this header. X-Real-IP did not arrive on this request, so Chronicle is using the proxy's own address for everyone.")
		case middleware.ClientIPForwardedFor:
			out = append(out, "Your proxy isn't setting this header. X-Forwarded-For did not arrive on this request, so Chronicle is using the proxy's own address for everyone.")
		default:
			out = append(out, "Your proxy isn't setting this header. Neither X-Real-IP nor X-Forwarded-For arrived on this request, so Chronicle is using the proxy's own address for everyone.")
		}
	}
	return out
}
