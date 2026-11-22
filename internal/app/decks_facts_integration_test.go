package app

// decks_facts_integration_test.go runs the decks Facts adapter, the watcher and
// the hooks that poke it over the real entities, tags, trackers and
// transformations services on a scratch database. Skipped under -short and
// without a database.

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"sync"
	"testing"
	"time"

	"github.com/keyxmakerx/chronicle/internal/database"
	"github.com/keyxmakerx/chronicle/internal/database/dbtest"
	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/decks"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/plugins/trackers"
	"github.com/keyxmakerx/chronicle/internal/plugins/transformations"
	"github.com/keyxmakerx/chronicle/internal/widgets/tags"
)

type factsWorld struct {
	t               *testing.T
	camp, other     string
	owner, tam      string
	hero, npc, away string
	entities        entities.EntityService
	tags            tags.TagService
	trackers        trackers.Service
	layers          transformations.Service
	decks           decks.Service
	moments         decks.MomentRepository
	facts           *decksFactsAdapter
	watch           *decksWatcher
	dir             decks.Viewer
	exec            func(q string, args ...any)
	heroTypeID      int
}

type typesOf struct{ ids []int }

func (c typesOf) CharacterTypeIDs(context.Context, string) ([]int, error) { return c.ids, nil }

const heroFields = `[
 {"key":"hp","label":"Hit points","type":"number","play":{"edit":"owner","kind":"resource","max_field":"hpmax"}},
 {"key":"hpmax","label":"Max hit points","type":"number"},
 {"key":"stamina","label":"Stamina","type":"number"},
 {"key":"motto","label":"Motto","type":"text"}
]`

