package homebrew

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

const (
	secretText = "the vault key is under the third stone"
	notesText  = "remember to nerf this before session nine"
)

// shareTestKind is a part whose owner keeps Director-only words and a link to
// a deck of the campaign by id.
func shareTestRegistry(t *testing.T) *Registry {
	t.Helper()
	reg := testRegistry(t)
	check := func(_ context.Context, campaignID string, raw json.RawMessage) (json.RawMessage, error) {
		var d struct {
			Name   string `json:"name"`
			Secret string `json:"secret,omitempty"`
			DeckID string `json:"deckId,omitempty"`
		}
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&d); err != nil {
			return nil, apperror.NewValidation("has a setting it doesn't recognise")
		}
		// A deck id is only good in the campaign it came from.
		if d.DeckID != "" && !strings.HasPrefix(d.DeckID, campaignID+":") {
			return nil, apperror.NewValidation("points at a deck that isn't in this campaign")
		}
		return json.Marshal(d)
	}
	share := func(raw json.RawMessage, waiting []Unlinked) (json.RawMessage, []Unlinked, error) {
		var d map[string]any
		if err := json.Unmarshal(raw, &d); err != nil {
			return nil, nil, apperror.NewValidation("can't be read")
		}
		var picks []Unlinked
		delete(d, "secret")
		if _, ok := d["deckId"]; ok {
			delete(d, "deckId")
			picks = append(picks, Unlinked{Pick: "a deck"})
		} else {
			// A marker the sender wrote stays only when the owner would have
			// made it: the data really has no deck.
			for _, w := range waiting {
				if w.Pick == "a deck" {
					picks = append(picks, w)
				}
			}
		}
		out, _ := json.Marshal(d)
		return out, picks, nil
	}
	if err := reg.RegisterPart(PartKind{Name: "keeper", Label: "Keeper", Check: check, Share: share}); err != nil {
		t.Fatal(err)
	}
	if err := reg.RegisterAction(ActionDef{Name: "browser-step", ClientRun: true, Check: okCheck}); err != nil {
		t.Fatal(err)
	}
	// whisper is an action whose args can be for the Director only; such a
	// step is refused, not shared.
	if err := reg.RegisterAction(ActionDef{Name: "whisper", Check: okCheck,
		Share: func(raw json.RawMessage) (json.RawMessage, error) {
			var a struct {
				See string `json:"see"`
			}
			_ = json.Unmarshal(raw, &a)
			if a.See == "director" {
				return nil, apperror.NewValidation("is only for the Director, so it is not shared")
			}
			return raw, nil
		}}); err != nil {
		t.Fatal(err)
	}
	reg.SetSystem(func(_ context.Context, campaignID string) string {
		if campaignID == "camp-sys" {
			return "drawsteel"
		}
		return ""
	})
	return reg
}

func shareService(t *testing.T) (Service, *fakeRepo) {
	t.Helper()
	repo := newFakeRepo()
	svc := NewService(repo, shareTestRegistry(t), nil, nil).(*service)
	n := 0
	svc.newID = func() string { n++; return fmt.Sprintf("rcp-%d", n) }
	return svc, repo
}

