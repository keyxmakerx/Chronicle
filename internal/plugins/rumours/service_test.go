package rumours

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/patch"
)

// codeOf is the HTTP status an error would be answered with, 0 for no error.
func codeOf(err error) int {
	if err == nil {
		return 0
	}
	var ae *apperror.AppError
	if errors.As(err, &ae) {
		return ae.Code
	}
	return http.StatusInternalServerError
}

func quotesOf(v *View) []string {
	var out []string
	for _, r := range v.Rumours {
		out = append(out, r.Quote)
	}
	return out
}

// What each viewer is told. The Director's truth reaches the Director and
// nobody else, whatever the repository hands over.
func TestView_WhoIsToldWhat(t *testing.T) {
	tests := []struct {
		name      string
		viewer    Viewer
		leaky     bool
		wantDM    bool
		wantAdd   bool
		wantTruth bool
	}{
		{name: "director gets the truth", viewer: vDM, wantDM: true, wantAdd: true, wantTruth: true},
		{name: "a scribe gets none of it", viewer: vMira, wantAdd: true},
		{name: "a scribe gets none of it even from a leaky repository", viewer: vMira, leaky: true, wantAdd: true},
		{name: "a player gets none of it", viewer: vJo},
		{name: "a player gets none of it even from a leaky repository", viewer: vJo, leaky: true},
		{name: "a visitor with no seat gets none of it", viewer: vVisitor, leaky: true},
		{name: "a granted co-director counts as the director", viewer: Viewer{UserID: "co", Role: 3}, wantDM: true, wantAdd: true, wantTruth: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := seeded()
			repo.leaky = tt.leaky
			v, err := newTestService(repo).View(context.Background(), tCamp, tPage, tt.viewer)
			if err != nil {
				t.Fatal(err)
			}
			if v.Hidden || len(v.Rumours) != 4 {
				t.Fatalf("hidden=%v, %d rumours; want all four", v.Hidden, len(v.Rumours))
			}
			if v.IsDM != tt.wantDM || v.CanAdd != tt.wantAdd || v.CanMark != tt.wantAdd {
				t.Errorf("IsDM/CanAdd/CanMark = %v/%v/%v, want %v/%v/%v", v.IsDM, v.CanAdd, v.CanMark, tt.wantDM, tt.wantAdd, tt.wantAdd)
			}
			for _, r := range v.Rumours {
				if (r.Truth != nil) != tt.wantTruth {
					t.Errorf("rumour %q truth present = %v, want %v", r.Quote, r.Truth != nil, tt.wantTruth)
				}
			}
			raw, _ := json.Marshal(v)
			if has := strings.Contains(string(raw), `"truth"`); has != tt.wantTruth {
				t.Errorf("JSON carries a truth key = %v, want %v: %s", has, tt.wantTruth, raw)
			}
			if tt.wantTruth != (repo.truthReads > 0) {
				t.Errorf("repository asked for the truth %d times; want it only for the director", repo.truthReads)
			}
		})
	}
}

