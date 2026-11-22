package ai_workspace

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/importer"
	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/records"
)

// seenKind keeps the Seen its Apply was handed.
type seenKind struct{ got *string }

func (seenKind) Name() string  { return "seen" }
func (seenKind) Label() string { return "Seen kind" }
func (seenKind) Doc() string   { return "" }
func (seenKind) Plan(context.Context, string, records.Actor, records.Record) records.Plan {
	return records.Plan{Summary: "does a thing", Seen: "state-1"}
}
func (k seenKind) Apply(_ context.Context, _ string, _ records.Actor, r records.Record) error {
	*k.got = r.Seen
	return nil
}
func (seenKind) Export(context.Context, string, records.Actor) (string, error) { return "", nil }

// A row's Seen is a hidden field on the review and reaches Apply on commit.
func TestReviewRecordRow_CarriesSeen(t *testing.T) {
	for _, tc := range []struct {
		seen string
		want bool
	}{{"state-1", true}, {"", false}} {
		r := RecordRow{Index: 3, Label: "Seen kind", Action: records.ActionUpdate, Name: "x", Plan: records.Plan{Summary: "s", Seen: tc.seen}}
		var buf bytes.Buffer
		if err := reviewRecordRow("camp-1", r, true).Render(context.Background(), &buf); err != nil {
			t.Fatal(err)
		}
		has := strings.Contains(buf.String(), `name="rec_3_seen" value="state-1"`)
		if has != tc.want {
			t.Errorf("seen=%q: hidden field rendered = %v, want %v", tc.seen, has, tc.want)
		}
	}
}

func TestImport_RecordsCommitHandsSeenToApply(t *testing.T) {
	var got string
	h := NewHandler(nil)
	h.SetImportLookup(&stubLookup{})
	h.SetImportCommitter(importer.NewCommitter(nil))
	h.SetRecords(records.NewRegistry(seenKind{got: &got}))
	form := url.Values{"markdown_source": {"---\nkind: seen\nname: A\n---\n"}, "rec_0_include": {"on"}, "rec_0_seen": {"state-1"}}
	req := httptest.NewRequest(http.MethodPost, "/campaigns/camp-1/ai-workspace/import/commit", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	c := echo.New().NewContext(req, httptest.NewRecorder())
	ownerCtx(c)
	if err := h.CommitImport(c); err != nil {
		t.Fatal(err)
	}
	if got != "state-1" {
		t.Errorf("Apply was handed Seen %q, want state-1", got)
	}
}