// seedRecipe stores a recipe whose Director-only text and deck link must not
// travel.
func seedRecipe(t *testing.T, svc Service, campaign, name string) string {
	t.Helper()
	r, err := svc.Create(context.Background(), campaign, dm, RecipeInput{Name: name, Doc: Doc{
		Parts: []Part{
			{Key: "k", Kind: "keeper", Label: "Keeper", See: SeeParty,
				Data: raw(fmt.Sprintf(`{"name":"Hunger","secret":%q,"deckId":%q}`, secretText, campaign+":deck-9"))},
			w("a"),
		},
		Rules:  []Rule{{When: When{Event: "moon"}, Timing: TimingHeld, Then: []Do{{Action: "ping"}, {Action: "bump", Part: "a"}}}},
		Layout: map[string]Pos{"p:k": {X: 10, Y: 20}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	return r.ID
}

func TestPackageLeavesOutDirectorText(t *testing.T) {
	svc, repo := shareService(t)
	ctx := context.Background()
	id := seedRecipe(t, svc, camp, "Hunger")
	// A notes field is not part of a recipe, but a stored doc may carry stray
	// keys from an older shape; none may reach the package.
	rec := repo.recipes[id]
	rec.Doc.Rules[0].When.Test = "does it feel tense?"
	repo.recipes[id] = rec

	pkg, err := svc.Package(ctx, camp, id, dm)
	if err != nil {
		t.Fatal(err)
	}
	for _, banned := range []string{secretText, notesText, camp + ":deck-9", "deck-9", "deckId", `"secret"`} {
		if bytes.Contains(pkg, []byte(banned)) {
			t.Errorf("package holds %q: %s", banned, pkg)
		}
	}
	var p packagedRecipe
	if err := json.Unmarshal(pkg, &p); err != nil {
		t.Fatal(err)
	}
	if got := p.Doc.Parts[0].Unlinked; len(got) != 1 || got[0].Pick != "a deck" {
		t.Errorf("unlinked = %+v", got)
	}
	if string(p.Doc.Parts[0].Data) != `{"name":"Hunger"}` {
		t.Errorf("data = %s", p.Doc.Parts[0].Data)
	}
	wantNeeds := PackageNeeds{PartKinds: []string{"keeper", "widget"}, Events: []string{"moon"}, Actions: []string{"bump", "ping"}}
	if fmt.Sprint(p.Needs) != fmt.Sprint(wantNeeds) {
		t.Errorf("needs = %+v, want %+v", p.Needs, wantNeeds)
	}
	if p.Doc.Layout["p:k"] != (Pos{10, 20}) {
		t.Errorf("layout lost: %+v", p.Doc.Layout)
	}
	// The recipe itself was not edited by packaging it.
	if !strings.Contains(string(repo.recipes[id].Doc.Parts[0].Data), secretText) {
		t.Error("packaging changed the stored recipe")
	}
}

func TestPackageStripDropsEveryUnknownField(t *testing.T) {
	svc, _ := shareService(t)
	in := fmt.Sprintf(`{"name":"X","notes":%q,"doc":{"off":false,"notes":%q,"parts":[{"key":"k","kind":"keeper","label":"K","see":"party","notes":%q,
		"data":{"name":"n","secret":%q,"deckId":"c:1"}}],"rules":[]},"needs":{"partKinds":["x"],"mystery":1}}`,
		notesText, notesText, notesText, secretText)
	out, err := svc.StripPackage(raw(in))
	if err != nil {
		t.Fatal(err)
	}
	for _, banned := range []string{secretText, notesText, "deckId", "mystery", `"c:1"`} {
		if bytes.Contains(out, []byte(banned)) {
			t.Errorf("stripped package holds %q: %s", banned, out)
		}
	}
	// Stripping twice changes nothing, and what it leaves validates.
	again, err := svc.StripPackage(out)
	if err != nil || !bytes.Equal(again, out) {
		t.Errorf("not idempotent: %s vs %s (%v)", again, out, err)
	}
	if _, err := svc.ValidatePackage(out); err != nil {
		t.Errorf("stripped package refused: %v", err)
	}
}

func TestValidatePackageRefuses(t *testing.T) {
	svc, _ := shareService(t)
	good := `{"name":"X","doc":{"parts":[{"key":"a","kind":"widget","label":"A","see":"party"}],"rules":[{"when":{"event":"moon"},"timing":"now","then":[{"action":"ping"}]}]},"needs":{"partKinds":[],"events":[],"actions":[]}}`
	if _, err := svc.ValidatePackage(raw(good)); err != nil {
		t.Fatalf("good package refused: %v", err)
	}
	huge := `{"name":"X","doc":{"parts":[],"rules":[]},"pad":"` + strings.Repeat("a", MaxBodyBytes) + `"}`
	bigData := `{"name":"X","doc":{"parts":[{"key":"a","kind":"widget","label":"A","data":{"x":"` + strings.Repeat("a", MaxDataBytes) + `"}}],"rules":[]}}`
	tests := []struct {
		name, in, want string
	}{
		{"not JSON", `nope`, "can read"},
		{"two documents", good + good, "can read"},
		{"unknown top field", strings.Replace(good, `"needs"`, `"notes":"hi","needs"`, 1), "can read"},
		{"unknown part field", strings.Replace(good, `"see":"party"`, `"see":"party","notes":"hi"`, 1), "can read"},
		{"off in the doc", strings.Replace(good, `"parts"`, `"off":false,"parts"`, 1), "can read"},
		{"oversized", huge, "too large"},
		{"oversized data", bigData, "too large"},
		{"data not an object", strings.Replace(good, `"see":"party"`, `"see":"party","data":[1]`, 1), "not a JSON object"},
		{"bad key", strings.Replace(good, `"key":"a"`, `"key":"A b"`, 1), "needs a key"},
		{"bad name", strings.Replace(good, `"name":"X"`, `"name":""`, 1), "name is required"},
		{"rule names a missing part", strings.Replace(good, `{"action":"ping"}`, `{"action":"bump","part":"zz"}`, 1), "doesn't have"},
		{"hostile kind name", strings.Replace(good, `"kind":"widget"`, `"kind":"<script>"`, 1), "doesn't have"},
		{"no steps", strings.Replace(good, `"then":[{"action":"ping"}]`, `"then":[]`, 1), "to do"},
		{"carries a secret", `{"name":"X","doc":{"parts":[{"key":"k","kind":"keeper","label":"K","data":{"secret":"s"}}],"rules":[]}}`, "Director-only"},
		{"carries an id link", `{"name":"X","doc":{"parts":[{"key":"k","kind":"keeper","label":"K","data":{"deckId":"c:1"}}],"rules":[]}}`, "Director-only"},
		{"too many markers", `{"name":"X","doc":{"parts":[{"key":"k","kind":"widget","label":"K","unlinked":[` +
			strings.TrimSuffix(strings.Repeat(`{"pick":"a deck"},`, 0), ",") + `{"pick":"1"},{"pick":"2"},{"pick":"3"},{"pick":"4"},{"pick":"5"},{"pick":"6"},{"pick":"7"},{"pick":"8"},{"pick":"9"},{"pick":"10"},{"pick":"11"}]}],"rules":[]}}`, "more than"},
		{"control character in a marker", `{"name":"X","doc":{"parts":[{"key":"k","kind":"widget","label":"K","unlinked":[{"pick":"a\u0007deck"}]}],"rules":[]}}`, "not allowed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.ValidatePackage(raw(tt.in))
			if err == nil {
				t.Fatal("accepted")
			}
			if codeOf(err) != 400 && codeOf(err) != 422 {
				t.Errorf("code = %d: %v", codeOf(err), err)
			}
			if !strings.Contains(msgOf(err), tt.want) {
				t.Errorf("message = %q, want it to contain %q", msgOf(err), tt.want)
			}
		})
	}
}

func TestPackageLacks(t *testing.T) {
	svc, _ := shareService(t)
	ctx := context.Background()
	pkgOf := func(parts, rules string) json.RawMessage {
		return raw(`{"name":"X","doc":{"parts":[` + parts + `],"rules":[` + rules + `]}}`)
	}
	tests := []struct {
		name     string
		campaign string
		pkg      json.RawMessage
		want     []string // each must be in some sentence
	}{
		{"nothing missing", camp, pkgOf(`{"key":"a","kind":"widget","label":"A"}`, `{"when":{"event":"moon"},"then":[{"action":"ping"}]}`), nil},
		{"unknown kind event and action", camp, pkgOf(`{"key":"a","kind":"far-off","label":"A"}`, `{"when":{"event":"far.event"},"then":[{"action":"far.action"}]}`),
			[]string{`"far-off" kind of part`, `"far.event" When`, `"far.action" Then`}},
		{"browser step needs a system", camp, raw(`{"name":"X","doc":{"parts":[],"rules":[{"when":{"event":"moon"},"then":[{"action":"browser-step"}]}]},"needs":{"system":"drawsteel"}}`),
			[]string{"package of the \"drawsteel\" game system", "no game system"}},
		{"same system is fine", "camp-sys", raw(`{"name":"X","doc":{"parts":[],"rules":[{"when":{"event":"moon"},"then":[{"action":"browser-step"}]}]},"needs":{"system":"drawsteel"}}`), nil},
		{"an owner refuses in this campaign", camp, pkgOf(`{"key":"p","kind":"picky","label":"P"}`, ``), []string{"won't fit this campaign", "bad thing"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := svc.PackageLacks(ctx, tt.campaign, tt.pkg)
			if err != nil {
				t.Fatal(err)
			}
			if len(tt.want) == 0 && len(got) != 0 {
				t.Fatalf("lacks = %v", got)
			}
			all := strings.Join(got, "\n")
			for _, w := range tt.want {
				if !strings.Contains(all, w) {
					t.Errorf("lacks %q missing %q", all, w)
				}
			}
			for _, s := range got {
				if !strings.HasSuffix(s, ".") {
					t.Errorf("not a sentence: %q", s)
				}
			}
		})
	}
	if _, err := svc.PackageLacks(ctx, camp, raw(`{"junk":1}`)); err == nil {
		t.Error("a hostile package was described, not refused")
	}
}

func TestPackageRoundTrip(t *testing.T) {
	svc, repo := shareService(t)
	ctx := context.Background()
	id := seedRecipe(t, svc, camp, "Hunger")
	pkg, err := svc.Package(ctx, camp, id, dm)
	if err != nil {
		t.Fatal(err)
	}
	canon, err := svc.ValidatePackage(pkg)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(canon, pkg) {
		t.Errorf("a package this server wrote is not its own canonical form:\n%s\n%s", pkg, canon)
	}
	lacks, err := svc.PackageLacks(ctx, "camp-2", canon)
	if err != nil || len(lacks) != 0 {
		t.Fatalf("lacks = %v, %v", lacks, err)
	}

	other := Viewer{UserID: "dm-2", Director: true}
	added, err := svc.AddPackaged(ctx, "camp-2", other, canon)
	if err != nil {
		t.Fatal(err)
	}
	if len(added.LeftToPoint) != 1 || added.LeftToPoint[0] != "Keeper: choose a deck." {
		t.Errorf("left to point = %v", added.LeftToPoint)
	}
	got := repo.recipes[added.ID]
	if got.CampaignID != "camp-2" || got.Name != "Hunger" || got.CreatedBy != "dm-2" {
		t.Errorf("added = %+v", got)
	}
	if !got.Doc.Off {
		t.Error("an added recipe is switched on")
	}
	if len(got.Doc.Parts[0].Unlinked) != 1 || got.Doc.Parts[0].Unlinked[0].Pick != "a deck" {
		t.Errorf("marker lost: %+v", got.Doc.Parts[0])
	}
	if len(got.Doc.Rules) != 1 || len(got.Doc.Rules[0].Then) != 2 || got.Doc.Layout["p:k"] != (Pos{10, 20}) {
		t.Errorf("recipe changed on the way: %+v", got.Doc)
	}
	stored, _ := json.Marshal(got.Doc)
	for _, banned := range []string{secretText, "deck-9"} {
		if bytes.Contains(stored, []byte(banned)) {
			t.Errorf("stored copy holds %q", banned)
		}
	}
	// Adding again keeps both: the name is numbered, not refused.
	again, err := svc.AddPackaged(ctx, "camp-2", other, canon)
	if err != nil || repo.recipes[again.ID].Name != "Hunger (2)" {
		t.Errorf("second add = %+v, %v", repo.recipes[again.ID], err)
	}
	// The source campaign is untouched.
	if len(repo.recipes) != 3 {
		t.Errorf("recipes = %d", len(repo.recipes))
	}
}

func TestAddedRecipeStaysOffUntilSaved(t *testing.T) {
	svc, repo := shareService(t)
	ctx := context.Background()
	id := seedRecipe(t, svc, camp, "Hunger")
	pkg, _ := svc.Package(ctx, camp, id, dm)
	added, err := svc.AddPackaged(ctx, "camp-2", dm, pkg)
	if err != nil {
		t.Fatal(err)
	}
	// Its rule does not fire while it is off.
	if err := svc.Fire(ctx, "camp-2", dm, Fired{Event: "moon"}); err != nil {
		t.Fatal(err)
	}
	if rs, _ := svcRecipesFor(svc, ctx, "camp-2", ""); len(rs) != 0 {
		t.Errorf("an off recipe is running: %d", len(rs))
	}
	if rs, _ := svcRecipesFor(svc, ctx, "camp-2", added.ID); len(rs) != 0 {
		t.Errorf("an off recipe is running by id: %d", len(rs))
	}
	// A save is refused while a part is unlinked, so it can't be switched on
	// with a half-made part.
	cur := repo.recipes[added.ID]
	_, err = svc.Update(ctx, "camp-2", added.ID, dm, RecipeInput{Name: cur.Name, Doc: cur.Doc, Version: cur.Version})
	if err == nil || !strings.Contains(msgOf(err), "still has a deck to choose") {
		t.Errorf("save while unlinked: %v", err)
	}
	// Once the link is chosen the save switches it on.
	cur.Doc.Parts[0].Unlinked = nil
	cur.Doc.Parts[0].Data = raw(fmt.Sprintf(`{"name":"Hunger","deckId":%q}`, "camp-2:deck-1"))
	if _, err := svc.Update(ctx, "camp-2", added.ID, dm, RecipeInput{Name: cur.Name, Doc: cur.Doc, Version: cur.Version}); err != nil {
		t.Fatal(err)
	}
	if repo.recipes[added.ID].Doc.Off {
		t.Error("the recipe is still off after a good save")
	}
	if rs, _ := svcRecipesFor(svc, ctx, "camp-2", ""); len(rs) != 1 {
		t.Errorf("a switched-on recipe is not running: %d", len(rs))
	}
}

func svcRecipesFor(s Service, ctx context.Context, campaignID, id string) ([]Recipe, error) {
	return s.(*service).recipesFor(ctx, campaignID, id)
}

func TestPackageDirectorOnly(t *testing.T) {
	svc, repo := shareService(t)
	ctx := context.Background()
	id := seedRecipe(t, svc, camp, "Hunger")
	pkg, err := svc.Package(ctx, camp, id, dm)
	if err != nil {
		t.Fatal(err)
	}
	before := len(repo.recipes)
	for _, who := range []Viewer{player, {}, {UserID: "x", VisibilityRole: 2}} {
		if _, err := svc.Package(ctx, camp, id, who); codeOf(err) != 403 {
			t.Errorf("%+v packaged: %v", who, err)
		}
		if _, err := svc.AddPackaged(ctx, "camp-2", who, pkg); codeOf(err) != 403 {
			t.Errorf("%+v added: %v", who, err)
		}
	}
	if len(repo.recipes) != before {
		t.Error("a refused add stored a recipe")
	}
}

func TestAddPackagedRefuses(t *testing.T) {
	svc, repo := shareService(t)
	ctx := context.Background()
	id := seedRecipe(t, svc, camp, "Hunger")
	good, _ := svc.Package(ctx, camp, id, dm)
	before := len(repo.recipes)
	tests := []struct {
		name string
		in   json.RawMessage
		want string
	}{
		{"not JSON", raw(`{`), "can read"},
		{"a kind this campaign lacks", raw(`{"name":"X","doc":{"parts":[{"key":"a","kind":"far-off","label":"A"}],"rules":[]}}`), "doesn't have"},
		{"a when this campaign lacks", raw(`{"name":"X","doc":{"parts":[],"rules":[{"when":{"event":"far.event"},"then":[{"action":"ping"}]}]}}`), "doesn't have"},
		{"an owner refuses", raw(`{"name":"X","doc":{"parts":[{"key":"p","kind":"picky","label":"P"}],"rules":[]}}`), "bad thing"},
		// A sender who writes the marker themselves to dodge the owner's
		// check still can't turn the recipe on: it stays off and unsaveable.
		{"too large", raw(`{"name":"X","doc":{"parts":[],"rules":[]},"pad":"` + strings.Repeat("a", MaxBodyBytes) + `"}`), "too large"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.AddPackaged(ctx, "camp-2", dm, tt.in)
			if err == nil || !strings.Contains(msgOf(err), tt.want) {
				t.Errorf("err = %v, want %q", err, tt.want)
			}
		})
	}
	if len(repo.recipes) != before {
		t.Error("a refused add stored a recipe")
	}

	// Hostile data in a part that kept its Director-only text and real id is
	// stripped on add, not trusted.
	hostile := fmt.Sprintf(`{"name":"Sneaky","doc":{"parts":[{"key":"k","kind":"keeper","label":"K","data":{"name":"n","secret":%q,"deckId":"camp-1:deck-1"}}],"rules":[]}}`, secretText)
	added, err := svc.AddPackaged(ctx, "camp-2", dm, raw(hostile))
	if err != nil {
		t.Fatal(err)
	}
	stored, _ := json.Marshal(repo.recipes[added.ID].Doc)
	if bytes.Contains(stored, []byte(secretText)) || bytes.Contains(stored, []byte("deck-1")) {
		t.Errorf("hostile content was stored: %s", stored)
	}
	if len(added.LeftToPoint) != 1 {
		t.Errorf("left to point = %v", added.LeftToPoint)
	}
	// The package a good sender made still adds after all that.
	if _, err := svc.AddPackaged(ctx, "camp-2", dm, good); err != nil {
		t.Errorf("good package refused: %v", err)
	}
}

func TestAddPackagedLimits(t *testing.T) {
	svc, repo := shareService(t)
	fill(repo, MaxRecipes)
	_, err := svc.AddPackaged(context.Background(), camp, dm, raw(`{"name":"X","doc":{"parts":[],"rules":[]}}`))
	if err == nil || !strings.Contains(msgOf(err), "at most") {
		t.Errorf("err = %v", err)
	}
}

func TestSenderMarkersAreNeverTrusted(t *testing.T) {
	svc, repo := shareService(t)
	ctx := context.Background()
	// The owner's Check is strict, and a marker used to exempt a part from it.
	reviewer := `{"name":"Sneaky","doc":{"parts":[{"key":"k","kind":"keeper","label":"K","data":{"bogus":"zzz","name":"x"},"unlinked":[{"pick":"a deck"}]}],"rules":[]}}`
	if _, err := svc.AddPackaged(ctx, "camp-2", dm, raw(reviewer)); err == nil || !strings.Contains(msgOf(err), "recognise") {
		t.Errorf("a part with a marker skipped the owner's Check: %v", err)
	}
	// A marker the owner would not make is dropped, and the data checked.
	made := `{"name":"Fine","doc":{"parts":[{"key":"k","kind":"keeper","label":"K","data":{"name":"x","deckId":"camp-2:d"},"unlinked":[{"pick":"anything"}]}],"rules":[]}}`
	added, err := svc.AddPackaged(ctx, "camp-2", dm, raw(made))
	if err != nil {
		t.Fatal(err)
	}
	got := repo.recipes[added.ID].Doc.Parts[0]
	if len(got.Unlinked) != 1 || got.Unlinked[0].Pick != "a deck" {
		t.Errorf("markers = %+v, want only the owner's", got.Unlinked)
	}
	// A kind with no Share has no markers to give.
	noShare := `{"name":"X","doc":{"parts":[{"key":"a","kind":"widget","label":"A","data":{"loose":1},"unlinked":[{"pick":"anything"}]}],"rules":[]}}`
	if _, err := svc.StripPackage(raw(noShare)); err == nil || !strings.Contains(msgOf(err), "can't have a link") {
		t.Errorf("strip kept a sender marker on a plain kind: %v", err)
	}
	if _, err := svc.ValidatePackage(raw(noShare)); err == nil {
		t.Error("validate accepted a sender marker on a plain kind")
	}
	if _, err := svc.AddPackaged(ctx, "camp-2", dm, raw(noShare)); err == nil {
		t.Error("add accepted a sender marker on a plain kind")
	}
	// Validate also refuses a marker its owner would not have made.
	forged := `{"name":"X","doc":{"parts":[{"key":"k","kind":"keeper","label":"K","data":{"name":"x"},"unlinked":[{"pick":"a vault"}]}],"rules":[]}}`
	if _, err := svc.ValidatePackage(raw(forged)); err == nil {
		t.Error("validate accepted a forged marker")
	}
}

func TestStepArgsAreShared(t *testing.T) {
	svc, repo := shareService(t)
	ctx := context.Background()
	mk := func(see string) RecipeInput {
		return RecipeInput{Name: "Whisper " + see, Doc: Doc{
			Parts: []Part{w("a")},
			Rules: []Rule{{When: When{Event: "moon"}, Timing: TimingNow,
				Then: []Do{{Action: "whisper", Args: raw(`{"name":"Eclipse","text":"` + secretText + `","see":"` + see + `"}`)}}}},
		}}
	}
	dirOnly, err := svc.Create(ctx, camp, dm, mk("director"))
	if err != nil {
		t.Fatal(err)
	}
	// A Director-only step is refused on share, and the text is nowhere in
	// what came back.
	pkg, err := svc.Package(ctx, camp, dirOnly.ID, dm)
	if err == nil || bytes.Contains(pkg, []byte(secretText)) {
		t.Fatalf("a Director-only step was packaged: %s, %v", pkg, err)
	}
	if !strings.Contains(msgOf(err), "only for the Director") {
		t.Errorf("message = %q", msgOf(err))
	}
	// A package that carries one anyway is refused by Validate, Strip and Add.
	hostile := `{"name":"H","doc":{"parts":[],"rules":[{"when":{"event":"moon"},"then":[{"action":"whisper","args":{"see":"director","text":"` + secretText + `"}}]}]}}`
	for name, call := range map[string]func() error{
		"validate": func() error { _, err := svc.ValidatePackage(raw(hostile)); return err },
		"strip":    func() error { _, err := svc.StripPackage(raw(hostile)); return err },
		"add":      func() error { _, err := svc.AddPackaged(ctx, "camp-2", dm, raw(hostile)); return err },
	} {
		if err := call(); err == nil || !strings.Contains(msgOf(err), "only for the Director") {
			t.Errorf("%s: %v", name, err)
		}
	}
	// A party-visible step travels.
	open, err := svc.Create(ctx, camp, dm, mk("party"))
	if err != nil {
		t.Fatal(err)
	}
	before := len(repo.recipes)
	pkg, err = svc.Package(ctx, camp, open.ID, dm)
	if err != nil || !bytes.Contains(pkg, []byte("Eclipse")) {
		t.Errorf("a party step was not packaged: %s, %v", pkg, err)
	}
	if len(repo.recipes) != before {
		t.Error("packaging stored a recipe")
	}
}

func TestOffRecipesAreListedAndNotFound(t *testing.T) {
	svc, _ := shareService(t)
	ctx := context.Background()
	id := seedRecipe(t, svc, camp, "Hunger")
	pkg, _ := svc.Package(ctx, camp, id, dm)
	if _, err := svc.AddPackaged(ctx, "camp-2", dm, pkg); err != nil {
		t.Fatal(err)
	}
	list, err := svc.List(ctx, "camp-2", dm)
	if err != nil || len(list) != 1 || !list[0].Off {
		t.Fatalf("list = %+v, %v", list, err)
	}
	if got, _ := svc.PartsOfKind(ctx, "camp-2", dm, "", "keeper"); len(got) != 0 {
		t.Errorf("an off recipe's parts were found: %v", got)
	}
	if got, _ := svc.PartsOfKind(ctx, camp, dm, "", "keeper"); len(got) != 1 {
		t.Errorf("a live recipe's parts were not found: %v", got)
	}
}
