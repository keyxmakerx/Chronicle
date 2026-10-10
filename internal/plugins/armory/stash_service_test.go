package armory

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/patch"
)

// --- fakes ---

type fakeStashRepo struct {
	nextStash  int
	nextMove   int64
	stashes    map[int]*Stash
	viewers    map[int][]string
	items      map[int]map[string]int
	moves      []*Move
	downtime   bool
	failInsert bool
	failCredit bool // makes every credit fail, to exercise the restore path
}

func newFakeRepo() *fakeStashRepo {
	return &fakeStashRepo{stashes: map[int]*Stash{}, viewers: map[int][]string{}, items: map[int]map[string]int{}}
}

func (r *fakeStashRepo) CreateStash(_ context.Context, s *Stash) error {
	r.nextStash++
	s.ID = r.nextStash
	c := *s
	r.stashes[s.ID] = &c
	return nil
}
func (r *fakeStashRepo) GetStash(_ context.Context, campaignID string, id int) (*Stash, error) {
	s, ok := r.stashes[id]
	if !ok || s.CampaignID != campaignID {
		return nil, apperror.NewNotFound("stash")
	}
	c := *s
	return &c, nil
}
func (r *fakeStashRepo) ListStashes(_ context.Context, campaignID string) ([]Stash, error) {
	var out []Stash
	for _, s := range r.stashes {
		if s.CampaignID == campaignID {
			out = append(out, *s)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}
func (r *fakeStashRepo) UpdateStash(_ context.Context, s *Stash) error {
	c := *s
	r.stashes[s.ID] = &c
	return nil
}
func (r *fakeStashRepo) DeleteStash(_ context.Context, campaignID string, id int) error {
	if s, ok := r.stashes[id]; !ok || s.CampaignID != campaignID {
		return apperror.NewNotFound("stash")
	}
	delete(r.stashes, id)
	return nil
}
func (r *fakeStashRepo) SetViewers(_ context.Context, _ string, id int, ids []string) error {
	r.viewers[id] = ids
	return nil
}
func (r *fakeStashRepo) ListViewers(_ context.Context, _ string) (map[int][]string, error) {
	return r.viewers, nil
}
func (r *fakeStashRepo) ListItems(_ context.Context, _ string) (map[int][]StashItemRow, error) {
	out := map[int][]StashItemRow{}
	for sid, m := range r.items {
		for id, q := range m {
			out[sid] = append(out[sid], StashItemRow{ItemEntityID: id, Quantity: q})
		}
	}
	return out, nil
}
func (r *fakeStashRepo) CreditItem(_ context.Context, campaignID string, sid int, id string, q int) error {
	if r.failCredit {
		return errors.New("boom")
	}
	if s, ok := r.stashes[sid]; !ok || s.CampaignID != campaignID {
		return apperror.NewNotFound("stash")
	}
	if r.items[sid] == nil {
		r.items[sid] = map[string]int{}
	}
	r.items[sid][id] += q
	return nil
}
func (r *fakeStashRepo) DebitItem(_ context.Context, _ string, sid int, id string, q int) (bool, error) {
	if r.items[sid][id] < q {
		return false, nil
	}
	r.items[sid][id] -= q
	if r.items[sid][id] == 0 {
		delete(r.items[sid], id)
	}
	return true, nil
}
func (r *fakeStashRepo) CreditMoney(_ context.Context, _ string, sid int, a Cents) error {
	if r.failCredit {
		return errors.New("boom")
	}
	r.stashes[sid].Money += a
	return nil
}
func (r *fakeStashRepo) DebitMoney(_ context.Context, _ string, sid int, a Cents) (bool, error) {
	if r.stashes[sid].Money < a {
		return false, nil
	}
	r.stashes[sid].Money -= a
	return true, nil
}
func (r *fakeStashRepo) InsertMove(_ context.Context, m *Move) error {
	if r.failInsert {
		return errors.New("db down")
	}
	r.nextMove++
	m.ID = r.nextMove
	c := *m
	r.moves = append(r.moves, &c)
	return nil
}
func (r *fakeStashRepo) GetMove(_ context.Context, campaignID string, id int64) (*Move, error) {
	for _, m := range r.moves {
		if m.ID == id && m.CampaignID == campaignID {
			c := *m
			return &c, nil
		}
	}
	return nil, apperror.NewNotFound("request")
}
func (r *fakeStashRepo) SettleMove(_ context.Context, campaignID string, id int64, status, reason, by string) (bool, error) {
	for _, m := range r.moves {
		if m.ID == id && m.CampaignID == campaignID && m.Status == MovePending {
			m.Status, m.Reason, m.DecidedBy = status, reason, by
			return true, nil
		}
	}
	return false, nil
}
func (r *fakeStashRepo) ListMoves(_ context.Context, campaignID string, f MoveFilter) ([]Move, error) {
	var out []Move
	for i := len(r.moves) - 1; i >= 0; i-- {
		m := r.moves[i]
		if m.CampaignID != campaignID {
			continue
		}
		if f.Endpoint != nil && m.From != *f.Endpoint && m.To != *f.Endpoint {
			continue
		}
		if f.RequestedBy != "" && m.RequestedBy != f.RequestedBy {
			continue
		}
		if f.Status != "" && m.Status != f.Status {
			continue
		}
		out = append(out, *m)
	}
	return out, nil
}
func (r *fakeStashRepo) ListPending(_ context.Context, campaignID string) ([]Move, error) {
	var out []Move
	for _, m := range r.moves {
		if m.CampaignID == campaignID && m.Status == MovePending {
			out = append(out, *m)
		}
	}
	return out, nil
}
func (r *fakeStashRepo) GetDowntime(context.Context, string) (*Downtime, error) {
	return &Downtime{IsOpen: r.downtime}, nil
}
func (r *fakeStashRepo) SetDowntime(_ context.Context, _ string, open bool, _ string) error {
	r.downtime = open
	return nil
}

type fakeDir struct {
	ents      map[string]*EntityRef // id -> ref (all in campaign "camp")
	truncated bool
}

func (d *fakeDir) GetEntity(_ context.Context, campaignID, id string) (*EntityRef, error) {
	if campaignID != "camp" {
		return nil, nil
	}
	e, ok := d.ents[id]
	if !ok {
		return nil, nil
	}
	c := *e
	return &c, nil
}
func (d *fakeDir) ListCharacters(context.Context, string, int, string) ([]EntityRef, error) {
	var out []EntityRef
	for _, e := range d.ents {
		if e.IsCharacter {
			out = append(out, *e)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}
func (d *fakeDir) ListCharactersChecked(ctx context.Context, c string, r int, u string) ([]EntityRef, bool, error) {
	out, err := d.ListCharacters(ctx, c, r, u)
	return out, d.truncated, err
}
func (d *fakeDir) ListItems(context.Context, string, int, string, int) ([]EntityRef, error) {
	return nil, nil
}
func (d *fakeDir) OwnedCharacterIDs(_ context.Context, _, userID string) (map[string]bool, error) {
	out := map[string]bool{}
	for id, e := range d.ents {
		if e.OwnerUserID == userID {
			out[id] = true
		}
	}
	return out, nil
}

type fakeVis struct{ hidden map[string]bool }

func (v fakeVis) FilterViewableEntityIDs(_ context.Context, _ string, ids []string, _ int, _ string) (map[string]bool, error) {
	out := map[string]bool{}
	for _, id := range ids {
		out[id] = !v.hidden[id]
	}
	return out, nil
}

// fakeActor mirrors the real rule: Scribe+ or the claimed owner.
type fakeActor struct{ d *fakeDir }

func (f fakeActor) CanUserActAsBuyer(_ context.Context, _, id, userID string, role int) (bool, error) {
	if role >= 2 {
		return true, nil
	}
	e := f.d.ents[id]
	return e != nil && e.OwnerUserID == userID, nil
}

type fakeFields struct {
	data map[string]map[string]any
	// afterUpdate, when set, runs after each write, standing in for a sheet
	// edit that lands between two steps of a purchase.
	afterUpdate func()
}

func (f *fakeFields) GetEntityFields(_ context.Context, id string) (map[string]any, error) {
	out := map[string]any{}
	for k, v := range f.data[id] {
		out[k] = v
	}
	return out, nil
}
func (f *fakeFields) UpdateEntityFields(_ context.Context, id string, p map[string]any) error {
	if f.data[id] == nil {
		f.data[id] = map[string]any{}
	}
	for k, v := range p {
		f.data[id][k] = v
	}
	if f.afterUpdate != nil {
		f.afterUpdate()
	}
	return nil
}

type fakeRels struct {
	next           int
	rels           map[string][]*HasItemRelation // character id -> relations
	fail           bool
	createConflict bool // Create reports a duplicate, like the unique key
}

func (f *fakeRels) ListByCharacter(_ context.Context, _, cid string) ([]HasItemRelation, error) {
	var out []HasItemRelation
	for _, r := range f.rels[cid] {
		out = append(out, *r)
	}
	return out, nil
}
func (f *fakeRels) Create(_ context.Context, _, cid, item, _ string, meta []byte) (int, error) {
	if f.fail {
		return 0, errors.New("boom")
	}
	if f.createConflict {
		return 0, apperror.NewConflict("exists")
	}
	f.next++
	f.rels[cid] = append(f.rels[cid], &HasItemRelation{ID: f.next, ItemEntityID: item, ItemName: item, Metadata: meta})
	return f.next, nil
}
func (f *fakeRels) find(id int) *HasItemRelation {
	for _, l := range f.rels {
		for _, r := range l {
			if r.ID == id {
				return r
			}
		}
	}
	return nil
}
func (f *fakeRels) UpdateMetadata(_ context.Context, id int, m []byte) error {
	f.find(id).Metadata = m
	return nil
}
func (f *fakeRels) UpdateMetadataIf(_ context.Context, id int, exp, m []byte) (bool, error) {
	r := f.find(id)
	if string(r.Metadata) != string(exp) {
		return false, nil
	}
	r.Metadata = m
	return true, nil
}
func (f *fakeRels) Delete(_ context.Context, id int) error {
	for cid, l := range f.rels {
		for i, r := range l {
			if r.ID == id {
				f.rels[cid] = append(l[:i], l[i+1:]...)
				return nil
			}
		}
	}
	return nil
}

type fx struct {
	svc    StashService
	repo   *fakeStashRepo
	dir    *fakeDir
	fields *fakeFields
	rels   *fakeRels
	vis    fakeVis
}

const (
	rPlayer = 1
	rScribe = 2
	rOwner  = 3
)

func newFx() *fx {
	dir := &fakeDir{ents: map[string]*EntityRef{
		"c1": {ID: "c1", Name: "Thorin", IsCharacter: true, OwnerUserID: "u1", MoneyKey: "gp"},
		"c2": {ID: "c2", Name: "Mira", IsCharacter: true, OwnerUserID: "u2", MoneyKey: "gp"},
		"c3": {ID: "c3", Name: "Grub", IsCharacter: true, OwnerUserID: "u3"}, // no money field
		"i1": {ID: "i1", Name: "Potion", IsItem: true},
		"i2": {ID: "i2", Name: "Secret", IsItem: true},
		"n1": {ID: "n1", Name: "Tavern"}, // neither
	}}
	f := &fx{
		repo:   newFakeRepo(),
		dir:    dir,
		fields: &fakeFields{data: map[string]map[string]any{"c1": {"gp": 50.0}, "c2": {"gp": "10"}, "c3": {}}},
		rels:   &fakeRels{rels: map[string][]*HasItemRelation{}},
		vis:    fakeVis{hidden: map[string]bool{}},
	}
	f.rels.rels["c1"] = []*HasItemRelation{{ID: 100, ItemEntityID: "i1", ItemName: "Potion", Metadata: []byte(`{"quantity":3,"equipped":true}`)}}
	f.rels.next = 100
	f.svc = NewStashService(StashDeps{Repo: f.repo, Directory: dir, Visibility: f.vis, Actor: fakeActor{dir}, Fields: f.fields, Relations: f.rels})
	return f
}

func (f *fx) stash(t *testing.T, name string, viewers ...string) int {
	t.Helper()
	st, err := f.svc.CreateStash(context.Background(), "camp", Actor{"gm", rOwner}, CreateStashInput{Name: name})
	if err != nil {
		t.Fatal(err)
	}
	f.repo.viewers[st.ID] = viewers
	return st.ID
}

func code(err error) int {
	var ae *apperror.AppError
	if errors.As(err, &ae) {
		return ae.Code
	}
	return 0
}

func (f *fx) carried(cid, item string) int {
	for _, r := range f.rels.rels[cid] {
		if r.ItemEntityID == item {
			q, _ := parseCarried(r.Metadata)
			return q
		}
	}
	return 0
}

func charEP(id string) Endpoint { return Endpoint{Kind: EndpointCharacter, ID: id} }

// --- tests ---

func TestMove_ImmediateOrRequest(t *testing.T) {
	tests := []struct {
		name      string
		role      int
		downtime  bool
		wantApply bool
	}{
		{"player, downtime closed asks", rPlayer, false, false},
		{"player, downtime open applies", rPlayer, true, true},
		{"scribe always applies", rScribe, false, true},
		{"owner always applies", rOwner, false, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newFx()
			f.repo.downtime = tc.downtime
			sid := f.stash(t, "Chest", "c1")
			out, err := f.svc.Move(context.Background(), "camp", Actor{"u1", tc.role}, MoveInput{
				Kind: MoveKindItem, ItemEntityID: "i1", Quantity: 2, From: charEP("c1"), To: StashEndpoint(sid)})
			if err != nil {
				t.Fatal(err)
			}
			if out.Applied != tc.wantApply {
				t.Fatalf("applied=%v want %v", out.Applied, tc.wantApply)
			}
			wantHeld, wantStash := 3, 0
			if tc.wantApply {
				wantHeld, wantStash = 1, 2
			}
			if f.carried("c1", "i1") != wantHeld || f.repo.items[sid]["i1"] != wantStash {
				t.Fatalf("held=%d stash=%d", f.carried("c1", "i1"), f.repo.items[sid]["i1"])
			}
			wantStatus := MovePending
			if tc.wantApply {
				wantStatus = MoveApplied
			}
			if got := f.repo.moves[0].Status; got != wantStatus {
				t.Fatalf("history status %q want %q", got, wantStatus)
			}
		})
	}
}

func TestMove_PreservesOtherRelationMetadata(t *testing.T) {
	f := newFx()
	sid := f.stash(t, "Chest", "c1")
	if _, err := f.svc.Move(context.Background(), "camp", Actor{"gm", rOwner}, MoveInput{
		Kind: MoveKindItem, ItemEntityID: "i1", Quantity: 1, From: charEP("c1"), To: StashEndpoint(sid)}); err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	_ = json.Unmarshal(f.rels.rels["c1"][0].Metadata, &m)
	if m["equipped"] != true || m["quantity"] != 2.0 {
		t.Fatalf("metadata %v", m)
	}
}

func TestMove_ItemToCharacterCreatesAndIncrements(t *testing.T) {
	f := newFx()
	f.repo.downtime = true
	for i := 0; i < 2; i++ {
		if _, err := f.svc.Move(context.Background(), "camp", Actor{"u1", rPlayer}, MoveInput{
			Kind: MoveKindItem, ItemEntityID: "i1", Quantity: 1, From: charEP("c1"), To: charEP("c2")}); err != nil {
			t.Fatal(err)
		}
	}
	if f.carried("c2", "i1") != 2 || f.carried("c1", "i1") != 1 {
		t.Fatalf("c1=%d c2=%d", f.carried("c1", "i1"), f.carried("c2", "i1"))
	}
}

func TestMove_Rejections(t *testing.T) {
	tests := []struct {
		name string
		who  Actor
		in   MoveInput
		want int
	}{
		{"not own character", Actor{"u2", rPlayer}, MoveInput{Kind: MoveKindItem, ItemEntityID: "i1", Quantity: 1, From: charEP("c1"), To: charEP("c2")}, http.StatusForbidden},
		{"more than held", Actor{"u1", rPlayer}, MoveInput{Kind: MoveKindItem, ItemEntityID: "i1", Quantity: 4, From: charEP("c1"), To: charEP("c2")}, http.StatusBadRequest},
		{"zero quantity", Actor{"u1", rPlayer}, MoveInput{Kind: MoveKindItem, ItemEntityID: "i1", Quantity: 0, From: charEP("c1"), To: charEP("c2")}, http.StatusBadRequest},
		{"money over balance", Actor{"u1", rPlayer}, MoveInput{Kind: MoveKindMoney, Amount: "50.01", From: charEP("c1"), To: charEP("c2")}, http.StatusBadRequest},
		{"money three decimals", Actor{"u1", rPlayer}, MoveInput{Kind: MoveKindMoney, Amount: "1.001", From: charEP("c1"), To: charEP("c2")}, http.StatusBadRequest},
		{"money negative", Actor{"u1", rPlayer}, MoveInput{Kind: MoveKindMoney, Amount: "-1", From: charEP("c1"), To: charEP("c2")}, http.StatusBadRequest},
		{"money zero", Actor{"u1", rPlayer}, MoveInput{Kind: MoveKindMoney, Amount: "0", From: charEP("c1"), To: charEP("c2")}, http.StatusBadRequest},
		{"destination has no money field", Actor{"u1", rPlayer}, MoveInput{Kind: MoveKindMoney, Amount: "1", From: charEP("c1"), To: charEP("c3")}, http.StatusBadRequest},
		{"source has no money field", Actor{"u3", rPlayer}, MoveInput{Kind: MoveKindMoney, Amount: "1", From: charEP("c3"), To: charEP("c1")}, http.StatusBadRequest},
		{"unknown item", Actor{"u1", rPlayer}, MoveInput{Kind: MoveKindItem, ItemEntityID: "nope", Quantity: 1, From: charEP("c1"), To: charEP("c2")}, http.StatusNotFound},
		{"non-item entity", Actor{"u1", rPlayer}, MoveInput{Kind: MoveKindItem, ItemEntityID: "n1", Quantity: 1, From: charEP("c1"), To: charEP("c2")}, http.StatusNotFound},
		{"destination not a character", Actor{"u1", rPlayer}, MoveInput{Kind: MoveKindItem, ItemEntityID: "i1", Quantity: 1, From: charEP("c1"), To: charEP("n1")}, http.StatusNotFound},
		{"unknown stash", Actor{"u1", rPlayer}, MoveInput{Kind: MoveKindItem, ItemEntityID: "i1", Quantity: 1, From: charEP("c1"), To: StashEndpoint(999)}, http.StatusNotFound},
		{"same place", Actor{"u1", rPlayer}, MoveInput{Kind: MoveKindItem, ItemEntityID: "i1", Quantity: 1, From: charEP("c1"), To: charEP("c1")}, http.StatusBadRequest},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newFx()
			f.repo.downtime = true
			_, err := f.svc.Move(context.Background(), "camp", tc.who, tc.in)
			if code(err) != tc.want {
				t.Fatalf("got %v (code %d), want %d", err, code(err), tc.want)
			}
			if f.carried("c1", "i1") != 3 || f.fields.data["c1"]["gp"] != 50.0 || len(f.repo.moves) != 0 {
				t.Fatal("a refused move changed something")
			}
		})
	}
}

func TestMove_ForeignCampaignIsNotFound(t *testing.T) {
	f := newFx()
	f.repo.downtime = true
	_, err := f.svc.Move(context.Background(), "other", Actor{"u1", rPlayer}, MoveInput{
		Kind: MoveKindItem, ItemEntityID: "i1", Quantity: 1, From: charEP("c1"), To: charEP("c2")})
	if code(err) != http.StatusNotFound {
		t.Fatalf("got %v", err)
	}
}

func TestMove_HiddenItemIsNotFoundForPlayer(t *testing.T) {
	f := newFx()
	f.repo.downtime = true
	f.vis.hidden["i1"] = true
	f.svc = NewStashService(StashDeps{Repo: f.repo, Directory: f.dir, Visibility: f.vis, Actor: fakeActor{f.dir}, Fields: f.fields, Relations: f.rels})
	_, err := f.svc.Move(context.Background(), "camp", Actor{"u1", rPlayer}, MoveInput{
		Kind: MoveKindItem, ItemEntityID: "i1", Quantity: 1, From: charEP("c1"), To: charEP("c2")})
	if code(err) != http.StatusNotFound {
		t.Fatalf("got %v", err)
	}
}

func TestMove_MoneyBetweenCharactersAndStash(t *testing.T) {
	f := newFx()
	f.repo.downtime = true
	sid := f.stash(t, "Chest", "c1", "c2")
	ctx := context.Background()
	u1 := Actor{"u1", rPlayer}
	if _, err := f.svc.Move(ctx, "camp", u1, MoveInput{Kind: MoveKindMoney, Amount: "12.50", From: charEP("c1"), To: StashEndpoint(sid)}); err != nil {
		t.Fatal(err)
	}
	if f.fields.data["c1"]["gp"] != 37.5 || f.repo.stashes[sid].Money != 1250 {
		t.Fatalf("c1=%v stash=%v", f.fields.data["c1"]["gp"], f.repo.stashes[sid].Money)
	}
	// String numerals on the sheet are read; the write keeps the field numeric.
	if _, err := f.svc.Move(ctx, "camp", Actor{"u2", rPlayer}, MoveInput{Kind: MoveKindMoney, Amount: "4", From: charEP("c2"), To: StashEndpoint(sid)}); err != nil {
		t.Fatal(err)
	}
	if f.fields.data["c2"]["gp"] != 6.0 {
		t.Fatalf("c2=%v (%T)", f.fields.data["c2"]["gp"], f.fields.data["c2"]["gp"])
	}
	// Taking the stash's money: the stash can't go negative either.
	if _, err := f.svc.Move(ctx, "camp", u1, MoveInput{Kind: MoveKindMoney, Amount: "16.51", From: StashEndpoint(sid), To: charEP("c1")}); code(err) != http.StatusBadRequest {
		t.Fatalf("got %v", err)
	}
}

func TestApproveGate(t *testing.T) {
	tests := []struct {
		name string
		role int
		want int // 0 = allowed
	}{
		{"scribe cannot approve", rScribe, http.StatusForbidden},
		{"player cannot approve", rPlayer, http.StatusForbidden},
		{"owner can", rOwner, 0},
		{"dm-granted co-dm (promoted to owner visibility) can", rOwner, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newFx()
			sid := f.stash(t, "Chest", "c1")
			ctx := context.Background()
			out, err := f.svc.Move(ctx, "camp", Actor{"u1", rPlayer}, MoveInput{Kind: MoveKindItem, ItemEntityID: "i1", Quantity: 1, From: charEP("c1"), To: StashEndpoint(sid)})
			if err != nil || out.Applied {
				t.Fatalf("setup: %v %v", err, out)
			}
			if tc.want != 0 {
				for _, op := range []string{"approve", "decline"} {
					var err error
					if op == "approve" {
						_, err = f.svc.Approve(ctx, "camp", Actor{"x", tc.role}, out.Move.ID)
					} else {
						_, err = f.svc.Decline(ctx, "camp", Actor{"x", tc.role}, out.Move.ID)
					}
					if code(err) != tc.want {
						t.Fatalf("%s: got %v", op, err)
					}
					if f.repo.moves[0].Status != MovePending || f.carried("c1", "i1") != 3 {
						t.Fatal("a refused answer changed something")
					}
				}
			}
			if tc.want == 0 {
				if _, err := f.svc.Approve(ctx, "camp", Actor{"gm", tc.role}, out.Move.ID); err != nil {
					t.Fatal(err)
				}
				if f.repo.moves[0].Status != MoveApplied || f.repo.moves[0].DecidedBy != "gm" || f.carried("c1", "i1") != 2 || f.repo.items[sid]["i1"] != 1 {
					t.Fatalf("approve result %+v", f.repo.moves[0])
				}
				// Answering twice is refused and moves nothing more.
				if _, err := f.svc.Approve(ctx, "camp", Actor{"gm", tc.role}, out.Move.ID); code(err) != http.StatusConflict {
					t.Fatalf("second answer: %v", err)
				}
				if f.carried("c1", "i1") != 2 {
					t.Fatal("double apply")
				}
			}
		})
	}
}