func TestView_NewestFirstAndWhatTheViewerSeesAboutWhoDidWhat(t *testing.T) {
	tests := []struct {
		name         string
		viewer       Viewer
		wantAddedBy  map[string]string // rumour id -> byline
		wantBeliefBy map[string]string
		wantCanEdit  map[string]bool
	}{
		{name: "director: no byline on their own, names on the others; edits all", viewer: vDM,
			wantAddedBy:  map[string]string{idShip: "", idTreas: "Mira", idLog: "", idFavour: "Tomas"},
			wantBeliefBy: map[string]string{idShip: "Tomas", idTreas: "Mira", idLog: ""},
			wantCanEdit:  map[string]bool{idShip: true, idTreas: true, idLog: true, idFavour: true}},
		{name: "scribe: 'you' on their own; edits only those", viewer: vMira,
			wantAddedBy:  map[string]string{idShip: "", idTreas: "you", idLog: "", idFavour: "Tomas"},
			wantBeliefBy: map[string]string{idShip: "Tomas", idTreas: "Mira"},
			wantCanEdit:  map[string]bool{idShip: false, idTreas: true, idLog: false, idFavour: false}},
		{name: "player: edits nothing", viewer: vJo,
			wantAddedBy: map[string]string{idShip: "", idTreas: "Mira", idLog: "", idFavour: "Tomas"},
			wantCanEdit: map[string]bool{idShip: false, idTreas: false, idLog: false, idFavour: false}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, err := newTestService(seeded()).View(context.Background(), tCamp, tPage, tt.viewer)
			if err != nil {
				t.Fatal(err)
			}
			if got := quotesOf(v); got[0] != "The harbourmaster owes her a favour." || got[3] != "She sank her own ship for the insurance." {
				t.Errorf("order = %v; want newest first", got)
			}
			for _, r := range v.Rumours {
				if want := tt.wantAddedBy[r.ID]; r.AddedBy != want {
					t.Errorf("%s AddedBy = %q, want %q", r.ID, r.AddedBy, want)
				}
				if want, ok := tt.wantBeliefBy[r.ID]; ok && r.BeliefBy != want {
					t.Errorf("%s BeliefBy = %q, want %q", r.ID, r.BeliefBy, want)
				}
				if r.CanEdit != tt.wantCanEdit[r.ID] {
					t.Errorf("%s CanEdit = %v, want %v", r.ID, r.CanEdit, tt.wantCanEdit[r.ID])
				}
			}
		})
	}
}

func TestView_HiddenOnlyWhenThereIsNothingToReadOrDo(t *testing.T) {
	tests := []struct {
		name       string
		viewer     Viewer
		wantHidden bool
	}{
		{name: "a player on a page with no rumours gets no block, so no blank gap", viewer: vJo, wantHidden: true},
		{name: "a visitor on a page with no rumours gets no block", viewer: vVisitor, wantHidden: true},
		{name: "a scribe still gets it, to add the first", viewer: vMira},
		{name: "the director still gets it, to add the first", viewer: vDM},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, err := newTestService(newFakeRepo()).View(context.Background(), tCamp, tPage, tt.viewer)
			if err != nil {
				t.Fatal(err)
			}
			if v.Hidden != tt.wantHidden {
				t.Errorf("Hidden = %v, want %v", v.Hidden, tt.wantHidden)
			}
		})
	}
}

func TestView_ABoundThatCutsTheListOffIsSaid(t *testing.T) {
	repo := newFakeRepo()
	repo.rows = nil
	for i := 0; i < MaxRumours+30; i++ {
		repo.rows = append(repo.rows, Rumour{ID: "x", CampaignID: tCamp, EntityID: tPage, Quote: "q", Sure: 2, CreatedAt: seeded().rows[0].CreatedAt.Add(-timeHours(i))})
	}
	v, err := newTestService(repo).View(context.Background(), tCamp, tPage, vJo)
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Rumours) != MaxRumours || v.Total != MaxRumours+30 || !v.Cut() {
		t.Errorf("sent %d of %d, Cut=%v; want %d of %d and cut", len(v.Rumours), v.Total, v.Cut(), MaxRumours, MaxRumours+30)
	}
	if got := cutNotice(v); got != "Showing the newest 200 of 230 rumours on this page." {
		t.Errorf("notice = %q", got)
	}
	whole, _ := newTestService(seeded()).View(context.Background(), tCamp, tPage, vJo)
	if whole.Cut() {
		t.Error("a whole list reported itself cut")
	}
}

