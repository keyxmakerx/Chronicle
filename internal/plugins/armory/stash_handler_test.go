package armory

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// TestUpdateStashRequest_DecodesThreeStates pins that the wire type tells an
// absent key from an explicit null, the whole point of the partial contract.
func TestUpdateStashRequest_DecodesThreeStates(t *testing.T) {
	tests := []struct {
		body                    string
		namePresent, locPresent bool
		locNull                 bool
	}{
		{`{"name":"A"}`, true, false, false},
		{`{"location":null}`, false, true, true},
		{`{"location":"X"}`, false, true, false},
		{`{}`, false, false, false},
	}
	for _, tc := range tests {
		var r updateStashRequest
		if err := json.Unmarshal([]byte(tc.body), &r); err != nil {
			t.Fatal(err)
		}
		in := r.input()
		if in.Name.Present() != tc.namePresent || in.Location.Present() != tc.locPresent || in.Location.IsNull() != tc.locNull {
			t.Errorf("%s: name=%v loc=%v null=%v", tc.body, in.Name.Present(), in.Location.Present(), in.Location.IsNull())
		}
	}
}

// TestStashHandler_Update_JSONAndForm drives the real handler against the
// service fakes: a name-only JSON push keeps the location, a form with a blank
// location clears it.
func TestStashHandler_Update_JSONAndForm(t *testing.T) {
	tests := []struct {
		name, ctype, body string
		wantName, wantLoc string
	}{
		{"json rename only", "application/json", `{"name":"New"}`, "New", "Cellar"},
		{"json null location", "application/json", `{"location":null}`, "Old", ""},
		{"form blank location clears", "application/x-www-form-urlencoded", "name=Old&location=", "Old", ""},
		{"form name only", "application/x-www-form-urlencoded", "name=New", "New", "Cellar"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newFx()
			st, _ := f.svc.CreateStash(t.Context(), "camp", Actor{"gm", rOwner}, CreateStashInput{Name: "Old", Location: "Cellar"})
			e := echo.New()
			req := httptest.NewRequest(http.MethodPut, "/", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", tc.ctype)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.SetParamNames("sid")
			c.SetParamValues(strconv.Itoa(st.ID))
			c.Set("campaign_context", &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp"}, MemberRole: campaigns.RoleOwner})
			if err := NewStashHandler(f.svc).UpdateStash(c); err != nil {
				t.Fatal(err)
			}
			got := f.repo.stashes[st.ID]
			if got.Name != tc.wantName || got.Location != tc.wantLoc {
				t.Fatalf("%q/%q", got.Name, got.Location)
			}
		})
	}
}
