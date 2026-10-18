package fieldrules

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// fakeRepo is an in-memory Repository; err forces every call to fail.
type fakeRepo struct {
	rules   []Rule
	words   []LocalWord
	nextID  int
	err     error
	dupRule bool
	dupWord bool
	// rulesCount/wordsCount override the counts to exercise limits cheaply.
	rulesCount, wordsCount *int
}

func (f *fakeRepo) id() int { f.nextID++; return f.nextID }

func (f *fakeRepo) ListRules(context.Context, string) ([]Rule, error) { return f.rules, f.err }
func (f *fakeRepo) GetRule(_ context.Context, _ string, id int) (*Rule, error) {
	if f.err != nil {
		return nil, f.err
	}
	for i := range f.rules {
		if f.rules[i].ID == id {
			r := f.rules[i]
			return &r, nil
		}
	}
	return nil, nil
}
func (f *fakeRepo) CountRules(context.Context, string) (int, error) {
	if f.rulesCount != nil {
		return *f.rulesCount, f.err
	}
	return len(f.rules), f.err
}
func (f *fakeRepo) InsertRule(_ context.Context, _ string, r Rule) (int, error) {
	if f.dupRule {
		return 0, errDuplicate
	}
	if f.err != nil {
		return 0, f.err
	}
	r.ID = f.id()
	f.rules = append(f.rules, r)
	return r.ID, nil
}
func (f *fakeRepo) UpdateRule(_ context.Context, _ string, r Rule) (bool, error) {
	if f.dupRule {
		return false, errDuplicate
	}
	for i := range f.rules {
		if f.rules[i].ID == r.ID {
			f.rules[i] = r
			return true, nil
		}
	}
	return false, nil
}
func (f *fakeRepo) DeleteRule(_ context.Context, _ string, id int) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	for i := range f.rules {
		if f.rules[i].ID == id {
			f.rules = append(f.rules[:i], f.rules[i+1:]...)
			return true, nil
		}
	}
	return false, nil
}
func (f *fakeRepo) ListWords(context.Context, string) ([]LocalWord, error) { return f.words, f.err }
func (f *fakeRepo) CountWords(context.Context, string) (int, error) {
	if f.wordsCount != nil {
		return *f.wordsCount, f.err
	}
	return len(f.words), f.err
}
func (f *fakeRepo) InsertWord(_ context.Context, _ string, w LocalWord) (int, error) {
	if f.dupWord {
		return 0, errDuplicate
	}
	if f.err != nil {
		return 0, f.err
	}
	w.ID = f.id()
	f.words = append(f.words, w)
	return w.ID, nil
}
func (f *fakeRepo) UpdateWord(_ context.Context, _ string, w LocalWord) (bool, error) {
	if f.dupWord {
		return false, errDuplicate
	}
	for i := range f.words {
		if f.words[i].ID == w.ID {
			f.words[i] = w
			return true, nil
		}
	}
	return false, nil
}
func (f *fakeRepo) DeleteWord(_ context.Context, _ string, id int) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	for i := range f.words {
		if f.words[i].ID == id {
			f.words = append(f.words[:i], f.words[i+1:]...)
			return true, nil
		}
	}
	return false, nil
}

// fakeWorld serves a fixed set of places and eras.
type fakeWorld struct {
	places map[string]string
	eras   map[int]string
	chain  []string
	era    int
}

func (w *fakeWorld) PlaceName(_ context.Context, _, id string) (string, bool) {
	n, ok := w.places[id]
	return n, ok
}
func (w *fakeWorld) EraNames(context.Context, string, Viewer) map[int]string { return w.eras }
func (w *fakeWorld) PlaceChain(context.Context, string, string) []string     { return w.chain }
func (w *fakeWorld) CurrentEra(context.Context, string, Viewer) int          { return w.era }

var (
	director = Viewer{UserID: "dm", Role: permissions.RoleOwner}
	player   = Viewer{UserID: "p", Role: permissions.RolePlayer}
	scribe   = Viewer{UserID: "s", Role: permissions.RoleScribe}
)

const camp = "camp-1"

func newTestService(repo *fakeRepo) Service {
	return NewService(repo, &fakeWorld{
		places: map[string]string{"town": "Townsville"},
		eras:   map[int]string{2: "Age of Ash"},
	})
}

// wantCode asserts err is an AppError carrying the HTTP status.
// wantCode fails the test unless err is an AppError with code.
func wantCode(t *testing.T, err error, code int) {
	t.Helper()
	_ = wantAppErr(t, err, code)
}