func TestWhenText(t *testing.T) {
	tests := []struct{ name, at, want string }{
		{"this year drops the year", "2026-03-04T10:00:00Z", "4 Mar"},
		{"another year keeps it", "2025-12-31T23:00:00Z", "31 Dec 2025"},
	}
	now := newTestService(newFakeRepo()).(*service).now()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			at, _ := parseTime(tt.at)
			if got := whenText(at, now); got != tt.want {
				t.Errorf("whenText = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAdd(t *testing.T) {
	long := strings.Repeat("a", QuoteMax+1)
	tests := []struct {
		name     string
		viewer   Viewer
		in       AddInput
		noPage   bool
		full     bool
		wantCode int
		wantBy   string // AddedBy in the answer
		wantByDM bool
	}{
		{name: "director adds, no byline", viewer: vDM, in: AddInput{Quote: "  The tide is wrong  ", Who: "Pell", Sure: 3}, wantByDM: true},
		{name: "scribe adds under their name", viewer: vMira, in: AddInput{Quote: "x", Who: "Pell", Sure: 1}, wantBy: "you"},
		{name: "sureness defaults to fairly sure; speaker may be blank", viewer: vMira, in: AddInput{Quote: "x"}, wantBy: "you"},
		{name: "a player cannot add", viewer: vJo, in: AddInput{Quote: "x", Sure: 2}, wantCode: 403},
		{name: "a visitor cannot add", viewer: vVisitor, in: AddInput{Quote: "x", Sure: 2}, wantCode: 403},
		{name: "empty rumour", viewer: vDM, in: AddInput{Quote: "   ", Sure: 2}, wantCode: 422},
		{name: "too long", viewer: vDM, in: AddInput{Quote: long, Sure: 2}, wantCode: 422},
		{name: "control characters", viewer: vDM, in: AddInput{Quote: "a\x00b", Sure: 2}, wantCode: 422},
		{name: "speaker too long", viewer: vDM, in: AddInput{Quote: "x", Who: strings.Repeat("w", SourceMax+1), Sure: 2}, wantCode: 422},
		{name: "sureness out of range", viewer: vDM, in: AddInput{Quote: "x", Sure: 4}, wantCode: 422},
		{name: "the page is full", viewer: vDM, in: AddInput{Quote: "x", Sure: 2}, full: true, wantCode: 422},
		{name: "a page that is not in the campaign", viewer: vDM, in: AddInput{Quote: "x", Sure: 2}, noPage: true, wantCode: 404},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := seeded()
			repo.noPage = tt.noPage
			if tt.full {
				for len(repo.rows) < MaxRumours {
					repo.rows = append(repo.rows, Rumour{ID: "f", CampaignID: tCamp, EntityID: tPage, Quote: "f", Sure: 2})
				}
			}
			before := len(repo.rows)
			tt.in.CampaignID, tt.in.EntityID, tt.in.Viewer = tCamp, tPage, tt.viewer
			got, err := newTestService(repo).Add(context.Background(), tt.in)
			if codeOf(err) != tt.wantCode {
				t.Fatalf("status = %d (%v), want %d", codeOf(err), err, tt.wantCode)
			}
			if tt.wantCode != 0 {
				if len(repo.rows) != before {
					t.Error("a refused add still stored a rumour")
				}
				return
			}
			if got.AddedBy != tt.wantBy {
				t.Errorf("AddedBy = %q, want %q", got.AddedBy, tt.wantBy)
			}
			stored := repo.rows[len(repo.rows)-1]
			if stored.ByDM != tt.wantByDM || stored.Truth != TruthOpen || stored.Belief != BeliefNone {
				t.Errorf("stored = %+v; want ByDM %v, truth open, belief unmarked", stored, tt.wantByDM)
			}
			if stored.Quote != strings.TrimSpace(tt.in.Quote) || stored.Sure < 1 {
				t.Errorf("stored quote %q sure %d", stored.Quote, stored.Sure)
			}
			// What the answer carries for this viewer: the truth only to the Director.
			if (got.Truth != nil) != tt.viewer.IsDM() {
				t.Errorf("answer carries truth = %v for %+v", got.Truth != nil, tt.viewer)
			}
		})
	}
}