func newFactsWorld(t *testing.T) *factsWorld {
	t.Helper()
	if testing.Short() {
		t.Skip("DB integration test")
	}
	plugin := func(slug string, fsys fs.FS) dbtest.PluginMigrations {
		sub, err := fs.Sub(fsys, database.PluginMigrationsSubdir)
		if err != nil {
			t.Fatal(err)
		}
		return dbtest.PluginMigrations{Slug: slug, FS: sub}
	}
	db := dbtest.Open(t, dbtest.Options{Prefix: "chronicle_deckfacts_", Plugins: []dbtest.PluginMigrations{
		plugin(decks.PluginSlug, decks.MigrationsFS), plugin(trackers.PluginSlug, trackers.MigrationsFS),
		plugin(transformations.PluginSlug, transformations.MigrationsFS)}})
	w := &factsWorld{t: t, exec: func(q string, args ...any) { t.Helper(); mustGalleryExec(t, db, q, args...) }}
	w.camp, w.other = galleryTestUUID(t), galleryTestUUID(t)
	w.owner, w.tam = galleryTestUUID(t), galleryTestUUID(t)
	for _, u := range []string{w.owner, w.tam} {
		w.exec(`INSERT INTO users (id, email, display_name, password_hash) VALUES (?, ?, 'U', 'x')`, u, u+"@example.test")
	}
	for _, c := range []string{w.camp, w.other} {
		w.exec(`INSERT INTO campaigns (id, name, slug, created_by) VALUES (?, ?, ?, ?)`, c, "C"+c[:4], "c-"+c[:8], w.owner)
		w.exec(`INSERT INTO entity_types (campaign_id, slug, name, name_plural, fields) VALUES (?, 'hero', 'Hero', 'Heroes', ?)`, c, heroFields)
	}
	w.exec(`INSERT INTO campaign_members (campaign_id, user_id, role) VALUES (?, ?, 'owner'), (?, ?, 'player')`, w.camp, w.owner, w.camp, w.tam)
	if err := db.QueryRow(`SELECT id FROM entity_types WHERE campaign_id = ? AND slug = 'hero'`, w.camp).Scan(&w.heroTypeID); err != nil {
		t.Fatal(err)
	}
	page := func(camp, name string, claimed bool, fields map[string]any) string {
		id := galleryTestUUID(t)
		raw, _ := json.Marshal(fields)
		owner, playable := any(nil), false
		if claimed {
			owner, playable = w.tam, true
		}
		w.exec(`INSERT INTO entities (id, campaign_id, entity_type_id, name, slug, is_private, visibility, created_by, owner_user_id, playable, fields_data, created_at, updated_at)
			VALUES (?, ?, (SELECT id FROM entity_types WHERE campaign_id = ? AND slug = 'hero'), ?, ?, false, 'default', ?, ?, ?, ?, NOW(), NOW())`,
			id, camp, camp, name, "p-"+id[:8], w.owner, owner, playable, string(raw))
		return id
	}
	w.hero = page(w.camp, "Tam", true, map[string]any{"hp": 5, "hpmax": 20, "motto": "Onward"})
	w.npc = page(w.camp, "Nobody", false, map[string]any{})
	w.away = page(w.other, "Elsewhere", false, map[string]any{"hp": 1})

	w.entities = entities.NewEntityService(entities.NewEntityRepository(db), entities.NewEntityTypeRepository(db), entities.NewEntityPermissionRepository(db))
	campSvc := campaigns.NewCampaignService(campaigns.NewCampaignRepository(db), trashUserFinder{}, nil, nil, "http://localhost")
	cards := entities.NewPageCards(db)
	w.tags = tags.NewTagService(tags.NewTagRepository(db))
	w.watch = newDecksWatcher()
	w.watch.delay = 20 * time.Millisecond
	w.trackers = trackers.NewService(trackers.NewRepository(db), &trackersEntityAdapter{svc: w.entities, cards: cards}, &trackersMemberAdapter{svc: campSvc}, nil)
	pages := &encountersPageAdapter{svc: w.entities, cards: cards}
	w.layers = transformations.NewService(transformations.NewRepository(db), &transformationsPageAdapter{pages: pages},
		transformations.WithNotifier(decksLayersNotifier{next: noLayersNotifier{}, poke: w.watch.Poke}))
	w.facts = &decksFactsAdapter{entities: w.entities, characters: typesOf{ids: []int{w.heroTypeID}}, tags: w.tags, trackers: w.trackers, layers: w.layers}

	w.entities.SetEventPublisher(&decksEntityEvents{next: entities.NoopEntityEventPublisher{}, poke: w.watch.Poke})
	w.tags.SetHookDispatcher(decksTagHooks{next: noTagHooks{}, poke: w.watch.Poke})
	w.trackers.(interface{ SetChangeObserver(trackers.ChangeObserver) }).SetChangeObserver(decksTrackerObserver{poke: w.watch.Poke})

	w.moments = decks.NewMomentRepository(db)
	w.decks = decks.NewService(decks.NewRepository(db), &decksHeroAdapter{svc: w.entities}, &decksMemberAdapter{svc: campSvc},
		decks.WithFacts(w.facts), decks.WithMoments(w.moments), decks.WithPoke(w.watch.Poke))
	w.watch.bind(w.decks)
	w.dir = decks.Viewer{UserID: w.owner, MemberRole: 3, VisibilityRole: 3, Director: true}
	return w
}

type noLayersNotifier struct{}

func (noLayersNotifier) LayersChanged(string, string) {}

type noTagHooks struct{}

func (noTagHooks) TagAdded(context.Context, string, string, tags.Tag)   {}
func (noTagHooks) TagRemoved(context.Context, string, string, tags.Tag) {}

var factsCtx = context.Background()

func (w *factsWorld) layerOn(page, name, field, op string, value any) {
	w.t.Helper()
	raw, _ := json.Marshal(value)
	_, err := w.layers.Add(factsCtx, w.camp, page, transformations.Viewer{UserID: w.owner, VisibilityRole: permissions.RoleOwner},
		transformations.AddInput{Name: name, Changes: []transformations.Change{{Field: field, Op: op, Value: raw}}})
	if err != nil {
		w.t.Fatal(err)
	}
}

