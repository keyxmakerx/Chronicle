package armory

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

type fakeMembers struct {
	roles   map[string]int
	granted map[string]bool
}

func (m fakeMembers) MemberRole(_ context.Context, _, uid string) (int, bool, error) {
	r, ok := m.roles[uid]
	return r, ok, nil
}
func (m fakeMembers) IsDmGranted(_ context.Context, _, uid string) (bool, error) {
	return m.granted[uid], nil
}

type recordedEvent struct {
	typ, campaign, resource string
	payload                 map[string]any
}

type fakeEvents struct{ got []recordedEvent }

func (f *fakeEvents) PublishStashEvent(typ, campaign, resource string, payload map[string]any) {
	f.got = append(f.got, recordedEvent{typ, campaign, resource, payload})
}

func (f *fakeEvents) types() []string {
	var out []string
	for _, e := range f.got {
		out = append(out, e.typ)
	}
	return out
}

// newAPIFx wires the shared stash fixture to an API facade and an event sink.
// u1 owns c1 (a player), u2 owns c2 (a player), gm is the Owner (the key
// holder), sc is a Scribe, co is a player holding a co-DM grant.
func newAPIFx() (*fx, *StashAPI, *fakeEvents) {
	f := newFx()
	ev := &fakeEvents{}
	f.svc = NewStashService(StashDeps{Repo: f.repo, Directory: f.dir, Visibility: f.vis, Actor: fakeActor{f.dir},
		Fields: f.fields, Relations: f.rels, Events: ev})
	api := NewStashAPI(f.svc, fakeMembers{
		roles:   map[string]int{"gm": rOwner, "sc": rScribe, "u1": rPlayer, "u2": rPlayer, "co": rPlayer},
		granted: map[string]bool{"co": true},
	})
	return f, api, ev
}

func TestActorFor(t *testing.T) {
	_, api, _ := newAPIFx()
	tests := []struct {
		name     string
		key      string
		acting   string
		wantUser string
		wantRole int
		wantCode int
	}{
		{"absent acts as the key holder (Owner)", "gm", "", "gm", rOwner, 0},
		{"named player gets the player role, not the key's", "gm", "u1", "u1", rPlayer, 0},
		{"named scribe", "gm", "sc", "sc", rScribe, 0},
		{"co-DM grant promotes to Owner visibility", "gm", "co", "co", rOwner, 0},
		{"non-member is not found", "gm", "stranger", "", 0, http.StatusNotFound},
		{"former member key holder is not found", "ghost", "", "", 0, http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, err := api.ActorFor(context.Background(), "camp", tt.key, tt.acting)
			if code(err) != tt.wantCode {
				t.Fatalf("err = %v, want code %d", err, tt.wantCode)
			}
			if tt.wantCode == 0 && (a.UserID != tt.wantUser || a.Role != tt.wantRole) {
				t.Fatalf("actor = %+v, want %s/%d", a, tt.wantUser, tt.wantRole)
			}
		})
	}
}

func TestStashAPI_Move(t *testing.T) {
	item := func(acting string, from, to Endpoint) APIMoveRequest {
		return APIMoveRequest{ActingUserID: acting, Kind: MoveKindItem, ItemID: "i1", Quantity: 1,
			From: APIEndpoint(from), To: APIEndpoint(to)}
	}
	tests := []struct {
		name       string
		downtime   bool
		req        func(sid int) APIMoveRequest
		wantStatus string
		wantCode   int
		wantHeld   int
	}{
		{"player outside downtime becomes a request", false,
			func(sid int) APIMoveRequest { return item("u1", charEP("c1"), StashEndpoint(sid)) }, MovePending, 0, 3},
		{"player during downtime applies", true,
			func(sid int) APIMoveRequest { return item("u1", charEP("c1"), StashEndpoint(sid)) }, MoveApplied, 0, 2},
		{"absent actingUserId acts as Owner and applies", false,
			func(sid int) APIMoveRequest { return item("", charEP("c1"), StashEndpoint(sid)) }, MoveApplied, 0, 2},
		{"player cannot move from someone else's character", true,
			func(sid int) APIMoveRequest { return item("u2", charEP("c1"), StashEndpoint(sid)) }, "", http.StatusForbidden, 3},
		{"non-member actingUserId is not found", false,
			func(sid int) APIMoveRequest { return item("stranger", charEP("c1"), StashEndpoint(sid)) }, "", http.StatusNotFound, 3},
		{"player cannot reach a stash they do not see", true,
			func(sid int) APIMoveRequest { return item("u2", charEP("c2"), StashEndpoint(sid)) }, "", http.StatusNotFound, 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, api, _ := newAPIFx()
			f.repo.downtime = tt.downtime
			sid := f.stash(t, "Chest", "c1")
			res, err := api.Move(context.Background(), "camp", "gm", tt.req(sid))
			if code(err) != tt.wantCode {
				t.Fatalf("err = %v, want code %d", err, tt.wantCode)
			}
			if tt.wantCode == 0 && res.Status != tt.wantStatus {
				t.Fatalf("status = %q, want %q", res.Status, tt.wantStatus)
			}
			if got := f.carried("c1", "i1"); got != tt.wantHeld {
				t.Fatalf("c1 holds %d, want %d", got, tt.wantHeld)
			}
		})
	}
}

