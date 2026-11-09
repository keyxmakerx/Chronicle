package ai_workspace

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"
)

func getAs(t *testing.T, h func(*echo.Context) error, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, path, nil), rec)
	ownerCtx(c)
	if err := h(c); err != nil {
		t.Fatal(err)
	}
	return rec
}

// The AI button counts only the owner's own campaign's waiting requests, and
// the answer is never cached.
func TestAIChip_Counts(t *testing.T) {
	h, _, _, _ := requestRig(t)
	rec := getAs(t, h.AIChip, "/campaigns/camp-1/ai/chip")
	var got ChipCounts
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("%v: %s", err, rec.Body)
	}
	if got.Waiting != 1 || got.Warnings != 0 || got.Stopped {
		t.Errorf("counts = %+v, want one waiting", got)
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Error("chip counts may be cached")
	}
	// Without the connector there is nothing to count, and no error.
	bare := recordsHandler(&memTables{})
	if rec := getAs(t, bare.AIChip, "/campaigns/camp-1/ai/chip"); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"waiting":0`) {
		t.Errorf("no connector: %d %s", rec.Code, rec.Body)
	}
}

// The panel holds the waiting requests and copy and paste, the only place
// either lives, and links back to Your AI.
func TestAIPanel_HoldsRequestsAndPaste(t *testing.T) {
	h, _, _, _ := requestRig(t)
	html := getAs(t, h.AIPanel, "/campaigns/camp-1/ai/panel").Body.String()
	for _, want := range []string{`id="ai-panel-heading"`, "Claude asks for 1 change", `id="ai-requests"`, `id="ai-import-paste-area"`,
		`href="/campaigns/camp-1/ai"`, "data-ai-close", "Nothing waiting"} {
		if !strings.Contains(html, want) {
			t.Errorf("panel missing %q", want)
		}
	}
	if strings.Contains(html, "@paste.window") {
		t.Error("the panel takes every paste itself; only ai_chip.js may, and only an AI's answer")
	}
	// Copy and paste works without the connector; requests need it.
	bare := recordsHandler(&memTables{})
	html = getAs(t, bare.AIPanel, "/campaigns/camp-1/ai/panel").Body.String()
	if !strings.Contains(html, `id="ai-import-paste-area"`) || strings.Contains(html, `id="ai-requests"`) {
		t.Error("without the connector the panel should be copy and paste only")
	}
}
