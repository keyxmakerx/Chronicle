package ai_workspace

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/aiexport"
	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/connector"
)

// reqRepo is just enough of the connector's repository for its requests and
// one connection.
type reqRepo struct {
	mu   sync.Mutex
	reqs []*connector.Request
	conn *connector.Connection
}

func (r *reqRepo) Replace(context.Context, *connector.Connection) (int64, error) { return 1, nil }
func (r *reqRepo) RevokeAll(context.Context, string, time.Time) error            { return nil }
func (r *reqRepo) Current(_ context.Context, campaignID string) (*connector.Connection, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.conn == nil || r.conn.CampaignID != campaignID {
		return nil, nil
	}
	cp := *r.conn
	return &cp, nil
}
func (r *reqRepo) SetPaused(_ context.Context, _ string, id int64, at *time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.conn != nil && r.conn.ID == id {
		r.conn.PausedAt = at
	}
	return nil
}
func (r *reqRepo) SetAccess(_ context.Context, _ string, id int64, privacy aiexport.PrivacyMode, canAsk bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.conn != nil && r.conn.ID == id {
		r.conn.Privacy, r.conn.CanAsk = privacy, canAsk
	}
	return nil
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
		`name="ai_request" value="1"`, `hx-target="#ai-request-1-review"`, `name="rec_0_include"`,
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
	// The card goes with the answered request, so the result lands above
	// the list.
	if got := rec.Header().Get("HX-Retarget"); got != "#ai-request-result" || !strings.Contains(rec.Body.String(), "Made 1 of 1 change.") {
		t.Errorf("result retargeted to %q: %s", got, rec.Body)
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

// auditLog keeps the events the page logs.
type auditLog struct{ events []string }

func (a *auditLog) LogCampaignEvent(_ context.Context, _ string, action string, details map[string]any) {
	a.events = append(a.events, action+" "+fmtDetails(details))
}

func fmtDetails(d map[string]any) string {
	keys := make([]string, 0, len(d))
	for k := range d {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		fmt.Fprintf(&b, "%s=%v ", k, d[k])
	}
	return strings.TrimSpace(b.String())
}

// Pause, Resume and Change act on the key the page shows, re-render the
// section in its new state, and are logged; letting the AI see what the
// owner sees needs the page's confirmation.
func TestConnection_PauseAndChange(t *testing.T) {
	future := time.Now().Add(time.Hour)
	used := time.Now()
	repo := &reqRepo{conn: &connector.Connection{ID: 7, CampaignID: "camp-1", ClientName: "Claude", CanAsk: true,
		Privacy: aiexport.PrivacyModeSafe, CreatedAt: used, ExpiresAt: future, LastUsedAt: &used}}
	h := recordsHandler(&memTables{})
	log := &auditLog{}
	h.SetAuditLogger(log)
	h.SetConnector(connector.NewService(repo, anyOwner{}), "https://c.example")

	if rec := post(t, h.PauseAI, "/x", url.Values{"connection": {"7"}}); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Claude is paused") {
		t.Fatalf("pause: %d %s", rec.Code, rec.Body)
	}
	if repo.conn.PausedAt == nil {
		t.Fatal("not paused")
	}
	if rec := post(t, h.ResumeAI, "/x", url.Values{"connection": {"7"}}); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Claude is connected") {
		t.Fatalf("resume: %d %s", rec.Code, rec.Body)
	}
	if rec := post(t, h.PauseAI, "/x", url.Values{"connection": {"6"}}); rec.Code == http.StatusOK || repo.conn.PausedAt != nil {
		t.Errorf("paused a key the page doesn't show: %d", rec.Code)
	}

	widen := url.Values{"connection": {"7"}, "sees": {"owner"}, "change": {"look"}}
	if rec := post(t, h.ChangeAIAccess, "/x", widen); rec.Code == http.StatusOK || repo.conn.Privacy != aiexport.PrivacyModeSafe {
		t.Errorf("widened without the confirmation: %d", rec.Code)
	}
	widen.Set("widen", "yes")
	if rec := post(t, h.ChangeAIAccess, "/x", widen); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "sees what you see") {
		t.Fatalf("change: %d %s", rec.Code, rec.Body)
	}
	if repo.conn.Privacy != aiexport.PrivacyModePermitted || repo.conn.CanAsk {
		t.Errorf("after change: %+v", repo.conn)
	}
	// Narrowing never needs the confirmation.
	if rec := post(t, h.ChangeAIAccess, "/x", url.Values{"connection": {"7"}, "sees": {"players"}, "change": {"ask"}}); rec.Code != http.StatusOK {
		t.Errorf("narrow: %d %s", rec.Code, rec.Body)
	}
	want := []string{
		"campaign.ai_connection.paused connection_id=7",
		"campaign.ai_connection.resumed connection_id=7",
		"campaign.ai_connection.changed can_ask_from=true can_ask_to=false connection_id=7 privacy_from=safe privacy_to=permitted",
		"campaign.ai_connection.changed can_ask_from=false can_ask_to=true connection_id=7 privacy_from=permitted privacy_to=safe",
	}
	if strings.Join(log.events, "\n") != strings.Join(want, "\n") {
		t.Errorf("audit:\n%s\nwant:\n%s", strings.Join(log.events, "\n"), strings.Join(want, "\n"))
	}
}

// "What it did" redraws only when something new came in, so the sittings the
// owner opened stay open; Show older asks for more.
func TestAIActivity_RedrawsOnlyWhenNew(t *testing.T) {
	h := recordsHandler(&memTables{})
	h.SetConnector(connector.NewService(&reqRepo{}, anyOwner{}), "https://c.example")
	get := func(query string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/campaigns/camp-1/ai/activity?"+query, nil)
		rec := httptest.NewRecorder()
		c := echo.New().NewContext(req, rec)
		ownerCtx(c)
		if err := h.AIActivity(c); err != nil {
			t.Fatal(err)
		}
		return rec
	}
	if rec := get("shown=8&seen=0-8"); rec.Code != http.StatusNoContent {
		t.Errorf("nothing new: %d", rec.Code)
	}
	if rec := get("shown=8&seen=5-8"); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "seen=0-8") {
		t.Errorf("changed: %d %s", rec.Code, rec.Body)
	}
	// Asking for more than there are is cut to what exists, plus one step.
	if rec := get("shown=999"); !strings.Contains(rec.Body.String(), "shown=8&amp;seen=0-8") {
		t.Errorf("shown not clamped: %s", rec.Body)
	}
}