func TestDecksFactsReadAsADirectorWouldSeeThem(t *testing.T) {
	w := newFactsWorld(t)
	got, err := w.facts.Numbers(factsCtx, w.camp, w.hero)
	if err != nil || !got.Sheet || got.Values["hp"] != 5 || got.Values["hpmax"] != 20 || len(got.Sources) != 0 || got.ChangedAt.IsZero() {
		t.Fatalf("stored numbers = %+v, %v", got, err)
	}
	if _, ok := got.Values["motto"]; ok {
		t.Error("a text field is not a number")
	}
	if _, ok := got.Values["stamina"]; ok {
		t.Error("a number field with no stored value must be absent")
	}

	// Layers apply in order: a set, then an add; an add on a missing base stays
	// missing; a list edit is not a number.
	w.layerOn(w.hero, "Revenant", "hp", "set", 12)
	w.layerOn(w.hero, "Blessed", "hp", "add", 3)
	w.layerOn(w.hero, "Hungry", "stamina", "add", 4)
	w.layerOn(w.hero, "Brand", "motto", "append", "x")
	got, err = w.facts.Numbers(factsCtx, w.camp, w.hero)
	if err != nil || got.Values["hp"] != 15 || got.Sources["hp"] != "layer" || got.Values["hpmax"] != 20 {
		t.Fatalf("with layers = %+v, %v", got, err)
	}
	if _, ok := got.Values["stamina"]; ok {
		t.Error("an add on a missing base must stay missing")
	}
	if _, ok := got.Sources["hpmax"]; ok {
		t.Error("an untouched stored value has no source")
	}

	layers, err := w.facts.Layers(factsCtx, w.camp, w.hero)
	if err != nil || len(layers) != 4 || layers[0].Name != "Revenant" {
		t.Fatalf("layers = %+v, %v", layers, err)
	}

	// Trackers: a Directors-only one is read, as a Director would.
	kind, err := w.trackers.CreateKind(factsCtx, w.camp, trackers.Viewer{UserID: w.owner, MemberRole: 3, VisibilityRole: 3, Director: true},
		trackers.KindInput{Name: "Doom", Shape: trackers.ShapeCount, Visibility: trackers.SeeDirectors})
	if err != nil {
		t.Fatal(err)
	}
	dirV := trackers.Viewer{UserID: w.owner, MemberRole: 3, VisibilityRole: 3, Director: true}
	tv, err := w.trackers.Place(factsCtx, w.camp, w.hero, dirV, trackers.PlaceInput{KindID: kind.ID, Visibility: trackers.SeeDirectors})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.trackers.Log(factsCtx, w.camp, w.hero, dirV, trackers.LogInput{TrackerID: tv.ID, Op: trackers.OpAdd, Amount: 3}); err != nil {
		t.Fatal(err)
	}
	trk, err := w.facts.Trackers(factsCtx, w.camp, w.hero)
	if err != nil || len(trk) != 1 || trk[0].KindID != kind.ID || trk[0].Value != 3 {
		t.Fatalf("trackers = %+v, %v", trk, err)
	}

	// Tags: a Director-only tag is read.
	secret, err := w.tags.Create(factsCtx, w.camp, "Secretly-a-spy", "#ff0000", true)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.tags.SetEntityTags(factsCtx, w.hero, w.camp, []int{secret.ID}); err != nil {
		t.Fatal(err)
	}
	tg, err := w.facts.Tags(factsCtx, w.camp, w.hero)
	if err != nil || len(tg) != 1 || tg[0].ID != secret.ID {
		t.Fatalf("tags = %+v, %v", tg, err)
	}

	f, err := w.facts.Fields(factsCtx, w.camp, w.hero)
	if err != nil || len(f) != 3 || f[0].Key != "hp" || f[0].MaxKey != "hpmax" {
		t.Fatalf("fields = %+v, %v", f, err)
	}

	opts, err := w.facts.Options(factsCtx, w.camp)
	if err != nil {
		t.Fatal(err)
	}
	if len(opts.Heroes) != 1 || opts.Heroes[0].ID != w.hero || len(opts.Fields) != 3 || len(opts.Trackers) != 1 || len(opts.Tags) != 1 || opts.Tags[0].Name != "Secretly-a-spy" {
		t.Errorf("options = %+v", opts)
	}
}

