// foundry_connect.go — the connection row on the owner's Foundry page:
// connection status, the one-paste connect line, and the POST that mints a
// fresh line.
//
// The campaigns plugin never touches the sync plugin's keys or the socket hub
// directly; the app wires a FoundryConnector adapter over both (plugins talk
// through interfaces). Everything that decides what the owner reads —
// status wording, the 24-hour freshness cut-off, the line format — is a pure
// function here so it is table-tested without a database or a socket.

package campaigns

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/a-h/templ"
)

// FoundryConnection is the raw, adapter-supplied picture of a campaign's
// Foundry link. Raw on purpose: the freshness rules live in
// ComputeFoundryStatus so they are tested in one place.
type FoundryConnection struct {
	// Connected is true while the websocket hub reports a live Foundry socket.
	Connected bool
	// HubLastSeen is the hub's last Foundry contact, nil if never recorded
	// since this process started.
	HubLastSeen *time.Time
	// KeyLastUsed is the newest last_used_at across the campaign's active
	// keys; it survives restarts, unlike the hub's in-memory record.
	KeyLastUsed *time.Time
	// ModuleVersion is the version reported by the most recently used active
	// key, empty when no module has reported one.
	ModuleVersion string
	// KeyPrefix is the stored, non-secret start of the newest active key.
	KeyPrefix string
	// HasKey is true when the campaign has at least one active, unexpired key.
	HasKey bool
	// LinePreview is the connect line built with KeyPrefix and a trailing
	// ellipsis. It carries nothing secret: the full key is never recoverable.
	LinePreview string
	// ServedVersion is the module version Chronicle serves this campaign,
	// empty when no module is installed or it could not be read.
	ServedVersion string
}

// FoundryConnector is what the campaigns plugin needs from the sync plugin
// and the websocket hub. Nil means the row degrades to a plain link.
type FoundryConnector interface {
	FoundryConnection(ctx context.Context, campaignID string) (FoundryConnection, error)
	// NewFoundryConnectLine mints a new key and returns the full connect line.
	// The raw key exists only inside the returned string.
	NewFoundryConnectLine(ctx context.Context, campaignID, userID string) (string, error)
}

// FoundryStatusLevel drives the dot colour; the text is the accessible form.
type FoundryStatusLevel string

const (
	FoundryConnectedNow FoundryStatusLevel = "connected"
	FoundrySeenRecently FoundryStatusLevel = "recent"
	FoundrySeenLongAgo  FoundryStatusLevel = "stale"
	FoundryNeverSeen    FoundryStatusLevel = "never"
)

// foundryFreshWindow is how long after the last contact the dot stays green.
const foundryFreshWindow = 24 * time.Hour

// FoundryStatus is the computed status line.
type FoundryStatus struct {
	Level FoundryStatusLevel
	Text  string
}

// ComputeFoundryStatus turns a connection into the dot level and wording.
// A live socket wins; otherwise the newer of the hub's last contact and the
// keys' last use decides, because the hub forgets on restart and a key
// forgets nothing.
func ComputeFoundryStatus(now time.Time, c FoundryConnection) FoundryStatus {
	if c.Connected {
		return FoundryStatus{Level: FoundryConnectedNow, Text: "Connected now"}
	}
	seen := latestTime(c.HubLastSeen, c.KeyLastUsed)
	if seen == nil {
		return FoundryStatus{Level: FoundryNeverSeen, Text: "Never connected"}
	}
	age := now.Sub(*seen)
	level := FoundrySeenRecently
	if age >= foundryFreshWindow {
		level = FoundrySeenLongAgo
	}
	return FoundryStatus{Level: level, Text: "Last seen " + relativeAgo(age)}
}

// latestTime returns the later of two optional times, nil when both are nil.
func latestTime(a, b *time.Time) *time.Time {
	switch {
	case a == nil:
		return b
	case b == nil:
		return a
	case b.After(*a):
		return b
	default:
		return a
	}
}