// wantAppErr is wantCode returning the error, for checks on its message.
func wantAppErr(t *testing.T, err error, code int) *apperror.AppError {
	t.Helper()
	var ae *apperror.AppError
	if !errors.As(err, &ae) {
		t.Fatalf("error = %v, want *AppError with code %d", err, code)
	}
	if ae.Code != code {
		t.Fatalf("code = %d (%s), want %d", ae.Code, ae.Message, code)
	}
	return ae
}

func validRule() RuleInput {
	return RuleInput{Name: "Rank", Follows: "echelon", Rows: []RuleRow{{Is: "1", Gives: "Iron"}}}
}

func TestNonDirectorRefused(t *testing.T) {
	ctx := context.Background()
	for _, v := range []Viewer{player, scribe} {
		svc := newTestService(&fakeRepo{rules: []Rule{{ID: 1, Name: "Rank"}}, words: []LocalWord{{ID: 1}}})
		calls := map[string]func() error{
			"Rules":      func() error { _, err := svc.Rules(ctx, camp, v); return err },
			"SaveRule":   func() error { _, err := svc.SaveRule(ctx, camp, validRule(), v); return err },
			"DeleteRule": func() error { return svc.DeleteRule(ctx, camp, 1, v) },
			"Words":      func() error { _, err := svc.Words(ctx, camp, v); return err },
			"SaveWord": func() error {
				_, err := svc.SaveWord(ctx, camp, WordInput{Word: "coin", Local: "mark", PlaceID: "town"}, v)
				return err
			},
			"DeleteWord": func() error { return svc.DeleteWord(ctx, camp, 1, v) },
			"CheckWord": func() error {
				return svc.CheckWord(ctx, camp, WordInput{Word: "coin", Local: "mark", PlaceID: "town"}, v)
			},
		}
		for name, call := range calls {
			t.Run(name, func(t *testing.T) { wantCode(t, call(), http.StatusForbidden) })
		}
		if got := svc.Eras(ctx, camp, v); got == nil || len(got) != 0 {
			t.Errorf("Eras for role %d = %v, want empty non-nil map", v.Role, got)
		}
	}
}

func TestNonDirectorWritesNothing(t *testing.T) {
	repo := &fakeRepo{}
	svc := newTestService(repo)
	_, _ = svc.SaveRule(context.Background(), camp, validRule(), player)
	if len(repo.rules) != 0 {
		t.Errorf("refused SaveRule still wrote %d rules", len(repo.rules))
	}
}

func TestDirectorEras(t *testing.T) {
	svc := newTestService(&fakeRepo{})
	got := svc.Eras(context.Background(), camp, director)
	if got[2] != "Age of Ash" {
		t.Errorf("Eras = %v", got)
	}
}

func TestGrantedCoDirectorIsDM(t *testing.T) {
	// The handler passes the visibility role, so a granted co-Director arrives
	// with the owner role and must be allowed.
	svc := newTestService(&fakeRepo{})
	if _, err := svc.Rules(context.Background(), camp, Viewer{Role: permissions.RoleOwner}); err != nil {
		t.Fatalf("Rules: %v", err)
	}
}

func TestRulesEmptyIsNonNil(t *testing.T) {
	got, err := newTestService(&fakeRepo{}).Rules(context.Background(), camp, director)
	if err != nil || got == nil || len(got) != 0 {
		t.Fatalf("Rules = (%v, %v), want empty non-nil slice", got, err)
	}
}

func TestRulesRepoErrorIsInternal(t *testing.T) {
	_, err := newTestService(&fakeRepo{err: errors.New("db down")}).Rules(context.Background(), camp, director)
	wantCode(t, err, http.StatusInternalServerError)
}