func TestStashAPI_ApprovalNeedsAnApprover(t *testing.T) {
	f, api, _ := newAPIFx()
	sid := f.stash(t, "Chest", "c1")
	ctx := context.Background()
	res, err := api.Move(ctx, "camp", "gm", APIMoveRequest{ActingUserID: "u1", Kind: MoveKindItem, ItemID: "i1", Quantity: 1,
		From: APIEndpoint{Kind: EndpointCharacter, ID: "c1"}, To: APIEndpoint{Kind: EndpointStash, ID: StashEndpoint(sid).ID}})
	if err != nil || res.Status != MovePending {
		t.Fatalf("setup: %v %+v", err, res)
	}
	id := res.Move.ID

	// A module relaying a player cannot approve, decline, list or flip downtime,
	// even though the key behind it is the Owner's.
	for name, call := range map[string]func() error{
		"approve":  func() error { _, e := api.Approve(ctx, "camp", "gm", "u1", id); return e },
		"decline":  func() error { _, e := api.Decline(ctx, "camp", "gm", "u1", id); return e },
		"requests": func() error { _, e := api.Requests(ctx, "camp", "gm", "u1"); return e },
		"downtime": func() error { _, e := api.SetDowntime(ctx, "camp", "gm", "u1", true); return e },
		"scribe":   func() error { _, e := api.Approve(ctx, "camp", "gm", "sc", id); return e },
	} {
		if c := code(call()); c != http.StatusForbidden {
			t.Errorf("%s as a non-approver: code %d, want 403", name, c)
		}
	}
	if f.repo.moves[0].Status != MovePending || f.repo.downtime {
		t.Fatal("a refused call changed state")
	}

	reqs, err := api.Requests(ctx, "camp", "gm", "")
	if err != nil || len(reqs.Requests) != 1 || reqs.Requests[0].RequesterName == "" {
		t.Fatalf("requests: %v %+v", err, reqs)
	}
	done, err := api.Approve(ctx, "camp", "gm", "", id)
	if err != nil || done.Status != MoveApplied {
		t.Fatalf("approve: %v %+v", err, done)
	}
}

func TestStashAPI_ViewRedactsAsTheActor(t *testing.T) {
	f, api, _ := newAPIFx()
	f.stash(t, "Party chest", "c1")
	f.stash(t, "Mira's cache", "c2")
	f.stash(t, "GM only")
	ctx := context.Background()

	pl, err := api.View(ctx, "camp", "gm", "u1", "c1")
	if err != nil {
		t.Fatal(err)
	}
	if len(pl.Stashes) != 1 || pl.Stashes[0].Name != "Party chest" || pl.CanApprove {
		t.Fatalf("player view stashes=%+v canApprove=%v", pl.Stashes, pl.CanApprove)
	}
	if pl.Character.ID != "c1" || pl.Character.MoneyKey != "gp" || pl.Character.Money != 50 || len(pl.Character.Items) != 1 {
		t.Fatalf("character = %+v", pl.Character)
	}
	if pl.Stashes[0].ID == "" || len(pl.Destinations) == 0 {
		t.Fatalf("missing ids/destinations: %+v", pl)
	}

	if _, err := api.View(ctx, "camp", "gm", "u1", "c2"); code(err) != http.StatusNotFound {
		t.Fatalf("player viewing another's character: %v", err)
	}
	if _, err := api.View(ctx, "camp", "gm", "stranger", "c1"); code(err) != http.StatusNotFound {
		t.Fatalf("non-member: %v", err)
	}

	gm, err := api.View(ctx, "camp", "gm", "", "c1")
	if err != nil || len(gm.Stashes) != 3 || !gm.CanApprove {
		t.Fatalf("owner view: %v stashes=%d canApprove=%v", err, len(gm.Stashes), gm.CanApprove)
	}
	if _, err := json.Marshal(gm); err != nil {
		t.Fatal(err)
	}
}