// relativeAgo words an age as "just now" or "N unit(s) ago". Equivalent of the
// sync plugin's compact helper but spelled out, since it reads as a sentence
// here and that helper is unexported.
func relativeAgo(d time.Duration) string {
	plural := func(n int, unit string) string {
		if n == 1 {
			return fmt.Sprintf("1 %s ago", unit)
		}
		return fmt.Sprintf("%d %ss ago", n, unit)
	}
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return plural(int(d.Minutes()), "minute")
	case d < 24*time.Hour:
		return plural(int(d.Hours()), "hour")
	default:
		return plural(int(d.Hours()/24), "day")
	}
}

// BuildFoundryConnectLine is the single definition of the connect-line wire
// format the Foundry module parses:
//
//	https base -> chronicle://host[:port]<path>/c/<campaignID>?key=<key>
//	http base  -> chronicle+http://host[:port]<path>/c/<campaignID>?key=<key>
//
// The path is the base URL's path without a trailing slash (sub-path
// installs), and the key is query-escaped.
func BuildFoundryConnectLine(baseURL, campaignID, key string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil {
		return "", fmt.Errorf("parsing base url: %w", err)
	}
	if u.Host == "" {
		return "", fmt.Errorf("base url has no host")
	}
	var scheme string
	switch u.Scheme {
	case "https":
		scheme = "chronicle"
	case "http":
		scheme = "chronicle+http"
	default:
		return "", fmt.Errorf("unsupported base url scheme %q", u.Scheme)
	}
	path := strings.TrimRight(u.EscapedPath(), "/")
	return fmt.Sprintf("%s://%s%s/c/%s?key=%s",
		scheme, u.Host, path, url.PathEscape(campaignID), url.QueryEscape(key)), nil
}

// FoundryRowData is everything the Foundry row renders.
type FoundryRowData struct {
	// Wired is false when no connector is configured; the row then shows only
	// the settings link.
	Wired         bool
	Status        FoundryStatus
	ModuleVersion string
	HasKey        bool
	// Preview is the non-secret line start shown for an existing key.
	Preview string
	// NewLine is the full line, set only on the response to a fresh mint.
	NewLine string
	// conn is the raw connection the row was built from, for the checklist.
	conn FoundryConnection
}

// newFoundryRowData derives the view model from a connection.
func newFoundryRowData(now time.Time, c FoundryConnection) FoundryRowData {
	return FoundryRowData{
		Wired:         true,
		Status:        ComputeFoundryStatus(now, c),
		ModuleVersion: c.ModuleVersion,
		HasKey:        c.HasKey,
		Preview:       c.LinePreview,
		conn:          c,
	}
}

// moduleVersionText is the tail of the status line.
func (d FoundryRowData) moduleVersionText() string {
	if d.ModuleVersion == "" {
		return "module version unknown"
	}
	return "module " + d.ModuleVersion
}

// connectButtonLabel is "Make a connect line" until a key exists.
func (d FoundryRowData) connectButtonLabel() string {
	if d.HasKey {
		return "Make a new connect line"
	}
	return "Make a connect line"
}

// foundryDotClass colours the status dot; the status text beside it carries
// the meaning for anyone who cannot see colour.
func foundryDotClass(l FoundryStatusLevel) string {
	switch l {
	case FoundryConnectedNow, FoundrySeenRecently:
		return "bg-emerald-500"
	case FoundrySeenLongAgo:
		return "bg-amber-500"
	default:
		return "bg-gray-400"
	}
}

// foundryCopyOnClick copies the connect line and flashes the button. It is an
// inline IIFE with no <script> sibling because the row is swapped in by HTMX,
// where sibling scripts do not reliably run. Nothing user-controlled is
// interpolated: the element id is a constant.
func foundryCopyOnClick() templ.ComponentScript {
	body := `(function(b){` +
		`var el=document.getElementById('foundry-connect-line');if(!el)return;` +
		`var done=function(){b.textContent='Copied';setTimeout(function(){b.textContent='Copy';},2000);};` +
		`if(navigator.clipboard&&navigator.clipboard.writeText){navigator.clipboard.writeText(el.textContent.trim()).then(done);return;}` +
		`var r=document.createRange();r.selectNodeContents(el);var s=window.getSelection();s.removeAllRanges();s.addRange(r);` +
		`try{document.execCommand('copy');done();}catch(e){}` +
		`})(this)`
	return templ.ComponentScript{Name: "campaigns_foundryCopy", Call: body}
}