func TestDecline_LeavesEverythingInPlace(t *testing.T) {
	f := newFx()
	sid := f.stash(t, "Chest", "c1")
	ctx := context.Background()
	out, _ := f.svc.Move(ctx, "camp", Actor{"u1", rPlayer}, MoveInput{Kind: MoveKindItem, ItemEntityID: "i1", Quantity: 1, From: charEP("c1"), To: StashEndpoint(sid)})
	if _, err := f.svc.Decline(ctx, "camp", Actor{"gm", rOwner}, out.Move.ID); err != nil {
		t.Fatal(err)
	}
	if f.repo.moves[0].Status != MoveDeclined || f.carried("c1", "i1") != 3 {
		t.Fatalf("%+v", f.repo.moves[0])
	}
}

func TestApprove_ForeignCampaignIsNotFound(t *testing.T) {
	f := newFx()
	sid := f.stash(t, "Chest", "c1")
	out, _ := f.svc.Move(context.Background(), "camp", Actor{"u1", rPlayer}, MoveInput{Kind: MoveKindItem, ItemEntityID: "i1", Quantity: 1, From: charEP("c1"), To: StashEndpoint(sid)})
	_, err := f.svc.Approve(context.Background(), "other", Actor{"gm", rOwner}, out.Move.ID)
	if code(err) != http.StatusNotFound {
		t.Fatalf("got %v", err)
	}
}

