package ai_workspace

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/connector"
)

// reqRepo is just enough of the connector's repository for its requests.
type reqRepo struct {
	mu   sync.Mutex
	reqs []*connector.Request
}

func (r *reqRepo) Replace(context.Context, *connector.Connection) (int64, error) { return 1, nil }
func (r *reqRepo) RevokeAll(context.Context, string, time.Time) error            { return nil }
func (r *reqRepo) Current(context.Context, string) (*connector.Connection, error) {
	return nil, nil
}
func (r *reqRepo) FindByHash(context.Context, string) (*connector.Connection, error) {
	return nil, nil
}
func (r *reqRepo) MarkUsed(context.Context, int64, time.Time, string) error { return nil }
func (r *reqRepo) AddCall(context.Context, *connector.Call) error           { return nil }
func (r *reqRepo) RecentCalls(context.Context, string, int) ([]connector.Call, error) {
	return nil, nil
}
func (r *reqRepo) PruneCalls(context.Context, string, int) error    { return nil }
func (r *reqRepo) PruneRequests(context.Context, string, int) error { return nil }

func (r *reqRepo) AddRequest(_ context.Context, q *connector.Request, _ int) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := *q
	cp.ID = int64(len(r.reqs) + 1)
	r.reqs = append(r.reqs, &cp)
	return cp.ID, nil
}

func (r *reqRepo) Requests(_ context.Context, campaignID, status string, _ int) ([]connector.Request, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []connector.Request
	for i := len(r.reqs) - 1; i >= 0; i-- {
		if q := r.reqs[i]; q.CampaignID == campaignID && q.Status == status {
			out = append(out, *q)
		}
	}
	return out, nil
}

func (r *reqRepo) Request(_ context.Context, campaignID string, id int64) (*connector.Request, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, q := range r.reqs {
		if q.ID == id && q.CampaignID == campaignID {
			cp := *q
			return &cp, nil
		}
	}
	return nil, nil
}

func (r *reqRepo) FinishRequest(_ context.Context, campaignID string, id int64, status, outcome string, _ time.Time) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, q := range r.reqs {
		if q.ID == id && q.CampaignID == campaignID && q.Status == connector.RequestWaiting {
			q.Status, q.Outcome = status, outcome
			return true, nil
		}
	}
	return false, nil
}

type anyOwner struct{}

func (anyOwner) Check(context.Context, string, string) (string, error) { return "Ashfall", nil }

const askedTable = "---\nkind: table\nname: Tavern names\n---\n- The Rusty Anchor\n- The Drowned Rat\n"

// askedPage is a page whose text tries to be markup; the review shows it
// as text.
const askedPage = "---\nname: Coral Court\n---\n\nA market. <script>alert(1)</script>\n"

// requestRig is a handler whose campaign has one waiting request (a new
// rolling table) from a connected AI, and one in another campaign.
func requestRig(t *testing.T) (*Handler, *reqRepo, *memTables, int64) {
	t.Helper()
	m := &memTables{}
	h := recordsHandler(m)
	repo := &reqRepo{}
	svc := connector.NewService(repo, anyOwner{})
	h.SetConnector(svc, "https://c.example")
	conn := &connector.Connection{ID: 1, CampaignID: "camp-1", CanAsk: true, ClientName: "Claude"}
	q, err := svc.Ask(context.Background(), conn, "Names <b>for</b> taverns", askedTable)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Ask(context.Background(), &connector.Connection{ID: 2, CampaignID: "camp-2", CanAsk: true}, "theirs", askedTable); err != nil {
		t.Fatal(err)
	}
	return h, repo, m, q.ID
}

func post(t *testing.T, h func(echo.Context) error, path string, form url.Values, params ...string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	ownerCtx(c)
	if len(params) == 2 {
		c.SetParamNames("id", params[0])
		c.SetParamValues("camp-1", params[1])
	}
	if err := h(c); err != nil {
		rec.Code = -1
		rec.Body.WriteString(err.Error())
	}
	return rec
}

// Review opens the request's own blocks in the review screen under the
// requests, and empties the paste box's review; Apply then applies only
// what the owner said yes to and marks the request answered.
func TestRequest_ReviewThenApply(t *testing.T) {
	h, repo, m, id := requestRig(t)
	rec := post(t, h.ParseImport, "/campaigns/camp-1/ai-workspace/import/parse", url.Values{"ai_request": {"1"}})
	html := rec.Body.String()
	if rec.Code != http.StatusOK {
		t.Fatalf("review: %d %s", rec.Code, html)
	}
	for _, want := range []string{
		`name="ai_request" value="1"`, `hx-target="#ai-request-review-host"`, `name="rec_0_include"`,
		`id="ai-import-review-host" hx-swap-oob="innerHTML"`, "Tavern names", "Not now",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("review missing %q", want)
		}
	}
	if len(m.doc.Tables) != 0 {
		t.Fatal("reviewing applied something")
	}

	rec = post(t, h.CommitImport, "/campaigns/camp-1/ai-workspace/import/commit", url.Values{
		"markdown_source": {askedTable}, "rec_0_include": {"on"}, "ai_request": {"1"},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("apply: %d %s", rec.Code, rec.Body)
	}
	if len(m.doc.Tables) != 1 || m.doc.Tables[0].Name != "Tavern names" {
		t.Errorf("tables = %+v", m.doc.Tables)
	}
	if q := repo.reqs[id-1]; q.Status != connector.RequestAnswered || q.Outcome != "1 applied, 0 left out" {
		t.Errorf("request = %+v", q)
	}
	if !strings.Contains(rec.Body.String(), `id="ai-requests"`) || !strings.Contains(rec.Body.String(), `hx-swap-oob="true"`) {
		t.Error("the requests list wasn't refreshed")
	}
	// Answered, so it can't be reviewed again.
	if rec := post(t, h.ParseImport, "/campaigns/camp-1/ai-workspace/import/parse", url.Values{"ai_request": {"1"}}); rec.Code == http.StatusOK {
		t.Error("an answered request opened again")
	}
}