// A page that is gone or belongs to another campaign reads as an unavailable
// fact, quietly, and nothing of the other campaign is read.
func TestDecksFactsNeverReadAnotherCampaignsPage(t *testing.T) {
	w := newFactsWorld(t)
	missing := galleryTestUUID(t)
	for name, page := range map[string]string{"another campaign": w.away, "no such page": missing} {
		if _, err := w.facts.Numbers(factsCtx, w.camp, page); !errors.Is(err, decks.ErrUnavailable) {
			t.Errorf("%s: numbers err = %v", name, err)
		}
		if _, err := w.facts.Trackers(factsCtx, w.camp, page); !errors.Is(err, decks.ErrUnavailable) {
			t.Errorf("%s: trackers err = %v", name, err)
		}
		if _, err := w.facts.Tags(factsCtx, w.camp, page); !errors.Is(err, decks.ErrUnavailable) {
			t.Errorf("%s: tags err = %v", name, err)
		}
		if _, err := w.facts.Layers(factsCtx, w.camp, page); !errors.Is(err, decks.ErrUnavailable) {
			t.Errorf("%s: layers err = %v", name, err)
		}
		if _, err := w.facts.Fields(factsCtx, w.camp, page); !errors.Is(err, decks.ErrUnavailable) {
			t.Errorf("%s: fields err = %v", name, err)
		}
	}
	// A page with no stored sheet has none, and its numbers never satisfy a test.
	got, err := w.facts.Numbers(factsCtx, w.camp, w.npc)
	if err != nil || got.Sheet || len(got.Values) != 0 {
		t.Errorf("an empty sheet = %+v, %v", got, err)
	}
}

func TestDecksFactsWithAPluginDegradedAreUnavailableNotEmpty(t *testing.T) {
	w := newFactsWorld(t)
	w.facts.trackers, w.facts.layers = nil, nil
	if _, err := w.facts.Trackers(factsCtx, w.camp, w.hero); !errors.Is(err, decks.ErrUnavailable) {
		t.Errorf("trackers err = %v", err)
	}
	if _, err := w.facts.Layers(factsCtx, w.camp, w.hero); !errors.Is(err, decks.ErrUnavailable) {
		t.Errorf("layers err = %v", err)
	}
	// Numbers are judged with layers applied, so without the layers they are not judged.
	if _, err := w.facts.Numbers(factsCtx, w.camp, w.hero); !errors.Is(err, decks.ErrUnavailable) {
		t.Errorf("numbers err = %v", err)
	}
	opts, err := w.facts.Options(factsCtx, w.camp)
	if err != nil || len(opts.Trackers) != 0 || len(opts.Heroes) != 1 {
		t.Errorf("options = %+v, %v", opts, err)
	}
}

func TestStoredNumbers(t *testing.T) {
	tests := []struct {
		in   any
		want float64
		ok   bool
	}{
		{float64(3.5), 3.5, true}, {7, 7, true}, {int64(9), 9, true}, {json.Number("4"), 4, true}, {" 12 ", 12, true},
		{"", 0, false}, {"abc", 0, false}, {nil, 0, false}, {true, 0, false}, {[]any{1}, 0, false},
	}
	for _, tt := range tests {
		got, ok := storedNumber(tt.in)
		if ok != tt.ok || (ok && got != tt.want) {
			t.Errorf("storedNumber(%#v) = %v, %v", tt.in, got, ok)
		}
	}
}