func TestDowntimeOpen_AppliesPendingOldestFirst(t *testing.T) {
	f := newFx()
	sid := f.stash(t, "Chest", "c1")
	ctx := context.Background()
	u1 := Actor{"u1", rPlayer}
	mv := func(q int) {
		if _, err := f.svc.Move(ctx, "camp", u1, MoveInput{Kind: MoveKindItem, ItemEntityID: "i1", Quantity: q, From: charEP("c1"), To: StashEndpoint(sid)}); err != nil {
			t.Fatal(err)
		}
	}
	// Each passes the request-time check (3 held), but together they want 5.
	mv(2) // #1 fits
	mv(2) // #2 fits only 1 left -> must fail
	mv(1) // #3 fits
	if _, err := f.svc.SetDowntime(ctx, "camp", Actor{"u1", rPlayer}, true); code(err) != http.StatusForbidden {
		t.Fatalf("player opened downtime: %v", err)
	}
	if _, err := f.svc.SetDowntime(ctx, "camp", Actor{"gm", rScribe}, true); code(err) != http.StatusForbidden {
		t.Fatalf("scribe opened downtime: %v", err)
	}
	res, err := f.svc.SetDowntime(ctx, "camp", Actor{"gm", rOwner}, true)
	if err != nil {
		t.Fatal(err)
	}
	if res.Applied != 2 || res.Failed != 1 {
		t.Fatalf("%+v", res)
	}
	got := []string{f.repo.moves[0].Status, f.repo.moves[1].Status, f.repo.moves[2].Status}
	if got[0] != MoveApplied || got[1] != MoveFailed || got[2] != MoveApplied {
		t.Fatalf("statuses %v", got)
	}
	if f.repo.moves[1].Reason == "" {
		t.Fatal("failed move has no reason")
	}
	if f.carried("c1", "i1") != 0 || f.repo.items[sid]["i1"] != 3 {
		t.Fatalf("held=%d stash=%d (a failed move must not apply in part)", f.carried("c1", "i1"), f.repo.items[sid]["i1"])
	}
	if open, _ := f.svc.IsDowntimeOpen(ctx, "camp"); !open {
		t.Fatal("downtime not open")
	}
	// Closing only flips the switch.
	if _, err := f.svc.SetDowntime(ctx, "camp", Actor{"gm", rOwner}, false); err != nil {
		t.Fatal(err)
	}
	if open, _ := f.svc.IsDowntimeOpen(ctx, "camp"); open {
		t.Fatal("downtime still open")
	}
}