func TestCleanRule(t *testing.T) {
	row := func(is string, from, to *float64, gives string) RuleRow {
		return RuleRow{Is: is, From: from, To: to, Gives: gives}
	}
	manyRows := make([]RuleRow, MaxRuleRows+1)
	for i := range manyRows {
		manyRows[i] = row("a", nil, nil, "b")
	}
	okRows := manyRows[:MaxRuleRows]
	tests := []struct {
		name    string
		in      RuleInput
		wantMsg string // substring; empty means valid
	}{
		{"valid is row", validRule(), ""},
		{"valid range row", RuleInput{Name: "n", Follows: "f", Rows: []RuleRow{row("", fp(1), nil, "g")}}, ""},
		{"valid equal bounds", RuleInput{Name: "n", Follows: "f", Rows: []RuleRow{row("", fp(1), fp(1), "g")}}, ""},
		{"max rows allowed", RuleInput{Name: "n", Follows: "f", Rows: okRows}, ""},
		{"no name", RuleInput{Name: "  ", Follows: "f", Rows: okRows[:1]}, "needs a name"},
		{"name too long", RuleInput{Name: strings.Repeat("x", MaxNameLen+1), Follows: "f", Rows: okRows[:1]}, "at most"},
		{"no follows", RuleInput{Name: "n", Follows: " ", Rows: okRows[:1]}, "which field"},
		{"follows too long", RuleInput{Name: "n", Follows: strings.Repeat("x", maxFollowLen+1), Rows: okRows[:1]}, "too long"},
		{"no rows", RuleInput{Name: "n", Follows: "f"}, "at least one row"},
		{"too many rows", RuleInput{Name: "n", Follows: "f", Rows: manyRows}, "at most"},
		{"missing gives", RuleInput{Name: "n", Follows: "f", Rows: []RuleRow{row("a", nil, nil, "  ")}}, "row 1 needs what it gives"},
		{"gives too long", RuleInput{Name: "n", Follows: "f", Rows: []RuleRow{row("a", nil, nil, strings.Repeat("x", MaxGivesLen+1))}}, "row 1 is too long"},
		{"is too long", RuleInput{Name: "n", Follows: "f", Rows: []RuleRow{row(strings.Repeat("x", maxIsLen+1), nil, nil, "g")}}, "too long"},
		{"is and range", RuleInput{Name: "n", Follows: "f", Rows: []RuleRow{row("a", fp(1), nil, "g")}}, "not both"},
		{"is and to", RuleInput{Name: "n", Follows: "f", Rows: []RuleRow{row("a", nil, fp(1), "g")}}, "not both"},
		{"neither", RuleInput{Name: "n", Follows: "f", Rows: []RuleRow{row("  ", nil, nil, "g")}}, "needs the value it matches"},
		{"from after to", RuleInput{Name: "n", Follows: "f", Rows: []RuleRow{row("", fp(5), fp(1), "g")}}, "starts after it ends"},
		{"error names the failing row", RuleInput{Name: "n", Follows: "f", Rows: []RuleRow{row("a", nil, nil, "g"), row("", nil, nil, "g")}}, "row 2"},
	}
	svc := newTestService(&fakeRepo{})
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := svc.CheckRule(tc.in)
			if tc.wantMsg == "" {
				if err != nil {
					t.Fatalf("CheckRule = %v, want nil", err)
				}
				return
			}
			ae := wantAppErr(t, err, http.StatusBadRequest)
			if !strings.Contains(ae.Message, tc.wantMsg) {
				t.Errorf("message %q does not contain %q", ae.Message, tc.wantMsg)
			}
		})
	}
}

func TestSaveRuleCreateTrimsAndReturnsSaved(t *testing.T) {
	repo := &fakeRepo{}
	in := RuleInput{Name: "  Rank ", Follows: " echelon ", Rows: []RuleRow{{Is: " 1 ", Gives: " Iron "}}}
	got, err := newTestService(repo).SaveRule(context.Background(), camp, in, director)
	if err != nil {
		t.Fatalf("SaveRule: %v", err)
	}
	if got.ID == 0 || got.Name != "Rank" || got.Follows != "echelon" || got.Rows[0].Is != "1" || got.Rows[0].Gives != "Iron" {
		t.Errorf("saved = %+v", got)
	}
}

func TestSaveRuleUpdate(t *testing.T) {
	tests := []struct {
		name     string
		existing []Rule
		id       int
		wantCode int
	}{
		{"existing is replaced", []Rule{{ID: 7, Name: "Old"}}, 7, 0},
		{"missing is not found", nil, 7, http.StatusNotFound},
		{"other id is not found", []Rule{{ID: 1, Name: "Old"}}, 7, http.StatusNotFound},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeRepo{rules: tc.existing}
			in := validRule()
			in.ID = tc.id
			got, err := newTestService(repo).SaveRule(context.Background(), camp, in, director)
			if tc.wantCode != 0 {
				wantCode(t, err, tc.wantCode)
				return
			}
			if err != nil || got.Name != "Rank" || got.ID != 7 {
				t.Fatalf("SaveRule = (%+v, %v)", got, err)
			}
		})
	}
}