func TestUpdate_WhoMayEditAndHowPartialItIs(t *testing.T) {
	tests := []struct {
		name     string
		viewer   Viewer
		id       string
		in       UpdateRumourInput
		wantCode int
		wantQ    string
		wantWho  string
		wantSure int
	}{
		{name: "director edits any rumour", viewer: vDM, id: idTreas, in: UpdateRumourInput{Quote: patch.Of("Dessa sailed west.")},
			wantQ: "Dessa sailed west.", wantWho: "Mara", wantSure: 1},
		{name: "a scribe edits their own", viewer: vMira, id: idTreas, in: UpdateRumourInput{Who: patch.Of("Mara's cousin")},
			wantQ: "Dessa Quill took the ship's treasure and sailed east.", wantWho: "Mara's cousin", wantSure: 1},
		{name: "a scribe cannot edit the director's", viewer: vMira, id: idShip, in: UpdateRumourInput{Quote: patch.Of("hijack")}, wantCode: 403},
		{name: "a scribe cannot edit another scribe's", viewer: vMira, id: idFavour, in: UpdateRumourInput{Quote: patch.Of("hijack")}, wantCode: 403},
		{name: "a player cannot edit", viewer: vJo, id: idTreas, in: UpdateRumourInput{Quote: patch.Of("hijack")}, wantCode: 403},
		{name: "absent keys preserve everything", viewer: vDM, id: idShip, in: UpdateRumourInput{},
			wantQ: "She sank her own ship for the insurance.", wantWho: "Old Bram", wantSure: 3},
		{name: "an explicit null on a required field preserves, never blanks", viewer: vDM, id: idShip, in: UpdateRumourInput{Quote: patch.Null[string](), Sure: patch.Null[int]()},
			wantQ: "She sank her own ship for the insurance.", wantWho: "Old Bram", wantSure: 3},
		{name: "a present sureness replaces", viewer: vDM, id: idShip, in: UpdateRumourInput{Sure: patch.Of(1)},
			wantQ: "She sank her own ship for the insurance.", wantWho: "Old Bram", wantSure: 1},
		{name: "the merged value is validated: an empty quote is refused", viewer: vDM, id: idShip, in: UpdateRumourInput{Quote: patch.Of("  ")}, wantCode: 422},
		{name: "sureness out of range is refused", viewer: vDM, id: idShip, in: UpdateRumourInput{Sure: patch.Of(9)}, wantCode: 422},
		{name: "an id that is not a rumour", viewer: vDM, id: "nope", in: UpdateRumourInput{Quote: patch.Of("x")}, wantCode: 404},
		{name: "an id that is not on this page", viewer: vDM, id: "6f0a7c52-6b64-4d6f-8d0c-0b6f3a1f0099", in: UpdateRumourInput{Quote: patch.Of("x")}, wantCode: 404},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := seeded()
			before := *repo.at(tCamp, tPage, idShip)
			got, err := newTestService(repo).Update(context.Background(), tCamp, tPage, tt.id, tt.in, tt.viewer)
			if codeOf(err) != tt.wantCode {
				t.Fatalf("status = %d (%v), want %d", codeOf(err), err, tt.wantCode)
			}
			if tt.wantCode != 0 {
				if after := *repo.at(tCamp, tPage, idShip); after != before {
					t.Error("a refused edit still changed a rumour")
				}
				return
			}
			if got.Quote != tt.wantQ || got.Who != tt.wantWho || got.Sure != tt.wantSure {
				t.Errorf("got %q / %q / %d, want %q / %q / %d", got.Quote, got.Who, got.Sure, tt.wantQ, tt.wantWho, tt.wantSure)
			}
		})
	}
}