func TestMove_CreditFailureRestoresSource(t *testing.T) {
	t.Run("item to stash", func(t *testing.T) {
		f := newFx()
		sid := f.stash(t, "Chest", "c1")
		f.repo.downtime = true
		f.repo.failCredit = true
		_, err := f.svc.Move(context.Background(), "camp", Actor{"u1", rPlayer}, MoveInput{Kind: MoveKindItem, ItemEntityID: "i1", Quantity: 3, From: charEP("c1"), To: StashEndpoint(sid)})
		if code(err) != http.StatusConflict {
			t.Fatalf("got %v", err)
		}
		if f.carried("c1", "i1") != 3 {
			t.Fatalf("source not restored: %d", f.carried("c1", "i1"))
		}
		if len(f.repo.moves) != 1 || f.repo.moves[0].Status != MoveFailed || f.repo.moves[0].Reason == "" {
			t.Fatalf("history %+v", f.repo.moves)
		}
	})
	t.Run("money to stash", func(t *testing.T) {
		f := newFx()
		sid := f.stash(t, "Chest", "c1")
		f.repo.downtime = true
		f.repo.failCredit = true
		_, err := f.svc.Move(context.Background(), "camp", Actor{"u1", rPlayer}, MoveInput{Kind: MoveKindMoney, Amount: "20", From: charEP("c1"), To: StashEndpoint(sid)})
		if code(err) != http.StatusConflict {
			t.Fatalf("got %v", err)
		}
		if f.fields.data["c1"]["gp"] != 50.0 {
			t.Fatalf("money not restored: %v", f.fields.data["c1"]["gp"])
		}
	})
}

