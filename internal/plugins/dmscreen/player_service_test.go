package dmscreen

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/keyxmakerx/chronicle/internal/systems"
)

// fakePlayerParty records who asked and returns its heroes as the players'
// party source would: already cut to the pages the asker may open.
type fakePlayerParty struct {
	heroes []Hero
	err    error
	asked  Viewer
}

func (f *fakePlayerParty) PlayerHeroes(_ context.Context, _ string, v Viewer) ([]Hero, error) {
	f.asked = v
	return f.heroes, f.err
}

// fakePlayerWorld records the viewer it was asked as.
type fakePlayerWorld struct {
	w     *WorldView
	asked Viewer
}

func (f *fakePlayerWorld) World(_ context.Context, _ string, v Viewer) (*WorldView, error) {
	f.asked = v
	if f.w == nil {
		return nil, nil
	}
	c := *f.w
	return &c, nil
}
func (f *fakePlayerWorld) Advance(context.Context, string, Viewer, int, int) error { return nil }

// fakePlayerNotes keeps one note per user, as the adapter does by id.
type fakePlayerNotes struct {
	byUser   map[string]*ScreenNote
	asked    []string
	saveErr  error
	savedVer string
}

func (f *fakePlayerNotes) FindMine(_ context.Context, _ string, v Viewer) (*ScreenNote, error) {
	f.asked = append(f.asked, v.UserID)
	return f.byUser[v.UserID], nil
}

func (f *fakePlayerNotes) SaveMine(_ context.Context, _ string, v Viewer, title, entry, _, version string) (*ScreenNote, error) {
	f.asked = append(f.asked, v.UserID)
	if f.saveErr != nil {
		return nil, f.saveErr
	}
	f.savedVer = version
	if f.byUser == nil {
		f.byUser = map[string]*ScreenNote{}
	}
	f.byUser[v.UserID] = &ScreenNote{ID: "n-" + v.UserID, Title: title, Entry: entry}
	return f.byUser[v.UserID], nil
}

// partyHeroes is a two-hero party: the viewer's own (u1) and another
// player's (u2), each with a GM-only and an owner-only value on its sheet.
func partyHeroes() []Hero {
	sheet := func(stamina string) map[string]any {
		return map[string]any{
			"stamina_current": stamina, "stamina_max": "30", "class": "Tactician", "level": 3.0,
			"gm_secret": "the bell-wraith is her sister", "backstory": "stitched inside the coat",
			"conditions_json": `["bleeding"]`,
		}
	}
	return []Hero{
		{ID: "h1", Name: "Orrin", PlayerUserID: "u1", PlayerName: "Ana", Fields: sheet("24"),
			GMOnly: []string{"gm_secret"}, OwnerOnly: []string{"backstory"}, RestrictionsKnown: true},
		{ID: "h2", Name: "Tyne", PlayerUserID: "u2", PlayerName: "Bo", Fields: sheet("12"),
			GMOnly: []string{"gm_secret"}, OwnerOnly: []string{"backstory", "class"}, RestrictionsKnown: true},
	}
}

func TestPlayerScreen_Roles(t *testing.T) {
	svc := NewService(Sources{PlayerNotes: &fakePlayerNotes{}})
	ctx := context.Background()
	tests := []struct {
		name    string
		v       Viewer
		wantErr bool
	}{
		{"player", Viewer{UserID: "u1", Role: 1}, false},
		{"scribe", Viewer{UserID: "u1", Role: 2}, false},
		{"owner or co-DM (gets the DM Screen)", Viewer{UserID: "u1", Role: 3}, true},
		{"non-member", Viewer{UserID: "u1", Role: 0}, true},
		{"no user", Viewer{Role: 1}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.BuildPlayer(ctx, "c1", tt.v)
			if (err != nil) != tt.wantErr {
				t.Fatalf("BuildPlayer err = %v, wantErr %v", err, tt.wantErr)
			}
			_, err = svc.PlayerNote(ctx, "c1", tt.v)
			if (err != nil) != tt.wantErr {
				t.Fatalf("PlayerNote err = %v, wantErr %v", err, tt.wantErr)
			}
			_, err = svc.SavePlayerNote(ctx, "c1", tt.v, "hi", "")
			if (err != nil) != tt.wantErr {
				t.Fatalf("SavePlayerNote err = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && appErrCode(err) != http.StatusForbidden {
				t.Errorf("refusal code = %d, want 403", appErrCode(err))
			}
		})
	}
}