// A Scribe can add and can set what the party thinks, and can never change
// the Director's truth, by any route.
func TestSetTruth_IsTheDirectorsAlone(t *testing.T) {
	tests := []struct {
		name     string
		viewer   Viewer
		truth    string
		id       string
		wantCode int
		wantNow  string
	}{
		{name: "director settles it", viewer: vDM, id: idLog, truth: TruthTrue, wantNow: TruthTrue},
		{name: "director reopens it", viewer: vDM, id: idShip, truth: TruthOpen, wantNow: TruthOpen},
		{name: "a scribe cannot, even on their own rumour", viewer: vMira, id: idTreas, truth: TruthTrue, wantCode: 403, wantNow: TruthFalse},
		{name: "a scribe cannot settle an open one", viewer: vTomas, id: idLog, truth: TruthFalse, wantCode: 403, wantNow: TruthOpen},
		{name: "a player cannot", viewer: vJo, id: idLog, truth: TruthTrue, wantCode: 403, wantNow: TruthOpen},
		{name: "a visitor cannot", viewer: vVisitor, id: idLog, truth: TruthTrue, wantCode: 403, wantNow: TruthOpen},
		{name: "a word that is not a verdict", viewer: vDM, id: idLog, truth: "maybe", wantCode: 422, wantNow: TruthOpen},
		{name: "unsure is the party's word, not the Director's", viewer: vDM, id: idLog, truth: BeliefUnsure, wantCode: 422, wantNow: TruthOpen},
		{name: "a rumour that is not there", viewer: vDM, id: "6f0a7c52-6b64-4d6f-8d0c-0b6f3a1f0099", truth: TruthTrue, wantCode: 404},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := seeded()
			got, err := newTestService(repo).SetTruth(context.Background(), tCamp, tPage, tt.id, tt.truth, tt.viewer)
			if codeOf(err) != tt.wantCode {
				t.Fatalf("status = %d (%v), want %d", codeOf(err), err, tt.wantCode)
			}
			if tt.wantNow != "" && repo.truthOf(tt.id) != tt.wantNow {
				t.Errorf("stored truth = %q, want %q", repo.truthOf(tt.id), tt.wantNow)
			}
			if tt.wantCode == 0 && (got.Truth == nil || *got.Truth != tt.truth) {
				t.Errorf("answer truth = %v", got.Truth)
			}
		})
	}
}

// Every other write a Scribe can make leaves the truth exactly as it was.
func TestAScribeNeverChangesTheTruthByAnyOtherRoute(t *testing.T) {
	repo := seeded()
	svc := newTestService(repo)
	ctx := context.Background()
	snap := map[string]string{}
	for _, r := range repo.rows {
		snap[r.ID] = r.Truth
	}
	added, err := svc.Add(ctx, AddInput{CampaignID: tCamp, EntityID: tPage, Quote: "new", Sure: 2, Viewer: vMira})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Update(ctx, tCamp, tPage, idTreas, UpdateRumourInput{Quote: patch.Of("edited")}, vMira); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{idShip, idTreas, idLog, idFavour, added.ID} {
		if _, err := svc.SetBelief(ctx, tCamp, tPage, id, BeliefUnsure, vMira); err != nil {
			t.Fatal(err)
		}
	}
	if err := svc.Remove(ctx, tCamp, tPage, idTreas, vMira); err != nil {
		t.Fatal(err)
	}
	for id, was := range snap {
		if id == idTreas {
			continue
		}
		if got := repo.truthOf(id); got != was {
			t.Errorf("%s truth went from %q to %q", id, was, got)
		}
	}
	if repo.truthOf(added.ID) != TruthOpen {
		t.Errorf("a scribe's new rumour truth = %q, want open", repo.truthOf(added.ID))
	}
}