func TestStashVisibility(t *testing.T) {
	f := newFx()
	hidden := f.stash(t, "Cache")             // no viewers: GM only
	shared := f.stash(t, "Party chest", "c1") // u1 owns c1
	ctx := context.Background()

	names := func(a Actor) []string {
		v, err := f.svc.StashesPage(ctx, "camp", a)
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, s := range v.Stashes {
			out = append(out, s.Name)
		}
		return out
	}
	if got := names(Actor{"u1", rPlayer}); len(got) != 1 || got[0] != "Party chest" {
		t.Fatalf("owner of a viewer character sees %v", got)
	}
	if got := names(Actor{"u2", rPlayer}); len(got) != 0 {
		t.Fatalf("unrelated player sees %v", got)
	}
	if got := names(Actor{"gm", rScribe}); len(got) != 2 {
		t.Fatalf("scribe sees %v", got)
	}

	// A hidden stash is not found, not forbidden, for a player.
	if _, err := f.svc.StashHistory(ctx, "camp", Actor{"u1", rPlayer}, hidden); code(err) != http.StatusNotFound {
		t.Fatalf("got %v", err)
	}
	if _, err := f.svc.StashHistory(ctx, "camp", Actor{"u1", rPlayer}, shared); err != nil {
		t.Fatal(err)
	}
	// ...and cannot be a move destination.
	f.repo.downtime = true
	_, err := f.svc.Move(ctx, "camp", Actor{"u1", rPlayer}, MoveInput{Kind: MoveKindItem, ItemEntityID: "i1", Quantity: 1, From: charEP("c1"), To: StashEndpoint(hidden)})
	if code(err) != http.StatusNotFound {
		t.Fatalf("got %v", err)
	}
	// Players never see others' pending requests; only Owner visibility gets the list.
	v, _ := f.svc.StashesPage(ctx, "camp", Actor{"u1", rPlayer})
	if v.CanApprove || len(v.Pending) != 0 {
		t.Fatal("player got the requests panel")
	}
}