func TestNumberFieldsHonourHiddenRetypedAndAddedFields(t *testing.T) {
	defs := []entities.FieldDefinition{{Key: "a", Label: "A", Type: "number"}, {Key: "b", Label: "B", Type: "number"},
		{Key: "c", Label: "C", Type: "text"}, {Key: "d", Label: "D", Type: "number", Play: &entities.FieldPlay{MaxField: "a"}}}
	text, number := "text", "number"
	ov := &entities.FieldOverrides{Hidden: []string{"b"}, Modified: map[string]entities.FieldOverride{"a": {Type: &text}, "c": {Type: &number}},
		Added: []entities.FieldDefinition{{Key: "e", Label: "E", Type: "number"}, {Key: "d", Label: "Again", Type: "number"}}}
	got := numberFields(defs, ov)
	keys := ""
	for _, f := range got {
		keys += f.Key
	}
	if keys != "cde" {
		t.Errorf("keys = %q, want cde", keys)
	}
	if got[1].MaxKey != "a" {
		t.Errorf("max key lost: %+v", got[1])
	}
}

// waitMoment polls the store for the number of open moments to reach want. It
// reads the store and not the Director's route, because that route looks again
// by itself every few seconds and would hide a hook that never fired.
func (w *factsWorld) waitMoment(want int) []decks.Moment {
	w.t.Helper()
	deadline := time.Now().Add(4 * time.Second)
	var last []decks.Moment
	for time.Now().Before(deadline) {
		list, err := w.moments.ListOpen(factsCtx, w.camp)
		if err != nil {
			w.t.Fatal(err)
		}
		last = list
		if len(list) == want {
			return list
		}
		time.Sleep(25 * time.Millisecond)
	}
	w.t.Fatalf("%d moments are open, want %d", len(last), want)
	return nil
}