func TestStashAPI_History(t *testing.T) {
	f, api, _ := newAPIFx()
	sid := f.stash(t, "Chest", "c1")
	ctx := context.Background()
	if _, err := api.Move(ctx, "camp", "gm", APIMoveRequest{Kind: MoveKindItem, ItemID: "i1", Quantity: 1,
		From: APIEndpoint{Kind: EndpointCharacter, ID: "c1"}, To: APIEndpoint{Kind: EndpointStash, ID: StashEndpoint(sid).ID}}); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name               string
		acting, char, stsh string
		wantCode, wantRows int
	}{
		{"owner of the character", "u1", "c1", "", 0, 1},
		{"stash viewer", "u1", "", StashEndpoint(sid).ID, 0, 1},
		{"other player cannot see the character", "u2", "c1", "", http.StatusNotFound, 0},
		{"other player cannot see the stash", "u2", "", StashEndpoint(sid).ID, http.StatusNotFound, 0},
		{"needs exactly one subject", "u1", "", "", http.StatusBadRequest, 0},
		{"both subjects refused", "u1", "c1", "1", http.StatusBadRequest, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, err := api.History(ctx, "camp", "gm", tt.acting, tt.char, tt.stsh)
			if code(err) != tt.wantCode {
				t.Fatalf("err = %v, want %d", err, tt.wantCode)
			}
			if tt.wantCode == 0 && len(h.History) != tt.wantRows {
				t.Fatalf("rows = %d", len(h.History))
			}
		})
	}
}

func TestStashEvents_IdsOnly(t *testing.T) {
	f, api, ev := newAPIFx()
	sid := f.stash(t, "Secret Hoard", "c1")
	ctx := context.Background()
	ref := APIEndpoint{Kind: EndpointStash, ID: StashEndpoint(sid).ID}
	char := APIEndpoint{Kind: EndpointCharacter, ID: "c1"}

	// Request, then approval, then an immediate move, then downtime.
	req, err := api.Move(ctx, "camp", "gm", APIMoveRequest{ActingUserID: "u1", Kind: MoveKindItem, ItemID: "i1", Quantity: 2, From: char, To: ref})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := api.Approve(ctx, "camp", "gm", "", req.Move.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := api.Move(ctx, "camp", "gm", APIMoveRequest{Kind: MoveKindMoney, Amount: "5", From: char, To: ref}); err != nil {
		t.Fatal(err)
	}
	if _, err := api.SetDowntime(ctx, "camp", "gm", "", true); err != nil {
		t.Fatal(err)
	}

	want := []string{EventStashRequested, EventStashMoved, EventStashSettled, EventStashMoved, EventDowntimeChanged}
	if got := ev.types(); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("event order = %v, want %v", got, want)
	}

	b, _ := json.Marshal(ev.got)
	for _, name := range []string{"Thorin", "Secret Hoard", "Potion", "Mira"} {
		if strings.Contains(string(b), name) {
			t.Errorf("event payloads leak the name %q: %s", name, b)
		}
	}

	moved := ev.got[1]
	if moved.payload["status"] != MoveApplied || moved.resource == "" || moved.campaign != "camp" {
		t.Errorf("moved = %+v", moved)
	}
	if ids, _ := moved.payload["characterIds"].([]string); len(ids) != 1 || ids[0] != "c1" {
		t.Errorf("characterIds = %v", moved.payload["characterIds"])
	}
	if ids, _ := moved.payload["stashIds"].([]string); len(ids) != 1 || ids[0] != ref.ID {
		t.Errorf("stashIds = %v", moved.payload["stashIds"])
	}
	if got := ev.got[0].payload["requestedBy"]; got != "u1" {
		t.Errorf("requestedBy = %v", got)
	}
	if got := ev.got[2].payload["decidedBy"]; got != "gm" {
		t.Errorf("decidedBy = %v", got)
	}
	if got := ev.got[4].payload["open"]; got != true {
		t.Errorf("downtime open = %v", got)
	}
}

func TestStashEvents_NoneOnRefusalOrDecline(t *testing.T) {
	f, api, ev := newAPIFx()
	sid := f.stash(t, "Chest", "c1")
	ctx := context.Background()
	from := APIEndpoint{Kind: EndpointCharacter, ID: "c1"}
	to := APIEndpoint{Kind: EndpointStash, ID: StashEndpoint(sid).ID}

	if _, err := api.Move(ctx, "camp", "gm", APIMoveRequest{ActingUserID: "u2", Kind: MoveKindItem, ItemID: "i1", Quantity: 1, From: from, To: to}); err == nil {
		t.Fatal("expected refusal")
	}
	if len(ev.got) != 0 {
		t.Fatalf("a refused move published %v", ev.types())
	}

	res, _ := api.Move(ctx, "camp", "gm", APIMoveRequest{ActingUserID: "u1", Kind: MoveKindItem, ItemID: "i1", Quantity: 1, From: from, To: to})
	ev.got = nil
	if _, err := api.Decline(ctx, "camp", "gm", "", res.Move.ID); err != nil {
		t.Fatal(err)
	}
	if got := ev.types(); len(got) != 1 || got[0] != EventStashSettled || ev.got[0].payload["status"] != MoveDeclined {
		t.Fatalf("decline events = %v", got)
	}
}
