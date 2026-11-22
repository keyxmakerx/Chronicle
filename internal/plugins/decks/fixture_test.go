package decks

import (
	"context"
	"errors"
	"github.com/keyxmakerx/chronicle/internal/patch"
	"net/http"
	"testing"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

const camp = "camp-1"

// fixture is a campaign with a Director, two players who each own a hero, a
// Scribe with no hero, a former member and a public visitor.
type fixture struct {
	t                                       *testing.T
	repo                                    *memRepo
	svc                                     Service
	cap                                     *capture
	heroes                                  fakeHeroes
	members                                 fakeMembers
	pics                                    fakePictures
	dir, tam, ilse, scribe, former, visitor Viewer
}

var longAgo = time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)

func newFixture(t *testing.T) *fixture { return newFixtureWith(t) }

// newFixtureWith is newFixture with more service options, such as a story.
func newFixtureWith(t *testing.T, opts ...Option) *fixture {
	t.Helper()
	fx := &fixture{t: t, repo: newMemRepo(), cap: &capture{}}
	fx.heroes = fakeHeroes{
		pages: map[string]Hero{
			"h-tam":   {ID: "h-tam", Name: "Tam", OwnerUserID: "u-tam"},
			"h-ilse":  {ID: "h-ilse", Name: "Ilse", OwnerUserID: "u-ilse"},
			"h-vault": {ID: "h-vault", Name: "Hidden"},
			"h-npc":   {ID: "h-npc", Name: "Npc"},
		},
		hidden: map[string]bool{"h-vault": true},
	}
	members := fakeMembers{list: []Member{
		{UserID: "u-dir", Role: 3, Director: true, JoinedAt: longAgo},
		{UserID: "u-tam", Role: 1, JoinedAt: longAgo},
		{UserID: "u-ilse", Role: 1, JoinedAt: longAgo},
		{UserID: "u-scribe", Role: 2, JoinedAt: longAgo},
	}}
	fx.members = members
	fx.svc = NewService(fx.repo, fx.heroes, members, append([]Option{WithNotifier(fx.cap), WithBell(fx.cap), WithPictures(fx.pics), WithRandom(func(int) int { return 0 })}, opts...)...)
	fx.dir = Viewer{UserID: "u-dir", MemberRole: 3, VisibilityRole: 3, Director: true}
	fx.tam = Viewer{UserID: "u-tam", MemberRole: 1, VisibilityRole: 1}
	fx.ilse = Viewer{UserID: "u-ilse", MemberRole: 1, VisibilityRole: 1}
	fx.scribe = Viewer{UserID: "u-scribe", MemberRole: 2, VisibilityRole: 2}
	fx.former = Viewer{UserID: "u-former"}
	fx.visitor = Viewer{}
	return fx
}

// code is the HTTP status an error carries, 0 for nil.
func code(err error) int {
	if err == nil {
		return 0
	}
	var ae *apperror.AppError
	if errors.As(err, &ae) {
		return ae.Code
	}
	return -1
}

func (fx *fixture) must(err error) {
	fx.t.Helper()
	if err != nil {
		fx.t.Fatalf("unexpected error: %v", err)
	}
}

func ptr[T any](v T) *T { return &v }

// deck makes a deck as the Director and returns its id.
func (fx *fixture) deck(name, kind string, in *SettingsInput) string {
	fx.t.Helper()
	v, err := fx.svc.Create(nil2ctx(), camp, fx.dir, CreateDeckInput{Name: name, Kind: kind, Settings: in})
	fx.must(err)
	return v.(DirectorDeck).ID
}

// card adds a card and returns its id.
func (fx *fixture) card(deckID string, in CreateCardInput) string {
	fx.t.Helper()
	if in.Copies == nil {
		in.Copies = ptr(1)
	}
	_, err := fx.svc.CreateCard(nil2ctx(), camp, deckID, fx.dir, in)
	fx.must(err)
	for _, c := range fx.repo.cards {
		if c.DeckID == deckID && c.Name == in.Name {
			return c.ID
		}
	}
	fx.t.Fatalf("card %q not stored", in.Name)
	return ""
}

// copiesOf lists a card's copy ids in the order they were made.
func (fx *fixture) copiesOf(cardID string) []string {
	var out []string
	for _, c := range fx.repo.copies {
		if c.CardID == cardID {
			out = append(out, c.ID)
		}
	}
	return out
}

// place puts a copy straight into a spot, the way earlier moves would have.
func (fx *fixture) place(copyID, zone, holder string, faceUp bool) {
	fx.t.Helper()
	i := fx.repo.index(copyID)
	if i < 0 {
		fx.t.Fatalf("no copy %s", copyID)
	}
	fx.repo.copies[i].Zone, fx.repo.copies[i].HolderEntityID, fx.repo.copies[i].FaceUp = zone, holder, faceUp
	fx.repo.copies[i].MovedAt = fx.repo.tick()
}

func (fx *fixture) copy(copyID string) Copy {
	fx.t.Helper()
	i := fx.repo.index(copyID)
	if i < 0 {
		fx.t.Fatalf("no copy %s", copyID)
	}
	return fx.repo.copies[i]
}

func (fx *fixture) choose(deckID string, players ...PlayerInput) {
	fx.t.Helper()
	d := fx.repo.decks[deckID]
	_, err := fx.svc.SetPlayers(nil2ctx(), camp, deckID, fx.dir, SetPlayersInput{Version: d.Version, Players: players})
	fx.must(err)
}

var _ = http.StatusOK

func nil2ctx() context.Context { return context.Background() }

func patchOf[T any](v T) patch.Field[T] { return patch.Of(v) }