// Each source of change reaches the watcher: a page save, a tag, a layer, a
// tracker, and the deck's own edits. The Director's own read looks again at
// most every few seconds, so every wait here is for the hook, not for that read.
func TestEachHookBringsTheMomentItWaitsFor(t *testing.T) {
	type step struct {
		name  string
		build func(w *factsWorld) decks.WhenList
		flip  func(w *factsWorld)
	}
	bl := func(v bool) *bool { return &v }
	steps := []step{
		{"a page save", func(w *factsWorld) decks.WhenList {
			v := 3.0
			return decks.WhenList{{Test: decks.TestNumber, Who: w.hero, Field: "hp", Op: "lte", Value: &v}}
		}, func(w *factsWorld) {
			if _, err := w.entities.Update(factsCtx, w.hero, entities.UpdateEntityInput{FieldsData: map[string]any{"hp": 2, "hpmax": 20, "motto": "Onward"}}); err != nil {
				t.Fatal(err)
			}
		}},
		{"a tag", func(w *factsWorld) decks.WhenList {
			tag, err := w.tags.Create(factsCtx, w.camp, "Cursed", "#111111", true)
			if err != nil {
				t.Fatal(err)
			}
			id := tag.ID
			cursedTag = id
			return decks.WhenList{{Test: decks.TestTag, Who: w.hero, Tag: &id, Has: bl(true)}}
		}, func(w *factsWorld) {
			if err := w.tags.SetEntityTags(factsCtx, w.hero, w.camp, []int{cursedTag}); err != nil {
				t.Fatal(err)
			}
		}},
		{"a layer", func(w *factsWorld) decks.WhenList {
			return decks.WhenList{{Test: decks.TestLayer, Who: w.hero, Name: "Revenant", On: bl(true)}}
		}, func(w *factsWorld) { w.layerOn(w.hero, "Revenant", "hp", "set", 1) }},
		{"a tracker", func(w *factsWorld) decks.WhenList {
			dirV := trackers.Viewer{UserID: w.owner, MemberRole: 3, VisibilityRole: 3, Director: true}
			kind, err := w.trackers.CreateKind(factsCtx, w.camp, dirV, trackers.KindInput{Name: "Doom", Shape: trackers.ShapeCount, Visibility: trackers.SeeDirectors})
			if err != nil {
				t.Fatal(err)
			}
			tv, err := w.trackers.Place(factsCtx, w.camp, w.hero, dirV, trackers.PlaceInput{KindID: kind.ID, Visibility: trackers.SeeDirectors})
			if err != nil {
				t.Fatal(err)
			}
			doomTracker = tv.ID
			v := 2.0
			return decks.WhenList{{Test: decks.TestTracker, Who: w.hero, Kind: kind.ID, Op: "gte", Value: &v}}
		}, func(w *factsWorld) {
			dirV := trackers.Viewer{UserID: w.owner, MemberRole: 3, VisibilityRole: 3, Director: true}
			if _, err := w.trackers.Log(factsCtx, w.camp, w.hero, dirV, trackers.LogInput{TrackerID: doomTracker, Op: trackers.OpAdd, Amount: 2}); err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, s := range steps {
		t.Run(s.name, func(t *testing.T) {
			w := newFactsWorld(t)
			deck, err := w.decks.Create(factsCtx, w.camp, w.dir, decks.CreateDeckInput{Name: "Omens", Kind: decks.KindEvent})
			if err != nil {
				t.Fatal(err)
			}
			id := deck.(decks.DirectorDeck).ID
			one := 1
			if _, err := w.decks.CreateCard(factsCtx, w.camp, id, w.dir, decks.CreateCardInput{Name: "Doom", Copies: &one, When: s.build(w)}); err != nil {
				t.Fatal(err)
			}
			// Nothing holds yet. Creating the card itself pokes the watcher; give it
			// time to run and make sure it raised nothing.
			time.Sleep(150 * time.Millisecond)
			if list, _ := w.moments.ListOpen(factsCtx, w.camp); len(list) != 0 {
				t.Fatalf("a moment came before the test held: %+v", list)
			}
			s.flip(w)
			got := w.waitMoment(1)
			if got[0].CardID == "" || got[0].DeckID != id {
				t.Errorf("moment = %+v", got[0])
			}
		})
	}
}

var cursedTag int
var doomTracker string

// Pokes in a burst run one recheck, and one that arrives during a recheck is
// not lost.
func TestThePokesOfABurstAreCoalescedPerCampaign(t *testing.T) {
	counter := &countingRechecker{}
	w := newDecksWatcher()
	w.delay = 30 * time.Millisecond
	w.bind(counter)
	for i := 0; i < 50; i++ {
		w.Poke("camp-a")
	}
	w.Poke("camp-b")
	w.Poke("")
	time.Sleep(200 * time.Millisecond)
	if a, b := counter.n("camp-a"), counter.n("camp-b"); a != 1 || b != 1 {
		t.Fatalf("rechecks: camp-a %d, camp-b %d, want 1 each", a, b)
	}
	w.Poke("camp-a")
	time.Sleep(150 * time.Millisecond)
	if counter.n("camp-a") != 2 {
		t.Errorf("a later poke was lost: %d", counter.n("camp-a"))
	}
	// Unbound (the plugin is degraded), a poke does nothing and is not remembered.
	idle := newDecksWatcher()
	idle.delay = 10 * time.Millisecond
	idle.Poke("camp-a")
	idle.bind(counter)
	idle.Poke("camp-a")
	time.Sleep(100 * time.Millisecond)
	if counter.n("camp-a") != 3 {
		t.Errorf("rechecks = %d", counter.n("camp-a"))
	}
}

type countingRechecker struct {
	mu    sync.Mutex
	calls map[string]int
}

func (c *countingRechecker) Recheck(_ context.Context, campaignID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.calls == nil {
		c.calls = map[string]int{}
	}
	c.calls[campaignID]++
	return nil
}

func (c *countingRechecker) n(campaignID string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls[campaignID]
}