func TestSaveRuleLimitAndErrors(t *testing.T) {
	ctx := context.Background()
	at := MaxRules
	below := MaxRules - 1

	t.Run("at limit refused", func(t *testing.T) {
		repo := &fakeRepo{rulesCount: &at}
		_, err := newTestService(repo).SaveRule(ctx, camp, validRule(), director)
		wantCode(t, err, http.StatusBadRequest)
	})
	t.Run("below limit allowed", func(t *testing.T) {
		repo := &fakeRepo{rulesCount: &below}
		if _, err := newTestService(repo).SaveRule(ctx, camp, validRule(), director); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("limit does not block updates", func(t *testing.T) {
		repo := &fakeRepo{rulesCount: &at, rules: []Rule{{ID: 3}}}
		in := validRule()
		in.ID = 3
		if _, err := newTestService(repo).SaveRule(ctx, camp, in, director); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("duplicate on create is conflict", func(t *testing.T) {
		_, err := newTestService(&fakeRepo{dupRule: true}).SaveRule(ctx, camp, validRule(), director)
		ae := wantAppErr(t, err, http.StatusConflict)
		if !strings.Contains(ae.Message, "Rank") {
			t.Errorf("message %q should name the rule", ae.Message)
		}
	})
	t.Run("duplicate on update is conflict", func(t *testing.T) {
		in := validRule()
		in.ID = 3
		_, err := newTestService(&fakeRepo{dupRule: true, rules: []Rule{{ID: 3}}}).SaveRule(ctx, camp, in, director)
		wantCode(t, err, http.StatusConflict)
	})
	t.Run("repo failure is internal", func(t *testing.T) {
		_, err := newTestService(&fakeRepo{err: errors.New("boom")}).SaveRule(ctx, camp, validRule(), director)
		wantCode(t, err, http.StatusInternalServerError)
	})
	t.Run("validation runs before the repo", func(t *testing.T) {
		repo := &fakeRepo{}
		_, err := newTestService(repo).SaveRule(ctx, camp, RuleInput{}, director)
		wantCode(t, err, http.StatusBadRequest)
		if len(repo.rules) != 0 {
			t.Error("invalid rule was stored")
		}
	})
}

func TestDeleteRuleAndWord(t *testing.T) {
	ctx := context.Background()
	repo := &fakeRepo{rules: []Rule{{ID: 1}}, words: []LocalWord{{ID: 1}}}
	svc := newTestService(repo)
	if err := svc.DeleteRule(ctx, camp, 1, director); err != nil {
		t.Fatal(err)
	}
	wantCode(t, svc.DeleteRule(ctx, camp, 1, director), http.StatusNotFound)
	if err := svc.DeleteWord(ctx, camp, 1, director); err != nil {
		t.Fatal(err)
	}
	wantCode(t, svc.DeleteWord(ctx, camp, 1, director), http.StatusNotFound)
	bad := newTestService(&fakeRepo{err: errors.New("boom")})
	wantCode(t, bad.DeleteRule(ctx, camp, 1, director), http.StatusInternalServerError)
	wantCode(t, bad.DeleteWord(ctx, camp, 1, director), http.StatusInternalServerError)
}

func TestRuleNamedAndExists(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(&fakeRepo{rules: []Rule{{ID: 4, Name: "Guild Rank"}}})
	if r, err := svc.RuleNamed(ctx, camp, "  guild RANK "); err != nil || r == nil || r.ID != 4 {
		t.Errorf("RuleNamed = (%v, %v)", r, err)
	}
	if r, err := svc.RuleNamed(ctx, camp, "nope"); err != nil || r != nil {
		t.Errorf("RuleNamed miss = (%v, %v)", r, err)
	}
	if !svc.RuleExists(ctx, camp, 4) || svc.RuleExists(ctx, camp, 5) {
		t.Error("RuleExists wrong")
	}
	bad := newTestService(&fakeRepo{err: errors.New("boom")})
	if bad.RuleExists(ctx, camp, 4) {
		t.Error("RuleExists should be false on repo error")
	}
	_, err := bad.RuleNamed(ctx, camp, "x")
	wantCode(t, err, http.StatusInternalServerError)
}

func TestCleanWord(t *testing.T) {
	tests := []struct {
		name    string
		in      WordInput
		wantMsg string
	}{
		{"place ok", WordInput{Word: "coin", Local: "mark", PlaceID: "town"}, ""},
		{"era ok", WordInput{Word: "coin", Local: "mark", EraID: 2}, ""},
		{"neither", WordInput{Word: "coin", Local: "mark"}, "one place or one era"},
		{"both", WordInput{Word: "coin", Local: "mark", PlaceID: "town", EraID: 2}, "one place or one era"},
		{"blank place counts as none", WordInput{Word: "coin", Local: "mark", PlaceID: "  "}, "one place or one era"},
		{"no word", WordInput{Local: "mark", PlaceID: "town"}, "needs the usual word"},
		{"no local", WordInput{Word: "coin", PlaceID: "town"}, "needs the usual word"},
		{"word too long", WordInput{Word: strings.Repeat("x", MaxNameLen+1), Local: "m", PlaceID: "town"}, "at most"},
		{"local too long", WordInput{Word: "c", Local: strings.Repeat("x", MaxNameLen+1), PlaceID: "town"}, "at most"},
		{"unknown place", WordInput{Word: "coin", Local: "mark", PlaceID: "ghost"}, "not a page"},
		{"unknown era", WordInput{Word: "coin", Local: "mark", EraID: 99}, "not on this campaign's calendar"},
	}
	svc := newTestService(&fakeRepo{})
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := svc.CheckWord(context.Background(), camp, tc.in, director)
			if tc.wantMsg == "" {
				if err != nil {
					t.Fatalf("CheckWord = %v", err)
				}
				return
			}
			ae := wantAppErr(t, err, http.StatusBadRequest)
			if !strings.Contains(ae.Message, tc.wantMsg) {
				t.Errorf("message %q does not contain %q", ae.Message, tc.wantMsg)
			}
		})
	}
}

func TestSaveWord(t *testing.T) {
	ctx := context.Background()
	in := WordInput{Word: " coin ", Local: " mark ", PlaceID: "town", DMOnly: true}

	t.Run("create fills place name", func(t *testing.T) {
		repo := &fakeRepo{}
		got, err := newTestService(repo).SaveWord(ctx, camp, in, director)
		if err != nil {
			t.Fatal(err)
		}
		if got.ID == 0 || got.Word != "coin" || got.Local != "mark" || got.PlaceName != "Townsville" || !got.DMOnly {
			t.Errorf("saved = %+v", got)
		}
	})
	t.Run("era word fills era name", func(t *testing.T) {
		got, err := newTestService(&fakeRepo{}).SaveWord(ctx, camp, WordInput{Word: "c", Local: "m", EraID: 2}, director)
		if err != nil || got.EraName != "Age of Ash" {
			t.Fatalf("got (%+v, %v)", got, err)
		}
	})
	t.Run("update existing", func(t *testing.T) {
		repo := &fakeRepo{words: []LocalWord{{ID: 5, Word: "old", Local: "old", PlaceID: "town"}}}
		u := in
		u.ID = 5
		if _, err := newTestService(repo).SaveWord(ctx, camp, u, director); err != nil {
			t.Fatal(err)
		}
		if repo.words[0].Word != "coin" {
			t.Errorf("not updated: %+v", repo.words[0])
		}
	})
	t.Run("update missing is not found", func(t *testing.T) {
		u := in
		u.ID = 5
		_, err := newTestService(&fakeRepo{}).SaveWord(ctx, camp, u, director)
		wantCode(t, err, http.StatusNotFound)
	})
	t.Run("limit", func(t *testing.T) {
		at := MaxWords
		_, err := newTestService(&fakeRepo{wordsCount: &at}).SaveWord(ctx, camp, in, director)
		wantCode(t, err, http.StatusBadRequest)
	})
	t.Run("duplicate create is conflict naming place", func(t *testing.T) {
		_, err := newTestService(&fakeRepo{dupWord: true}).SaveWord(ctx, camp, in, director)
		ae := wantAppErr(t, err, http.StatusConflict)
		if !strings.Contains(ae.Message, "Townsville") {
			t.Errorf("message %q should name the place", ae.Message)
		}
	})
	t.Run("duplicate update is conflict naming era", func(t *testing.T) {
		repo := &fakeRepo{dupWord: true, words: []LocalWord{{ID: 5}}}
		_, err := newTestService(repo).SaveWord(ctx, camp, WordInput{ID: 5, Word: "c", Local: "m", EraID: 2}, director)
		ae := wantAppErr(t, err, http.StatusConflict)
		if !strings.Contains(ae.Message, "Age of Ash") {
			t.Errorf("message %q should name the era", ae.Message)
		}
	})
	t.Run("repo failure is internal", func(t *testing.T) {
		_, err := newTestService(&fakeRepo{err: errors.New("boom")}).SaveWord(ctx, camp, in, director)
		wantCode(t, err, http.StatusInternalServerError)
	})
}

func TestWordsDropsWordsOfRemovedEras(t *testing.T) {
	repo := &fakeRepo{words: []LocalWord{
		{ID: 1, Word: "a", Local: "x", PlaceID: "town"},
		{ID: 2, Word: "b", Local: "y", EraID: 2},
		{ID: 3, Word: "c", Local: "z", EraID: 99},
	}}
	got, err := newTestService(repo).Words(context.Background(), camp, director)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[1].EraName != "Age of Ash" {
		t.Errorf("Words = %+v", got)
	}
	_, err = newTestService(&fakeRepo{err: errors.New("boom")}).Words(context.Background(), camp, director)
	wantCode(t, err, http.StatusInternalServerError)
}

func TestEvaluate(t *testing.T) {
	ctx := context.Background()
	repo := &fakeRepo{rules: []Rule{
		{ID: 1, Follows: "echelon", Rows: []RuleRow{{Is: "1", Gives: "Iron"}, {From: fp(2), Gives: "Steel"}}},
		{ID: 2, Follows: "level", Rows: []RuleRow{{To: fp(5), Gives: "Novice"}}},
	}}
	svc := newTestService(repo)
	data := map[string]any{"echelon": 3.0, "level": 9.0}

	t.Run("only requested ids", func(t *testing.T) {
		got := svc.Evaluate(ctx, camp, []int{1}, data)
		if len(got) != 1 || got[1] != "Steel" {
			t.Errorf("got %v", got)
		}
	})
	t.Run("matches nothing gives empty", func(t *testing.T) {
		got := svc.Evaluate(ctx, camp, []int{1, 2}, data)
		if got[1] != "Steel" || got[2] != "" || len(got) != 2 {
			t.Errorf("got %v", got)
		}
	})
	t.Run("missing field gives empty", func(t *testing.T) {
		got := svc.Evaluate(ctx, camp, []int{1}, map[string]any{})
		if got[1] != "" {
			t.Errorf("got %v", got)
		}
	})
	t.Run("unknown id is absent", func(t *testing.T) {
		if got := svc.Evaluate(ctx, camp, []int{42}, data); len(got) != 0 {
			t.Errorf("got %v", got)
		}
	})
	t.Run("no ids gives empty non-nil map", func(t *testing.T) {
		if got := svc.Evaluate(ctx, camp, nil, data); got == nil || len(got) != 0 {
			t.Errorf("got %v", got)
		}
	})
	t.Run("repo error degrades to empty map", func(t *testing.T) {
		bad := newTestService(&fakeRepo{err: errors.New("boom")})
		if got := bad.Evaluate(ctx, camp, []int{1}, data); got == nil || len(got) != 0 {
			t.Errorf("got %v", got)
		}
	})
}

func TestWordsFor(t *testing.T) {
	ctx := context.Background()
	words := []LocalWord{
		{ID: 1, Word: "Coin", Local: "mark", PlaceID: "town"},
		{ID: 2, Word: "Rank", Local: "grade", EraID: 2, DMOnly: true},
	}
	world := &fakeWorld{chain: []string{"town"}, era: 2}
	svc := NewService(&fakeRepo{words: words}, world)

	if got := svc.WordsFor(ctx, camp, "page", player); got["coin"] != "mark" || len(got) != 1 {
		t.Errorf("player got %v", got)
	}
	if got := svc.WordsFor(ctx, camp, "page", director); got["rank"] != "grade" || len(got) != 2 {
		t.Errorf("director got %v", got)
	}
	if got := svc.WordsFor(ctx, camp, "", player); len(got) != 0 {
		t.Errorf("no page should skip place words, got %v", got)
	}
	if got := NewService(&fakeRepo{}, world).WordsFor(ctx, camp, "page", player); got != nil {
		t.Errorf("no words = %v, want nil", got)
	}
	if got := NewService(&fakeRepo{err: errors.New("boom")}, world).WordsFor(ctx, camp, "page", player); got != nil {
		t.Errorf("repo error = %v, want nil", got)
	}
}