func TestManagementIsGMOnly(t *testing.T) {
	f := newFx()
	ctx := context.Background()
	p := Actor{"u1", rPlayer}
	if _, err := f.svc.CreateStash(ctx, "camp", p, CreateStashInput{Name: "x"}); code(err) != http.StatusForbidden {
		t.Fatalf("create: %v", err)
	}
	sid := f.stash(t, "Chest")
	if err := f.svc.UpdateStash(ctx, "camp", p, sid, UpdateStashInput{Name: patch.Of("y")}); code(err) != http.StatusForbidden {
		t.Fatalf("update: %v", err)
	}
	if err := f.svc.DeleteStash(ctx, "camp", p, sid); code(err) != http.StatusForbidden {
		t.Fatalf("delete: %v", err)
	}
	if err := f.svc.SetStashViewers(ctx, "camp", p, sid, []string{"c1"}); code(err) != http.StatusForbidden {
		t.Fatalf("viewers: %v", err)
	}
	if err := f.svc.SetStashViewers(ctx, "camp", Actor{"gm", rScribe}, sid, []string{"n1"}); code(err) != http.StatusNotFound {
		t.Fatalf("a non-character viewer: %v", err)
	}
	if err := f.svc.AddStashItem(ctx, "camp", Actor{"gm", rScribe}, sid, "i1", 4); err != nil || f.repo.items[sid]["i1"] != 4 {
		t.Fatalf("add item: %v", err)
	}
	if err := f.svc.AddStashItem(ctx, "camp", Actor{"gm", rScribe}, sid, "c1", 1); code(err) != http.StatusNotFound {
		t.Fatalf("a non-item: %v", err)
	}
}

// TestUpdateStash_IsPartial pins the partial-update contract: a rename must
// leave the location alone, and only an explicit null clears it.
func TestUpdateStash_IsPartial(t *testing.T) {
	tests := []struct {
		name     string
		in       UpdateStashInput
		wantName string
		wantLoc  string
	}{
		{"rename only keeps location", UpdateStashInput{Name: patch.Of("New")}, "New", "Cellar"},
		{"location only keeps name", UpdateStashInput{Location: patch.Of("Attic")}, "Old", "Attic"},
		{"null clears location", UpdateStashInput{Location: patch.Null[string]()}, "Old", ""},
		{"null name keeps name", UpdateStashInput{Name: patch.Null[string]()}, "Old", "Cellar"},
		{"empty body changes nothing", UpdateStashInput{}, "Old", "Cellar"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newFx()
			st, _ := f.svc.CreateStash(context.Background(), "camp", Actor{"gm", rOwner}, CreateStashInput{Name: "Old", Location: "Cellar"})
			if err := f.svc.UpdateStash(context.Background(), "camp", Actor{"gm", rScribe}, st.ID, tc.in); err != nil {
				t.Fatal(err)
			}
			got := f.repo.stashes[st.ID]
			if got.Name != tc.wantName || got.Location != tc.wantLoc {
				t.Fatalf("%q/%q", got.Name, got.Location)
			}
		})
	}
	f := newFx()
	st, _ := f.svc.CreateStash(context.Background(), "camp", Actor{"gm", rOwner}, CreateStashInput{Name: "Old"})
	if err := f.svc.UpdateStash(context.Background(), "camp", Actor{"gm", rOwner}, st.ID, UpdateStashInput{Name: patch.Of("  ")}); code(err) != http.StatusBadRequest {
		t.Fatalf("blank name: %v", err)
	}
}

func TestCharacterPanelAndHistoryAccess(t *testing.T) {
	f := newFx()
	ctx := context.Background()
	v, err := f.svc.CharacterPanel(ctx, "camp", Actor{"u1", rPlayer}, "c1")
	if err != nil || v == nil || len(v.Items) != 1 || v.Items[0].Quantity != 3 || !v.HasMoney || v.Money != 5000 {
		t.Fatalf("%+v %v", v, err)
	}
	// Someone else's character: no panel, no history.
	if v, err := f.svc.CharacterPanel(ctx, "camp", Actor{"u2", rPlayer}, "c1"); err != nil || v != nil {
		t.Fatalf("%+v %v", v, err)
	}
	if _, err := f.svc.CharacterHistory(ctx, "camp", Actor{"u2", rPlayer}, "c1"); code(err) != http.StatusNotFound {
		t.Fatalf("got %v", err)
	}
	if v, _ := f.svc.CharacterPanel(ctx, "camp", Actor{"u1", rPlayer}, "n1"); v != nil {
		t.Fatal("panel for a non-character")
	}
	if _, err := f.svc.CharacterPanel(ctx, "other", Actor{"gm", rOwner}, "c1"); code(err) != http.StatusNotFound {
		t.Fatalf("foreign campaign: %v", err)
	}
	// No money field: the panel says so rather than showing a zero.
	if v, _ := f.svc.CharacterPanel(ctx, "camp", Actor{"gm", rScribe}, "c3"); v == nil || v.HasMoney {
		t.Fatalf("%+v", v)
	}
	// The GM sees it too.
	if v, _ := f.svc.CharacterPanel(ctx, "camp", Actor{"gm", rScribe}, "c2"); v == nil || v.Money != 1000 {
		t.Fatalf("%+v", v)
	}
}