func TestSetBelief(t *testing.T) {
	tests := []struct {
		name       string
		viewer     Viewer
		belief     string
		id         string
		wantCode   int
		wantBy     string
		wantTruth  bool // the answer carries the truth
		wantStored string
	}{
		{name: "a scribe marks any rumour", viewer: vMira, id: idLog, belief: BeliefTrue, wantBy: "Mira", wantStored: BeliefTrue},
		{name: "a scribe marks one the director wrote as unsure", viewer: vTomas, id: idShip, belief: BeliefUnsure, wantBy: "Tomas", wantStored: BeliefUnsure},
		{name: "the director sets it for the party and is named 'the DM'", viewer: vDM, id: idLog, belief: BeliefFalse, wantBy: "the DM", wantTruth: true, wantStored: BeliefFalse},
		{name: "a player cannot", viewer: vJo, id: idLog, belief: BeliefTrue, wantCode: 403, wantStored: BeliefNone},
		{name: "a visitor cannot", viewer: vVisitor, id: idLog, belief: BeliefTrue, wantCode: 403, wantStored: BeliefNone},
		{name: "only the three words", viewer: vMira, id: idLog, belief: "open", wantCode: 422, wantStored: BeliefNone},
		{name: "no empty mark", viewer: vMira, id: idLog, belief: "", wantCode: 422, wantStored: BeliefNone},
		{name: "a rumour that is not there", viewer: vMira, id: "6f0a7c52-6b64-4d6f-8d0c-0b6f3a1f0099", belief: BeliefTrue, wantCode: 404},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := seeded()
			got, err := newTestService(repo).SetBelief(context.Background(), tCamp, tPage, tt.id, tt.belief, tt.viewer)
			if codeOf(err) != tt.wantCode {
				t.Fatalf("status = %d (%v), want %d", codeOf(err), err, tt.wantCode)
			}
			if tt.wantStored != "" || tt.wantCode == 0 {
				if r := repo.at(tCamp, tPage, tt.id); r != nil && r.Belief != tt.wantStored {
					t.Errorf("stored belief = %q, want %q", r.Belief, tt.wantStored)
				}
			}
			if tt.wantCode != 0 {
				return
			}
			if got.Belief != tt.belief || got.BeliefBy != tt.wantBy {
				t.Errorf("answer belief %q by %q, want %q by %q", got.Belief, got.BeliefBy, tt.belief, tt.wantBy)
			}
			if (got.Truth != nil) != tt.wantTruth {
				t.Errorf("answer carries truth = %v, want %v", got.Truth != nil, tt.wantTruth)
			}
		})
	}
}

func TestRemove(t *testing.T) {
	tests := []struct {
		name     string
		viewer   Viewer
		id       string
		wantCode int
		wantGone bool
	}{
		{name: "director removes any", viewer: vDM, id: idTreas, wantGone: true},
		{name: "a scribe removes their own", viewer: vMira, id: idTreas, wantGone: true},
		{name: "a scribe cannot remove the director's", viewer: vMira, id: idShip, wantCode: 403},
		{name: "a scribe cannot remove another scribe's", viewer: vMira, id: idFavour, wantCode: 403},
		{name: "a player cannot", viewer: vJo, id: idTreas, wantCode: 403},
		{name: "a rumour that is not there", viewer: vDM, id: "6f0a7c52-6b64-4d6f-8d0c-0b6f3a1f0099", wantCode: 404},
		{name: "not even an id", viewer: vDM, id: "'; DROP TABLE page_rumours;--", wantCode: 404},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := seeded()
			err := newTestService(repo).Remove(context.Background(), tCamp, tPage, tt.id, tt.viewer)
			if codeOf(err) != tt.wantCode {
				t.Fatalf("status = %d (%v), want %d", codeOf(err), err, tt.wantCode)
			}
			gone := repo.at(tCamp, tPage, tt.id) == nil
			if tt.wantCode == 404 {
				return
			}
			if gone != tt.wantGone {
				t.Errorf("gone = %v, want %v", gone, tt.wantGone)
			}
			if len(repo.rows) != 4-map[bool]int{true: 1}[tt.wantGone] {
				t.Errorf("%d rumours left", len(repo.rows))
			}
		})
	}
}

// Nothing reaches across pages or campaigns.
func TestWritesAreScopedToThePageAndCampaign(t *testing.T) {
	repo := seeded()
	svc := newTestService(repo)
	ctx := context.Background()
	if _, err := svc.SetTruth(ctx, "other-camp", tPage, idLog, TruthTrue, vDM); codeOf(err) != 404 {
		t.Errorf("another campaign: status %d", codeOf(err))
	}
	if err := svc.Remove(ctx, tCamp, "other-page", idLog, vDM); codeOf(err) != 404 {
		t.Errorf("another page: status %d", codeOf(err))
	}
	if v, _ := svc.View(ctx, "other-camp", tPage, vDM); len(v.Rumours) != 0 {
		t.Error("another campaign's read returned rumours")
	}
	if len(repo.rows) != 4 || repo.truthOf(idLog) != TruthOpen {
		t.Error("a scoped-out write changed something")
	}
}