func TestBuildPlayer_Party(t *testing.T) {
	party := &fakePlayerParty{heroes: partyHeroes()}
	svc := NewService(Sources{PlayerParty: party, System: fakeSystemSource{fakeSystem{m: drawSteel}}})
	v, err := svc.BuildPlayer(context.Background(), "c1", Viewer{UserID: "u1", Role: 1})
	if err != nil {
		t.Fatal(err)
	}
	if party.asked.Role != 1 || party.asked.UserID != "u1" {
		t.Errorf("party asked as %+v, want the player themself", party.asked)
	}

	t.Run("only the viewer's own hero is My hero", func(t *testing.T) {
		if v.Hero == nil || v.Hero.ID != "h1" {
			t.Fatalf("Hero = %+v, want h1", v.Hero)
		}
		if len(v.Party) != 1 || v.Party[0].ID != "h2" {
			t.Fatalf("Party = %+v, want only h2", v.Party)
		}
		if v.Hero.Level != "3" || v.Hero.HeroLine() != "Tactician, level 3" {
			t.Errorf("hero line = %q", v.Hero.HeroLine())
		}
		if len(v.Hero.Conditions) != 1 || v.Hero.Conditions[0] != "Bleeding" {
			t.Errorf("conditions = %v", v.Hero.Conditions)
		}
	})

	t.Run("an owner-only value shows on the owner's own hero only", func(t *testing.T) {
		if v.Party[0].Subtitle != "" {
			t.Errorf("another player's owner-only class leaked: %q", v.Party[0].Subtitle)
		}
		if v.Hero.Subtitle != "Tactician" {
			t.Errorf("own hero's class = %q, want it shown", v.Hero.Subtitle)
		}
	})

	t.Run("a hero whose restrictions are unknown shows no values", func(t *testing.T) {
		hs := partyHeroes()
		hs[1].RestrictionsKnown = false
		svc := NewService(Sources{PlayerParty: &fakePlayerParty{heroes: hs}, System: fakeSystemSource{fakeSystem{m: drawSteel}}})
		v, err := svc.BuildPlayer(context.Background(), "c1", Viewer{UserID: "u1", Role: 1})
		if err != nil {
			t.Fatal(err)
		}
		if p := v.Party[0]; len(p.Meters) != 0 || p.Subtitle != "" || p.Level != "" || len(p.Conditions) != 0 {
			t.Errorf("hero with unknown restrictions showed values: %+v", p)
		}
	})

	t.Run("with several heroes, the most recently played is My hero", func(t *testing.T) {
		now := time.Now()
		hs := partyHeroes()
		hs[0].UpdatedAt = now.Add(-time.Hour)
		hs = append(hs, Hero{ID: "h3", Name: "Ash", PlayerUserID: "u1", UpdatedAt: now, RestrictionsKnown: true})
		mine, others := playerParty(nil, hs, "u1")
		if mine == nil || mine.ID != "h3" || len(others) != 2 {
			t.Fatalf("mine = %+v, others = %d", mine, len(others))
		}
	})

	t.Run("no hero of their own leaves My hero empty, not someone else's", func(t *testing.T) {
		mine, others := playerParty(nil, partyHeroes(), "u9")
		if mine != nil || len(others) != 2 {
			t.Fatalf("mine = %+v, others = %d", mine, len(others))
		}
	})
}