func TestMoveDialog(t *testing.T) {
	f := newFx()
	sid := f.stash(t, "Chest", "c1")
	f.stash(t, "Cache") // hidden from u1
	ctx := context.Background()
	d, err := f.svc.MoveDialog(ctx, "camp", Actor{"u1", rPlayer}, MoveKindItem, charEP("c1"), "i1")
	if err != nil {
		t.Fatal(err)
	}
	if d.Immediate || d.Max != 3 {
		t.Fatalf("%+v", d)
	}
	var stashes int
	for _, x := range d.Destinations {
		if x.Group == "Stashes" {
			stashes++
			if x.Endpoint != StashEndpoint(sid) {
				t.Fatalf("offered %+v", x)
			}
		}
		if x.Endpoint == charEP("c1") {
			t.Fatal("offered the source as a destination")
		}
	}
	if stashes != 1 {
		t.Fatalf("stash choices %d", stashes)
	}
	if _, err := f.svc.MoveDialog(ctx, "camp", Actor{"u2", rPlayer}, MoveKindItem, charEP("c1"), "i1"); code(err) != http.StatusForbidden {
		t.Fatalf("got %v", err)
	}
	// Money destinations skip characters with no money field.
	dm, _ := f.svc.MoveDialog(ctx, "camp", Actor{"u1", rPlayer}, MoveKindMoney, charEP("c1"), "")
	for _, x := range dm.Destinations {
		if x.Endpoint == charEP("c3") {
			t.Fatal("offered a character with no money field")
		}
	}
	f.repo.downtime = true
	if d, _ := f.svc.MoveDialog(ctx, "camp", Actor{"u1", rPlayer}, MoveKindItem, charEP("c1"), "i1"); !d.Immediate {
		t.Fatal("downtime open should be immediate")
	}
}

func TestParseCents(t *testing.T) {
	tests := []struct {
		in   string
		want Cents
		ok   bool
	}{
		{"12", 1200, true}, {"12.5", 1250, true}, {"12.50", 1250, true}, {".5", 50, true}, {" 3 ", 300, true},
		{"0.01", 1, true},
		{"", 0, false}, {"0", 0, false}, {"-1", 0, false}, {"+1", 0, false}, {"1e3", 0, false},
		{"1.234", 0, false}, {"1,000", 0, false}, {"abc", 0, false}, {".", 0, false}, {"1234567890123", 0, false},
	}
	for _, tc := range tests {
		got, err := ParseCents(tc.in)
		if (err == nil) != tc.ok || (tc.ok && got != tc.want) {
			t.Errorf("ParseCents(%q) = %v, %v", tc.in, got, err)
		}
	}
	if Cents(1250).Decimal() != "12.50" || Cents(1200).String() != "12" || Cents(1205).String() != "12.05" {
		t.Error("formatting")
	}
}

func TestMoveSummary(t *testing.T) {
	base := MoveLine{Move: Move{Kind: MoveKindItem, Quantity: 2, RequestedBy: "u1"}, ItemName: "Potion", FromName: "Thorin", ToName: "Chest", RequesterName: "Ana"}
	tests := []struct {
		name string
		mut  func(*MoveLine)
		want string
	}{
		{"applied", func(l *MoveLine) { l.Status = MoveApplied }, "Ana moved 2 × Potion from Thorin to Chest."},
		{"approved", func(l *MoveLine) { l.Status = MoveApplied; l.DecidedBy = "gm" }, "The GM approved Ana's request to move 2 × Potion from Thorin to Chest."},
		{"pending", func(l *MoveLine) { l.Status = MovePending }, "Ana asked to move 2 × Potion from Thorin to Chest. Waiting for the GM."},
		{"declined", func(l *MoveLine) { l.Status = MoveDeclined }, "The GM turned down Ana's request to move 2 × Potion from Thorin to Chest."},
		{"failed", func(l *MoveLine) { l.Status = MoveFailed; l.Reason = "No." }, "Ana's move of 2 × Potion from Thorin to Chest didn't go through. No."},
		{"money", func(l *MoveLine) { l.Status = MoveApplied; l.Kind = MoveKindMoney; l.Amount = 1250 }, "Ana moved 12.50 money from Thorin to Chest."},
	}
	for _, tc := range tests {
		l := base
		tc.mut(&l)
		if got := MoveSummary(l); got != tc.want {
			t.Errorf("%s: %q", tc.name, got)
		}
	}
	_ = strconv.Itoa
}

func TestHistory_HidesOthersPendingFromPlayers(t *testing.T) {
	f := newFx()
	sid := f.stash(t, "Chest", "c1", "c2")
	ctx := context.Background()
	mine, _ := f.svc.Move(ctx, "camp", Actor{"u1", rPlayer}, MoveInput{Kind: MoveKindItem, ItemEntityID: "i1", Quantity: 1, From: charEP("c1"), To: StashEndpoint(sid)})
	f.repo.moves = append(f.repo.moves, &Move{ID: 99, CampaignID: "camp", Kind: MoveKindMoney, Amount: 100, From: StashEndpoint(sid), To: charEP("c2"), Status: MovePending, RequestedBy: "u2"})
	f.repo.moves = append(f.repo.moves, &Move{ID: 98, CampaignID: "camp", Kind: MoveKindMoney, Amount: 100, From: StashEndpoint(sid), To: charEP("c2"), Status: MoveFailed, RequestedBy: "u2"})
	for name, get := range map[string]func(Actor) ([]MoveLine, error){
		"stash":     func(a Actor) ([]MoveLine, error) { return f.svc.StashHistory(ctx, "camp", a, sid) },
		"character": func(a Actor) ([]MoveLine, error) { return f.svc.CharacterHistory(ctx, "camp", a, "c1") },
	} {
		lines, err := get(Actor{"u1", rPlayer})
		if err != nil {
			t.Fatal(err)
		}
		for _, l := range lines {
			if l.Status == MovePending && l.RequestedBy != "u1" {
				t.Fatalf("%s: leaked another player's pending request", name)
			}
		}
		if name == "stash" {
			if len(lines) != 2 { // own pending + others' failed
				t.Fatalf("stash lines %d", len(lines))
			}
			if gm, _ := get(Actor{"gm", rScribe}); len(gm) != 3 {
				t.Fatalf("gm sees %d", len(gm))
			}
		}
	}
	_ = mine
}