// A request's page text is one fold away in its review, escaped; a paste's
// isn't (the owner already has it).
func TestRequest_ReviewShowsText(t *testing.T) {
	h, _, _, _ := requestRig(t)
	svc := h.connector
	q, err := svc.Ask(context.Background(), &connector.Connection{ID: 1, CampaignID: "camp-1", CanAsk: true}, "page", askedPage)
	if err != nil {
		t.Fatal(err)
	}
	html := post(t, h.ParseImport, "/x", url.Values{"ai_request": {strconv.FormatInt(q.ID, 10)}}).Body.String()
	if !strings.Contains(html, "Read what it writes") || !strings.Contains(html, "&lt;script&gt;alert(1)&lt;/script&gt;") || strings.Contains(html, "<script>alert(1)") {
		t.Errorf("request text not shown escaped:\n%s", html)
	}
	if html := post(t, h.ParseImport, "/x", url.Values{"markdown_paste": {askedPage}}).Body.String(); strings.Contains(html, "Read what it writes") {
		t.Error("a paste's review shows the text fold")
	}
}

// Only the campaign's own waiting requests open; the paste box's text, when
// there is some, is reviewed instead.
func TestRequest_ReviewRefusals(t *testing.T) {
	h, _, _, _ := requestRig(t)
	for _, id := range []string{"2", "99", "x", "-1"} {
		if rec := post(t, h.ParseImport, "/x", url.Values{"ai_request": {id}}); rec.Code == http.StatusOK {
			t.Errorf("request %q opened: %s", id, rec.Body)
		}
	}
	rec := post(t, h.ParseImport, "/x", url.Values{"ai_request": {"1"}, "markdown_paste": {"---\nkind: table\nname: Pasted\n---\n- A\n"}})
	if !strings.Contains(rec.Body.String(), "Pasted") || strings.Contains(rec.Body.String(), `name="ai_request"`) {
		t.Errorf("the paste wasn't what was reviewed: %s", rec.Body)
	}
}

func TestRequest_TurnDownAndList(t *testing.T) {
	h, repo, m, _ := requestRig(t)
	get := func(seen string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/campaigns/camp-1/ai/requests?seen="+seen, nil)
		rec := httptest.NewRecorder()
		c := echo.New().NewContext(req, rec)
		ownerCtx(c)
		if err := h.AIRequests(c); err != nil {
			t.Fatal(err)
		}
		return rec
	}
	if rec := get("1-1"); rec.Code != http.StatusNoContent {
		t.Errorf("nothing new: %d", rec.Code)
	}
	rec := get("0")
	html := rec.Body.String()
	for _, want := range []string{"Claude asks for 1 change", "“Names &lt;b&gt;for&lt;/b&gt; taverns”", "1 to add", "Nothing has changed yet", "/campaigns/camp-1/ai/requests/1/turn-down"} {
		if !strings.Contains(html, want) {
			t.Errorf("list missing %q", want)
		}
	}
	if strings.Contains(html, "theirs") {
		t.Error("listed another campaign's request")
	}
	if rec := post(t, h.TurnDownRequest, "/x", nil, "rid", "2"); rec.Code != http.StatusOK || repo.reqs[1].Status != connector.RequestWaiting {
		t.Errorf("turned down another campaign's request: %d %+v", rec.Code, repo.reqs[1])
	}
	rec = post(t, h.TurnDownRequest, "/x", nil, "rid", "1")
	if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), "asks for") {
		t.Errorf("after turning down: %d %s", rec.Code, rec.Body)
	}
	if repo.reqs[0].Status != connector.RequestTurnedDown || len(m.doc.Tables) != 0 {
		t.Errorf("request = %+v, tables = %+v", repo.reqs[0], m.doc.Tables)
	}
}

// The Change choice is part of making a key, defaulting to Ask as signed.
func TestAIPage_ChangeChoice(t *testing.T) {
	html := renderConnect(t, ConnectView{CampaignID: "camp-1", Available: true, Now: time.Now()})
	for _, want := range []string{`name="change" value="ask" checked`, `name="change" value="look"`, "ChatGPT Plus and Pro"} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %q", want)
		}
	}
	h := recordsHandler(&memTables{})
	h.SetConnector(connector.NewService(&reqRepo{}, anyOwner{}), "https://c.example")
	if rec := post(t, h.ConnectAI, "/x", url.Values{"sees": {"players"}}); rec.Code == http.StatusOK {
		t.Error("made a key with no Change choice")
	}
}

// Disconnecting stops new requests but keeps the waiting ones, which are the
// owner's to answer; the confirm says so only when there are some.
func TestRequest_StayAfterDisconnect(t *testing.T) {
	h, _, _, _ := requestRig(t)
	rec := post(t, h.DisconnectAI, "/x", nil)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Claude asks for 1 change") {
		t.Errorf("after disconnect: %d %s", rec.Code, rec.Body)
	}
	const stays = "What it already asked for stays here"
	v := ConnectView{CampaignID: "camp-1", Available: true, Now: time.Now(),
		Conn: &connector.Connection{CampaignID: "camp-1", CreatedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour)}}
	if strings.Contains(renderConnect(t, v), stays) {
		t.Error("confirm mentions requests when none wait")
	}
	v.Requests = []connector.Request{{ID: 1, Blocks: 1, Counts: "1 to add", CreatedAt: time.Now()}}
	if !strings.Contains(renderConnect(t, v), stays) {
		t.Error("confirm doesn't say waiting requests stay")
	}
}
