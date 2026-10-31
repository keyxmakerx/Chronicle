package bestiary

import (
	"bytes"
	"encoding/json"
	"strings"
)

// This file defends against stored XSS: bestiary widgets interpolate
// statblock string fields (organization, role, size, ...) into innerHTML, so
// an unsanitized statblock lets any authenticated user publish a creature
// whose fields execute HTML in every other user's browser.
//
// Defense is server-side, on the publish path (server = authority): every
// string in the statblock — keys and values, at any nesting depth — has the
// HTML metacharacters '<' and '>' removed. Angle brackets have no legitimate
// meaning in Draw Steel statblock text, and stripping (unlike escaping) is
// idempotent, so the same pass is safe on write, on read, and over
// already-stored rows.
//
// Attribute-context injection (quotes) is deliberately not handled here:
// quotes are legitimate in creature text ("The GM's ally"), and neutralizing
// attribute breakout is the rendering side's job (Chronicle.escapeAttr /
// escapeHtml in the widgets).

// stripAngles removes '<' and '>' from a string. Idempotent.
func stripAngles(s string) string {
	if !strings.ContainsAny(s, "<>") {
		return s
	}
	s = strings.ReplaceAll(s, "<", "")
	return strings.ReplaceAll(s, ">", "")
}

// stripAnglesPtr strips a nullable text column in place.
func stripAnglesPtr(s *string) {
	if s == nil {
		return
	}
	*s = stripAngles(*s)
}

// sanitizeJSONValue walks a decoded JSON tree stripping angle brackets from
// every string it finds — map keys, map values, and array elements alike
// (widgets iterate ability maps, so keys reach innerHTML too).
func sanitizeJSONValue(v any) any {
	switch t := v.(type) {
	case string:
		return stripAngles(t)
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			out[stripAngles(k)] = sanitizeJSONValue(val)
		}
		return out
	case []any:
		for i := range t {
			t[i] = sanitizeJSONValue(t[i])
		}
		return t
	default:
		// Numbers, bools, nulls carry no markup.
		return v
	}
}

// sanitizeStatblockJSON returns the statblock with angle brackets stripped
// from every string. Fast path: raw bytes with no '<'/'>' return unchanged
// (the overwhelmingly common case — one scan, zero allocations). On
// malformed JSON it returns the input unchanged; validateStatblock has
// already rejected malformed input on the write paths, and the read path
// must never turn a scan into an error.
func sanitizeStatblockJSON(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 || !bytes.ContainsAny(raw, "<>") {
		return raw
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return raw
	}
	clean, err := json.Marshal(sanitizeJSONValue(v))
	if err != nil {
		return raw
	}
	return clean
}

// sanitizePublicationInPlace scrubs the statblock and the denormalized
// columns extracted from it. Called on every row leaving the repository so
// rows stored unsanitized (or written by a path that skips the write-side
// pass) are still served clean. Idempotent; needs no migration.
func sanitizePublicationInPlace(p *Publication) {
	if p == nil {
		return
	}
	p.StatblockJSON = sanitizeStatblockJSON(p.StatblockJSON)
	stripAnglesPtr(p.Organization)
	stripAnglesPtr(p.Role)
}
