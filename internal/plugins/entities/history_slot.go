package entities

import (
	"context"
	"fmt"
	"net/url"
)

type openHistoryKey struct{}

// withOpenHistory carries a request to open the History panel as the page
// loads: a link to a page's history lands on the page itself, never on the
// bare panel. v names the version to show; "" or "1" means the newest.
func withOpenHistory(ctx context.Context, v string) context.Context {
	if v == "" {
		return ctx
	}
	return context.WithValue(ctx, openHistoryKey{}, v)
}

// openHistoryURL is the panel to load into the slot as the page opens, or "".
func openHistoryURL(ctx context.Context, campaignID, entityID string) string {
	v, _ := ctx.Value(openHistoryKey{}).(string)
	if v == "" {
		return ""
	}
	u := fmt.Sprintf("/campaigns/%s/entities/%s/history", campaignID, entityID)
	if v != "1" {
		u += "?v=" + url.QueryEscape(v)
	}
	return u
}