func TestMove_DmOnlyRelation(t *testing.T) {
	f := newFx()
	f.repo.downtime = true
	f.rels.rels["c1"][0].DmOnly = true
	in := MoveInput{Kind: MoveKindItem, ItemEntityID: "i1", Quantity: 1, From: charEP("c1"), To: charEP("c2")}
	_, err := f.svc.Move(context.Background(), "camp", Actor{"u1", rPlayer}, in)
	if code(err) != http.StatusBadRequest || f.carried("c1", "i1") != 3 {
		t.Fatalf("player took from a DM-only line: %v", err)
	}
	// Same message as an item they simply don't hold.
	in2 := in
	in2.ItemEntityID = "i2"
	_, err2 := f.svc.Move(context.Background(), "camp", Actor{"u1", rPlayer}, in2)
	if err == nil || err2 == nil || err.Error() != err2.Error() {
		t.Fatalf("messages differ: %v / %v", err, err2)
	}
	// A Scribe cannot see DM-only lines either, so cannot move one.
	if _, err := f.svc.Move(context.Background(), "camp", Actor{"scribe", rScribe}, in); code(err) != http.StatusBadRequest || f.carried("c1", "i1") != 3 {
		t.Fatalf("scribe took from a DM-only line: %v", err)
	}
	// The Owner (or a co-DM, whose role is promoted) can.
	if _, err := f.svc.Move(context.Background(), "camp", Actor{"gm", rOwner}, in); err != nil {
		t.Fatal(err)
	}
	// A player's credit never merges into a DM-only line.
	f.rels.rels["c2"] = []*HasItemRelation{{ID: 500, ItemEntityID: "i1", DmOnly: true, Metadata: []byte(`{"quantity":1}`)}}
	f.rels.rels["c1"] = []*HasItemRelation{{ID: 501, ItemEntityID: "i1", Metadata: []byte(`{"quantity":2}`)}}
	f.rels.createConflict = true
	_, _ = f.svc.Move(context.Background(), "camp", Actor{"u1", rPlayer}, in)
	if q, _ := parseCarried(f.rels.find(500).Metadata); q != 1 {
		t.Fatalf("merged into DM-only line: %d", q)
	}
	if f.carried("c1", "i1") != 2 {
		t.Fatalf("source not restored: %d", f.carried("c1", "i1"))
	}
}

func TestMove_FullStackFailedCreditRestoresMetadata(t *testing.T) {
	f := newFx()
	f.repo.downtime = true
	sid := f.stash(t, "Chest", "c1")
	f.repo.failCredit = true
	orig := string(f.rels.rels["c1"][0].Metadata)
	_, err := f.svc.Move(context.Background(), "camp", Actor{"u1", rPlayer}, MoveInput{Kind: MoveKindItem, ItemEntityID: "i1", Quantity: 3, From: charEP("c1"), To: StashEndpoint(sid)})
	if code(err) != http.StatusConflict {
		t.Fatal(err)
	}
	rels := f.rels.rels["c1"]
	if len(rels) != 1 || rels[0].ID != 100 {
		t.Fatalf("relation replaced: %+v", rels)
	}
	var a, b map[string]any
	_ = json.Unmarshal([]byte(orig), &a)
	_ = json.Unmarshal(rels[0].Metadata, &b)
	if b["equipped"] != a["equipped"] || b["quantity"] != a["quantity"] {
		t.Fatalf("metadata %s vs %s", rels[0].Metadata, orig)
	}
	// And on success the emptied line is removed.
	f.repo.failCredit = false
	if _, err := f.svc.Move(context.Background(), "camp", Actor{"u1", rPlayer}, MoveInput{Kind: MoveKindItem, ItemEntityID: "i1", Quantity: 3, From: charEP("c1"), To: StashEndpoint(sid)}); err != nil {
		t.Fatal(err)
	}
	if len(f.rels.rels["c1"]) != 0 {
		t.Fatal("emptied line left behind")
	}
}

func TestMove_PendingCap(t *testing.T) {
	f := newFx()
	sid := f.stash(t, "Chest", "c1", "c2")
	in := MoveInput{Kind: MoveKindItem, ItemEntityID: "i1", Quantity: 1, From: charEP("c1"), To: StashEndpoint(sid)}
	for i := 0; i < 20; i++ {
		if _, err := f.svc.Move(context.Background(), "camp", Actor{"u1", rPlayer}, in); err != nil {
			t.Fatalf("request %d: %v", i+1, err)
		}
	}
	_, err := f.svc.Move(context.Background(), "camp", Actor{"u1", rPlayer}, in)
	if code(err) != http.StatusBadRequest || err.Error() == "" || !strings.Contains(err.Error(), "You have 20 requests waiting for the GM already.") {
		t.Fatalf("21st: %v", err)
	}
	// Another player is unaffected by u1's queue.
	f.rels.rels["c2"] = []*HasItemRelation{{ID: 600, ItemEntityID: "i1", Metadata: []byte(`{"quantity":1}`)}}
	if _, err := f.svc.Move(context.Background(), "camp", Actor{"u2", rPlayer}, MoveInput{Kind: MoveKindItem, ItemEntityID: "i1", Quantity: 1, From: charEP("c2"), To: StashEndpoint(sid)}); err != nil {
		t.Fatal(err)
	}
}

func TestMove_InsertFailureMovesNothing(t *testing.T) {
	f := newFx()
	f.repo.downtime = true
	f.repo.failInsert = true
	sid := f.stash(t, "Chest", "c1")
	_, err := f.svc.Move(context.Background(), "camp", Actor{"u1", rPlayer}, MoveInput{Kind: MoveKindItem, ItemEntityID: "i1", Quantity: 1, From: charEP("c1"), To: StashEndpoint(sid)})
	if err == nil || f.carried("c1", "i1") != 3 || f.repo.items[sid]["i1"] != 0 {
		t.Fatalf("moved without a record: %v", err)
	}
}

// DM-only inventory lines follow Chronicle's DM-only rule: the Owner (or a
// co-DM, whose role is promoted) sees them, a Scribe does not.
func TestCarried_DmOnlyLinesFollowTheDmOnlyRule(t *testing.T) {
	for _, tc := range []struct {
		name string
		role int
		want bool
	}{{"player", rPlayer, false}, {"scribe", rScribe, false}, {"owner or co-DM", rOwner, true}} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFx()
			f.rels.rels["c1"][0].DmOnly = true
			dmItem := f.rels.rels["c1"][0].ItemEntityID
			held, err := f.svc.(*stashService).carried(context.Background(), "camp", Actor{"u", tc.role}, "c1")
			if err != nil {
				t.Fatal(err)
			}
			got := false
			for _, h := range held {
				if h.ItemID == dmItem {
					got = true
				}
			}
			if got != tc.want {
				t.Errorf("sees the DM-only line = %v, want %v", got, tc.want)
			}
		})
	}
}
