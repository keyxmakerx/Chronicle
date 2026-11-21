package sessions

import (
	"context"
	"net/http/httptest"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

type fakeEventSource struct{ events []SessionEvent }

func (f fakeEventSource) SessionEvents(context.Context, string, int) ([]SessionEvent, error) {
	return f.events, nil
}

type fakeWorder struct{ words string }

func (f fakeWorder) WorldDateWords(_ context.Context, _, _ string, y, m, d int) string {
	return f.words
}

func eventCtx(method, target string, role campaigns.Role, params ...string) (*echo.Context, *httptest.ResponseRecorder) {
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(httptest.NewRequest(method, target, nil), rec)
	pv := echo.PathValues{{Name: "id", Value: "camp-1"}}
	for i := 0; i+1 < len(params); i += 2 {
		pv = append(pv, echo.PathValue{Name: params[i], Value: params[i+1]})
	}
	c.SetPathValues(pv)
	c.Set("campaign_context", &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1"}, MemberRole: role})
	return c, rec
}

// A night with no real date says so in every list row, with its in-world day,
// and a night with a real date is worded as before.