// A GM-only value never reaches the drawn screen, on anyone's hero.
func TestPlayerPanel_NeverDrawsGMOnly(t *testing.T) {
	hs := partyHeroes()
	// The system draws the secret as a meter label and the subtitle, so a leak
	// would print it.
	m := *drawSteel
	dm := *m.DMScreen
	dm.Party = append([]systems.DMScreenMeter{{LabelField: "gm_secret", Current: "stamina_current"}}, dm.Party...)
	dm.HeroSubtitle = "gm_secret"
	m.DMScreen = &dm
	svc := NewService(Sources{PlayerParty: &fakePlayerParty{heroes: hs}, System: fakeSystemSource{fakeSystem{m: &m}}})
	v, err := svc.BuildPlayer(context.Background(), "c1", Viewer{UserID: "u1", Role: 1})
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := PlayerPanel(v).Render(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	html := buf.String()
	if strings.Contains(html, "sister") {
		t.Error("a GM-only sheet value was drawn on the players' screen")
	}
	if strings.Contains(html, "stitched") {
		t.Error("an owner-only value was drawn (the screen never draws backstory at all)")
	}
	for _, want := range []string{`data-dms-open="hero"`, `data-dms-open="party"`, `data-tt-mount="rolls"`, `data-tt-mount="tonight"`, `data-tt-mount="dice"`, "Orrin", "Tyne"} {
		if !strings.Contains(html, want) {
			t.Errorf("panel is missing %s", want)
		}
	}
}

func TestBuildPlayer_World(t *testing.T) {
	world := &fakePlayerWorld{w: &WorldView{CalendarID: "cal", DateLabel: "14 Rainmoot 1189", TimeLabel: "Evening", Weather: "Light rain", CanStep: true}}
	svc := NewService(Sources{World: world})
	v, err := svc.BuildPlayer(context.Background(), "c1", Viewer{UserID: "u1", Role: 2})
	if err != nil {
		t.Fatal(err)
	}
	// The calendar is read at the players' audience: as the scribe themself,
	// never raised to the Director's view.
	if world.asked.Role != 2 || world.asked.UserID != "u1" {
		t.Errorf("calendar asked as %+v, want the viewer at their own role", world.asked)
	}
	if v.World == nil || v.World.CanStep {
		t.Fatalf("World = %+v, want it shown and never steppable", v.World)
	}
	if got := sectionIDs(v.PlayerSections()); !strings.Contains(got, "world") {
		t.Errorf("sections %s lack Today", got)
	}

	t.Run("no calendar leaves Today out", func(t *testing.T) {
		svc := NewService(Sources{World: &fakePlayerWorld{}})
		v, _ := svc.BuildPlayer(context.Background(), "c1", Viewer{UserID: "u1", Role: 1})
		if got := sectionIDs(v.PlayerSections()); strings.Contains(got, "world") {
			t.Errorf("sections %s include Today with no calendar", got)
		}
	})
}

func sectionIDs(secs []SectionView) string {
	ids := make([]string, len(secs))
	for i, s := range secs {
		ids[i] = s.ID
	}
	return strings.Join(ids, ",")
}

func TestPlayerSections_Order(t *testing.T) {
	tests := []struct {
		name string
		v    PlayerView
		want string
	}{
		{"nothing but the Table", PlayerView{}, "tonight,rolls,dice"},
		{"everything", PlayerView{
			HasParty: true, World: &WorldView{DateLabel: "d"},
			Conditions: []ConditionView{{Name: "Bleeding"}}, Notes: &NotesView{Label: "Only you can see this"},
		}, "hero,party,world,tonight,rolls,dice,rules,notes"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sectionIDs(tt.v.PlayerSections()); got != tt.want {
				t.Errorf("sections = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestBuildPlayer_PowerRoll(t *testing.T) {
	m := *drawSteel
	m.Crafting = &systems.CraftingRules{Dice: "2d10", Tiers: []int{11, 16},
		Characteristics: []systems.CraftingStat{{Key: "might", Label: "Might"}}}
	if _, ok := m.PowerRoll(); !ok {
		t.Fatal("test manifest does not declare a power roll")
	}
	svc := NewService(Sources{PlayerParty: &fakePlayerParty{heroes: partyHeroes()}, System: fakeSystemSource{fakeSystem{m: &m}}})
	v, _ := svc.BuildPlayer(context.Background(), "c1", Viewer{UserID: "u1", Role: 1})
	if v.PowerRoll == "" {
		t.Fatal("a system with a power roll offers none")
	}
	var buf bytes.Buffer
	_ = PlayerPanel(v).Render(context.Background(), &buf)
	if !strings.Contains(buf.String(), "data-pls-power") {
		t.Error("My hero has no Power roll word")
	}
}

func TestPlayerNotes(t *testing.T) {
	ctx := context.Background()

	t.Run("each player reads and writes only their own note", func(t *testing.T) {
		notes := &fakePlayerNotes{byUser: map[string]*ScreenNote{
			"u2": {ID: "n-u2", Entry: `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"u2 secret"}]}]}`},
		}}
		svc := NewService(Sources{PlayerNotes: notes})
		nv, err := svc.PlayerNote(ctx, "c1", Viewer{UserID: "u1", Role: 1})
		if err != nil {
			t.Fatal(err)
		}
		if nv.Text != "" || nv.NoteID != "" {
			t.Errorf("u1 read %+v, want an empty note of their own", nv)
		}
		if _, err := svc.SavePlayerNote(ctx, "c1", Viewer{UserID: "u1", Role: 1}, "mine", ""); err != nil {
			t.Fatal(err)
		}
		for _, u := range notes.asked {
			if u != "u1" {
				t.Fatalf("the note source was asked for %q", u)
			}
		}
		if notes.byUser["u2"].ID != "n-u2" || strings.Contains(notes.byUser["u1"].Entry, "u2 secret") {
			t.Error("a save touched another player's note")
		}
	})

	t.Run("first save creates titled My notes and round-trips the text", func(t *testing.T) {
		notes := &fakePlayerNotes{}
		svc := NewService(Sources{PlayerNotes: notes})
		nv, err := svc.SavePlayerNote(ctx, "c1", Viewer{UserID: "u1", Role: 1}, "Ask Mara about the sigil\nWho pulls the bell?", "")
		if err != nil {
			t.Fatal(err)
		}
		if notes.byUser["u1"].Title != playerNoteTitle {
			t.Errorf("title = %q", notes.byUser["u1"].Title)
		}
		if nv.Text != "Ask Mara about the sigil\nWho pulls the bell?" || nv.Label != "Only you can see this" || nv.Version == "" {
			t.Errorf("view = %+v", nv)
		}
		if nv.Link != "/campaigns/c1/journal/n-u1" {
			t.Errorf("link = %q", nv.Link)
		}
	})

	t.Run("a shared note says so", func(t *testing.T) {
		notes := &fakePlayerNotes{byUser: map[string]*ScreenNote{"u1": {ID: "n", Shared: true}}}
		nv, _ := NewService(Sources{PlayerNotes: notes}).PlayerNote(ctx, "c1", Viewer{UserID: "u1", Role: 1})
		if nv.Label != "Shared from your Journal" {
			t.Errorf("label = %q", nv.Label)
		}
	})

	t.Run("a formatted note is read-only and refuses a save", func(t *testing.T) {
		notes := &fakePlayerNotes{byUser: map[string]*ScreenNote{"u1": {ID: "n", Entry: `{"type":"doc","content":[{"type":"heading","content":[{"type":"text","text":"H"}]}]}`}}}
		svc := NewService(Sources{PlayerNotes: notes})
		nv, _ := svc.PlayerNote(ctx, "c1", Viewer{UserID: "u1", Role: 1})
		if !nv.ReadOnly {
			t.Error("formatted note not read-only")
		}
		if _, err := svc.SavePlayerNote(ctx, "c1", Viewer{UserID: "u1", Role: 1}, "x", nv.Version); appErrCode(err) != http.StatusUnprocessableEntity {
			t.Errorf("save err = %v, want 422", err)
		}
	})

	t.Run("a stale version is a conflict", func(t *testing.T) {
		notes := &fakePlayerNotes{saveErr: ErrNoteChanged}
		_, err := NewService(Sources{PlayerNotes: notes}).SavePlayerNote(ctx, "c1", Viewer{UserID: "u1", Role: 1}, "x", "old")
		if !errors.Is(err, ErrNoteChanged) {
			t.Errorf("err = %v, want ErrNoteChanged", err)
		}
	})

	t.Run("too long is refused before the store", func(t *testing.T) {
		notes := &fakePlayerNotes{}
		_, err := NewService(Sources{PlayerNotes: notes}).SavePlayerNote(ctx, "c1", Viewer{UserID: "u1", Role: 1}, strings.Repeat("a", maxNoteChars+1), "")
		if appErrCode(err) != http.StatusBadRequest || len(notes.asked) != 0 {
			t.Errorf("err = %v, asked %v", err, notes.asked)
		}
	})

	t.Run("no notes source: the card is left out and the routes say not found", func(t *testing.T) {
		svc := NewService(Sources{})
		v, _ := svc.BuildPlayer(ctx, "c1", Viewer{UserID: "u1", Role: 1})
		if v.Notes != nil {
			t.Error("notes card drawn without a source")
		}
		if _, err := svc.PlayerNote(ctx, "c1", Viewer{UserID: "u1", Role: 1}); appErrCode(err) != http.StatusNotFound {
			t.Errorf("err = %v, want 404", err)
		}
	})
}
